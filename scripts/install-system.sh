#!/usr/bin/env bash
# Install a prebuilt Release Watcher artifact on a systemd host. Run as root on
# the target. This script does not start or enable the timer; smoke first.
set -Eeuo pipefail
umask 022

usage() {
  cat >&2 <<'USAGE'
usage: install-system.sh --artifact PATH --manifest PATH [--units-dir DIR]
                         [--root /opt/release-watcher]
                         [--data-root /var/lib/release-watcher]
                         [--backup-root /var/backups/release-watcher]
                         [--user release-watcher] [--keep 5]

Installs only Release Watcher paths and units. It does not enable the timer.
USAGE
  exit 2
}

artifact=""
manifest=""
units_dir=""
root=/opt/release-watcher
data_root=/var/lib/release-watcher
backup_root=/var/backups/release-watcher
runtime_user=release-watcher
keep=5

while [[ $# -gt 0 ]]; do
  case "$1" in
    --artifact) artifact=${2:?}; shift 2 ;;
    --manifest) manifest=${2:?}; shift 2 ;;
    --units-dir) units_dir=${2:?}; shift 2 ;;
    --root) root=${2:?}; shift 2 ;;
    --data-root) data_root=${2:?}; shift 2 ;;
    --backup-root) backup_root=${2:?}; shift 2 ;;
    --user) runtime_user=${2:?}; shift 2 ;;
    --keep) keep=${2:?}; shift 2 ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done

[[ $(id -u) -eq 0 ]] || { echo 'install-system: must run as root' >&2; exit 1; }
[[ -f $artifact && -f $manifest ]] || { echo 'install-system: artifact and manifest are required' >&2; exit 1; }
[[ $root == /* && $root != *..* && $root != *//* ]] || { echo 'install-system: invalid root' >&2; exit 1; }
[[ $data_root == /* && $data_root != *..* && $data_root != *//* ]] || { echo 'install-system: invalid data root' >&2; exit 1; }
[[ $backup_root == /* && $backup_root != *..* && $backup_root != *//* ]] || { echo 'install-system: invalid backup root' >&2; exit 1; }
[[ $runtime_user =~ ^[a-z_][a-z0-9_-]*$ ]] || { echo 'install-system: invalid user' >&2; exit 1; }
[[ $keep =~ ^[0-9]+$ && $keep -ge 2 ]] || { echo 'install-system: --keep must be at least 2' >&2; exit 1; }
if [[ -z $units_dir ]]; then
  units_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/../deploy/systemd/system" && pwd)
fi
[[ -f $units_dir/release-watcher.service && -f $units_dir/release-watcher.timer ]] || {
  echo 'install-system: systemd units are missing' >&2
  exit 1
}
command -v python3 >/dev/null || { echo 'install-system: python3 is required for manifest validation' >&2; exit 1; }

read -r commit version goarch <<<"$(python3 - "$manifest" "$artifact" <<'PY'
import hashlib, json, os, sys
manifest, artifact = sys.argv[1:]
with open(manifest, encoding="utf-8") as handle:
    data = json.load(handle)
if data.get("format") != "release-watcher-artifact" or data.get("format_version") != 1:
    raise SystemExit("install-system: unsupported manifest")
for key in ("commit", "version", "goarch", "built_at", "sha256", "bytes"):
    if not data.get(key):
        raise SystemExit(f"install-system: manifest missing {key}")
commit = data["commit"]
if len(commit) != 40 or any(c not in "0123456789abcdef" for c in commit):
    raise SystemExit("install-system: invalid commit")
if data.get("goos") != "linux":
    raise SystemExit("install-system: artifact is not linux")
if data["goarch"] not in ("amd64", "arm64"):
    raise SystemExit("install-system: unsupported arch")
with open(artifact, "rb") as handle:
    digest = hashlib.sha256(handle.read()).hexdigest()
if digest != data["sha256"]:
    raise SystemExit("install-system: checksum mismatch")
if os.path.getsize(artifact) != int(data["bytes"]):
    raise SystemExit("install-system: size mismatch")
print(commit, data["version"], data["goarch"])
PY
)"
machine=$(uname -m)
case "$machine" in x86_64) machine=amd64 ;; aarch64|arm64) machine=arm64 ;; esac
[[ $machine == "$goarch" ]] || { echo "install-system: artifact is $goarch, host is $machine" >&2; exit 1; }

if ! getent group "$runtime_user" >/dev/null; then
  groupadd --system "$runtime_user"
fi
if ! id -u "$runtime_user" >/dev/null 2>&1; then
  useradd --system --gid "$runtime_user" --home-dir "$data_root" --shell /usr/sbin/nologin "$runtime_user"
fi

release_dir="$root/releases/$commit"
install -d -m 0755 -o root -g root "$root" "$root/releases" "$root/shared"
install -d -m 0755 -o root -g root "$release_dir"
install -d -m 0700 -o "$runtime_user" -g "$runtime_user" "$data_root" "$backup_root"
install -d -m 0755 -o root -g root /etc/release-watcher
if [[ ! -e /etc/release-watcher/environment ]]; then
  cat >/etc/release-watcher/environment <<'ENV'
# Release Watcher environment. Keep private. Do not commit secrets.
# Optional Telegram:
# RELEASE_WATCHER_TELEGRAM_BOT_TOKEN=
# RELEASE_WATCHER_TELEGRAM_CHAT_ID=
# RELEASE_WATCHER_LANG=ru
# RELEASE_WATCHER_TIMEZONE=Europe/Berlin
ENV
  chown "$runtime_user:$runtime_user" /etc/release-watcher/environment
  chmod 0600 /etc/release-watcher/environment
fi

install -m 0755 -o root -g root "$artifact" "$release_dir/release-watcher"
install -m 0644 -o root -g root "$manifest" "$release_dir/manifest.json"
tmp_link="$root/.current.$$"
ln -sfn "$release_dir" "$tmp_link"
mv -Tf "$tmp_link" "$root/current"

install -m 0644 -o root -g root "$units_dir/release-watcher.service" /etc/systemd/system/release-watcher.service
install -m 0644 -o root -g root "$units_dir/release-watcher.timer" /etc/systemd/system/release-watcher.timer
systemctl daemon-reload
systemd-analyze verify /etc/systemd/system/release-watcher.service /etc/systemd/system/release-watcher.timer

live=$("$root/current/release-watcher" version --json | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["commit"])')
[[ $live == "$commit" ]] || { echo "install-system: live commit $live != $commit" >&2; exit 1; }

# Keep the newest releases, but never remove the current target.
current_real=$(readlink -f "$root/current")
mapfile -t releases < <(find "$root/releases" -mindepth 1 -maxdepth 1 -type d -printf '%T@ %p\n' | sort -rn | awk '{print $2}')
kept=0
for release in "${releases[@]}"; do
  if [[ $kept -lt $keep ]]; then
    kept=$((kept + 1))
    continue
  fi
  [[ $release == "$current_real" ]] && continue
  [[ $release == "$root/releases/"* ]] || continue
  rm -rf -- "$release"
done

echo "install-system: installed $commit ($version) at $release_dir"
echo "install-system: current -> $root/current"
echo "install-system: timer is NOT enabled"
echo "Next: systemctl start release-watcher.service   # smoke"
echo "Then: systemctl enable --now release-watcher.timer"
