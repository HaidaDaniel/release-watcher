#!/usr/bin/env bash
# Prune old .rwbackup files without deleting the explicitly verified backup.
# Dry-run by default; pass --apply to delete.
set -Eeuo pipefail
usage() {
  echo "usage: prune-backups.sh --dir DIR [--keep 7] [--verified FILE] [--apply]" >&2
  exit 2
}
dir=""
keep=7
verified=""
apply=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir) dir=${2:?}; shift 2 ;;
    --keep) keep=${2:?}; shift 2 ;;
    --verified) verified=${2:?}; shift 2 ;;
    --apply) apply=true; shift ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done
[[ -n $dir && $dir == /* && $dir != *..* && $dir != *//* ]] || { echo 'prune-backups: invalid --dir' >&2; exit 1; }
[[ -d $dir ]] || { echo 'prune-backups: directory not found' >&2; exit 1; }
[[ $keep =~ ^[0-9]+$ && $keep -ge 2 ]] || { echo 'prune-backups: --keep must be at least 2' >&2; exit 1; }
if [[ -n $verified ]]; then
  [[ -f $verified ]] || { echo 'prune-backups: verified backup not found' >&2; exit 1; }
fi

mapfile -t files < <(find "$dir" -maxdepth 1 -type f -name '*.rwbackup' -printf '%T@ %p\n' | sort -rn | awk '{print $2}')
kept=0
for file in "${files[@]}"; do
  if [[ $kept -lt $keep ]]; then
    kept=$((kept + 1))
    continue
  fi
  if [[ -n $verified && $file == "$verified" ]]; then
    echo "keep verified: $file"
    continue
  fi
  if $apply; then
    rm -f -- "$file"
    echo "removed: $file"
  else
    echo "would remove: $file"
  fi
done
$apply || echo "dry-run: pass --apply to delete"
