package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/Han8931/gorae/internal/ai"
)

func (m *Model) renderGoraeView() string {
	width := m.width
	// The chat view is full-screen (no "Dir:" header), so it fills the whole
	// window height. Using windowHeight — not viewportHeight, which is reserved
	// for the file browser's header/footer chrome — keeps the status bar pinned
	// to the very bottom row instead of floating a few lines above it.
	height := m.windowHeight
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	// The /load fuzzy-find box takes over the screen as a centered modal.
	if m.aiFindMode {
		return m.renderFindModal(width, height)
	}

	// Build overlay lines shown between separator and input.
	var overlayLines []string
	if !m.aiStreaming {
		muted := m.styles.Preview.Body
		accent := m.styles.StatusValue
		bright := m.styles.Preview.Info
		overlayLines = m.goraeCommandHint(muted, accent, bright)
	}

	// Layout, bottom-anchored: chat + separator(1) + overlay + input region + status(1).
	const statusRows = 1
	const sepRows = 1
	inputRegionRows := m.goraeInputRegionRows()
	chatHeight := height - inputRegionRows - statusRows - sepRows - len(overlayLines)
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

	// Input region. During streaming/compacting/normal mode it collapses to a
	// single status line; while composing it is the multi-line message box.
	if m.aiStreaming {
		frame := spinnerFrames[m.aiSpinnerFrame%len(spinnerFrames)]
		b.WriteString(m.styles.StatusLabel.Render(" " + frame + " "))
		b.WriteString(m.styles.StatusValue.Render(" Thinking…  Esc to stop"))
		b.WriteString("\n")
	} else if m.aiCompacting {
		frame := spinnerFrames[m.aiSpinnerFrame%len(spinnerFrames)]
		b.WriteString(m.styles.StatusLabel.Render(" " + frame + " "))
		b.WriteString(m.styles.StatusValue.Render(" Compacting…"))
		b.WriteString("\n")
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
		b.WriteString("\n")
	} else {
		for _, line := range m.renderGoraeInputBox(width) {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	// Status bar — the final line, pinned to the bottom row.
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
	{"/load", "add paper(s) to the conversation — Tab to multi-select"},
	{"/unfocus", "clear all papers from the conversation"},
	{"/summarize", "summarize the focused paper and save to its note"},
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

// renderFindModal renders the /load fuzzy-find box as a centered floating modal,
// telescope/fzf style: a search input on top and a scroll of results below, each
// row showing the title, the filename, and (for content hits) a matching snippet.
// The main status bar stays pinned to the bottom row of the screen.
func (m *Model) renderFindModal(width, height int) string {
	titleStyle := m.styles.Preview.Info
	labelStyle := m.styles.StatusValue
	mutedStyle := m.styles.Preview.Body
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7aa2f7")).
		Padding(0, 1)

	// Box is ~3/4 of the screen, clamped to a comfortable range.
	boxW := width * 3 / 4
	if boxW > 96 {
		boxW = 96
	}
	if boxW < 30 {
		boxW = 30
	}
	innerW := boxW - 2 // account for horizontal padding
	if innerW < 20 {
		innerW = 20
	}

	// Search input row.
	m.aiInput.Width = innerW - 3
	var b strings.Builder
	header := " Load papers into context"
	if n := len(m.aiFindMarked); n > 0 {
		header += fmt.Sprintf("  (%d selected)", n)
	}
	b.WriteString(labelStyle.Render(header))
	b.WriteByte('\n')
	b.WriteString(m.aiInput.View())
	b.WriteByte('\n')
	b.WriteString(mutedStyle.Render(strings.Repeat("─", innerW)))
	b.WriteByte('\n')

	// How many result lines fit: leave room for borders, the three header rows
	// above, the footer row, and a little breathing space around the box.
	maxBodyLines := height - 8
	if maxBodyLines < 2 {
		maxBodyLines = 2
	}

	if len(m.aiSearchResults) == 0 {
		b.WriteString(mutedStyle.Render("  no matches"))
	} else {
		// Both the cursor row and every selected row are drawn as solid,
		// full-width highlight bars (lf/ranger style) — no marker glyph. Two
		// distinct bar colours keep the cursor identifiable among selections:
		//   • cursor (marked or not) → warm bar (List.Cursor) — "you are here"
		//   • selected, off-cursor   → selection bar (List.CursorSelected)
		// The cursor bar wins on a selected row it happens to sit on, just as lf
		// lets the cursor highlight override the selection highlight.
		cursorBar := m.styles.List.Cursor
		if isZeroStyle(cursorBar) {
			cursorBar = lipgloss.NewStyle().
				Background(lipgloss.Color("#e0af68")).
				Foreground(lipgloss.Color("#1a1b26")).
				Bold(true)
		}
		selBar := m.styles.List.CursorSelected
		if isZeroStyle(selBar) {
			selBar = lipgloss.NewStyle().
				Background(lipgloss.Color("#2ac3de")).
				Foreground(lipgloss.Color("#1a1b26")).
				Bold(true)
		}
		used := 0
		for i, r := range m.aiSearchResults {
			// Each result needs up to 2 lines (title + detail); stop before overflow.
			if used+2 > maxBodyLines && i != m.aiSearchCursor {
				break
			}
			title := runewidth.Truncate(r.Title, innerW-4, "…")
			detail := filepath.Base(r.Path)
			if strings.TrimSpace(r.Snippet) != "" {
				detail = r.Snippet
			}
			detail = runewidth.Truncate(detail, innerW-4, "…")

			switch {
			case i == m.aiSearchCursor:
				b.WriteString(cursorBar.Render(panelContent(innerW, title)))
			case m.isFindMarked(r.Path):
				b.WriteString(selBar.Render(panelContent(innerW, title)))
			default:
				b.WriteString(" " + titleStyle.Render(title))
			}
			b.WriteByte('\n')
			b.WriteString("  " + mutedStyle.Render(detail))
			b.WriteByte('\n')
			used += 2
		}
	}

	// Footer hint.
	b.WriteByte('\n')
	b.WriteString(mutedStyle.Render("↑/↓ move · Tab select · Enter load · Esc cancel"))

	box := borderStyle.Width(boxW).Render(b.String())

	// Center the box in every row except the last, which holds the status bar.
	centerArea := height - 1
	if centerArea < 1 {
		centerArea = 1
	}
	placed := lipgloss.Place(width, centerArea, lipgloss.Center, lipgloss.Center, box)
	return placed + "\n" + m.renderStatusBar()
}

// goraeInputRegionRows reports how many rows the input region occupies for the
// current mode. It must match what renderGoraeView emits so the status bar stays
// pinned to the bottom and scroll math (aiChatMaxScroll) lines up.
func (m *Model) goraeInputRegionRows() int {
	if m.aiStreaming || m.aiCompacting || m.aiNormalMode {
		return 1
	}
	return goraeInputBoxRows
}

// renderGoraeInputBox renders the multi-line chat message box: a " YOU " badge
// and hint line, then the textarea framed in a rounded border. It always returns
// exactly goraeInputBoxRows lines.
func (m *Model) renderGoraeInputBox(width int) []string {
	if width < 12 {
		width = 12
	}
	// Total box width == width; the border adds 2 columns, padding adds 2 more.
	boxContentW := width - 2
	m.aiTextarea.SetWidth(boxContentW - 2)
	m.aiTextarea.SetHeight(goraeInputTextareaRows)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7aa2f7")).
		Padding(0, 1).
		Width(boxContentW)

	badge := m.styles.StatusLabel.Render(" YOU ")
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Italic(true).
		Render("  Enter send · Ctrl+J newline · Esc navigate")

	lines := []string{badge + hint}
	lines = append(lines, strings.Split(boxStyle.Render(m.aiTextarea.View()), "\n")...)

	// Guard against geometry surprises so the caller's row budget always holds.
	for len(lines) < goraeInputBoxRows {
		lines = append(lines, "")
	}
	return lines[:goraeInputBoxRows]
}

func (m Model) goraeCommandHint(muted, accent, bright lipgloss.Style) []string {
	val := m.aiTextarea.Value()
	if !strings.HasPrefix(val, "/") || strings.ContainsRune(val, ' ') || strings.ContainsRune(val, '\n') {
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
