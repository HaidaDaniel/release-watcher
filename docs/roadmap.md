# Roadmap

The order is dependency-driven. A stage is complete only when its implementation and named verification evidence exist; documentation alone does not complete a stage.

## Stage 0 — Foundation (task 0002)

- Repository rules, project/scope/architecture/domain/contract docs and non-goals.
- Go bootstrap, build identity, CLI `version`/`help`/`doctor` skeleton.
- Lefthook fast/push checks, build scripts, systemd unit skeletons.
- No CI, no product data features.

Exit: documented scope, `go build`/`go test`/`go vet`, hooks fire, honest status.

## Stage 1 — Persistence and domain (task 0003) — done

- SQLite migrations and schema version.
- Work, Publication, Source, Watch, Release, Event, NotificationDelivery, ScanRun repositories.
- Transactions, indexes, baseline storage, event dedupe.
- Migration and store tests.

Exit: state survives restart; migrations are append-only; dedupe and baseline are tested.

## Stage 2 — Narō vertical slice (tasks 0004–0005) — done

- HTTP client with timeout, retry, pacing and SSRF guard.
- Narō adapter: official API metadata, catalog pages, pagination, count-only degradation, explicit status semantics.
- RSS/Atom adapter with sliding-window semantics.
- Scan comparison, event creation, independent per-watch errors.
- CLI `add`, `list`, `show`, `scan`, `events`, `pause`, `resume`, `interval`, `remove` with stable JSON.

Exit: add `https://ncode.syosetu.com/n2166fe/`, baseline without a false notification, detect a fixture new chapter, read the event through CLI JSON. Reached with fixture HTTP servers; the live Narō smoke remains a separate polite check.

## Stage 3 — Notifications (task 0006) — done

- Durable Telegram outbox, pending/sent/failed, retry/backoff, dedupe.
- Grouped RU/EN messages.
- Failure/retry tests; no secret leakage.

Exit: a failed Telegram request is retried and a successful event produces exactly one message.

## Stage 4 — Multiple sources and operations (task 0007)

- Multiple watches, per-watch intervals, source isolation.
- Backup/restore, readable event export, doctor.
- systemd timer installation and docs; build linux/amd64 and linux/arm64.
- Whoosh Core assessment and deployment spike plan/backlog.

Exit: MVP acceptance scenario is executed on a Linux host; backup restore and restart are verified. First Whoosh deployment is recorded separately only after it actually happens.

## Stage 5 — Media Core documentation (separate repository)

- README, AGENTS, project/scope/architecture/domain-model/roadmap/integration-contract.
- ContentReference/ContentAsset/ContentProvider/DeliveryTarget concept only; no business logic.
- Contract aligned with Release Watcher events.

## Deferred backlog

- Kakuyomu and Royal Road HTML-only adapters.
- Manga, anime, publisher-site and serialized-film adapters.
- MCP stdio over the same application use cases (after a proven CLI).
- Optional title-translation integration at add time.
- Media Core event import and reading-history linkage.
- Whoosh Core vs ResLedger deploy-system comparison (after a successful MVP deployment).
