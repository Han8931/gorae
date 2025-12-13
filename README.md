
## Install 

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

## Configuration

On first run the app writes `~/.config/gorae/config.json` (or `${XDG_CONFIG_HOME}/gorae/config.json`). Edit it via `:config` to tweak paths and behavior. Use `:config show` to print the current paths and `:config editor <cmd>` (e.g. `:config editor vim`) to change the editor without opening the JSON file. Two useful keys:

- `editor`: command used when pressing `:config`, editing metadata, or editing notes
- `pdf_viewer`: command used to open PDFs. Provide the binary plus optional arguments; the PDF path is appended automatically. Quotes are supported and required if your command contains spaces, e.g. `"pdf_viewer": "\"C:\\\\Program Files\\\\SumatraPDF\\\\SumatraPDF.exe\""`
- `notes_dir`: directory where per-PDF note files are stored (defaults to `${meta_dir}/notes`). Files are regular text/Markdown so you can sync or back them up separately.

## Metadata

- Press `e` to preview metadata, `e` again to edit inline, or `v` to open the structured fields in your configured editor.
- Press `n` while in the metadata popup to open the note for the current file in your editor (notes are stored as Markdown files in `notes_dir`).
- Press `y` on any PDF to copy a BibTeX entry for it to your clipboard (fields come from the stored metadata when available). The BibTeX always includes `published` and `url` keys, plus `doi` when present.
- Press `f` to toggle Favorite on the current/selected files, `t` to toggle To-read, and `u` to open a prompt that clears one or both flags.
- Metadata fields include Title, Author, Year, Published, URL, DOI, Tag, and Abstract. Notes are stored separately.
- In the metadata popup use ↑/↓ or PgUp/PgDn to scroll through long content.
- Fetch fresh arXiv metadata with `:arxiv <arxiv-id> [files...]`; to avoid typing long filenames, select files beforehand (space or `v`) and run `:arxiv -v <arxiv-id>` to apply the ID to the selection. If you omit the ID entirely (e.g. `:arxiv -v`) the app first tries to extract IDs from each filename (e.g. `2101.12345v2` or `math.GT/0309136`); any files without detectable IDs fall back to an interactive prompt. arXiv imports populate title, authors, year, abstract, and DOI when available.

## Search

- Press `:` to enter command mode and run `:search <query>` to scan PDFs under the current directory. Matches are shown in the dedicated search view with highlighted snippets.
- Shortcut: press `/` in the main view to open the search prompt directly (no colon needed); type queries plus optional `-t`/`-a`/`-c`/`-y` flags and press Enter to run.
- After a search finishes the UI switches to a dedicated results view: use `j`/`k` (or the arrow keys) to move the selection, `PgUp`/`PgDn` to page, `Enter` to open the highlighted PDF, and `Esc` or `q` to return to the file browser.
- Quick filters: press `F` to show favorites or `T` to show to-read items at any time; the interface switches to the search results view so you can browse and exit with `Esc`/`q`.
- Use flags to customize the lookup:
  - `-mode title|author|year|content` (default `content`) or short forms `-t`, `-a`, `-y`, `-c`
  - `-case` for case-sensitive search
  - `-root PATH` to override the directory you want to scan (paths must stay within the watched directory; relative paths are resolved from the current directory)
- Shortcut syntax: start your query with `title:`, `author:`, `year:`, or `content:` to choose the search mode without flags (e.g. `/title:attention`).
- `:search` relies on Poppler’s `pdftotext` and `pdfinfo` utilities (the same package that powers previews). Make sure they’re installed so content/metadata extraction works.

TODO
- Check whether read / unread
- Cursor position after going back to the parent dir
- UI improvement
- logo command
- Command autocomplete
- Command history by up/down arrows
- Screen renew or update key or auto
- Page count

AI features:
- [Text extraction, pymupdf4llm](https://pymupdf.readthedocs.io/en/latest/pymupdf4llm/)
- AI tag
- AI Summary
- Extract texts
- Knowledge Graphs
- RAG
- Prompt management
- 

## [Bangudae Petroglyphs](https://en.wikipedia.org/wiki/Bangudae_Petroglyphs)

[Pictures](https://www.khan.co.kr/article/202007080300025)
The world's earliest known depictions of whale hunting are found in the Bangudae Petroglyphs in South Korea, dating back around 7,000 years (6,000 BC), showcasing detailed scenes of boats and harpoons; however, similar ancient whale art is also found in the White Sea region (Russia/Scandinavia) and Norway, possibly as old, depicting complex hunts and spiritual meanings beyond simple prey, suggesting widespread ancient maritime cultures. 
