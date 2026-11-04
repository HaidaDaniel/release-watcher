#!/usr/bin/env bash
set -Eeuo pipefail
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo"

usage() {
  cat >&2 <<'EOF'
usage: scripts/build.sh [--os linux] [--arch amd64|arm64] [--all]
                        [--out-dir bin] [--version V] [--commit SHA]
                        [--built-at RFC3339] [--mode development|release]
EOF
  exit 2
}

os_name=linux
arch=amd64
all=false
out_dir=bin
version=0.1.0-dev
commit=""
built_at=""
mode=development

while [[ $# -gt 0 ]]; do
  case "$1" in
    --os) os_name=$2; shift 2 ;;
    --arch) arch=$2; shift 2 ;;
    --all) all=true; shift ;;
    --out-dir) out_dir=$2; shift 2 ;;
    --version) version=$2; shift 2 ;;
    --commit) commit=$2; shift 2 ;;
    --built-at) built_at=$2; shift 2 ;;
    --mode) mode=$2; shift 2 ;;
    *) usage ;;
  esac
done

if [[ -z "$commit" ]] && git rev-parse --git-dir >/dev/null 2>&1; then
  commit=$(git rev-parse HEAD)
fi
if [[ -z "$commit" ]]; then
  commit=unknown
fi
if [[ -z "$built_at" ]]; then
  built_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
fi

if [[ "$mode" == release ]]; then
  if ! [[ "$commit" =~ ^[0-9a-f]{40}$ ]]; then
    echo "build: release mode requires a full 40-character commit SHA" >&2
    exit 1
  fi
  if ! date -u -d "$built_at" +%Y-%m-%dT%H:%M:%SZ >/dev/null 2>&1; then
    echo "build: release mode requires an RFC3339 built_at" >&2
    exit 1
  fi
fi

ldflags=(
  "-s" "-w"
  "-X" "github.com/HaidaDaniel/release-watcher/internal/buildinfo.version=$version"
  "-X" "github.com/HaidaDaniel/release-watcher/internal/buildinfo.commit=$commit"
  "-X" "github.com/HaidaDaniel/release-watcher/internal/buildinfo.builtAt=$built_at"
  "-X" "github.com/HaidaDaniel/release-watcher/internal/buildinfo.buildMode=$mode"
)

mkdir -p "$out_dir"
targets=()
if $all; then
  targets=(amd64 arm64)
else
  targets=("$arch")
fi

for target_arch in "${targets[@]}"; do
  out="$out_dir/release-watcher-$os_name-$target_arch"
  echo "build: $out"
  CGO_ENABLED=0 GOOS="$os_name" GOARCH="$target_arch" \
    go build -trimpath -buildvcs=false -ldflags "${ldflags[*]}" -o "$out" ./cmd/release-watcher
done

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$out_dir" && sha256sum release-watcher-* > SHA256SUMS)
  echo "build: wrote $out_dir/SHA256SUMS"
fi
