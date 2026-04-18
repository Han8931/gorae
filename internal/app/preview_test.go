package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"gorae/internal/meta"
	"gorae/internal/theme"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestWindowSizeMsgRefreshesDirectoryPreview(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "preview-dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir preview dir: %v", err)
	}
	for i := 0; i < 10; i++ {
		writeDummyPDF(t, filepath.Join(dir, filepath.Base(dir)+"-"+string(rune('a'+i))+".pdf"))
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root entries: %v", err)
	}

	m := Model{
		root:           root,
		cwd:            root,
		entries:        entries,
		viewportHeight: 20,
		width:          80,
	}
	m.applyTheme(theme.Default())
	m.updateTextPreview()

	before := append([]string(nil), m.previewText...)
	if len(before) < 10 {
		t.Fatalf("expected full directory preview before resize, got %v", before)
	}

	updatedAny, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	if cmd != nil {
		t.Fatalf("expected synchronous preview refresh for directory resize")
	}

	updated, ok := updatedAny.(Model)
	if !ok {
		t.Fatalf("expected Model after resize, got %T", updatedAny)
	}
	if len(updated.previewText) >= len(before) {
		t.Fatalf("expected resized preview to shrink, before=%d after=%d", len(before), len(updated.previewText))
	}
	if got := updated.previewText[len(updated.previewText)-1]; !strings.Contains(got, "... 5 more") {
		t.Fatalf("expected truncated directory preview after resize, got %q", got)
	}
}

func TestRenderPreviewPanelPrefersMetadataOverTextFallback(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "paper.pdf")
	writeDummyPDF(t, file)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read preview test dir: %v", err)
	}
	store, err := meta.Open(filepath.Join(root, "meta.db"))
	if err != nil {
		t.Fatalf("open metadata store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	m := Model{
		cwd:             root,
		entries:         entries,
		meta:            store,
		currentMetaPath: canonicalPath(file),
		currentMeta: &meta.Metadata{
			Title:  "Test Driven Preview",
			Author: "Ada Lovelace",
			Year:   "2026",
		},
		previewText: []string{
			"Preview line one.",
			"Preview line two.",
		},
	}
	m.applyTheme(theme.Default())

	rendered := strings.Join(m.renderPreviewPanel(60, 16), "\n")
	rendered = ansiPattern.ReplaceAllString(rendered, "")

	if !strings.Contains(rendered, "Test Driven Preview") {
		t.Fatalf("expected metadata in preview panel, got %q", rendered)
	}
	if strings.Contains(rendered, "Preview line one.") {
		t.Fatalf("expected metadata-only fallback without preview split, got %q", rendered)
	}
}

func TestRenderPreviewPanelUsesBlankCanvasForGraphicPreview(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "paper.pdf")
	writeDummyPDF(t, file)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read preview test dir: %v", err)
	}

	m := Model{
		cwd:            root,
		entries:        entries,
		previewGraphic: "GRAPHIC",
		currentMeta: &meta.Metadata{
			Title: "Should Be Hidden",
		},
	}
	m.applyTheme(theme.Default())

	rendered := strings.Join(m.renderPreviewPanel(60, 16), "\n")
	rendered = ansiPattern.ReplaceAllString(rendered, "")

	if strings.Contains(rendered, "Should Be Hidden") {
		t.Fatalf("expected metadata to be hidden behind graphic preview, got %q", rendered)
	}
}

func TestViewAppendsGraphicPreviewOverlay(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "paper.pdf")
	writeDummyPDF(t, file)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read preview test dir: %v", err)
	}

	m := Model{
		cwd:            root,
		entries:        entries,
		width:          100,
		viewportHeight: 18,
		previewGraphic: "GRAPHIC",
	}
	m.applyTheme(theme.Default())

	leftWidth, middleWidth, _ := m.panelWidths()
	col := leftWidth + panelSeparatorWidth/2 + middleWidth + panelSeparatorWidth/2 + 3
	want := "\x1b7\x1b[5;" + strconv.Itoa(col) + "HGRAPHIC\x1b8"

	if view := m.View(); !strings.Contains(view, want) {
		t.Fatalf("expected graphic overlay %q in view", want)
	}
}

func TestViewClearsKittyGraphicPreviewWhenNoPreviewRemains(t *testing.T) {
	m := Model{
		width:               100,
		viewportHeight:      18,
		previewGraphicFmt:   "kitty",
		previewGraphicClear: true,
	}
	m.applyTheme(theme.Default())

	if view := m.View(); !strings.Contains(view, kittyDeletePreviewSequence()) {
		t.Fatalf("expected kitty delete sequence in view")
	}
}

func TestPreviewReadyMsgWithStaleSeqDoesNotOverwriteDirectoryPreview(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "subdir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write nested file: %v", err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root entries: %v", err)
	}

	m := Model{
		root:           root,
		cwd:            root,
		entries:        entries,
		cursor:         0,
		viewportHeight: 18,
		width:          100,
		previewSeq:     2,
		previewPath:    dir,
		previewText:    []string{"  nested.txt"},
	}

	updatedAny, cmd := m.Update(previewReadyMsg{
		seq:    1,
		path:   filepath.Join(root, "paper.pdf"),
		image:  []string{"IMAGE"},
		imageW: 20,
		imageH: 10,
	})
	if cmd != nil {
		t.Fatalf("expected no command for stale preview result")
	}

	updated, ok := updatedAny.(Model)
	if !ok {
		t.Fatalf("expected Model after stale preview result, got %T", updatedAny)
	}
	if got := strings.Join(updated.previewText, "\n"); got != "  nested.txt" {
		t.Fatalf("expected directory preview to remain intact, got %q", got)
	}
	if len(updated.previewImage) != 0 {
		t.Fatalf("expected stale image preview to be discarded, got %v", updated.previewImage)
	}
}

func TestUpdateTextPreviewAsyncClearsScreenWhenLeavingITermGraphicPreviewForDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "subdir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root entries: %v", err)
	}

	m := Model{
		root:              root,
		cwd:               root,
		entries:           entries,
		viewportHeight:    18,
		width:             100,
		previewGraphic:    "GRAPHIC",
		previewGraphicFmt: "iterm",
	}

	cmd := m.updateTextPreviewAsync()
	if cmd == nil {
		t.Fatal("expected clear-screen command when leaving iterm graphic preview")
	}
	if got := fmt.Sprintf("%T", cmd()); got != "tea.clearScreenMsg" {
		t.Fatalf("expected clear-screen message, got %s", got)
	}
	if len(m.previewText) == 0 {
		t.Fatal("expected directory preview after leaving iterm graphic preview")
	}
}

func TestPreviewReadyMsgClearsScreenWhenReplacingITermGraphicPreview(t *testing.T) {
	m := Model{
		previewSeq: 3,
	}

	updatedAny, cmd := m.Update(previewReadyMsg{
		seq:           3,
		path:          "/tmp/paper.pdf",
		graphic:       "NEW",
		graphicFormat: "iterm",
		imageW:        20,
		imageH:        10,
	})
	if cmd == nil {
		t.Fatal("expected clear-screen command when replacing iterm graphic preview")
	}
	if got := fmt.Sprintf("%T", cmd()); got != "tea.clearScreenMsg" {
		t.Fatalf("expected clear-screen message, got %s", got)
	}

	updated, ok := updatedAny.(Model)
	if !ok {
		t.Fatalf("expected Model after previewReadyMsg, got %T", updatedAny)
	}
	if updated.previewGraphic != "NEW" {
		t.Fatalf("expected updated graphic preview, got %q", updated.previewGraphic)
	}
}

func TestPreviewReadyMsgClearsScreenWhenApplyingKittyGraphicPreview(t *testing.T) {
	m := Model{
		previewSeq: 4,
	}

	updatedAny, cmd := m.Update(previewReadyMsg{
		seq:           4,
		path:          "/tmp/paper.pdf",
		graphic:       "KITTY",
		graphicFormat: "kitty",
		imageW:        20,
		imageH:        10,
	})
	if cmd == nil {
		t.Fatal("expected clear-screen command when applying kitty graphic preview")
	}
	if got := fmt.Sprintf("%T", cmd()); got != "tea.clearScreenMsg" {
		t.Fatalf("expected clear-screen message, got %s", got)
	}

	updated, ok := updatedAny.(Model)
	if !ok {
		t.Fatalf("expected Model after previewReadyMsg, got %T", updatedAny)
	}
	if updated.previewGraphic != "KITTY" {
		t.Fatalf("expected kitty graphic preview, got %q", updated.previewGraphic)
	}
}

func TestUpdateTextPreviewAsyncClearsScreenWhenEnteringITermGraphicPreview(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "paper.pdf")
	writeDummyPDF(t, file)

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root entries: %v", err)
	}

	t.Setenv("GORAE_PDF_PREVIEW_FORMAT", "iterm")
	m := Model{
		root:           root,
		cwd:            root,
		entries:        entries,
		viewportHeight: 18,
		width:          100,
	}

	cmd := m.updateTextPreviewAsync()
	if cmd == nil {
		t.Fatal("expected sequence command when entering iterm graphic preview")
	}
	if got := fmt.Sprintf("%T", cmd()); got != "tea.sequenceMsg" {
		t.Fatalf("expected sequence message, got %s", got)
	}
}

func TestEndKeyRequestsPreviewRefresh(t *testing.T) {
	root := t.TempDir()
	writeDummyPDF(t, filepath.Join(root, "a-first.pdf"))
	writeDummyPDF(t, filepath.Join(root, "z-last.pdf"))

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root entries: %v", err)
	}

	m := Model{
		root:           root,
		cwd:            root,
		entries:        entries,
		viewportHeight: 18,
		width:          100,
	}
	m.applyTheme(theme.Default())

	updatedAny, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if cmd == nil {
		t.Fatal("expected preview refresh command for end/G navigation")
	}
	updated, ok := updatedAny.(Model)
	if !ok {
		t.Fatalf("expected Model after G, got %T", updatedAny)
	}
	if updated.cursor != len(entries)-1 {
		t.Fatalf("expected cursor at last entry, got %d of %d", updated.cursor, len(entries))
	}
	if updated.entries[updated.cursor].Name() != "z-last.pdf" {
		t.Fatalf("expected cursor on last pdf entry, got %q", updated.entries[updated.cursor].Name())
	}
}

func TestPageDownRequestsPreviewRefresh(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 8; i++ {
		writeDummyPDF(t, filepath.Join(root, fmt.Sprintf("%02d.pdf", i)))
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root entries: %v", err)
	}

	m := Model{
		root:           root,
		cwd:            root,
		entries:        entries,
		viewportHeight: 10,
		width:          100,
	}
	m.applyTheme(theme.Default())

	updatedAny, cmd := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if cmd == nil {
		t.Fatal("expected preview refresh command for pgdown navigation")
	}
	updated, ok := updatedAny.(Model)
	if !ok {
		t.Fatalf("expected Model after pgdown, got %T", updatedAny)
	}
	if updated.cursor <= 0 {
		t.Fatalf("expected cursor to move down, got %d", updated.cursor)
	}
}

func TestCtrlDRequestsPreviewRefresh(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 8; i++ {
		writeDummyPDF(t, filepath.Join(root, fmt.Sprintf("%02d.pdf", i)))
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root entries: %v", err)
	}

	m := Model{
		root:           root,
		cwd:            root,
		entries:        entries,
		viewportHeight: 10,
		width:          100,
	}
	m.applyTheme(theme.Default())

	updatedAny, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Fatal("expected preview refresh command for ctrl+d navigation")
	}
	updated, ok := updatedAny.(Model)
	if !ok {
		t.Fatalf("expected Model after ctrl+d, got %T", updatedAny)
	}
	if updated.cursor <= 0 {
		t.Fatalf("expected cursor to move down, got %d", updated.cursor)
	}
}
