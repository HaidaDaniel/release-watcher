#!/usr/bin/env bash
# Build a release artifact on the development machine and write a manifest.
# The NAS receives only the resulting binary and manifest; it never compiles.
set -Eeuo pipefail
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo"

usage() {
  cat >&2 <<'USAGE'
usage: scripts/package-release.sh --version V [--arch amd64|arm64]
                                  [--out-dir bin/release] [--built-at RFC3339]
                                  [--commit SHA]

Requires a clean Git tree and HEAD equal to --commit (default HEAD).
Runs verify-fast, verify-push and go test -race before building.
USAGE
  exit 2
}

version=""
arch=amd64
out_dir=bin/release
built_at=""
commit=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) version=${2:?}; shift 2 ;;
    --arch) arch=${2:?}; shift 2 ;;
    --out-dir) out_dir=${2:?}; shift 2 ;;
    --built-at) built_at=${2:?}; shift 2 ;;
    --commit) commit=${2:?}; shift 2 ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done

[[ -n $version ]] || usage
[[ $version != *-dev ]] || { echo 'package-release: use a non-dev version' >&2; exit 1; }
case "$arch" in amd64|arm64) ;; *) echo 'package-release: unsupported arch' >&2; exit 1 ;; esac
if [[ -z $commit ]]; then commit=$(git rev-parse HEAD); fi
[[ $commit =~ ^[0-9a-f]{40}$ ]] || { echo 'package-release: commit must be a full SHA' >&2; exit 1; }
[[ $(git rev-parse HEAD) == "$commit" ]] || { echo 'package-release: --commit must equal HEAD' >&2; exit 1; }
[[ -z $(git status --porcelain) ]] || { echo 'package-release: working tree is not clean' >&2; exit 1; }
if [[ -z $built_at ]]; then built_at=$(date -u +%Y-%m-%dT%H:%M:%SZ); fi
[[ $built_at =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(Z|[+-][0-9]{2}:[0-9]{2})$ ]] || {
  echo 'package-release: --built-at must be RFC3339' >&2
  exit 1
}

echo "package-release: verify-fast"
scripts/verify-fast.sh
echo "package-release: verify-push"
scripts/verify-push.sh
echo "package-release: go test -race ./..."
go test -race ./...

echo "package-release: build linux/$arch"
scripts/build.sh --os linux --arch "$arch" --version "$version" --commit "$commit" \
  --built-at "$built_at" --mode release --out-dir "$out_dir"

artifact="$out_dir/release-watcher-linux-$arch"
[[ -x $artifact ]] || { echo "package-release: missing artifact $artifact" >&2; exit 1; }
sha256=$(sha256sum "$artifact" | awk '{print $1}')
bytes=$(stat -c %s "$artifact")
host_arch=$(uname -m)
case "$host_arch" in x86_64) host_arch=amd64 ;; aarch64|arm64) host_arch=arm64 ;; esac
identity_probe=skipped-cross-arch
if [[ $host_arch == "$arch" ]]; then
  identity_probe=executed
fi

python3 - "$artifact" "$version" "$commit" "$built_at" "$sha256" "$bytes" "$arch" "$identity_probe" <<'PY'
import json, os, subprocess, sys
artifact, version, commit, built_at, sha256, size, arch, identity_probe = sys.argv[1:]
if identity_probe == "executed":
    out = subprocess.check_output([artifact, "version", "--json"], text=True)
    doc = json.loads(out)
    data = doc.get("data", {})
    want = {"version": version, "commit": commit, "built_at": built_at, "build_mode": "release"}
    for key, value in want.items():
        if data.get(key) != value:
            raise SystemExit(f"package-release: binary {key}={data.get(key)!r}, want {value!r}")
    if data.get("go_version") is None:
        raise SystemExit("package-release: binary has no Go version")
else:
    print("package-release: cross-arch build; skipping execution identity probe")
manifest = {
    "format": "release-watcher-artifact",
    "format_version": 1,
    "name": "release-watcher",
    "version": version,
    "commit": commit,
    "goos": "linux",
    "goarch": arch,
    "built_at": built_at,
    "identity_probe": identity_probe,
    "artifact": os.path.basename(artifact),
    "sha256": sha256,
    "bytes": int(size),
    "checks": {
        "verify_fast": True,
        "verify_push": True,
        "race": True,
    },
}
path = os.path.join(os.path.dirname(artifact), "manifest.json")
with open(path, "w", encoding="utf-8") as handle:
    json.dump(manifest, handle, indent=2, sort_keys=True)
    handle.write("\n")
print(path)
PY

echo "package-release: artifact $artifact"
echo "package-release: sha256 $sha256"
echo "package-release: manifest $out_dir/manifest.json"
