# Task 0003 - Persistence: domain model, SQLite schema and repositories

State: done
Owner: opencode-primary
Mode: mvp
Lane: implementation
Scope: domain,storage,docs
Risk: high
Parallel: false
Depends on: none
Tags: none

## Goal

Implement the minimal domain entities and a pure-Go SQLite store with append-only migrations, indexes, transactional writes, event dedupe, outbox tables and restart-safe state. No adapters or CLI product commands yet.

## Context files

- AGENTS.md
- docs/domain-model.md
- docs/architecture.md
- docs/integration-contract.md
- docs/decisions.md

## Files allowed to edit

- go.mod
- go.sum
- internal/domain/**
- internal/id/**
- internal/store/**
- migrations/**
- tests/store/**
- docs/domain-model.md
- docs/architecture.md
- docs/decisions.md
- docs/progress.md

## Files forbidden to edit



## Steps

1. Add modernc.org/sqlite and document the dependency decision if not already recorded.
2. Define domain types for Work, Publication, Source, Watch, Release, Event, NotificationDelivery, ScanRun and settings with explicit unknown/optional values.
3. Add migration 0001 with tables, foreign keys, uniqueness and indexes for due watches, releases and events.
4. Implement Open/Migrate/InTx and repositories for all MVP entities, including event and delivery dedupe.
5. Add store tests for migrations, restart, rollback, due-watch selection and dedupe.

## Acceptance criteria

- Opening a fresh database applies schema version 1; reopening is idempotent.
- A transaction rollback leaves no partial rows.
- Duplicate event and delivery inserts are rejected without creating a second row.
- Due watches are selected by enabled state and next_check_at.
- go test ./..., go vet ./... and go build ./... pass.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- Update docs/progress.md and any clarified domain/architecture details.

## Notes

- Keep the schema minimal. Do not add adapters, scan logic, CLI commands or notification sending in this task.
