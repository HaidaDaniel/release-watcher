# Release Watcher

Status: MVP vertical slice. The repository foundation, SQLite persistence, Narō/RSS/Atom adapters, scan detection and the CLI vertical slice are implemented. Telegram delivery, backup/restore, installation proof and the Whoosh deployment spike are still planned; this document separates those states explicitly.

## Mission

Release Watcher is a small, self-hosted, agent-first tool for noticing when serialized content publishes something new.

The founding user story is narrow: a reader finishes a web novel that stopped updating, registers its URL once, and later receives a notification if the author ever continues. The tool must work without a permanently running server, without an LLM during scans, and without Media Core.

## Product principles

1. Small useful core. One Go binary, SQLite state, and a CLI are enough for the MVP.
2. Agent-first, not AI-dependent. Every read and mutation has a stable JSON contract. Any agent can drive the same CLI the human uses.
3. Local-first. Adding, listing, pausing, inspecting history, backup and restore work without Internet. Only configured sources are contacted during scans.
4. Deterministic monitoring. Source adapters parse public APIs/feeds and catalogs; no model is used while scanning.
5. Facts, computed values and unknowns are distinct. A missing field is not invented, and stopped updates never imply official completion.
6. One publication line is one line. WN, LN, translations and mirrors are not merged by title and are not summed into one chapter count.
7. Durable events. Detected changes are committed to SQLite before any notification is attempted. Telegram is optional, not the source of truth.
8. MVP-sized infrastructure. No Docker, PostgreSQL, Redis, message broker, plugin platform, microservices, HTTP server or CI before a working MVP.

## Primary workflow

```text
release-watcher add https://ncode.syosetu.com/n2166fe/
  -> detect the source adapter
  -> fetch metadata and the current release catalog
  -> store a baseline (no "210 new chapters" notification)
  -> schedule the next check

release-watcher scan --due
  -> compare the current snapshot with the stored baseline
  -> store immutable events in SQLite
  -> enqueue an optional Telegram delivery
  -> record the scan result
```

## Current implementation status

| Area | State |
| --- | --- |
| Repository docs, non-goals, ADRs | implemented |
| Go module, CLI bootstrap, build identity | implemented |
| Lefthook fast/push checks, build scripts | implemented |
| SQLite domain storage and migrations | planned |
| Narō adapter and baseline detection | planned |
| RSS/Atom adapter | planned |
| `add`, `list`, `show`, `scan`, `events`, pause/resume/interval/remove | planned |
| Telegram outbox and retries | planned |
| Doctor (foundation config/environment checks) | implemented |
| Backup/restore | planned |
| systemd timer installation proof | planned |
| Whoosh Core deployment spike | planned, not executed |
| Media Core documentation | separate repository |

## Boundary

Release Watcher is not Media Core. It detects publications and produces events. Media Core, when built, owns personal reading history, progress, ratings, notes and content delivery. The two projects share a versioned event contract, not a database file.

See [scope](scope.md), [architecture](architecture.md), [domain model](domain-model.md), [roadmap](roadmap.md), [decisions](decisions.md), [integration contract](integration-contract.md) and [deployment](deployment.md).
