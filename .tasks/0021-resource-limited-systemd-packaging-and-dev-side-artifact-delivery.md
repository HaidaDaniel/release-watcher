# Task 0021 - Resource-limited systemd packaging and dev-side artifact delivery

State: doing
Owner: opencode-primary
Mode: mvp
Lane: implementation
Type: feature
Scope: systemd,packaging,deployment,docs
Risk: high
Parallel: false
Depends on: 0020
Tags: none

## Goal

Add separate system-level NAS systemd units with explicit cgroup limits and a dev-side prebuilt-artifact release layout, without changing existing services or user-level templates.

## Context files

- AGENTS.md
- README.md
- docs/deployment.md
- docs/decisions.md
- docs/progress.md

## Files allowed to edit

- deploy/**
- scripts/**
- tests/install/**
- docs/**
- README.md
- .tasks/**

## Files forbidden to edit

- internal/**
- cmd/**
- migrations/**

## Steps

1. Add deploy/systemd/system units for a dedicated runtime user and limited oneshot service; add a dev-side artifact manifest/install helper that verifies commit
2. architecture and SHA-256; document release retention
3. backup retention
4. cgroup limits and rollback; keep user-level units intact.

## Acceptance criteria

- System-level units are separate and use MemoryAccounting/MemoryHigh/MemoryMax/CPUAccounting/CPUQuota/TasksAccounting/TasksMax/IOAccounting/IOWeight/Nice/IOSchedulingClass/TimeoutStartSec/LimitNOFILE/NoNewPrivileges/PrivateTmp/ProtectSystem/ProtectHome/RestrictSUIDSGID/RestrictRealtime as supported; no existing unit is edited; artifact manifest contains commit/version/GOOS/GOARCH/build time/SHA-256; dev-side install helper refuses wrong architecture or checksum; retention preserves the last verified backup; scripts/verify-push.sh passes.

## Correctness assumptions

- The deployed environment may restart or contain configuration from a prior version.

## Invariants

- A failed rollout does not leave an unready or irrecoverable deployment.

## Required evidence

- Install, restart, upgrade, readiness, permissions, persistence, and rollback evidence.

## Review questions

- What state remains if deployment fails after configuration or schema mutation?

## Counterexample searches

- Search clean-host, restart-during-upgrade, missing-permission, stale-config, and rollback paths.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- docs/deployment.md
- docs/decisions.md
- docs/progress.md
- README.md

## Notes

- Do not deploy in this task. Do not add Docker or a new framework. Do not modify user-level units.
