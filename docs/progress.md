# Progress

Status vocabulary: `planned`, `implemented`, `tested`, `deployed`, `blocked`. Documentation and task creation never imply implementation.

## Planned

- Scan comparison, application use cases and CLI JSON (task 0005).
- Telegram outbox and RU/EN notifications (task 0006).
- Backup/restore, doctor, systemd installation and Whoosh assessment (task 0007).
- Media Core documentation in its own repository.
- Whoosh Core deployment spike and first controlled deployment after MVP.
- Backlog sources and MCP stdio.

## Implemented

- APK v0.5.2 pinned as a dev dependency; repository initialized in `mvp` mode.
- Foundation documentation with explicit non-goals and honest status.
- Go CLI bootstrap with build identity (`version`, `help`, `doctor` skeleton).
- Lefthook pre-commit/pre-push configuration and local verification scripts.
- Linux systemd service/timer skeletons.
- Build script for linux/amd64 and linux/arm64 with ldflags identity.
- Domain entities and SQLite schema version 1 with Work, Source, Publication, Watch, Release, Event, NotificationDelivery, ScanRun and Setting tables.
- Transactional repository layer with event/delivery dedupe and due-watch queries.
- SSRF-safe HTTP client with timeout, redirect limit, per-host pacing, retries, body limits and no environment proxy.
- Narō adapter using the official novel API plus paginated catalog pages, with complete/partial/count_only snapshots.
- RSS 2.0/RSS 1.0 and Atom adapter with stable entry ids and sliding-window semantics.

## Tested

- Foundation CLI JSON, help localization and build identity tests: see task 0002 evidence.
- Store tests: migrations idempotent, restart persistence, rollback, due-watch selection, event/delivery dedupe, release baseline/removal, transactional observation deletion (including shared Work/Source retention), invalid-enum rejection, unknown-cursor rejection, concurrent first-open migration locking and settings: see task 0003 evidence.
- `scripts/verify-push.sh` runs gofmt, secret scan, `go test ./...`, `go vet ./...`, `go build ./...` and APK lint.
- Source tests: SSRF address policy (including exotic IPv6) and dial-time blocking, retry, pacing, header/body timeout, cancellation, redirect/body limits, status mapping, Narō catalog/pagination/partial/count-only/status-unknown/undercount/missing-update-block, RSS 2.0/RSS 1.0/Atom parsing, relative links and kind mismatch: see task 0004 evidence.
- No live Narō smoke test has been run; adapter tests use local fixtures and test servers.

## Deployed

- Not deployed. No production deployment claim exists yet.

## Blocked

- Whoosh Core live deployment: no test host and no MVP candidate yet. Planned after task 0007.
- Fixture-based adapter and detection tests plus a single polite live Narō smoke test: blocked until the adapter tasks implement them.
- Real Narō behavior is not yet verified; no live smoke test has been run.

## Next step

Task 0005: scan detection, application use cases and CLI JSON.
