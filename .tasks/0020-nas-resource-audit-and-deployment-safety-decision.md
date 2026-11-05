# Task 0020 - NAS resource audit and deployment safety decision

State: done
Owner: opencode-primary
Mode: mvp
Lane: research
Type: audit
Scope: nas,audit,resources,deployment,docs
Risk: medium
Parallel: false
Depends on: 0019
Tags: audit

## Goal

Audit the home NAS read-only, record generalized resource capacity and existing-service usage, and decide SAFE_TO_DEPLOY or BLOCKED without committing private infrastructure details.

## Context files

- AGENTS.md
- README.md
- docs/deployment.md
- docs/decisions.md
- docs/progress.md

## Files allowed to edit

- docs/**
- README.md
- .tasks/**

## Files forbidden to edit

- internal/**
- cmd/**
- migrations/**
- deploy/**
- scripts/**

## Steps

1. Inspect OS/systemd/CPU/RAM/swap/cgroups/filesystems/PSI; inspect ResLedger and Telegram agent service resource use and limits; record the available budget
2. recommended limits and blockers in local tmp audit files and generalized docs; do not change NAS state.

## Acceptance criteria

- Audit covers OS/systemd/CPU/RAM/swap/load/cgroups/disk/PSI and existing ResLedger/Telegram usage; no secrets or private host details are committed; a SAFE_TO_DEPLOY or BLOCKED decision is recorded with recommended MemoryHigh/MemoryMax/CPUQuota/TasksMax/IOWeight; no NAS state is changed; pnpm exec apkit lint passes.

## Verification

- `{"id":"apk-lint","type":"automated","required":true,"environment":"local","profile":"deterministic","apkOperation":"lint"}`

## Documentation updates

- docs/deployment.md
- docs/decisions.md
- docs/progress.md

## Notes

- Read-only audit only. Do not print or commit SSH targets
- usernames
- internal IPs
- secrets or journal contents. Stop if memory/CPU/IO pressure is unsafe.
