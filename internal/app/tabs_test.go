package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Han8931/gorae/internal/meta"
)

func newTabTestModel(t *testing.T) Model {
	t.Helper()
	root := canonicalPath(t.TempDir())
	for _, d := range []string{favoritesDirName, toReadDirName, "papers"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "papers", "a.md"), []byte("# a"), 0o644); err != nil {
		t.Fatal(err)
	}
	fav := filepath.Join(root, favoritesDirName)
	toRead := filepath.Join(root, toReadDirName)
	m := Model{
		root: root, cwd: root, width: 120, viewportHeight: 20,
		selected:              map[string]bool{},
		entryTitles:           map[string]string{},
		favoritesDir:          fav,
		favoritesDirCanonical: fav,
		toReadDir:             toRead,
		toReadDirCanonical:    toRead,
	}
	m.loadEntries()
	return m
}

func TestLibraryHidesCollectionDirs(t *testing.T) {
	m := newTabTestModel(t)
	for _, e := range m.entries {
		if e.Name() == favoritesDirName || e.Name() == toReadDirName {
			t.Fatalf("collection dir %q listed in Library tab", e.Name())
		}
	}
	if len(m.entries) != 1 || m.entries[0].Name() != "papers" {
		t.Fatalf("unexpected library entries: %v", m.entries)
	}
}

func TestArrowsCycleTabsAndRestoreLibrary(t *testing.T) {
	m := newTabTestModel(t)
	// Descend into papers/ so we can check the Library position is restored.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = next.(Model)
	papers := filepath.Join(m.root, "papers")
	if m.cwd != papers {
		t.Fatalf("cwd = %q, want %q", m.cwd, papers)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	if m.activeTab() != 1 || m.cwd != m.toReadDir {
		t.Fatalf("right should open To Read, got tab %d cwd %q", m.activeTab(), m.cwd)
	}

	// Wraps around to Library and restores papers/.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	if m.activeTab() != 0 || m.cwd != papers {
		t.Fatalf("wrap should restore Library at %q, got tab %d cwd %q", papers, m.activeTab(), m.cwd)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = next.(Model)
	if m.cwd != m.toReadDir {
		t.Fatalf("left from Library should wrap to To Read, got %q", m.cwd)
	}

	// h at the top of a collection tab returns to the Library position.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = next.(Model)
	if m.activeTab() != 0 || m.cwd != papers {
		t.Fatalf("h should return to Library at %q, got %q", papers, m.cwd)
	}
}

func TestTabStripFitsAndCollapses(t *testing.T) {
	m := newTabTestModel(t)
	wide := m.renderTabStrip(80)
	if lipgloss.Width(wide) != 80 || !strings.Contains(wide, "To Read") {
		t.Fatalf("wide strip = %q", wide)
	}
	narrow := m.renderTabStrip(20)
	if w := lipgloss.Width(narrow); w != 20 {
		t.Fatalf("narrow strip width = %d, want 20", w)
	}
	if strings.Contains(wide, "Favorites") {
		t.Fatalf("Favorites was removed and should not be a tab: %q", wide)
	}
	if !strings.Contains(narrow, "Library") {
		t.Fatalf("narrow strip should keep the active tab, got %q", narrow)
	}
}

func TestReadingStateTabsFollowMetadata(t *testing.T) {
	m := newTabTestModel(t)
	store, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m.meta = store
	m.initReadingStateViews(t.TempDir())

	paper := canonicalPath(filepath.Join(m.root, "papers", "a.md"))
	if err := store.Upsert(context.Background(), &meta.Metadata{Path: paper, Title: "A", ReadingState: readingStateReading}); err != nil {
		t.Fatal(err)
	}

	tabs := m.listTabs()
	var labels []string
	for _, tab := range tabs {
		labels = append(labels, tab.label)
	}
	if got := strings.Join(labels, ","); got != "Library,Reading,To Read,Unread,Read" {
		t.Fatalf("tab order = %s", got)
	}

	m.switchToTab(1)
	if !m.cwdIsStateView || len(m.entries) != 1 {
		t.Fatalf("Reading tab should list the paper, got %d entries (view=%v)", len(m.entries), m.cwdIsStateView)
	}

	// r moves it Reading -> Read, so it leaves the Reading tab.
	m.cycleReadingState()
	if len(m.entries) != 0 {
		t.Fatalf("paper should leave the Reading tab after r, still %d entries", len(m.entries))
	}
	m.switchToTab(4)
	if len(m.entries) != 1 {
		t.Fatalf("paper should appear in the Read tab, got %d entries", len(m.entries))
	}
}
