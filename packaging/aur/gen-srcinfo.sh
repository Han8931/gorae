#!/usr/bin/env bash
# Regenerate .SRCINFO files for both gorae AUR packages using a throwaway
# Arch Linux container. Works from any host with Docker installed (macOS, Linux).
#
# Usage: ./gen-srcinfo.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

docker run --rm -v "$SCRIPT_DIR:/work" -w /work archlinux:latest bash -c '
  set -euo pipefail
  pacman -Sy --noconfirm --needed base-devel >/dev/null
  useradd -m builder
  chown -R builder /work
  for pkg in gorae gorae-bin; do
    echo ">> $pkg"
    su builder -c "cd /work/$pkg && makepkg --printsrcinfo" > "$pkg/.SRCINFO"
  done
'

echo
echo "Generated:"
ls -la "$SCRIPT_DIR/gorae/.SRCINFO" "$SCRIPT_DIR/gorae-bin/.SRCINFO"
