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
	"linux amd64 gorae"
	"darwin amd64 gorae-darwin-amd64"
	"darwin arm64 gorae-darwin-arm64"
	"windows amd64 gorae-windows-amd64.exe"
)

cd "$repo_root"
for target in "${targets[@]}"; do
	read -r goos goarch filename <<<"$target"
	echo "Building $goos/$goarch -> $dist_dir/$filename"
	GOOS="$goos" GOARCH="$goarch" go build -o "$dist_dir/$filename" ./cmd/gorae
done

echo "All binaries written to $dist_dir"
