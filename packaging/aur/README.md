# AUR packaging

Two AUR packages, both maintained from this folder:

| AUR name | Source | Use case |
|---|---|---|
| [`gorae`](https://aur.archlinux.org/packages/gorae) | builds from the tagged source tarball | preferred — gets Arch users a native build |
| [`gorae-bin`](https://aur.archlinux.org/packages/gorae-bin) | downloads the pre-built `gorae-linux-amd64` from the GitHub release | for users who don't want Go installed |

The PKGBUILDs here are the **canonical** copies. The AUR-side repos are pure mirrors of the contents of each subdirectory.

## Per-release update workflow

After cutting a new GitHub release (e.g. `v2.3.0`):

1. **Compute hashes** for the new release artefacts (run from the repo root):
   ```sh
   # Source tarball (used by gorae)
   curl -fsSL -o /tmp/src.tgz https://github.com/Han8931/gorae/archive/refs/tags/v2.3.0.tar.gz
   sha256sum /tmp/src.tgz

   # Linux binary (used by gorae-bin)
   curl -fsSL -o /tmp/gorae https://github.com/Han8931/gorae/releases/download/v2.3.0/gorae-linux-amd64
   sha256sum /tmp/gorae
   ```

2. **Bump `pkgver` and reset `pkgrel=1`** in both PKGBUILDs. Update each `sha256sums=(...)` line with the values from step 1.

3. **Regenerate `.SRCINFO`** in each subdir (must be done on a machine with `makepkg` — i.e. an Arch box or container):
   ```sh
   cd packaging/aur/gorae      && makepkg --printsrcinfo > .SRCINFO
   cd packaging/aur/gorae-bin  && makepkg --printsrcinfo > .SRCINFO
   ```

4. **Test locally** before publishing:
   ```sh
   cd packaging/aur/gorae      && makepkg -si --clean
   cd packaging/aur/gorae-bin  && makepkg -si --clean
   ```

5. **Push to AUR** (each subdir maps 1:1 to a separate AUR git repo):
   ```sh
   # First-time setup per package (creates a local clone next to this folder)
   git clone ssh://aur@aur.archlinux.org/gorae.git     ../aur-repos/gorae
   git clone ssh://aur@aur.archlinux.org/gorae-bin.git ../aur-repos/gorae-bin

   # Update workflow
   cp packaging/aur/gorae/{PKGBUILD,.SRCINFO}     ../aur-repos/gorae/
   cp packaging/aur/gorae-bin/{PKGBUILD,.SRCINFO} ../aur-repos/gorae-bin/

   (cd ../aur-repos/gorae     && git add . && git commit -m "Update to v2.3.0" && git push)
   (cd ../aur-repos/gorae-bin && git add . && git commit -m "Update to v2.3.0" && git push)
   ```

   > Your AUR account's SSH public key must be registered at <https://aur.archlinux.org/account/>.

6. **Commit the canonical copies back to this repo** so the next maintainer sees the latest hashes.

## Notes

- `gorae-bin` declares `provides=('gorae')` and `conflicts=('gorae')` so users can't accidentally install both.
- Only `x86_64` is listed today. Add `aarch64` once you publish a linux-arm64 binary as part of the release artefacts (and add an entry to `source=(...)` keyed by `$arch`).
- The `gorae` source build uses `CGO_ENABLED=0` because the SQLite driver is `modernc.org/sqlite` (pure Go), so no C toolchain or build flags are needed.
