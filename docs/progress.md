# Progress

Status vocabulary: `planned`, `implemented`, `tested`, `deployed`, `blocked`. Documentation and task creation never imply implementation.

## Planned

- Media Core documentation in its own repository.
- Whoosh Core deployment spike and first controlled deployment after MVP.
- Backlog sources and MCP stdio.

## Implemented

- APK v0.5.2 pinned as a dev dependency; repository initialized in `mvp` mode.
- Foundation documentation with explicit non-goals and honest status.
- Go CLI bootstrap with build identity (`version`, `help`, `doctor` skeleton).
- Lefthook pre-commit/pre-push configuration and local verification scripts; pre-push requires pnpm/APK lint and the secret scanner prints only file/line/type.
- Linux systemd service/timer skeletons.
- Build script for linux/amd64 and linux/arm64 with ldflags identity.
- Domain entities and SQLite schema version 1 with Work, Source, Publication, Watch, Release, Event, NotificationDelivery, ScanRun and Setting tables.
- Transactional repository layer with event/delivery dedupe and due-watch queries.
- SSRF-safe HTTP client with timeout, redirect limit, per-host pacing, retries, body limits and no environment proxy.
- Narō adapter using the official novel API plus paginated catalog pages, with complete/partial/count_only snapshots and the official `end` mapping (`0` completed/short story, `1` ongoing, unknown otherwise).
- RSS 2.0/RSS 1.0 and Atom adapter with stable entry ids and sliding-window semantics.
- Scan comparison with silent baselines, catalog add/update/remove, count-only change events, metadata/status events and event dedupe.
- CLI commands `add`, `list`, `show`, `scan`, `events`, `pause`, `resume`, `interval`, `remove` and `purge` with stable JSON, RU/EN human output and exit codes. `remove` preserves history; `purge` is the explicit destructive operation.
- Process-level scan lock and per-watch timeout/error isolation.
- Durable Telegram outbox with pending/sent/failed states, bounded retry/backoff, permanent-failure handling and dedupe.
- Grouped RU/EN event messages and `notify` command; events remain stored when Telegram is absent or failing.
- Single-file backup archive with manifest/hash and validated atomic restore.
- Read-only, schema-aware doctor without implicit migration.
- User-level install script and systemd service/timer installation path with a daily persistent timer, XDG rendering and explicit lingering guidance.
- Linux amd64 and arm64 builds; static systemd verification and local binary smoke checks.

## Tested

- Foundation CLI JSON, help localization and build identity tests: see task 0002 evidence.
- Store tests: migrations idempotent, restart persistence, rollback, due-watch selection, event/delivery dedupe, release baseline/removal, transactional observation deletion (including shared Work/Source retention), invalid-enum rejection, unknown-cursor rejection, concurrent first-open migration locking and settings: see task 0003 evidence.
- `scripts/verify-push.sh` runs gofmt, secret scan, `go test ./...`, `go vet ./...`, `go build ./...` and APK lint, and fails clearly when pnpm is missing. Secret-scanner tests prove the matched value never reaches stdout/stderr.
- Narō status tests cover `end=0`, `end=1`, `null`, missing and unsupported values, plus `ongoing -> completed -> ongoing` transitions: see task 0011 evidence.
- Source tests: SSRF address policy (including exotic IPv6) and dial-time blocking, retry, pacing, header/body timeout, cancellation, redirect/body limits, status mapping, Narō catalog/pagination/partial/count-only/status-unknown/undercount/missing-update-block/empty-first-page/404-page-boundary, RSS 2.0/RSS 1.0/Atom parsing, relative links, fallback-id collisions and kind mismatch: see task 0004 evidence.
- No live Narō smoke test has been run; adapter tests use local fixtures and test servers.
- Notification tests: grouped RU/EN messages and grammar, dedupe, failure then retry, 429 retry_after, non-JSON 5xx/429 retries, permanent 403/400 not retried, max-attempt exhaustion, invalid responses, failed-delivery restart persistence, no configured Telegram, token not leaked in errors/stored last_error, `scan --no-notify` pending counts, `notify` JSON and automatic delivery: see task 0006 evidence.
- Application/scan/CLI tests: first add baseline without events, duplicate add (including offline), one/many new chapters, replay without duplicates, failure/recovery, empty and sliding RSS windows, count-only reconciliation (including previously removed releases), repeated status transitions, due/global intervals, pause/resume/remove, restart, recent-event history and pagination, metadata/status payloads, invalid/unknown URLs, concurrent scan lock, partial scan isolation and JSON CLI flows: see task 0005 evidence. Unwatch history preservation, re-add restoration and explicit purge: see task 0014 evidence.
- Installation tests: daily timer cadence, timer drop-ins and calendar-step parsing, doctor schedule warning, and lingering guidance: see task 0013 evidence.
- Installation tests: backup/restore round-trip, force/exclusivity, bad/missing manifest hash or size, newer/empty schema, sidecar cleanup, symlink/hardlink backup-target rejection, database/archive permissions, doctor schema/newer-schema/read-only sidecar behavior, systemd verification, install rendering with XDG/special paths: see task 0007 evidence.
- Linux amd64/arm64 builds and local binary smoke checks are recorded in docs/deployment.md.

## Deployed

- Not deployed to a production host. Linux binaries were built locally, the amd64 binary was smoke-checked, and systemd units were statically verified.

## Blocked

- Whoosh Core live deployment: no test host; assessment documented but not executed. Separate backlog task.
- A live Narō smoke test has not been run; adapters use fixtures and local test servers.

## Next step

Remaining backlog: Whoosh Core deployment spike (task 0008), Whoosh vs ResLedger comparison (task 0009), a polite live Narō smoke test, MCP stdio, and future sources/content delivery. Media Core documentation is complete in its separate repository. See docs/roadmap.md.
