# Release Watcher

A small self-hosted watcher for new chapters and volumes from public sources. One Go binary, SQLite state, a CLI and an optional daily systemd timer. No server, no Docker, no LLM during scans.

Status: MVP foundation. The repository, docs, build identity and local hooks are in place. Storage, source adapters, scanning, notifications and installation proof are being implemented task by task. See [docs/progress.md](docs/progress.md).

## Build from source

```sh
go build -o bin/release-watcher ./cmd/release-watcher
# or cross-compile:
scripts/build.sh --all
```

`version` reports honest build identity. A plain `go build` reports `0.1.0-dev` / `unknown` / `development`.

## Quick start (after the MVP tasks land)

```sh
release-watcher add https://ncode.syosetu.com/n2166fe/
release-watcher list --json
release-watcher scan --due --json
release-watcher events --json
```

The first `add` stores a baseline, so an old 210-chapter work does not produce a false "210 new chapters" event.

## Documentation

- [Project](docs/project.md) and [scope/non-goals](docs/scope.md)
- [Architecture](docs/architecture.md)
- [Domain model](docs/domain-model.md)
- [Integration contract](docs/integration-contract.md)
- [Roadmap](docs/roadmap.md) and [progress](docs/progress.md)
- [Deployment](docs/deployment.md)
- [Decisions](docs/decisions.md)

## Development workflow

APK v0.5.2 is pinned as a dev dependency for task contracts, verification, review and gates. Lefthook runs fast checks on commit and the full local verification on push. CI is intentionally absent until a working MVP exists.

```sh
pnpm install --frozen-lockfile
pnpm exec lefthook install
scripts/verify-fast.sh
scripts/verify-push.sh
```

Node.js and pnpm are required only for the development workflow, never for production.

## Boundaries

Release Watcher detects publication changes and stores events. Media Core (separate repository) owns personal reading history and content delivery. They share a versioned JSON event contract, not a database. See [docs/integration-contract.md](docs/integration-contract.md).
