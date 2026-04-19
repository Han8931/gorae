package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"gorae/internal/ai"
)

func (m Model) renderGoraeView() string {
	width := m.width
	height := m.viewportHeight
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	// Layout: status bar (1) + input bar (1) + borders/padding = ~4 overhead
	const inputRows = 1
	const statusRows = 1
	const paddingRows = 2
	chatHeight := height - inputRows - statusRows - paddingRows
	if chatHeight < 3 {
		chatHeight = 3
	}

	chatLines := m.buildChatLines(width)

	// Scroll: clamp to valid range
	maxScroll := len(chatLines) - chatHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := m.aiChatScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}

	end := scroll + chatHeight
	if end > len(chatLines) {
		end = len(chatLines)
	}
	visible := chatLines[scroll:end]

	// Pad to chatHeight
	for len(visible) < chatHeight {
		visible = append(visible, "")
	}

	var b strings.Builder

	// Chat area
	for _, line := range visible {
		fmt.Fprintln(&b, line)
	}

	// Divider
	b.WriteString(m.styles.Separator.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	// Input row
	if m.aiStreaming {
		frame := spinnerFrames[m.aiSpinnerFrame%len(spinnerFrames)]
		b.WriteString(m.styles.StatusLabel.Render(" " + frame + " "))
		b.WriteString(m.styles.StatusValue.Render(" Thinking…  Esc to stop"))
	} else {
		b.WriteString(m.styles.StatusLabel.Render(" YOU "))
		b.WriteString(" " + m.aiInput.View())
	}
	b.WriteString("\n")

	// Status bar
	b.WriteString(m.renderStatusBar())

	return b.String()
}

var goraeCommandDescs = []struct{ name, desc string }{
	{"/find", "find files by title or filename"},
	{"/select", "clear focused file"},
	{"/summarize", "summarize focused file and save to its note"},
	{"/clear", "clear chat history"},
	{"/export", "save conversation to a file"},
	{"/sources", "show documents cited in last answer"},
	{"/help", "show help"},
}

func (m Model) buildChatLines(width int) []string {
	userStyle := m.styles.Preview.Info
	assistantStyle := m.styles.Preview.Body
	labelUserStyle := m.styles.StatusLabel
	labelAIStyle := m.styles.StatusValue

	wrapW := width - 6
	if wrapW < 20 {
		wrapW = 20
	}

	var lines []string

	// Base content: welcome screen or conversation
	if len(m.aiMessages) == 0 && !m.aiSearchSelecting {
		lines = m.goraeWelcomeLines()
	} else {
		for _, msg := range m.aiMessages {
			switch msg.Role {
			case ai.RoleUser:
				lines = append(lines, labelUserStyle.Render(" YOU ")+" "+userStyle.Render(""))
				for _, l := range wrapTextToWidth(msg.Content, wrapW) {
					lines = append(lines, "       "+userStyle.Render(l))
				}
				lines = append(lines, "")
			case ai.RoleAssistant:
				lines = append(lines, labelAIStyle.Render(" GORAE ")+" "+assistantStyle.Render(""))
				for _, pl := range renderMarkdownCustom(msg.Content, wrapW, m.styles.Markdown) {
					lines = append(lines, "   "+pl.text)
				}
				lines = append(lines, "")
			}
		}

		// Streaming buffer
		if m.aiStreaming && m.aiStreamBuf != "" {
			lines = append(lines, labelAIStyle.Render(" GORAE ")+" ")
			for _, pl := range renderMarkdownCustom(m.aiStreamBuf, wrapW, m.styles.Markdown) {
				lines = append(lines, "   "+pl.text)
			}
		}
	}

	// Interactive search selection list
	if m.aiSearchSelecting && len(m.aiSearchResults) > 0 {
		lines = append(lines, "")
		lines = append(lines, assistantStyle.Render("  Select a file  (↑/↓ navigate · Enter select · Esc cancel)"))
		lines = append(lines, "")
		for i, r := range m.aiSearchResults {
			if i == m.aiSearchCursor {
				marker := labelAIStyle.Render(" ▶ ")
				title := userStyle.Render(r.Title)
				fname := assistantStyle.Render("     " + filepath.Base(r.Path))
				lines = append(lines, marker+title)
				lines = append(lines, fname)
			} else {
				title := assistantStyle.Render("     " + r.Title)
				fname := assistantStyle.Render("       " + filepath.Base(r.Path))
				lines = append(lines, title)
				lines = append(lines, fname)
			}
		}
		lines = append(lines, "")
	}

	// Command hint: show when input starts with "/" and no space yet
	if !m.aiStreaming && !m.aiSearchSelecting {
		if hint := m.goraeCommandHint(assistantStyle, labelAIStyle, userStyle); len(hint) > 0 {
			lines = append(lines, hint...)
		}
	}

	return lines
}

func (m Model) goraeCommandHint(muted, accent, bright lipgloss.Style) []string {
	val := m.aiInput.Value()
	if !strings.HasPrefix(val, "/") || strings.ContainsRune(val, ' ') {
		return nil
	}
	lower := strings.ToLower(val)
	var matched []struct{ name, desc string }
	for _, c := range goraeCommandDescs {
		if strings.HasPrefix(c.name, lower) {
			matched = append(matched, c)
		}
	}
	if len(matched) == 0 {
		return nil
	}
	lines := []string{"", muted.Render("  Commands:")}
	for _, c := range matched {
		prefix := accent.Render(val)
		rest := muted.Render(c.name[len(val):])
		desc := muted.Render("  " + c.desc)
		lines = append(lines, "    "+prefix+rest+desc)
	}
	lines = append(lines, "")
	return lines
}
