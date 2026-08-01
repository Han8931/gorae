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
| `show_tree` | Show the directory tree at startup; default `true` |
| `enable_mouse` | Enable mouse input and scrolling |
| `text_preview_only` | Disable inline image previews |
| `recent_days` | Age window for Recently Added |
| `recently_opened_limit` | Maximum entries in Recently Read |

The `ai` and `web_search` objects configure providers, models, retrieval, tool
calling, and optional web search. The generated config documents every field.

Gorae maintains `Favorites/`, `To Read/`, `Recently Added/`, and
`Recently Read/` helper directories beneath the library. Back up `meta_dir` and
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
| `j` / `k` or arrows | Move down/up |
| `l`, `Right`, or `Enter` | Enter a directory or open a document |
| `h`, `Left`, or `Backspace` | Parent directory |
| `g` / `G` | Top/bottom; `g` also begins a reading-state filter |
| `,n` | Toggle the tree pane for this session |
| `Space` | Toggle selection and advance |
| `v` | Select/clear all PDF files |
| `a` | Create a directory |
| `R` | Rename |
| `D` | Delete with confirmation |
| `d` / `p` | Cut/paste |
| `st` / `sy` | Sort by title/year |
| `q` or `Ctrl+C` | Quit |

Set `"show_tree": false` to start with the tree folded. When folded, the list
and detail panes share the reclaimed width.

## Metadata, notes, and flags

- `e` opens metadata preview. Press `e` again to edit metadata in the
  configured external editor.
- `n` edits the current document's Markdown note.
- `f` toggles Favorite.
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
