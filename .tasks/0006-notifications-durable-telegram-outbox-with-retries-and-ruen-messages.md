# Task 0006 - Notifications: durable Telegram outbox with retries and RU/EN messages

State: done
Owner: opencode-primary
Mode: mvp
Lane: implementation
Scope: notifications,telegram,outbox,cli,docs
Risk: high
Parallel: false
Depends on: none
Tags: none

## Goal

Add an optional Telegram notifier and a durable SQLite outbox. New events enqueue pending deliveries transactionally, delivery retries are bounded and deduplicated, messages group multiple chapters and are formatted in RU/EN, and events remain readable through the CLI when Telegram is absent or failing.

## Context files

- AGENTS.md
- README.md
- docs/architecture.md
- docs/domain-model.md
- docs/integration-contract.md
- docs/decisions.md
- docs/progress.md

## Files allowed to edit

- internal/notifier/**
- internal/app/**
- internal/cli/**
- internal/store/**
- internal/config/**
- tests/app/**
- tests/cli/**
- README.md
- docs/architecture.md
- docs/domain-model.md
- docs/integration-contract.md
- docs/decisions.md
- docs/progress.md
- docs/project.md
- docs/roadmap.md

## Files forbidden to edit



## Steps

1. Implement a Telegram client with typed retryable/permanent errors and no secret leakage.
2. Enqueue one pending delivery per new event in the scan transaction when Telegram is configured.
3. Flush due deliveries with bounded retry/backoff, permanent-failure handling and dedupe; expose `notify` and scan notification counts.
4. Format grouped RU/EN messages for added, updated, removed, metadata/status and count-only events.
5. Test success, failure then retry, non-retryable failure, grouping, RU/EN text, no duplicates and no Telegram configured.

## Acceptance criteria

- A new event with Telegram configured creates exactly one delivery and sends one grouped message.
- A failed delivery is retried after backoff and succeeds without creating a second event or duplicate message.
- Events remain stored and readable when Telegram is absent or failing.
- RU and EN messages are covered by tests, and no token appears in logs or errors.
- go test ./..., go vet ./... and go build ./... pass.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- Update progress and notification/outbox contract details.

## Notes

- Do not make Telegram mandatory. Do not add delivery for historical events or a web UI.
