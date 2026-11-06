# Task 0024 - MCP Streamable HTTP protocol and application adapter

State: done
Owner: opencode-primary
Mode: mvp
Lane: implementation
Type: feature
Scope: mcp,app,cli,protocol,tests,docs
Risk: high
Parallel: false
Depends on: 0023
Tags: feature

## Goal

Add a Streamable HTTP MCP endpoint backed by the existing app.Service, with the required typed read-only and mutation tools and wire-level tests using the official Go MCP SDK.

## Context files

- AGENTS.md
- README.md
- docs/architecture.md
- docs/integration-contract.md
- docs/decisions.md
- docs/progress.md

## Files allowed to edit

- internal/mcp/**
- internal/app/**
- internal/cli/**
- internal/config/**
- cmd/**
- tests/mcp/**
- tests/app/**
- go.mod
- go.sum
- docs/**
- README.md
- .tasks/**

## Files forbidden to edit

- migrations/**
- deploy/**
- scripts/**
- internal/store/**
- internal/source/**
- internal/notifier/**

## Steps

1. Pin the official MCP Go SDK; add mcp serve mode and an HTTP handler; implement typed tools over app.Service with strict schemas and bounded results; add request limits
2. origin/host checks and graceful shutdown; add local MCP client tests and negative protocol tests; document the tool surface.

## Acceptance criteria

- MCP SDK v1.8.0 is pinned; /mcp Streamable HTTP initialize and tools/list work; list_watches
- get_watch
- list_events
- add_watch
- pause_watch
- resume_watch
- set_watch_interval and remove_watch reuse app.Service; no purge/backup/restore/shell/filesystem tools; results are bounded; errors use stable codes; context cancellation works; wire-level tests pass; scripts/verify-push.sh passes.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- docs/architecture.md
- docs/integration-contract.md
- docs/decisions.md
- docs/progress.md
- README.md

## Notes

- Do not implement MCP JSON-RPC manually. Do not add a second DB. Do not run a CLI subprocess per tool call.
