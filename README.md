# Gorae

<p align="center">
  <img src="assets/gorae.svg" alt="Gorae logo" width="180">
</p>

**Gorae** (*고래*, *whale*) is a terminal-first **TUI knowledge base for PDFs, EPUBs, and Markdown**—fast browsing, solid metadata, instant full-text search, bidirectional links, and mouse support—built as a **Vim/CLI-friendly alternative to Zotero, Mendeley, Notion, and Obsidian**.

> The Gorae logo is inspired by the **Bangudae Petroglyphs** (반구대 암각화) in Ulsan, South Korea—one of the earliest known depictions of whales and whale hunting. The “glyph-like” whale shape is meant to feel like an engraving: minimal, timeless, and a little handmade—like a good terminal tool.


<p align="center">
  <img src="assets/gorae_final_demo.gif" alt="App Demo" width="650">
</p>

## ✨ Highlights

- ⚡ **Vim-style browsing**: fast file navigation with a cozy TUI feel.
- ⭐ **Favorites**: keep your best papers one keystroke away.
- 📌 **To-read queue**: stash papers for later.
- 🕮 **Reading states**: *Unread / Reading / Read* tracked via metadata.
- 🔎 **Search**: metadata + full-text search with previews/snippets.
- ⚡ **FTS5 instant search**: index your library once with `:index`; content search becomes instant and ranked, no `pdftotext` on every query.
- 🏷️ **Hierarchical tags**: multi-tag support with prefix matching (`ml/` matches `ml/cnn`, `ml/transformers`).
- 🔗 **Bidirectional links**: write `[[document name]]` in any Markdown note to link documents; backlinks appear in the info pane automatically.
- 🖼️ **PDF image preview**: first-page previews inside the preview pane, with terminal-aware rendering paths.
- 🧾 **Auto metadata import**: detect **DOI / arXiv IDs** inside PDFs and fetch info.
- ✍️ **In-app editing**: edit metadata, import from arXiv, copy **BibTeX**.
- 🎨 **Themeable UI**: colors, glyphs, borders — plus helper folders usable in any file manager.


<!-- TODO: Add a screenshot / GIF / asciinema link -->

## Everyday use

> For deeper instructions, read **[Wiki](https://github.com/Han8931/gorae/wiki)** or run `:help`.

| Action             | Key       |
| ------------------ | --------- |
| Move               | `j/k`     |
| Enter dir / up     | `l` / `h` |
| Select             | `Space`   |
| Favorite / To-read | `f` / `t` |
| Reading state      | `r`       |
| Edit metadata      | `ee`      |
| Search             | `/`       |
| Help               | `:help`   |

> **Arrow keys and mouse** input are also supported.

### Preview support

Gorae includes first-page PDF image previews in the right preview pane.

- **Kitty**: native graphic preview path via `pdftocairo`
- **iTerm2**: terminal image preview support is included
- **Fallback path**: `chafa`-based symbol/image preview when terminal graphics are unavailable or not desired

Tested terminals for the image preview feature:

- **Kitty** on Linux
- **iTerm2** on macOS

### 🔎 Search tips

Press `/` to open search, then type your query.

You can scope the search with flags:

- `-t <title>`     search in title
- `-a <author>`    search in author
- `-y <year>`      filter by year
- `-c <keyword>`   search in full text (content)
- `--tag <tag>`    filter by a single tag
- `--tag <t1,t2>`  filter by multiple tags (comma-separated)

**Examples**
- `/ -t transformer`
- `/ -a "Yoshua Bengio"`
- `/ -y 2023`
- `/ -c attention`
- `/ --tag llm,graph`

### ⚡ Full-text index (FTS)

Content search (`-c`) uses the **FTS5 index** when available — instant, ranked results with no external tools required. Build the index once, then keep it fresh:

```
:index          index all documents under the watch root
:index here     index only the current directory
```

Files are skipped automatically when their size hasn't changed, so re-indexing is fast. The index supports **stemming** (searching "running" also finds "run") via SQLite's built-in porter tokenizer.

> Without an index, content search falls back to scanning files with `pdftotext` as before.

### 🏷️ Tags

Tags are stored as comma-separated values in the metadata editor and normalized into a dedicated table for fast lookup. Browse all tags with:

```
:tags
```

Hierarchical tags (`ml/transformers`, `ml/cnn`) are supported — searching for `ml/` matches all tags under that prefix.

### 🔗 Bidirectional links (Markdown)

In any Markdown file, write `[[filename]]` to link to another document by name. Run `:index` to resolve those links and build the link graph. After indexing:

- **Backlinks** appear at the bottom of the info pane for every document that other files point to.
- Links are resolved case-insensitively by filename (with or without extension).

## Install

For Arch Linux users:
```sh
yay -S gorae
```

### Option A) Run the pre-built executable (no Go required)

Download the ready-to-run binary from the **latest GitHub Release**.

1. **Download the file for your OS/CPU**

   * **Linux:** `gorae`
   * **macOS (Intel):** `gorae-darwin-amd64`
   * **macOS (Apple Silicon / M1–M3):** `gorae-darwin-arm64`
   * **Windows (64-bit):** `gorae-windows-amd64.exe`

2. **(Linux/macOS) Make it executable**

   ```sh
   chmod +x gorae*
   ```

3. **Move it into a folder on your PATH** (so you can run it anywhere)

   * Linux/macOS examples: `~/.local/bin`, `/usr/local/bin`
   * Windows example: `%USERPROFILE%\bin`

4. **Run it**

   ```sh
   gorae
   ```

> Tip: If your downloaded file has a long name (e.g., `gorae-darwin-arm64`), you can rename it to just `gorae` for convenience.

---

### Option B) Quick install (script)

This option builds and installs Gorae using Go.

#### Requirements

* **Go 1.21+**
* **Poppler CLI tools**: `pdftotext`, `pdfinfo`, `pdftocairo`
* **Kitty preview**: native first-page PDF image previews use `pdftocairo` and do not require `chafa`
* **iTerm2 preview**: inline image preview path is supported
* **Optional fallback preview**: `chafa` for non-Kitty / non-inline image text-art previews

Install prerequisites:

* **macOS:** `brew install golang poppler`
* **Debian/Ubuntu:** `sudo apt install golang-go poppler-utils`
* **Arch:** `sudo pacman -S go poppler`

If you want fallback previews outside Kitty:

* **macOS:** `brew install chafa`
* **Debian/Ubuntu:** `sudo apt install chafa`
* **Arch:** `sudo pacman -S chafa`

#### Optional (recommended)

* A fast PDF viewer (**Zathura** recommended)

  * **Debian/Ubuntu:** `sudo apt install zathura zathura-pdf-mupdf`
  * **Arch:** `sudo pacman -S zathura zathura-pdf-mupdf`

1. **Clone the repo**

   ```sh
   git clone https://github.com/Han8931/gorae.git
   cd gorae
   ```

2. **Run the installer**
   (Default install path: `~/.local/bin/gorae` on Linux, `/usr/local/bin/gorae` on macOS)

   ```sh
   ./install.sh

   # Install to a custom path
   GORAE_INSTALL_PATH=/usr/local/bin/gorae ./install.sh
   ./install.sh ~/bin/gorae
   ```

3. **Make sure the install directory is on your PATH**, then run:

   ```sh
   gorae
   ```

> Linux + Kitty: install `poppler`/`poppler-utils` so `pdftocairo` is available, and Gorae will use native image-based first-page PDF previews. `chafa` is not required for the Kitty path.

## Recommended PDF viewer

- Gorae works with any viewer command, but we recommend [Zathura](https://pwmt.org/projects/zathura/) with the MuPDF backend. 
- Zathura is minimal, keyboard-driven, starts instantly, supports vi-style navigation, and renders beautifully through MuPDF—great for tiling window managers.

Install:

* Debian/Ubuntu: `sudo apt install zathura zathura-pdf-mupdf`
* Arch: `sudo pacman -S zathura zathura-pdf-mupdf`

Then set the viewer command in your config:

```json
"pdf_viewer": "zathura"
```

If `zathura` is on your `PATH`, Gorae will auto-detect it, so most users can accept the default.

## Roadmap

### Fix
* [ ] Delete confirmation prompt
* [ ] Metadata view improvement

### New Features and Todo

* [ ] Open URL
* [ ] ToDo management
* [ ] Vault warden for cloud support
* [ ] WebServer
* [ ] Trash

### Knowledge base (in progress)

* [x] **FTS5 full-text index** — instant ranked content search via `:index`
* [x] **Hierarchical multi-tag table** — normalized tags with prefix matching and `:tags` browser
* [x] **Bidirectional links** — `[[wikilinks]]` in Markdown with backlinks in the info pane
* [ ] **Semantic / vector search** — embeddings via local model (ollama) stored in SQLite
* [ ] **AI Q&A over collection** — RAG: ask a question, get an answer citing your own documents
* [ ] **Citation network** — auto-fetch citation relationships from DOIs via Semantic Scholar

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

## Attribution / Credit

This project is licensed under the MIT License.

If you use Gorae in your project, documentation, or distribution, please credit:
- **Gorae by Han**
- link to the project repository

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
