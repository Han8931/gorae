package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// keyHint is one entry of the hint bar: a key chip followed by a short label.
type keyHint struct {
	key, label string
}

// renderHintBar draws key chips ( j  move   /  search …) on one row, dropping
// trailing hints that don't fit rather than wrapping. It never writes the last
// column so terminals don't enter a pending-wrap state.
func (m Model) renderHintBar(hints []keyHint, width int) string {
	if width <= 0 {
		width = 80
	}
	limit := width - 1
	var b strings.Builder
	used := 0
	for i, h := range hints {
		chip := m.styles.KeyCap.Render(" "+h.key+" ") + " " + m.styles.KeyLabel.Render(h.label)
		w := lipgloss.Width(chip)
		sep := 0
		if i > 0 {
			sep = 2
		}
		if used+sep+w > limit {
			break
		}
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(chip)
		used += sep + w
	}
	return padStyledLine(b.String(), width)
}

// pendingPrefixHints lists what may follow a pending multi-key prefix, so
// sequences like g… s… y… ,… are discoverable instead of flashing by.
func (m Model) pendingPrefixHints() (string, []keyHint) {
	switch {
	case m.awaitingQuickFilter:
		return "g", []keyHint{{"r", "reading"}, {"u", "unread"}, {"d", "read"}, {"any", "cancel"}}
	case m.awaitingSort:
		return "sort", []keyHint{{"n", "name"}, {"t", "title"}, {"y", "year"}, {"any", "cancel"}}
	case m.prefixPending("y"):
		return "y", []keyHint{{"y", "BibTeX"}, {"t", "title/author/year"}}
	case m.prefixPending(","):
		return ",", []keyHint{{"n", "toggle tree pane"}}
	}
	return "", nil
}

func (m Model) prefixPending(key string) bool {
	return m.lastKey == key && time.Since(m.lastKeyAt) <= 1200*time.Millisecond
}

// browserHints returns the context hints shown under the three-pane browser.
func (m Model) browserHints() []keyHint {
	switch m.state {
	case stateMetaPreview:
		return []keyHint{{"j/k", "scroll"}, {"e", "edit metadata"}, {"n", "edit note"}, {"esc", "close"}}
	case stateUnmarkPrompt:
		return []keyHint{{"f", "favorite"}, {"t", "to-read"}, {"b", "both"}, {"esc", "cancel"}}
	}
	if m.commandOutputPinned {
		return []keyHint{{"j/k", "scroll"}, {"PgUp/PgDn", "page"}, {"esc", "close"}}
	}
	// Ordered by usefulness: hints that don't fit are dropped from the end.
	hints := []keyHint{
		{"j/k", "move"},
		{"h/l", "dir"},
		{"←/→", "tabs"},
		{"/", "search"},
		{":", "command"},
		{"e", "meta"},
		{"space", "select"},
		{"s", "sort"},
		{"g", "filter"},
		{"r", "read state"},
		{"yy", "BibTeX"},
		{"D", "delete"},
		{":h", "help"},
		{"q", "quit"},
	}
	if len(m.selected) > 0 {
		hints = append([]keyHint{{"d", "cut"}, {"D", "delete selected"}}, hints...)
	}
	if len(m.cut) > 0 {
		hints = append([]keyHint{{"p", "paste"}}, hints...)
	}
	return hints
}

// renderBrowserFooter returns the single row above the status bar: the active
// prompt if any, otherwise the delete confirmation, pending-prefix
// continuations, or the regular context hints.
func (m Model) renderBrowserFooter(promptLine string) string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	if promptLine != "" {
		return promptLine
	}
	if m.state == stateConfirmDelete {
		return m.renderDeleteConfirmLine(width)
	}
	if prefix, hints := m.pendingPrefixHints(); prefix != "" {
		lead := m.styles.WarnChip.Render(" "+prefix+"… ") + " "
		return padStyledLine(lead+m.renderHintBar(hints, width-lipgloss.Width(lead)), width)
	}
	return m.renderHintBar(m.browserHints(), width)
}

func (m Model) renderDeleteConfirmLine(width int) string {
	var what string
	if len(m.confirmItems) == 1 {
		what = fmt.Sprintf("Move %q to Trash?", filepath.Base(m.confirmItems[0]))
	} else {
		what = fmt.Sprintf("Move %d items to Trash?", len(m.confirmItems))
	}
	line := m.styles.DangerChip.Render(" DELETE ") + " " + m.styles.Danger.Render(what) + "  " +
		m.renderHintBar([]keyHint{{"y", "trash"}, {"n/esc", "cancel"}}, width/2)
	return padStyledLine(line, width)
}

func (m Model) renderStatusBar() string {
	width := m.width
	if width <= 0 {
		width = 80
	}

	modeChip := m.styles.ModeChip
	if m.state == stateConfirmDelete {
		modeChip = m.styles.DangerChip
	}
	label, value := m.selectionSummary()
	segments := []string{m.statusSegment(strings.ToUpper(label), value)}
	if m.sortMode != sortByName {
		segments = append(segments, m.styles.InfoChip.Render(" sort:"+sortModeLabel(m.sortMode)+" "))
	}
	if f := m.quickFilter.labelLower(); f != "" {
		segments = append(segments, m.styles.InfoChip.Render(" filter:"+f+" "))
	}
	status := m.statusMessage(time.Now())
	switch {
	case status == "":
		segments = append(segments, m.statusSegment("MSG", "Ready"))
	case isErrorStatus(status):
		segments = append(segments, m.styles.DangerChip.Render(" ERR ")+m.styles.StatusValue.Render(" "+status+" "))
	default:
		segments = append(segments, m.statusSegment("MSG", status))
	}

	// The directory gets whatever room the other segments leave, trimmed from
	// the left so the most specific part of the path stays visible.
	mode := modeChip.Render(" " + strings.ToUpper(m.currentModeLabel()) + " ")
	rest := strings.Join(segments, " ")
	dirRoom := width - 1 - lipgloss.Width(mode) - lipgloss.Width(rest) - lipgloss.Width(m.statusSegment("DIR", "")) - 2
	dir := m.statusSegment("DIR", truncateLeft(abbreviateHome(m.cwd), dirRoom))

	line := mode + " " + dir + " " + rest
	line = padStyledLine(line, width)
	return m.styles.StatusBar.Render(line)
}

func abbreviateHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}

// truncateLeft keeps the tail of s within width display columns, marking the
// cut with a leading ellipsis.
func truncateLeft(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	runes := []rune(s)
	for i := range runes {
		if tail := string(runes[i:]); lipgloss.Width(tail) <= width-1 {
			return "…" + tail
		}
	}
	return "…"
}

// isErrorStatus reports whether a status message describes a failure, so the
// status bar can flag it in the theme's danger colour.
func isErrorStatus(msg string) bool {
	lower := strings.ToLower(msg)
	for _, marker := range []string{"failed", "error", "invalid", "cannot", "can't", "unable"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func (m Model) currentModeLabel() string {
	switch m.state {
	case stateCommand:
		return "Command"
	case stateSearchPrompt, stateSearchResults:
		return "Search"
	case stateMetaPreview:
		return "Meta"
	case stateNewDir:
		return "New Dir"
	case stateRename:
		return "Rename"
	case stateArxivPrompt:
		return "arXiv"
	case stateUnmarkPrompt:
		return "Unmark"
	case stateConfirmDelete:
		return "Delete"
	case stateHelp:
		return "Help"
	case stateLaunch:
		return "Home"
	case stateGorae:
		if m.aiNormalMode {
			return "Gorae · Nav"
		}
		return "Gorae"
	case stateSessionList:
		return "Sessions"
	default:
		return "Normal"
	}
}
