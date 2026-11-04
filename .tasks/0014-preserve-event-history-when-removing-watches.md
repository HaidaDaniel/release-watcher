# Task 0014 - Preserve event history when removing watches

State: todo
Owner: none
Mode: mvp
Lane: implementation
Type: bugfix
Scope: persistence,store,app,cli,domain,migrations,docs,tests
Risk: high
Parallel: false
Depends on: 0007
Tags: bugfix

## Goal

Change remove/unwatch so it stops tracking but preserves releases, events, deliveries and scan history; add an explicit destructive purge with confirmation; keep event ids stable and make re-adding the same URL safe without duplicate data or old-chapter notifications.

## Context files

- AGENTS.md
- README.md
- docs/domain-model.md
- docs/integration-contract.md
- docs/decisions.md
- docs/progress.md

## Files allowed to edit

- migrations/**
- internal/store/**
- internal/app/**
- internal/cli/**
- internal/domain/**
- tests/app/**
- tests/cli/**
- tests/store/**
- docs/domain-model.md
- docs/integration-contract.md
- docs/decisions.md
- docs/progress.md
- README.md
- .tasks/**

## Files forbidden to edit

- internal/source/**
- internal/notifier/**
- deploy/**

## Steps

1. Add an append-only migration for unwatch state; implement remove as stop-tracking that keeps publication history and disables scheduling; add purge as the explicit destructive operation; filter removed watches from default list while keeping show/export available; make add re-enable an existing unwatched publication without creating duplicate rows or replaying old chapters; update CLI help
2. JSON contract
3. domain docs and tests; run canonical verification.

## Acceptance criteria

- remove/unwatch preserves releases
- events
- deliveries and scan runs; event_id remains stable; events export still works after unwatch; purge is the only operation that deletes observation history and requires explicit confirmation; re-adding the same URL reuses existing publication/watch rows and does not duplicate releases or notify about already-known chapters; existing SQLite databases migrate safely; other watches are untouched; scripts/verify-push.sh passes.

## Correctness assumptions

- A reproducer or captured failing observation is usually available but not universally deterministic or economical.
- A plausible hypothesis and a successful patch alone do not establish root cause.

## Invariants

- No implementation patch precedes a documented failing-signal attempt; objective reproduction limits stay explicit.
- No forced red test is required for unsupported, flaky, UI-only, or provider-dependent cases.
- The fix preserves behavior outside the reported defect.

## Required evidence

- Pre-fix failing signal or captured best-effort observation with environment and limits, plus the post-fix result and regression output where practical.

## Review questions

- Does remove still delete events through a cascade? Can re-add duplicate releases or emit old-chapter events?

## Counterexample searches

- Remove a watch with events
- export events
- re-add same URL
- scan; run purge and compare rows.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- docs/domain-model.md
- docs/integration-contract.md
- docs/decisions.md
- docs/progress.md
- README.md

## Notes

- Preserve stable event ids and export history. Do not delete another watch's data. Do not make destructive purge the default.
