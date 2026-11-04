# Task 0004 - Sources: SSRF-safe HTTP client, Naro and RSS/Atom adapters

State: todo
Owner: none
Mode: mvp
Lane: implementation
Scope: source,http,security,docs
Risk: high
Parallel: false
Depends on: none
Tags: none

## Goal

Implement a shared HTTP client with timeout, retry, pacing and SSRF protection, plus normalized Naro and RSS/Atom adapters that return metadata, release identities, completeness and field availability. No scan/detection or CLI product commands yet.

## Context files

- AGENTS.md
- docs/architecture.md
- docs/domain-model.md
- docs/integration-contract.md
- docs/decisions.md

## Files allowed to edit

- go.mod
- go.sum
- internal/config/**
- internal/source/**
- testdata/**
- tests/source/**
- docs/architecture.md
- docs/decisions.md
- docs/progress.md
- docs/domain-model.md

## Files forbidden to edit



## Steps

1. Implement internal/source/httpx with http/https enforcement, redirect limits, per-host pacing, retry/backoff for timeouts and 429/5xx, body limits and dial-time SSRF rejection.
2. Define normalized Snapshot, Metadata, Release, adapter registry and stable source error codes.
3. Implement the Naro adapter with official API metadata, catalog pagination, explicit status/field availability, complete/partial/count_only semantics and polite request behaviour.
4. Implement the RSS/Atom adapter with stable entry ids, date parsing, sliding-window semantics and unsupported-source detection.
5. Add fixtures and tests for SSRF, retries, pacing, Naro catalog/partial/count_only, RSS and Atom.

## Acceptance criteria

- The HTTP client rejects loopback/private targets by default, including redirects, and allows them only with explicit test configuration.
- Naro metadata and chapters are normalized with stable ids, and catalog failure degrades safely without false removals.
- RSS/Atom entries use stable ids and never imply removal when an old entry leaves the feed window.
- Source errors are typed and diagnosable without leaking secrets.
- go test ./..., go vet ./... and go build ./... pass.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- Update progress and any clarified adapter/SSRF decisions.

## Notes

- Do not add scan/detection, app use cases, CLI commands, scheduling or notifications in this task. Do not run mass live requests; fixtures are primary.
