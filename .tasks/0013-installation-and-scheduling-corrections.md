# Task 0013 - Installation and scheduling corrections

State: doing
Owner: opencode-primary
Mode: mvp
Lane: implementation
Type: feature
Scope: operations,systemd,install,doctor,docs,tests
Risk: high
Parallel: false
Depends on: 0007
Tags: feature

## Goal

Make the default user-level schedule one run per day with Persistent=true, keep process-per-run semantics, document XDG paths and lingering, and warn in doctor when the timer frequency cannot honor configured watch intervals.

## Context files

- AGENTS.md
- README.md
- docs/architecture.md
- docs/deployment.md
- docs/integration-contract.md
- docs/progress.md

## Files allowed to edit

- deploy/systemd/**
- scripts/install.sh
- internal/cli/commands.go
- internal/cli/cli.go
- internal/store/inspect.go
- tests/install/**
- tests/foundation/**
- docs/deployment.md
- docs/architecture.md
- docs/decisions.md
- docs/progress.md
- README.md
- .tasks/**

## Files forbidden to edit

- internal/app/**
- internal/source/**
- internal/notifier/**
- migrations/**

## Steps

1. Change the shipped timer from hourly to a documented daily OnCalendar; update install rendering for XDG and custom paths; document loginctl enable-linger and reboot recovery without automatic enabling; add a read-only doctor check comparing the configured systemd timer cadence with stored watch intervals; update systemd/install tests; run canonical verification.

## Acceptance criteria

- The default timer runs once per day with Persistent=true and the service remains oneshot; missed runs are recovered by scan --due; per-watch intervals are preserved; doctor warns when a configured interval is shorter than the timer cadence and explains the systemd override; user-level XDG install and non-standard paths are tested; lingering is documented and not enabled automatically; systemd-analyze validation passes; scripts/verify-push.sh passes.

## Review questions

- Can the default timer still run hourly? Does doctor detect a 6h watch with a daily timer? Are custom XDG paths rendered safely?

## Counterexample searches

- Inspect timer unit text; install into temporary XDG dirs; query doctor with a short-interval watch.

## Verification

- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- docs/deployment.md
- docs/architecture.md
- docs/decisions.md
- docs/progress.md
- README.md

## Notes

- Do not add a daemon. Do not enable lingering silently. Do not claim a 6h watch is honored by a daily timer.
