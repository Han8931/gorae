package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"gorae/internal/ai"
	"gorae/internal/config"
	"gorae/internal/meta"
)

type indexCompleteMsg struct {
	indexed  int
	skipped  int
	failed   int
	warnings []string
}

// extractDocumentText extracts plain text from a PDF, EPUB, or Markdown file.
func extractDocumentText(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return readPDFText(path)
	case ".epub":
		return readEPUBText(path)
	case ".md", ".markdown", ".mdown", ".mkd", ".mkdn":
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	default:
		return "", fmt.Errorf("unsupported file type: %s", filepath.Ext(path))
	}
}

// indexAllCmd walks root and indexes all documents whose content has changed.
func indexAllCmd(root string, store *meta.Store, skipDirs []string, cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		files, warnings, err := collectDocumentFiles(root, skipDirs)
		if err != nil {
			return indexCompleteMsg{warnings: append(warnings, err.Error())}
		}

		ctx := context.Background()
		indexed := 0
		skipped := 0
		failed := 0

		// Set up embedding client if vector search is enabled.
		var embClient *ai.Client
		var embModel string
		if cfg != nil && cfg.AI != nil && cfg.AI.VectorSearch {
			if c, err2 := ai.NewClient(cfg.AI); err2 == nil {
				embClient = c
				embModel = strings.TrimSpace(cfg.AI.EmbeddingModel)
				if embModel == "" {
					embModel = "nomic-embed-text"
				}
			} else {
				warnings = append(warnings, "[WARN] vector search enabled but AI client failed: "+err2.Error())
			}
		}

		for _, path := range files {
			canonical := canonicalPath(path)
			if canonical == "" {
				canonical = filepath.Clean(path)
			}

			info, err := os.Stat(path)
			if err != nil {
				failed++
				warnings = append(warnings, fmt.Sprintf("[WARN] stat %s: %v", filepath.Base(path), err))
				continue
			}

			ftsState, _ := store.GetIndexState(ctx, canonical)
			ftsUpToDate := ftsState != nil && ftsState.FileSize == info.Size()

			vecState, _ := store.GetVecIndexState(ctx, canonical)
			vecUpToDate := embClient == nil || (vecState != nil && vecState.FileSize == info.Size())

			if ftsUpToDate && vecUpToDate {
				skipped++
				continue
			}

			text, err := extractDocumentText(path)
			if err != nil {
				failed++
				warnings = append(warnings, fmt.Sprintf("[WARN] extract %s: %v", filepath.Base(path), err))
				continue
			}

			if !ftsUpToDate {
				if err := store.IndexContent(ctx, canonical, text, info.Size()); err != nil {
					failed++
					warnings = append(warnings, fmt.Sprintf("[WARN] index %s: %v", filepath.Base(path), err))
					continue
				}
				if isMarkdown(path) {
					links := extractWikilinks(text, canonical, root)
					_ = store.SetOutlinks(ctx, canonical, links)
				}
			}

			if !vecUpToDate && embClient != nil {
				chunks := meta.ChunkText(text, 400, 50)
				vecs := make([][]float32, 0, len(chunks))
				embFailed := false
				for _, chunk := range chunks {
					vec, err2 := embClient.GetEmbedding(ctx, embModel, chunk)
					if err2 != nil {
						warnings = append(warnings, fmt.Sprintf("[WARN] embed %s: %v", filepath.Base(path), err2))
						embFailed = true
						break
					}
					vecs = append(vecs, vec)
				}
				if !embFailed {
					if err2 := store.StoreEmbeddings(ctx, canonical, chunks, vecs, info.Size()); err2 != nil {
						warnings = append(warnings, fmt.Sprintf("[WARN] store embeddings %s: %v", filepath.Base(path), err2))
					}
				}
			}

			indexed++
		}

		return indexCompleteMsg{
			indexed:  indexed,
			skipped:  skipped,
			failed:   failed,
			warnings: warnings,
		}
	}
}
