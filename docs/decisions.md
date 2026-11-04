# Decisions

Architecture and dependency decisions for Release Watcher. Add a new entry for every material decision; do not silently change an accepted one.

## ADR-0001 — CLI-first modular monolith with SQLite and no daemon

- Date: 2026-11-04
- Status: Accepted
- Context: The founding scenario is a periodic check that must run on a weak home Linux device. A server, broker or always-on process would add cost without user value.
- Decision: One Go binary invoked as a CLI. An external systemd timer runs `scan --due`. The process exits after each scan; no daemon, HTTP listener or idle RAM is required. SQLite is the only datastore. The binary is split into small packages (`cli`, `app`, `scan`, `domain`, `store`, `source`, `notifier`) with inward dependencies, not a framework.
- Consequences: Scheduling is external and explicit. Missed runs are recovered by `--due`. There is no remote API in the MVP; agents call the CLI and consume JSON.

## ADR-0002 — Pure-Go SQLite driver

- Date: 2026-11-04
- Status: Accepted
- Context: The tool must ship on Linux x86-64 and ARM64 and be easy for ordinary users to build.
- Decision: Use `modernc.org/sqlite`, a pure-Go driver, with `CGO_ENABLED=0`. Do not require a C toolchain. Enable WAL, foreign keys and a busy timeout.
- Consequences: Cross-compilation is simple. The driver is an added dependency; it is recorded here as required by repository policy. No ORM is added. Migration `0001_init.sql` is the initial unreleased schema and may be refined in place during task 0003. After the first MVP release ships, migrations become append-only and must never be edited in place.

## ADR-0003 — Narō official API plus catalog, baseline first

- Date: 2026-11-04
- Status: Accepted
- Decision: Narō metadata comes from `api.syosetu.com/novelapi/api/`. Stable chapter identities and titles come from the public catalog pages on `ncode.syosetu.com`. The adapter treats `general_all_no` as a count and `end` as an explicit completion field only when present. It never treats inactivity as completion. The first observation is stored as a baseline with no release events. If catalog pages are unavailable but the API count works, the adapter may degrade to `count_only`; when catalog data returns, it reconciles silently before resuming detection.
- Consequences: A newly added 210-chapter work does not produce a false "210 new chapters" event. The adapter makes at most a bounded number of polite catalog requests and must handle pagination.

## ADR-0004 — Publication lines are never merged by title

- Date: 2026-11-04
- Status: Accepted
- Decision: One Work may own several Publications. WN, LN, translations and mirrors have separate Publication rows, separate release identities and separate events. The system does not auto-merge by title or sum chapter counts across sources. `Work` provides only the logical grouping.
- Consequences: The event contract identifies the publication line. Media Core can later link editions deliberately, with provenance.

## ADR-0005 — Immutable, deduplicated, versioned events

- Date: 2026-11-04
- Status: Accepted
- Decision: Detection writes immutable events with a schema version and a unique dedupe key. A scan replay, crash recovery or duplicate source entry cannot insert the same event twice. Events are stored before notification. Consumers deduplicate by `event_id`; repeated reads are safe. The JSON contract is documented in `docs/integration-contract.md`.
- Consequences: Notifications are derived from events, not the other way round. Media Core can import events without sharing the SQLite file.

## ADR-0006 — Durable outbox; Telegram is optional

- Date: 2026-11-04
- Status: Accepted
- Decision: `NotificationDelivery` rows provide a pending/sent/failed outbox with attempts and retry times. Telegram is one optional channel configured only through environment variables. A missing token does not create delivery rows and never blocks event storage. Secrets are never logged or stored in SQLite. Multiple new chapters are grouped into one message per event/publication.
- Consequences: Delivery failures are recoverable and deduplicated. The CLI and JSON event stream remain the primary interface.

## ADR-0007 — SSRF-safe HTTP client

- Date: 2026-11-04
- Status: Accepted
- Decision: All adapter HTTP traffic goes through a shared client that only allows `http`/`https`, limits redirects, applies timeouts, bounds response bodies, paces requests per host and retries 429/5xx with backoff. A custom dialer resolves the host and rejects loopback, private, link-local, CGNAT, multicast, benchmarking, reserved and unspecified addresses at dial time, which also covers redirects and DNS rebinding. Environment HTTP proxies are disabled so a proxy cannot bypass the address check. An explicit test/dev setting may allow private hosts; it is off by default.
- Consequences: Tests use local HTTP servers with the explicit setting. No CAPTCHA, login or paywall bypass exists.

## ADR-0008 — Local hooks and APK; no CI until a working MVP

- Date: 2026-11-04
- Status: Accepted
- Decision: Lefthook provides a fast pre-commit (gofmt on staged Go files, secret scan, repository/build sanity) and a fuller pre-push (`go test ./...`, `go vet ./...`, `go build ./...`, APK lint). `go test -race` is an explicit extended check, enabled in pre-push only when its cost is acceptable. The secret scan covers staged, working-tree and untracked additions; it is an accident guard, not a full Git-history audit. No `.github/workflows` exists before MVP acceptance.
- Consequences: Verification stays local and runnable. CI can be added later as a mirror of the same commands, not as a new source of truth.

## ADR-0009 — AgenticProjectKit is a development tool, not a runtime dependency

- Date: 2026-11-04
- Status: Accepted
- Decision: Pin APK v0.5.2 as a repository dev dependency. Use it for task contracts, evidence, review and gate. The application does not import or run APK at runtime, and APK is not installed on production hosts.
- Consequences: The repo keeps `package.json`/`pnpm-lock.yaml` only for tooling. Node.js is not required to build or run Release Watcher.

## ADR-0010 — systemd timer now, Whoosh Core later

- Date: 2026-11-04
- Status: Accepted; deployment spike not executed
- Decision: The MVP ships user-level systemd service/timer units and documents timer frequency honestly: `--due` cannot honor a 6h interval if the timer runs daily. Whoosh Core is evaluated as an external deployment mechanism after MVP verification. It is not a runtime dependency and is not added to this repository as a framework.
- Consequences: No production deployment claim is made until it is actually performed on a Linux host. The Whoosh assessment remains a backlog task.

## ADR-0011 — Russian default, English alternative, original titles preserved

- Date: 2026-11-04
- Status: Accepted
- Decision: Human help text and notification templates default to Russian with English selectable. Machine JSON field names stay stable English. `original_title` is always stored. `title_ru`/`title_en` may be supplied at add time and are reused; missing translations fall back to the original. Translation is an optional add-time integration, never a scan dependency. In the foundation, `version`/`doctor` human diagnostic strings remain English; full CLI localization lands with the product commands in task 0005 and notification localization in task 0006.
- Consequences: No hidden LLM call and no per-scan translation. Documentation does not claim that every foundation diagnostic string is localized yet.

## ADR-0012 — Minimal dependency set

- Date: 2026-11-04
- Status: Accepted
- Dependencies: `modernc.org/sqlite` (ADR-0002), `golang.org/x/net/html` for bounded Narō catalog parsing, and `lefthook` as a development-only hook runner. Standard library `log/slog`, `net/http`, `encoding/xml`, `archive/zip` and `database/sql` cover the rest.
- Decision: No web framework, ORM, CLI framework, retry library, scheduler library or notification framework. Add a dependency only with a recorded reason and a test.
- Consequences: Small binary and small audit surface. Parsing and retry helpers stay explicit and testable.
