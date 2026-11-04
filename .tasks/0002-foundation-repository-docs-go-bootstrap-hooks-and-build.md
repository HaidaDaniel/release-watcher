# Task 0002 - Foundation: repository docs, Go bootstrap, hooks and build

State: done
Owner: opencode-primary
Mode: mvp
Lane: implementation
Scope: docs,build,tooling,cli
Risk: medium
Parallel: false
Depends on: none
Tags: none

## Goal

Establish the repository foundation for Release Watcher: accurate product and architecture documentation, non-goals, a minimal Go CLI bootstrap with build identity, Lefthook fast/push verification, build scripts, and systemd unit skeletons. No product data features yet.

## Context files

- AGENTS.md
- docs/project.md
- docs/scope.md
- docs/architecture.md
- docs/task-system.md
- docs/decisions.md

## Files allowed to edit

- README.md
- go.mod
- go.sum
- cmd/release-watcher/main.go
- internal/buildinfo/**
- internal/config/**
- internal/logging/**
- internal/cli/**
- scripts/**
- deploy/systemd/**
- lefthook.yml
- .gitignore
- package.json
- pnpm-lock.yaml
- docs/project.md
- docs/scope.md
- docs/architecture.md
- docs/domain-model.md
- docs/decisions.md
- docs/roadmap.md
- docs/progress.md
- docs/deployment.md
- docs/integration-contract.md
- tests/foundation/**

## Files forbidden to edit



## Steps

1. Write the project, scope, architecture, domain-model, decisions, roadmap, progress, deployment and integration-contract documents with explicit non-goals and honest implementation status.
2. Bootstrap go.mod, cmd/release-watcher and internal/buildinfo/config/logging/cli for version, help and a non-functional doctor placeholder.
3. Add lefthook.yml, scripts/verify-fast.sh, scripts/verify-push.sh, scripts/build.sh and systemd service/timer files.
4. Verify hooks are installed and fire, then run the canonical verification and record results.

## Acceptance criteria

- Documentation describes only implemented or explicitly planned capabilities and lists non-goals.
- go build ./..., go vet ./... and go test ./... pass.
- release-watcher version --json returns stable build identity fields.
- Lefthook pre-commit and pre-push fire locally, and pre-commit runs gofmt on changed Go files.
- No GitHub Actions workflow is created.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh"}`

## Documentation updates

- Update docs/progress.md with planned/implemented/tested/blocked status.

## Notes

- CI remains disabled until a working MVP exists. Do not implement domain, storage, adapters or notifications in this task.
