<p align="center">
  <img src="assets/gorae.svg" alt="Gorae logo" width="180">
</p>

<h1 align="center">Gorae</h1>

<p align="center">
  <em>A terminal-first knowledge base for PDFs, EPUBs, and Markdown — with a built-in AI assistant that talks to your library.</em>
</p>

<p align="center">
  <a href="https://github.com/Han8931/gorae/releases"><img alt="Release" src="https://img.shields.io/github/v/release/Han8931/gorae?sort=semver"></a>
  <a href="https://github.com/Han8931/gorae/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/github/license/Han8931/gorae"></a>
  <img alt="Go" src="https://img.shields.io/badge/go-1.21%2B-00ADD8?logo=go&logoColor=white">
  <a href="https://github.com/Han8931/gorae/stargazers"><img alt="Stars" src="https://img.shields.io/github/stars/Han8931/gorae?style=social"></a>
</p>

<p align="center">
  <img src="assets/gorae_final_demo.gif" alt="App Demo" width="800">
</p>

## Why Gorae

- **Lives in your terminal.** Vim-style browsing, mouse support, no Electron, no browser tab.
- **AI assistant that knows your library.** Chat, summarize, and RAG-Q&A directly against your indexed documents.
- **Open and local-first.** Your files, your metadata store, your choice of model (Ollama, OpenAI, anything OpenAI-compatible).

Like Obsidian + Zotero, but in your terminal — and Vim-friendly.

## Quickstart

1. **Download** the [latest release](https://github.com/Han8931/gorae/releases) for your platform — `gorae` (Linux), `gorae-darwin-arm64` (macOS Apple Silicon), `gorae-darwin-amd64` (macOS Intel), or `gorae-windows-amd64.exe`.
2. **Make it executable** and move it onto your `PATH`:
   ```sh
   chmod +x gorae && mv gorae ~/.local/bin/   # or /usr/local/bin
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

## Features at a glance

| Area | What you get |
|---|---|
| **Browsing** | Vim-style nav, to-read queue, reading states, hierarchical tags (`ml/cnn`) |
| **Search** | FTS5 full-text index with stemming; scope by `-t/-a/-y/-c/--tag` |
| **Preview** | First-page PDF previews on Kitty / iTerm2; `chafa` fallback elsewhere |
| **Metadata** | Auto-detect DOI / arXiv IDs, in-app editor, BibTeX copy |
| **Links** | `[[wikilinks]]` in Markdown with auto backlinks |
| **AI chat** | RAG against your library, streaming responses, sessions, skills |
| **AI tools** | Model can invoke in-app actions like `save_markdown` (optional) |
| **UI** | Themeable colors, glyphs, borders; mouse + Vim modal navigation |

## AI Chat (`:gorae`)

A built-in RAG chat assistant grounded in your indexed library. Responses stream in real time.

```
:gorae         open the chat
:index         build the FTS index first so the assistant has context
```

**Headline features:**

- **RAG out of the box** — relevant chunks are retrieved from your library and injected into every query.
- **Sessions** — conversations are auto-saved; resume with `/sessions`, fork with `/new`.
- **`/load <query>`** — fzf-style live picker pins a file as primary context for follow-up questions.
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
| `Enter` | Send message / select file in `/load` picker |
| `↑` / `↓` | Browse input history · navigate `/load` results |
| `Tab` | Autocomplete `/` command |
| `Ctrl+T` | Toggle reasoning display |
| `Esc` | Switch to navigation mode (or exit if chat is empty) |
| `Ctrl+C` | Exit chat |
| `Ctrl+P` / `Ctrl+N` · `PgUp` / `PgDn` · mouse wheel | Scroll chat |

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
| `/load <query>` | Live fzf-style file picker; pin a file as focused context |
| `/select` | Clear the currently focused file |
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

**Tags:** comma-separated in the metadata editor, hierarchical (`ml/transformers` matches the `ml/` prefix). Browse all tags with `:tags`.

**Bidirectional links:** write `[[filename]]` in any Markdown file, run `:index`, and backlinks appear at the bottom of the info pane for every document that points to it.

## Install

### Option A — Pre-built binary (recommended)

Covered in [Quickstart](#quickstart) above.

<details>
<summary><b>Option B — Build from source</b></summary>

#### Requirements

- **Go 1.21+**
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

`yay -S gorae` currently installs an **older version** and is out of sync with `main`. I plan to refresh the AUR package — until then, use Option A or B.

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
rm -rf ~/.config/gorae        # config + theme
rm -rf ~/.local/share/gorae   # metadata, notes, db
```

## Roadmap

See [ROADMAP.md](ROADMAP.md) — what's shipped, what's in progress, and what's planned.

## Contributing

PRs, bug reports, and feature ideas welcome. See **[CONTRIBUTING.md](CONTRIBUTING.md)** for dev setup, conventions, and the PR checklist.

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
