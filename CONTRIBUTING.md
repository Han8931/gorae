# Contributing to Gorae

Thanks for helping improve **Gorae**. PRs, bug reports, and feature ideas are all welcome.

## Quick start

```sh
git clone https://github.com/<your-username>/gorae.git
cd gorae
go build -o gorae ./cmd/gorae
./gorae
```

Prereqs: **Go 1.25+** and Poppler (`pdftotext`, `pdfinfo`, `pdftocairo`). See [README → Install](README.md#install) for OS-specific commands.

## Project layout

```
cmd/gorae       # entrypoint
internal/ai     # AI/RAG chat + providers (OpenAI, Ollama, custom)
internal/app    # application wiring
internal/ui     # Bubble Tea TUI
internal/meta   # metadata store, FTS5 index, tags, links
internal/arxiv  # arXiv import
internal/crossref # DOI lookup
internal/config # config loader (jsonc)
```

## Before sending a PR

- [ ] `go build ./...` and `go test ./...` pass
- [ ] `gofmt -w .` and `go vet ./...` are clean
- [ ] Tested manually in a terminal (Kitty + one other for UI changes)
- [ ] README / `docs/user-guide.md` updated for user-visible changes
- [ ] PR describes **what**, **why**, and **how to test**

Keep PRs focused — bug fixes and refactors should be separate.

## Style

- **Go:** stdlib idioms, `gofmt`-clean, wrap errors with `fmt.Errorf("…: %w", err)`.
- **TUI:** follow Bubble Tea's `Init / Update / View` pattern.
- **Comments:** explain *why*, not *what*.
- **No premature abstractions** — three similar lines is fine.
- **Dependencies:** prefer the standard library; justify any new module in the PR.

## Commits

Short, imperative, ~50–72 chars (`Add X`, `Fix Y`). Match the existing `git log` voice.

## Bug reports

Include: Gorae version/commit, OS + terminal + Go version, repro steps, expected vs. actual, and logs or an `asciinema` clip for TUI glitches.

## Feature requests

Check the [Roadmap](README.md#roadmap) and open issues first. For larger ideas, open an issue before coding so we can align on direction.

## License

By contributing, you agree your contributions are licensed under the [MIT License](LICENSE).
