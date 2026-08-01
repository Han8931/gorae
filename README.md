<p align="center">
  <img src="assets/gorae.svg" alt="Gorae logo" width="180">
</p>

<h1 align="center">Gorae</h1>

<p align="center">
  <em>Browse, search, annotate, and discuss your document library without leaving the terminal.</em>
</p>

<p align="center">
  <a href="https://github.com/Han8931/gorae/releases"><img alt="Release" src="https://img.shields.io/github/v/release/Han8931/gorae?sort=semver"></a>
  <a href="https://github.com/Han8931/gorae/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/github/license/Han8931/gorae"></a>
  <img alt="Go" src="https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go&logoColor=white">
  <a href="https://github.com/Han8931/gorae/stargazers"><img alt="Stars" src="https://img.shields.io/github/stars/Han8931/gorae?style=social"></a>
</p>

<p align="center">
  <img src="assets/gorae_final_demo.gif" alt="App Demo" width="800">
</p>

## What is Gorae?

Gorae is a small, keyboard-focused knowledge base for PDFs, EPUBs, and Markdown.
It combines file browsing, metadata, notes, full-text search, and an optional AI
assistant in one terminal interface.

Your documents stay as ordinary files. Metadata and the search index are stored
locally, and AI is optional: use Ollama on your machine or configure an
OpenAI-compatible provider.

Gorae may be a good fit if you prefer terminal tools, keep a folder-based paper
library, or want fast search and notes without running a larger desktop app.

## Quickstart

1. **Download** the binary for your platform from the [latest release](https://github.com/Han8931/gorae/releases):
   - **Linux:** `gorae-linux-amd64`
   - **macOS (Apple Silicon):** `gorae-darwin-arm64`
   - **macOS (Intel):** `gorae-darwin-amd64`
   - **Windows:** `gorae-windows-amd64.exe`
2. **Rename it to `gorae`, make it executable**, and move it onto your `PATH`:
   ```sh
   mv gorae-linux-amd64 gorae                 # use the file you downloaded
   mkdir -p ~/.local/bin
   chmod +x gorae && mv gorae ~/.local/bin/
   ```
3. **Run it:**
   ```sh
   gorae
   ```
4. **Index your library** so the AI assistant has something to read:
   ```
   :index
   ```
5. **Open the chat:**
   ```
   :gorae
   ```

> Prefer building from source or using a package manager? See [Install](#install) below.

## Highlights

| Area | What you get |
|---|---|
| **Browse** | Vim-style navigation, mouse support, reading states, favorites, and a to-read queue |
| **Search** | Fast FTS5 metadata and full-text search, including every matching passage and PDF page |
| **Preview** | Metadata, text, and first-page PDF previews in supported terminals |
| **Organize** | Markdown notes, hierarchical tags, `[[wikilinks]]`, backlinks, and BibTeX copy |
| **Import** | DOI and arXiv detection with metadata retrieval from Crossref and arXiv |
| **AI (optional)** | Grounded chat, focused papers, summaries, sessions, skills, and streaming responses |
| **Customize** | Accessible bundled themes, custom colors and glyphs, and a foldable tree pane |

Gorae is under active development. The interface and configuration may continue
to evolve, but existing config files are migrated when new top-level settings
are introduced.

## AI Chat (`:gorae`)

A built-in RAG chat assistant grounded in your indexed library. Responses stream in real time.

```
:gorae         open the chat
:index         build the FTS index first so the assistant has context
```

Features:

- **Library retrieval** — relevant indexed passages are included with each question.
- **Sessions** — conversations are auto-saved; resume with `/sessions`, fork with `/new`.
- **`/load <query>`** — a live fuzzy picker focuses one or more papers for follow-up questions.
- **Vim-style navigation mode** — `Esc` switches from typing to a message cursor: `j/k` between messages, `Space` to mark, `y` to yank one or many to the clipboard.
- **Tool calling** — with `enable_tools: true`, the model can invoke in-app actions like `save_markdown` to write summaries straight to `notes_dir`.
- **Reasoning display** — collapsible `<think>` blocks for DeepSeek-R1 / Qwen3 / QwQ.
- **Web search** — optional Brave / Tavily routing with a hybrid rules + LLM classifier.
- **Skills** — custom prompt templates as `.md` files become slash commands.

<details>
<summary><b>Minimal AI config</b> — drop into <code>config.json</code> via <code>:config</code></summary>

```jsonc
"ai": {
  "provider": "ollama",       // "openai" | "ollama" | "custom"
  "model": "llama3.2",
  "api_key": "",              // required for OpenAI / custom
  "base_url": "",             // override endpoint
  "top_k": 3,                 // chunks injected per query
  "system_prompt": "",
  "enable_tools": false       // let the model call in-app tools
}
```

| Field | Description |
|---|---|
| `provider` | `"openai"`, `"ollama"`, or `"custom"` |
| `model` | e.g. `gpt-4o`, `llama3.2`, `mistral` |
| `api_key` | Required for OpenAI / custom. Empty for Ollama. |
| `base_url` | Defaults: OpenAI → `https://api.openai.com/v1`, Ollama → `http://localhost:11434/v1` |
| `top_k` | Document chunks injected per query (default `3`) |
| `system_prompt` | Extra instruction prepended before RAG context |
| `enable_tools` | Allow tool calls (e.g. `save_markdown`). Default `false`. |

</details>

<details>
<summary><b>Provider examples</b> — Ollama, OpenAI, OpenAI-compatible</summary>

```json
// Ollama (local — no API key needed)
"ai": { "provider": "ollama", "model": "llama3.2" }

// OpenAI
"ai": { "provider": "openai", "model": "gpt-4o-mini", "api_key": "sk-..." }

// Any OpenAI-compatible endpoint (Together, Groq, OpenRouter, …)
"ai": {
  "provider": "custom",
  "base_url": "https://api.together.xyz/v1",
  "model":    "meta-llama/Llama-3-8b-chat-hf",
  "api_key":  "..."
}
```

Pull the local model first if you haven't: `ollama pull llama3.2`. Good picks for document Q&A: `llama3.2`, `mistral`, `gemma3`.

</details>

<details>
<summary><b>Keyboard reference</b> — insert mode + navigation mode</summary>

**Insert mode** (default — typing flows into the prompt):

| Key | Action |
|---|---|
| `Enter` | Send a message · load marked/current papers in `/load` |
| `↑` / `↓` | Browse input history · move through `/load` results |
| `Tab` | Autocomplete `/` commands · mark and advance in `/load` |
| `Ctrl+T` | Toggle reasoning display |
| `Esc` | Switch to navigation mode (or exit if chat is empty) |
| `Ctrl+C` | Exit chat |
| `Ctrl+P` / `Ctrl+N` · `PgUp` / `PgDn` · mouse wheel | Scroll chat |

Mouse input is enabled by default. Set `"enable_mouse": false` in `config.json` to disable it.

**Navigation mode** (press `Esc` to enter):

| Key | Action |
|---|---|
| `i`, `a` | Back to insert mode |
| `j` / `k` | Next / previous message |
| `h` / `l` | Jump to previous / next user message |
| `gg`, `G` | First / last message |
| `Space` | Toggle a mark on the current message |
| `y` | Yank current — or every marked — message to the clipboard |
| `c` | Clear all marks |
| `/`, `?`, `q` | Slash command · help · exit |

</details>

<details>
<summary><b>Slash commands</b></summary>

| Command | Description |
|---|---|
| `/load <query>` | Live fuzzy picker; mark papers with `Tab`, then load with `Enter` |
| `/unfocus` | Clear all focused papers (`/select` remains a legacy alias) |
| `/summarize` | Summarize the focused file and save to its note |
| `/sources` | Documents cited in the last answer |
| `/clear` | Clear the conversation |
| `/compact` | Summarize older messages to free up context window |
| `/export` | Save conversation to markdown in `notes_dir` |
| `/sessions` | Session picker — load or manage past conversations |
| `/new` | Start a fresh session (current stays saved) |
| `/skills` | Manage custom prompt templates |
| `/help` | Show in-chat help and all keybindings |

</details>

For deeper docs — tool-call internals, skill placeholders, session compaction, reasoning UI — see the **[Wiki](https://github.com/Han8931/gorae/wiki)** or press `/help` in chat.

## Browsing & search

Vim-style navigation everywhere. Cheat sheet:

| Action | Key |
|---|---|
| Move / enter / up | `j/k`, `l/h` (or arrow keys) |
| Select | `Space` |
| Toggle tree pane | `,n` |
| To-read queue | `t` |
| Reading state | `r` |
| Edit metadata | `ee` |
| Search | `/` |
| AI chat | `:gorae` |
| Index library | `:index` |
| Help | `:help` |

**Search flags** (after `/`):

- `-t <title>`, `-a <author>`, `-y <year>`, `-c <keyword>` (full-text), `--tag <t1,t2>`
- Examples: `/ -t transformer`, `/ -a "Yoshua Bengio"`, `/ -c attention`, `/ --tag llm,graph`

**Full-text index:**

```
:index            index all documents under the watch root
:index here       only the current directory
```

Files whose size hasn't changed are skipped, so re-indexing is fast. The index uses SQLite's porter tokenizer, so "running" also matches "run".

**Search results — every hit, not just one:** a content search lists each matching document with its **full hit count**, and the detail pane shows a snippet for every occurrence (page-numbered for PDFs). Within the selected document you can walk through hits and open the file right where the match is:

| Key | Action |
|---|---|
| `j` / `k` | Move between matching documents (result list) |
| `n` / `N` (or `Tab` / `Shift+Tab`) | Move the cursor between hits in the multi-hit box |
| `Enter` | Open the document at the selected hit's page |
| `PgUp` / `PgDn` | Page through the results list |
| `Esc` / `q` | Close results · `/` searches again |

Opening at a specific page works with viewers that support it (`zathura`, `sioyek`, `okular`, `evince`); others open at the first page.

**Tags:** comma-separated in the metadata editor, hierarchical (`ml/transformers` matches the `ml/` prefix). Browse all tags with `:tags`.

**Bidirectional links:** write `[[filename]]` in any Markdown file, run `:index`, and backlinks appear at the bottom of the info pane for every document that points to it.

### Themes and layout

Use `:theme <name>` to switch among bundled themes. After typing `:theme `,
press `Tab` for the next theme, `Shift+Tab` for the previous theme, and `Enter`
to apply the highlighted choice. `:theme list` shows every bundled theme, while
`:theme reload` reloads a custom `theme.toml`.

Press `,n` in normal mode to toggle the directory tree for the current session.
Set its startup behavior in `~/.config/gorae/config.json` (or the equivalent
`XDG_CONFIG_HOME` path):

```json
"show_tree": true
```

## Install

### Option A — Pre-built binary (recommended)

Covered in [Quickstart](#quickstart) above.

<details>
<summary><b>Option B — Build from source</b></summary>

#### Requirements

- **Go 1.25+**
- **Poppler CLI tools** (`pdftotext`, `pdfinfo`, `pdftocairo`)
- **Optional fallback preview:** `chafa` for non-Kitty / non-iTerm2 terminals

```sh
# macOS
brew install golang poppler

# Debian / Ubuntu
sudo apt install golang-go poppler-utils

# Arch
sudo pacman -S go poppler
```

#### Build & install

```sh
git clone https://github.com/Han8931/gorae.git
cd gorae
./install.sh                                       # → ~/.local/bin/gorae (Linux) or /usr/local/bin/gorae (macOS)
GORAE_INSTALL_PATH=/usr/local/bin/gorae ./install.sh   # custom path
```

> Linux + Kitty: with `poppler-utils` installed, Gorae uses native image-based PDF previews via `pdftocairo`. `chafa` is not required for the Kitty path.

</details>

<details>
<summary><b>Option C — Arch (AUR)</b></summary>

Two packages, pick whichever fits:

| Package | What it does |
|---|---|
| [`gorae`](https://aur.archlinux.org/packages/gorae) | builds from source via `go` (Arch-native) |
| [`gorae-bin`](https://aur.archlinux.org/packages/gorae-bin) | installs the pre-built x86_64 binary from the latest GitHub release |

**With an AUR helper** (e.g. [`yay`](https://github.com/Jguer/yay) or `paru`):

```sh
yay -S gorae       # builds from source via go (Arch-native)
yay -S gorae-bin   # pre-built x86_64 binary from the latest GitHub release
```

**Without an AUR helper** — clone and build with `makepkg`, which installs the package through `pacman`:

```sh
git clone https://aur.archlinux.org/gorae.git      # or gorae-bin.git
cd gorae
makepkg -si                                         # -s pulls build deps, -i installs via pacman
```

To update later, `git pull` in the clone and re-run `makepkg -si`.

Both pull in `poppler` automatically and suggest `chafa`, `zathura`, and `zathura-pdf-mupdf` as optional dependencies.

> Maintainer notes for keeping the PKGBUILDs in sync: see [`packaging/aur/README.md`](packaging/aur/README.md).

</details>

<details>
<summary><b>Recommended PDF viewer: Zathura</b></summary>

Any PDF viewer works, but [Zathura](https://pwmt.org/projects/zathura/) with the MuPDF backend is a great fit: minimal, keyboard-driven, instant start, vi-style navigation.

```sh
sudo apt install zathura zathura-pdf-mupdf     # Debian / Ubuntu
sudo pacman -S zathura zathura-pdf-mupdf       # Arch
```

```json
"pdf_viewer": "zathura"
```

Gorae auto-detects `zathura` on `PATH`, so the default works for most users.

</details>

## Uninstall

```sh
rm <path-to-installed-binary>
rm -rf ~/.config/gorae        # permanently removes config + custom theme
rm -rf ~/.local/share/gorae   # permanently removes metadata, notes, and database
```

## Roadmap

See [ROADMAP.md](ROADMAP.md) — what's shipped, what's in progress, and what's planned.

## Documentation

- [User guide](docs/user-guide.md) — configuration, themes, keybindings, search, and AI
- [Contributing](CONTRIBUTING.md) — development setup and pull requests
- [Roadmap](ROADMAP.md) — shipped and planned work

## Contributing

PRs, bug reports, and feature ideas are welcome. See
**[CONTRIBUTING.md](CONTRIBUTING.md)** for development setup and the pull-request
checklist. If Gorae is useful to you, a star helps other terminal users find it.

## License & credit

MIT. If you use Gorae in your project, documentation, or distribution, please credit **Gorae by Han** with a link to this repository.

> The Gorae logo is inspired by the **Bangudae Petroglyphs** (반구대 암각화) in Ulsan, South Korea — one of the earliest known depictions of whales and whale hunting.

## Acknowledgements

<table>
  <tr>
    <td align="center" width="170">
      <a href="https://github.com/fineday38">
        <img src="https://github.com/fineday38.png?size=120" width="50" height="50" alt="fineday38" style="border-radius:50%;" />
      </a>
      <br/>
      <a href="https://github.com/fineday38">fineday38</a>
    </td>
  </tr>
</table>
