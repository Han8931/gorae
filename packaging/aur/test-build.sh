#!/usr/bin/env bash
# Build both gorae AUR packages inside a throwaway Arch container — sanity
# check before pushing to AUR. Works on any host with Docker.
#
# Usage: ./test-build.sh            # builds both
#        ./test-build.sh gorae      # builds just one
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PKGS=("$@")
if [ ${#PKGS[@]} -eq 0 ]; then
  PKGS=(gorae gorae-bin)
fi

docker run --rm -v "$SCRIPT_DIR:/work" -w /work archlinux:latest bash -c "
  set -euo pipefail
  pacman -Sy --noconfirm --needed base-devel go poppler curl >/dev/null
  useradd -m builder
  chown -R builder /work
  for pkg in ${PKGS[*]}; do
    echo
    echo '==================== '\$pkg' ===================='
    su builder -c \"cd /work/\$pkg && makepkg --noconfirm --clean --syncdeps\"
  done
"

echo
echo "Built artefacts:"
find "$SCRIPT_DIR" -maxdepth 2 -name '*.pkg.tar.zst' -print
