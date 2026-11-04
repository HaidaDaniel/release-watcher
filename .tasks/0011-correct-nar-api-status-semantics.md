# Task 0011 - Correct Narō API status semantics

State: doing
Owner: opencode-primary
Mode: mvp
Lane: implementation
Type: bugfix
Scope: source,naro,tests,docs
Risk: high
Parallel: false
Depends on: 0007
Tags: bugfix

## Goal

Implement the official Narō API end semantics (0=completed/short story, 1=ongoing, missing/unknown=unknown), update fixtures, tests and documentation, and prove status transitions including ongoing->completed->ongoing.

## Context files

- AGENTS.md
- docs/architecture.md
- docs/integration-contract.md
- docs/decisions.md
- docs/progress.md

## Files allowed to edit

- internal/source/naro.go
- tests/source/naro_test.go
- testdata/naro_api.json
- tests/app/helpers_test.go
- tests/app/app_test.go
- docs/integration-contract.md
- docs/architecture.md
- docs/domain-model.md
- docs/decisions.md
- docs/progress.md
- README.md
- .tasks/**

## Files forbidden to edit

- internal/store/**
- cmd/**
- deploy/**
- scripts/**

## Steps

1. Replace the inverted end interpretation in internal/source/naro.go; update the Narō fixture and table-driven adapter tests for end=0
2. end=1
3. null
4. missing and unsupported values; add transition tests; correct JSON examples and status documentation; run focused source/app tests and canonical verification.

## Acceptance criteria

- end=0 maps to completed; end=1 maps to ongoing; null
- missing and unsupported values map to unknown with no status source; N2166FE is never treated as completed merely from inactivity; completed watches remain enabled; table-driven tests cover all values; transition tests cover ongoing->completed->ongoing; docs and JSON examples match the official specification; scripts/verify-push.sh passes.

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

- Does any path still infer completion from inactivity or map end=1 to completed?

## Counterexample searches

- Search fixtures
- docs and code for inverted end semantics; test unknown values.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- docs/integration-contract.md
- docs/architecture.md
- docs/domain-model.md
- docs/decisions.md
- docs/progress.md

## Notes

- Official spec: https://dev.syosetu.com/man/api/ . Do not infer completion from last update time. Do not disable watches on completed.
- Pre-fix observation at baseline `c172aad376c63dcbe914ddeca6cb0d2ecde70ac4`: `internal/source/naro.go` mapped `end=1` to `completed` and only `end=0`/other values to `ongoing`; `tests/source/naro_test.go` asserted `completed` for the `end=1` fixture. A polite live request to `https://api.syosetu.com/novelapi/api/?out=json&ncode=N2166FE` returned `end=1`, `general_all_no=210`, and `general_lastup=2023-08-01 22:25:09`, proving the baseline misreported the work as completed. Reproduction limits: one live API request, no catalog crawl, network required for the live observation; the baseline code and old test assertion are visible in the baseline diff.
