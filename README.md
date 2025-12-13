
```sh
go mod init gorae
go mod tidy
```

Build a binary

```sh
go build -o gorae
./gorae
./gorae -root ~/Documents/Papers
```



```sh
sudo pacman -S noto-fonts-emoji
```

```sh
sudo apt install fonts-noto-color-emoji
```

## Configuration

On first run the app writes `~/.config/gorae/config.json` (or `${XDG_CONFIG_HOME}/gorae/config.json`). Edit it via `:config edit` to tweak paths and behavior. Two useful keys:

- `editor`: command used when pressing `:config edit` or editing metadata
- `pdf_viewer`: command used to open PDFs. Provide the binary plus optional arguments; the PDF path is appended automatically. Quotes are supported and required if your command contains spaces, e.g. `"pdf_viewer": "\"C:\\\\Program Files\\\\SumatraPDF\\\\SumatraPDF.exe\""`
- `notes_dir`: directory where per-PDF note files are stored (defaults to `${meta_dir}/notes`). Files are regular text/Markdown so you can sync or back them up separately.

## Metadata

- Press `e` to preview metadata, `e` again to edit inline, or `v` to open the structured fields in your configured editor.
- Press `n` while in the metadata popup to open the note for the current file in your editor (notes are stored as Markdown files in `notes_dir`).
- Metadata fields include Title, Author, Journal/Conference, Year, Tag, and Abstract. Notes are stored separately.
- In the metadata popup use ↑/↓ or PgUp/PgDn to scroll through long content.
- Fetch fresh arXiv metadata with `:arxiv <arxiv-id> [files...]`; to avoid typing long filenames, select files beforehand (space or `v`) and run `:arxiv -v <arxiv-id>` to apply the ID to the selection. If you omit the ID entirely (e.g. `:arxiv -v`) the app first tries to extract IDs from each filename (e.g. `2101.12345v2` or `math.GT/0309136`); any files without detectable IDs fall back to an interactive prompt.

## Search

- Press `:` to enter command mode and run `:search <query>` to scan PDFs under the current directory. Matches are shown in the dedicated search view with highlighted snippets.
- Shortcut: press `/` in the main view to open the search prompt directly (no colon needed); type queries plus optional `-t`/`-a`/`-c`/`-y` flags and press Enter to run.
- After a search finishes the UI switches to a dedicated results view: use `j`/`k` (or the arrow keys) to move the selection, `PgUp`/`PgDn` to page, `Enter` to open the highlighted PDF, and `Esc` or `q` to return to the file browser.
- Use flags to customize the lookup:
  - `-mode title|author|year|content` (default `content`) or short forms `-t`, `-a`, `-y`, `-c`
  - `-case` for case-sensitive search
  - `-root PATH` to override the directory you want to scan (paths must stay within the watched directory; relative paths are resolved from the current directory)
- Shortcut syntax: start your query with `title:`, `author:`, `year:`, or `content:` to choose the search mode without flags (e.g. `/title:attention`).
- `:search` relies on Poppler’s `pdftotext` and `pdfinfo` utilities (the same package that powers previews). Make sure they’re installed so content/metadata extraction works.

TODO
- arxiv command with selections
- Yank bibtex / line style
- Bookmark / Favorite
- Page count
- Cursor position after going back to the parent dir
- UI improvement
- logo command
- Command autocomplete
- Screen renew or update key or auto

AI features:
- AI tag
- 
