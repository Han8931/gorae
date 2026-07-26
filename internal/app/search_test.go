package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectPDFFilesSkipsHelperDirs(t *testing.T) {
	root := t.TempDir()

	mainPDF := filepath.Join(root, "paper.pdf")
	writeDummyPDF(t, mainPDF)

	recentDir := filepath.Join(root, "Recently Added")
	if err := os.MkdirAll(recentDir, 0o755); err != nil {
		t.Fatalf("mkdir recently added: %v", err)
	}
	writeDummyPDF(t, filepath.Join(recentDir, "dup.pdf"))

	files, warnings, err := collectPDFFiles(root, []string{recentDir})
	if err != nil {
		t.Fatalf("collect pdfs: %v", err)
	}
	if len(warnings) > 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(files) != 1 || files[0] != mainPDF {
		t.Fatalf("expected only %s, got %v", mainPDF, files)
	}
}

func TestCollectPDFFilesSkipsMultipleHelperDirs(t *testing.T) {
	root := t.TempDir()
	writeDummyPDF(t, filepath.Join(root, "base.pdf"))

	helpers := []string{
		filepath.Join(root, "Recently Read"),
		filepath.Join(root, "Favorites"),
		filepath.Join(root, "To Read"),
	}
	for _, dir := range helpers {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir helper %s: %v", dir, err)
		}
		writeDummyPDF(t, filepath.Join(dir, "dup.pdf"))
	}

	files, _, err := collectPDFFiles(root, helpers)
	if err != nil {
		t.Fatalf("collect pdfs: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(files), files)
	}
	if files[0] != filepath.Join(root, "base.pdf") {
		t.Fatalf("unexpected file returned: %v", files)
	}
}

func TestPageForOffset(t *testing.T) {
	// Two form feeds → three pages. "page1\fpage2\fpage3"
	text := "page1\fpage2\fpage3"
	cases := []struct {
		offset int
		want   int
	}{
		{0, 1},                     // start of page 1
		{4, 1},                     // within page 1
		{len("page1\f"), 2},        // just after first form feed → page 2
		{len("page1\fpage2\f"), 3}, // after second form feed → page 3
		{len(text), 3},             // end of text
		{-1, 0},                    // out of range
		{len(text) + 1, 0},         // out of range
	}
	for _, c := range cases {
		if got := pageForOffset(text, c.offset); got != c.want {
			t.Errorf("pageForOffset(offset=%d) = %d, want %d", c.offset, got, c.want)
		}
	}
}

func TestBuildContentMatchMultipleHits(t *testing.T) {
	// "cat" appears 3 times across 2 pages: two on page 1, one on page 2.
	text := "a cat and a cat sat\fthen the cat ran"
	match, ok := buildContentMatch("/tmp/x.pdf", text, "cat", false, 80, true, nil)
	if !ok {
		t.Fatal("expected a match")
	}
	if match.MatchCount != 3 {
		t.Fatalf("MatchCount = %d, want 3", match.MatchCount)
	}
	if len(match.Snippets) != 3 {
		t.Fatalf("len(Snippets) = %d, want 3", len(match.Snippets))
	}
	wantPages := []int{1, 1, 2}
	if len(match.HitPages) != len(wantPages) {
		t.Fatalf("len(HitPages) = %d, want %d", len(match.HitPages), len(wantPages))
	}
	for i, want := range wantPages {
		if match.HitPages[i] != want {
			t.Errorf("HitPages[%d] = %d, want %d", i, match.HitPages[i], want)
		}
	}
	// Page-annotated snippets carry a "[p.N]" prefix for paginated documents.
	if !strings.HasPrefix(match.Snippets[2], "[p.2]") {
		t.Errorf("expected 3rd snippet to start with [p.2], got %q", match.Snippets[2])
	}
}

func TestBuildContentMatchSurfacesEveryHit(t *testing.T) {
	// Many occurrences (well over the old display cap of 6) must all become
	// navigable hits, not be truncated — this is the 68-hits-but-only-6-shown bug.
	var sb strings.Builder
	total := 68
	for i := 0; i < total; i++ {
		sb.WriteString("needle ")
	}
	match, ok := buildContentMatch("/tmp/x.epub", sb.String(), "needle", false, 80, false, nil)
	if !ok {
		t.Fatal("expected a match")
	}
	if match.MatchCount != total {
		t.Fatalf("MatchCount = %d, want %d", match.MatchCount, total)
	}
	if len(match.Snippets) != total {
		t.Fatalf("len(Snippets) = %d, want %d (every hit navigable)", len(match.Snippets), total)
	}
	if len(match.HitPages) != total {
		t.Fatalf("len(HitPages) = %d, want %d", len(match.HitPages), total)
	}
	// Non-paginated document → all pages unknown (0), no "[p.N]" prefix.
	for i, p := range match.HitPages {
		if p != 0 {
			t.Errorf("HitPages[%d] = %d, want 0 for non-paginated", i, p)
		}
	}
}

func TestBuildContentMatchCapsAtSafetyLimit(t *testing.T) {
	var sb strings.Builder
	total := maxHitsPerFile + 50
	for i := 0; i < total; i++ {
		sb.WriteString("needle ")
	}
	match, ok := buildContentMatch("/tmp/x.epub", sb.String(), "needle", false, 80, false, nil)
	if !ok {
		t.Fatal("expected a match")
	}
	// MatchCount still reports the true total, but hits are capped for safety.
	if match.MatchCount != total {
		t.Fatalf("MatchCount = %d, want %d", match.MatchCount, total)
	}
	if len(match.HitPages) != maxHitsPerFile {
		t.Fatalf("len(HitPages) = %d, want %d (safety cap)", len(match.HitPages), maxHitsPerFile)
	}
}

func TestBuildContentMatchNoHit(t *testing.T) {
	if _, ok := buildContentMatch("/tmp/x.pdf", "nothing here", "absent", false, 80, true, nil); ok {
		t.Fatal("expected no match for absent query")
	}
}

func TestViewerPageArgs(t *testing.T) {
	cases := []struct {
		viewer string
		page   int
		want   []string
	}{
		{"zathura", 12, []string{"--page", "12"}},
		{"/usr/bin/zathura", 3, []string{"--page", "3"}},
		{"sioyek", 5, []string{"--page", "5"}},
		{"xdg-open", 5, nil},
		{"zathura", 0, nil},
	}
	for _, c := range cases {
		got := viewerPageArgs(c.viewer, c.page)
		if len(got) != len(c.want) {
			t.Errorf("viewerPageArgs(%q, %d) = %v, want %v", c.viewer, c.page, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("viewerPageArgs(%q, %d)[%d] = %q, want %q", c.viewer, c.page, i, got[i], c.want[i])
			}
		}
	}
}

func TestSearchResultsHeightsFitViewport(t *testing.T) {
	// The list + detail bodies plus all chrome (borders, header, warnings,
	// quick-filter line) must never exceed the viewport, or the results list
	// scrolls its cursor past the visible area without the viewport following.
	cases := []struct {
		name        string
		windowH     int
		warnings    int
		quickFilter quickFilterMode
	}{
		{"small, no warnings", 24, 0, quickFilterNone},
		{"tall, no warnings", 60, 0, quickFilterNone},
		{"with warnings", 40, 12, quickFilterNone},
		{"warnings + quick filter", 40, 12, quickFilterFavorites},
		{"many warnings, short window", 24, 50, quickFilterToRead},
	}
	for _, c := range cases {
		m := &Model{windowHeight: c.windowH, viewportHeight: c.windowH - 5, quickFilter: c.quickFilter}
		m.searchWarnings = make([]string, c.warnings)

		list, detail := m.searchResultsHeights()
		chrome := m.searchResultsChromeHeight()
		total := list + detail + chrome
		if total > m.viewportHeight {
			t.Errorf("%s: total %d (list %d + detail %d + chrome %d) exceeds viewport %d",
				c.name, total, list, detail, chrome, m.viewportHeight)
		}
		if list < 1 {
			t.Errorf("%s: list height %d must be at least 1", c.name, list)
		}
	}
}

func writeDummyPDF(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatalf("write pdf %s: %v", path, err)
	}
}
