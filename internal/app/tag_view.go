package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Han8931/gorae/internal/meta"
)

// reloadTreeTags refreshes the cached list of unique tags shown in the tree's
// Tags section. Cheap DISTINCT query; called at startup and whenever the tree
// gains focus so newly added tags appear.
func (m *Model) reloadTreeTags() {
	if m.meta == nil {
		m.treeTags = nil
		return
	}
	tags, err := m.meta.AllTags(context.Background())
	if err != nil {
		return
	}
	m.treeTags = tags
}

// filterByTag materializes a symlink directory of every document carrying tag
// and makes it the current directory, so the Files pane lists the matches. This
// reuses the same mechanism as the Favorites/Recently special folders, so
// preview, open, and metadata all work unchanged. Focus moves to the Files pane.
func (m *Model) filterByTag(tag string) tea.Cmd {
	tag = strings.TrimSpace(tag)
	if m.meta == nil || tag == "" {
		return nil
	}
	if m.tagViewDir == "" {
		m.setStatus("Tag view unavailable (no meta dir)")
		return nil
	}
	if err := rebuildTagDirectory(m.tagViewDir, tag, m.meta); err != nil {
		m.setStatus("Tag filter failed: " + err.Error())
		return nil
	}
	if !m.cwdIsTagView {
		m.tagReturnDir = m.cwd
	}
	m.cwd = m.tagViewDir
	m.loadEntries()
	m.cwdIsTagView = true
	m.activeTagFilter = tag
	m.cursor = 0
	m.viewportStart = 0
	m.ensureCursorVisible()
	m.focusedPane = focusFiles
	m.setStatus(fmt.Sprintf("Tag: %s (%d) — press h to return", tag, len(m.entries)))
	return m.updateTextPreviewAsync()
}

// leaveTagView dismisses an active tag filter and returns to the directory the
// user was in when they applied it.
func (m *Model) leaveTagView() tea.Cmd {
	target := m.tagReturnDir
	if target == "" {
		target = m.root
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		target = m.root
	}
	m.cwd = target
	m.cwdIsTagView = false
	m.activeTagFilter = ""
	m.tagReturnDir = ""
	m.loadEntries()
	m.cursor = 0
	m.viewportStart = 0
	m.ensureCursorVisible()
	m.clearStatus()
	return m.updateTextPreviewAsync()
}

// rebuildTagDirectory replaces the contents of dest with symlinks pointing to
// every document that has the given tag.
func rebuildTagDirectory(dest, tag string, store *meta.Store) error {
	if dest == "" || strings.TrimSpace(tag) == "" || store == nil {
		return nil
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		return err
	}

	list, err := store.ListByTag(context.Background(), tag)
	if err != nil {
		return err
	}

	// Drop any existing symlinks so stale matches don't linger.
	dirEntries, err := os.ReadDir(destAbs)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, entry := range dirEntries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		_ = os.Remove(filepath.Join(destAbs, entry.Name()))
	}

	desired := make(map[string]string)
	for _, md := range list {
		target := strings.TrimSpace(md.Path)
		if target == "" {
			continue
		}
		target = canonicalPath(target)
		if target == "" {
			continue
		}
		if _, err := os.Stat(target); err != nil {
			continue
		}
		linkName := mapBackedLinkName(filepath.Base(target), md.Title, md.Year, desired)
		desired[linkName] = target
	}

	for name, target := range desired {
		linkPath := filepath.Join(destAbs, name)
		relTarget, err := filepath.Rel(filepath.Dir(linkPath), target)
		if err != nil {
			relTarget = target
		}
		_ = os.Remove(linkPath)
		if err := os.Symlink(relTarget, linkPath); err != nil {
			return fmt.Errorf("creating tag link for %s: %w", target, err)
		}
	}
	return nil
}
