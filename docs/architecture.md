# Architecture

Status: the repository, CLI bootstrap, SQLite persistence, normalized Narō/RSS/Atom adapters, scan detection and the CLI vertical slice are implemented. Notifications, backup/restore, deployment proof and the Whoosh spike are planned. The target shape below is intentionally small.

## Shape

Release Watcher is a modular monolith in one Go binary. It is invoked as a CLI, usually by a systemd timer. There is no long-running server, no background goroutine that must survive between invocations, and no network listener.

```text
cmd/release-watcher        process entrypoint, signals, exit code
internal/cli               argument parsing, human and JSON rendering
internal/app               use cases: add, list, show, scan, events, pause/resume/interval/remove,
                           doctor, backup, restore, notify
internal/scan              snapshot comparison and event creation
internal/domain            Work, Publication, Source, Watch, Release, Event, Delivery, ScanRun
internal/store             SQLite repositories, migrations, transactions
internal/source            adapter registry, Narō, RSS/Atom, HTTP client and SSRF guard
internal/notifier          outbox delivery and Telegram client
internal/i18n              RU/EN user-facing strings
internal/buildinfo         version, commit, build time, build mode
internal/config            environment-driven configuration
migrations                 embedded append-only SQL migrations
```

Dependencies point inward: CLI and adapters depend on application/domain types; the domain does not depend on SQLite or Telegram. The application layer owns transactions and use-case rules. This is not a heavy Clean Architecture framework; it is a small set of packages with one reason to change each.

## Data flow

1. `add` normalizes the URL, selects an adapter, fetches one snapshot, creates Work/Publication/Source/Watch and stores the snapshot as a baseline in one transaction.
2. `scan --due` selects enabled watches whose `next_check_at` is due. Each watch is processed independently with its own timeout, so one failure does not stop the others.
3. The adapter returns a normalized `Snapshot`: metadata, release identities, a completeness mode (`catalog` or `count_only`), and which fields are trustworthy.
4. `internal/scan` compares the snapshot with persisted releases and produces at most a small set of immutable events.
5. The same transaction updates the watch/check state and inserts the event(s) plus a `scan_run` row. A unique dedupe key makes replays safe.
6. If Telegram is configured, one pending delivery per event is inserted in the same transaction. Delivery runs after commit and can be retried independently.
7. `events` reads the stored, versioned event contract. Notification failure never hides or deletes an event.

## Scheduling model

The model is "run when needed", not "stay alive".

- `next_check_at` is computed from the watch interval after a successful scan.
- `scan --due` runs anything overdue, which recovers missed timer runs.
- `scan --all` is an explicit manual override.
- `scan --id` processes one watch.
- A process-local file lock prevents overlapping scans; SQLite provides the durable transaction boundary.
- The shipped systemd timer runs hourly and `--due` is a cheap no-op when nothing is due. A daily timer is sufficient for the default 24h interval; a shorter interval needs a correspondingly more frequent timer. The installation docs state this explicitly.

## Storage

SQLite is the only datastore. Pure-Go SQLite keeps `CGO_ENABLED=0` cross-compilation possible for Linux x86-64 and ARM64. State lives outside the source tree (`$XDG_DATA_HOME/release-watcher` or `RELEASE_WATCHER_STATE_DIR`). WAL mode, a busy timeout, foreign keys and short transactions are enabled. Migrations are append-only and recorded with a schema version.

## Source adapters

An adapter reports applicability, fetches metadata plus releases, normalizes identities and states which fields it can prove. It never invents chapter numbers, URLs or completion status.

- Narō uses `api.syosetu.com/novelapi/api/` for metadata and `ncode.syosetu.com` catalog pages for stable chapter identities. If the catalog is temporarily unavailable the adapter may degrade to `count_only`; recovery is reconciled without a bulk false notification.
- RSS/Atom uses XML parsing and stable entry IDs (guid/id, else link, else a bounded hash). Feed windows are sliding: an old entry disappearing from the feed is not a removal.
- Unsupported URLs fail with a stable error code before any state is written.

## Security and robustness

- The HTTP client only permits `http`/`https`, limits redirects, applies timeouts and retries 429/5xx with backoff.
- SSRF protection rejects loopback, private, link-local, CGNAT, multicast and unspecified addresses at dial time, including redirect targets and DNS rebinding. The client does not use environment HTTP proxies, so a proxy cannot bypass the address check. Tests and explicit local feeds can enable private hosts only through configuration.
- Per-host request pacing is enforced. No CAPTCHA, login or paywall bypass exists.
- Secrets (Telegram token) come from the environment, are never logged, and are not stored in SQLite.
- Logs are text by default and JSON when requested. Event and CLI JSON contracts have stable field names and schema versions.

## Explicit non-architecture

No event sourcing, CQRS, message broker, plugin runtime, generic workflow engine, HTTP server, multi-tenant authorization, external scheduler platform or second database. Those would be added only after a real, recorded need.
