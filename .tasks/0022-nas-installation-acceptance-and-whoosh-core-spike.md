# Task 0022 - NAS installation acceptance and Whoosh Core spike

State: blocked
Owner: none
Mode: mvp
Lane: deployment
Type: deployment
Scope: deployment,whoosh,acceptance,operations,docs
Risk: high
Parallel: false
Depends on: 0021
Tags: deployment

## Goal

Install the prebuilt artifact on the NAS under verified cgroup limits, run smoke/backup/restart/timer/rollback acceptance, and record whether Whoosh Core or a bounded SSH/SCP fallback is viable without touching other services.

## Context files

- AGENTS.md
- README.md
- docs/deployment.md
- docs/decisions.md
- docs/progress.md

## Files allowed to edit

- deploy/whoosh/**
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

1. Verify host identity
2. resource headroom and existing services; install only Release Watcher paths and units; run limited smoke with no Telegram; verify effective cgroup limits; enable timer only after smoke; test repeat deploy
3. version B
4. backup/restore and rollback; evaluate Whoosh Core prebuilt-artifact support and record the result.

## Acceptance criteria

- Installation uses the dev-side artifact and checksum; runtime is non-privileged and isolated; MemoryMax/CPUQuota/TasksMax/IOWeight are effective in cgroups; SQLite persists across restart; timer is daily and Persistent; rollback restores the previous binary and compatible database; existing services are untouched; Whoosh result is recorded honestly as success or fallback/blocked; no Telegram is configured unless explicitly provided.

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
- `{"id":"nas-live-acceptance","type":"manual","required":true,"environment":"live","profile":"trusted","instruction":"Run the installation, cgroup, timer, backup/restore, upgrade and rollback acceptance on the NAS and record the transcript.","evidenceRef":"NAS deployment transcript"}`

## Documentation updates

- docs/deployment.md
- docs/decisions.md
- docs/progress.md
- README.md

## Notes

- Do not stop or restart ResLedger or Telegram agent. Do not reboot the NAS. Do not change global systemd/cgroups/firewall. If sudo or lingering is unavailable
- stop and record the exact blocker.
- block: Blocked before installation: non-interactive sudo is unavailable, so the system-level unit and dedicated release-watcher user cannot be created; user lingering is disabled, so a user-level timer would not survive logout/reboot. Resource audit is SAFE_TO_DEPLOY, a transient user-level smoke passed under cgroup limits, but no installation, timer enable, upgrade or rollback was performed.
- privilege check 2026-11-05: `sudo -n -l` allows `systemctl`, `cp`, `ln`, `rm`, `chown`, `mkdir`, `chmod`, `journalctl` and similar, but `sudo -n useradd/groupadd` and running the installer script require a password. A human must run the exact `groupadd`/`useradd` and installer commands from `docs/deployment.md`; do not bypass the restriction with transient root units.
