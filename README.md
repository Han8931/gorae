# Gorae

<p align="center">
  <img src="gorae.svg" alt="Gorae logo" width="180">
</p>

**Gorae** (*고래*, *whale*) is a terminal-first **TUI librarian for PDFs** — fast browsing, solid metadata, and quick search for people who live in **Vim/CLI**.

> The Gorae logo is inspired by the **Bangudae Petroglyphs** (반구대 암각화) in Ulsan, South Korea—one of the earliest known depictions of whales and whale hunting. The “glyph-like” whale shape is meant to feel like an engraving: minimal, timeless, and a little handmade—like a good terminal tool.

## ✨ Highlights

- ⚡ **Vim-style browsing**: fast file navigation with a cozy TUI feel.
- ⭐ **Favorites**: keep your best papers one keystroke away.
- 📌 **To-read queue**: stash papers for later.
- 🕮 **Reading states**: *Unread / Reading / Read* tracked via metadata.
- 🔎 **Search**: metadata + full-text search with previews/snippets.
- 🧾 **Auto metadata import**: detect **DOI / arXiv IDs** inside PDFs and fetch info.
- ✍️ **In-app editing**: edit metadata, import from arXiv, copy **BibTeX**.
- 🎨 **Themeable UI**: colors, glyphs, borders — plus helper folders usable in any file manager.


## Demo

<!-- TODO: Add a screenshot / GIF / asciinema link -->

## Everyday use

> For deeper instructions, read **[docs/user-guide.md](docs/user-guide.md)** or run `:help`.

| Action             | Key       |
| ------------------ | --------- |
| Move               | `j/k`     |
| Enter dir / up     | `l` / `h` |
| Select             | `Space`   |
| Favorite / To-read | `f` / `t` |
| Reading state      | `r`       |
| Edit metadata      | `ee`       |
| Search             | `/`       |
| Help               | `:help`   |

> Arrow keys are also supported.

### Search tips

Search (`/`) with flags like:

* `-t [title]`
* `-a [author]`
* `-y [year]`
* `-c [content]`

### Fetch arXiv metadata

Gorae scan DOI or arXiv identifiers from new PDFs and populate metadata automatically.

You can do this manually by

* Commands:
    * `:autofetch [files...]` scans the current file (or specified files) for identifiers.
* Batch apply:
    * Select multiple files, then run:
    * `:autofetch -v` applies to the current selection.

## Install

### Requirements

**Required**
- Go 1.21+
- Poppler CLI tools: `pdftotext`, `pdfinfo`

**Optional (recommended)**
- A fast PDF viewer (Zathura recommended below)
- OCR / AI features (planned)

Install prerequisites:
- macOS: `brew install golang poppler`
- Debian/Ubuntu: `sudo apt install golang-go poppler-utils`
- Arch: `sudo pacman -S go poppler`

### Quick install (script)

1. Clone this repository:

```sh
git clone https://github.com/Han8931/gorae.git
cd gorae
```

2. Run the helper script (default path: `~/.local/bin/gorae` on Linux, `/usr/local/bin/gorae` on macOS):

```sh
./install.sh

# or choose another destination via env var or first argument
GORAE_INSTALL_PATH=/usr/local/bin/gorae ./install.sh
./install.sh ~/bin/gorae
   ```

3. Ensure the destination directory is on your `PATH`, then launch:

```sh
gorae        # optionally: gorae -root /path/to/Papers
```

> You can also use the pre-built binary for your platform from the latest GitHub Release (or from the `dist/` folder if you cloned the repo), place it somewhere on your `PATH`, and run it directly:
> 
> 1. Download the file that matches your OS/architecture (`gorae`, `gorae-darwin-amd64`, `gorae-darwin-arm64`, or `gorae-windows-amd64.exe`).
> 2. Make it executable if needed (`chmod +x gorae-*` on Linux/macOS).
> 3. Move it into a directory on your `PATH` (e.g., `~/.local/bin`, `/usr/local/bin`, or `%USERPROFILE%\bin`).
> 4. Launch it from any terminal: `gorae -root /path/to/Papers`.

### Manual install

```sh
git clone https://github.com/Han8931/gorae.git
cd gorae

# Install to $(go env GOPATH)/bin so it is available everywhere
go install ./cmd/gorae
export PATH="$(go env GOPATH)/bin:$PATH"

# or build/copy to a directory you manage
go build -o gorae ./cmd/gorae
install -Dm755 gorae ~/.local/bin/gorae   # adjust destination as needed
```

After the binary is on `PATH`, launch `gorae` from any folder (pass `-root /path/to/Papers` to point at a different library).

## Config & themes

Gorae stores configuration and user data in standard locations:

* Config + theme:
  * `~/.config/gorae/`
  * `~/.config/gorae/theme.toml`
* Data (metadata DB, notes, cache):
  * `~/.local/share/gorae/`

You can open and edit the config from inside the app using `:config`.

If you prefer a different look, pick one of the ready-made themes in `themes/` (e.g., `aurora.toml`, `matcha.toml`, `fancy-dark.toml`) and set `theme_path` in the config (via `:config`), or copy a theme file to:

```sh
cp themes/matcha.toml ~/.config/gorae/theme.toml
```

## Recommended PDF viewer

Gorae works with any viewer command, but we recommend [Zathura](https://pwmt.org/projects/zathura/) with the MuPDF backend. Zathura is minimal, keyboard-driven, starts instantly, supports vi-style navigation, and renders beautifully through MuPDF—great for tiling window managers.

Install:

* Debian/Ubuntu: `sudo apt install zathura zathura-pdf-mupdf`
* Arch: `sudo pacman -S zathura zathura-pdf-mupdf`

Then set the viewer command in your config:

```json
"pdf_viewer": "zathura"
```

If `zathura` is on your `PATH`, Gorae will auto-detect it, so most users can accept the default.

## Roadmap

### New Features and Todo

* [ ] Confirm file dir on the first launch 
* [ ] Support mouse
* [ ] Epub support
* [ ] Gif file
* [ ] Open a file at a certain position
* [ ] `gorae logo` command
* [ ] Vault warden
* [ ] WebServer

### AI features (planned)

* Audio reading
* AI tagging and summarization
* Text extraction (OCR) (see: [https://pymupdf.readthedocs.io/en/latest/pymupdf4llm/](https://pymupdf.readthedocs.io/en/latest/pymupdf4llm/))
* RAG and knowledge graphs
* Prompt management

## Uninstall

1. Delete the binary you installed (default `~/.local/bin/gorae` on Linux or `/usr/local/bin/gorae` on macOS).
2. Remove the config/data folders if you no longer need them:

   ```sh
   rm -rf ~/.config/gorae        # config + theme
   rm -rf ~/.local/share/gorae   # metadata store, notes, db
   ```

That's it—you can re-clone and reinstall at any time.

## Acknowledgements

<table>
  <tr>
    <td align="center" width="170">
      <a href="https://github.com/fineday38">
        <img src="https://github.com/fineday38.png?size=120" width="50" height="50" alt="fineday38" style="border-radius:50%;" />
      </a>
      <br/>
      <a href="https://github.com/fineday38">fineday38</a>
      <br/>
    </td>
  </tr>
</table>
