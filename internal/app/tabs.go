package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// listTab is one tab of the Files pane. The Library tab (dir == "") is the
// regular file browser; the others are the helper collection directories.
type listTab struct {
	label string
	dir   string // canonical directory, "" for Library
}

func (m Model) listTabs() []listTab {
	tabs := []listTab{{label: "Library"}}
	add := func(label, dir string) {
		if strings.TrimSpace(dir) != "" {
			tabs = append(tabs, listTab{label: label, dir: dir})
		}
	}
	// Ordered by workflow: resume what you were doing, then what's next, then
	// the backlog, then what's finished.
	add("Recent", m.recentlyOpenedDirCanonical)
	add("Reading", m.readingViewDir)
	add("To Read", m.toReadDirCanonical)
	add("Added", m.recentlyAddedDirCanonical)
	add("Unread", m.unreadViewDir)
	add("Read", m.readViewDir)
	return tabs
}

// activeTab derives the current tab from the working directory, so every way
// of reaching a collection directory (keys, launch menu, mouse) stays in sync.
func (m Model) activeTab() int {
	raw := filepath.Clean(m.cwd)
	canonical := canonicalPath(m.cwd)
	for i, tab := range m.listTabs() {
		if tab.dir == "" {
			continue
		}
		if pathWithin(raw, tab.dir) || pathWithin(canonical, tab.dir) {
			return i
		}
	}
	return 0
}

func pathWithin(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// isTabDir reports whether path is one of the collection directories shown as
// tabs; those are hidden from the Library listing and the tree pane. The
// legacy Favorites/ helper directory (no longer synced since favorites were
// removed) is hidden too.
func (m Model) isTabDir(path string) bool {
	canonical := canonicalPath(path)
	if m.favoritesDirCanonical != "" && canonical == m.favoritesDirCanonical {
		return true
	}
	for _, tab := range m.listTabs() {
		if tab.dir != "" && canonical == tab.dir {
			return true
		}
	}
	return false
}

// cycleTab moves delta tabs left or right, wrapping around.
func (m *Model) cycleTab(delta int) tea.Cmd {
	n := len(m.listTabs())
	return m.switchToTab(((m.activeTab()+delta)%n + n) % n)
}

// switchToTab opens tab idx. Leaving the Library tab remembers the directory
// and file under the cursor so returning to it lands in the same place.
func (m *Model) switchToTab(idx int) tea.Cmd {
	tabs := m.listTabs()
	if idx < 0 || idx >= len(tabs) {
		return nil
	}
	cur := m.activeTab()
	if idx == cur {
		return nil
	}
	if cur == 0 {
		m.libraryCwd = m.cwd
		m.libraryCursorPath = m.currentEntryPath()
	}

	m.cursor = 0
	m.viewportStart = 0
	if idx == 0 {
		m.cwd = m.root
		if info, err := os.Stat(m.libraryCwd); err == nil && info.IsDir() {
			m.cwd = m.libraryCwd
		}
		m.loadEntries()
		if i := m.findEntryIndex(m.libraryCursorPath); i >= 0 {
			m.cursor = i
			m.ensureCursorVisible()
		}
	} else {
		if m.isReadingStateView(tabs[idx].dir) {
			if err := m.syncReadingStateViews(); err != nil {
				m.setStatus("Reading state view sync failed: " + err.Error())
			}
		}
		m.cwd = tabs[idx].dir
		m.loadEntries()
	}
	m.clearStatus()
	return m.updateTextPreviewAsync()
}

// emptyTabMessage tells the user how an empty tab gets filled.
func (m Model) emptyTabMessage() string {
	switch canonicalPath(m.cwd) {
	case m.readingViewDir:
		return "Nothing in progress. Press r on a paper to mark it Reading."
	case m.unreadViewDir:
		return "No unread papers."
	case m.readViewDir:
		return "Nothing finished yet. Press r on a Reading paper to mark it Read."
	case m.toReadDirCanonical:
		return "Your to-read list is empty. Press t on a paper to add it."
	case m.recentlyOpenedDirCanonical:
		return "Papers you open will appear here."
	case m.recentlyAddedDirCanonical:
		return "Newly added papers will appear here."
	}
	return "(empty)"
}

// renderTabStrip draws the Files pane header: the active tab is a filled chip
// carrying the entry count, the rest are muted. When not every tab fits, the
// tabs furthest from the active one are dropped and ‹ / › mark the hidden side.
func (m Model) renderTabStrip(innerWidth int) string {
	tabs := m.listTabs()
	active := m.activeTab()

	render := func(lo, hi int) string {
		var b strings.Builder
		b.WriteString(" ")
		if lo > 0 {
			b.WriteString(m.styles.Muted.Render("‹ "))
		}
		for i := lo; i <= hi; i++ {
			if i > lo {
				b.WriteString("  ")
			}
			if i == active {
				b.WriteString(m.styles.ModeChip.Render(fmt.Sprintf(" %s %d ", tabs[i].label, len(m.entries))))
			} else {
				b.WriteString(m.styles.Muted.Render(tabs[i].label))
			}
		}
		if hi < len(tabs)-1 {
			b.WriteString(m.styles.Muted.Render(" ›"))
		}
		return b.String()
	}

	// Grow a window around the active tab, alternating right then left, and
	// keep the widest one that fits.
	lo, hi := active, active
	best := render(lo, hi)
	for lo > 0 || hi < len(tabs)-1 {
		grew := false
		if hi < len(tabs)-1 {
			if cand := render(lo, hi+1); lipgloss.Width(cand) <= innerWidth {
				hi, best, grew = hi+1, cand, true
			}
		}
		if lo > 0 {
			if cand := render(lo-1, hi); lipgloss.Width(cand) <= innerWidth {
				lo, best, grew = lo-1, cand, true
			}
		}
		if !grew {
			break
		}
	}
	return padStyledLine(lipgloss.NewStyle().MaxWidth(innerWidth).Render(best), innerWidth)
}
