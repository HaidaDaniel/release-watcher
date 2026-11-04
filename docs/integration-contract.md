# Integration contract

Status: version 1.0 contract. The CLI JSON envelope and event schema are the stable interfaces for agents and Media Core. Implementation lands in tasks 0005–0007; this document is the target and must be updated if the implementation intentionally changes a field.

## Contract versions

- CLI envelope: `schema_version: "1.0"`.
- Event: `schema_version: "1.0"`.
- Consumers must ignore unknown fields and must branch on the schema version, not on prose.
- Additive fields are allowed within a major version. Removing/renaming a field or changing its meaning requires a new major version.
- No field is localized. Localized human text may appear in `message`, but machine codes and names are English.

## CLI invocation

Every command supports `--json`. Read and mutation commands return one JSON document on stdout. Diagnostics and logs go to stderr. Exit codes:

| Exit | Meaning |
| --- | --- |
| 0 | success |
| 1 | runtime/configuration failure |
| 2 | usage error (unknown command, missing argument, invalid flag) |
| 3 | partial scan: the scan completed but at least one watch failed |
| 4 | a scan lock is held by another process |

Human output is the default. JSON output is the contract for agents. A command must not mix human text into the JSON stdout stream.

## Success envelope

```json
{
  "schema_version": "1.0",
  "ok": true,
  "data": {}
}
```

`data` is command-specific. Examples below.

## Error envelope

```json
{
  "schema_version": "1.0",
  "ok": false,
  "error": {
    "code": "invalid_url",
    "message": "Некорректный URL.",
    "details": {}
  }
}
```

Stable error codes:

| Code | Meaning |
| --- | --- |
| `usage` | invalid command or argument |
| `invalid_url` | URL is malformed or unsupported scheme |
| `unsupported_source` | URL is not a Narō URL and is not a valid RSS/Atom feed |
| `fetch_failed` | source request/parse failed |
| `source_timeout` | source exceeded its timeout |
| `rate_limited` | source returned 429 or pacing denied the request |
| `source_not_found` | source returned 404 |
| `source_forbidden` | source returned 401/403 |
| `ssrf_blocked` | target resolves to a disallowed address |
| `not_found` | watch/publication/event id not found |
| `invalid_argument` | a flag or value is invalid (for example an interval below 1m) |
| `locked` | another scan holds the lock |
| `storage_error` | SQLite/migration/backup failure |
| `notification_failed` | delivery attempt failed (events remain stored) |
| `config_error` | invalid environment/configuration |
| `internal_error` | unexpected failure |

A repeated `add` is not an error: it returns `ok: true`, `created: false`, and `data.reason = "already_exists"`.

## `version`

```json
{
  "schema_version": "1.0",
  "ok": true,
  "data": {
    "name": "release-watcher",
    "version": "0.1.0-dev",
    "commit": "unknown",
    "built_at": "unknown",
    "build_mode": "development",
    "go_version": "go1.27.1"
  }
}
```

## `add`

Input: `release-watcher add <url> [--title-ru ...] [--title-en ...] [--type wn|ln|translation|other|unknown] [--interval 24h] [--json]`.

Success `data`:

```json
{
  "watch_id": "watch_...",
  "publication_id": "pub_...",
  "work_id": "work_...",
  "created": true,
  "original_title": "夢見る男子は現実主義者",
  "title_ru": null,
  "title_en": null,
  "publication_type": "wn",
  "source_type": "naro",
  "original_url": "https://ncode.syosetu.com/n2166fe/",
  "external_work_reference": "ncode:N2166FE",
  "status": "ongoing",
  "status_known": true,
  "status_source": "naro:end",
  "last_published_at": "2023-08-01T13:25:09Z",
  "last_checked_at": "2026-11-04T12:00:00Z",
  "next_check_at": "2026-11-05T12:00:00Z",
  "interval_seconds": 86400,
  "baseline_releases": 210,
  "mode": "catalog",
  "fields_available": ["title", "status", "release_count", "release_catalog"],
  "warnings": []
}
```

A repeated `add` for the same source line returns `ok: true`, `created: false`, the existing watch, and `data.reason = "already_exists"` (not an error envelope), so automation can be idempotent. Re-adding a previously removed URL returns `created: false` with `data.reason = "restored"` and re-enables the existing watch. The existing watch is resolved locally before any network request, so a duplicate add works even when the source is currently unavailable. It never creates a second watch or a baseline event.

## `list` and `show`

`list` returns watches, not events:

```json
{
  "watches": [
    {
      "watch_id": "watch_...",
      "publication_id": "pub_...",
      "work_id": "work_...",
      "original_title": "...",
      "title_ru": null,
      "title_en": null,
      "publication_type": "wn",
      "source_type": "naro",
      "original_url": "...",
      "status": "ongoing",
      "status_known": true,
      "status_source": "naro:end",
      "last_published_at": "...",
      "enabled": true,
      "interval_seconds": 86400,
      "last_checked_at": "...",
      "last_success_at": "...",
      "next_check_at": "...",
      "removed_at": null,
      "last_error_category": null,
      "release_count": 210,
      "mode": "catalog",
      "owner": null
    }
  ]
}
```

`show <watch-id>` returns one nested document. `releases` are bounded (200) and newest first; `events` (20) and `scans` (10) are recent history. Facts, computed values and nulls stay distinct. A missing localized title is `null`, never a fabricated string.

```json
{
  "watch": {"watch_id": "watch_...", "work_id": "work_...", "...": "..."},
  "work": {"id": "work_...", "original_title": "...", "title_ru": null, "title_en": null},
  "publication": {"id": "pub_...", "publication_type": "wn", "status": "ongoing", "status_known": true},
  "source": {"id": "src_...", "kind": "naro", "uri": "...", "external_id": "N2166FE"},
  "releases": [{"id": "rel_...", "external_id": "N2166FE/1", "kind": "chapter", "is_baseline": true}],
  "events": [],
  "scans": [{"id": "scan_...", "result": "baseline", "events_created": 0}]
}
```

## `scan`

Input: `scan [--due|--all|--id <watch-id>] [--no-notify] [--json] [--log-json]`. `--no-notify` stores events and deliveries without sending them in this run; notification counts still report the pending outbox, and a later `notify` flushes it.

Success `data`:

```json
{
  "started_at": "...",
  "finished_at": "...",
  "scanned": 2,
  "succeeded": 1,
  "failed": 1,
  "events_created": 1,
  "notifications": {"pending": 0, "sent": 1, "failed": 0},
  "results": [
    {
      "watch_id": "watch_...",
      "publication_id": "pub_...",
      "outcome": "ok",
      "events_created": 1,
      "mode": "catalog",
      "error": null,
      "duration_ms": 412
    },
    {
      "watch_id": "watch_...",
      "publication_id": "pub_...",
      "outcome": "error",
      "events_created": 0,
      "mode": null,
      "error": {"code": "source_timeout", "message": "..."},
      "duration_ms": 30001
    }
  ]
}
```

One watch failure never prevents the rest. The process exits 3 for a partial failure; the JSON is still complete. The lock error exits 4. Notification counts reflect the outbox after the scan: pending, sent and failed. Events are durable regardless of Telegram.

## `events`

Input: `events [--json] [--limit N] [--since RFC3339] [--publication <id>] [--watch <id>] [--after <event-id>]`.

Event listing is ordered `detected_at ASC, event_id ASC`; `show` uses the newest events. The dedupe key includes the detection time so a later legitimate recurrence (for example `ongoing → completed → ongoing → completed`) is not suppressed. State-based replay safety comes from committing each scan and its events atomically; a replayed scan with no new state emits nothing. `--since` is inclusive; `--after` is an opaque cursor for repeated reads. Repeated reads are safe: events are immutable and `event_id` is stable. Consumers should deduplicate by `event_id` even when paging.

```json
{
  "schema_version": "1.0",
  "ok": true,
  "data": {
    "next_cursor": "evt_...",
    "events": [
    {
      "schema_version": "1.0",
      "event_id": "evt_...",
      "event_type": "release.added",
      "source_id": "src_...",
      "publication_id": "pub_...",
      "watch_id": "watch_...",
      "external_work_reference": "ncode:N2166FE",
      "work": {
        "original_title": "夢見る男子は現実主義者",
        "title_ru": null,
        "title_en": null,
        "publication_type": "wn"
      },
      "publication": {
        "source_type": "naro",
        "original_url": "https://ncode.syosetu.com/n2166fe/",
        "status": "ongoing",
        "status_known": true
      },
      "release_ids": ["N2166FE/211", "N2166FE/212", "N2166FE/213"],
      "releases": [
        {
          "external_id": "N2166FE/211",
          "kind": "chapter",
          "number": "211",
          "title": "...",
          "url": "https://ncode.syosetu.com/n2166fe/211/",
          "published_at": "..."
        }
      ],
      "previous_count": null,
      "current_count": null,
      "count_delta": null,
      "published_at": "...",
      "detected_at": "...",
      "url": "https://ncode.syosetu.com/n2166fe/",
      "metadata_changes": null,
      "dedupe_key": "..."
    }
    ]
  }
}
```

Event types:

- `release.added` — one or more new release identities (or a positive count delta in count-only mode).
- `release.updated` — an existing release's title/URL/time changed.
- `release.removed` — only with authoritative catalog data; never inferred from an RSS window.
- `publication.metadata_changed` — title/URL/publication metadata changed.
- `publication.status_changed` — explicit source status changed.
- `release.count_changed` — count-only source count changed decreasingly or without identities.

Count-only events have `release_ids: []`, a non-null `previous_count`/`current_count`/`count_delta`, and no invented numbers or URLs. An empty RSS/Atom window is not a count change and emits no event. A count-only source that recovers to a complete catalog is reconciled silently; a partial recovery stays count-only.

## Narō status semantics

The Narō novel API `end` field is authoritative only for the values documented by the official specification (`https://dev.syosetu.com/man/api/`): `0` means a short story or completed work, and `1` means ongoing. A missing, `null` or unsupported value leaves `status` as `unknown` and `status_source` empty. Inactivity and an old `general_lastup` never imply completion. A `completed` watch remains enabled because an author may resume publication.

## `pause`, `resume`, `interval`, `remove`, `purge`

`pause`, `resume` and `interval` return the affected watch object in `data` (same fields as `list`). `interval --global 24h` returns `{"global_interval_seconds": 86400}` and updates every watch still using the previous default interval; watches with a custom interval keep it. `interval <id> 24h` validates a duration of at least 1m and stores it in seconds.

`remove <id> --yes` stops watching but preserves history. It returns `{"removed": true, "history_preserved": true, "watch_id": "...", "publication_id": "..."}`. The watch is disabled, disappears from `list`, and its publication line, releases, events, deliveries and scan runs remain readable through `show` and `events`; `event_id` values stay stable. Re-adding the same URL restores the same watch (`created: false`, `reason: "restored"`) without duplicating releases or replaying old chapters. Removed watches cannot be scanned, paused, resumed or have their interval changed until re-added.

`purge <id> --yes` is the explicit destructive operation and returns `{"purged": true, "watch_id": "...", "publication_id": "..."}`. It deletes the watch, publication line, releases, events, deliveries and scan runs in one transaction. Work/Source rows are removed only when unreferenced. Neither command touches another publication line.

## `notify`

`notify [--json]` flushes due deliveries and returns `{"pending":N,"sent":N,"failed":N}`. It uses the same process lock as `scan`, so a manual notify cannot race a timer scan into sending the same delivery twice. It requires Telegram configuration; without it the command returns `config_error`. Sending is bounded by `RELEASE_WATCHER_MAX_DELIVERY_ATTEMPTS`; 429 uses `retry_after`, other 5xx/network failures use bounded exponential backoff even when the error body is not JSON, and permanent 4xx failures are not retried. A delivery is unique per `(event_id, channel)`, so a repeated scan/flush never duplicates a message. Deliveries are created only for events detected while Telegram is configured; historical events are not sent retroactively. Events remain readable if every attempt fails.

Messages are grouped per event (a multi-chapter event becomes one message), use RU by default and EN with `RELEASE_WATCHER_LANG=en`, never include the bot token, and are truncated below Telegram's 4096-character limit.

## `doctor`, `backup`, `restore`

- `doctor --json` is implemented as a read-only configuration/environment check and returns `{"healthy":true|false,"checks":[{"id":"...","status":"ok|warn|error","message":"..."}]}`. It does not migrate and includes a read-only schema-version and database-permission probe. On a closed database it does not create WAL sidecars; a WAL is inspected through a temporary copy. Stop writers for an authoritative check.
- `backup --out <path> [--force] --json` returns `{"path":"...","schema_version":N,"bytes":N,"created_at":"..."}`. The archive contains `manifest.json` (format, schema, app version, SHA-256, size) and `release-watcher.db`.
- `restore --in <path> --force --json` validates format, size, hash and schema compatibility, acquires the scan/notify lock and replaces the database. It returns `{"path":"...","schema_version":N,"bytes":N}`. It refuses a newer schema and requires `--force` when a database exists.

## Media Core import

Media Core reads `events --json` (or a file export produced from the same contract) and stores events by `event_id`. It must not open Release Watcher's SQLite file. The two projects may run on different hosts. A repeated import of the same `event_id` is a no-op. `release_ids` are scoped to one publication; Media Core must not assume they are globally unique or that numbering is contiguous.

## Compatibility examples

- An agent that only understands `release.added` can ignore all other event types.
- An agent that does not understand `releases` can use `release_ids` and `url`.
- A consumer that sees `schema_version: "2.0"` must stop and report an unsupported contract rather than guessing.
