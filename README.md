# Gorae

<p align="center">
  <img src="gorae.svg" alt="Gorae logo" width="180">
</p>

Gorae (*고래*, *whale*) is a cozy TUI librarian for your PDFs—built for Vim/CLI/TUI lovers who want to stay in the terminal, keep metadata in sync, and enjoy quick search/favorite/to-read queues.

**Highlights**
- Fast file browser with metadata-aware favorites, to-read list, and reading states.
- Search across content or metadata with instant previews/snippets.
- In-app metadata editor, arXiv importer, and BibTeX copy.
- Themeable UI (colors, glyphs, borders) plus helper folders you can browse in any file manager.

## Quick install

1. Install Go 1.21+ from [go.dev](https://go.dev/dl/).
2. Clone this repository:

   ```sh
   git clone https://github.com/Han8931/gorae.git
   cd gorae
   ```

3. Run the helper script (default path: `~/.local/bin/gorae` on Linux, `/usr/local/bin/gorae` on macOS):

   ```sh
   ./install.sh
   # choose another destination via env var or first argument
   GORAE_INSTALL_PATH=/usr/local/bin/gorae ./install.sh
   ./install.sh ~/bin/gorae
   ```

Once the script finishes, ensure the destination directory is on your `PATH`, then launch the app with:

```sh
gorae        # optionally: gorae -root /path/to/Papers
```

## Platform support

Gorae is tested on macOS (Apple Silicon + Intel) and common Linux distros including Arch Linux and Debian/Ubuntu. As long as Go 1.21+ and the Poppler CLI tools (`pdftotext`, `pdfinfo`) are available, the TUI runs the same everywhere. Use `brew install golang poppler`, `sudo apt install golang-go poppler-utils`, or `sudo pacman -S go poppler` to grab the prerequisites on your platform.

## Manual install

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

After the binary is on `PATH`, launch `gorae` from any folder (pass `-root /path/to/Papers` to point at a different library). One command is all it takes to start browsing your collection.

## Uninstall

1. Delete the binary you installed (default `~/.local/bin/gorae` on Linux or `/usr/local/bin/gorae` on macOS).
2. Remove the config/data folders if you no longer need them:

   ```sh
   rm -rf ~/.config/gorae    # config + theme
   rm -rf ~/.local/share/gorae   # metadata store, notes, db
   ```

That's it—you can re-clone and reinstall at any time.

## Everyday use

- `f` = toggle Favorite, `t` = toggle To-read, `r` = cycle reading state (unread → reading → read).
- `y` copies BibTeX, `n` edits notes, `e`/`v` open metadata editors.
- Navigate files with Vim-style motion: `j/k` move, `h` goes up a directory, `l` enters, space toggles selection. Support arrow keys too. 
- `a` creates a directory, `d` cuts selected files, 
- `/` searches (use flags like `-t title`, `-a author`, `-y year`, `-c content`), 
- `F`/`T` open smart lists (favorites, to-read).
- `:help` inside the app lists every command.
- `config` opens the config file.

For deeper instructions (config, themes, metadata, search tips, helper folders, etc.) read **[docs/user-guide.md](docs/user-guide.md)**. Prefer a different look? Grab one of the ready-made themes in `themes/` (e.g., `aurora.toml`, `matcha.toml`, `fancy-dark.toml`) and point `config.theme_path` at it or copy it to `~/.config/gorae/theme.toml`.

TODO
- Fix Recently read dir issue.
- Update and revise README and manual. 
- logo command

AI features:
- AI tag
- AI Summary
- [Extract texts (OCR)](https://pymupdf.readthedocs.io/en/latest/pymupdf4llm/)
- Knowledge Graphs
- RAG
- Prompt management

## [Bangudae Petroglyphs](https://en.wikipedia.org/wiki/Bangudae_Petroglyphs)

The world's earliest known depictions of whale hunting are found in the [Bangudae Petroglyphs (picture link)](https://www.khan.co.kr/article/202007080300025) in South Korea, dating back around 7,000 years (6,000 BC), showcasing detailed scenes of boats and harpoons; however, similar ancient whale art is also found in the White Sea region (Russia/Scandinavia) and Norway, possibly as old, depicting complex hunts and spiritual meanings beyond simple prey, suggesting widespread ancient maritime cultures. 

## Acknowledgement
