package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"gorae/internal/ai"
)

func (m *Model) renderGoraeView() string {
	width := m.width
	height := m.viewportHeight
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	// Build overlay lines shown between separator and input.
	var overlayLines []string
	if m.aiSearchSelecting && len(m.aiSearchResults) > 0 {
		overlayLines = m.buildFindOverlay(width)
	} else if !m.aiStreaming {
		muted := m.styles.Preview.Body
		accent := m.styles.StatusValue
		bright := m.styles.Preview.Info
		overlayLines = m.goraeCommandHint(muted, accent, bright)
	}

	// Layout: status(1) + input(1) + separator(1) + overlay + padding(1)
	const inputRows = 1
	const statusRows = 1
	const sepRows = 1
	const paddingRows = 1
	chatHeight := height - inputRows - statusRows - sepRows - paddingRows - len(overlayLines)
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
	if m.aiFollowBottom {
		scroll = maxScroll
	}
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

	// Emit kitty/iTerm2 delete sequence once when transitioning into this view.
	if m.previewGraphicClear {
		if m.previewGraphicFmt == "kitty" || terminalGraphicFormat() == "kitty" {
			b.WriteString(kittyDeletePreviewSequence())
		}
		m.previewGraphicClear = false
	}

	// Chat area
	for _, line := range visible {
		fmt.Fprintln(&b, line)
	}

	// Separator
	b.WriteString(m.styles.Separator.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	// Overlay (find results or command hints) above the input row
	for _, ol := range overlayLines {
		fmt.Fprintln(&b, ol)
	}

	// Input row
	if m.aiStreaming {
		frame := spinnerFrames[m.aiSpinnerFrame%len(spinnerFrames)]
		b.WriteString(m.styles.StatusLabel.Render(" " + frame + " "))
		b.WriteString(m.styles.StatusValue.Render(" Thinking…  Esc to stop"))
	} else if m.aiCompacting {
		frame := spinnerFrames[m.aiSpinnerFrame%len(spinnerFrames)]
		b.WriteString(m.styles.StatusLabel.Render(" " + frame + " "))
		b.WriteString(m.styles.StatusValue.Render(" Compacting…"))
	} else {
		b.WriteString(m.styles.StatusLabel.Render(" YOU "))
		b.WriteString(" " + m.aiInput.View())
	}
	b.WriteString("\n")

	// Status bar
	b.WriteString(m.renderStatusBar())

	return b.String()
}

// renderThinkingBlock renders a collapsible thinking section.
// When expanded=false it shows only the "▶ Thinking…" header line.
// When expanded=true it shows the full content, like Claude's expanded view.
func (m Model) renderThinkingBlock(thinking string, wrapW int, expanded bool) []string {
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#777777")).Italic(true)
	thinkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Italic(true)
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#444444"))

	// Count lines of content for the collapsed summary
	contentLines := strings.Count(strings.TrimSpace(thinking), "\n") + 1

	var lines []string
	if !expanded {
		label := fmt.Sprintf("▶  Thinking… (%d lines)  Ctrl+T to expand", contentLines)
		lines = append(lines, "   "+headerStyle.Render(label))
		return lines
	}

	// Expanded view
	lines = append(lines, "   "+headerStyle.Render("▼  Thinking  (Ctrl+T to collapse)"))
	lines = append(lines, "   "+borderStyle.Render(strings.Repeat("╌", wrapW)))
	for _, pl := range renderMarkdownCustom(thinking, wrapW-2, m.styles.Markdown) {
		lines = append(lines, "   "+thinkStyle.Render(stripANSI(pl.text)))
	}
	lines = append(lines, "   "+borderStyle.Render(strings.Repeat("╌", wrapW)))
	return lines
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var goraeCommandDescs = []struct{ name, desc string }{
	{"/find", "find files by title or filename"},
	{"/select", "clear focused file"},
	{"/summarize", "summarize focused file and save to its note"},
	{"/clear", "clear chat history"},
	{"/compact", "summarise old messages to free up context window"},
	{"/export", "save conversation to a file"},
	{"/sources", "show documents cited in last answer"},
	{"/sessions", "open session picker — load or manage past conversations"},
	{"/skills", "manage custom prompt templates (edit / list)"},
	{"/new", "start a new session (current session stays saved)"},
	{"/help", "show help"},
}

func (m Model) buildChatLines(width int) []string {
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
				chatUserStyle := m.styles.ChatUser
				lines = append(lines, labelUserStyle.Render(" YOU "))
				for _, l := range wrapTextToWidth(msg.Content, wrapW) {
					pad := wrapW - runewidth.StringWidth(l)
					if pad < 0 {
						pad = 0
					}
					padded := l + strings.Repeat(" ", pad)
					lines = append(lines, "  "+chatUserStyle.Render(" "+padded+" "))
				}
				lines = append(lines, "")
			case ai.RoleAssistant:
				if msg.IsSummary {
					compactStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Italic(true)
					lines = append(lines, compactStyle.Render("  ╌╌╌ context summary ╌╌╌"))
					for _, pl := range renderMarkdownCustom(msg.Content, wrapW, m.styles.Markdown) {
						lines = append(lines, "   "+compactStyle.Render(pl.text))
					}
					lines = append(lines, compactStyle.Render("  ╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌"))
					lines = append(lines, "")
				} else {
					lines = append(lines, labelAIStyle.Render(" GORAE ")+" "+assistantStyle.Render(""))
					if msg.Thinking != "" {
						lines = append(lines, m.renderThinkingBlock(msg.Thinking, wrapW, m.aiShowThinking)...)
					}
					for _, pl := range renderMarkdownCustom(msg.Content, wrapW, m.styles.Markdown) {
						lines = append(lines, "   "+pl.text)
					}
					lines = append(lines, "")
				}
			}
		}

		// Streaming buffer
		if m.aiStreaming {
			lines = append(lines, labelAIStyle.Render(" GORAE ")+" ")
			if m.aiThinkBuf != "" {
				lines = append(lines, m.renderThinkingBlock(m.aiThinkBuf, wrapW, m.aiShowThinking)...)
			}
			if m.aiStreamBuf != "" {
				for _, pl := range renderMarkdownCustom(m.aiStreamBuf, wrapW, m.styles.Markdown) {
					lines = append(lines, "   "+pl.text)
				}
			}
		}
	}

	return lines
}

// buildFindOverlay renders the live /find results as an fzf-style bottom overlay.
func (m Model) buildFindOverlay(width int) []string {
	labelStyle := m.styles.StatusValue
	titleStyle := m.styles.Preview.Info
	mutedStyle := m.styles.Preview.Body
	cursorStyle := m.styles.StatusLabel

	wrapW := width - 6
	if wrapW < 20 {
		wrapW = 20
	}

	var lines []string
	lines = append(lines, mutedStyle.Render("  ↑/↓ navigate · Enter select · Esc cancel"))
	for i, r := range m.aiSearchResults {
		title := runewidth.Truncate(r.Title, wrapW-4, "…")
		fname := runewidth.Truncate(filepath.Base(r.Path), wrapW-6, "…")
		if i == m.aiSearchCursor {
			lines = append(lines, "  "+cursorStyle.Render(" ▶ ")+labelStyle.Render(" "+title+" "))
			lines = append(lines, "      "+mutedStyle.Render(fname))
		} else {
			lines = append(lines, "      "+titleStyle.Render(title))
			lines = append(lines, "        "+mutedStyle.Render(fname))
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
	// Include user-defined skills.
	for _, sk := range m.aiUserSkills {
		skillCmd := "/" + sk.Name
		if strings.HasPrefix(skillCmd, lower) {
			desc := sk.Prompt
			if len([]rune(desc)) > 45 {
				desc = string([]rune(desc)[:44]) + "…"
			}
			matched = append(matched, struct{ name, desc string }{skillCmd, desc})
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
