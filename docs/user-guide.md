# Gorae User Guide

Gorae is a terminal knowledge base for PDF, EPUB, and Markdown libraries. This
guide covers configuration, themes, browsing, metadata, search, and the AI
assistant. Run `:help` inside Gorae for the compact key reference.

## Configuration

On first launch, Gorae creates:

- `${XDG_CONFIG_HOME:-~/.config}/gorae/config.json`
- `${XDG_CONFIG_HOME:-~/.config}/gorae/theme.toml`

Open the configuration with `:config`, or inspect it with `:config show`.
Gorae automatically adds newly introduced top-level settings without replacing
existing values or comments.

Important settings:

| Key | Purpose |
|---|---|
| `watch_dir` | Root of the document library |
| `meta_dir` | SQLite database, index, and application data |
| `notes_dir` | Markdown notes |
| `editor` | External editor command, such as `nvim` |
| `pdf_viewer` | PDF viewer command, such as `zathura` |
| `theme` | Bundled theme name; takes precedence over `theme_path` |
| `theme_path` | Custom TOML theme path |
| `show_tree` | Show the directory tree at startup; default `false` (folded) |
| `enable_mouse` | Enable mouse input and scrolling |
| `text_preview_only` | Disable inline image previews |
| `recent_days` | Age window for Recently Added |
| `recently_opened_limit` | Maximum entries in Recently Read |

The `ai` and `web_search` objects configure providers, models, retrieval, tool
calling, and optional web search. The generated config documents every field.

Gorae maintains `To Read/`, `Recently Added/`, and
`Recently Read/` helper directories beneath the library. They appear as tabs
along the top of the Files pane (switch with `Left`/`Right`) rather than as
folders in the Library listing. The Reading, Unread, and Read tabs list papers
by reading state; their link folders live under `meta_dir/views/`, and a paper
moves between them as soon as `r` changes its state. Back up `meta_dir` and
`notes_dir` to preserve metadata, reading state, the search index, and notes.

## Themes

Use `:theme list` to see bundled themes and `:theme <name>` to apply one. In the
theme chooser:

| Key | Action |
|---|---|
| `Tab` | Next theme |
| `Shift+Tab` | Previous theme |
| `Enter` | Apply highlighted theme |
| `Esc` | Cancel command input |

Other theme commands:

- `:theme show` displays the active theme and path.
- `:theme reload` reloads the active bundled or custom theme.

Custom themes use `[palette]`, `[icons]`, and `[components.*]` sections.
Component styles support `fg`, `bg`, `bold`, `italic`, and `faint`. Palette
references such as `fg = "palette.accent"` are supported. Common palette keys
are `bg`, `fg`, `muted`, `accent`, `success`, `warning`, `danger`, and
`selection`.

Selected file rows use the selection background. The cursor uses a distinct
cursor background and wins visually when it rests on a selected row.

## File browsing

| Key | Action |
|---|---|
| `j` / `k` or `Up` / `Down` | Move down/up |
| `l` or `Enter` | Enter a directory or open a document |
| `h` or `Backspace` | Parent directory (from the top of a collection tab, back to Library) |
| `Tab` / `Shift+Tab` or `Left` / `Right` | Switch Files tab: Library, Recent, Reading, To Read, Added, Unread, Read |
| `g` / `G` | Top/bottom; `g` also begins a reading-state filter |
| `,n` | Toggle the tree pane for this session |
| `Ctrl+W` `h` / `l` | Move focus to the tree / the files |
| `Space` | Toggle selection and advance |
| `v` | Select/clear all PDF files |
| `a` | Create a directory |
| `R` | Rename |
| `D` | Delete; press `y` to confirm, `n`/`Esc` to cancel |
| `d` / `p` | Cut/paste |
| `sn` / `st` / `sy` | Sort by name/title/year |
| `q` or `Ctrl+C` | Quit |

The tree starts folded, so the list and detail panes share its width. Set
`"show_tree": true` to show it at startup.

## Metadata, notes, and flags

- `e` opens the editing view: one row per metadata field, in place of the file
  list. Everything is saved the moment you change it, so `Esc` is always safe.

  | Key | Action |
  |---|---|
  | `j` / `k` | Move between fields (long values unfold under the cursor) |
  | `Enter` (or `e`) | Edit the selected field in the prompt line |
  | `Tab` / `Shift+Tab` | While editing: save and move to the next/previous field |
  | `Esc` | While editing: cancel the field · otherwise close the view |
  | `r` / `f` / `t` | Cycle reading state · toggle Favorite · toggle To Read |
  | `n` | Edit the note in your editor |
  | `E` | Edit every field at once as JSON in your editor |
  | `g` / `G`, `PgUp` / `PgDn` | First/last field, page through |

  A `Year` has to contain a four-digit year, otherwise the year sort and BibTeX
  export cannot use it.
- `n` edits the current document's Markdown note.
- `t` toggles To Read.
- `u` opens the flag-removal prompt.
- `r` cycles Unread → Reading → Read.
- `yy` copies BibTeX.
- `yt` copies title, author, and year.

Use `:arxiv <id>` to import arXiv metadata. Add `-v` to operate on the current
selection. `:autofetch` detects DOI and arXiv identifiers automatically;
`:autofetch -v` restricts it to selected files. These features require
`pdftotext` from Poppler.

## Search and filters

Press `/` to search. Supported flags include:

- `-t <title>`
- `-a <author>`
- `-y <year>`
- `-c <content>`
- `--tag <tag>` or `--tag <tag1,tag2>`

Run `:index` to build/update the entire FTS5 index, or `:index here` for the
current directory.

Search-result keys:

| Key | Action |
|---|---|
| `j` / `k` | Move between matching documents |
| `n` / `N` or `Tab` / `Shift+Tab` | Move between hits within a document |
| `Enter` | Open at the selected hit/page |
| `PgUp` / `PgDn` | Page through results |
| `/` | Search again |
| `Esc` / `q` | Close results |

Quick filters: `F` Favorites, `T` To Read, and `O` Recently Read. Use `g r`,
`g u`, and `g d` to filter Reading, Unread, and Read states.

## AI assistant

Run `:index`, then `:gorae`. The assistant supports retrieval from the library,
streaming responses, saved sessions, focused papers, summaries, skills, and
optional tool calling and web search.

In `/load` mode:

| Key | Action |
|---|---|
| Type | Filter papers live |
| `↑` / `↓` | Move through results |
| `Tab` | Mark the current paper and advance |
| `Enter` | Load marked papers, or the current paper when none are marked |
| `Esc` | Cancel |

Use `/unfocus` to clear focused papers. `/select` remains a legacy alias.
Press `/help` in chat for all slash commands and insert/navigation-mode keys.

## Status and command output

The status bar shows the current mode, directory, active item or selection
count, and the latest message. `:` opens command mode, `:help` opens the full
help view, and `:clear` closes displayed command output.

## Preview requirements

Install Poppler (`pdftotext`, `pdfinfo`, and `pdftocairo`) for extraction,
metadata, and PDF previews. Kitty and iTerm2 support inline first-page images;
other terminals can use optional `chafa` fallback rendering.
