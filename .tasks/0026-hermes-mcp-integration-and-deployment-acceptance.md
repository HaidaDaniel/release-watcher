# Task 0026 - Hermes MCP integration and deployment acceptance

State: todo
Owner: none
Mode: mvp
Lane: deployment
Type: deployment
Scope: hermes,mcp,deployment,acceptance,security,docs
Risk: high
Parallel: false
Depends on: 0025
Tags: deployment

## Goal

Configure Hermes to use the remote Streamable HTTP MCP endpoint over a private TLS/VPN route and verify real client compatibility, read-only and write-mode behavior, security, SQLite persistence, deployment and rollback.

## Context files

- AGENTS.md
- README.md
- docs/deployment.md
- docs/decisions.md
- docs/progress.md

## Files allowed to edit

- deploy/hermes/**
- docs/**
- README.md
- .tasks/**
- scripts/**

## Files forbidden to edit

- internal/**
- cmd/**
- migrations/**
- deploy/systemd/system/**

## Steps

1. Inspect the Hermes host/config and private network route; add the MCP server without overwriting existing config; store the token in Hermes secrets; verify initialize
2. tools/list
3. read tools and 401 paths; enable write mode separately for a controlled test source and clean up; verify restart
4. timer coexistence
5. cgroups and rollback; record any missing network prerequisite honestly.

## Acceptance criteria

- Hermes connects over HTTPS/VPN with ssl_verify=true and no SSH; initialize and tools/list succeed; bad/missing tokens get 401; read tools return real NAS SQLite data; write tools are blocked in read-only mode and work after explicit write enable; duplicate add is idempotent; pause/resume/interval/remove work; restart persistence and timer coexistence are verified; ResLedger/Telegram agent untouched; rollback is verified; if no secure route exists
- implementation is marked done and deployment blocked with the exact missing prerequisite.

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
- `{"id":"hermes-live-acceptance","type":"manual","required":true,"environment":"live","profile":"trusted","instruction":"Run the real Hermes MCP client acceptance over the private secure route and record the transcript.","evidenceRef":"Hermes MCP acceptance transcript"}`

## Documentation updates

- docs/deployment.md
- docs/decisions.md
- docs/progress.md
- README.md

## Notes

- Do not use ssl_verify=false. Do not expose the MCP port to the public internet. Do not overwrite the whole Hermes config. Do not use production Telegram.
