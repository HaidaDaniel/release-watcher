# Task 0012 - Repository and local verification hardening

State: doing
Owner: opencode-primary
Mode: mvp
Lane: implementation
Type: security
Scope: repository,git,hooks,security,scripts,tests,docs
Risk: high
Parallel: false
Depends on: 0007
Tags: security

## Goal

Harden repository hygiene and local verification: ignore secrets/runtime state, make the secret scanner non-leaking and staged-aware, and make APK lint mandatory on pre-push instead of silently skipping when pnpm is absent.

## Context files

- AGENTS.md
- README.md
- docs/decisions.md
- docs/progress.md
- lefthook.yml
- scripts/verify-fast.sh
- scripts/verify-push.sh
- scripts/check-secrets.sh

## Files allowed to edit

- .gitignore
- scripts/check-secrets.sh
- scripts/verify-push.sh
- scripts/verify-fast.sh
- lefthook.yml
- tests/install/**
- tests/foundation/**
- docs/decisions.md
- docs/progress.md
- docs/task-system.md
- README.md
- .tasks/**

## Files forbidden to edit

- internal/**
- cmd/**
- migrations/**
- deploy/**

## Steps

1. Extend .gitignore for .env variants
2. Telegram token configs
3. SQLite/WAL and backups/logs while keeping fixtures and .env.example; rewrite check-secrets.sh to scan staged additions and print only file/location/type without secret values; add tests proving no secret text reaches stdout/stderr; make verify-push fail clearly when pnpm/APK is unavailable; keep pre-commit fast; run canonical verification.

## Acceptance criteria

- Ignored patterns cover .env and .env.* except .env.example
- Telegram token configs
- local SQLite/WAL files
- temporary backups and runtime logs/cache; test fixtures and public config samples remain tracked; secret scanner prints only file/location/type and never the matched secret; scanner covers staged additions; verify-push always runs gofmt/secret scan/go test/go vet/go build/APK lint and fails clearly without pnpm; tests prove secret non-leakage; scripts/verify-push.sh passes.

## Correctness assumptions

- Inputs, callers, and dependencies may be malicious or compromised.

## Invariants

- Unauthorized actors cannot cross the protected capability boundary.

## Required evidence

- report
- evidence
- Negative-path, fail-closed, leakage, privilege, and auth-boundary results.

## Review questions

- Can any secret value still appear in scanner output? Can APK lint still be skipped silently?

## Counterexample searches

- Create staged fake tokens and run the scanner; run verify-push with PATH lacking pnpm.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`
- `{"id":"security-report","type":"manual","required":true,"environment":"local","profile":"report","instruction":"Record the scanner non-leakage, staged-only coverage and mandatory APK lint evidence in the task report.","evidenceRef":"task report"}`

## Documentation updates

- docs/decisions.md
- docs/progress.md
- docs/task-system.md
- README.md

## Notes

- Do not add GitHub Actions. The secret scanner is an accident guard
- not a Git-history audit. Do not ignore test fixtures or .env.example.
