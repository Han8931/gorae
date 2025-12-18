package app

import (
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// handleMouse implements basic mouse interactions:
// - Scroll wheel: scroll list/search results.
// - Left click: move cursor; double-click (or click) opens file/dir.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.MouseWheelUp:
		return m.scrollMouse(-1), nil
	case tea.MouseWheelDown:
		return m.scrollMouse(1), nil
	case tea.MouseLeft:
		return m.clickMouse(msg), nil
	case tea.MouseRight:
		return m.clickMouse(msg), nil
	default:
		return m, nil
	}
}

func (m Model) scrollMouse(delta int) Model {
	if m.state == stateSearchResults {
		if delta < 0 {
			m.moveSearchCursor(-1)
		} else {
			m.moveSearchCursor(1)
		}
		return m
	}
	// normal list
	step := delta
	if step == 0 {
		return m
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
	m.syncCurrentEntryState()
	return m
}

func (m Model) clickMouse(msg tea.MouseMsg) Model {
	// We only handle clicks on the middle list panel when in normal state.
	// Use viewportStart and listVisibleRows to hit-test rows.
	// Adjust X/Y if you add panel-aware hit tests later.
	if m.state == stateSearchResults {
		row := m.searchResultOffset + msg.Y
		if row >= 0 && row < len(m.searchResults) {
			m.searchResultCursor = row
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
		return m
	}
	if msg.Button != tea.MouseButtonLeft {
		if msg.Button == tea.MouseButtonRight {
			if _, ok := m.clickInListPanel(msg); ok {
				m.goToParentDir()
			}
		}
		return m
	}
	localY, ok := m.clickInListPanel(msg)
	if !ok {
		return m
	}
	row := m.hitTestListRow(localY)
	if row < 0 {
		return m
	}
	m.cursor = row
	m.ensureCursorVisible()
	m.syncCurrentEntryState()

	now := time.Now()
	if row == m.lastClickRow && now.Sub(m.lastClickAt) < 500*time.Millisecond {
		// double-click: open
		if len(m.entries) == 0 {
			return m
		}
		entry := m.entries[m.cursor]
		full := filepath.Join(m.cwd, entry.Name())
		if entry.IsDir() {
			m.cwd = full
			m.loadEntries()
			m.clearStatus()
			m.updateTextPreview()
			return m
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".pdf" || ext == ".epub" {
			openPath := full
			if m.cwdIsRecentlyOpened || m.cwdIsRecentlyAdded {
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
		return m
	}
	m.lastClickRow = row
	m.lastClickAt = now
	return m
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
	listStartX := leftWidth + gapWidth
	listEndX := listStartX + middleWidth
	if msg.X < listStartX || msg.X >= listEndX {
		return 0, false
	}

	// Vertical hit test: header lines (Dir + blank), list area of viewportHeight
	const headerLines = 2
	listStartY := headerLines
	listEndY := listStartY + m.viewportHeight
	if msg.Y < listStartY || msg.Y >= listEndY {
		return 0, false
	}
	localY := msg.Y - listStartY
	return localY, true
}

// goToParentDir mirrors the keyboard handler for "h"/left/backspace.
func (m *Model) goToParentDir() {
	currentDir := m.cwd
	parent := filepath.Dir(m.cwd)
	if parent == m.cwd || !strings.HasPrefix(parent, m.root) {
		m.setStatus("Already at root")
		return
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
	m.updateTextPreview()
}


