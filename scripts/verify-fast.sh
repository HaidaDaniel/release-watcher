#!/usr/bin/env bash
set -Eeuo pipefail
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo"

if git rev-parse --git-dir >/dev/null 2>&1; then
  mapfile -t go_files < <(
    {
      git diff --cached --name-only --diff-filter=ACM
      git diff --name-only --diff-filter=ACM
      git ls-files --others --exclude-standard
    } | sort -u | grep -E '\.go$' || true
  )
  if [[ ${#go_files[@]} -gt 0 ]]; then
    unformatted=$(gofmt -l "${go_files[@]}" || true)
    if [[ -n "$unformatted" ]]; then
      echo "verify-fast: gofmt required for:" >&2
      printf '%s\n' "$unformatted" >&2
      exit 1
    fi
  fi

  if ! git diff --cached --check >/dev/null 2>&1; then
    echo "verify-fast: staged diff has whitespace errors" >&2
    git diff --cached --check >&2 || true
    exit 1
  fi
  if ! git diff --check >/dev/null 2>&1; then
    echo "verify-fast: working diff has whitespace errors" >&2
    git diff --check >&2 || true
    exit 1
  fi
fi

scripts/check-secrets.sh
go build ./...
echo "verify-fast: pass"
