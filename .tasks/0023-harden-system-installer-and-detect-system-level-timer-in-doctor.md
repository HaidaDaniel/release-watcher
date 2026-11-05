# Task 0023 - Harden system installer and detect system-level timer in doctor

State: doing
Owner: opencode-primary
Mode: mvp
Lane: implementation
Type: bugfix
Scope: installer,systemd,doctor,deployment,tests,docs
Risk: high
Parallel: false
Depends on: 0021
Tags: bugfix

## Goal

Make the system installer verify artifact, checksum, build identity and units before switching current, roll back current and units on failure, and make doctor detect and validate a system-level timer while keeping user-level support.

## Context files

- AGENTS.md
- README.md
- docs/deployment.md
- docs/decisions.md
- docs/progress.md
- scripts/install-system.sh
- internal/cli/cli.go

## Files allowed to edit

- scripts/install-system.sh
- internal/cli/cli.go
- tests/install/**
- tests/foundation/**
- docs/**
- README.md
- .tasks/**

## Files forbidden to edit

- internal/app/**
- internal/store/**
- internal/source/**
- internal/notifier/**
- migrations/**
- deploy/systemd/system/**
- cmd/**

## Steps

1. Refactor install-system.sh into validate/stage/commit phases with backups and rollback; verify units with systemd-analyze before commit; add system timer discovery and scope-aware doctor messages; add regression tests for failed install rollback and system timer detection; update docs.

## Acceptance criteria

- Installer rejects bad checksum
- wrong build identity
- invalid units and wrong architecture before changing current; on a later failure it restores the previous current symlink and unit files; existing services are untouched; doctor detects /etc/systemd/system/release-watcher.timer and drop-ins
- reports system scope
- keeps user-level detection
- and warns correctly when no timer exists; tests cover rollback and both timer scopes; scripts/verify-push.sh passes.

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

- Was a failing signal attempted before the fix, and are hypotheses distinguished from proven root cause?
- Does the fix address the root cause rather than only the symptom?

## Counterexample searches

- Search a UI-only defect, an intermittent race, an inaccessible provider, a host without a test framework, and an expensive reproducer.
- Search adjacent inputs, error paths, and repeated execution.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- docs/deployment.md
- docs/decisions.md
- docs/progress.md

## Notes

- Do not install on the NAS in this task. Do not change other services. Do not enable the timer. Preserve user-level behavior.
