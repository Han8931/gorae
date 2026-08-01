#!/usr/bin/env bash
set -euo pipefail

default_dest="$HOME/.local/bin/gorae"
if [[ "$(uname -s)" == "Darwin" ]]; then
	default_dest="/usr/local/bin/gorae"
fi
dest="${GORAE_INSTALL_PATH:-${1:-$default_dest}}"
build_dir="$(mktemp -d)"
cleanup() {
	rm -rf "$build_dir"
}
trap cleanup EXIT

if ! command -v go >/dev/null 2>&1; then
	echo "error: Go 1.25+ is required but was not found in PATH" >&2
	exit 1
fi

missing_poppler=()
for cmd in pdftotext pdfinfo pdftocairo; do
	if ! command -v "$cmd" >/dev/null 2>&1; then
		missing_poppler+=("$cmd")
	fi
done

echo "Building gorae..."
GO111MODULE=on go build -o "$build_dir/gorae" ./cmd/gorae

target_dir="$(dirname "$dest")"
mkdir -p "$target_dir"
install -m 755 "$build_dir/gorae" "$dest"

config_home="${XDG_CONFIG_HOME:-$HOME/.config}"
if [[ -f "$config_home/gorae/config.json" ]]; then
	"$dest" --migrate-config
fi

echo "gorae installed to $dest"
echo "Add $(dirname "$dest") to your PATH if it is not already available."

if [[ ${#missing_poppler[@]} -gt 0 ]]; then
	echo
	echo "warning: missing Poppler tools: ${missing_poppler[*]}" >&2
	echo "Gorae will build, but PDF metadata extraction and PDF previews will be limited until Poppler is installed." >&2
	echo "Install Poppler with one of:" >&2
	echo "  macOS:        brew install poppler" >&2
	echo "  Debian/Ubuntu: sudo apt install poppler-utils" >&2
	echo "  Arch:         sudo pacman -S poppler" >&2
fi

if command -v chafa >/dev/null 2>&1; then
	echo "Optional preview fallback detected: chafa"
else
	echo "Note: chafa is optional and only used for non-Kitty preview fallbacks."
fi
