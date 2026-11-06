# Progress

Status vocabulary: `planned`, `implemented`, `tested`, `deployed`, `blocked`. Documentation and task creation never imply implementation.

## Planned

- Whoosh Core deployment spike and first controlled deployment after MVP.
- Backlog sources and MCP stdio.

## Implemented

- APK v0.5.2 pinned as a dev dependency; repository initialized in `mvp` mode.
- Foundation documentation with explicit non-goals and honest status.
- Go CLI bootstrap with build identity (`version`, `help`, `doctor` skeleton).
- Lefthook pre-commit/pre-push configuration and local verification scripts; pre-push requires pnpm/APK lint and the secret scanner prints only file/line/type.
- Linux systemd service/timer skeletons.
- Build script for linux/amd64 and linux/arm64 with ldflags identity.
- Domain entities and SQLite schema version 3 with Work, Source, Publication, Watch, Release, Event, NotificationDelivery, ScanRun and Setting tables.
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
- System-level NAS units with cgroup limits, dev-side release packaging/manifest, install/rollback and backup-prune helpers are installed on the NAS. The installer stages and verifies before switching current and rolls back previous current/units on failure; doctor detects system timers first and still supports user timers.
- A transient user-level smoke on the NAS (no installation, no timer) ran the release artifact against live Narō: baseline 210 chapters, zero false events, one unchanged scan, backup schema 3, cgroup peak ~11.6 MiB, ~0.35–0.63 s CPU across runs, zero OOM. This is smoke evidence, not a deployment.
- Whoosh Core was assessed at commit `24d8dc398e2d7baacf13338e44197ba0d471dde3`; its Git-build model does not support the required prebuilt-artifact flow without fighting the tool.
- Narō `end` mapping corrected to the official specification; unwatch preserves history and `purge` is explicit; daily persistent timer and doctor cadence check; non-leaking secret scanner and mandatory APK lint.
- Calendar-aware day scheduling prevents daily timer jitter/DST from skipping a day while preserving 6h/12h/24h/48h intervals.
- Streamable HTTP MCP server over `app.Service` with read-only and mutation tools, Bearer token auth and server-side write-mode gating is implemented.
- Separate MCP systemd unit with cgroup limits, a root-owned `mcp.env` token file and install/rotation helper is implemented and statically verified; it is not installed yet.
- `remove` cancels that watch's pending/failed deliveries atomically; canceled deliveries never send after restart or re-add, while pause does not cancel.
- Live Narō smoke and isolated E2E regression are implemented and pass.
- Media Core documentation is complete in its separate repository (docs only).

## Tested

- Foundation CLI JSON, help localization and build identity tests: see task 0002 evidence.
- Store tests: migrations idempotent, restart persistence, rollback, due-watch selection, event/delivery dedupe, release baseline/removal, transactional observation deletion (including shared Work/Source retention), invalid-enum rejection, unknown-cursor rejection, concurrent first-open migration locking and settings: see task 0003 evidence.
- `scripts/verify-push.sh` runs gofmt, secret scan, `go test ./...`, `go vet ./...`, `go build ./...` and APK lint, and fails clearly when pnpm is missing. Secret-scanner tests prove the matched value never reaches stdout/stderr.
- Narō status tests cover `end=0`, `end=1`, `null`, missing and unsupported values, plus `ongoing -> completed -> ongoing` transitions: see task 0011 evidence.
- Source tests: SSRF address policy (including exotic IPv6) and dial-time blocking, retry, pacing, header/body timeout, cancellation, redirect/body limits, status mapping, Narō catalog/pagination/partial/count-only/status-unknown/undercount/missing-update-block/empty-first-page/404-page-boundary, RSS 2.0/RSS 1.0/Atom parsing, relative links, fallback-id collisions and kind mismatch: see task 0004 evidence.
- A polite live Narō smoke against `N2166FE` records the actual API/catalog response: title `夢見る男子は現実主義者`, `ncode:N2166FE`, status `ongoing` (`end=1`), 210 baseline chapters, more than one catalog page and parsed publication dates; the baseline emitted no events. See task 0015 evidence.
- Isolated E2E covers 210→212 chapters, one grouped RU Telegram message (`+2`), replay dedupe, Telegram failure, restart/redelivery with no due watches, two-watch isolation and JSON output at each CLI stage: see task 0015 evidence.
- Notification tests: grouped RU/EN messages and grammar, dedupe, failure then retry, 429 retry_after, non-JSON 5xx/429 retries, permanent 403/400 not retried, max-attempt exhaustion, invalid responses, failed-delivery restart persistence, no configured Telegram, token not leaked in errors/stored last_error, `scan --no-notify` pending counts, `notify` JSON and automatic delivery: see task 0006 evidence.
- Application/scan/CLI tests: first add baseline without events, duplicate add (including offline), one/many new chapters, replay without duplicates, failure/recovery, empty and sliding RSS windows, count-only reconciliation (including previously removed releases), repeated status transitions, due/global intervals, pause/resume/remove, restart, recent-event history and pagination, metadata/status payloads, invalid/unknown URLs, concurrent scan lock, partial scan isolation and JSON CLI flows: see task 0005 evidence. Unwatch history preservation, re-add restoration and explicit purge: see task 0014 evidence.
- Installation tests: daily timer cadence, timer drop-ins and calendar-step parsing, doctor schedule warning, and lingering guidance: see task 0013 evidence.
- Installation tests: backup/restore round-trip, force/exclusivity, bad/missing manifest hash or size, newer/empty schema, sidecar cleanup, symlink/hardlink backup-target rejection, database/archive permissions, doctor schema/newer-schema/read-only sidecar behavior, systemd verification, install rendering with XDG/special paths: see task 0007 evidence.
- Linux amd64/arm64 builds and local binary smoke checks are recorded in docs/deployment.md.
- MVP acceptance matrix: `go test ./...`, `go test -race ./...`, `go vet ./...`, `go build ./...`, `scripts/verify-push.sh`, `pnpm exec apkit lint`, linux/amd64 and linux/arm64 builds, release build identity, backup/restore with real data, doctor schema check, systemd unit validation, live Narō smoke, isolated E2E and JSON stdout purity.
- MCP wire-level tests cover initialize, tools/list, read tools, mutations, duplicate add, pagination, read-only rejection, auth/origin/host rejection, oversized body, invalid schemas, unknown/forbidden tools, SSRF, lock propagation, restart persistence and cancellation: see tasks 0024/0025 evidence.
- Notification cancellation tests cover pending, failed, sent, remove, re-add, restart, pause and shared watches: see task 0018 evidence.
- Scheduling regression tests cover day1 04:29 -> day2 04:18, evening add, missed days, 6h/12h/24h/48h, manual scan --all, same-window replay and Europe/Berlin DST transitions: see task 0017 evidence.
- NAS deployment acceptance: system-level install, effective cgroup limits (`cpu.max=40000 100000`, memory/swap/pids/io), SQLite persistence, service run, daily timer, backup/restore on a separate copy, upgrade from `1d89864` to `4d08eff` on a restored test DB and rollback on a separate test root: see task 0022 evidence.
- MCP deployment: `0.2.0`/`c7f996b` installed, loopback-only service active, effective cgroup limits and auth/tools verified; rollback `c7f996b`->`4d08eff` verified on a separate root. See task 0026 evidence and `deploy/hermes/README.md`.
- Signal/timeout/lock acceptance: a second concurrent scan exits 4 with a JSON `locked` error; SIGINT ends the scan with a partial-failure JSON envelope and exit 3; an external timeout terminates the process; a later run succeeds.

## Deployed

- Release Watcher `0.2.0` at commit `c7f996b` is deployed system-level on a home NAS under the unprivileged `release-watcher` user. The daily scanner timer remains active; the new separate MCP service is active on `127.0.0.1:8741` with verified cgroup limits, read-only/write-mode checks and real SQLite reads. SQLite schema 3, verified backups, separate-copy restore and separate-root rollback passed. Telegram is not configured. Whoosh was not used.

## Blocked

- Hermes remote MCP acceptance: the NAS MCP service is deployed and locally verified, but there is no private HTTPS/VPN route and the Hermes client is operator-owned. Task 0026 remains blocked pending a secure route and Hermes installation.
- Whoosh Core live deployment: no test host; assessment documented but not executed. Task 0008 remains blocked pending a real test host and explicit deployment permission.

## APK process note

Tasks 0003–0007 were recorded with `Depends on: none` even though their work was sequential. Their history and candidate-bound evidence are not rewritten. Corrective tasks 0011–0016 declare their real dependencies (`0007 -> 0011..0014 -> 0015 -> 0016 -> 0008`), and 0008 now depends on 0016. Some corrective tasks needed an append-only APK epoch because their claim baseline was captured after edits; the predecessor history remains visible and no old evidence was copied.

APK v0.5.2 typed 0016 as `release`, whose template asks for hosted CI, a tag and post-tag install evidence. 0016 actually accepted the functional MVP with local checks, a live Narō smoke and a report; no CI/tag/deploy was performed or claimed. The mismatch is recorded in ADR-0019. Future deployment work uses `Type: deployment` (0008) with real live checks; 0016 is not rewritten.

## Next step

Remaining backlog: Hermes remote MCP acceptance (task 0026), Whoosh Core deployment spike (task 0008), Whoosh vs ResLedger comparison (task 0009), and future sources/content delivery. Media Core documentation is complete in its separate repository. See docs/roadmap.md.
