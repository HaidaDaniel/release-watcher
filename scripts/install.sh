#!/usr/bin/env bash
set -Eeuo pipefail
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

prefix=${PREFIX:-$HOME/.local}
config_home=${XDG_CONFIG_HOME:-$HOME/.config}
enable=false
binary=""
for arg in "$@"; do
  case "$arg" in
    --enable) enable=true ;;
    -h|--help)
      echo "usage: scripts/install.sh [--enable] [path-to-linux-binary]" >&2
      exit 0
      ;;
    *) binary=$arg ;;
  esac
done

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "install: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac
if [[ -z "$binary" ]]; then
  binary="$repo/bin/release-watcher-linux-$arch"
fi
if [[ ! -x "$binary" ]]; then
  echo "install: binary not found or not executable: $binary" >&2
  echo "install: run scripts/build.sh --all first" >&2
  exit 1
fi

unit_dir="$config_home/systemd/user"
env_dir="$config_home/release-watcher"
data_home=${XDG_DATA_HOME:-$HOME/.local/share}
state_dir=${RELEASE_WATCHER_STATE_DIR:-$data_home/release-watcher}
escape_for_unit() { printf '%s' "$1" | sed -e 's/%/%%/g'; }
escape_sed() { printf '%s' "$1" | sed -e 's/[\\&|]/\\&/g'; }
prefix_esc=$(escape_sed "$(escape_for_unit "$prefix")")
env_esc=$(escape_sed "$(escape_for_unit "$env_dir")")
state_esc=$(escape_sed "$(escape_for_unit "$state_dir")")
install -Dm0755 "$binary" "$prefix/bin/release-watcher"
mkdir -p "$unit_dir"
sed \
  -e "s|%h/.local/bin/release-watcher|$prefix_esc/bin/release-watcher|g" \
  -e "s|%h/.config/release-watcher/env|$env_esc/env|g" \
  -e "s|%h/.local/share/release-watcher|$state_esc|g" \
  "$repo/deploy/systemd/release-watcher.service" > "$unit_dir/release-watcher.service"
chmod 0644 "$unit_dir/release-watcher.service"
install -Dm0644 "$repo/deploy/systemd/release-watcher.timer" "$unit_dir/release-watcher.timer"
mkdir -p "$env_dir"
chmod 700 "$env_dir"
if [[ ! -e "$env_dir/env" ]]; then
  cat > "$env_dir/env" <<'ENVEOF'
# Release Watcher environment. Keep this file private.
# Optional Telegram:
# RELEASE_WATCHER_TELEGRAM_BOT_TOKEN=
# RELEASE_WATCHER_TELEGRAM_CHAT_ID=
# RELEASE_WATCHER_LANG=ru
# RELEASE_WATCHER_LOG_FORMAT=json
ENVEOF
  chmod 600 "$env_dir/env"
fi

if command -v systemctl >/dev/null 2>&1; then
  systemctl --user daemon-reload || true
  if $enable; then
    systemctl --user enable --now release-watcher.timer
  fi
fi

echo "Installed: $prefix/bin/release-watcher"
echo "Units:     $unit_dir/release-watcher.service, $unit_dir/release-watcher.timer"
echo "State:     $state_dir"
if ! $enable; then
  echo "Next:      systemctl --user enable --now release-watcher.timer"
fi
echo "Check:     release-watcher doctor --json"
