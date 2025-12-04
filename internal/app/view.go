package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pdf-tui/internal/meta"
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

// Left panel: simple "tree" panel from root to cwd.
func (m Model) renderTreePanel(width, height int) []string {
	lines := []string{"[Parent]"}

	parent := filepath.Dir(m.cwd)

	// No valid parent under root → just show a note.
	if parent == m.cwd || !strings.HasPrefix(parent, m.root) {
		lines = append(lines, "(no parent under root)")
		lines = trimLinesToWidth(lines, width)
		return fitLines(lines, height)
	}

	ents, err := os.ReadDir(parent)
	if err != nil {
		lines = append(lines, "(error reading parent)")
		lines = trimLinesToWidth(lines, width)
		return fitLines(lines, height)
	}

	// hide dotfiles
	filtered := make([]os.DirEntry, 0, len(ents))
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		filtered = append(filtered, e)
	}

	// Optionally: sort dirs first, then alphabetically
	sort.SliceStable(filtered, func(i, j int) bool {
		di, dj := filtered[i].IsDir(), filtered[j].IsDir()
		if di != dj {
			return di && !dj
		}
		return strings.ToLower(filtered[i].Name()) <
			strings.ToLower(filtered[j].Name())
	})

	lines = append(lines, parent)

	for _, e := range filtered {
		full := filepath.Join(parent, e.Name())

		// mark the current directory
		marker := "  "
		if full == m.cwd {
			marker = "➜ "
		}

		name := e.Name()
		if e.IsDir() {
			name += "/"
		}

		lines = append(lines, marker+name)
	}

	return fitLines(lines, height)
}

// Middle panel: file list (what your old View used to show).
func (m Model) renderListPanel(width, height int) []string {
	var lines []string

	if len(m.entries) == 0 {
		lines = append(lines, "(empty)")
		lines = trimLinesToWidth(lines, width)
		return fitLines(lines, height)
	}

	end := m.viewportStart + height
	if end > len(m.entries) {
		end = len(m.entries)
	}

	for i := m.viewportStart; i < end; i++ {
		e := m.entries[i]

		cursor := "  "
		if i == m.cursor {
			cursor = "➜ "
		}

		full := filepath.Join(m.cwd, e.Name())
		sel := "[ ] "
		if m.selected[full] {
			sel = "[x] "
		}

		var line string

		if e.IsDir() {
			line = fmt.Sprintf("%s%s %s/", cursor, sel, e.Name())
		} else {
			line = fmt.Sprintf("%s%s %s", cursor, sel, e.Name())
		}

		lines = append(lines, line)
	}

	return fitLines(lines, height)
}

func (m Model) renderPreviewPanel(width, height int) []string {
	lines := []string{"[Preview]"}

	if len(m.entries) == 0 {
		lines = append(lines, "", "No selection")
		lines = trimLinesToWidth(lines, width)
		return fitLines(lines, height)
	}

	if metaLines := m.metadataPreviewLines(); len(metaLines) > 0 {
		lines = append(lines, "", "Metadata:")
		lines = append(lines, metaLines...)
	}

	if len(m.previewText) > 0 {
		lines = append(lines, "") // blank after header

		// 1) start with the raw previewText
		preview := make([]string, len(m.previewText))
		copy(preview, m.previewText)

		// 2) trim horizontally to the panel width
		preview = trimLinesToWidth(preview, width)
		// preview = wrapLinesToWidth(preview, width)

		// 3) append and clamp vertically
		lines = append(lines, preview...)
		lines = trimLinesToWidth(lines, width)
		return fitLines(lines, height)
	}

	// Fallback: basic info if no previewText set
	e := m.entries[m.cursor]
	full := filepath.Join(m.cwd, e.Name())

	lines = append(lines, "")
	if e.IsDir() {
		lines = append(lines, "Directory:", "  "+e.Name()+"/")
	} else {
		lines = append(lines, "File:", "  "+e.Name())
		lines = append(lines, "", "Path:", "  "+full)
	}

	lines = trimLinesToWidth(lines, width)
	return fitLines(lines, height)
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
		r := []rune(l)
		if len(r) > width {
			out[i] = string(r[:width])
		} else {
			out[i] = l
		}
	}
	return out
}

func (m Model) renderMetaPopup() string {
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
		popupLines = append(popupLines, fmt.Sprintf("%s%s: %s", prefix, fieldLabel, value))
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
			"Esc       → cancel",
		)
	} else {
		popupLines = append(popupLines,
			"",
			"Press 'e' again to edit metadata.",
			"Press Esc to cancel.",
		)
	}

	return renderPopupBox("Metadata Editor", popupLines, m.width)
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
	var b strings.Builder

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

		separatorWidth := 6                        // " │ " + " │ "
		leftWidth := int(float64(m.width) * 0.22)  // 22%
		rightWidth := int(float64(m.width) * 0.33) // 33%
		middleWidth := m.width - leftWidth - rightWidth - separatorWidth

		if leftWidth < 12 {
			leftWidth = 12
		}
		if middleWidth < 25 {
			middleWidth = 25
		}
		if rightWidth < 25 {
			rightWidth = 25
		}

		height := m.viewportHeight

		treeLines := m.renderTreePanel(leftWidth, height)
		listLines := m.renderListPanel(middleWidth, height)
		prevLines := m.renderPreviewPanel(rightWidth, height)

		for i := 0; i < height; i++ {
			tl := ""
			if i < len(treeLines) {
				tl = treeLines[i]
			}
			ll := ""
			if i < len(listLines) {
				ll = listLines[i]
			}
			pl := ""
			if i < len(prevLines) {
				pl = prevLines[i]
			}

			line := padRight(tl, leftWidth) +
				"" +
				padRight(ll, middleWidth) +
				"" +
				padRight(pl, rightWidth)

			b.WriteString(line + "\n")
		}
	}

	// Footer
	if m.state == stateNewDir {
		fmt.Fprintf(&b, "\nCreate directory: %s\n", m.input.View())
	} else if m.state == stateRename {
		fmt.Fprintf(&b, "\nRename: %s\n", m.input.View())
	} else if m.state == stateEditMeta || m.state == stateMetaPreview {
		b.WriteString("\n")
		b.WriteString(m.renderMetaPopup())
		b.WriteString("\n")
	} else {
		b.WriteString("\n[j/k] move  [l/enter] enter/open  [h/backspace] up  [space] select  [d] cut  [p] paste  [a] mkdir  [r] rename  [e] edit meta  [D] delete  [q] quit\n")
	}

	if m.status != "" {
		fmt.Fprintf(&b, "\n%s\n", m.status)
	}

	return b.String()
}

func (m Model) metadataPreviewLines() []string {
	if m.meta == nil || m.currentMetaPath == "" {
		return nil
	}
	var md meta.Metadata
	if m.currentMeta != nil {
		md = *m.currentMeta
	}
	md.Path = m.currentMetaPath
	lines := make([]string, 0, metaFieldCount())
	for i := 0; i < metaFieldCount(); i++ {
		val := strings.TrimSpace(metadataFieldValue(md, i))
		if val == "" {
			val = "(empty)"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", metaFieldLabel(i), val))
	}
	return lines
}
