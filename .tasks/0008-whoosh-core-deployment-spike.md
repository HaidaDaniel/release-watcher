# Task 0008 - Whoosh Core deployment spike

State: todo
Owner: none
Mode: mvp
Lane: deployment
Scope: deployment
Risk: medium
Parallel: false
Depends on: 0016
Tags: none

## Goal

Run a recorded Whoosh Core spike on a test Linux host: deploy the prebuilt Release Watcher binary to versioned releases, keep SQLite in shared state, trigger the systemd timer, run backup/doctor/smoke checks, repeat the deploy with existing state, and verify rollback. Record the exact Whoosh commit/version and any limitations.

## Context files

- AGENTS.md
- docs/deployment.md
- docs/decisions.md

## Files allowed to edit

- deploy/whoosh/**
- docs/deployment.md
- docs/decisions.md
- docs/progress.md

## Files forbidden to edit



## Steps

1. Pin and record the exact Whoosh Core release/commit used by the spike.
2. Configure a stage that publishes a prebuilt linux binary, links shared SQLite state, runs backup/doctor/smoke hooks and restarts the timer.
3. Deploy twice with existing state, verify version identity, timer, events and SQLite persistence.
4. Force a failed smoke/rollback and verify the previous release plus database state recover.

## Acceptance criteria

- The spike runs on a real test host and records exact commands, versions, and outputs.
- A repeat deploy preserves SQLite state and the timer remains active.
- Rollback restores the previous release and a pre-upgrade backup can be restored.

## Verification

- `{"id":"contract-lint","type":"automated","required":true,"environment":"local","profile":"deterministic","apkOperation":"lint"}`
- `{"id":"live-whoosh-deploy","type":"manual","required":true,"environment":"live","profile":"trusted","instruction":"Deploy the prebuilt binary with Whoosh Core to a test host, verify timer/state, repeat deploy, and roll back with SQLite state preserved.","evidenceRef":"test-host deploy transcript and /version output"}`

## Documentation updates

- Record the spike result in docs/deployment.md and docs/progress.md.

## Notes

- Do not call this production deployment. Do not turn Whoosh into a runtime dependency or add a third deployment repository.
