package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// newTreeTestModel builds a Model rooted at a temp directory containing:
//
//	root/
//	  alpha/
//	    nested/
//	  beta/
func newTreeTestModel(t *testing.T) (Model, string) {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{
		filepath.Join(root, "alpha", "nested"),
		filepath.Join(root, "beta"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	return Model{
		root:           root,
		cwd:            root,
		viewportHeight: 20,
		treeExpanded:   make(map[string]bool),
	}, root
}

func TestVisibleTreeNodesRootExpandedByDefault(t *testing.T) {
	m, root := newTreeTestModel(t)
	nodes := m.visibleTreeNodes()

	// Root plus its two immediate subdirectories (nested stays hidden while
	// alpha is collapsed).
	if len(nodes) != 3 {
		t.Fatalf("visible nodes = %d, want 3 (%v)", len(nodes), nodeNames(nodes))
	}
	if !nodes[0].isRoot || nodes[0].path != root {
		t.Fatalf("first node = %+v, want root", nodes[0])
	}
	if nodes[1].name != "alpha" || nodes[2].name != "beta" {
		t.Fatalf("children = %v, want [alpha beta]", nodeNames(nodes))
	}
	if !nodes[1].hasKids {
		t.Fatal("alpha should report having children")
	}
	if nodes[2].hasKids {
		t.Fatal("beta should report no children")
	}
}

func TestTreeExpandRevealsNestedDir(t *testing.T) {
	m, root := newTreeTestModel(t)
	// Cursor on "alpha" (index 1), expand it with "l".
	m.treeCursor = 1
	if _, handled := m.handleTreeKey("l"); !handled {
		t.Fatal("expected l to be handled")
	}
	nodes := m.visibleTreeNodes()
	if len(nodes) != 4 {
		t.Fatalf("visible nodes after expand = %d, want 4 (%v)", len(nodes), nodeNames(nodes))
	}
	if nodes[2].name != "nested" || nodes[2].depth != 2 {
		t.Fatalf("node[2] = %+v, want nested at depth 2", nodes[2])
	}
	if !m.isTreeExpanded(filepath.Join(root, "alpha")) {
		t.Fatal("alpha should be recorded as expanded")
	}

	// Collapse again with "h".
	if _, handled := m.handleTreeKey("h"); !handled {
		t.Fatal("expected h to be handled")
	}
	if m.isTreeExpanded(filepath.Join(root, "alpha")) {
		t.Fatal("alpha should be collapsed after h")
	}
}

func TestTreeEnterOpensDirInFilesPane(t *testing.T) {
	m, root := newTreeTestModel(t)
	m.treeCursor = 2 // "beta"
	if _, handled := m.handleTreeKey("enter"); !handled {
		t.Fatal("expected enter to be handled")
	}
	if want := filepath.Join(root, "beta"); m.cwd != want {
		t.Fatalf("cwd = %q, want %q", m.cwd, want)
	}
}

func TestShowingTreePaneFocusesIt(t *testing.T) {
	m, _ := newTreeTestModel(t)
	m.treePaneHidden = true
	m.focusedPane = focusFiles

	m.handleNavigationPrefix(",")
	m.handleNavigationPrefix("n")

	if m.treePaneHidden {
		t.Fatal("expected ,n to show the tree pane")
	}
	if m.focusedPane != focusTree {
		t.Fatal("expected focus to move to the tree when shown")
	}

	// Hiding it again returns focus to the Files pane.
	m.handleNavigationPrefix(",")
	m.handleNavigationPrefix("n")
	if m.focusedPane != focusFiles {
		t.Fatal("expected focus to return to files when tree hidden")
	}
}

func TestTreeFocusRoutesArrowKeysThroughUpdate(t *testing.T) {
	m, _ := newTreeTestModel(t)
	m.focusedPane = focusTree // tree visible + focused

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	nm := updated.(Model)
	if nm.treeCursor != 1 {
		t.Fatalf("treeCursor = %d, want 1 after j", nm.treeCursor)
	}

	updated, _ = nm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	nm = updated.(Model)
	if nm.treeCursor != 0 {
		t.Fatalf("treeCursor = %d, want 0 after k", nm.treeCursor)
	}
}

func TestTagsAppearBelowDirectories(t *testing.T) {
	m, _ := newTreeTestModel(t)
	m.treeTags = []string{"ml/cnn", "nlp"}

	nodes := m.visibleTreeNodes()
	// root, alpha, beta, [Tags header], ml/cnn, nlp
	if len(nodes) != 6 {
		t.Fatalf("nodes = %d, want 6 (%v)", len(nodes), nodeNames(nodes))
	}
	header := nodes[3]
	if header.kind != treeNodeTagHeader {
		t.Fatalf("nodes[3] kind = %v, want tag header", header.kind)
	}
	if nodes[4].kind != treeNodeTag || nodes[4].tag != "ml/cnn" {
		t.Fatalf("nodes[4] = %+v, want tag ml/cnn", nodes[4])
	}
	if nodes[5].tag != "nlp" {
		t.Fatalf("nodes[5].tag = %q, want nlp", nodes[5].tag)
	}
}

func TestWindowPrefixSwitchesFocus(t *testing.T) {
	m, _ := newTreeTestModel(t)
	m.focusedPane = focusFiles

	// <C-w> then h focuses the tree.
	if !m.handleWindowPrefix("ctrl+w") {
		t.Fatal("expected ctrl+w to start the window prefix")
	}
	if !m.handleWindowPrefix("h") {
		t.Fatal("expected h to be consumed by the window prefix")
	}
	if m.focusedPane != focusTree {
		t.Fatal("expected C-w h to focus the tree")
	}

	// <C-w> then l returns focus to the files.
	m.handleWindowPrefix("ctrl+w")
	m.handleWindowPrefix("l")
	if m.focusedPane != focusFiles {
		t.Fatal("expected C-w l to focus the files")
	}
}

// With the tree focused the Files-pane cursor is not even highlighted, so keys
// that act on it must not reach it — pressing D must not arm a delete for a row
// the user cannot see.
func TestTreeFocusSwallowsFilesPaneKeys(t *testing.T) {
	m, root := newTreeTestModel(t)
	if err := os.WriteFile(filepath.Join(root, "paper.pdf"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	m.loadEntries()
	if len(m.entries) == 0 {
		t.Fatal("expected the Files pane to have an entry to delete")
	}
	m.focusedPane = focusTree

	for _, key := range []string{"D", "R", "a", "d", "p", " "} {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		nm := updated.(Model)
		if nm.state != stateNormal {
			t.Fatalf("key %q changed state to %v while the tree was focused", key, nm.state)
		}
		if len(nm.confirmItems) != 0 {
			t.Fatalf("key %q armed confirmItems = %v while the tree was focused", key, nm.confirmItems)
		}
		if len(nm.selected) != 0 || len(nm.cut) != 0 {
			t.Fatalf("key %q mutated the Files-pane selection while the tree was focused", key)
		}
	}
}

// Pane-independent keys keep working, so the tree never traps the user.
func TestTreeFocusAllowsPaneIndependentKeys(t *testing.T) {
	m, _ := newTreeTestModel(t)
	m.focusedPane = focusTree
	m.input = textinput.New() // ':' focuses the command input

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	if nm := updated.(Model); nm.state != stateCommand {
		t.Fatalf("state = %v after ':', want stateCommand", nm.state)
	}
}

// Right-click in the Files pane backs out of a tag view the same way "h" does;
// a tag view has no parent directory to climb to.
func TestGoToParentDirLeavesTagView(t *testing.T) {
	m, root := newTreeTestModel(t)
	tagView := filepath.Join(root, ".tagview")
	if err := os.MkdirAll(tagView, 0o755); err != nil {
		t.Fatalf("mkdir tag view: %v", err)
	}
	m.cwd = tagView
	m.tagViewDir = tagView
	m.cwdIsTagView = true
	m.activeTagFilter = "ml"
	m.tagReturnDir = filepath.Join(root, "alpha")

	m.goToParentDir()

	if m.cwdIsTagView {
		t.Fatal("expected the tag filter to be dismissed")
	}
	if want := filepath.Join(root, "alpha"); m.cwd != want {
		t.Fatalf("cwd = %q, want %q", m.cwd, want)
	}
	if m.activeTagFilter != "" {
		t.Fatalf("activeTagFilter = %q, want cleared", m.activeTagFilter)
	}
}

func nodeNames(nodes []treeNode) []string {
	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.name
	}
	return names
}
