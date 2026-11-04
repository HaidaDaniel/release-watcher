#!/usr/bin/env bash
set -Eeuo pipefail
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo"

usage() {
  echo "usage: verify-push.sh [--race]" >&2
  exit 2
}

race=false
for arg in "$@"; do
  case "$arg" in
    --race) race=true ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done

if ! command -v go >/dev/null 2>&1; then
  echo "verify-push: go is required" >&2
  exit 1
fi
if ! command -v pnpm >/dev/null 2>&1; then
  echo "verify-push: pnpm is required for APK lint; install pnpm and run pnpm install" >&2
  exit 1
fi

echo "verify-push: gofmt"
if git rev-parse --git-dir >/dev/null 2>&1; then
  mapfile -t go_files < <(git ls-files -- '*.go')
else
  mapfile -t go_files < <(find . -name '*.go' -not -path './node_modules/*')
fi
if [[ ${#go_files[@]} -gt 0 ]]; then
  unformatted=$(gofmt -l "${go_files[@]}" || true)
  if [[ -n "$unformatted" ]]; then
    echo "verify-push: gofmt required for:" >&2
    printf '%s\n' "$unformatted" >&2
    exit 1
  fi
fi

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

echo "verify-push: apk lint"
pnpm exec apkit lint

echo "verify-push: pass"
