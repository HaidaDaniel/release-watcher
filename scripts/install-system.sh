#!/usr/bin/env bash
# Install a prebuilt Release Watcher artifact on a systemd host. Run as root on
# the target. This script does not start or enable the timer; smoke first.
set -Eeuo pipefail
umask 022

usage() {
  cat >&2 <<'USAGE'
usage: install-system.sh --artifact PATH --manifest PATH [--units-dir DIR] [--keep 5]

Installs only Release Watcher paths and units. Paths and the runtime user are
fixed to match deploy/systemd/system. It does not enable the timer.
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
    --keep) keep=${2:?}; shift 2 ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done

[[ $(id -u) -eq 0 ]] || { echo 'install-system: must run as root' >&2; exit 1; }
[[ -f $artifact && -f $manifest ]] || { echo 'install-system: artifact and manifest are required' >&2; exit 1; }
[[ $keep =~ ^[0-9]+$ && $keep -ge 2 ]] || { echo 'install-system: --keep must be at least 2' >&2; exit 1; }
if [[ -z $units_dir ]]; then
  units_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/../deploy/systemd/system" && pwd)
fi
[[ -f $units_dir/release-watcher.service && -f $units_dir/release-watcher.timer && -f $units_dir/release-watcher-mcp.service ]] || {
  echo 'install-system: systemd units are missing' >&2
  exit 1
}
command -v python3 >/dev/null || { echo 'install-system: python3 is required for manifest validation' >&2; exit 1; }
command -v systemd-analyze >/dev/null || { echo 'install-system: systemd-analyze is required' >&2; exit 1; }

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

if [[ -e $root/current && ! -L $root/current ]]; then
  echo "install-system: $root/current exists but is not a symlink" >&2
  exit 1
fi

if ! getent group "$runtime_user" >/dev/null; then
  groupadd --system "$runtime_user"
fi
if ! id -u "$runtime_user" >/dev/null 2>&1; then
  useradd --system --gid "$runtime_user" --home-dir "$data_root" --shell /usr/sbin/nologin "$runtime_user"
fi

install -d -m 0755 -o root -g root "$root" "$root/releases" "$root/shared"
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

staging=$(mktemp -d "$root/.staging.XXXXXX")
backup_dir=$(mktemp -d)
previous_current=""
previous_service=0
previous_timer=0
previous_mcp=0
units_changed=0
current_changed=0
release_dir="$root/releases/$commit"

cleanup() {
  local rc=$?
  set +e
  if [[ $rc -ne 0 ]]; then
    if [[ $units_changed -eq 1 ]]; then
      if [[ $previous_service -eq 1 ]]; then
        cp -a "$backup_dir/release-watcher.service" /etc/systemd/system/release-watcher.service
      else
        rm -f /etc/systemd/system/release-watcher.service
      fi
      if [[ $previous_timer -eq 1 ]]; then
        cp -a "$backup_dir/release-watcher.timer" /etc/systemd/system/release-watcher.timer
      else
        rm -f /etc/systemd/system/release-watcher.timer
      fi
      if [[ $previous_mcp -eq 1 ]]; then
        cp -a "$backup_dir/release-watcher-mcp.service" /etc/systemd/system/release-watcher-mcp.service
      else
        rm -f /etc/systemd/system/release-watcher-mcp.service
      fi
      systemctl daemon-reload >/dev/null 2>&1 || true
    fi
    if [[ $current_changed -eq 1 ]]; then
      if [[ -n $previous_current ]]; then
        tmp_link="$root/.current.rollback.$$"
        ln -sfn "$previous_current" "$tmp_link" && mv -Tf "$tmp_link" "$root/current"
      else
        rm -f "$root/current"
      fi
    fi
    echo "install-system: failed; restored previous current/units" >&2
  fi
  rm -rf -- "$staging" "$backup_dir"
  exit "$rc"
}
trap cleanup EXIT

install -m 0755 -o root -g root "$artifact" "$staging/release-watcher"
install -m 0644 -o root -g root "$manifest" "$staging/manifest.json"

live=$("$staging/release-watcher" version --json | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["commit"])')
[[ $live == "$commit" ]] || { echo "install-system: binary commit $live != $commit" >&2; exit 1; }

verify_units="$staging/units"
install -d -m 0755 "$verify_units"
sed "s|ExecStart=/opt/release-watcher/current/release-watcher|ExecStart=$staging/release-watcher|" \
  "$units_dir/release-watcher.service" > "$verify_units/release-watcher.service"
cp "$units_dir/release-watcher.timer" "$verify_units/release-watcher.timer"
sed "s|ExecStart=/opt/release-watcher/current/release-watcher|ExecStart=$staging/release-watcher|" \
  "$units_dir/release-watcher-mcp.service" > "$verify_units/release-watcher-mcp.service"
systemd-analyze verify "$verify_units/release-watcher.service" "$verify_units/release-watcher.timer" "$verify_units/release-watcher-mcp.service"

install -d -m 0755 -o root -g root "$release_dir"
install -m 0755 -o root -g root "$staging/release-watcher" "$release_dir/release-watcher"
install -m 0644 -o root -g root "$staging/manifest.json" "$release_dir/manifest.json"

if [[ -L $root/current ]]; then
  previous_current=$(readlink -f "$root/current" 2>/dev/null || readlink "$root/current")
fi
if [[ -f /etc/systemd/system/release-watcher.service ]]; then
  cp -a /etc/systemd/system/release-watcher.service "$backup_dir/release-watcher.service"
  previous_service=1
fi
if [[ -f /etc/systemd/system/release-watcher.timer ]]; then
  cp -a /etc/systemd/system/release-watcher.timer "$backup_dir/release-watcher.timer"
  previous_timer=1
fi
if [[ -f /etc/systemd/system/release-watcher-mcp.service ]]; then
  cp -a /etc/systemd/system/release-watcher-mcp.service "$backup_dir/release-watcher-mcp.service"
  previous_mcp=1
fi

tmp_link="$root/.current.$$"
ln -sfn "$release_dir" "$tmp_link"
mv -Tf "$tmp_link" "$root/current"
current_changed=1

units_changed=1
install -m 0644 -o root -g root "$units_dir/release-watcher.service" /etc/systemd/system/release-watcher.service
install -m 0644 -o root -g root "$units_dir/release-watcher.timer" /etc/systemd/system/release-watcher.timer
install -m 0644 -o root -g root "$units_dir/release-watcher-mcp.service" /etc/systemd/system/release-watcher-mcp.service
systemctl daemon-reload
systemd-analyze verify /etc/systemd/system/release-watcher.service /etc/systemd/system/release-watcher.timer /etc/systemd/system/release-watcher-mcp.service

# Keep the newest releases, but never remove the current target. Pruning is
# best-effort: a pruning failure must not roll back a successful install.
if ! (
  set +e
  current_real=$(readlink -f "$root/current")
  mapfile -d '' -t releases < <(find "$root/releases" -mindepth 1 -maxdepth 1 -type d -printf '%T@ %p\0' | sort -z -rn | cut -z -d' ' -f2-)
  kept=0
  for release in "${releases[@]}"; do
    if [[ $kept -lt $keep ]]; then
      kept=$((kept + 1))
      continue
    fi
    [[ $release == "$current_real" ]] && continue
    [[ $release == "$root/releases/"* ]] || continue
    rm -rf -- "$release" || exit 1
  done
); then
  echo 'install-system: warning: release pruning failed; current release is intact' >&2
fi

echo "install-system: installed $commit ($version) at $release_dir"
echo "install-system: current -> $root/current"
echo "install-system: timer and MCP service are NOT enabled"
echo "Next: systemctl start release-watcher.service   # smoke"
echo "Then: systemctl enable --now release-watcher.timer"
echo "MCP:  sudo scripts/install-mcp-env.sh [--write-enabled], then start release-watcher-mcp.service"
