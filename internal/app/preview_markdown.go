package app

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reLinkText   = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	reWikiText   = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	reBold       = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
)

// renderMarkdownPreview tries glow first; falls back to the custom renderer.
func renderMarkdownPreview(path string, width int, styles mdStyles) ([]panelLine, error) {
	if lines, err := renderWithGlow(path, width); err == nil {
		return lines, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return renderMarkdownCustom(string(data), width, styles), nil
}

// renderWithGlow renders the file using the `glow` CLI if available.
func renderWithGlow(path string, width int) ([]panelLine, error) {
	if _, err := exec.LookPath("glow"); err != nil {
		return nil, err
	}
	cmd := exec.Command("glow", "--style", "dark", "--width", fmt.Sprintf("%d", width), path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	raw := strings.TrimRight(out.String(), "\n")
	rawLines := strings.Split(raw, "\n")
	result := make([]panelLine, 0, len(rawLines))
	for _, l := range rawLines {
		result = append(result, panelLine{text: l, kind: panelLineImage})
	}
	return result, nil
}

// renderMarkdownCustom renders markdown text with lipgloss colors line by line.
func renderMarkdownCustom(text string, width int, styles mdStyles) []panelLine {
	if width <= 0 {
		width = 80
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	result := make([]panelLine, 0, len(lines))

	inFence := false
	var tableBuf []string

	flushTable := func() {
		if len(tableBuf) > 0 {
			result = append(result, renderMarkdownTable(tableBuf, width, styles)...)
			tableBuf = nil
		}
	}

	for _, raw := range lines {
		// Fenced code block toggle
		if strings.HasPrefix(raw, "```") {
			flushTable()
			inFence = !inFence
			lang := strings.TrimSpace(strings.TrimPrefix(raw, "```"))
			label := "─── code"
			if !inFence {
				label = "───"
			} else if lang != "" {
				label = "─── " + lang
			}
			result = append(result, panelLine{text: styles.CodeBlock.Render(mdTruncate(label, width)), kind: panelLineImage})
			continue
		}
		if inFence {
			result = append(result, panelLine{text: styles.CodeBlock.Render(mdTruncate(raw, width)), kind: panelLineImage})
			continue
		}

		// Table rows — buffer consecutive lines that look like table rows
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "|") {
			tableBuf = append(tableBuf, raw)
			continue
		}
		flushTable()

		// Headings
		if strings.HasPrefix(raw, "###### ") || strings.HasPrefix(raw, "##### ") ||
			strings.HasPrefix(raw, "#### ") || strings.HasPrefix(raw, "### ") {
			content := strings.TrimLeft(raw, "# ")
			result = append(result, panelLine{text: styles.H3.Render(mdTruncate(content, width)), kind: panelLineImage})
			continue
		}
		if strings.HasPrefix(raw, "## ") {
			content := strings.TrimPrefix(raw, "## ")
			result = append(result, panelLine{text: styles.H2.Render(mdTruncate(content, width)), kind: panelLineImage})
			continue
		}
		if strings.HasPrefix(raw, "# ") {
			content := strings.TrimPrefix(raw, "# ")
			result = append(result, panelLine{text: styles.H1.Render(mdTruncate(content, width)), kind: panelLineImage})
			continue
		}

		// Horizontal rules
		if trimmed == "---" || trimmed == "***" || trimmed == "___" ||
			trimmed == "- - -" || trimmed == "* * *" {
			hr := strings.Repeat("─", width)
			result = append(result, panelLine{text: styles.HR.Render(hr), kind: panelLineImage})
			continue
		}

		// Blockquote
		if strings.HasPrefix(raw, "> ") || raw == ">" {
			content := strings.TrimPrefix(strings.TrimPrefix(raw, "> "), ">")
			plain := stripInlineMarkdown(content)
			result = append(result, panelLine{text: styles.Blockquote.Render(mdTruncate("│ "+plain, width)), kind: panelLineImage})
			continue
		}

		// Regular body line — word-wrap then apply inline styles per segment
		for _, segment := range wrapTextToWidth(raw, width) {
			result = append(result, panelLine{text: applyInlineStyles(segment, width, styles), kind: panelLineImage})
		}
	}
	flushTable()

	return result
}

// renderMarkdownTable parses and renders a slice of markdown table rows.
func renderMarkdownTable(rows []string, width int, styles mdStyles) []panelLine {
	parseRow := func(row string) []string {
		row = strings.TrimSpace(row)
		row = strings.Trim(row, "|")
		cells := strings.Split(row, "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		return cells
	}

	isSepRow := func(cells []string) bool {
		for _, c := range cells {
			c = strings.Trim(c, ": ")
			if len(c) == 0 {
				return false
			}
			for _, ch := range c {
				if ch != '-' {
					return false
				}
			}
		}
		return true
	}

	parsed := make([][]string, 0, len(rows))
	for _, r := range rows {
		parsed = append(parsed, parseRow(r))
	}

	// Find separator row
	sepIdx := -1
	for i, cells := range parsed {
		if isSepRow(cells) {
			sepIdx = i
			break
		}
	}

	// Max column count
	cols := 0
	for _, cells := range parsed {
		if len(cells) > cols {
			cols = len(cells)
		}
	}
	if cols == 0 {
		return nil
	}

	// Column widths
	colW := make([]int, cols)
	for _, cells := range parsed {
		if len(cells) == 1 && isSepRow(cells) {
			continue
		}
		for j, c := range cells {
			if j < cols && len(c) > colW[j] {
				colW[j] = len(c)
			}
		}
	}
	// Minimum width of 1
	for j := range colW {
		if colW[j] < 1 {
			colW[j] = 1
		}
	}

	pad := func(s string, w int) string {
		if len(s) >= w {
			return s
		}
		return s + strings.Repeat(" ", w-len(s))
	}

	var result []panelLine
	for i, cells := range parsed {
		if i == sepIdx {
			// Draw separator line between header and body
			var sb strings.Builder
			for j := 0; j < cols; j++ {
				if j > 0 {
					sb.WriteString("─┼─")
				}
				sb.WriteString(strings.Repeat("─", colW[j]))
			}
			result = append(result, panelLine{text: styles.HR.Render(sb.String()), kind: panelLineImage})
			continue
		}
		isHeader := sepIdx > 0 && i < sepIdx
		var sb strings.Builder
		for j := 0; j < cols; j++ {
			if j > 0 {
				sb.WriteString(" │ ")
			}
			cell := ""
			if j < len(cells) {
				cell = cells[j]
			}
			sb.WriteString(pad(cell, colW[j]))
		}
		if isHeader {
			result = append(result, panelLine{text: styles.H3.Render(sb.String()), kind: panelLineImage})
		} else {
			result = append(result, panelLine{text: styles.Body.Render(sb.String()), kind: panelLineImage})
		}
	}
	return result
}

// applyInlineStyles applies code, link, bold, and italic styling within a body line.
func applyInlineStyles(line string, width int, styles mdStyles) string {
	if line == "" {
		return ""
	}

	type span struct {
		start, end int
		display    string
		style      lipgloss.Style
	}

	var spans []span

	addSpans := func(re *regexp.Regexp, s lipgloss.Style, displayFn func([]string) string) {
		for _, m := range re.FindAllStringSubmatchIndex(line, -1) {
			sub := re.FindStringSubmatch(line[m[0]:m[1]])
			spans = append(spans, span{
				start:   m[0],
				end:     m[1],
				display: displayFn(sub),
				style:   s,
			})
		}
	}

	addSpans(reInlineCode, styles.Code, func(sm []string) string {
		if len(sm) > 1 {
			return sm[1]
		}
		return sm[0]
	})
	addSpans(reLinkText, styles.Link, func(sm []string) string {
		if len(sm) > 1 && sm[1] != "" {
			return sm[1]
		}
		return sm[0]
	})
	addSpans(reWikiText, styles.Link, func(sm []string) string {
		if len(sm) > 1 {
			return sm[1]
		}
		return sm[0]
	})
	addSpans(reBold, lipgloss.NewStyle().Bold(true), func(sm []string) string {
		for _, g := range sm[1:] {
			if g != "" {
				return g
			}
		}
		return sm[0]
	})

	if len(spans) == 0 {
		return styles.Body.Render(mdTruncate(line, width))
	}

	// Sort spans by start position (insertion sort — lines are short)
	for i := 1; i < len(spans); i++ {
		for j := i; j > 0 && spans[j].start < spans[j-1].start; j-- {
			spans[j], spans[j-1] = spans[j-1], spans[j]
		}
	}

	var b strings.Builder
	pos := 0
	for _, sp := range spans {
		if sp.start < pos {
			continue // overlapping, skip
		}
		if sp.start > pos {
			b.WriteString(styles.Body.Render(line[pos:sp.start]))
		}
		b.WriteString(sp.style.Render(sp.display))
		pos = sp.end
	}
	if pos < len(line) {
		b.WriteString(styles.Body.Render(line[pos:]))
	}
	return b.String()
}

func mdTruncate(s string, width int) string {
	if width <= 0 {
		return s
	}
	return runewidth.Truncate(s, width, "")
}

// stripInlineMarkdown removes inline markdown syntax for contexts where we
// apply a block-level style (blockquotes) and don't want nested spans.
func stripInlineMarkdown(s string) string {
	s = reInlineCode.ReplaceAllString(s, "$1")
	s = reLinkText.ReplaceAllString(s, "$1")
	s = reWikiText.ReplaceAllString(s, "$1")
	s = reBold.ReplaceAllString(s, func() string {
		// handled by ReplaceAllStringFunc below
		return "$1$2"
	}())
	return s
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
