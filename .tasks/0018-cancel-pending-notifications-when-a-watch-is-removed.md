# Task 0018 - Cancel pending notifications when a watch is removed

State: todo
Owner: none
Mode: mvp
Lane: implementation
Type: bugfix
Scope: notifications,outbox,app,store,cli,migrations,tests,docs
Risk: high
Parallel: false
Depends on: 0016
Tags: bugfix

## Goal

After remove/unwatch, pending and failed deliveries for that watch must become terminal canceled and must never be sent later, while history, event ids, other watches, pause semantics and re-add behavior remain correct.

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
- docs/**
- README.md
- .tasks/**

## Files forbidden to edit

- internal/source/**
- internal/notifier/**
- internal/config/**
- deploy/**
- cmd/**
- scripts/**

## Steps

1. Add an append-only SQLite migration for a terminal canceled delivery status; cancel pending/failed deliveries for the removed watch in the same transaction that disables the watch; exclude canceled deliveries from due selection and counts; keep sent deliveries unchanged; expose the canceled count in remove output; add tests for pending
2. failed
3. sent
4. remove
5. re-add
6. restart and shared watches; update docs.

## Acceptance criteria

- remove atomically disables the watch and cancels only that watch's pending/failed deliveries; canceled deliveries never send after notify
- restart or re-add; sent deliveries stay sent; other watches are untouched; pause does not cancel deliveries; purge still deletes history; event ids remain stable; existing databases migrate safely; tests cover pending/failed/sent/remove/re-add/restart/shared watches; scripts/verify-push.sh passes.

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

- Can a canceled delivery be sent after re-add or restart? Are other watches affected? Is the migration safe?

## Counterexample searches

- Create pending and failed deliveries
- remove
- notify
- re-add
- scan
- restart; inspect delivery statuses and Telegram request counts.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- docs/domain-model.md
- docs/integration-contract.md
- docs/decisions.md
- docs/progress.md

## Notes

- Do not promise exactly-once Telegram delivery. Do not delete history. Do not cancel on pause. Do not alter released migrations.
