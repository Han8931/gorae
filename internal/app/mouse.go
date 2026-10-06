package app

import (
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// handleMouse implements basic mouse interactions:
// - Scroll wheel: scroll list / search results / AI chat.
// - Left click: move cursor; double-click (or click) opens file/dir.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// The launch splash has no mouse targets; ignore clicks and wheel so they
	// don't quietly move the file browser hidden behind it.
	if m.state == stateLaunch {
		return m, nil
	}
	if m.state == stateGorae {
		switch msg.Type {
		case tea.MouseWheelUp:
			m.scrollAIChatBy(-3)
		case tea.MouseWheelDown:
			m.scrollAIChatBy(3)
		}
		return m, nil
	}
	switch msg.Type {
	case tea.MouseWheelUp:
		return m.scrollMouse(-1)
	case tea.MouseWheelDown:
		return m.scrollMouse(1)
	case tea.MouseLeft:
		return m.clickMouse(msg)
	case tea.MouseRight:
		return m.clickMouse(msg)
	default:
		return m, nil
	}
}

func (m Model) scrollMouse(delta int) (Model, tea.Cmd) {
	if m.state == stateSearchResults {
		if delta < 0 {
			m.moveSearchCursor(-1)
		} else {
			m.moveSearchCursor(1)
		}
		return m, nil
	}
	// normal list
	step := delta
	if step == 0 {
		return m, nil
	}
	m.cursor += step
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.entries) {
		m.cursor = len(m.entries) - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
	}
	m.ensureCursorVisible()
	return m, m.updateTextPreviewAsync()
}

func (m Model) clickMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	// We only handle clicks on the middle list panel when in normal state.
	// Use viewportStart and listVisibleRows to hit-test rows.
	// Adjust X/Y if you add panel-aware hit tests later.
	if m.state == stateSearchResults {
		row := m.searchResultOffset + msg.Y
		if row >= 0 && row < len(m.searchResults) {
			m.searchResultCursor = row
			m.searchHitCursor = 0
			m.ensureSearchResultVisible()
			// double-click to open search result
			now := time.Now()
			if row == m.lastClickSearchRow && now.Sub(m.lastClickSearchAt) < 500*time.Millisecond {
				m.openSearchResultAtCursor()
				m.lastClickSearchRow = -1
				m.lastClickSearchAt = time.Time{}
			} else {
				m.lastClickSearchRow = row
				m.lastClickSearchAt = now
			}
		}
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft {
		if msg.Button == tea.MouseButtonRight {
			if _, ok := m.clickInListPanel(msg); ok {
				return m, m.goToParentDir()
			}
		}
		return m, nil
	}
	// A click in the tree pane activates that row: opening a directory in the
	// Files pane or applying a tag filter. Either way focus moves to the files.
	if idx, ok := m.clickInTreePanel(msg); ok {
		cmd := m.activateTreeNode(idx)
		return m, cmd
	}

	localY, ok := m.clickInListPanel(msg)
	if !ok {
		return m, nil
	}
	row := m.hitTestListRow(localY)
	if row < 0 {
		return m, nil
	}
	m.cursor = row
	m.ensureCursorVisible()
	cmd := m.updateTextPreviewAsync()

	now := time.Now()
	if row == m.lastClickRow && now.Sub(m.lastClickAt) < 500*time.Millisecond {
		// double-click: open
		if len(m.entries) == 0 {
			return m, cmd
		}
		entry := m.entries[m.cursor]
		full := filepath.Join(m.cwd, entry.Name())
		if entry.IsDir() {
			m.cwd = full
			m.loadEntries()
			m.clearStatus()
			return m, m.updateTextPreviewAsync()
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".pdf" || ext == ".epub" {
			openPath := full
			if m.cwdIsLinkView() {
				openPath = canonicalPath(full)
			}
			if err := m.openPDF(openPath); err != nil {
				m.setStatus("Failed to open file: " + err.Error())
			} else if !m.cwdIsRecentlyOpened {
				m.recordRecentlyOpened(openPath)
			}
		}
		m.lastClickRow = -1
		m.lastClickAt = time.Time{}
		return m, cmd
	}
	m.lastClickRow = row
	m.lastClickAt = now
	return m, cmd
}

// clickInListPanel returns the local Y (relative to list area) and ok when inside the list panel.
func (m Model) clickInListPanel(msg tea.MouseMsg) (int, bool) {
	if m.width <= 0 || m.viewportHeight <= 0 {
		return 0, false
	}

	// Horizontal hit test
	leftWidth, middleWidth, _ := m.panelWidths()
	gapWidth := panelSeparatorWidth / 2
	if gapWidth < 1 {
		gapWidth = 1
	}
	listStartX := leftWidth
	if !m.treePaneHidden {
		listStartX += gapWidth
	}
	listEndX := listStartX + middleWidth
	if msg.X < listStartX || msg.X >= listEndX {
		return 0, false
	}

	// Vertical hit test: the panels now start at the top row (the old Dir header
	// and blank spacer were removed), so the list area spans viewportHeight rows
	// from row 0.
	listStartY := 0
	listEndY := listStartY + m.viewportHeight
	if msg.Y < listStartY || msg.Y >= listEndY {
		return 0, false
	}
	localY := msg.Y - listStartY
	return localY, true
}

// clickInTreePanel maps a click in the left tree pane to a visible tree-node
// index, returning ok=false when the click lands outside the pane. The pane has
// a top border row but no title, so the first node sits at screen row 1.
func (m Model) clickInTreePanel(msg tea.MouseMsg) (int, bool) {
	if m.treePaneHidden || m.width <= 0 || m.viewportHeight <= 0 {
		return 0, false
	}
	leftWidth, _, _ := m.panelWidths()
	if leftWidth <= 0 {
		return 0, false
	}
	if msg.X < 0 || msg.X >= leftWidth {
		return 0, false
	}
	const headerLines = 1 // top border only
	localY := msg.Y - headerLines
	if localY < 0 || msg.Y >= m.viewportHeight {
		return 0, false
	}
	idx := m.treeViewStart + localY
	nodes := m.visibleTreeNodes()
	if idx < 0 || idx >= len(nodes) {
		return 0, false
	}
	return idx, true
}

// goToParentDir mirrors the keyboard handler for "h"/left/backspace, and backs
// the right-click gesture in the Files pane. In a tag view there is no parent to
// climb to, so it dismisses the filter instead, exactly as "h" does.
func (m *Model) goToParentDir() tea.Cmd {
	if m.cwdIsTagView {
		return m.leaveTagView()
	}
	currentDir := m.cwd
	parent := filepath.Dir(m.cwd)
	if parent == m.cwd || !strings.HasPrefix(parent, m.root) {
		m.setStatus("Already at root")
		return nil
	}

	m.cwd = parent
	m.loadEntries()
	childName := filepath.Base(currentDir)
	if childName != "" {
		target := filepath.Join(m.cwd, childName)
		if idx := m.findEntryIndex(target); idx >= 0 {
			m.cursor = idx
			m.ensureCursorVisible()
		}
	}
	m.clearStatus()
	return m.updateTextPreviewAsync()
}
