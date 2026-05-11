# Roadmap

A snapshot of what's shipped, what's in flight, and what's next. PRs welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## Shipped

### AI assistant

- [x] **AI chat sessions** — persist and resume conversations; `/sessions`, `/new`, `/compact`, `/export`
- [x] **Skills** — user-defined prompt templates as `.md` files, invoked as slash commands
- [x] **`/summarize`** — streams a summary and saves to the file's note
- [x] **Reasoning display** — collapsible `<think>` blocks (`Ctrl+T`) for DeepSeek-R1, Qwen3, QwQ
- [x] **Live `/load` picker** — fzf-style file finder, results update as you type
- [x] **Vim-style navigation mode** — `Esc`/`i`, `hjkl`, `gg`/`G`, marks (`Space`), yank (`y`)
- [x] **Tool calling** — model can invoke in-app actions like `save_markdown`
- [x] **Mouse-wheel scrolling** in chat
- [x] **Web search integration** — Brave / Tavily with rules + LLM routing

### Knowledge base

- [x] FTS5 full-text search with stemming
- [x] Hierarchical tags
- [x] Bidirectional `[[wikilinks]]` with backlinks
- [x] Auto DOI / arXiv metadata + BibTeX copy

## In progress

- [ ] **Citation network** — auto-fetch citation relationships from DOIs via Semantic Scholar

## Planned

### AI assistant

- [ ] Semantic search in Gorae mode
- [ ] Daily digest — `:digest` command summarising recent additions + matching arXiv papers
- [ ] Multi-file handling
- [ ] Session renaming
- [ ] Text extraction / OCR
- [ ] TTS

### Knowledge base & UX

- [ ] Open URL
- [ ] To-do management
- [ ] Vault warden for cloud support
- [ ] Web server
- [ ] Trash

### Fixes

- [ ] Delete-confirmation prompt
- [ ] Metadata view improvements
