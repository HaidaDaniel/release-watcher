# Task 0025 - MCP authentication, HTTP hardening and systemd packaging

State: doing
Owner: opencode-primary
Mode: mvp
Lane: implementation
Type: security
Scope: mcp,auth,http,security,systemd,packaging,docs
Risk: high
Parallel: false
Depends on: 0024
Tags: security

## Goal

Protect the MCP endpoint with a single static 256-bit Bearer token, server-side read/write mode, HTTP hardening and a separate resource-limited systemd service without changing the existing scanner units.

## Context files

- AGENTS.md
- README.md
- docs/deployment.md
- docs/decisions.md
- docs/progress.md

## Files allowed to edit

- internal/mcp/**
- internal/config/**
- internal/cli/**
- deploy/systemd/system/**
- scripts/**
- tests/mcp/**
- tests/install/**
- docs/**
- README.md
- .tasks/**

## Files forbidden to edit

- internal/app/**
- internal/store/**
- internal/source/**
- internal/notifier/**
- migrations/**
- cmd/**

## Steps

1. Add MCP token/listen/write config; constant-time Bearer auth on every MCP request; omit and reject mutation tools in read-only mode; add Origin/Host/body/timeout/concurrency protections; add release-watcher-mcp.service with cgroup limits and a protected mcp.env; add security tests and deployment docs.

## Acceptance criteria

- Missing/wrong/empty/malformed bearer tokens get 401 without leaking state; valid token works; write tools are not advertised or executable in read-only mode and are executable only when enabled; token is never logged or returned; request body/timeouts/concurrency/session bounds are enforced; service runs as release-watcher with MemoryHigh=128M/MemoryMax=256M/MemorySwapMax=128M/CPUQuota=25%/TasksMax=64/IOWeight=50; existing scanner units unchanged; security tests pass; scripts/verify-push.sh passes.

## Correctness assumptions

- Inputs, callers, and dependencies may be malicious or compromised.

## Invariants

- Unauthorized actors cannot cross the protected capability boundary.

## Required evidence

- Negative-path, fail-closed, leakage, privilege, and auth-boundary results.

## Review questions

- Is any default, error path, or fallback more permissive than the success path?

## Counterexample searches

- Search missing credentials, confused deputy, privilege escalation, injection, and secret-in-log paths.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`
- `{"id":"security-report","type":"manual","required":true,"environment":"local","profile":"report","instruction":"Record bearer-auth, read/write mode, HTTP hardening, secret non-leakage and cgroup packaging evidence.","evidenceRef":"security test report"}`

## Documentation updates

- docs/deployment.md
- docs/decisions.md
- docs/progress.md
- README.md

## Notes

- No OAuth
- JWT
- accounts or sessions. Do not put the token in Git or args. Do not modify existing scanner units.
