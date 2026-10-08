package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	textinput "github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Han8931/gorae/internal/config"
	"github.com/Han8931/gorae/internal/meta"
	"github.com/Han8931/gorae/internal/theme"
)

// newMetaEditTestModel builds a Model with a real meta store so the editing
// view's save paths can be exercised end to end.
func newMetaEditTestModel(t *testing.T) (*Model, string) {
	t.Helper()
	base := t.TempDir()
	metaDir := filepath.Join(base, "meta")
	store, err := meta.Open(filepath.Join(metaDir, "meta.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = defaultInputCharLimit

	m := &Model{
		cfg:   &config.Config{MetaDir: metaDir},
		meta:  store,
		input: input,
	}
	return m, base
}

// The editor file has no favorite/to-read fields, but Upsert writes every
// column — so the save path must carry the stored flags and timestamps over
// instead of resetting them.
func TestApplyMetadataEditPreservesFlagsAndTimestamps(t *testing.T) {
	m, base := newMetaEditTestModel(t)
	target := filepath.Join(base, "paper.pdf")
	ctx := context.Background()

	added := time.Now().Add(-72 * time.Hour).Truncate(time.Second)
	before := meta.Metadata{
		Path:         target,
		Title:        "Old title",
		Author:       "Vaswani",
		Favorite:     true,
		ToRead:       true,
		ReadingState: readingStateReading,
		AddedAt:      added,
	}
	if err := m.meta.Upsert(ctx, &before); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}

	tmp := filepath.Join(base, "edit.json")
	edited := `{"title":"New title","author":"Vaswani","year":"2017","reading_state":"read"}`
	if err := os.WriteFile(tmp, []byte(edited), 0o644); err != nil {
		t.Fatalf("write edit file: %v", err)
	}

	if status := m.applyMetadataEdit(metadataEditFinishedMsg{tmpPath: tmp, targetPath: target}); status != "Metadata saved" {
		t.Fatalf("applyMetadataEdit status = %q, want %q", status, "Metadata saved")
	}

	after, err := m.meta.Get(ctx, target)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after == nil {
		t.Fatal("metadata row missing after edit")
	}
	if after.Title != "New title" || after.Year != "2017" {
		t.Fatalf("edited fields not saved: title = %q, year = %q", after.Title, after.Year)
	}
	if !after.Favorite {
		t.Error("Favorite was cleared by the metadata edit")
	}
	if !after.ToRead {
		t.Error("To-read was cleared by the metadata edit")
	}
	if got := normalizeReadingStateValue(after.ReadingState); got != readingStateRead {
		t.Errorf("ReadingState = %q, want %q", got, readingStateRead)
	}
	if !after.AddedAt.Equal(added) {
		t.Errorf("AddedAt = %v, want %v", after.AddedAt, added)
	}
}

// A failed or empty editor run must not wipe the stored row.
func TestApplyMetadataEditRejectsBadInput(t *testing.T) {
	m, base := newMetaEditTestModel(t)
	target := filepath.Join(base, "paper.pdf")
	ctx := context.Background()

	before := meta.Metadata{Path: target, Title: "Keep me", Favorite: true}
	if err := m.meta.Upsert(ctx, &before); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}

	bad := filepath.Join(base, "bad.json")
	if err := os.WriteFile(bad, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write bad file: %v", err)
	}

	cases := []struct {
		name string
		msg  metadataEditFinishedMsg
	}{
		{"no data returned", metadataEditFinishedMsg{targetPath: target}},
		{"unknown target", metadataEditFinishedMsg{tmpPath: bad}},
		{"unparseable json", metadataEditFinishedMsg{tmpPath: bad, targetPath: target}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if status := m.applyMetadataEdit(tc.msg); status == "Metadata saved" {
				t.Fatal("applyMetadataEdit reported success on invalid input")
			}
			after, err := m.meta.Get(ctx, target)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if after == nil || after.Title != "Keep me" || !after.Favorite {
				t.Fatalf("stored row changed: %+v", after)
			}
		})
	}
}

// newMetaEditorModel builds a Model sitting in the editing view on one seeded
// document, sized so the panel has room for every row.
func newMetaEditorModel(t *testing.T, md meta.Metadata) *Model {
	t.Helper()
	m, base := newMetaEditTestModel(t)
	target := filepath.Join(base, "attention.pdf")
	if err := os.WriteFile(target, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	md.Path = target
	if err := m.meta.Upsert(context.Background(), &md); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}
	m.root, m.cwd = base, base
	m.entries, _ = os.ReadDir(base)
	m.width = 100
	m.viewportHeight = 30
	m.applyTheme(theme.Default())
	m.openMetaEditor(target)
	if m.state != stateMetaPreview {
		t.Fatalf("openMetaEditor state = %v, want stateMetaPreview", m.state)
	}
	return m
}

func metaRowIndexFor(t *testing.T, m *Model, label string) int {
	t.Helper()
	for i, row := range m.metaRows() {
		if row.label == label {
			return i
		}
	}
	t.Fatalf("no row labelled %q", label)
	return -1
}

// key feeds one keypress through the editing view's handlers.
func (m *Model) key(t *testing.T, k string) {
	t.Helper()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	switch k {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEscape}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		msg = tea.KeyMsg{Type: tea.KeyShiftTab}
	case "backspace":
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	}
	var updated tea.Model
	if m.state == stateMetaField {
		updated, _ = m.handleMetaFieldKey(msg, k)
	} else {
		updated, _ = m.handleMetaEditorKey(k)
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("handler returned %T, want Model", updated)
	}
	*m = next
}

func TestMetaEditorEditsFieldInline(t *testing.T) {
	m := newMetaEditorModel(t, meta.Metadata{Title: "Old", Favorite: true})

	m.setMetaRow(metaRowIndexFor(t, m, "Title"))
	m.key(t, "enter")
	if m.state != stateMetaField {
		t.Fatalf("state after enter = %v, want stateMetaField", m.state)
	}
	if got := m.input.Value(); got != "Old" {
		t.Fatalf("prompt seeded with %q, want %q", got, "Old")
	}
	// The abstract is routinely longer than the shared prompt's default limit.
	if m.input.CharLimit != 0 {
		t.Errorf("CharLimit = %d, want 0 (unlimited) while editing", m.input.CharLimit)
	}

	m.input.SetValue("Attention Is All You Need")
	m.key(t, "enter")
	if m.state != stateMetaPreview {
		t.Fatalf("state after commit = %v, want stateMetaPreview", m.state)
	}
	if m.input.CharLimit != defaultInputCharLimit {
		t.Errorf("CharLimit = %d, want it restored to %d", m.input.CharLimit, defaultInputCharLimit)
	}

	stored, err := m.meta.Get(context.Background(), m.metaEditingPath)
	if err != nil || stored == nil {
		t.Fatalf("get: %v (row %v)", err, stored)
	}
	if stored.Title != "Attention Is All You Need" {
		t.Errorf("stored title = %q", stored.Title)
	}
	if !stored.Favorite {
		t.Error("inline edit cleared Favorite")
	}
}

func TestMetaEditorTabMovesToNextTextField(t *testing.T) {
	m := newMetaEditorModel(t, meta.Metadata{})

	m.setMetaRow(metaRowIndexFor(t, m, "Title"))
	m.key(t, "enter")
	m.input.SetValue("A paper")
	m.key(t, "tab")

	if m.state != stateMetaField {
		t.Fatalf("state after tab = %v, want stateMetaField (still editing)", m.state)
	}
	row, _ := m.currentMetaRow()
	if row.label != "Author" {
		t.Fatalf("tab moved to %q, want Author", row.label)
	}
	if got := m.input.Value(); got != "" {
		t.Errorf("prompt for the next field = %q, want empty", got)
	}

	stored, _ := m.meta.Get(context.Background(), m.metaEditingPath)
	if stored == nil || stored.Title != "A paper" {
		t.Fatalf("tab did not save the previous field: %+v", stored)
	}
}

func TestMetaEditorEscapeKeepsStoredValue(t *testing.T) {
	m := newMetaEditorModel(t, meta.Metadata{Title: "Keep me"})

	m.setMetaRow(metaRowIndexFor(t, m, "Title"))
	m.key(t, "enter")
	m.input.SetValue("Discard me")
	m.key(t, "esc")

	if m.state != stateMetaPreview {
		t.Fatalf("state after esc = %v, want stateMetaPreview", m.state)
	}
	stored, _ := m.meta.Get(context.Background(), m.metaEditingPath)
	if stored == nil || stored.Title != "Keep me" {
		t.Fatalf("esc saved the cancelled value: %+v", stored)
	}
	if m.metaDraft.Title != "Keep me" {
		t.Errorf("draft kept the cancelled value: %q", m.metaDraft.Title)
	}
}

func TestMetaEditorRejectsYearWithoutFourDigits(t *testing.T) {
	m := newMetaEditorModel(t, meta.Metadata{Year: "2017"})

	m.setMetaRow(metaRowIndexFor(t, m, "Year"))
	m.key(t, "enter")
	m.input.SetValue("last year")
	m.key(t, "enter")

	if m.state != stateMetaField {
		t.Fatalf("state = %v, want stateMetaField (prompt stays open on a bad year)", m.state)
	}
	if got := m.input.Value(); got != "last year" {
		t.Errorf("rejected input was discarded: %q", got)
	}
	stored, _ := m.meta.Get(context.Background(), m.metaEditingPath)
	if stored == nil || stored.Year != "2017" {
		t.Fatalf("bad year was saved: %+v", stored)
	}
}

func TestMetaEditorTogglesFlagsAndReadingState(t *testing.T) {
	m := newMetaEditorModel(t, meta.Metadata{Title: "Paper"})
	ctx := context.Background()

	m.key(t, "f")
	if row, _ := m.currentMetaRow(); row.label != "Favorite" {
		t.Errorf("'f' left the cursor on %q, want Favorite", row.label)
	}
	if stored, _ := m.meta.Get(ctx, m.metaEditingPath); stored == nil || !stored.Favorite {
		t.Fatalf("'f' did not set Favorite: %+v", stored)
	}

	m.key(t, "t")
	if stored, _ := m.meta.Get(ctx, m.metaEditingPath); stored == nil || !stored.ToRead {
		t.Fatalf("'t' did not set To-read: %+v", stored)
	}

	m.key(t, "r")
	if stored, _ := m.meta.Get(ctx, m.metaEditingPath); stored == nil || normalizeReadingStateValue(stored.ReadingState) != readingStateReading {
		t.Fatalf("'r' did not advance the reading state: %+v", stored)
	}
	// Flags must survive the state change and vice versa.
	stored, _ := m.meta.Get(ctx, m.metaEditingPath)
	if !stored.Favorite || !stored.ToRead || stored.Title != "Paper" {
		t.Fatalf("reading state change disturbed other fields: %+v", stored)
	}
}

func TestMetaEditorNavigationStaysInRange(t *testing.T) {
	m := newMetaEditorModel(t, meta.Metadata{Title: "Paper"})
	last := m.metaRowCount() - 1

	m.key(t, "G")
	if m.metaRowIndex != last {
		t.Errorf("G landed on %d, want %d", m.metaRowIndex, last)
	}
	m.key(t, "j") // must not run past the end
	if m.metaRowIndex != last {
		t.Errorf("j past the end moved to %d, want %d", m.metaRowIndex, last)
	}
	m.key(t, "tab") // tab wraps
	if m.metaRowIndex != 0 {
		t.Errorf("tab at the end moved to %d, want 0 (wrap)", m.metaRowIndex)
	}
	m.key(t, "k")
	if m.metaRowIndex != 0 {
		t.Errorf("k before the start moved to %d, want 0", m.metaRowIndex)
	}
	m.key(t, "esc")
	if m.state != stateNormal || m.metaEditingPath != "" {
		t.Errorf("esc did not close the editor: state = %v, path = %q", m.state, m.metaEditingPath)
	}
}

// The panel must keep its frame intact at every scroll position: the old popup
// sliced the border rows along with the content.
func TestMetaEditorPanelKeepsFrameWhileScrolling(t *testing.T) {
	long := strings.Repeat("a long abstract sentence. ", 40)
	m := newMetaEditorModel(t, meta.Metadata{Title: "Paper", Abstract: long})
	m.viewportHeight = 14
	_, middle, _ := m.panelWidths()

	for _, row := range []int{0, 3, m.metaRowCount() - 1} {
		m.setMetaRow(row)
		block := m.renderMetaEditorPanel(middle, m.paneHeight())
		if len(block) != m.paneHeight() {
			t.Fatalf("row %d: panel height = %d, want %d", row, len(block), m.paneHeight())
		}
		first := ansiPattern.ReplaceAllString(block[0], "")
		last := ansiPattern.ReplaceAllString(block[len(block)-1], "")
		if !strings.HasPrefix(first, m.borderChars.TopLeft) {
			t.Errorf("row %d: top border missing, got %q", row, first)
		}
		if !strings.HasPrefix(last, m.borderChars.BottomLeft) {
			t.Errorf("row %d: bottom border missing, got %q", row, last)
		}
		header := ansiPattern.ReplaceAllString(block[1], "")
		if !strings.Contains(header, "attention.pdf") {
			t.Errorf("row %d: header lost the file name, got %q", row, header)
		}
	}
}

// The reading state and both flags are editable, so they must be on screen.
func TestMetaEditorPanelShowsStatusRows(t *testing.T) {
	m := newMetaEditorModel(t, meta.Metadata{Title: "Paper", Favorite: true, ReadingState: readingStateRead})
	_, middle, _ := m.panelWidths()
	m.setMetaRow(m.metaRowCount() - 1) // scroll to the bottom

	var b strings.Builder
	for _, line := range m.renderMetaEditorPanel(middle, m.paneHeight()) {
		b.WriteString(ansiPattern.ReplaceAllString(line, "") + "\n")
	}
	out := b.String()
	for _, want := range []string{"Reading", "Read", "Favorite", "Yes", "To-read"} {
		if !strings.Contains(out, want) {
			t.Errorf("panel is missing %q:\n%s", want, out)
		}
	}
}

// The wheel must scroll the editing view, not the file cursor hidden behind it.
func TestMetaEditorWheelDoesNotMoveFileCursor(t *testing.T) {
	long := strings.Repeat("a long abstract sentence. ", 40)
	m := newMetaEditorModel(t, meta.Metadata{Title: "Paper", Abstract: long})
	m.viewportHeight = 12
	m.cursor = 0
	m.setMetaRow(metaRowIndexFor(t, m, "Abstract"))
	before := m.metaPopupOffset

	updated, _ := m.handleMouse(tea.MouseMsg{Type: tea.MouseWheelDown})
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("handleMouse returned %T", updated)
	}
	if next.cursor != 0 {
		t.Errorf("wheel moved the file cursor to %d, want 0", next.cursor)
	}
	if next.metaPopupOffset <= before {
		t.Errorf("wheel did not scroll the editing view: offset %d -> %d", before, next.metaPopupOffset)
	}
}

// Editing a field must also refresh the detail pane's cached record.
func TestMetaEditorSaveRefreshesDetailPane(t *testing.T) {
	m := newMetaEditorModel(t, meta.Metadata{Title: "Old"})
	m.currentMetaPath = m.metaEditingPath
	m.currentMeta = &meta.Metadata{Path: m.metaEditingPath, Title: "Old"}

	m.setMetaRow(metaRowIndexFor(t, m, "Title"))
	m.key(t, "enter")
	m.input.SetValue("New")
	m.key(t, "enter")

	if m.currentMeta == nil || m.currentMeta.Title != "New" {
		t.Fatalf("detail pane still shows %+v", m.currentMeta)
	}
}
