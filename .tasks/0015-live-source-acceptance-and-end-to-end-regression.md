# Task 0015 - Live source acceptance and end-to-end regression

State: doing
Owner: opencode-primary
Mode: mvp
Lane: verification
Type: test
Scope: integration,e2e,live,naro,notifications,tests,docs
Risk: high
Parallel: false
Depends on: 0011,0012,0013,0014
Tags: tests

## Goal

Run a polite live Narō smoke test against N2166FE and an isolated local end-to-end regression covering baseline, two new chapters, durable events, Telegram RU notification, retries, restart recovery and JSON output at every stage.

## Context files

- AGENTS.md
- README.md
- docs/integration-contract.md
- docs/deployment.md
- docs/progress.md

## Files allowed to edit

- tests/e2e/**
- tests/source/**
- tests/app/**
- scripts/**
- testdata/**
- internal/app/**
- internal/cli/**
- internal/notifier/**
- internal/store/**
- internal/source/**
- internal/config/**
- docs/progress.md
- docs/integration-contract.md
- README.md
- .tasks/**

## Files forbidden to edit

- deploy/**
- migrations/**

## Steps

1. Fetch the real Narō API and catalog for N2166FE without mass requests and record actual title/ncode/status/chapter count/pagination/dates; add a local-server E2E with temporary SQLite and fake Telegram covering 210->212 chapters
2. dedupe
3. failure
4. restart
5. redelivery and two-watch isolation; verify --json emits only JSON on stdout; record blocked if the network is unavailable; run canonical verification.

## Acceptance criteria

- Live Narō smoke records the actual current response and does not hard-code a permanent chapter count; no false baseline events; isolated E2E proves baseline 210
- unchanged scan
- 212 chapters produces two new chapters and one grouped RU message
- replay creates no events
- Telegram failure is retried after restart
- pending delivery flushes even when no watch is due
- two-watch isolation works
- and every stage returns valid JSON; network failure is recorded as blocked
- not passed; scripts/verify-push.sh passes.

## Review questions

- Does the E2E prove exactly two new chapters and one notification? Is a pending delivery still flushed when no watch is due? Is live failure reported as blocked?

## Counterexample searches

- Repeat scans after delivery failure and restart; scan with zero due watches; compare stdout JSON purity.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`
- `{"id":"live-naro-smoke","type":"manual","required":true,"environment":"live","profile":"trusted","instruction":"Run the polite live Narō smoke against https://ncode.syosetu.com/n2166fe/ and record the actual API/catalog response.","evidenceRef":"live Narō smoke transcript"}`

## Documentation updates

- docs/progress.md
- docs/integration-contract.md
- README.md

## Notes

- Do not use the user's real Telegram chat. Do not make mass requests. Do not pin the live chapter count permanently.
