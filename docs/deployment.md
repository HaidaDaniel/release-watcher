# Deployment

Status: local installation and recovery are implemented and statically verified; no live production deployment has been performed. The Whoosh Core deployment spike remains a separate backlog task. Nothing in this document claims a live deployment.

## Runtime shape

- One statically linked (or pure-Go) binary: `release-watcher`.
- One state directory outside the source tree, defaulting to `$XDG_DATA_HOME/release-watcher` (normally `~/.local/share/release-watcher`) or `RELEASE_WATCHER_STATE_DIR`.
- SQLite database in that directory, secured to `0600` by the process; SQLite sidecars live in the same private directory.
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
scripts/build.sh --all
scripts/install.sh --enable
release-watcher doctor --json
systemctl --user list-timers release-watcher.timer
```

`scripts/install.sh` installs the binary to `~/.local/bin`, the user service/timer to `$XDG_CONFIG_HOME/systemd/user` (normally `~/.config/systemd/user`), creates a private `~/.config/release-watcher/env` template, reloads the user manager and optionally enables the timer. Set `PREFIX` to install elsewhere and `XDG_DATA_HOME`/`RELEASE_WATCHER_STATE_DIR` to control the rendered state path. The installer renders the actual binary, env-file and state paths into the service rather than copying placeholders. Without `--enable`, enable it manually:

```sh
systemctl --user enable --now release-watcher.timer
```

The timer runs once per day at 04:15 local time with `Persistent=true`, so a missed run is recovered after boot or downtime. `scan --due` is a cheap no-op when no watch is due. The default interval is 24h; if a watch uses a shorter interval, the timer must run at least that often. Change `OnCalendar` in the timer unit and reload it:

```sh
systemctl --user edit release-watcher.timer
systemctl --user daemon-reload
systemctl --user restart release-watcher.timer
```

There is no promise that a 6h interval is honored by a daily timer. The timer frequency is part of the installation contract. `release-watcher doctor --json` reads the installed user timer, including `release-watcher.timer.d/*.conf` drop-ins, and warns when its cadence is slower than the shortest enabled watch interval, or when the timer unit cannot be found or parsed. Supported calendar forms include aliases (`hourly`, `daily`, `weekly`), daily clock times, hour/minute steps and weekday-prefixed schedules.

### User lingering and reboot recovery

The timer is user-level. Without lingering, systemd starts it only while the user has a session. For a home server that reboots without an interactive login, enable lingering explicitly after reviewing the security implications:

```sh
sudo loginctl enable-linger "$USER"
loginctl show-user "$USER" -p Linger
```

The installer does not enable lingering automatically; it prints the command when lingering is off. With lingering enabled, the daily timer and `Persistent=true` recover missed runs after reboot. Test this in an isolated user environment before relying on it. Use `systemctl --user list-timers release-watcher.timer` and `journalctl --user -u release-watcher.service` to diagnose scheduling.

## Configuration

Environment variables are read at process start. Put secrets only in a protected environment file, for example `~/.config/release-watcher/env` (`0600`), and reference it from a drop-in or the service unit.

| Variable | Default | Meaning |
| --- | --- | --- |
| `RELEASE_WATCHER_STATE_DIR` | `$XDG_DATA_HOME/release-watcher` | state directory |
| `RELEASE_WATCHER_DB` | `<state>/release-watcher.db` | SQLite path |
| `RELEASE_WATCHER_DEFAULT_INTERVAL` | `24h` | default watch interval |
| `RELEASE_WATCHER_TIMEZONE` | system local | IANA timezone used for day-based scheduling and DST, for example `Europe/Berlin` |
| `RELEASE_WATCHER_HTTP_TIMEOUT` | `30s` | per-request timeout |
| `RELEASE_WATCHER_RATE_INTERVAL` | `1s` | minimum delay between requests to one host |
| `RELEASE_WATCHER_MAX_RETRIES` | `3` | retries for timeouts and 429/5xx |
| `RELEASE_WATCHER_MAX_BODY_BYTES` | `4194304` | maximum response body (64 KiB–16 MiB) |
| `RELEASE_WATCHER_LANG` | `ru` | human/notification language (`ru`/`en`) |
| `RELEASE_WATCHER_LOG_FORMAT` | `text` | `text` or `json` |
| `RELEASE_WATCHER_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `RELEASE_WATCHER_TELEGRAM_BOT_TOKEN` | unset | optional Telegram bot token |
| `RELEASE_WATCHER_TELEGRAM_CHAT_ID` | unset | optional Telegram chat id |
| `RELEASE_WATCHER_ALLOW_PRIVATE_HOSTS` | `false` | test/dev only; weakens SSRF protection |
| `RELEASE_WATCHER_NARO_API_BASE` | unset | test/dev only; override the Narō API base for local E2E |
| `RELEASE_WATCHER_NARO_CATALOG_BASE` | unset | test/dev only; override the Narō catalog base for local E2E |

Set `RELEASE_WATCHER_TIMEZONE` to the same timezone as the systemd timer (normally system local) so the daily calendar window matches the timer.

Secrets are never logged and never stored in SQLite.

## Backup and restore

The binary provides a single-file, readable backup archive containing a manifest (format version, schema version, app version, size, SHA-256) and a consistent `VACUUM INTO` SQLite copy.

```sh
release-watcher backup --out /var/backups/release-watcher.rwbackup
# before upgrading or restoring, stop the timer and other commands
systemctl --user stop release-watcher.timer
release-watcher restore --in /var/backups/release-watcher.rwbackup --force
systemctl --user start release-watcher.timer
```

Restore acquires the same process lock as scans/notify, validates the archive, size and hash, inspects the schema read-only, and refuses a schema newer than the binary. Replacement is staged in the database directory and the previous database is kept aside until the new file is in place. `--force` is required when a database already exists. Run a backup before every upgrade.

## Doctor

`release-watcher doctor --json` is read-only. It checks configuration, state/database permissions, the SQLite schema version against the binary's known migrations, optional Telegram configuration, SSRF settings, build identity and the installed user timer cadence against the shortest enabled watch interval. It does not create or migrate the database. On a closed database it opens with `immutable=1`, so it does not create `-wal`/`-shm` sidecars; if a WAL exists it inspects a temporary copy, leaving the live files untouched. Stop writers when an authoritative schema/permission check is required.

## Local verification evidence

The installation task verified:

- `scripts/build.sh --all` produced linux/amd64 and linux/arm64 binaries; `file` reports a statically linked ARM64 ELF.
- The amd64 binary reports honest build identity with `version --json` and doctor parses configuration without a database.
- `systemd-analyze verify` accepted the service and timer with a temporary executable path, including the rendered `EnvironmentFile` without quoting warnings.
- The install script and build script pass `bash -n`.
- Backup/restore and doctor are covered by tests at application and CLI levels.

This is local evidence, not a claim that a production host was deployed or that ARM64 was executed on hardware.

## Upgrade and rollback

1. Run `release-watcher backup --out <file>`.
2. Stop the timer: `systemctl --user stop release-watcher.timer`.
3. Install the new binary over the old path.
4. Run `release-watcher doctor` and `release-watcher scan --due`.
5. If migrations or behavior fail, restore the backup with the previous binary, then re-enable the timer.

A database whose schema is newer than the binary is reported by doctor and by the store's migration guard; it is not downgraded in place.

## Whoosh Core assessment (documented, not executed)

Whoosh Core is a single-binary SSH deployment tool. From its README it builds timestamped releases from Git, links shared files/directories, atomically swaps a `current` symlink, keeps a bounded number of releases, supports rollback, and runs per-stage tasks/hooks. It also supports a local execution mode and per-stage host inventories.

Mapping to Release Watcher:

| Requirement | Whoosh capability | Spike question |
| --- | --- | --- |
| Prebuilt Go binary over SSH | Git-based release build; local mode; task commands | Is there a first-class prebuilt-artifact upload, or must a task copy the binary into a release? Verify before relying on it. |
| Linux x86-64 / ARM64 | Whoosh is a Go binary and is arch-independent for transport | Build and deploy both artifacts; execute only on matching hosts. |
| Versioned releases | Timestamped release directories, `keep_releases` | Confirm retention leaves enough rollback targets. |
| Atomic `current` | Symlink swap | Confirm the service uses the `current` path and restarts after the swap. |
| systemd timer/service | Task/hook commands | Non-login SSH may lack a user system bus; verify `systemctl --user` on the target or use a system unit. |
| SQLite outside releases | `linked_dirs` from `shared/` | Point `RELEASE_WATCHER_STATE_DIR` at a shared data directory; never at a release directory. |
| Backup before migration | `before` hook/task | Run `release-watcher backup` and verify the artifact before the new binary starts. |
| Smoke after install | `after` hook/task | Run `doctor`, `version`, and a `scan --due`; fail the deploy on a non-zero result. |
| Rollback | `deploy:rollback` | Verify old release + restored/pre-upgrade SQLite backup. |

The spike is a separate backlog task. It must record the exact Whoosh version/commit, a test host, a prebuilt-artifact path, the SQLite shared path, timer delivery, a successful repeat deploy and a rollback, plus ARM64 evidence. Until then this table is an assessment, not a verified deployment. Whoosh is not a runtime dependency and no third deployment-framework repository is created.

## Security

- Run as an unprivileged user.
- Keep the state directory `0700` and the database `0600`.
- Do not expose a network port; there is none in the MVP.
- Do not put tokens in command lines, URLs, logs, Git or test fixtures.
- The SSRF guard blocks private targets by default. Do not enable `RELEASE_WATCHER_ALLOW_PRIVATE_HOSTS` in production.
