package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// enterSessionList switches to the session picker. If no sessions exist it goes
// straight to a new chat.
func (m *Model) enterSessionList() tea.Cmd {
	if m.meta == nil {
		return m.enterGoraeChat()
	}
	sessions, err := m.meta.ListSessions(context.Background())
	if err != nil || len(sessions) == 0 {
		m.aiSessionID = 0
		return m.enterGoraeChat()
	}
	m.aiSessionList = sessions
	m.aiSessionCursor = 0
	m.state = stateSessionList
	m.setPersistentStatus("Sessions  ↑/↓ navigate · Enter load · n new · d delete · Esc cancel")
	return nil
}

func (m *Model) updateSessionList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.state = stateNormal
			m.setPersistentStatus("")
			return m, nil
		case "up", "k":
			if m.aiSessionCursor > 0 {
				m.aiSessionCursor--
			}
		case "down", "j":
			if m.aiSessionCursor < len(m.aiSessionList)-1 {
				m.aiSessionCursor++
			}
		case "n":
			m.aiSessionID = 0
			return m, m.enterGoraeChat()
		case "d":
			if len(m.aiSessionList) == 0 {
				return m, nil
			}
			sess := m.aiSessionList[m.aiSessionCursor]
			if err := m.meta.DeleteSession(context.Background(), sess.ID); err != nil {
				m.setStatus("Delete failed: " + err.Error())
				return m, nil
			}
			m.aiSessionList = append(m.aiSessionList[:m.aiSessionCursor], m.aiSessionList[m.aiSessionCursor+1:]...)
			if len(m.aiSessionList) == 0 {
				m.aiSessionID = 0
				return m, m.enterGoraeChat()
			}
			if m.aiSessionCursor >= len(m.aiSessionList) {
				m.aiSessionCursor = len(m.aiSessionList) - 1
			}
		case "enter":
			if len(m.aiSessionList) == 0 {
				return m, nil
			}
			sess := m.aiSessionList[m.aiSessionCursor]
			m.aiSessionID = sess.ID
			return m, m.enterGoraeChat()
		}
	}
	return m, nil
}

func (m Model) renderSessionList() string {
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

	b.WriteString(labelStyle.Render(" CHAT SESSIONS "))
	b.WriteString("\n\n")

	listHeight := height - 7
	if listHeight < 3 {
		listHeight = 3
	}

	offset := 0
	if m.aiSessionCursor >= listHeight {
		offset = m.aiSessionCursor - listHeight + 1
	}

	visible := m.aiSessionList
	if offset > 0 && offset < len(visible) {
		visible = visible[offset:]
	}
	if len(visible) > listHeight {
		visible = visible[:listHeight]
	}

	for i, sess := range visible {
		idx := i + offset

		// Show the session as a filename matching the export convention.
		name := sessionFilename(sess.Title, sess.CreatedAt)

		date := sessionAgeLabel(sess.UpdatedAt)
		msgCount := fmt.Sprintf("%d msg", sess.MessageCount)
		if sess.MessageCount != 1 {
			msgCount += "s"
		}

		dateCol := 11
		msgCol := 8
		fixed := dateCol + msgCol + 4 // separators
		maxName := width - fixed - 4  // 4 for cursor prefix
		if maxName < 10 {
			maxName = 10
		}
		runes := []rune(name)
		if len(runes) > maxName {
			name = string(runes[:maxName-1]) + "…"
		}

		line := fmt.Sprintf("%-*s  %-*s  %s", maxName, name, dateCol, date, msgCount)
		if idx == m.aiSessionCursor {
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
	b.WriteString(bodyStyle.Render("  n new   Enter load   d delete   Esc cancel"))
	b.WriteString("\n")
	b.WriteString(m.renderStatusBar())

	return b.String()
}

// sessionFilename builds a filename matching the export convention:
// gorae-chat-<slug>-YYYYMMDD-HHMMSS.md
func sessionFilename(title string, createdAt time.Time) string {
	ts := createdAt.Format("20060102-150405")
	slug := slugify(title)
	if slug == "" {
		return "gorae-chat-" + ts + ".md"
	}
	return "gorae-chat-" + slug + "-" + ts + ".md"
}

func sessionAgeLabel(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)
	switch {
	case diff < 60*time.Minute:
		return "just now"
	case diff < 24*time.Hour && t.Day() == now.Day():
		return "today " + t.Format("15:04")
	case diff < 48*time.Hour:
		return "yesterday"
	default:
		return t.Format("Jan 2, 2006")
	}
}
