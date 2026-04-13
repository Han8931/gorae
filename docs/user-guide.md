# Gorae User Guide

Welcome to **Gorae** (고래) 🐋, a cozy TUI librarian for your PDF collection.
This guide covers setup, configuration, themes, and daily workflow tips.
If you only need the short version, check the README; otherwise, swim on!

---

## Configuration

On first launch, Gorae creates:

- `~/.config/gorae/config.json`  
  (or `${XDG_CONFIG_HOME}/gorae/config.json` if `XDG_CONFIG_HOME` is set)

You can edit the config from inside the app:

- `:config`  open the config in your editor

### Important keys

- `watch_dir`: the root folder that Gorae watches (your PDF library).
- `meta_dir`: where metadata (SQLite DB) is stored.
- `editor`:  your preferred editor command (e.g., `nvim`).
- `pdf_viewer`: viewer command (e.g., `zathura`).
- `notes_dir`: where notes are stored (Markdown).
- `theme_path`: path to your active theme file.

### Helper folders

Gorae can maintain helper folders under your library so you can browse curated subsets
from **any file manager** (not only inside Gorae):

- `Favorites/`
- `To Read/`
- `Recently Added/`
- `Recently Read/`

> Tip: Back up `meta_dir` to preserve reading states, tags, and notes.

---

## Themes

Default theme path:

- `~/.config/gorae/theme.toml`

You can start from a built-in theme:

```sh
cp themes/fancy-dark.toml ~/.config/gorae/theme.toml
```

### Theme structure

* `[palette]` defines base colors.
* `[icons]` defines glyphs for the UI (favorite/to-read/read states, etc.).
* `[components.*]` defines styles for specific UI elements.

Supported style keys:

* `fg`, `bg`, `bold`, `italic`, `faint`

Palette references are supported, e.g.:

```toml
fg = "palette.accent"
bg = "palette.bg"
```

Common palette keys:
`bg`, `fg`, `muted`, `accent`, `success`, `warning`, `danger`, `selection`

### Reloading themes

* `:theme show` show the currently active theme path
* `:theme reload` reload theme without restarting (or restart Gorae)

### Component reference (overview)

Most UI parts map 1:1 to TOML keys (e.g., `tree_body`, `list_cursor`, `preview_info`).
Borders can be set to styles like `rounded`, `square`, or `none`.

---

## File browsing

Gorae's file browsing is inspired by tools like `lf` and `ranger`.

Navigation:
* `j` / `k`: move down / up
* `l`: enter directory
* `h`: go up to parent directory
- `g`: go to the top (start) of the list
- `G`: go to the bottom (end) of the list
* `a`: creates a directory 
* `R`: rename a directory
* `D`: delete files or dirs. 
* `q` or `Esc`: quit Gorae

> Arrow keys are also supported.

Selection:

* `Space`  toggle selection for the current item
* `v` toggle selection for all PDF files (select all / clear all). 

Sort:
* `sy`: sort by year
* `st`: sort by title


---

## Metadata & notes

Metadata:

* `e`  open the metadata editor for the current PDF
* From the editor:
  * `e`  edit inline
  * `v`  open in your external editor (configured via `editor`)

Notes:

* `n`  edit the note (Markdown) for the current PDF

---

## Copy BibTeX

* `y`  copy BibTeX for the current file (current cursor)

---

## Fetch arXiv metadata

Commands:
* `:arxiv <id> [files...]`

Batch apply:
* Select multiple files, then run:
  * `:arxiv -v <id>` (applies to selected files)

> Tip: Want zero typing? Run `:autofetch` to detect DOI or arXiv IDs from the PDF text automatically.

## Auto metadata detection

Use `:autofetch` to scan PDFs for DOI or arXiv identifiers and pull metadata automatically (Crossref + arXiv):

* `:autofetch` operates on the current file (or the selection if nothing is passed).
* `:autofetch -v` restricts the command to the current selection.
* `:autofetch <files...>` takes explicit file paths relative to the current directory.

The command relies on `pdftotext` (Poppler) to read the PDF text. Make sure Poppler tools are installed and reachable from `PATH`.

---

## Favorites, To-read, and reading states

Flags:
* `f`  toggle Favorite
* `t`  toggle To-read
* `u`  clear flags (opens a prompt)

Reading state:

* `r`  cycle reading state:

  * Unread → Reading → Read

---

## Search & filters

Search:

* `/`  open search

Flags:

* `-t <title>`
* `-y <year>`
* `-a <author>`
* `-c <content>`

Results view:

* `j/k`  move
* `Enter`  open the selected result
* `Esc` or `q`  exit

Quick filters:

* `F`  Show favorites papers
* `T`  Show to-read papers
* `O`  Show recently read papers (DB history)


## Status bar & command palette

* Status bar shows: mode, current directory, selection summary, and last message.
* `:` opens command mode.
* `:help` lists available commands.
* `?` also opens help (if enabled).

---

## Tips

* Keep Poppler updated for faster previews and better text extraction.
* Kitty uses native image previews for the first PDF page. Install `chafa` only if you want the fallback image/text-art preview path in other terminals.
* Back up `meta_dir` regularly to preserve annotations and reading states.
* Use helper folders (`Favorites/`, `To Read/`, `Recently Added/`, `Recently Read/`) in your desktop file manager to open curated subsets outside of Gorae.

Enjoy exploring your papers with Gorae! If you run into issues or have feature ideas, please open a GitHub issuewe'd love to hear from fellow readers.
