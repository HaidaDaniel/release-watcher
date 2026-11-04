# Deployment

Status: documented target, not deployed. The MVP installation proof and the Whoosh Core spike happen in task 0007. Nothing in this document claims a live deployment.

## Runtime shape

- One statically linked (or pure-Go) binary: `release-watcher`.
- One state directory outside the source tree, defaulting to `$XDG_DATA_HOME/release-watcher` (normally `~/.local/share/release-watcher`) or `RELEASE_WATCHER_STATE_DIR`.
- SQLite database in that directory, created `0600` by the process.
- A user-level systemd service (oneshot) and timer. No root daemon, no Docker, no PostgreSQL, no Redis.

## Build

```sh
scripts/build.sh --os linux --arch amd64
scripts/build.sh --os linux --arch arm64
scripts/build.sh --all
```

The script uses `CGO_ENABLED=0`, `-trimpath`, and injects version, commit, build time and build mode. A plain `go build` honestly reports `0.1.0-dev` / `unknown` / `development`.

## Install without root

```sh
install -Dm0755 bin/release-watcher-linux-amd64 ~/.local/bin/release-watcher
install -Dm0644 deploy/systemd/release-watcher.service ~/.config/systemd/user/release-watcher.service
install -Dm0644 deploy/systemd/release-watcher.timer ~/.config/systemd/user/release-watcher.timer
systemctl --user daemon-reload
systemctl --user enable --now release-watcher.timer
systemctl --user list-timers release-watcher.timer
```

The timer runs hourly with `Persistent=true`, so a missed run is recovered. `scan --due` is a cheap no-op when no watch is due. The default interval is 24h; if a watch uses a shorter interval, the timer must run at least that often. Change `OnCalendar` in the timer unit and reload it:

```sh
systemctl --user edit release-watcher.timer
systemctl --user daemon-reload
systemctl --user restart release-watcher.timer
```

There is no promise that a 6h interval is honored by a daily timer. The timer frequency is part of the installation contract.

## Configuration

Environment variables are read at process start. Put secrets only in a protected environment file, for example `~/.config/release-watcher/env` (`0600`), and reference it from a drop-in or the service unit.

| Variable | Default | Meaning |
| --- | --- | --- |
| `RELEASE_WATCHER_STATE_DIR` | `$XDG_DATA_HOME/release-watcher` | state directory |
| `RELEASE_WATCHER_DB` | `<state>/release-watcher.db` | SQLite path |
| `RELEASE_WATCHER_DEFAULT_INTERVAL` | `24h` | default watch interval |
| `RELEASE_WATCHER_HTTP_TIMEOUT` | `30s` | per-request timeout |
| `RELEASE_WATCHER_LANG` | `ru` | human/notification language (`ru`/`en`) |
| `RELEASE_WATCHER_LOG_FORMAT` | `text` | `text` or `json` |
| `RELEASE_WATCHER_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `RELEASE_WATCHER_TELEGRAM_BOT_TOKEN` | unset | optional Telegram bot token |
| `RELEASE_WATCHER_TELEGRAM_CHAT_ID` | unset | optional Telegram chat id |
| `RELEASE_WATCHER_ALLOW_PRIVATE_HOSTS` | `false` | test/dev only; weakens SSRF protection |

Secrets are never logged and never stored in SQLite.

## Backup and restore

The binary provides `backup` and `restore` (task 0007). The backup is a single readable archive containing a manifest and a consistent SQLite copy. Restore requires the CLI to be the only process using the database and validates the archive before replacing live state. Run a backup before every upgrade.

## Upgrade and rollback

1. Run `release-watcher backup --out <file>`.
2. Install the new binary over the old path.
3. Run `release-watcher doctor` and `release-watcher scan --due`.
4. If migrations or behavior fail, stop the timer, restore the backup with the previous binary, then re-enable the timer.

## Whoosh Core assessment (not executed)

Whoosh Core is a single-binary SSH deployment tool that builds timestamped releases, links shared paths, atomically swaps a `current` symlink and supports rollback. It can run in a local mode and via SSH, and supports staged tasks/hooks. That matches most of the desired deployment shape:

- deliver a prebuilt binary to Linux x86-64/ARM64 targets;
- keep the SQLite state in a shared directory outside release directories;
- run backup/migration/smoke/restart tasks through hooks;
- atomically switch `current` and roll back.

Open questions for the spike:

- whether Whoosh can publish a **prebuilt local artifact** without making the target build from Git (the README documents Git-based releases and a local mode; the exact prebuilt-artifact path must be verified);
- how SQLite backup runs before a migration hook and how a failed smoke test rolls back;
- service restart ordering for a oneshot scan timer;
- repeatable verification on ARM64 and a second deploy with existing state.

The spike is a separate backlog task after MVP acceptance. No Whoosh config is treated as verified until it runs on a test host. Whoosh is not a runtime dependency and no third deployment-framework repository is created.

## Security

- Run as an unprivileged user.
- Keep the state directory `0700` and the database `0600`.
- Do not expose a network port; there is none in the MVP.
- Do not put tokens in command lines, URLs, logs, Git or test fixtures.
- The SSRF guard blocks private targets by default. Do not enable `RELEASE_WATCHER_ALLOW_PRIVATE_HOSTS` in production.
