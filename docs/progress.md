# Progress

Status vocabulary: `planned`, `implemented`, `tested`, `deployed`, `blocked`. Documentation and task creation never imply implementation.

## Planned

- SQLite schema, repositories and migrations (task 0003).
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

## Tested

- Foundation CLI JSON and build identity tests: see task 0002 evidence.
- `scripts/verify-push.sh` runs gofmt, secret scan, `go test ./...`, `go vet ./...`, `go build ./...` and APK lint.

## Deployed

- Not deployed. No production deployment claim exists yet.

## Blocked

- Whoosh Core live deployment: no test host and no MVP candidate yet. Planned after task 0007.
- Real Narō behavior beyond a single polite live smoke test: fixtures are used for deterministic tests; the live smoke is not mass polling.

## Next step

Task 0003: domain model and SQLite persistence.
