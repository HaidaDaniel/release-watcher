# Task 0028 - Enable MCP service autostart by default in the system installer

State: doing
Owner: opencode-primary-hy4
Mode: mvp
Lane: implementation
Type: bugfix
Scope: installer,mcp,systemd,tests,docs
Risk: medium
Parallel: false
Depends on: 0025
Tags: bugfix,mcp

## Goal

Make every system-level deployment enable `release-watcher-mcp.service` by default and bootstrap its token file, so the MCP daemon survives a reboot without a separate operator command.

## Context files

- AGENTS.md
- docs/decisions.md
- docs/deployment.md
- docs/mcp.md
- docs/progress.md
- scripts/install-system.sh
- scripts/install-mcp-env.sh
- deploy/systemd/system/release-watcher-mcp.service
- tests/install/systemd_test.go

## Files allowed to edit

- scripts/install-system.sh
- tests/install/systemd_test.go
- docs/deployment.md
- docs/decisions.md
- docs/mcp.md
- docs/progress.md
- .tasks/0028-enable-mcp-service-autostart-by-default.md

## Files forbidden to edit

- internal/**
- cmd/**
- migrations/**
- deploy/systemd/**
- tests/app/**
- tests/cli/**
- tests/e2e/**
- tests/foundation/**
- tests/mcp/**
- tests/scan/**
- tests/source/**
- tests/store/**

## Steps

1. Inspect the installer, the MCP unit and the existing token helper; confirm the root cause (unit installed but never enabled, so a reboot leaves it inactive).
2. In `install-system.sh`, create `/etc/release-watcher/mcp.env` on first install only (256-bit token, loopback bind, read-only mode, root-owned 0600) and then run `systemctl enable --now release-watcher-mcp.service`. Never overwrite an existing token file. Keep the scanner timer manual (smoke first).
3. Keep the installer rollback contract: on any failure after enablement, restore the prior enable state together with the previous units and `current` symlink.
4. Add a regression test that asserts token bootstrap, enablement, their order and rollback state.
5. Update deployment docs, an ADR and progress; state clearly that the NAS still needs a redeploy.

## Acceptance criteria

- A fresh `install-system.sh` run leaves `release-watcher-mcp.service` enabled and started without extra operator commands; `mcp.env` is generated once with a 256-bit token, loopback bind and write mode off, root-owned 0600, and an existing file is preserved.
- Failed installs restore the previous MCP enable state as well as units and `current`; the scanner timer remains not enabled by the installer.
- Tests assert these invariants and all verification commands pass.

## Correctness assumptions

- The target host has `openssl`, `python3`, `systemd-analyze` and `systemctl`, as already required by the token helper and installer.

## Invariants

- A failed install never leaves an enabled-but-unstartable MCP unit or a rotated token.

## Required evidence

- Local static and installer regression evidence; no live NAS change is claimed by this task.

## Review questions

- Does the generated token file match the existing helper, and does rollback remove an enablement created by a failed install?

## Counterexample searches

- Check repeat installs with an existing `mcp.env`, failure before and after enable, and a host with MCP already enabled.

## Verification

- `{"id":"installer-focused","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"go test ./tests/install -run 'TestSystemInstaller|TestSystemLevelUnitsAndPackagingScripts' -count=1 && bash -n scripts/install-system.sh","evidenceRef":"local focused installer test run"}`
- `{"id":"push-checks","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"scripts/verify-push.sh","evidenceRef":"local scripts/verify-push.sh run"}`

## Documentation updates

- docs/deployment.md
- docs/decisions.md
- docs/mcp.md
- docs/progress.md

## Notes

- Do not change the scanner timer policy; only the MCP service becomes automatic.
- Do not rotate or print tokens. Do not touch the live NAS from this environment.
