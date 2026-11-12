package app

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// enterTrashList opens the Trash browser. Without it the names ':trash restore'
// expects are invisible — they are generated basenames under the meta dir, not
// anything the user typed — so browsing is what makes restore usable at all.
// An empty Trash reports itself on the status line rather than opening a view
// with nothing in it.
func (m *Model) enterTrashList() tea.Cmd {
	entries, err := m.trashEntries()
	if err != nil {
		m.setStatus("Trash unavailable: " + err.Error())
		return nil
	}
	if len(entries) == 0 {
		m.setStatus("Trash is empty")
		return nil
	}
	m.trashList = entries
	m.trashCursor = 0
	m.state = stateTrashList
	m.setPersistentStatus("Trash  ↑/↓ navigate · Enter/r restore · R restore all · Esc close")
	return nil
}

// refreshTrashList re-reads the Trash after a restore, closing the view once
// the last item is gone so the user is not left staring at an empty list.
func (m *Model) refreshTrashList() {
	entries, err := m.trashEntries()
	if err != nil || len(entries) == 0 {
		m.exitTrashList()
		return
	}
	m.trashList = entries
	if m.trashCursor >= len(entries) {
		m.trashCursor = len(entries) - 1
	}
	if m.trashCursor < 0 {
		m.trashCursor = 0
	}
}

func (m *Model) exitTrashList() {
	m.state = stateNormal
	m.trashList = nil
	m.trashCursor = 0
	m.setPersistentStatus("")
}

func (m *Model) updateTrashList(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "esc", "ctrl+c", "q":
		m.exitTrashList()
		return m, nil
	case "up", "k":
		if m.trashCursor > 0 {
			m.trashCursor--
		}
	case "down", "j":
		if m.trashCursor < len(m.trashList)-1 {
			m.trashCursor++
		}
	case "g":
		m.trashCursor = 0
	case "G":
		if len(m.trashList) > 0 {
			m.trashCursor = len(m.trashList) - 1
		}
	case "enter", "r":
		if len(m.trashList) == 0 {
			return m, nil
		}
		entry := m.trashList[m.trashCursor]
		if !entry.Restorable {
			m.setStatus(fmt.Sprintf("%s has no restore record — no original location to put it back to", entry.Name))
			return m, nil
		}
		dest, err := m.restoreFromTrash(entry.Name)
		if err != nil {
			m.setStatus("Restore failed: " + err.Error())
			return m, nil
		}
		m.afterTrashRestore()
		m.refreshTrashList()
		// A restore lands somewhere else when the original path is occupied
		// again, so report where the item actually went.
		if m.state == stateTrashList {
			m.setStatus("Restored to " + dest)
		} else {
			m.setStatus("Restored to " + dest + " — Trash is now empty")
		}
	case "R":
		restored, err := m.restoreAllFromTrash()
		if restored > 0 {
			m.afterTrashRestore()
		}
		m.refreshTrashList()
		switch {
		case err != nil && restored == 0:
			m.setStatus("Restore failed: " + err.Error())
		case err != nil:
			m.setStatus(fmt.Sprintf("Restored %d item(s); some failed: %s", restored, err.Error()))
		default:
			m.setStatus(fmt.Sprintf("Restored %d item(s) from Trash", restored))
		}
	}
	return m, nil
}

func (m Model) renderTrashList() string {
	width := m.width
	height := m.viewportHeight
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	labelStyle := m.styles.StatusLabel
	valueStyle := m.styles.StatusValue
	bodyStyle := m.styles.Preview.Body
	sepStyle := m.styles.Separator

	var b strings.Builder
	b.WriteString(labelStyle.Render(" TRASH "))
	b.WriteString("\n\n")

	listHeight := height - 7
	if listHeight < 3 {
		listHeight = 3
	}

	offset := 0
	if m.trashCursor >= listHeight {
		offset = m.trashCursor - listHeight + 1
	}
	visible := m.trashList
	if offset > 0 && offset < len(visible) {
		visible = visible[offset:]
	}
	if len(visible) > listHeight {
		visible = visible[:listHeight]
	}

	const deletedCol = 13
	// The origin directory is what distinguishes two same-named items, so it
	// gets whatever width is left after the name and deletion time.
	nameCol := width / 3
	if nameCol < 16 {
		nameCol = 16
	}
	originCol := width - nameCol - deletedCol - 8
	if originCol < 12 {
		originCol = 12
	}

	for i, entry := range visible {
		idx := i + offset

		name := entry.Name
		if entry.IsDir {
			name += "/"
		}
		origin := "(no restore record)"
		if entry.OriginalPath != "" {
			origin = filepath.Dir(entry.OriginalPath)
		}
		deleted := "unknown"
		if !entry.DeletedAt.IsZero() {
			deleted = sessionAgeLabel(entry.DeletedAt)
		}

		// Names keep their head (the start identifies the document); origins keep
		// their tail, since the leading library directories are the same for
		// everything and the final folder is what tells two items apart.
		line := fmt.Sprintf("%-*s  %-*s  %s",
			nameCol, truncateRunes(name, nameCol-1),
			originCol, truncateLeft(origin, originCol),
			deleted)
		if idx == m.trashCursor {
			b.WriteString(valueStyle.Render("▶ " + line))
		} else {
			b.WriteString(bodyStyle.Render("  " + line))
		}
		b.WriteString("\n")
	}

	for i := len(visible); i < listHeight; i++ {
		b.WriteString("\n")
	}

	b.WriteString(sepStyle.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	b.WriteString(bodyStyle.Render("  Enter/r restore   R restore all   Esc close   (':trash empty' deletes permanently)"))
	b.WriteString("\n")
	b.WriteString(m.renderStatusBar())

	return b.String()
}
