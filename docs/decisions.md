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
- Consequences: A newly added 210-chapter work does not produce a false "210 new chapters" event. The adapter makes at most a bounded number of polite catalog requests and must handle pagination. `general_all_no` is the chapter count; `allcount` is only the number of API search results. A catalog is marked complete only after a short or empty page after the first proves exhaustion, not merely because the parsed length equals the API count. An empty first page is always treated as incomplete. A 404 after a full page is treated as exhaustion only when the known count is already satisfied. Partial catalogs are never used for removal detection.

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
- Status: Accepted and implemented by task 0006
- Decision: `NotificationDelivery` rows provide a pending/sent/failed outbox with attempts and retry times. Telegram is one optional channel configured only through environment variables. A missing token does not create delivery rows and never blocks event storage. Secrets are never logged or stored in SQLite. Multiple new chapters are grouped into one message per event/publication.
- Consequences: Delivery failures are recoverable and deduplicated. Status-code classification happens before body parsing, so proxy/CDN 5xx or 429 pages still retry. `scan --no-notify` reports pending outbox counts without sending. The CLI and JSON event stream remain the primary interface.

## ADR-0007 — SSRF-safe HTTP client

- Date: 2026-11-04
- Status: Accepted
- Decision: All adapter HTTP traffic goes through a shared client that only allows `http`/`https`, limits redirects, applies timeouts, bounds response bodies, paces requests per host and retries 429/5xx with backoff. A custom dialer resolves the host and rejects loopback, private, link-local, CGNAT, multicast, benchmarking, reserved and unspecified addresses at dial time, which also covers redirects and DNS rebinding. IPv4-compatible, IPv4-mapped, NAT64, 6to4 and deprecated site-local IPv6 forms are rejected as well. Environment HTTP proxies are disabled so a proxy cannot bypass the address check. An explicit test/dev setting may allow private hosts; it is off by default. Body-read timeouts and cancellation during pacing/retry waits are returned as typed source errors.
- Consequences: Tests use local HTTP servers with the explicit setting. No CAPTCHA, login or paywall bypass exists. RSS 2.0, RSS 1.0/RDF and Atom feeds are parsed; relative links are resolved against the feed URL; Atom self/enclosure links are not used as release URLs; when an entry has no guid/id/link, a SHA-256 fallback id includes the entry position so identical malformed entries do not collapse into one release.

## ADR-0008 — Local hooks and APK; no CI until a working MVP

- Date: 2026-11-04
- Status: Accepted
- Decision: Lefthook provides a fast pre-commit (gofmt on staged Go files, whitespace check and a non-leaking secret scan of staged additions) and a mandatory pre-push (all Go files gofmt, secret scan, `go test ./...`, `go vet ./...`, `go build ./...`, APK lint). Pre-push fails with a clear error when `pnpm`/APK is unavailable; APK lint is never silently skipped. `go test -race` is an explicit extended check, required at MVP acceptance. The secret scanner prints only file, line and secret type, never the matched value, and is an accident guard, not a full Git-history audit. No `.github/workflows` exists before MVP acceptance.
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

## ADR-0013 — The CLI is the MVP contract, with a process lock and isolated watches

- Date: 2026-11-04
- Status: Accepted and implemented by task 0005
- Decision: Every read and mutation command returns either a localized human rendering or one JSON envelope (`schema_version`, `ok`, `data`/`error`). Machine codes and field names are English. Exit codes are 0 success, 1 runtime failure, 2 usage/input, 3 partial scan, 4 scan lock held. `add` is idempotent for the same source line and returns the existing watch with `created: false`; the first add stores a baseline and emits no release events. `scan --due` uses persisted `next_check_at`, `--all` scans all enabled watches, and `--id` forces one watch. A process file lock prevents overlapping scans. Each watch has its own timeout; one failure is recorded and does not stop the others.
- Consequences: Agents can drive the same use cases as humans without a server. Events remain the durable source for future MCP and Media Core. A partial scan is visible in both the JSON result and exit code 3, so a timer can alarm without hiding successful watches.

## ADR-0014 — Idempotent add is local; event dedupe is state-plus-time

- Date: 2026-11-04
- Status: Accepted and implemented by corrective review of task 0005
- Decision: A repeated `add` resolves the existing publication/watch from local state before any network fetch, so it remains idempotent when the source is down. It returns `created: false` and `reason: already_exists`. Scan changes and events are committed atomically; a replay with no new state emits nothing. The event dedupe key preserves transition order (for example `ongoing → completed` differs from `completed → ongoing`) and includes the detection time, so a later legitimate recurrence is not suppressed. Release-set parts are sorted for stability.
- Consequences: "Dedupe" does not mean an event can never recur; it means the same committed transition is not inserted twice. Consumers deduplicate by the stable `event_id`. `show` reads the newest events while the `events` command stays ascending for cursor paging. Add collapses duplicate source entry ids before creating baseline rows, so a malformed feed cannot fail the first add with a unique-constraint error.

## ADR-0015 — Backup is a verified single-file archive; restore is validated and exclusive

- Date: 2026-11-04
- Status: Accepted and implemented by task 0007
- Decision: `backup` runs `VACUUM INTO` for a consistent SQLite copy and writes one ZIP archive with a manifest (format version, schema version, app version, size, SHA-256). The live store is not copied by file-copy while WAL is active. `restore` acquires the same process lock as scans/notify, validates archive structure, size, hash and schema read-only, refuses a schema newer than the binary, and replaces the database through a staged rename with the previous file kept aside until success. It requires `--force` when a database exists.
- Consequences: Backup/restore are ordinary CLI operations and usable without a server. They do not protect against a caller that ignores the lock and opens the database directly; installation docs require stopping the timer and other commands before restore. `doctor` is read-only, reports the schema version without migrating, and uses an immutable open for closed databases so it does not create WAL sidecars and inspects a temporary copy when a WAL is present. The installer renders actual PREFIX/XDG paths into the unit instead of copying `%h` placeholders.

## ADR-0016 — Narō `end` field follows the official mapping

- Date: 2026-11-04
- Status: Accepted and implemented by task 0011
- Context: The original adapter inverted the Narō API `end` field (`1` was treated as completed), so an ongoing work such as `N2166FE` could be reported as completed. The official specification at `https://dev.syosetu.com/man/api/` defines `0` as a short story or completed work and `1` as ongoing.
- Decision: Map `end=0` to `completed`, `end=1` to `ongoing`, and a missing, `null` or unsupported value to `unknown` with no `status_source`. Never infer completion from inactivity. A `completed` watch stays enabled because an author may resume publication.
- Consequences: Status-change events follow the real source transitions, including `ongoing → completed → ongoing`. Fixtures and JSON examples use the corrected semantics. Unknown values are not guessed.

## ADR-0017 — Daily user timer with an explicit cadence check

- Date: 2026-11-04
- Status: Accepted and implemented by task 0013
- Context: The default watch interval is 24h, but the shipped timer ran hourly. Hourly wake-ups add noise and battery/IO cost without improving the default experience. A user-level timer also needs explicit lingering guidance for machines that reboot without a login.
- Decision: Ship one user-level timer at `OnCalendar=*-*-* 04:15:00` local time with `Persistent=true`, `RandomizedDelaySec=15m` and a oneshot service. Do not enable lingering automatically. `doctor` reads the installed timer, compares its cadence with the shortest enabled watch interval, and warns with an override hint when the timer is too slow or cannot be parsed.
- Consequences: Default installations check once per day and recover missed runs. Users who configure shorter intervals must edit `OnCalendar` or accept delayed checks. The installer prints the explicit `loginctl enable-linger` command when lingering is off.

## ADR-0018 — Unwatch preserves history; purge is explicit

- Date: 2026-11-04
- Status: Accepted and implemented by task 0014
- Context: The original `remove` deleted the watch, publication line, releases, events, deliveries and scan runs. That destroys the history a release watcher exists to keep and makes re-adding a work look like a fresh baseline.
- Decision: `remove`/unwatch marks the watch with `removed_at`, disables scheduling and preserves all observation history. `list` hides removed watches; `show`, `events` and exports still work. Re-adding the same URL restores the same watch and rows without duplicate releases or old-chapter notifications. `purge` is the only destructive command and requires `--yes`. Migration `0002_watch_unwatch.sql` adds the nullable `removed_at` column to existing databases.
- Consequences: Event ids remain stable across unwatch/restore. Existing databases migrate in place. Destructive deletion is explicit and separately tested. Removed watches must be re-added before scanning, pausing, resuming or changing their interval.

## ADR-0020 — Calendar-aware daily scheduling instead of a shorter interval

- Date: 2026-11-04
- Status: Accepted and implemented by task 0017
- Context: The shipped timer runs daily at 04:15 local time with `RandomizedDelaySec=15m`. A successful 24h watch stored `next_check_at = now + 24h`, so a scan at 04:29 made the next due time 04:29; a timer firing at 04:18 the next day was early and the scan was skipped until the following day. Shortening 24h to 23h would create drift, extra scans and wrong semantics for other intervals.
- Decision: For intervals that are whole local calendar days (24h, 48h, ...), compute `next_check_at` as the next eligible local midnight in the configured timezone (`RELEASE_WATCHER_TIMEZONE`, default system local). A daily timer after midnight is therefore due regardless of jitter, an evening add is checked at the first morning run, missed days are caught up once, and DST days are handled by the local calendar. Intervals that are whole calendar days (24h, 48h, ...) use this rule; sub-day intervals such as 6h and 12h remain duration-based and scheduler-agnostic. The application does not depend on systemd; cron and manual runs use the same due rule.
- Consequences: A 24h watch is checked once per local calendar day, not exactly 24h after the previous check. A 48h watch is checked on every second local calendar day. No daemon, extra timer or reduced interval is introduced. `scan --all` records a real success and therefore starts the next calendar window from that day, but does not create a duplicate scan within the same window. Existing databases with `next_check_at` already inside a jitter band may miss at most one post-upgrade run until the next successful scan realigns that row.

## ADR-0021 — Unwatch cancels pending deliveries

- Date: 2026-11-04
- Status: Accepted and implemented by task 0018
- Context: `remove` preserved history but left pending/failed deliveries eligible for `notify`. A user who stopped watching could later receive a notification about a work they had removed. `pause` is a temporary scheduling stop, so it must not cancel already-created notifications.
- Decision: Add a terminal `canceled` delivery status via append-only migration `0003_delivery_canceled.sql`. `remove` takes the same process lock as scan/notify, disables the watch and marks only that watch's pending and failed deliveries canceled in the same transaction. A scan that already fetched data re-checks `removed_at` before writing events and skips removed watches. `sent` deliveries stay sent; canceled deliveries are excluded from due selection and are never sent after restart or re-add. `purge` remains the destructive history operation. `pause` leaves deliveries pending.
- Consequences: `remove` reports `canceled_deliveries`, and notification counts include `canceled`. Existing databases migrate safely. Telegram can still deliver a message if the network accepted a request before cancellation; exactly-once delivery is not promised.

## ADR-0019 — APK release typing for functional-MVP acceptance

- Date: 2026-11-04
- Status: Accepted; process correction, task 0016 not rewritten
- Context: APK v0.5.2 maps `Type: release` to the `release` classification, which requires `live` and `report` evidence and carries generic release prose about hosted CI, an annotated tag and a post-tag cold install. Task 0016 was created with that type to accept the functional MVP. The repository intentionally has no GitHub Actions, no published tag and no deployment at that point.
- Decision: Treat 0016 as functional-MVP acceptance, not a published release or deployment. Its actual recorded evidence is the local deterministic matrix (`push-checks`, race tests, APK lint), the live Narō smoke and the acceptance report; no CI, tag or post-tag install is claimed. Do not rewrite the completed task or its evidence. Future deployment work uses `Type: deployment` (task 0008) with real manual live checks on a provided host. A real release/tag process, if wanted, is a separate later step with its own exact-SHA and post-tag evidence.
- Consequences: The 0016 contract contains release-typed prose that was not executed, but the recorded evidence and documentation do not claim it. 0008 stays blocked until a real host and explicit permission exist; 0009 stays in backlog. This note is the compact process correction instead of editing closed-task history.

## ADR-0022 — NAS deployment requires system-level cgroup isolation

- Date: 2026-11-04
- Status: Accepted; installation blocked on host access
- Context: A read-only audit found the target NAS resource-safe (ample RAM/disk, no pressure or OOM) and cgroup v2 capable. ResLedger and the Telegram agent have their own users, cgroups and limits. The preferred Release Watcher deployment is a system-level oneshot service plus daily timer under a dedicated non-privileged user, with explicit MemoryHigh/MemoryMax/CPUQuota/TasksMax/IOWeight limits. Non-interactive sudo is unavailable on the host and user lingering is disabled.
- Decision: Do not install a user-level fallback as a production deployment. A user-level timer would not survive logout/reboot without lingering, and the user manager does not expose IOWeight. Keep the system-level layout and limits as the deployment contract; require the operator to provide the exact privileged setup (dedicated user, `/opt/release-watcher`, `/var/lib/release-watcher`, `/var/backups/release-watcher`, `/etc/release-watcher`, system units, `daemon-reload`). Enable the timer only after smoke and cgroup verification.
- Consequences: The host is resource-safe but deployment remains blocked until privileged access is provided. No existing service, global systemd configuration, cgroup, firewall or routing is changed. Whoosh Core remains a separate blocked spike; a bounded SSH/SCP release layout is the fallback if Whoosh cannot deliver prebuilt artifacts.

## ADR-0023 — Dev-side artifact manifest and system-level packaging

- Date: 2026-11-04
- Status: Accepted and implemented by task 0021
- Context: The NAS must receive a prebuilt binary and must not install Go/Node/pnpm or clone the repository. A deployable artifact needs an exact commit, version, architecture, build time and checksum. The system-level unit also needs an explicit, auditable release layout and bounded retention.
- Decision: `scripts/package-release.sh` runs the local verification gates, builds with the existing release build script and writes `manifest.json` containing format version, version, full commit, GOOS/GOARCH, RFC3339 build time, SHA-256, size, checks and whether the identity probe executed on this host (cross-arch builds skip execution). `scripts/install-system.sh` runs as root on the target, validates the manifest/checksum/architecture and embedded commit, installs only Release Watcher paths and the separate system units, switches `current` atomically, and keeps the newest releases without deleting the current one. It never starts or enables the timer. `scripts/rollback-system.sh` switches to an installed release after verifying its commit. `scripts/prune-backups.sh` is dry-run by default and never deletes an explicitly verified backup.
- Consequences: Release artifacts are reproducible inputs to deployment, not Git builds on the target. The NAS needs only the binary, manifest, units and install helper plus `python3` for manifest validation; the runtime does not require Python. Whoosh Core remains unsuitable for this prebuilt-artifact flow and stays a separate blocked spike.
