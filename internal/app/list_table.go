package app

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// fileColumns are the Files table column widths. A zero width means the column
// is hidden because the pane is too narrow for it.
type fileColumns struct {
	state, toRead, title, author, year, date, kind int
	// stateWords shows "Reading"/"Read"/"Unread"; when the pane is too narrow
	// the column falls back to the compact state icons.
	stateWords bool
}

const (
	fileTableTitleMin = 18
	fileColGap        = 1
)

// fileTableColumns fits the columns into width. Title takes the remaining
// space; optional columns drop out (Type, then Added, Author, Year) whenever
// keeping them would squeeze Title below its minimum.
func (m Model) fileTableColumns(width int) fileColumns {
	stateW := 1
	for _, v := range []string{readingStateUnread, readingStateReading, readingStateRead} {
		if w := ansi.StringWidth(m.readingStateIcon(v)); w > stateW {
			stateW = w
		}
	}
	iconW := stateW
	c := fileColumns{
		state:      len("Reading"),
		stateWords: true,
		toRead:     maxInt(1, ansi.StringWidth(m.toReadIcon())),
		year:       5, date: 6, kind: 4,
	}
	c.author = clampInt(width/5, 14, 24)

	fixed := func() int {
		total := c.state + fileColGap + c.toRead + fileColGap
		for _, w := range []int{c.author, c.year, c.date, c.kind} {
			if w > 0 {
				total += fileColGap + w
			}
		}
		return total
	}
	shrink := []func(){
		func() { c.kind = 0 },
		func() { c.date = 0 },
		func() { c.state, c.stateWords = iconW, false },
		func() { c.author = 0 },
		func() { c.year = 0 },
	}
	for _, step := range shrink {
		if width-fixed() >= fileTableTitleMin {
			break
		}
		step()
	}
	c.title = width - fixed()
	if c.title < 1 {
		c.title = 1
	}
	return c
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// fitCell truncates s to width display columns (with an ellipsis) and pads it.
func fitCell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", width-ansi.StringWidth(s))
}

func (m Model) dateColumnLabel() string {
	if m.cwdIsRecentlyOpened {
		return "Opened"
	}
	return "Added"
}

// renderFileTableHeader draws the column titles, marking the sorted column
// with ▴ (a narrow triangle: ▲ is ambiguous-width and misaligns CJK terminals).
func (m Model) renderFileTableHeader(c fileColumns) string {
	mark := func(label string, mode sortMode) string {
		if m.sortMode == mode {
			return label + "▴"
		}
		return label
	}
	titleLabel := mark("Title", sortByTitle)
	if m.sortMode == sortByName {
		titleLabel = "Name▴"
	}
	stateLabel := ""
	if c.stateWords {
		stateLabel = "State"
	}
	cells := []string{fitCell(stateLabel, c.state), fitCell("", c.toRead), fitCell(titleLabel, c.title)}
	if c.author > 0 {
		cells = append(cells, fitCell("Author", c.author))
	}
	if c.year > 0 {
		cells = append(cells, fitCell(mark("Year", sortByYear), c.year))
	}
	if c.date > 0 {
		cells = append(cells, fitCell(m.dateColumnLabel(), c.date))
	}
	if c.kind > 0 {
		cells = append(cells, fitCell("Type", c.kind))
	}
	return m.styles.Muted.Bold(true).Underline(true).Render(strings.Join(cells, " "))
}

// fileTableRow renders one entry. With colored set, cells get their own
// semantic colours; otherwise the row is plain text so the cursor/selection
// bar can recolour the whole line without clashing.
func (m Model) fileTableRow(c fileColumns, full string, e fs.DirEntry, colored bool) string {
	base := fallbackStyle(m.styles.List.Body, lipgloss.NewStyle())
	paint := func(style lipgloss.Style, s string) string {
		if !colored {
			return s
		}
		return style.Render(s)
	}
	fg := func(s lipgloss.Style) lipgloss.Style {
		return base.Foreground(s.GetForeground())
	}
	sep := paint(base, " ")

	if e.IsDir() {
		name := e.Name() + "/"
		if icon := strings.TrimSpace(m.entryIcon(true)); icon != "" {
			name = icon + " " + name
		}
		row := paint(base, fitCell("", c.state)) + sep + paint(base, fitCell("", c.toRead)) + sep +
			paint(fg(m.styles.Accent).Bold(true), fitCell(name, c.title))
		rest := c.author + c.year + c.date + c.kind
		for _, w := range []int{c.author, c.year, c.date, c.kind} {
			if w > 0 {
				rest += fileColGap
			}
		}
		return row + paint(base, strings.Repeat(" ", rest))
	}

	info, ok := m.entryInfo[full]
	if !ok {
		info.title = strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
	}

	var stateStyle lipgloss.Style
	switch normalizeReadingStateValue(info.state) {
	case readingStateReading:
		stateStyle = fg(m.styles.Warning).Bold(true)
	case readingStateRead:
		stateStyle = fg(m.styles.Success)
	default:
		stateStyle = fg(m.styles.Muted)
	}

	flags := paint(fg(m.styles.Warning), fitCell(boolGlyph(info.toRead, m.toReadIcon()), c.toRead))

	stateText := m.readingStateIcon(info.state)
	if c.stateWords {
		stateText = readingStateLabel(info.state)
	}
	row := paint(stateStyle, fitCell(stateText, c.state)) + sep +
		flags + sep +
		paint(base, fitCell(info.title, c.title))
	muted := fg(m.styles.Muted)
	if c.author > 0 {
		row += sep + paint(muted, fitCell(shortAuthor(info.author), c.author))
	}
	if c.year > 0 {
		row += sep + paint(muted, fitCell(dashIfEmpty(info.year), c.year))
	}
	if c.date > 0 {
		when := info.added
		if m.cwdIsRecentlyOpened {
			when = info.opened
		}
		row += sep + paint(muted, fitCell(relativeAge(when, time.Now()), c.date))
	}
	if c.kind > 0 {
		row += sep + paint(muted, fitCell(docKind(e.Name()), c.kind))
	}
	return row
}

func boolGlyph(on bool, glyph string) string {
	if !on {
		return ""
	}
	return glyph
}

func dashIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// shortAuthor condenses an author list to the first author's surname, plus
// "et al." when there are more authors: "Vaswani et al.", "He".
func shortAuthor(authors string) string {
	authors = strings.TrimSpace(authors)
	if authors == "" {
		return "-"
	}
	var names []string
	switch {
	case strings.Contains(authors, ";"):
		names = strings.Split(authors, ";")
	case strings.Contains(authors, " and "):
		names = strings.Split(authors, " and ")
	case strings.Count(authors, ",") == 1:
		// A single "Last, First" author.
		names = []string{authors}
	default:
		names = strings.Split(authors, ",")
	}
	first := strings.TrimSpace(names[0])
	surname := first
	if i := strings.Index(first, ","); i > 0 {
		surname = strings.TrimSpace(first[:i]) // "Last, First"
	} else if fields := strings.Fields(first); len(fields) > 0 {
		surname = fields[len(fields)-1] // "First Last"
	}
	if len(names) > 1 {
		return surname + " et al."
	}
	return surname
}

// relativeAge renders a compact age such as "today", "3d", "2w", "5mo", "1y".
func relativeAge(t, now time.Time) string {
	if t.IsZero() {
		return "-"
	}
	days := int(now.Sub(t).Hours() / 24)
	switch {
	case days < 1:
		return "today"
	case days < 14:
		return fmt.Sprintf("%dd", days)
	case days < 60:
		return fmt.Sprintf("%dw", days/7)
	case days < 365:
		return fmt.Sprintf("%dmo", days/30)
	default:
		return fmt.Sprintf("%dy", days/365)
	}
}

func docKind(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf":
		return "PDF"
	case ".epub":
		return "EPUB"
	case ".md", ".markdown":
		return "MD"
	default:
		return ""
	}
}
