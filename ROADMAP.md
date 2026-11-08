# Roadmap

A snapshot of what's shipped, what's in flight, and what's next. PRs welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## Shipped

### AI assistant

- [x] **AI chat sessions** — persist and resume conversations; `/sessions`, `/new`, `/compact`, `/export`
- [x] **Skills** — user-defined prompt templates as `.md` files, invoked as slash commands
- [x] **`/summarize`** — streams a summary and saves to the file's note
- [x] **Reasoning display** — collapsible `<think>` blocks (`Ctrl+T`) for DeepSeek-R1, Qwen3, QwQ
- [x] **Live `/load` picker** — fuzzy file finder with multi-file marking
- [x] **Vim-style navigation mode** — `Esc`/`i`, `hjkl`, `gg`/`G`, marks (`Space`), yank (`y`)
- [x] **Tool calling** — model can invoke in-app actions like `save_markdown`
- [x] **Mouse-wheel scrolling** in chat
- [x] **Web search integration** — Brave / Tavily with rules + LLM routing
- [x] **Semantic search** — `vector_search: true` embeds chunks at `:index` time; retrieval fuses keyword and semantic rankings (RRF)

### Knowledge base

- [x] FTS5 full-text search with stemming
- [x] Hierarchical tags
- [x] Bidirectional `[[wikilinks]]` with backlinks
- [x] Auto DOI / arXiv metadata + BibTeX copy
- [x] Delete confirmation
- [x] Colorful circles next to tags in the tree pane

## In progress

- [ ] **Citation network** — auto-fetch citation relationships from DOIs via Semantic Scholar

## Planned

### AI assistant

- [ ] Scale semantic search — `SearchSemantic` scores every stored chunk per query; needs an ANN index or a candidate prefilter for large libraries
- [ ] Re-embed on `embedding_model` change — vectors of the old width are skipped, which silently drops to keyword-only until `:index` reruns
- [ ] Daily digest — `:digest` command summarising recent additions + matching arXiv papers
- [ ] Session renaming
- [ ] Text extraction / OCR
- [ ] TTS

### Knowledge base & UX

- [ ] Rethink the full paper list — organize it by tags or some other grouping
- [ ] Open URL
- [ ] To-do management
- [ ] Vault warden for cloud support
- [ ] Web server
- [~] Trash — `D` moves deletions to Trash (metadata snapshotted); `:trash restore [name]` puts items back with their tags and notes; `:trash empty` clears it. Still to come: a way to browse the Trash (listing / tree node) so restorable names are discoverable.

### Fixes

- [ ] Metadata view improvements
