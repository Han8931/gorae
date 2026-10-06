package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestFileTableColumnsDropOptionalWhenNarrow(t *testing.T) {
	m := Model{}
	wide := m.fileTableColumns(100)
	if wide.author == 0 || wide.year == 0 || wide.date == 0 || wide.kind == 0 {
		t.Fatalf("wide table should show every column: %+v", wide)
	}
	narrow := m.fileTableColumns(30)
	if narrow.title < fileTableTitleMin {
		t.Fatalf("title squeezed below minimum: %+v", narrow)
	}
	if narrow.kind != 0 || narrow.date != 0 {
		t.Fatalf("narrow table should drop Type and Added first: %+v", narrow)
	}
	for _, w := range []int{100, 60, 45, 30, 22} {
		c := m.fileTableColumns(w)
		if got := lipgloss.Width(m.renderFileTableHeader(c)); got != w {
			t.Errorf("header width at %d = %d", w, got)
		}
	}
}

func TestShortAuthor(t *testing.T) {
	cases := map[string]string{
		"":                                  "-",
		"Kaiming He":                        "He",
		"Vaswani, Ashish":                   "Vaswani",
		"Ashish Vaswani and Noam Shazeer":   "Vaswani et al.",
		"Devlin, Jacob; Chang, Ming-Wei":    "Devlin et al.",
		"A. Vaswani, N. Shazeer, N. Parmar": "Vaswani et al.",
	}
	for in, want := range cases {
		if got := shortAuthor(in); got != want {
			t.Errorf("shortAuthor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRelativeAge(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{time.Hour, "today"},
		{3 * 24 * time.Hour, "3d"},
		{21 * 24 * time.Hour, "3w"},
		{150 * 24 * time.Hour, "5mo"},
		{800 * 24 * time.Hour, "2y"},
	}
	for _, c := range cases {
		if got := relativeAge(now.Add(-c.ago), now); got != c.want {
			t.Errorf("relativeAge(-%v) = %q, want %q", c.ago, got, c.want)
		}
	}
	if relativeAge(time.Time{}, now) != "-" {
		t.Error("zero time should render as -")
	}
}

func TestFileTableRowsAlignWithHeader(t *testing.T) {
	m := newTabTestModel(t)
	m.cwd = m.root + "/papers"
	m.loadEntries()
	m.entryInfo = map[string]entrySortInfo{
		m.cwd + "/a.md": {title: "注意力 Attention Is All You Need", author: "Ashish Vaswani; Noam Shazeer", year: "2017", state: readingStateReading, favorite: true},
	}
	c := m.fileTableColumns(100)
	header := lipgloss.Width(m.renderFileTableHeader(c))
	for _, colored := range []bool{true, false} {
		row := m.fileTableRow(c, m.cwd+"/a.md", m.entries[0], colored)
		if got := lipgloss.Width(row); got != header {
			t.Fatalf("row width %d != header width %d (colored=%v): %q", got, header, colored, row)
		}
		if !strings.Contains(row, "2017") || !strings.Contains(row, "Vaswani et al.") {
			t.Fatalf("row missing metadata: %q", row)
		}
	}
}
