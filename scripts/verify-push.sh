#!/usr/bin/env bash
set -Eeuo pipefail
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo"

usage() {
  echo "usage: verify-push.sh [--ci] [--race]" >&2
  exit 2
}

ci=false
race=false
for arg in "$@"; do
  case "$arg" in
    --ci) ci=true ;;
    --race) race=true ;;
    *) usage ;;
  esac
done

scripts/verify-fast.sh

echo "verify-push: go test ./..."
go test ./...

echo "verify-push: go vet ./..."
go vet ./...

echo "verify-push: go build ./..."
go build ./...

if $race || [[ "${RELEASE_WATCHER_RACE:-}" == "1" ]]; then
  echo "verify-push: go test -race ./..."
  go test -race ./...
fi

if command -v pnpm >/dev/null 2>&1; then
  echo "verify-push: apk lint"
  pnpm exec apkit lint
else
  echo "verify-push: pnpm not found; skipping APK lint" >&2
  if $ci; then
    echo "verify-push: --ci requires pnpm for APK lint" >&2
    exit 1
  fi
fi

echo "verify-push: pass"
