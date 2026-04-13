package app

import (
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

func TestRenderPreviewPanelShowsTextPreviewAlongsideMetadata(t *testing.T) {
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
	if !strings.Contains(rendered, "Preview line one.") {
		t.Fatalf("expected preview text in preview panel, got %q", rendered)
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
