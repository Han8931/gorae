#!/usr/bin/env bash
set -euo pipefail

if ! command -v go >/dev/null 2>&1; then
	echo "error: Go toolchain not found in PATH" >&2
	exit 1
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
dist_dir="${1:-$repo_root/dist}"

mkdir -p "$dist_dir"

targets=(
	"linux amd64 gorae-linux-amd64"
	"linux arm64 gorae-linux-arm64"
	"darwin amd64 gorae-darwin-amd64"
	"darwin arm64 gorae-darwin-arm64"
	"windows amd64 gorae-windows-amd64.exe"
)

cd "$repo_root"

# These flags are what makes a release artefact reproducible, so changing any of
# them changes the checksums recorded in packaging/aur/gorae-bin/PKGBUILD:
#   -s -w          strip the symbol table and DWARF data
#   -trimpath      keep absolute build paths out of the binary
#   -mod=readonly  fail rather than quietly edit go.mod
#   -buildvcs=false  omit the commit stamp, which otherwise records
#                    vcs.modified=true and makes the output depend on whether
#                    the working tree happened to be clean
# They match the GOFLAGS in packaging/aur/gorae/PKGBUILD, so the source and
# binary AUR packages build the same bytes for linux/amd64.
export GOFLAGS="-trimpath -mod=readonly -buildvcs=false"

for target in "${targets[@]}"; do
	read -r goos goarch filename <<<"$target"
	echo "Building $goos/$goarch -> $dist_dir/$filename"
	CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
		go build -ldflags="-s -w" -o "$dist_dir/$filename" ./cmd/gorae
done

# SHA256SUMS ships with the release so users can verify a downloaded binary.
(cd "$dist_dir" && sha256sum gorae-* >SHA256SUMS)

echo "All binaries written to $dist_dir"
cat "$dist_dir/SHA256SUMS"
