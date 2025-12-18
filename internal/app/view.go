package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"gorae/internal/meta"
)

// pad or truncate a string to exactly width columns.
func padRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > width {
		return string(r[:width])
	}
	return s + strings.Repeat(" ", width-len(r))
}

// fit or pad lines to a given height.
func fitLines(lines []string, height int) []string {
	if height <= 0 {
		return nil
	}
	if len(lines) > height {
		return lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

type panelLineKind int

const (
	panelLineBody panelLineKind = iota
	panelLineInfo
	panelLineActive
	panelLineSelected
	panelLineCursor
	panelLineCursorSelected
)

type panelLine struct {
	text string
	kind panelLineKind
}

// Left panel: simple "tree" panel from root to cwd.
func (m Model) renderTreePanel(width, height int) []string {
	lines := []panelLine{
		{text: fmt.Sprintf("Current: %s", filepath.Base(m.cwd)), kind: panelLineInfo},
	}

	parent := filepath.Dir(m.cwd)
	if parent == m.cwd || !strings.HasPrefix(parent, m.root) {
		lines = append(lines, panelLine{text: "(root directory)", kind: panelLineInfo})
		return m.renderPanelBlock("Tree", lines, width, height, m.styles.Tree)
	}

	ents, err := os.ReadDir(parent)
	if err != nil {
		lines = append(lines, panelLine{text: "(error reading parent)", kind: panelLineInfo})
		return m.renderPanelBlock("Tree", lines, width, height, m.styles.Tree)
	}

	filtered := make([]os.DirEntry, 0, len(ents))
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		filtered = append(filtered, e)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		pathI := filepath.Join(parent, filtered[i].Name())
		pathJ := filepath.Join(parent, filtered[j].Name())
		pi := m.specialDirPriority(pathI)
		pj := m.specialDirPriority(pathJ)
		if pi != pj {
			return pi < pj
		}
		di, dj := filtered[i].IsDir(), filtered[j].IsDir()
		if di != dj {
			return di && !dj
		}
		return strings.ToLower(filtered[i].Name()) <
			strings.ToLower(filtered[j].Name())
	})

	lines = append(lines, panelLine{text: fmt.Sprintf("Parent: %s", parent), kind: panelLineInfo})
	for _, e := range filtered {
		full := filepath.Join(parent, e.Name())
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		icon := m.entryIcon(e.IsDir())
		text := fmt.Sprintf("%s %s", icon, name)
		kind := panelLineBody
		if full == m.cwd {
			kind = panelLineActive
		}
		lines = append(lines, panelLine{text: text, kind: kind})
	}

	return m.renderPanelBlock("Tree", lines, width, height, m.styles.Tree)
}

// Middle panel: file list (what your old View used to show).
func (m Model) renderListPanel(width, height int) []string {
	var lines []panelLine

	if len(m.entries) == 0 {
		lines = append(lines, panelLine{text: "(empty)", kind: panelLineInfo})
		return m.renderPanelBlock("Files", lines, width, height, m.styles.List)
	}

	bodyRows := height - 3
	if bodyRows < 1 {
		bodyRows = 1
	}
	end := m.viewportStart + bodyRows
	if end > len(m.entries) {
		end = len(m.entries)
	}

	for i := m.viewportStart; i < end; i++ {
		e := m.entries[i]
		full := filepath.Join(m.cwd, e.Name())
		display := m.entryDisplayName(full, e)

		kind := panelLineBody
		if i == m.cursor {
			kind = panelLineCursor
		}

		selMarker := " "
		if m.selected[full] {
			selMarker = m.selectionIndicator()
		}

		text := fmt.Sprintf("%s %s", selMarker, display)
		lines = append(lines, panelLine{text: text, kind: kind})
	}

	title := fmt.Sprintf("Files (%d)", len(m.entries))
	return m.renderPanelBlock(title, lines, width, height, m.styles.List)
}

func (m Model) renderPreviewPanel(width, height int) []string {
	if height <= 0 {
		return nil
	}
	innerWidth := width - 2
	if innerWidth <= 0 {
		innerWidth = width
	}

	// When metadata is available for the current file, prefer showing the full
	// metadata (including the abstract) instead of mixing it with the text
	// preview. This gives the metadata panel the full vertical space so long
	// abstracts are less likely to be visually truncated.
	showMetadataOnly := m.currentMeta != nil

	metaSection := panelizeLines(m.metadataPanelLines(width))
	if showMetadataOnly && len(metaSection) > 0 {
		return m.renderPanelBlock("Details", metaSection, width, height, m.styles.Preview)
	}

	previewSection := panelizeLines(m.previewPanelLines(innerWidth))
	if len(metaSection) == 0 {
		return m.renderPanelBlock("Details", previewSection, width, height, m.styles.Preview)
	}

	reservedMeta := height / 2
	if reservedMeta < 8 {
		if height >= 8 {
			reservedMeta = 8
		} else if height > 2 {
			reservedMeta = height / 2
		} else {
			reservedMeta = height
		}
	}
	if reservedMeta > height {
		reservedMeta = height
	}

	previewLimit := height - reservedMeta
	if previewLimit < 0 {
		previewLimit = 0
	}
	if previewLimit > len(previewSection) {
		previewLimit = len(previewSection)
	}

	lines := make([]panelLine, 0, height)
	lines = append(lines, previewSection[:previewLimit]...)
	if previewLimit > 0 && len(lines) < height {
		lines = append(lines, panelLine{
			text: dividerLine(innerWidth),
			kind: panelLineInfo,
		})
	}

	remaining := height - len(lines)
	if remaining < 0 {
		remaining = 0
	}
	metaCount := len(metaSection)
	if metaCount > remaining {
		metaCount = remaining
	}
	lines = append(lines, metaSection[:metaCount]...)

	return m.renderPanelBlock("Details", lines, width, height, m.styles.Preview)
}

// // wrapLinesToWidth wraps each line so that no visual line exceeds `width` runes.
// func wrapLinesToWidth(lines []string, width int) []string {
// 	if width <= 0 {
// 		return nil
// 	}

// 	var out []string
// 	for _, l := range lines {
// 		r := []rune(l)
// 		for len(r) > width {
// 			out = append(out, string(r[:width]))
// 			r = r[width:]
// 		}
// 		out = append(out, string(r))
// 	}
// 	return out
// }

// trim each line so it never exceeds the given width (in runes).
func trimLinesToWidth(lines []string, width int) []string {
	if width <= 0 {
		return nil
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = trimStringToWidth(l, width)
	}
	return out
}

func trimLine(s string, width int) string {
	return trimStringToWidth(s, width)
}

func trimStringToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	current := 0
	for _, r := range s {
		w := lipgloss.Width(string(r))
		if current+w > width {
			break
		}
		b.WriteRune(r)
		current += w
	}
	return b.String()
}

func wrapTextToWidth(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	current := ""
	appendCurrent := func() {
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
	}
	for _, word := range words {
		wordRunes := []rune(word)
		if runeLen(word) > width {
			appendCurrent()
			for len(wordRunes) > width {
				lines = append(lines, string(wordRunes[:width]))
				wordRunes = wordRunes[width:]
			}
			current = string(wordRunes)
			continue
		}
		if current == "" {
			current = word
			continue
		}
		candidate := current + " " + word
		if runeLen(candidate) > width {
			lines = append(lines, current)
			current = word
		} else {
			current = candidate
		}
	}
	appendCurrent()
	return lines
}

// panelContentUsableWidth mirrors the spacing logic in panelContent to return
// the maximum number of characters that can be displayed inside a panel line
// (excluding borders and internal padding).
func panelContentUsableWidth(panelWidth int) int {
	if panelWidth <= 0 {
		return 0
	}
	inner := panelWidth - 2 // account for borders
	if inner < 1 {
		inner = panelWidth
	}
	margin := 1
	if inner <= margin*2 {
		margin = 0
	}
	usable := inner - margin*2
	if usable < 1 {
		usable = inner
	}
	return usable
}

func isParagraphMetaField(label string) bool {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "abstract", "note":
		return true
	default:
		return false
	}
}

func boolLabel(v bool) string {
	if v {
		return "Yes"
	}
	return "No"
}

func (m Model) renderMetaPopupLines(width int) []string {
	lines := m.metaPopupContentLines(width)
	if len(lines) == 0 {
		return nil
	}
	height := m.viewportHeight
	if height <= 0 {
		height = len(lines)
	}
	if height <= 0 {
		return nil
	}
	maxOffset := len(lines) - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	offset := m.metaPopupOffset
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + height
	if end > len(lines) {
		end = len(lines)
	}
	return lines[offset:end]
}

func (m Model) metaPopupContentLines(width int) []string {
	if width <= 0 {
		width = 40
	}
	label := metaFieldLabel(m.metaFieldIndex)
	if label == "" {
		label = "Field"
	}
	fileName := filepath.Base(m.metaEditingPath)
	if fileName == "" || fileName == "." {
		fileName = m.metaEditingPath
	}

	popupLines := []string{
		fmt.Sprintf("File : %s", fileName),
		"",
		"Fields:",
	}

	wrapWidth := width - 6
	if wrapWidth < 10 {
		wrapWidth = width
	}

	for i := 0; i < metaFieldCount(); i++ {
		fieldLabel := metaFieldLabel(i)
		value := strings.TrimSpace(metadataFieldValue(m.metaDraft, i))
		if value == "" {
			value = "(empty)"
		}
		prefix := "  "
		if m.metaFieldIndex == i {
			prefix = "➤ "
		}

		if isParagraphMetaField(fieldLabel) {
			popupLines = append(popupLines, fmt.Sprintf("%s%s:", prefix, fieldLabel))
			wrapped := wrapTextToWidth(value, wrapWidth)
			for _, line := range wrapped {
				popupLines = append(popupLines, "    "+line)
			}
			continue
		}

		popupLines = append(popupLines, fmt.Sprintf("%s%s: %s", prefix, fieldLabel, value))
	}

	popupLines = append(popupLines, "", "Note preview:")
	note := strings.TrimSpace(m.currentNote)
	if note == "" {
		popupLines = append(popupLines, "    (none - press 'n' to edit)")
	} else {
		for _, line := range wrapTextToWidth(note, wrapWidth) {
			popupLines = append(popupLines, "    "+line)
		}
	}

	if m.state == stateEditMeta {
		popupLines = append(popupLines,
			"",
			fmt.Sprintf("Edit %s:", label),
			m.input.View(),
			"",
			"Tab       → next field",
			"Shift+Tab → previous field",
			"Enter     → next/save",
			"Esc or q  → cancel",
		)
	} else {
		popupLines = append(popupLines,
			"",
			"Use ↑/↓ or PgUp/PgDn to scroll fields.",
			"Press 'e' to edit fields here, 'v' to edit fields in your editor.",
			"Press 'n' to edit the note in your editor.",
			"Press 'Esc' or 'q' to cancel.",
		)
	}

	box := renderPopupBox("Metadata Editor", popupLines, width)
	box = strings.TrimRight(box, "\n")
	if box == "" {
		return nil
	}
	lines := strings.Split(box, "\n")
	for i, line := range lines {
		lines[i] = m.styles.MetaOverlay.Render(line)
	}
	return lines
}

func renderPopupBox(title string, lines []string, totalWidth int) string {
	if totalWidth <= 0 {
		totalWidth = 80
	}

	maxLen := runeLen(title)
	for _, line := range lines {
		if l := runeLen(line); l > maxLen {
			maxLen = l
		}
	}

	boxWidth := maxLen
	if boxWidth < 30 {
		boxWidth = 30
	}
	if limit := totalWidth - 4; limit > 10 && boxWidth > limit {
		boxWidth = limit
	}
	if boxWidth < 10 {
		boxWidth = 10
	}

	boxLineWidth := boxWidth + 4
	indent := 0
	if totalWidth > boxLineWidth {
		indent = (totalWidth - boxLineWidth) / 2
	}
	pad := strings.Repeat(" ", indent)

	horizontal := "+" + strings.Repeat("-", boxWidth+2) + "+\n"

	var b strings.Builder
	b.WriteString(pad)
	b.WriteString(horizontal)
	b.WriteString(pad)
	b.WriteString(fmt.Sprintf("| %s |\n", padRight(title, boxWidth)))
	b.WriteString(pad)
	b.WriteString("| " + strings.Repeat("-", boxWidth) + " |\n")
	for _, line := range lines {
		b.WriteString(pad)
		b.WriteString(fmt.Sprintf("| %s |\n", padRight(line, boxWidth)))
	}
	b.WriteString(pad)
	b.WriteString(horizontal)

	return b.String()
}

func runeLen(s string) int {
	return len([]rune(s))
}

func (m Model) View() string {
	if m.state == stateSearchResults {
		return m.renderSearchResultsView()
	}
	if m.state == stateHelp {
		return m.renderHelpView()
	}
	var b strings.Builder
	var overlayLines []string

	// Header (full width)
	fmt.Fprintf(&b, "Dir : %s\n\n", m.cwd)

	// If we don't know width yet (no WindowSizeMsg yet), fall back to single-panel list.
	if m.width <= 0 {
		for _, line := range m.renderListPanel(80, m.viewportHeight) {
			b.WriteString(line + "\n")
		}
	} else {
		// --- compute panel widths ---
		// // Left: 1/4, Right: 1/3, Middle: remaining.
		// leftWidth := m.width / 5
		// rightWidth := m.width / 3 + 10
		// middleWidth := m.width - leftWidth - rightWidth - 2  // 2 for "│" separators

		leftWidth, middleWidth, rightWidth := m.panelWidths()

		height := m.viewportHeight
		if height < 3 {
			height = 3
		}

		treeLines := m.renderTreePanel(leftWidth, height)
		listLines := m.renderListPanel(middleWidth, height)
		prevLines := m.renderPreviewPanel(rightWidth, height)

		if m.state == stateEditMeta || m.state == stateMetaPreview {
			overlayLines = m.renderMetaPopupLines(middleWidth)
			if len(overlayLines) > 0 {
				for i := range overlayLines {
					overlayLines[i] = padStyledLine(overlayLines[i], middleWidth)
				}
			}
		}

		gapWidth := panelSeparatorWidth / 2
		if gapWidth < 1 {
			gapWidth = 1
		}
		gap := strings.Repeat(" ", gapWidth)

		for i := 0; i < height; i++ {
			tl := ""
			if i < len(treeLines) {
				tl = treeLines[i]
			}
			ll := ""
			if len(overlayLines) > 0 && i < len(overlayLines) {
				ll = overlayLines[i]
			} else if i < len(listLines) {
				ll = listLines[i]
			}
			pl := ""
			if i < len(prevLines) {
				pl = prevLines[i]
			}

			line := tl
			if gap != "" {
				line += gap
			}
			line += ll
			if gap != "" {
				line += gap
			}
			line += pl

			b.WriteString(line + "\n")
		}
	}

	var promptLine string
	switch m.state {
	case stateNewDir:
		promptLine = m.renderPromptLine("new dir", m.input.View())
	case stateRename:
		promptLine = m.renderPromptLine("rename", m.input.View())
	case stateCommand:
		promptLine = m.renderMinimalPrompt(":", m.input.View())
	case stateSearchPrompt:
		promptLine = m.renderPromptLine("search", m.input.View())
	case stateArxivPrompt:
		promptLine = m.renderPromptLine("arxiv", m.input.View())
	}
	b.WriteString("\n")
	b.WriteString(m.renderStatusBar())
	b.WriteString("\n")
	if promptLine != "" {
		b.WriteString(promptLine + "\n")
	}
	if len(m.commandOutput) > 0 {
		lines := m.commandOutput
		if m.width > 0 {
			lines = trimLinesToWidth(lines, m.width)
		}
		start := 0
		end := len(lines)
		if m.commandOutputPinned && len(lines) > 0 {
			view := m.commandOutputViewHeight()
			if view > len(lines) {
				view = len(lines)
			}
			maxOffset := len(lines) - view
			offset := m.commandOutputOffset
			if offset < 0 {
				offset = 0
			}
			if maxOffset < 0 {
				maxOffset = 0
			}
			if offset > maxOffset {
				offset = maxOffset
			}
			start = offset
			end = offset + view
			if end > len(lines) {
				end = len(lines)
			}
		}
		for _, line := range lines[start:end] {
			b.WriteString(line + "\n")
		}
		if m.commandOutputPinned && len(lines) > 0 {
			limit := m.width
			if limit <= 0 {
				limit = 80
			}
			info := fmt.Sprintf("Lines %d-%d of %d (j/k scroll, Esc close)", start+1, end, len(lines))
			summary := m.styles.Separator.Render(padStyledLine(info, limit))
			b.WriteString(summary + "\n")
		}
	}

	return b.String()
}

func (m Model) renderSearchResultsView() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	listHeight, detailHeight := m.searchResultsHeights()

	var b strings.Builder
	header := strings.TrimSpace(m.searchSummary)
	if header == "" {
		header = fmt.Sprintf("%s search results", m.lastSearchMode.displayName())
	}
	b.WriteString(m.styles.AppHeader.Render(padStyledLine(header, width)) + "\n")

	if filter := strings.TrimSpace(m.quickFilter.label()); filter != "" {
		line := fmt.Sprintf("Quick filter: %s", filter)
		b.WriteString(m.styles.Tree.Active.Render(padStyledLine(line, width)) + "\n")
	}

	if value := strings.TrimSpace(m.lastSearchQuery); value != "" {
		line := fmt.Sprintf("Query (%s): %s", m.lastSearchMode.displayName(), value)
		b.WriteString(m.styles.Tree.Info.Render(padStyledLine(line, width)) + "\n")
	}

	controls := "Controls: j/k move • PgUp/PgDn page • Enter open • Esc/q close • / search again"
	b.WriteString(m.styles.Separator.Render(padStyledLine(controls, width)) + "\n")

	listLines, title := m.searchResultListLines(listHeight)
	if title == "" {
		title = "Results"
	}
	for _, line := range m.renderPanelBlock(title, listLines, width, listHeight+3, m.styles.List) {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")

	detailLines := panelizeLines(m.searchResultDetailLines(detailHeight, width))
	for _, line := range m.renderPanelBlock("Preview", detailLines, width, detailHeight+3, m.styles.Preview) {
		b.WriteString(line + "\n")
	}

	if warn := m.renderSearchWarnings(width, detailHeight); warn != "" {
		b.WriteString("\n" + warn + "\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderHelpView() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	window := m.helpWindowHeight()
	if window < 10 {
		window = 10
	}
	body := m.helpViewBodyHeight()
	lines := trimLinesToWidth(m.helpLines, width)
	total := len(lines)
	start := m.helpOffset
	if start < 0 {
		start = 0
	}
	maxOffset := total - body
	if maxOffset < 0 {
		maxOffset = 0
	}
	if start > maxOffset {
		start = maxOffset
	}
	end := start + body
	if end > total {
		end = total
	}

	var b strings.Builder
	header := fmt.Sprintf("Gorae Help — %d lines", total)
	b.WriteString(m.styles.AppHeader.Render(padStyledLine(header, width)) + "\n")
	controls := "Controls: j/k scroll • PgUp/PgDn jump • g g top • G/end bottom • Esc/q close • Ctrl+C quit"
	b.WriteString(m.styles.Separator.Render(padStyledLine(controls, width)) + "\n\n")

	if total == 0 {
		b.WriteString("(Help unavailable)\n")
	} else {
		for i := start; i < end; i++ {
			if i >= 0 && i < len(lines) {
				b.WriteString(lines[i] + "\n")
			}
		}
		visible := end - start
		for i := visible; i < body; i++ {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	if total == 0 {
		b.WriteString(m.styles.Separator.Render(padStyledLine("No manual content available", width)))
	} else {
		info := fmt.Sprintf("Lines %d-%d of %d (j/k scroll, Esc/q close)", start+1, end, total)
		b.WriteString(m.styles.Separator.Render(padStyledLine(info, width)))
	}

	return strings.TrimRight(b.String(), "\n")
}

func (m Model) searchResultListLines(visible int) ([]panelLine, string) {
	if visible < 1 {
		visible = 1
	}
	total := len(m.searchResults)
	if total == 0 {
		return []panelLine{{text: "(no matches)", kind: panelLineInfo}}, "Results (0)"
	}
	start := m.searchResultOffset
	if start < 0 {
		start = 0
	}
	if start >= total {
		start = total - 1
		if start < 0 {
			start = 0
		}
	}
	cursor := m.searchResultCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= total {
		cursor = total - 1
	}
	if start > cursor {
		start = cursor
	}
	end := start + visible
	if end > total {
		end = total
		start = end - visible
		if start < 0 {
			start = 0
		}
	}
	lines := make([]panelLine, 0, end-start)
	for i := start; i < end; i++ {
		match := m.searchResults[i]
		title := strings.TrimSpace(match.Title)
		if title == "" {
			title = untitledPlaceholder
		}
		year := strings.TrimSpace(match.Year)
		display := title
		if year != "" {
			display = fmt.Sprintf("%s (%s)", title, year)
		}
		info := []string{}
		if match.MatchCount > 0 {
			hits := "hit"
			if match.MatchCount > 1 {
				hits = "hits"
			}
			info = append(info, fmt.Sprintf("%d %s", match.MatchCount, hits))
		}
		if len(info) > 0 {
			display += "  · " + strings.Join(info, " · ")
		}
		text := fmt.Sprintf("%3d. %s", i+1, display)
		kind := panelLineBody
		if i == cursor {
			kind = panelLineCursor
		}
		lines = append(lines, panelLine{text: text, kind: kind})
	}
	title := fmt.Sprintf("Results %d-%d of %d", start+1, end, total)
	return lines, title
}

func (m Model) renderSearchWarnings(width, detailHeight int) string {
	if len(m.searchWarnings) == 0 {
		return ""
	}
	maxWarn := detailHeight / 2
	if maxWarn < 1 {
		maxWarn = 1
	}
	if maxWarn > len(m.searchWarnings) {
		maxWarn = len(m.searchWarnings)
	}
	lines := make([]panelLine, 0, maxWarn)
	for i := 0; i < maxWarn; i++ {
		lines = append(lines, panelLine{text: m.searchWarnings[i], kind: panelLineInfo})
	}
	panel := m.renderPanelBlock(fmt.Sprintf("Warnings (%d)", len(m.searchWarnings)), lines, width, maxWarn+3, m.styles.Tree)
	var b strings.Builder
	for _, line := range panel {
		b.WriteString(line + "\n")
	}
	if len(m.searchWarnings) > maxWarn {
		extra := fmt.Sprintf("... %d more warning(s)", len(m.searchWarnings)-maxWarn)
		b.WriteString(m.styles.Tree.Info.Render(padStyledLine(extra, width)))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) metadataPreviewLines(width int) []string {
	if m.meta == nil || m.currentMetaPath == "" {
		return nil
	}
	contentWidth := panelContentUsableWidth(width)
	if contentWidth < 8 {
		contentWidth = width
	}
	var md meta.Metadata
	if m.currentMeta != nil {
		md = *m.currentMeta
	}
	md.Path = m.currentMetaPath
	if strings.TrimSpace(md.Title) == "" {
		return m.metadataPreviewFromText(width)
	}
	lines := make([]string, 0, metaFieldCount()+1)
	type metaFieldLine struct {
		label string
		value string
	}
	var tailFields []metaFieldLine
	titleRendered := false
	for i := 0; i < metaFieldCount(); i++ {
		rawVal := strings.TrimSpace(metadataFieldValue(md, i))
		val := rawVal
		if val == "" {
			val = "(empty)"
		}
		label := metaFieldLabel(i)
		if label == "Published" || label == "URL" || label == "DOI" {
			tailFields = append(tailFields, metaFieldLine{label: label, value: val})
			continue
		}
		if strings.EqualFold(label, "Title") {
			labelLine := m.previewLabel("Title")
			if labelLine == "" {
				labelLine = "Title:"
			}
			lines = append(lines, labelLine)
			titleWidth := contentWidth - 2 // account for indent
			if titleWidth < 10 {
				titleWidth = contentWidth
			}
			if rawVal == "" {
				if fallback := m.metadataTitleFallback(titleWidth); len(fallback) > 0 {
					for _, w := range fallback {
						lines = append(lines, "  "+w)
					}
					lines = append(lines, "")
					titleRendered = true
					continue
				}
			}
			for _, w := range wrapTextToWidth(val, titleWidth) {
				lines = append(lines, "  "+w)
			}
			lines = append(lines, "")
			titleRendered = true
			continue
		}
		if isParagraphMetaField(label) {
			labelLine := m.previewLabel(label)
			if labelLine == "" {
				labelLine = label + ":"
			}
			lines = append(lines, labelLine)
			offsetWidth := contentWidth - 2 // account for indent
			if offsetWidth < 10 {
				offsetWidth = contentWidth
			}
			wrapped := wrapTextToWidth(val, offsetWidth)
			for _, w := range wrapped {
				lines = append(lines, "  "+w)
			}
			continue
		}
		lines = append(lines, m.formatDetailLine(label, val, contentWidth))
	}
	if titleRendered && len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}

	statusLabel := m.previewLabel("Status")
	if statusLabel == "" {
		statusLabel = "Status:"
	}
	lines = append(lines, statusLabel)
	lines = append(lines,
		fmt.Sprintf("  Favorite : %s (%s)", metadataStatusBadge(md.Favorite, m.favoriteIcon()), boolLabel(md.Favorite)))
	lines = append(lines,
		fmt.Sprintf("  To-read  : %s (%s)", metadataStatusBadge(md.ToRead, m.toReadIcon()), boolLabel(md.ToRead)))
	lines = append(lines,
		fmt.Sprintf("  Reading  : %s %s", m.readingStateIcon(md.ReadingState), readingStateLabel(md.ReadingState)))
	lines = append(lines, "")
	noteWidth := contentWidth - 2 // account for indent
	if noteWidth < 10 {
		noteWidth = contentWidth
	}
	noteLabel := m.previewLabel("Note")
	if noteLabel == "" {
		noteLabel = "Note:"
	}
	lines = append(lines, noteLabel)
	note := strings.TrimSpace(m.currentNote)
	if note == "" {
		lines = append(lines, "  (none - press 'n' to edit in your editor)")
	} else {
		for _, wrapped := range wrapTextToWidth(note, noteWidth) {
			lines = append(lines, "  "+wrapped)
		}
	}
	if len(tailFields) > 0 {
		lines = append(lines, "")
		for _, field := range tailFields {
			lines = append(lines, m.formatDetailLine(field.label, field.value, contentWidth))
		}
	}
	return lines
}

const (
	maxTitlePreviewLines = 6
	untitledPlaceholder  = "(untitled)"
)

func (m Model) metadataTitleFallback(width int) []string {
	if len(m.previewText) == 0 {
		return nil
	}
	if width < 10 {
		width = 10
	}
	lines := make([]string, 0, maxTitlePreviewLines)
	for _, raw := range m.previewText {
		text := strings.TrimSpace(raw)
		if text == "" {
			continue
		}
		for _, wrapped := range wrapTextToWidth(text, width) {
			w := strings.TrimSpace(wrapped)
			if w == "" {
				continue
			}
			lines = append(lines, w)
			if len(lines) >= maxTitlePreviewLines {
				return lines
			}
		}
	}
	return lines
}

func (m Model) metadataPreviewFromText(width int) []string {
	if width <= 0 {
		width = 40
	}
	contentWidth := panelContentUsableWidth(width) - 2 // account for indent
	if contentWidth < 10 {
		contentWidth = width
	}
	lines := []string{"Preview (first page):"}
	if len(m.previewText) == 0 {
		lines = append(lines, "  (no preview available)")
		return lines
	}
	added := false
	for _, raw := range m.previewText {
		text := strings.TrimSpace(raw)
		if text == "" {
			continue
		}
		for _, wrapped := range wrapTextToWidth(text, contentWidth) {
			lines = append(lines, "  "+wrapped)
			added = true
		}
	}
	if !added {
		lines = append(lines, "  (no preview available)")
	}
	return trimLinesToWidth(lines, width)
}

func (m Model) previewLabel(label string) string {
	clean := strings.TrimSpace(label)
	if clean == "" {
		return ""
	}
	clean = strings.TrimSuffix(clean, ":")
	text := clean + ":"
	return m.styles.Preview.Info.Render(text)
}

func (m Model) formatDetailLine(label, value string, width int) string {
	cleanLabel := strings.TrimSpace(label)
	if cleanLabel == "" {
		return value
	}
	rawLabel := fmt.Sprintf("%-10s", cleanLabel+":")
	colored := m.styles.Preview.Info.Render(rawLabel)
	formatted := fmt.Sprintf("%s %s", colored, value)
	if width > 0 {
		return trimLine(formatted, width)
	}
	return formatted
}

func metadataStatusBadge(active bool, icon string) string {
	if !active {
		return "-"
	}
	if trimmed := strings.TrimSpace(icon); trimmed != "" {
		return trimmed
	}
	return "*"
}

func (m Model) renderStatusBar() string {
	width := m.width
	if width <= 0 {
		width = 80
	}

	modeSeg := m.statusSegment("MODE", m.currentModeLabel())
	dirSeg := m.statusSegment("DIR", m.cwd)
	label, value := m.selectionSummary()
	itemSeg := m.statusSegment(strings.ToUpper(label), value)
	status := m.statusMessage(time.Now())
	if status == "" {
		status = "Ready"
	}
	statusSeg := m.statusSegment("MSG", status)

	segments := []string{modeSeg, dirSeg, itemSeg, statusSeg}
	line := strings.Join(segments, " ")
	line = padStyledLine(line, width)
	return m.styles.StatusBar.Render(line)
}

func (m Model) selectionSummary() (string, string) {
	selectedCount := len(m.selected)
	if len(m.entries) == 0 {
		if selectedCount > 0 {
			return "Selected", fmt.Sprintf("%d", selectedCount)
		}
		return "Items", "0"
	}

	entry := m.entries[m.cursor]
	name := entry.Name()
	if entry.IsDir() {
		name += "/"
	}

	value := name
	if selectedCount > 0 {
		value += fmt.Sprintf("  Sel:%d", selectedCount)
	}
	return "Item", value
}

func (m Model) currentModeLabel() string {
	switch m.state {
	case stateCommand:
		return "Command"
	case stateSearchPrompt, stateSearchResults:
		return "Search"
	case stateEditMeta, stateMetaPreview:
		return "Meta"
	case stateNewDir:
		return "New Dir"
	case stateRename:
		return "Rename"
	case stateArxivPrompt:
		return "arXiv"
	case stateUnmarkPrompt:
		return "Unmark"
	default:
		return "Normal"
	}
}

func (m Model) entryDisplayName(full string, entry fs.DirEntry) string {
	if title, ok := m.entryTitles[full]; ok && title != "" {
		return title
	}
	if entry.IsDir() {
		return entry.Name() + "/"
	}
	name := entry.Name()
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext)
}

func (m Model) metadataPanelLines(width int) []string {
	metaLines := m.metadataPreviewLines(width)
	if len(metaLines) == 0 {
		return nil
	}
	return trimLinesToWidth(metaLines, width)
}

func (m Model) previewPanelLines(width int) []string {
	lines := []string{}

	if len(m.entries) == 0 {
		return trimLinesToWidth([]string{"No selection"}, width)
	}

	if len(m.previewText) > 0 {
		preview := make([]string, len(m.previewText))
		copy(preview, m.previewText)
		return trimLinesToWidth(preview, width)
	}

	e := m.entries[m.cursor]
	full := filepath.Join(m.cwd, e.Name())
	display := m.entryDisplayName(full, e)

	if e.IsDir() {
		lines = append(lines, display)
	} else {
		lines = append(lines,
			"File:",
			"  "+display,
			"",
			"Path:",
			"  "+full,
		)
	}
	return trimLinesToWidth(lines, width)
}

func dividerLine(width int) string {
	if width <= 0 {
		width = 40
	}
	return strings.Repeat("─", width)
}

func (m Model) searchResultDetailLines(limit, width int) []string {
	match := m.currentSearchMatch()
	if match == nil {
		lines := []string{
			"(no selection)",
			"",
			"Press Esc to exit the search view.",
		}
		return trimLinesToWidth(lines, width)
	}
	lines := []string{
		fmt.Sprintf("File: %s", match.Path),
		fmt.Sprintf("Matches: %d", match.MatchCount),
		"",
	}
	if match.Mode == searchModeContent {
		lines = append(lines, "Snippets:")
		lines = append(lines, formatContentSnippets(match.Snippets)...)
	} else {
		lines = append(lines, "Metadata:")
		for _, snippet := range match.Snippets {
			lines = append(lines, "  "+snippet)
		}
	}
	lines = trimLinesToWidth(lines, width)
	if limit > 0 && len(lines) > limit {
		lines = lines[:limit]
	}
	return lines
}

func formatContentSnippets(snippets []string) []string {
	if len(snippets) == 0 {
		return []string{"  (no snippet data)"}
	}
	lines := make([]string, 0, len(snippets)*3)
	for i, snippet := range snippets {
		parts := strings.Split(snippet, "\n")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			lines = append(lines, "  "+part)
		}
		if i < len(snippets)-1 {
			lines = append(lines, "")
		}
	}
	if len(lines) == 0 {
		lines = []string{"  (no snippet data)"}
	}
	return lines
}

func panelizeLines(lines []string) []panelLine {
	out := make([]panelLine, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		kind := panelLineBody
		if trimmed == "" {
			kind = panelLineBody
		} else if strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(line, "  ") {
			kind = panelLineInfo
		}
		out = append(out, panelLine{text: line, kind: kind})
	}
	return out
}

func padStyledLine(line string, width int) string {
	w := lipgloss.Width(line)
	if w >= width {
		return lipgloss.PlaceHorizontal(width, lipgloss.Left, line)
	}
	return line + strings.Repeat(" ", width-w)
}

func (m Model) statusSegment(label, value string) string {
	lbl := strings.ToUpper(strings.TrimSpace(label))
	val := strings.TrimSpace(value)
	labelPart := m.styles.StatusLabel.Render(" " + lbl + " ")
	valuePart := m.styles.StatusValue.Render(" " + val + " ")
	return labelPart + valuePart
}

func (m Model) renderPromptLine(label, value string) string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	labelText := strings.ToUpper(strings.TrimSpace(label))
	segment := lipgloss.JoinHorizontal(
		lipgloss.Left,
		m.styles.PromptLabel.Render(" "+labelText+" "),
		m.styles.PromptValue.Render(" "+value),
	)
	return padStyledLine(segment, width)
}

func (m Model) renderMinimalPrompt(prefix, value string) string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	text := prefix + value
	return padStyledLine(text, width)
}

func (m Model) renderPanelBlock(title string, lines []panelLine, width, height int, styles panelStyles) []string {
	if width < 4 {
		width = 4
	}
	if height < 3 {
		height = 3
	}
	innerWidth := width - 2
	bodyHeight := height - 2
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	top := m.styles.Border.Render(m.borderChars.TopLeft + strings.Repeat(m.borderChars.Horizontal, innerWidth) + m.borderChars.TopRight)
	result := []string{top}

	header := panelContent(innerWidth, title)
	headerLine := fallbackStyle(styles.Header, lipgloss.NewStyle()).Render(header)
	result = append(result, m.borderRow(headerLine, width))

	bodyIndex := 0
	for i := 0; i < bodyHeight-1; i++ {
		text := ""
		kind := panelLineBody
		if bodyIndex < len(lines) {
			entry := lines[bodyIndex]
			bodyIndex++
			text = entry.text
			kind = entry.kind
		}
		content := panelContent(innerWidth, text)
		styled := m.styleForPanelLine(styles, kind).Render(content)
		result = append(result, m.borderRow(styled, width))
	}

	bottom := m.styles.Border.Render(m.borderChars.BottomLeft + strings.Repeat(m.borderChars.Horizontal, innerWidth) + m.borderChars.BottomRight)
	result = append(result, bottom)
	return result
}

func panelContent(innerWidth int, text string) string {
	if innerWidth <= 0 {
		return ""
	}
	margin := 1
	if innerWidth <= margin*2 {
		margin = 0
	}
	usable := innerWidth - margin*2
	if usable <= 0 {
		usable = innerWidth
		margin = 0
	}
	trimmed := trimLine(text, usable)
	padded := padStyledLine(trimmed, usable)
	if margin == 0 {
		return padded
	}
	return strings.Repeat(" ", margin) + padded + strings.Repeat(" ", margin)
}

func (m Model) borderRow(content string, width int) string {
	if width <= 2 {
		return content
	}
	left := m.styles.Border.Render(m.borderChars.Vertical)
	right := m.styles.Border.Render(m.borderChars.Vertical)
	return left + content + right
}

func (m Model) styleForPanelLine(styles panelStyles, kind panelLineKind) lipgloss.Style {
	switch kind {
	case panelLineInfo:
		return fallbackStyle(styles.Info, styles.Body)
	case panelLineActive:
		return fallbackStyle(styles.Active, styles.Body)
	case panelLineSelected:
		return fallbackStyle(styles.Selected, styles.Body)
	case panelLineCursor:
		return fallbackStyle(styles.Cursor, fallbackStyle(styles.Selected, styles.Body))
	case panelLineCursorSelected:
		if !isZeroStyle(styles.CursorSelected) {
			return styles.CursorSelected
		}
		if !isZeroStyle(styles.Cursor) {
			return styles.Cursor
		}
		return fallbackStyle(styles.Selected, styles.Body)
	default:
		return fallbackStyle(styles.Body, lipgloss.NewStyle())
	}
}

func fallbackStyle(primary, fallback lipgloss.Style) lipgloss.Style {
	if isZeroStyle(primary) {
		return fallback
	}
	return primary
}

func isZeroStyle(s lipgloss.Style) bool {
	return reflect.ValueOf(s).IsZero()
}
