# Deployment

Status: a system-level Release Watcher deployment has been performed on a home NAS and accepted with cgroup, SQLite, backup/restore and rollback checks. Whoosh Core was not used because its Git-build pipeline does not match the prebuilt-artifact requirement. The Whoosh comparison remains backlog.

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

There is no promise that a 6h interval is honored by a daily timer. The timer frequency is part of the installation contract. `release-watcher doctor --json` reads the installed system timer first (`/etc/systemd/system` and the standard system unit paths, including `release-watcher.timer.d/*.conf` drop-ins), then the user timer. It reports the detected scope and warns when the cadence is slower than the shortest enabled watch interval, or when no timer can be found or parsed. The `RELEASE_WATCHER_SYSTEMD_SYSTEM_DIRS` environment variable is a test/dev override for the system unit search path; normal installs do not need it. Supported calendar forms include aliases (`hourly`, `daily`, `weekly`), daily clock times, hour/minute steps and weekday-prefixed schedules.

### User lingering and reboot recovery

The timer is user-level. Without lingering, systemd starts it only while the user has a session. For a home server that reboots without an interactive login, enable lingering explicitly after reviewing the security implications:

```sh
sudo loginctl enable-linger "$USER"
loginctl show-user "$USER" -p Linger
```

The installer does not enable lingering automatically; it prints the command when lingering is off. With lingering enabled, the daily timer and `Persistent=true` recover missed runs after reboot. Test this in an isolated user environment before relying on it. Use `systemctl --user list-timers release-watcher.timer` and `journalctl --user -u release-watcher.service` to diagnose scheduling.

## System-level NAS layout (prepared, not installed)

The system-level layout is separate from the user-level templates:

```text
/opt/release-watcher/releases/<commit>/release-watcher
/opt/release-watcher/current -> releases/<commit>
/var/lib/release-watcher/release-watcher.db
/var/backups/release-watcher/
/etc/release-watcher/environment
/etc/systemd/system/release-watcher.service
/etc/systemd/system/release-watcher.timer
```

The unit runs as a dedicated `release-watcher` user, with `MemoryHigh=256M`, `MemoryMax=512M`, `MemorySwapMax=128M`, `CPUQuota=40%`, `CPUWeight=50`, `TasksMax=64`, `IOWeight=50`, `Nice=10`, `IOSchedulingClass=idle`, `TimeoutStartSec=15min`, `LimitNOFILE=4096` and the hardening options in `deploy/systemd/system/release-watcher.service`. Adjust limits only after measuring the real oneshot run. An isolated transient user-level smoke on the audited host (no installation, no timer) measured ~11.6 MiB cgroup memory peak and ~0.35 s CPU for a live 210-chapter baseline plus one scan, with zero OOM events; the 512M/256M limits and 64-task limit were effective. This is smoke evidence, not an installation.

Build the artifact on the development machine from a clean commit:

```sh
scripts/package-release.sh --version 0.1.0 --arch amd64
```

This runs `verify-fast`, `verify-push` and `go test -race`, builds with `CGO_ENABLED=0`/`-trimpath`, and writes `manifest.json` with commit, version, GOOS/GOARCH, build time, SHA-256, size and checks. The NAS receives the binary, manifest, systemd units and the small install helper; it never compiles or clones the repository.

On the NAS, as root, install without enabling the timer:

```sh
sudo scripts/install-system.sh --artifact release-watcher-linux-amd64 \
  --manifest manifest.json --units-dir deploy/systemd/system
sudo systemctl start release-watcher.service   # smoke
# after smoke:
sudo systemctl enable --now release-watcher.timer
```

`install-system.sh` validates the manifest/checksum/architecture, stages the binary, verifies its build identity and verifies the units with `systemd-analyze` before switching `current`. It backs up the previous `current` symlink and unit files; any later install failure restores them. It creates only Release Watcher paths and user, installs the units, switches `current` atomically and keeps the newest `--keep` releases (default 5, minimum 2, never the current target). It does not start or enable the timer. `rollback-system.sh --commit <sha>` switches back to an installed release and verifies its build identity; if the database schema changed, restore a compatible pre-upgrade backup first. Backups are not pruned by the installer. Use `scripts/prune-backups.sh --dir /var/backups/release-watcher --keep 7 --verified <last-verified.rwbackup>` (dry-run by default; add `--apply`) so the last verified backup is never deleted.

### Whoosh Core assessment for prebuilt artifacts

Whoosh Core (inspected commit `24d8dc398e2d7baacf13338e44197ba0d471dde3`) is Git-oriented: it maintains a bare mirror and builds a release from Git on the target, and its target requirements include `git` and `tar`. Its docs do not provide a first-class prebuilt-artifact upload that replaces the build phase. That conflicts with the requirement that the NAS receives a dev-built binary and does not clone/build. The bounded fallback is the SSH/SCP release layout above (or an equivalent minimal script), not a custom deployment framework. Whoosh remains an external evaluation, not a runtime dependency. A Whoosh live spike was not attempted because the same privileged installation prerequisite is missing.

## Deployed NAS acceptance (2026-11-05)

- Artifact: `0.1.0`, commit `4d08eff121b691478236a5330e002460b74df1ec`, linux/amd64, SHA-256 `9f17108746380bb57bb3cbd76d104177cbced54923f694418db9de6be0b8f2ca`.
- Layout: `/opt/release-watcher/releases/<commit>` + atomic `current`, state `/var/lib/release-watcher/release-watcher.db`, backups `/var/backups/release-watcher/`.
- Runtime: dedicated unprivileged `release-watcher` user, separate system-level oneshot service and daily timer.
- Effective limits: `MemoryMax=512M`, `MemoryHigh=256M`, `MemorySwapMax=128M`, `CPUQuota=40%`, `CPUWeight=50`, `TasksMax=64`, `IOWeight=50`, `Nice=10`, `IOSchedulingClass=idle`, `LimitNOFILE=4096`. The cgroup reported `memory.max=536870912`, `memory.high=268435456`, `memory.swap.max=134217728`, `pids.max=64`, `io.weight=default 50`.
- Measured service run: peak memory ~9.4 MiB, CPU ~0.16 s, 9 tasks, no OOM/high/max events. A longer isolated smoke measured ~11.6 MiB peak and ~0.63 s CPU.
- SQLite: schema 3, ~288 KiB database; 210-chapter Narō baseline, no false events; repeated `scan --due` stayed `unchanged` with 0 events.
- Backups: three verified `.rwbackup` files (~43 KiB each); restore was exercised on a separate smoke database copy, not on the only live database.
- Rollback: `rollback-system.sh` switched a separate test root from `4d08eff` to `1d89864` and verified the embedded commit.
- Timer: enabled with `OnCalendar=*-*-* 04:15:00`, `Persistent=true`, next run recorded by systemd; the service is oneshot and exits after each scan.
- Telegram: `NOT_CONFIGURED`; no messages were sent.
- The smoke watch was created with a short interval to force a due scan, then changed to the default 24h. `doctor` reports `system timer cadence 24h0m0s covers the shortest enabled watch interval 24h0m0s`.

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

System-level deployment:

```sh
sudo systemctl stop release-watcher.timer
sudo -u release-watcher /opt/release-watcher/current/release-watcher backup \
  --out "/var/backups/release-watcher/release-watcher-$(date -u +%Y%m%dT%H%M%SZ).rwbackup"
sudo -u release-watcher /opt/release-watcher/current/release-watcher restore \
  --in /var/backups/release-watcher/<file>.rwbackup --force
sudo systemctl start release-watcher.timer
```

User-level deployment uses the same commands with `systemctl --user` and the user-level paths. Backup files live in the retained directory; `scripts/prune-backups.sh` must run as root or as the `release-watcher` user and keeps the last verified file even when it is older than the retention window.

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

## NAS resource audit and isolation decision

A read-only audit of the target home NAS was performed before any installation. Generalized results:

- Ubuntu 24.04, systemd 255, cgroup v2 unified, 4 logical CPUs, 7.6 GiB RAM, 19 GiB swap.
- Root filesystem had ~138 GiB free; NAS arrays had >140 GiB free. No memory/CPU/IO pressure or OOM events were observed.
- ResLedger API/scheduler used ~25/13 MiB current with 512/256 MiB MemoryMax; the Telegram memory agent used ~280 MiB current (412 MiB peak) with a 1.5 GiB MemoryMax. All had zero restarts and zero OOM events.
- Recommended Release Watcher limits: `MemoryHigh=256M`, `MemoryMax=512M`, `MemorySwapMax=128M`, `CPUQuota=40%`, `CPUWeight=50`, `TasksMax=64`, `IOWeight=50`, `Nice=10`, `IOSchedulingClass=idle`, `TimeoutStartSec=15min`, `LimitNOFILE=4096`. Adjust only after measuring the real oneshot run.
- User-manager fallback cgroup probe applied MemoryMax/MemoryHigh/CPUQuota/TasksMax, but IOWeight was unavailable and lingering was disabled.

Installation is **blocked** on the audited host: non-interactive sudo is unavailable, so the preferred system-level unit and dedicated runtime user cannot be created; lingering is disabled, so a user-level timer would not survive logout/reboot. No NAS service, timer, firewall, routing, or global cgroup was changed. The exact privileged actions needed are: create a dedicated system user and the `/opt/release-watcher`, `/var/lib/release-watcher`, `/var/backups/release-watcher`, `/etc/release-watcher` paths; install the system units under `/etc/systemd/system`; run `systemctl daemon-reload`; then enable the timer only after smoke.
