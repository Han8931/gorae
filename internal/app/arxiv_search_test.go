package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Han8931/gorae/internal/arxiv"
	"github.com/Han8931/gorae/internal/config"
	"github.com/Han8931/gorae/internal/meta"
	"github.com/Han8931/gorae/internal/theme"
)

// ':arxiv' carries two meanings and they are told apart by the first argument.
// Routing a valid ID to search (or a query to the metadata path) would break
// whichever form was misread, so this is the seam worth pinning down.
func TestLooksLikeArxivSearch(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"no arguments keeps the selection-based form", nil, false},
		{"modern id", []string{"1706.03762"}, false},
		{"versioned id", []string{"1706.03762v5"}, false},
		{"legacy id", []string{"cs.CL/0701001"}, false},
		{"id with target files", []string{"1706.03762", "paper.pdf"}, false},
		{"option flag", []string{"-v"}, false},
		{"plain words are a query", []string{"attention", "is", "all", "you", "need"}, true},
		{"single word is a query", []string{"transformers"}, true},
		{"hyphenated model name is a query", []string{"gpt-4", "evaluation"}, true},
		{"field-qualified query", []string{"au:Vaswani"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeArxivSearch(tt.args); got != tt.want {
				t.Errorf("looksLikeArxivSearch(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestArxivImportFilename(t *testing.T) {
	tests := []struct {
		name string
		md   arxiv.Metadata
		want string
	}{
		{
			name: "title and id",
			md:   arxiv.Metadata{ID: "1706.03762v5", Title: "Attention Is All You Need"},
			want: "attention-is-all-you-need-1706.03762v5.pdf",
		},
		{
			name: "no title falls back to the id",
			md:   arxiv.Metadata{ID: "1706.03762"},
			want: "1706.03762.pdf",
		},
		{
			// A legacy ID contains a slash, which cannot appear in a filename.
			name: "legacy id slash is replaced",
			md:   arxiv.Metadata{ID: "cs.CL/0701001", Title: "Parsing"},
			want: "parsing-cs.CL-0701001.pdf",
		},
		{
			name: "nothing usable",
			md:   arxiv.Metadata{},
			want: "arxiv-paper.pdf",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := arxivImportFilename(tt.md)
			if got != tt.want {
				t.Errorf("arxivImportFilename() = %q, want %q", got, tt.want)
			}
			if strings.ContainsRune(got, filepath.Separator) {
				t.Errorf("filename %q contains a path separator", got)
			}
		})
	}
}

func TestFormatArxivAuthors(t *testing.T) {
	tests := []struct {
		authors []string
		want    string
	}{
		{nil, ""},
		{[]string{"Ashish Vaswani"}, "Ashish Vaswani"},
		{[]string{"Ashish Vaswani", "Noam Shazeer"}, "Ashish Vaswani, Noam Shazeer"},
		{[]string{"A", "B", "C"}, "A et al."},
	}
	for _, tt := range tests {
		if got := formatArxivAuthors(tt.authors); got != tt.want {
			t.Errorf("formatArxivAuthors(%v) = %q, want %q", tt.authors, got, tt.want)
		}
	}
}

func TestWrapArxivAbstract(t *testing.T) {
	text := "The dominant sequence transduction models are based on complex recurrent or convolutional neural networks."

	lines := wrapArxivAbstract(text, 30, 3)
	if len(lines) == 0 {
		t.Fatal("no lines produced")
	}
	if len(lines) > 3 {
		t.Errorf("got %d lines, want at most 3", len(lines))
	}
	for i, l := range lines {
		if len([]rune(l)) > 30 {
			t.Errorf("line %d is %d runes, over the 30 limit: %q", i, len([]rune(l)), l)
		}
	}

	// Newlines in the source abstract must not survive into the rendered block.
	if got := wrapArxivAbstract("one\ntwo\n\nthree", 40, 2); len(got) != 1 || got[0] != "one two three" {
		t.Errorf("wrapped = %q, want the whitespace collapsed", got)
	}
	// Degenerate sizes produce nothing rather than panicking.
	if got := wrapArxivAbstract(text, 0, 3); got != nil {
		t.Errorf("zero width = %q, want nil", got)
	}
	if got := wrapArxivAbstract(text, 30, 0); got != nil {
		t.Errorf("zero lines = %q, want nil", got)
	}
	if got := wrapArxivAbstract("", 30, 3); got != nil {
		t.Errorf("empty text = %q, want nil", got)
	}
}

func newArxivTestModel(t *testing.T) *Model {
	t.Helper()
	base := t.TempDir()
	store, err := meta.Open(filepath.Join(base, "meta.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	m := &Model{
		cfg:            &config.Config{MetaDir: base},
		meta:           store,
		root:           base,
		cwd:            base,
		width:          100,
		viewportHeight: 24,
	}
	m.applyTheme(theme.Default())
	return m
}

func TestHandleArxivSearchResultOpensBrowser(t *testing.T) {
	m := newArxivTestModel(t)
	m.arxivQuery = "attention"
	m.arxivSearching = true

	m.handleArxivSearchResult(arxivSearchResultMsg{
		query: "attention",
		start: 0,
		res: &arxiv.SearchResult{
			Total: 42,
			Results: []arxiv.Metadata{
				{ID: "1706.03762", Title: "Attention Is All You Need"},
				{ID: "2005.14165", Title: "Language Models are Few-Shot Learners"},
			},
		},
	})

	if m.state != stateArxivResults {
		t.Fatalf("state = %v, want stateArxivResults", m.state)
	}
	if m.arxivSearching {
		t.Error("arxivSearching should be cleared once a page arrives")
	}
	if len(m.arxivResults) != 2 {
		t.Errorf("got %d results, want 2", len(m.arxivResults))
	}
	if m.arxivTotal != 42 {
		t.Errorf("total = %d, want 42", m.arxivTotal)
	}
	if m.arxivCursor != 0 {
		t.Errorf("cursor = %d, want 0", m.arxivCursor)
	}
}

// A page belonging to a query the user has already replaced must not overwrite
// what is on screen — otherwise a slow first request clobbers a fast second.
func TestHandleArxivSearchResultIgnoresStalePage(t *testing.T) {
	m := newArxivTestModel(t)
	m.arxivQuery = "transformers"
	m.arxivResults = []arxiv.Metadata{{ID: "2005.14165", Title: "Current"}}
	m.state = stateArxivResults

	m.handleArxivSearchResult(arxivSearchResultMsg{
		query: "attention",
		res:   &arxiv.SearchResult{Total: 1, Results: []arxiv.Metadata{{ID: "1706.03762", Title: "Stale"}}},
	})

	if len(m.arxivResults) != 1 || m.arxivResults[0].Title != "Current" {
		t.Errorf("results = %+v, want the current query's page untouched", m.arxivResults)
	}
}

func TestHandleArxivSearchResultNoHits(t *testing.T) {
	m := newArxivTestModel(t)
	m.arxivQuery = "zzzznotathing"
	m.arxivSearching = true

	m.handleArxivSearchResult(arxivSearchResultMsg{
		query: "zzzznotathing",
		res:   &arxiv.SearchResult{},
	})

	if m.state == stateArxivResults {
		t.Error("an empty result set must not open the browser")
	}
	if m.arxivSearching {
		t.Error("arxivSearching should be cleared")
	}
}

func TestHandleArxivSearchResultError(t *testing.T) {
	m := newArxivTestModel(t)
	m.arxivQuery = "attention"
	m.arxivSearching = true

	m.handleArxivSearchResult(arxivSearchResultMsg{
		query: "attention",
		err:   context.DeadlineExceeded,
	})

	if m.state == stateArxivResults {
		t.Error("a failed search must not open the browser")
	}
	if m.arxivSearching {
		t.Error("arxivSearching should be cleared after a failure")
	}
}

func openArxivResults(t *testing.T, m *Model, n int) {
	t.Helper()
	results := make([]arxiv.Metadata, n)
	for i := range results {
		results[i] = arxiv.Metadata{ID: "id", Title: "paper", Abstract: "about something"}
	}
	m.arxivQuery = "attention"
	m.handleArxivSearchResult(arxivSearchResultMsg{
		query: "attention",
		res:   &arxiv.SearchResult{Total: n, Results: results},
	})
}

func TestArxivResultsNavigationStaysInBounds(t *testing.T) {
	m := newArxivTestModel(t)
	openArxivResults(t, m, 3)

	// Up at the top is a no-op rather than a negative cursor.
	m.updateArxivResults(tea.KeyMsg{Type: tea.KeyUp})
	if m.arxivCursor != 0 {
		t.Errorf("cursor = %d after up at top, want 0", m.arxivCursor)
	}

	for i := 0; i < 10; i++ {
		m.updateArxivResults(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.arxivCursor != 2 {
		t.Errorf("cursor = %d after paging past the end, want 2", m.arxivCursor)
	}

	m.updateArxivResults(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.arxivCursor != 0 {
		t.Errorf("cursor = %d after g, want 0", m.arxivCursor)
	}
	m.updateArxivResults(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if m.arxivCursor != 2 {
		t.Errorf("cursor = %d after G, want 2", m.arxivCursor)
	}
}

func TestArxivResultsEscCloses(t *testing.T) {
	m := newArxivTestModel(t)
	openArxivResults(t, m, 2)

	m.updateArxivResults(tea.KeyMsg{Type: tea.KeyEsc})

	if m.state != stateNormal {
		t.Errorf("state = %v, want stateNormal", m.state)
	}
	if m.arxivResults != nil {
		t.Error("results should be released when the browser closes")
	}
}

// Paging must not issue requests that cannot return anything: past the end of
// the result set, or before the first page.
func TestArxivResultsPagingBounds(t *testing.T) {
	m := newArxivTestModel(t)
	openArxivResults(t, m, 2) // total 2, so there is no second page

	_, cmd := m.updateArxivResults(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd != nil {
		t.Error("n past the last page should not issue a request")
	}

	_, cmd = m.updateArxivResults(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd != nil {
		t.Error("p on the first page should not issue a request")
	}

	// With more hits than one page holds, n does page forward.
	m.arxivTotal = 100
	_, cmd = m.updateArxivResults(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd == nil {
		t.Error("n with results remaining should issue a request")
	}
	if m.arxivQuery != "attention" {
		t.Errorf("query = %q, want it preserved across paging", m.arxivQuery)
	}
}

// An in-flight query must not be joined by another: two pages racing would
// leave the view showing whichever happened to land last.
func TestArxivResultsPagingIgnoredWhileSearching(t *testing.T) {
	m := newArxivTestModel(t)
	openArxivResults(t, m, 2)
	m.arxivTotal = 100
	m.arxivSearching = true

	if _, cmd := m.updateArxivResults(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}); cmd != nil {
		t.Error("n while a search is in flight should be ignored")
	}
}

func TestStartArxivSearchRejectsEmptyQuery(t *testing.T) {
	m := newArxivTestModel(t)
	if cmd := m.startArxivSearch("   ", 0); cmd != nil {
		t.Error("an empty query should not issue a request")
	}
	if m.arxivSearching {
		t.Error("arxivSearching should stay false")
	}
}

// The import path end to end: the PDF lands in the current directory and its
// metadata is queryable from the store afterwards.
func TestArxivImportCmdSavesPDFAndMetadata(t *testing.T) {
	m := newArxivTestModel(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("%PDF-1.4 attention"))
	}))
	t.Cleanup(srv.Close)

	hit := arxiv.Metadata{
		ID:       "1706.03762v5",
		Title:    "Attention Is All You Need",
		Authors:  []string{"Ashish Vaswani", "Noam Shazeer"},
		Year:     2017,
		DOI:      "10.1000/xyz123",
		Abstract: "The dominant sequence transduction models.",
		PDFURL:   srv.URL,
	}

	msg := arxivImportCmd(hit, m.cwd, m.meta)()
	done, ok := msg.(arxivImportDoneMsg)
	if !ok {
		t.Fatalf("got %T, want arxivImportDoneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("import: %v", done.err)
	}

	want := filepath.Join(m.cwd, "attention-is-all-you-need-1706.03762v5.pdf")
	if done.path != want {
		t.Errorf("path = %q, want %q", done.path, want)
	}
	if data, err := os.ReadFile(want); err != nil || string(data) != "%PDF-1.4 attention" {
		t.Fatalf("saved pdf = %q, err = %v", data, err)
	}

	md, err := m.meta.Get(context.Background(), canonicalPath(want))
	if err != nil {
		t.Fatalf("get metadata: %v", err)
	}
	if md == nil {
		t.Fatal("no metadata recorded for the imported paper")
	}
	if md.Title != hit.Title {
		t.Errorf("title = %q, want %q", md.Title, hit.Title)
	}
	if md.Author != "Ashish Vaswani, Noam Shazeer" {
		t.Errorf("author = %q", md.Author)
	}
	if md.Year != "2017" {
		t.Errorf("year = %q, want 2017", md.Year)
	}
	if md.DOI != hit.DOI {
		t.Errorf("doi = %q, want %q", md.DOI, hit.DOI)
	}
	if md.URL != "https://arxiv.org/abs/1706.03762v5" {
		t.Errorf("url = %q", md.URL)
	}
	if md.Abstract != hit.Abstract {
		t.Errorf("abstract = %q", md.Abstract)
	}
}

// Importing the same paper twice must not overwrite the first copy.
func TestArxivImportCmdDoesNotClobberExistingFile(t *testing.T) {
	m := newArxivTestModel(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("second"))
	}))
	t.Cleanup(srv.Close)

	hit := arxiv.Metadata{ID: "1706.03762", Title: "Attention", PDFURL: srv.URL}
	existing := filepath.Join(m.cwd, arxivImportFilename(hit))
	if err := os.WriteFile(existing, []byte("first"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	msg := arxivImportCmd(hit, m.cwd, m.meta)()
	done := msg.(arxivImportDoneMsg)
	if done.err != nil {
		t.Fatalf("import: %v", done.err)
	}
	if data, _ := os.ReadFile(existing); string(data) != "first" {
		t.Errorf("existing file was overwritten: %q", data)
	}
	if done.path == existing {
		t.Error("the second import should have landed on a different path")
	}
	if _, err := os.Stat(done.path); err != nil {
		t.Errorf("second copy missing: %v", err)
	}
}

func TestArxivImportCmdReportsDownloadFailure(t *testing.T) {
	m := newArxivTestModel(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	hit := arxiv.Metadata{ID: "1706.03762", Title: "Missing", PDFURL: srv.URL}
	done := arxivImportCmd(hit, m.cwd, m.meta)().(arxivImportDoneMsg)
	if done.err == nil {
		t.Fatal("expected an error for a 404 download")
	}
	// Nothing is left in the library and nothing was recorded.
	entries, err := os.ReadDir(m.cwd)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".pdf") {
			t.Errorf("a PDF was left behind after a failed download: %s", e.Name())
		}
	}
}

func TestRenderArxivResults(t *testing.T) {
	m := newArxivTestModel(t)
	m.arxivQuery = "attention"
	m.handleArxivSearchResult(arxivSearchResultMsg{
		query: "attention",
		res: &arxiv.SearchResult{
			Total: 137,
			Results: []arxiv.Metadata{
				{
					ID:       "1706.03762v5",
					Title:    "Attention Is All You Need",
					Authors:  []string{"Ashish Vaswani", "Noam Shazeer", "Niki Parmar"},
					Year:     2017,
					Abstract: "The dominant sequence transduction models are based on complex networks.",
				},
				{ID: "2005.14165", Title: "Language Models are Few-Shot Learners", Year: 2020},
			},
		},
	})

	out := m.renderArxivResults()
	for _, want := range []string{"ARXIV", "attention", "Attention Is All You Need", "et al.", "2017", "137", "import"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered results missing %q:\n%s", want, out)
		}
	}

	// A cramped terminal must still render.
	m.width = 20
	m.viewportHeight = 6
	if got := m.renderArxivResults(); got == "" {
		t.Error("narrow render produced no output")
	}
}
