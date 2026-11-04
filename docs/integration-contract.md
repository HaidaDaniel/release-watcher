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
| `already_exists` | add matched an existing publication; `data` contains the existing watch |
| `locked` | another scan holds the lock |
| `storage_error` | SQLite/migration/backup failure |
| `notification_failed` | delivery attempt failed (events remain stored) |
| `config_error` | invalid environment/configuration |
| `internal_error` | unexpected failure |

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
  "status": "completed",
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

A repeated `add` for the same source line returns `ok: true`, `created: false`, the existing watch, and code `already_exists` in `data.reason` (not an error envelope), so automation can be idempotent. It never creates a second watch or a baseline event.

## `list` and `show`

`list` returns watches, not events:

```json
{
  "watches": [
    {
      "watch_id": "watch_...",
      "publication_id": "pub_...",
      "original_title": "...",
      "title_ru": null,
      "title_en": null,
      "publication_type": "wn",
      "source_type": "naro",
      "original_url": "...",
      "status": "completed",
      "status_known": true,
      "enabled": true,
      "interval_seconds": 86400,
      "last_checked_at": "...",
      "last_success_at": "...",
      "next_check_at": "...",
      "last_error_category": null,
      "release_count": 210,
      "mode": "catalog",
      "owner": null
    }
  ]
}
```

`show <watch-id>` returns one watch plus `publication`, `releases` (bounded, newest first) and recent `events` (bounded). Facts, computed values and nulls are kept distinct. A missing localized title is `null`, never a fabricated string.

## `scan`

Input: `scan [--due|--all|--id <watch-id>] [--no-notify] [--json] [--log-json]`.

Success `data`:

```json
{
  "started_at": "...",
  "finished_at": "...",
  "scanned": 2,
  "succeeded": 1,
  "failed": 1,
  "events_created": 1,
  "notifications": {"pending": 1, "sent": 0, "failed": 0},
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

One watch failure never prevents the rest. The process exits 3 for a partial failure; the JSON is still complete. The lock error exits 4.

## `events`

Input: `events [--json] [--limit N] [--since RFC3339] [--publication <id>] [--watch <id>] [--after <event-id>]`.

Ordering is stable: `detected_at ASC, event_id ASC`. `--since` is inclusive; `--after` is an opaque cursor for repeated reads. Repeated reads are safe: events are immutable and `event_id` is stable. Consumers should deduplicate by `event_id` even when paging.

```json
{
  "schema_version": "1.0",
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
        "status": "completed",
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
```

Event types:

- `release.added` — one or more new release identities (or a positive count delta in count-only mode).
- `release.updated` — an existing release's title/URL/time changed.
- `release.removed` — only with authoritative catalog data; never inferred from an RSS window.
- `publication.metadata_changed` — title/URL/publication metadata changed.
- `publication.status_changed` — explicit source status changed.
- `release.count_changed` — count-only source count changed decreasingly or without identities.

Count-only events have `release_ids: []`, a non-null `previous_count`/`current_count`/`count_delta`, and no invented numbers or URLs.

## `pause`, `resume`, `interval`, `remove`

All return the affected watch (or a bounded removal summary) in `data`. `interval` validates a duration and stores it in seconds. `remove` requires `--yes` for non-interactive use and returns `removed: true` plus the ids removed. Removing one watch never touches another publication line.

## `doctor`, `backup`, `restore`, `notify`

- `doctor --json` returns `{"checks":[{"id":"...","status":"ok|warn|error","message":"..."}],"healthy":true|false}`. It is read-only and never migrates.
- `backup --out <path> --json` returns `{"path":"...","schema_version":...,"bytes":N,"created_at":"..."}`.
- `restore --in <path> --force --json` returns the restored schema version and a warning that the process must not be running. It fails closed on schema incompatibility.
- `notify --json` flushes due deliveries and returns counts plus per-delivery outcomes without secrets.

## Media Core import

Media Core reads `events --json` (or a file export produced from the same contract) and stores events by `event_id`. It must not open Release Watcher's SQLite file. The two projects may run on different hosts. A repeated import of the same `event_id` is a no-op. `release_ids` are scoped to one publication; Media Core must not assume they are globally unique or that numbering is contiguous.

## Compatibility examples

- An agent that only understands `release.added` can ignore all other event types.
- An agent that does not understand `releases` can use `release_ids` and `url`.
- A consumer that sees `schema_version: "2.0"` must stop and report an unsupported contract rather than guessing.
