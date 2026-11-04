# Progress

Status vocabulary: `planned`, `implemented`, `tested`, `deployed`, `blocked`. Documentation and task creation never imply implementation.

## Planned

- HTTP/SSRF client, Narō and RSS/Atom adapters (task 0004).
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

## Tested

- Foundation CLI JSON, help localization and build identity tests: see task 0002 evidence.
- Store tests: migrations idempotent, restart persistence, rollback, due-watch selection, event/delivery dedupe, release baseline/removal, transactional observation deletion (including shared Work/Source retention), invalid-enum rejection, unknown-cursor rejection, concurrent first-open migration locking and settings: see task 0003 evidence.
- `scripts/verify-push.sh` runs gofmt, secret scan, `go test ./...`, `go vet ./...`, `go build ./...` and APK lint.
- No adapter, fixture or live Narō test exists yet; those tests belong to tasks 0004–0005.

## Deployed

- Not deployed. No production deployment claim exists yet.

## Blocked

- Whoosh Core live deployment: no test host and no MVP candidate yet. Planned after task 0007.
- Fixture-based adapter and detection tests plus a single polite live Narō smoke test: blocked until the adapter tasks implement them.
- Real Narō behavior is not yet verified; no live smoke test has been run.

## Next step

Task 0004: HTTP/SSRF client and Narō/RSS adapters.
