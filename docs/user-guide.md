# Gorae User Guide

Welcome to Gorae (고래) 🐋 — a TUI librarian for your PDF collection. This guide dives into setup, configuration, and daily workflow tips. If you just need the short version, stick with the README; otherwise, swim on!

## Requirements & Installation

1. Install Go 1.21+ and Poppler utilities (`pdftotext`, `pdfinfo`):
   - **macOS**: `brew install golang poppler`
   - **Debian/Ubuntu**: `sudo apt install golang-go poppler-utils`
   - **Arch**: `sudo pacman -S go poppler`
2. Clone the repo and run the helper script:

   ```sh
   git clone https://github.com/Han8931/gorae.git
   cd gorae
   ./install.sh             # default path (~/.local/bin on Linux, /usr/local/bin on macOS)
   ./install.sh ~/bin/gorae # or pass your own destination
   ```

   Use `GORAE_INSTALL_PATH=/custom/path ./install.sh` if you prefer an env var over arguments.

3. Alternatively, install manually:

   ```sh
   go install ./cmd/gorae
   # or
   go build -o gorae ./cmd/gorae
   install -Dm755 gorae ~/.local/bin/gorae
   ```

Run `gorae` (optionally `-root /path/to/Papers`) to start the UI.

## Configuration

- First launch creates `~/.config/gorae/config.json` (or `${XDG_CONFIG_HOME}/gorae/config.json`).
- Use `:config` inside the app to edit with your preferred editor, `:config show` to inspect paths, and `:config editor <cmd>` to change the editor itself.
- Important keys:
  - `watch_dir`: root folder that Gorae watches.
  - `meta_dir`: where metadata/SQLite DB lives.
  - `editor`, `pdf_viewer`, `notes_dir`, `theme_path`.
  - Helper folders under `watch_dir`: `Recently Added`, `Recently Read`, `Favorites`, `To Read`. Gorae keeps them in sync so you can browse them with any file manager.

### Recommended PDF viewer

Gorae works with any viewer command, but the default `pdf_viewer` is [Zathura](https://pwmt.org/projects/zathura/) using the MuPDF backend. Zathura is lightweight, vi-key friendly, and renders quickly with MuPDF, which makes it ideal for bouncing between the TUI and an external window. Install it with your package manager (`sudo pacman -S zathura zathura-pdf-mupdf`, `sudo apt install zathura zathura-pdf-mupdf`, etc.) and either keep the auto-detected default or set `"pdf_viewer": "zathura"` explicitly in `config.json`.

## Themes

- Default theme path: `~/.config/gorae/theme.toml` (create from `themes/fancy-dark.toml` or edit the generated file).
- `[palette]` defines colors, `[icons]` defines glyphs, `[components.*]` customize each pane. Supported styles: `fg`, `bg`, `bold`, `italic`, `faint`.
- Components can reference palette entries via `fg = "palette.accent"` (supported keys: `bg`, `fg`, `muted`, `accent`, `success`, `warning`, `danger`, `selection`) so you can tweak palette values and see the whole UI update.
- Run `:theme reload` (or restart Gorae) after editing themes.
- Use `:theme show` inside the app to confirm which file is active, and `:theme reload` to apply changes without a restart.

### Component reference

- Tree/list panels, preview pane, metadata overlay, status bar, prompts, separators, etc. map 1:1 to the TOML keys (`tree_body`, `list_cursor`, `preview_info`, …).
- Borders can be `rounded`, `square`, `ascii`, `none`, etc.

## Metadata & Notes

- `e`: preview metadata, `e` again to edit inline, `v` to open the structured form in your editor.
- `n`: edit the note (Markdown) for the current PDF.
- `f` toggles Favorite, `t` toggles To-read, `u` opens a prompt to clear flags.
- Reading state cycles with `r`: `○` → `▶` → `✓`.
- `y`: copy BibTeX for the current file.
- Fetch arXiv metadata with `:arxiv <id> [files...]` or `:arxiv -v` to apply to selected files.

## Search & Filters

- `/` or `:search <query>` runs a lookup. Modes: `content`, `title`, `author`, `year`.
- Flags: `-mode`, `-case`, `-root PATH`. Shortcuts: prefix query with `title:` etc.
- Results view controls: `j/k` (up/down), `PgUp/PgDn`, `Enter` to open, `Esc/q` to exit, `/` to search again.
- Quick filters:
  - `F`: favorites
  - `T`: to-read
  - `g r` / `g u` / `g d`: Reading / Unread / Read states
- Result rows show `Title (Year)` plus hit counts; the preview pane lists snippets or metadata.

## Status & Command Palette

- Status bar displays mode, directory, selection summary, and last message.
- `:` opens command mode (`:help` lists available commands).
- `?` inside the UI (or `:help`) surfaces built-in help.

## Tips

- Keep Poppler updated for faster previews/search.
- Back up `meta_dir` to preserve your annotations and reading states.
- Use the helper folders (`Favorites/`, `To Read/`, `Recently Added/`, `Recently Read/`) in your desktop file manager to quickly open curated subsets outside of Gorae.

Enjoy exploring your papers with Gorae! If you bump into issues or have feature ideas, open a GitHub issue—we’re always happy to hear from fellow readers.
