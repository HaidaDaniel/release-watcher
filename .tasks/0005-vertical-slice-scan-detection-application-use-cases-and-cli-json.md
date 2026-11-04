# Task 0005 - Vertical slice: scan detection, application use cases and CLI JSON

State: todo
Owner: none
Mode: mvp
Lane: implementation
Scope: app,scan,cli,i18n,lock,docs
Risk: high
Parallel: false
Depends on: none
Tags: none

## Goal

Deliver the first working Narō vertical slice: add a URL with a silent baseline, list/show watches, scan due/all/id with isolated errors, detect new/updated/removed releases and metadata changes, read events, pause/resume/interval/remove, and expose stable human and JSON output through the CLI.

## Context files

- AGENTS.md
- docs/architecture.md
- docs/domain-model.md
- docs/integration-contract.md
- docs/decisions.md
- docs/progress.md

## Files allowed to edit

- internal/app/**
- internal/scan/**
- internal/cli/**
- internal/i18n/**
- internal/lock/**
- internal/store/**
- internal/config/**
- tests/app/**
- tests/scan/**
- tests/cli/**
- docs/architecture.md
- docs/domain-model.md
- docs/integration-contract.md
- docs/decisions.md
- docs/progress.md
- docs/project.md
- docs/roadmap.md

## Files forbidden to edit



## Steps

1. Implement scan comparison with baseline, catalog add/update/remove, count-only changes, metadata/status changes and dedupe keys.
2. Implement application use cases for add/list/show/scan/events/pause/resume/interval/remove with idempotent add, baselines, per-watch error isolation and scan locking.
3. Implement the CLI commands with stable JSON envelopes, RU/EN human output, exit codes and source error mapping.
4. Add tests for first add without false events, duplicate add, one/many new chapters, replay without duplicates, restart, API failure recovery, RSS sliding window, invalid/unknown URLs, intervals, concurrent scans and machine-readable JSON.

## Acceptance criteria

- Adding the Narō fixture URL stores a baseline and emits no release event.
- A later scan detects one or many new chapters and repeated scans do not duplicate events.
- One failing watch does not stop other watches and is recorded with a diagnosable error.
- All read and mutation commands support stable --json output.
- State survives restart and scan locking prevents overlapping scans.
- go test ./..., go vet ./... and go build ./... pass.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- Update progress and any clarified CLI/scan contract details.

## Notes

- Do not implement Telegram delivery in this task. Do not run mass live requests; use fixtures and local test servers. Do not add MCP or a web UI.
