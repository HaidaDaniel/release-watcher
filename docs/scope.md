# Scope

## MVP scope

The MVP must let one user on a Linux home machine:

- install one Go binary without Docker, PostgreSQL, Redis, Python or Node.js;
- add a Narō web-novel URL and see its title and known publication state;
- register the current catalog as a baseline without a false "new chapters" event;
- run `scan --due` from a systemd timer and detect later chapters;
- read the event history and machine-readable JSON from any agent;
- optionally receive a grouped Telegram message;
- add RSS/Atom sources and per-watch intervals;
- pause, resume, change the interval and remove a watch;
- restart the process without losing state;
- create and restore a verified SQLite backup;
- diagnose configuration and storage with `doctor`.

Required source adapters for the MVP:

1. Narō: official novel API plus the public chapter catalog.
2. RSS/Atom: generic feed adapter with sliding-window semantics.

Backlog sources, not part of the MVP: Kakuyomu, Royal Road HTML-only catalogs, manga, anime, publisher sites, browser automation, CAPTCHA/authorization bypass.

## Publication types

The domain separates `work` from `publication`. WN, LN, translation and other variants are distinct publication lines. The MVP implements the model and the Narō WN path. LN/translation ingestion from other sites is planned.

## Non-goals

- A continuously running daemon, HTTP API, web UI, MCP-over-HTTP or always-on server.
- Media Core business logic, content storage, translation harness or reading progress.
- LLM calls during periodic scans or mandatory translation of titles.
- Automatic merging of WN and LN by similar titles, or summing chapters across sources.
- CAPTCHA, login, paywall or anti-bot circumvention.
- Docker, PostgreSQL, Redis, message brokers, microservices or a plugin platform as runtime requirements.
- GitHub Actions or any hosted CI until a working MVP exists. Local Lefthook and APK checks are the verification path.
- A third deployment-framework repository. Whoosh Core is evaluated as an external deployment option later; it is not a runtime dependency.
- Production deployment before the MVP acceptance scenario is verified on a real Linux host.
- Migrating, modifying or switching off the existing Python Novel Tracker. Release Watcher is operated separately until an explicit cutover.

## Quality bar

The smallest correct behavior wins. A feature belongs in the MVP only when the founding user story needs it. Unknown source semantics stay unknown; missing data stays empty; errors are diagnosable and isolated per watch.
