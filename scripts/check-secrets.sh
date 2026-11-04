#!/usr/bin/env bash
set -Eeuo pipefail
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo"

if ! git rev-parse --git-dir >/dev/null 2>&1; then
  echo "check-secrets: not a git repository; skipping" >&2
  exit 0
fi

if git diff --cached --quiet -- . 2>/dev/null; then
  diff_args=(--no-ext-diff --unified=0)
  target="working tree"
else
  diff_args=(--cached --no-ext-diff --unified=0)
  target="staged changes"
fi

added=$(git diff "${diff_args[@]}" -- . | grep -E '^\+' | grep -Ev '^\+\+\+' || true)
if [[ -z "$added" ]]; then
  exit 0
fi

patterns=(
  'BEGIN (RSA|OPENSSH|EC|DSA|PGP) PRIVATE KEY'
  'AKIA[0-9A-Z]{16}'
  'ASIA[0-9A-Z]{16}'
  'gh[pousr]_[A-Za-z0-9]{36,}'
  'xox[baprs]-[A-Za-z0-9-]{10,}'
  '[0-9]{8,10}:[A-Za-z0-9_-]{35}'
)

found=0
for pattern in "${patterns[@]}"; do
  if matches=$(printf '%s\n' "$added" | grep -En "$pattern" || true); [[ -n "$matches" ]]; then
    echo "check-secrets: possible secret in $target:" >&2
    printf '%s\n' "$matches" >&2
    found=1
  fi
done

if [[ "$found" -ne 0 ]]; then
  echo "check-secrets: remove or replace the secret; never commit real credentials." >&2
  exit 1
fi
