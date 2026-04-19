package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

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
func indexAllCmd(root string, store *meta.Store, skipDirs []string) tea.Cmd {
	return func() tea.Msg {
		files, warnings, err := collectDocumentFiles(root, skipDirs)
		if err != nil {
			return indexCompleteMsg{warnings: append(warnings, err.Error())}
		}

		ctx := context.Background()
		indexed := 0
		skipped := 0
		failed := 0

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

			state, err := store.GetIndexState(ctx, canonical)
			if err == nil && state != nil && state.FileSize == info.Size() {
				skipped++
				continue
			}

			text, err := extractDocumentText(path)
			if err != nil {
				failed++
				warnings = append(warnings, fmt.Sprintf("[WARN] extract %s: %v", filepath.Base(path), err))
				continue
			}

			if err := store.IndexContent(ctx, canonical, text, info.Size()); err != nil {
				failed++
				warnings = append(warnings, fmt.Sprintf("[WARN] index %s: %v", filepath.Base(path), err))
				continue
			}

			// For markdown files, also update the outbound wikilinks.
			if isMarkdown(path) {
				links := extractWikilinks(text, canonical, root)
				_ = store.SetOutlinks(ctx, canonical, links)
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
