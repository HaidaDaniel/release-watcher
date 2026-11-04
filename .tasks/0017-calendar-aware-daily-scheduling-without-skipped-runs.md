# Task 0017 - Calendar-aware daily scheduling without skipped runs

State: doing
Owner: opencode-primary
Mode: mvp
Lane: implementation
Type: bugfix
Scope: scheduling,app,config,tests,docs
Risk: high
Parallel: false
Depends on: 0016
Tags: bugfix

## Goal

Eliminate skipped daily scans caused by timer jitter while preserving per-watch intervals, manual runs, restart recovery, DST correctness and the zero-idle architecture.

## Context files

- AGENTS.md
- README.md
- docs/architecture.md
- docs/deployment.md
- docs/decisions.md
- docs/progress.md
- docs/integration-contract.md

## Files allowed to edit

- internal/app/**
- internal/config/**
- internal/scan/**
- tests/app/**
- tests/scan/**
- tests/foundation/**
- docs/**
- README.md
- .tasks/**

## Files forbidden to edit

- migrations/**
- internal/store/**
- internal/notifier/**
- internal/source/**
- cmd/**
- deploy/**
- scripts/**

## Steps

1. Analyze calendar scheduling and grace windows; implement scheduler-agnostic next-due computation for day-multiple intervals using the configured local timezone; keep sub-day intervals duration-based; update add
2. scan success
3. interval changes and restore; add deterministic regression tests for the seven required scenarios; document the decision in an ADR.

## Acceptance criteria

- A 24h watch scanned at 04:29 is due at the next day's 04:18 timer run; an evening add is due at the first morning run; missed days are caught up once; 6h/12h/24h/48h intervals behave deterministically; manual scan --all does not shift the calendar grid unexpectedly; a second scan in the same window is a no-op; DST transitions use the configured timezone; no new daemon or timer is added; scripts/verify-push.sh passes.

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

- Can any daily timer jitter still skip a day? Does manual scan shift the grid? Are DST days 23/25h handled?

## Counterexample searches

- Run day1 04:29/day2 04:18
- evening add
- missed days
- 6/12/24/48h
- same-window replay and Europe/Berlin DST transitions.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- docs/decisions.md
- docs/architecture.md
- docs/deployment.md
- docs/progress.md

## Notes

- Do not solve this by shortening 24h to 23h. Do not couple domain logic to systemd. Use managed test time and local fixtures.
