#!/usr/bin/env bash
# Switch the Release Watcher current symlink to an already installed release.
# Run as root on the target. Database compatibility remains the operator's
# responsibility; restore a pre-upgrade backup when the schema changed.
set -Eeuo pipefail
umask 022

usage() {
  echo "usage: rollback-system.sh --commit SHA [--root /opt/release-watcher]" >&2
  exit 2
}

commit=""
root=/opt/release-watcher
while [[ $# -gt 0 ]]; do
  case "$1" in
    --commit) commit=${2:?}; shift 2 ;;
    --root) root=${2:?}; shift 2 ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done
[[ $(id -u) -eq 0 ]] || { echo 'rollback-system: must run as root' >&2; exit 1; }
[[ $commit =~ ^[0-9a-f]{40}$ ]] || { echo 'rollback-system: invalid commit' >&2; exit 1; }
[[ $root == /* && $root != *..* && $root != *//* ]] || { echo 'rollback-system: invalid root' >&2; exit 1; }
release="$root/releases/$commit"
[[ -x $release/release-watcher ]] || { echo "rollback-system: release not found: $release" >&2; exit 1; }
live=$("$release/release-watcher" version --json | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["commit"])')
[[ $live == "$commit" ]] || { echo "rollback-system: binary commit $live != $commit" >&2; exit 1; }
tmp_link="$root/.current.$$"
ln -sfn "$release" "$tmp_link"
mv -Tf "$tmp_link" "$root/current"
echo "rollback-system: current -> $release"
echo "rollback-system: binary commit verified; restart the service if it is running"
echo "rollback-system: if the database schema is newer, restore a compatible pre-upgrade backup before starting"
