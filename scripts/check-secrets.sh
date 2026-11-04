#!/usr/bin/env bash
# Guard against accidentally staging credentials. By default this scans only
# staged additions; pass --working to also scan the working diff and untracked
# files. It is not a full Git-history audit and must not be treated as one.
#
# Output deliberately contains only file, line and secret type. Never print the
# matched value, a token, a private key or a matching source line.
set -Eeuo pipefail
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo"

usage() {
  echo "usage: check-secrets.sh [--working]" >&2
  exit 2
}

scan_working=false
for arg in "$@"; do
  case "$arg" in
    --working) scan_working=true ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done

if ! git rev-parse --git-dir >/dev/null 2>&1; then
  echo "check-secrets: not a git repository; skipping" >&2
  exit 0
fi

patterns=(
  'private-key|BEGIN (RSA|OPENSSH|EC|DSA|PGP) PRIVATE KEY'
  'aws-access-key|AKIA[0-9A-Z]{16}'
  'aws-session-key|ASIA[0-9A-Z]{16}'
  'github-token|gh[pousr]_[A-Za-z0-9]{36,}'
  'slack-token|xox[baprs]-[A-Za-z0-9-]{10,}'
  'telegram-bot-token|[0-9]{8,10}:[A-Za-z0-9_-]{35}'
  'generic-credential|(token|secret|password|passwd|api[_-]?key)[[:space:]]*[:=][[:space:]]*["'\'']?[A-Za-z0-9_./+=-]{20,}'
)

found=0

check_line() {
  local file=$1 line=$2 content=$3 entry type regex
  for entry in "${patterns[@]}"; do
    type=${entry%%|*}
    regex=${entry#*|}
    if [[ $content =~ $regex ]]; then
      printf 'check-secrets: possible %s at %s:%s\n' "$type" "$file" "$line" >&2
      found=1
      return 0
    fi
  done
}

scan_diff() {
  local file="" lineno=0 line plus content
  while IFS= read -r line; do
    case "$line" in
      "+++ b/"*)
        file=${line#+++ b/}
        lineno=0
        ;;
      "+++ /dev/null")
        file=""
        ;;
      "@@ "*)
        plus=${line#* +}
        plus=${plus%% *}
        lineno=${plus%%,*}
        ;;
      "+"*)
        [[ $line == "+++"* ]] && continue
        [[ -z $file ]] && continue
        content=${line#+}
        check_line "$file" "$lineno" "$content"
        lineno=$((lineno + 1))
        ;;
    esac
  done
}

scan_diff < <(git diff --cached --no-ext-diff --unified=0 -- .)

if $scan_working; then
  scan_diff < <(git diff --no-ext-diff --unified=0 -- .)
  while IFS= read -r -d '' file; do
    [[ -f $file ]] || continue
    lineno=0
    while IFS= read -r content || [[ -n $content ]]; do
      lineno=$((lineno + 1))
      check_line "$file" "$lineno" "$content"
    done <"$file"
  done < <(git ls-files --others --exclude-standard -z)
fi

if [[ "$found" -ne 0 ]]; then
  echo "check-secrets: remove or replace the credential; never commit real secrets." >&2
  echo "check-secrets: this scanner covers staged additions only, not Git history." >&2
  exit 1
fi
