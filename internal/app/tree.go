package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// paneFocus selects which pane keyboard input drives in normal mode.
type paneFocus int

const (
	// focusFiles is the default: keys operate on the middle Files pane, exactly
	// as they always have.
	focusFiles paneFocus = iota
	// focusTree routes navigation keys to the left directory tree.
	focusTree
)

// treeNodeKind distinguishes the kinds of rows shown in the navigation tree.
type treeNodeKind int

const (
	treeNodeDir       treeNodeKind = iota // a directory
	treeNodeTagHeader                     // the "Tags" section label (not selectable)
	treeNodeTag                           // a tag; selecting it filters the Files pane
)

// treeNode is one visible row in the left navigation tree.
type treeNode struct {
	path     string
	name     string
	tag      string // full tag string, for treeNodeTag rows
	depth    int
	kind     treeNodeKind
	expanded bool
	hasKids  bool
	isRoot   bool
}

// isTreeExpanded reports whether dir is currently expanded. The root starts
// expanded so the library is visible on first paint; every other directory
// starts collapsed until the user opens it.
func (m Model) isTreeExpanded(dir string) bool {
	if v, ok := m.treeExpanded[dir]; ok {
		return v
	}
	return dir == m.root
}

// setTreeExpanded records the expanded/collapsed state for dir.
func (m *Model) setTreeExpanded(dir string, expanded bool) {
	if m.treeExpanded == nil {
		m.treeExpanded = make(map[string]bool)
	}
	m.treeExpanded[dir] = expanded
}

// readSubdirs returns the browsable subdirectory names of dir, ordered the same
// way the file list orders folders: special dirs first, then alphabetically.
// Dotfiles, the notes directory, and the collection directories reached through
// the Files pane tabs are hidden.
func (m Model) readSubdirs(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	notesAbs := ""
	if n := strings.TrimSpace(m.notesDir); n != "" {
		notesAbs = canonicalPath(n)
	}
	dirs := make([]string, 0, len(ents))
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		if notesAbs != "" && canonicalPath(full) == notesAbs {
			continue
		}
		// Collection directories are reached through the Files pane tabs.
		if m.isTabDir(full) {
			continue
		}
		dirs = append(dirs, e.Name())
	}
	sort.SliceStable(dirs, func(i, j int) bool {
		pi := m.specialDirPriority(filepath.Join(dir, dirs[i]))
		pj := m.specialDirPriority(filepath.Join(dir, dirs[j]))
		if pi != pj {
			return pi < pj
		}
		return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j])
	})
	return dirs
}

// visibleTreeNodes flattens the directory tree under root into the rows the
// tree pane displays, honoring each directory's expanded/collapsed state.
func (m Model) visibleTreeNodes() []treeNode {
	if strings.TrimSpace(m.root) == "" {
		return nil
	}
	rootExpanded := m.isTreeExpanded(m.root)
	nodes := []treeNode{{
		path:     m.root,
		name:     filepath.Base(m.root),
		depth:    0,
		kind:     treeNodeDir,
		expanded: rootExpanded,
		hasKids:  len(m.readSubdirs(m.root)) > 0,
		isRoot:   true,
	}}
	if rootExpanded {
		m.appendSubtree(&nodes, m.root, 1)
	}

	// Tags section: a labeled header followed by one row per unique tag. Tags are
	// cross-cutting labels, so they live in their own section below the folders.
	if len(m.treeTags) > 0 {
		nodes = append(nodes, treeNode{name: "Tags", depth: 0, kind: treeNodeTagHeader})
		for _, tag := range m.treeTags {
			nodes = append(nodes, treeNode{
				name:  tag,
				tag:   tag,
				depth: 1,
				kind:  treeNodeTag,
			})
		}
	}
	return nodes
}

// appendSubtree walks dir's subdirectories, appending a node for each and
// recursing into any that are expanded.
func (m Model) appendSubtree(nodes *[]treeNode, dir string, depth int) {
	for _, name := range m.readSubdirs(dir) {
		full := filepath.Join(dir, name)
		expanded := m.isTreeExpanded(full)
		kids := m.readSubdirs(full)
		*nodes = append(*nodes, treeNode{
			path:     full,
			name:     name,
			depth:    depth,
			kind:     treeNodeDir,
			expanded: expanded,
			hasKids:  len(kids) > 0,
		})
		if expanded && len(kids) > 0 {
			m.appendSubtree(nodes, full, depth+1)
		}
	}
}

// treeVisibleRows is the number of tree rows the pane body can show. The tree
// pane is untitled, so only the two border rows are subtracted.
func (m Model) treeVisibleRows() int {
	rows := m.viewportHeight - 2
	if rows < 1 {
		rows = 1
	}
	return rows
}

// ensureTreeCursorVisible clamps the tree cursor and scrolls the view window so
// the cursor stays on screen.
func (m *Model) ensureTreeCursorVisible() {
	nodes := m.visibleTreeNodes()
	if len(nodes) == 0 {
		m.treeCursor = 0
		m.treeViewStart = 0
		return
	}
	if m.treeCursor < 0 {
		m.treeCursor = 0
	}
	if m.treeCursor >= len(nodes) {
		m.treeCursor = len(nodes) - 1
	}
	rows := m.treeVisibleRows()
	if m.treeCursor < m.treeViewStart {
		m.treeViewStart = m.treeCursor
	}
	if m.treeCursor >= m.treeViewStart+rows {
		m.treeViewStart = m.treeCursor - rows + 1
	}
	if m.treeViewStart < 0 {
		m.treeViewStart = 0
	}
}

// treeParentIndex returns the index of the nearest preceding node whose depth is
// shallower than node i (its parent in the flattened list), or -1 if none.
func treeParentIndex(nodes []treeNode, i int) int {
	if i < 0 || i >= len(nodes) {
		return -1
	}
	depth := nodes[i].depth
	for j := i - 1; j >= 0; j-- {
		if nodes[j].depth < depth {
			return j
		}
	}
	return -1
}

// revealInTree expands every ancestor directory between root and target so that
// target becomes a visible tree row.
func (m *Model) revealInTree(target string) {
	if strings.TrimSpace(target) == "" || !strings.HasPrefix(target, m.root) {
		return
	}
	m.setTreeExpanded(m.root, true)
	dir := filepath.Dir(target)
	for strings.HasPrefix(dir, m.root) {
		m.setTreeExpanded(dir, true)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
}

// syncTreeCursorToCwd points the tree cursor at the node matching the current
// working directory (revealing it first), so focusing the tree lands on your
// current location.
func (m *Model) syncTreeCursorToCwd() {
	// Refresh the tag list so newly added tags show up whenever focus lands here.
	m.reloadTreeTags()
	m.revealInTree(m.cwd)
	nodes := m.visibleTreeNodes()
	for i, n := range nodes {
		if n.path == m.cwd {
			m.treeCursor = i
			m.ensureTreeCursorVisible()
			return
		}
	}
	m.ensureTreeCursorVisible()
}

// handleTreeKey processes a navigation key while the tree pane holds focus. It
// returns a command to run and whether the key was consumed; unconsumed keys
// fall through to the normal Files-pane handlers.
func (m *Model) handleTreeKey(key string) (tea.Cmd, bool) {
	nodes := m.visibleTreeNodes()
	if len(nodes) == 0 {
		return nil, false
	}
	if m.treeCursor >= len(nodes) {
		m.treeCursor = len(nodes) - 1
	}
	if m.treeCursor < 0 {
		m.treeCursor = 0
	}

	switch key {
	case "j", "down":
		if m.treeCursor < len(nodes)-1 {
			m.treeCursor++
			m.ensureTreeCursorVisible()
		}
		return nil, true

	case "k", "up":
		if m.treeCursor > 0 {
			m.treeCursor--
			m.ensureTreeCursorVisible()
		}
		return nil, true

	case "g", "home":
		m.treeCursor = 0
		m.ensureTreeCursorVisible()
		return nil, true

	case "G", "end":
		m.treeCursor = len(nodes) - 1
		m.ensureTreeCursorVisible()
		return nil, true

	case "l", "right":
		node := nodes[m.treeCursor]
		if node.kind != treeNodeDir {
			return nil, true
		}
		if node.hasKids && !node.expanded {
			m.setTreeExpanded(node.path, true)
			m.ensureTreeCursorVisible()
		} else if node.hasKids && node.expanded && m.treeCursor < len(nodes)-1 {
			// Already open: step into the first child.
			m.treeCursor++
			m.ensureTreeCursorVisible()
		}
		return nil, true

	case "h", "left":
		node := nodes[m.treeCursor]
		if node.kind == treeNodeDir && node.hasKids && node.expanded {
			m.setTreeExpanded(node.path, false)
			m.ensureTreeCursorVisible()
			return nil, true
		}
		if idx := treeParentIndex(nodes, m.treeCursor); idx >= 0 {
			m.treeCursor = idx
			m.ensureTreeCursorVisible()
		}
		return nil, true

	case "enter", "o":
		return m.activateTreeNode(m.treeCursor), true
	}

	return nil, false
}

// activateTreeNode performs the default action for the tree row at idx: opening
// a directory in the Files pane, or applying a tag filter. It is shared by the
// Enter key and mouse clicks. Either way, focus moves to the Files pane.
func (m *Model) activateTreeNode(idx int) tea.Cmd {
	nodes := m.visibleTreeNodes()
	if idx < 0 || idx >= len(nodes) {
		return nil
	}
	m.treeCursor = idx
	switch nodes[idx].kind {
	case treeNodeDir:
		return m.openTreeNode(idx)
	case treeNodeTag:
		return m.filterByTag(nodes[idx].tag)
	}
	return nil
}

// openTreeNode loads the directory at the given tree row into the Files pane and
// moves keyboard focus there, so the user can start browsing files immediately.
// It also reveals the directory's children in the tree. Shared by the Enter key
// and mouse clicks.
func (m *Model) openTreeNode(idx int) tea.Cmd {
	nodes := m.visibleTreeNodes()
	if idx < 0 || idx >= len(nodes) {
		return nil
	}
	node := nodes[idx]
	m.treeCursor = idx

	m.cwd = node.path
	m.loadEntries()
	m.cursor = 0
	m.viewportStart = 0
	m.ensureCursorVisible()

	if node.hasKids {
		m.setTreeExpanded(node.path, true)
	}
	m.ensureTreeCursorVisible()

	// Choosing a directory hands focus to the Files pane.
	m.focusedPane = focusFiles
	m.clearStatus()
	return m.updateTextPreviewAsync()
}
