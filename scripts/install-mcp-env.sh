#!/usr/bin/env bash
# Create or rotate /etc/release-watcher/mcp.env with a fresh 256-bit token.
# Run as root on the target. The token is never printed.
set -Eeuo pipefail
umask 077
usage() {
  echo "usage: install-mcp-env.sh [--write-enabled] [--force] [--listen ADDR]
                           [--allowed-hosts CSV] [--allowed-origins CSV]" >&2
  exit 2
}
write_enabled=false
force=false
listen=127.0.0.1:8741
allowed_hosts=""
allowed_origins=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --write-enabled) write_enabled=true; shift ;;
    --force) force=true; shift ;;
    --listen) listen=${2:?}; shift 2 ;;
    --allowed-hosts) allowed_hosts=${2:?}; shift 2 ;;
    --allowed-origins) allowed_origins=${2:?}; shift 2 ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done
[[ $(id -u) -eq 0 ]] || { echo 'install-mcp-env: must run as root' >&2; exit 1; }
[[ $listen =~ ^[a-zA-Z0-9_.:\[\]-]+$ ]] || { echo 'install-mcp-env: invalid listen address' >&2; exit 1; }
command -v openssl >/dev/null || { echo 'install-mcp-env: openssl is required' >&2; exit 1; }
install -d -m 0755 -o root -g root /etc/release-watcher
if [[ -e /etc/release-watcher/mcp.env && $force != true ]]; then
  echo 'install-mcp-env: mcp.env already exists; use --force to rotate the token' >&2
  exit 1
fi
token=$(openssl rand -hex 32)
tmp=$(mktemp /etc/release-watcher/.mcp.env.XXXXXX)
trap 'rm -f "$tmp"' EXIT
cat >"$tmp" <<ENV
# Release Watcher MCP secret. Root-owned, read by the systemd manager.
RELEASE_WATCHER_MCP_TOKEN=$token
RELEASE_WATCHER_MCP_LISTEN=$listen
RELEASE_WATCHER_MCP_WRITE_ENABLED=$write_enabled
# Add the reverse-proxy hostname when MCP is reached through HTTPS.
RELEASE_WATCHER_MCP_ALLOWED_HOSTS=$allowed_hosts
RELEASE_WATCHER_MCP_ALLOWED_ORIGINS=$allowed_origins
ENV
chown root:root "$tmp"
chmod 0600 "$tmp"
mv -f "$tmp" /etc/release-watcher/mcp.env
trap - EXIT
echo "install-mcp-env: wrote /etc/release-watcher/mcp.env (token not printed)"
echo "install-mcp-env: copy the token to Hermes' protected secret store, then restart release-watcher-mcp.service"
