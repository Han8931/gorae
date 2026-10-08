# Local release binary builds

These notes walk through building standalone binaries for each supported platform without requiring Go on the target machine.

## Prerequisites

- Go 1.25+ and the project dependencies installed on this machine.
- Poppler CLI tools (`pdftotext`, `pdfinfo`) available so you can test the Linux build; end users still need these tools, but not Go.

From the repo root (the `gorae` checkout):

```sh
mkdir -p dist
```

## Build Everything

`tools/build-binaries.sh` builds every target with the exact flags the published
artefacts use and writes `dist/SHA256SUMS`. Prefer it over the per-platform
commands below:

```sh
./tools/build-binaries.sh
```

## Build Per Platform

Set the release flags first. They are what makes the output reproducible, so a
binary built without them will not match the checksums recorded in
`packaging/aur/gorae-bin/PKGBUILD`:

```sh
export GOFLAGS="-trimpath -mod=readonly -buildvcs=false"
```

`-buildvcs=false` matters more than it looks: without it Go stamps the commit
and a `vcs.modified=true` flag into the binary, so the checksum changes
depending on whether your working tree happened to be clean.

1. **Linux (amd64)**
   ```sh
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o dist/gorae-linux-amd64 ./cmd/gorae

    ./dist/gorae-linux-amd64 -help   # sanity-check
   ```
2. **Linux (arm64)**
   ```sh
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
    go build -ldflags="-s -w" -o dist/gorae-linux-arm64 ./cmd/gorae
   ```
3. **macOS (Intel)**
   ```sh
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 \
    go build -ldflags="-s -w" -o dist/gorae-darwin-amd64 ./cmd/gorae
   ```
4. **macOS (Apple Silicon)**
   ```sh
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
    go build -ldflags="-s -w" -o dist/gorae-darwin-arm64 ./cmd/gorae
   ```
5. **Windows (amd64)**
   ```sh
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
    go build -ldflags="-s -w" -o dist/gorae-windows-amd64.exe ./cmd/gorae
   ```

`-ldflags="-s -w"` strips the symbol table and DWARF data; the published
release artefacts are built this way, so a binary built without it will not
match the checksums recorded in `packaging/aur/gorae-bin/PKGBUILD`.

With these flags the `linux/amd64` artefact is byte-identical to what
`packaging/aur/gorae/PKGBUILD` builds from the release tarball, so `gorae` and
`gorae-bin` install the same binary. Worth re-checking when the flags change:

```sh
sha256sum dist/gorae-linux-amd64   # must equal gorae-bin's first sha256sum
```

## Distribute

- Share the files inside `dist/` that match each user's platform. Compress them if needed.
- Users copy the binary to a directory on their `PATH` (e.g., `~/.local/bin`, `/usr/local/bin`, `%USERPROFILE%\\bin`) and run `gorae -root /path/to/Papers`.
- No Go toolchain required on the target system, but Poppler tools must be installed for metadata/text extraction features.

The generated `dist/` artifacts are ignored by Git.
