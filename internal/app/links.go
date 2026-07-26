package app

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Han8931/gorae/internal/meta"
)

var wikilinkPattern = regexp.MustCompile(`\[\[([^\[\]\n]+)\]\]`)

type linksScannedMsg struct {
	source string
	count  int
	err    error
}

// parseWikilinks extracts all [[link text]] targets from markdown text.
func parseWikilinks(text string) []string {
	matches := wikilinkPattern.FindAllStringSubmatch(text, -1)
	seen := make(map[string]struct{}, len(matches))
	var links []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		name := strings.TrimSpace(m[1])
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		links = append(links, name)
	}
	return links
}

// resolveWikilink walks root looking for a file whose base name (without extension)
// or full name matches the given wikilink name case-insensitively.
// Returns the canonical path, or empty string if not found.
func resolveWikilink(name, root string) string {
	if name == "" || root == "" {
		return ""
	}
	nameLower := strings.ToLower(name)
	var found string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		baseName := d.Name()
		baseNoExt := strings.ToLower(strings.TrimSuffix(baseName, filepath.Ext(baseName)))
		if baseNoExt == nameLower || strings.ToLower(baseName) == nameLower {
			if c := canonicalPath(path); c != "" {
				found = c
			} else {
				found = filepath.Clean(path)
			}
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// extractWikilinks parses wikilinks from text and resolves them against root.
// source is the canonical path of the document containing the links.
func extractWikilinks(text, source, root string) []meta.Link {
	names := parseWikilinks(text)
	links := make([]meta.Link, 0, len(names))
	for _, name := range names {
		target := resolveWikilink(name, root)
		if target != "" && target != source {
			links = append(links, meta.Link{Target: target, LinkText: name})
		}
	}
	return links
}

// scanLinksCmd parses [[wikilinks]] in a markdown file and stores resolved outlinks.
func scanLinksCmd(sourcePath, root string, store *meta.Store) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return linksScannedMsg{source: sourcePath, err: err}
		}

		canonical := canonicalPath(sourcePath)
		if canonical == "" {
			canonical = filepath.Clean(sourcePath)
		}

		names := parseWikilinks(string(data))
		links := make([]meta.Link, 0, len(names))
		for _, name := range names {
			target := resolveWikilink(name, root)
			if target != "" && target != canonical {
				links = append(links, meta.Link{Target: target, LinkText: name})
			}
		}

		ctx := context.Background()
		if err := store.SetOutlinks(ctx, canonical, links); err != nil {
			return linksScannedMsg{source: sourcePath, err: err}
		}
		return linksScannedMsg{source: sourcePath, count: len(links)}
	}
}
