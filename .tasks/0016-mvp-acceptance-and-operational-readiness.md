# Task 0016 - MVP acceptance and operational readiness

State: doing
Owner: opencode-primary
Mode: mvp
Lane: verification
Type: release
Scope: release,acceptance,operations,docs
Risk: high
Parallel: false
Depends on: 0015
Tags: release

## Goal

Verify the functional and deployed-MVP readiness claims honestly: full tests including race, vet, builds, verify-push, APK lint, Linux amd64/arm64 artifacts, build identity, SQLite migration/restart, real backup/restore, doctor, systemd units, live Narō smoke and local E2E, then update status documents.

## Context files

- AGENTS.md
- README.md
- docs/project.md
- docs/scope.md
- docs/architecture.md
- docs/deployment.md
- docs/progress.md
- docs/roadmap.md

## Files allowed to edit

- docs/**
- README.md
- .tasks/**
- scripts/**

## Files forbidden to edit

- internal/**
- cmd/**
- migrations/**
- deploy/**

## Steps

1. Run the complete acceptance matrix; verify JSON stdout purity and Ctrl+C/lock/timeout behavior; record functional MVP evidence; check Linux build identity and installation readiness; update project/progress/roadmap/README with planned/implemented/tested/deployed/blocked states; leave Whoosh deployment blocked without a real test host and do not claim deployed MVP.

## Acceptance criteria

- go test ./...
- go test -race ./...
- go vet ./...
- go build ./...
- scripts/verify-push.sh and pnpm exec apkit lint pass; linux/amd64 and linux/arm64 build; build identity is honest; SQLite migration and restart work; a real backup restores data; doctor passes; systemd units validate; live Narō smoke and local E2E evidence exist or are explicitly blocked; --json writes only JSON to stdout; documentation separates implemented/tested/deployed/blocked; scripts/verify-push.sh passes.

## Correctness assumptions

- Release evidence must identify the exact immutable candidate SHA/tree.
- Some release checks can only execute after publication and must be recorded as post-tag evidence.
- A release-note file committed inside the tag cannot contain its own commit SHA or the later tag/CI/install identity.

## Invariants

- Pre-tag and post-tag evidence are temporally distinct and never conflated.
- A published tag is immutable; the tagged artifact and recorded evidence refer to the same candidate tree.
- Tagged release notes contain only facts knowable before tagging.

## Required evidence

- live
- report
- evidence
- Pre-tag criteria observed against the frozen candidate SHA, exact-SHA hosted CI, annotated tag peel to that SHA, and a distinct post-tag cold-install/released-consumer record.

## Review questions

- Are deployed and tested claims separated? Is any live evidence actually simulated?

## Counterexample searches

- Check stdout purity with logs enabled; restore a backup into a fresh state directory; inspect build identity.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`
- `{"id":"acceptance-report","type":"manual","required":true,"environment":"local","profile":"report","instruction":"Record the full acceptance matrix and honest deployed/blocked status in the task report.","evidenceRef":"task report"}`
- `{"id":"race-tests","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"go test -race ./...","evidenceRef":"local race test run"}`
- `{"id":"apk-lint","type":"automated","required":true,"environment":"local","profile":"deterministic","apkOperation":"lint"}`
- `{"id":"live-naro-smoke","type":"manual","required":true,"environment":"live","profile":"trusted","instruction":"Run the polite live Narō smoke and record the actual API/catalog response.","evidenceRef":"live Narō smoke transcript"}`

## Documentation updates

- docs/project.md
- docs/progress.md
- docs/roadmap.md
- README.md

## Notes

- Do not declare Deployed MVP without a real Linux host deployment. Whoosh stays blocked if no test host exists. Do not create cosmetic follow-up tasks.
