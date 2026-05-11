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

	chatLines, _ := m.buildChatLines(width)

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

	// Input row — the leftmost badge doubles as the mode indicator so it sits
	// right next to where the user is looking when they wonder why typing
	// doesn't work.
	if m.aiStreaming {
		frame := spinnerFrames[m.aiSpinnerFrame%len(spinnerFrames)]
		b.WriteString(m.styles.StatusLabel.Render(" " + frame + " "))
		b.WriteString(m.styles.StatusValue.Render(" Thinking…  Esc to stop"))
	} else if m.aiCompacting {
		frame := spinnerFrames[m.aiSpinnerFrame%len(spinnerFrames)]
		b.WriteString(m.styles.StatusLabel.Render(" " + frame + " "))
		b.WriteString(m.styles.StatusValue.Render(" Compacting…"))
	} else if m.aiNormalMode {
		normalBadge := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#FFD580")).
			Bold(true).
			Render(" NORMAL ")
		hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Italic(true)
		hint := "  i:insert  j/k:nav  gg/G:top/bot  h/l:jump  space:mark  y:yank  q:quit"
		if n := len(m.aiMsgMarks); n > 0 {
			hint = fmt.Sprintf("  %d mark(s)  y:yank all  c:clear  i:insert  gg/G:top/bot  q:quit", n)
		}
		b.WriteString(normalBadge)
		b.WriteString(hintStyle.Render(hint))
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
	{"/load", "load a file into chat context (search by title or filename)"},
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

func (m Model) buildChatLines(width int) ([]string, []int) {
	assistantStyle := m.styles.Preview.Body
	labelUserStyle := m.styles.StatusLabel
	labelAIStyle := m.styles.StatusValue
	// Cursor: invert label colours so the active message reads as a bright
	// chip. Mark: a yellow asterisk to the left of the role badge.
	cursorBadge := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#FFD580")).
		Bold(true)
	markStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD580")).Bold(true)

	wrapW := width - 6
	if wrapW < 20 {
		wrapW = 20
	}

	var lines []string
	msgStartLines := make([]int, 0, len(m.aiMessages))

	// renderRoleLabel returns the role badge plus a fixed-width left margin
	// (" *" if marked, "  " otherwise) so the chat doesn't shift horizontally
	// as marks come and go. When the message is the cursor target, the badge
	// itself is rendered in a high-contrast style so it's impossible to miss.
	renderRoleLabel := func(idx int, baseStyle lipgloss.Style, text string) string {
		margin := "  "
		if m.aiMsgMarks[idx] {
			margin = " " + markStyle.Render("*")
		}
		style := baseStyle
		if m.aiNormalMode && m.aiMsgCursor == idx {
			style = cursorBadge
		}
		return margin + style.Render(text)
	}

	// Base content: welcome screen or conversation
	if len(m.aiMessages) == 0 && !m.aiSearchSelecting {
		lines = m.goraeWelcomeLines()
	} else {
		for i, msg := range m.aiMessages {
			msgStartLines = append(msgStartLines, len(lines))
			switch msg.Role {
			case ai.RoleUser:
				chatUserStyle := m.styles.ChatUser
				lines = append(lines, renderRoleLabel(i, labelUserStyle, " YOU "))
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
					lines = append(lines, renderRoleLabel(i, compactStyle, " ╌╌╌ context summary ╌╌╌"))
					for _, pl := range renderMarkdownCustom(msg.Content, wrapW, m.styles.Markdown) {
						lines = append(lines, "   "+compactStyle.Render(pl.text))
					}
					lines = append(lines, compactStyle.Render("  ╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌"))
					lines = append(lines, "")
				} else {
					hasContent := strings.TrimSpace(msg.Content) != ""
					if hasContent || msg.Thinking != "" {
						lines = append(lines, renderRoleLabel(i, labelAIStyle, " GORAE ")+" "+assistantStyle.Render(""))
						if msg.Thinking != "" {
							lines = append(lines, m.renderThinkingBlock(msg.Thinking, wrapW, m.aiShowThinking)...)
						}
						if hasContent {
							for _, pl := range renderMarkdownCustom(msg.Content, wrapW, m.styles.Markdown) {
								lines = append(lines, "   "+pl.text)
							}
						}
						lines = append(lines, "")
					}
					if len(msg.ToolCalls) > 0 {
						lines = append(lines, renderToolCallLines(msg.ToolCalls, wrapW)...)
					}
				}
			case ai.RoleTool:
				lines = append(lines, renderToolResultLines(msg.Name, msg.Content, wrapW)...)
			}
		}

		// Streaming buffer
		if m.aiStreaming {
			lines = append(lines, "  "+labelAIStyle.Render(" GORAE ")+" ")
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

	return lines, msgStartLines
}

// renderToolCallLines renders a muted block summarising one or more tool
// invocations the model requested.
func renderToolCallLines(calls []ai.ToolCall, wrapW int) []string {
	toolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Italic(true)
	var lines []string
	for _, c := range calls {
		args := strings.TrimSpace(c.Func.Arguments)
		if args == "" || args == "{}" {
			lines = append(lines, "   "+toolStyle.Render("⚙ "+c.Func.Name+"()"))
			continue
		}
		header := "⚙ " + c.Func.Name + "(" + args + ")"
		for _, l := range wrapTextToWidth(header, wrapW) {
			lines = append(lines, "   "+toolStyle.Render(l))
		}
	}
	return lines
}

// renderToolResultLines renders the reply we sent back to the model.
func renderToolResultLines(name, content string, wrapW int) []string {
	resultStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6a9955")).Italic(true)
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#c94a4a")).Italic(true)
	style := resultStyle
	if strings.HasPrefix(content, "Error") {
		style = errStyle
	}
	var lines []string
	prefix := "   ↳ "
	for i, l := range wrapTextToWidth(content, wrapW-len(prefix)) {
		if i == 0 {
			lines = append(lines, prefix+style.Render(l))
		} else {
			lines = append(lines, "     "+style.Render(l))
		}
	}
	lines = append(lines, "")
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
