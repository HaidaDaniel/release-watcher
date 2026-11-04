# Task 0007 - Installation: backup/restore, full doctor, systemd and Whoosh assessment

State: todo
Owner: none
Mode: mvp
Lane: implementation
Scope: operations,backup,doctor,systemd,deployment,docs
Risk: high
Parallel: false
Depends on: none
Tags: none

## Goal

Make the MVP installable and recoverable on Linux: consistent SQLite backup/restore, read-only schema-aware doctor, user systemd service/timer installation path, cross-compiled linux/amd64 and linux/arm64 artifacts, and a documented Whoosh Core deployment spike. No automatic production deployment.

## Context files

- AGENTS.md
- docs/architecture.md
- docs/deployment.md
- docs/decisions.md
- docs/integration-contract.md
- docs/progress.md

## Files allowed to edit

- internal/app/**
- internal/cli/**
- internal/store/**
- internal/config/**
- scripts/**
- deploy/**
- tests/app/**
- tests/cli/**
- tests/store/**
- tests/install/**
- docs/**
- README.md

## Files forbidden to edit



## Steps

1. Implement a consistent SQLite backup archive with manifest and hash, plus validated restore with schema compatibility and exclusivity.
2. Make doctor read-only and schema-aware without migrating, and check state/database permissions, Telegram and build identity.
3. Add a user-level install script and static checks for the systemd service/timer; document timer frequency versus intervals.
4. Cross-compile linux/amd64 and linux/arm64, run local binary smoke checks, and record actual installation evidence.
5. Document the Whoosh Core assessment and create separate backlog tasks for the deployment spike and the later ResLedger comparison.

## Acceptance criteria

- Backup creates a readable archive whose restore reproduces the database and survives restart.
- Restore fails closed on a bad archive, incompatible schema, or missing --force when a database exists.
- doctor reports schema/version/permissions without writing or migrating.
- Installation and systemd files are documented and statically verified; no production deploy is claimed.
- Linux amd64 and arm64 binaries build, and the amd64 binary passes version/doctor smoke checks.
- go test ./..., go vet ./... and go build ./... pass.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- Update deployment, progress, README and the Whoosh backlog.

## Notes

- Do not deploy to a production host in this task. Do not add Docker or a deployment framework repository. Mark Whoosh as assessed/documented, not verified live.
