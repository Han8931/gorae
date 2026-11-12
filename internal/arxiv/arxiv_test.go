package arxiv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serveFeed points the package at a test server returning body, and reports
// the query string of the last request so URL construction can be asserted.
func serveFeed(t *testing.T, body string) *url.Values {
	t.Helper()
	var last url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = r.URL.Query()
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	prev := apiBaseURL
	apiBaseURL = srv.URL
	t.Cleanup(func() { apiBaseURL = prev })
	return &last
}

const searchFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opensearch="http://a9.com/-/spec/opensearch/1.1/" xmlns:arxiv="http://arxiv.org/schemas/atom">
  <opensearch:totalResults>137</opensearch:totalResults>
  <opensearch:startIndex>0</opensearch:startIndex>
  <entry>
    <id>http://arxiv.org/abs/1706.03762v5</id>
    <published>2017-06-12T17:57:34Z</published>
    <title>Attention Is All You
  Need</title>
    <summary>  The dominant sequence transduction models are based on
complex recurrent networks.
</summary>
    <author><name>Ashish Vaswani</name></author>
    <author><name>Noam Shazeer</name></author>
    <arxiv:doi>10.1000/xyz123</arxiv:doi>
    <link href="http://arxiv.org/abs/1706.03762v5" rel="alternate" type="text/html"/>
    <link title="pdf" href="http://arxiv.org/pdf/1706.03762v5" rel="related" type="application/pdf"/>
  </entry>
  <entry>
    <id>http://arxiv.org/abs/2005.14165v4</id>
    <published>2020-05-28T15:00:00Z</published>
    <title>Language Models are Few-Shot Learners</title>
    <summary>We train GPT-3.</summary>
    <author><name>Tom B. Brown</name></author>
  </entry>
</feed>`

func TestSearchParsesEntries(t *testing.T) {
	serveFeed(t, searchFeed)

	res, err := Search(context.Background(), "attention", 0, 25)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.Total != 137 {
		t.Errorf("total = %d, want 137", res.Total)
	}
	if len(res.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(res.Results))
	}

	first := res.Results[0]
	if first.ID != "1706.03762v5" {
		t.Errorf("id = %q, want the bare versioned id", first.ID)
	}
	// A wrapped title must collapse to one line to be usable as a list row.
	if first.Title != "Attention Is All You Need" {
		t.Errorf("title = %q, want the unwrapped title", first.Title)
	}
	if len(first.Authors) != 2 || first.Authors[0] != "Ashish Vaswani" {
		t.Errorf("authors = %v", first.Authors)
	}
	if first.Year != 2017 {
		t.Errorf("year = %d, want 2017", first.Year)
	}
	if first.DOI != "10.1000/xyz123" {
		t.Errorf("doi = %q, want 10.1000/xyz123", first.DOI)
	}
	if strings.Contains(first.Abstract, "\n") {
		t.Errorf("abstract still contains newlines: %q", first.Abstract)
	}
	if first.PDFURL != "http://arxiv.org/pdf/1706.03762v5" {
		t.Errorf("pdf url = %q, want the advertised link", first.PDFURL)
	}
}

// An entry without a PDF link still has to be importable, so the URL falls
// back to the documented arxiv.org layout.
func TestSearchDerivesPDFURLWhenAbsent(t *testing.T) {
	serveFeed(t, searchFeed)

	res, err := Search(context.Background(), "gpt", 0, 25)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	second := res.Results[1]
	if second.PDFURL != "https://arxiv.org/pdf/2005.14165v4" {
		t.Errorf("pdf url = %q, want the derived URL", second.PDFURL)
	}
}

// Multi-word queries go out as a quoted phrase first. Sending the words loose
// makes arXiv match them disjunctively, which buries the paper being looked for
// under every paper containing any one of the words.
func TestSearchSendsPhraseFirst(t *testing.T) {
	last := serveFeed(t, searchFeed)

	if _, err := Search(context.Background(), "attention is all you need", 0, 10); err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := last.Get("search_query"); got != `all:"attention is all you need"` {
		t.Errorf("search_query = %q, want a quoted phrase", got)
	}
}

// A query that already names a field is the user speaking arXiv's own syntax;
// wrapping it would change what they asked for.
func TestSearchPassesFieldQueriesThrough(t *testing.T) {
	last := serveFeed(t, searchFeed)

	if _, err := Search(context.Background(), `au:"Vaswani" AND ti:attention`, 0, 10); err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := last.Get("search_query"); got != `au:"Vaswani" AND ti:attention` {
		t.Errorf("search_query = %q, want field-qualified queries passed through", got)
	}
}

func TestSearchSingleWordNeedsNoPhrase(t *testing.T) {
	last := serveFeed(t, searchFeed)

	if _, err := Search(context.Background(), "transformers", 0, 10); err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := last.Get("search_query"); got != "all:transformers" {
		t.Errorf("search_query = %q, want a bare all: term", got)
	}
}

// A description rather than a literal title matches no phrase, so the search
// falls back to a conjunction of the content words instead of giving up.
func TestSearchFallsBackToConjunctionWhenPhraseMatchesNothing(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sq := r.URL.Query().Get("search_query")
		queries = append(queries, sq)
		if strings.HasPrefix(sq, `all:"`) {
			// The phrase matches nothing.
			_, _ = w.Write([]byte(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom" xmlns:opensearch="http://a9.com/-/spec/opensearch/1.1/"><opensearch:totalResults>0</opensearch:totalResults></feed>`))
			return
		}
		_, _ = w.Write([]byte(searchFeed))
	}))
	t.Cleanup(srv.Close)
	prev := apiBaseURL
	apiBaseURL = srv.URL
	t.Cleanup(func() { apiBaseURL = prev })

	res, err := Search(context.Background(), "retrieval augmented generation for legal contracts", 0, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Results) == 0 {
		t.Fatal("fallback produced no results")
	}
	if len(queries) != 2 {
		t.Fatalf("issued %d queries (%q), want a phrase then a fallback", len(queries), queries)
	}
	if !strings.HasPrefix(queries[0], `all:"`) {
		t.Errorf("first query = %q, want the phrase", queries[0])
	}
	if !strings.Contains(queries[1], " AND ") {
		t.Errorf("second query = %q, want a conjunction", queries[1])
	}
	// "for" is not matchable in arXiv's index; leaving it in a conjunction
	// would zero out the whole query.
	if strings.Contains(queries[1], "all:for") {
		t.Errorf("fallback %q still contains a stopword term", queries[1])
	}
}

// A phrase that does match must not trigger a second request.
func TestSearchStopsAtTheFirstMatchingForm(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(searchFeed))
	}))
	t.Cleanup(srv.Close)
	prev := apiBaseURL
	apiBaseURL = srv.URL
	t.Cleanup(func() { apiBaseURL = prev })

	if _, err := Search(context.Background(), "attention is all you need", 0, 10); err != nil {
		t.Fatalf("search: %v", err)
	}
	if calls != 1 {
		t.Errorf("made %d requests, want 1 when the phrase matches", calls)
	}
}

// Paging must not change which query form is in play: the choice is made on
// the total match count, so a page past the end cannot swap the result set.
func TestSearchFormIsStableAcrossPages(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("search_query"))
		// A result set with matches overall but no entries on this page —
		// what the last page of a short result set looks like.
		_, _ = w.Write([]byte(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom" xmlns:opensearch="http://a9.com/-/spec/opensearch/1.1/"><opensearch:totalResults>47</opensearch:totalResults><opensearch:startIndex>50</opensearch:startIndex></feed>`))
	}))
	t.Cleanup(srv.Close)
	prev := apiBaseURL
	apiBaseURL = srv.URL
	t.Cleanup(func() { apiBaseURL = prev })

	res, err := Search(context.Background(), "attention is all you need", 50, 25)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(queries) != 1 {
		t.Errorf("issued %d queries (%q), want no fallback when the set has matches", len(queries), queries)
	}
	if res.Total != 47 {
		t.Errorf("total = %d, want 47 preserved", res.Total)
	}
}

func TestBuildSearchQueries(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"single word", "transformers", []string{"all:transformers"}},
		{
			name:  "multi word tries phrase then conjunction",
			query: "medical image segmentation",
			want:  []string{`all:"medical image segmentation"`, "all:medical AND all:image AND all:segmentation"},
		},
		{
			name:  "stopwords are dropped from the conjunction",
			query: "attention is all you need",
			want:  []string{`all:"attention is all you need"`, "all:attention AND all:need"},
		},
		{
			name:  "field syntax passes through",
			query: `au:"Vaswani" AND ti:attention`,
			want:  []string{`au:"Vaswani" AND ti:attention`},
		},
		{
			// Nothing but stopwords leaves no conjunction to send.
			name:  "only stopwords",
			query: "is the of",
			want:  []string{`all:"is the of"`},
		},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSearchQueries(tt.query)
			if len(got) != len(tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("query %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// A quote inside the query would close the phrase early and corrupt it.
func TestBuildSearchQueriesNeutralizesQuotes(t *testing.T) {
	got := buildSearchQueries(`attention "is all" you need`)
	if len(got) == 0 {
		t.Fatal("no queries built")
	}
	if strings.Count(got[0], `"`) != 2 {
		t.Errorf("phrase %q should carry exactly its own two quotes", got[0])
	}
}

func TestSearchPagingParameters(t *testing.T) {
	last := serveFeed(t, searchFeed)

	if _, err := Search(context.Background(), "attention", 50, 25); err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := last.Get("start"); got != "50" {
		t.Errorf("start = %q, want 50", got)
	}
	if got := last.Get("max_results"); got != "25" {
		t.Errorf("max_results = %q, want 25", got)
	}
	// Relevance, not date: a search is usually a hunt for one known paper,
	// and newest-first buries it under unrelated recent preprints.
	if got := last.Get("sortBy"); got != "relevance" {
		t.Errorf("sortBy = %q, want relevance", got)
	}
	if got := last.Get("sortOrder"); got != "descending" {
		t.Errorf("sortOrder = %q", got)
	}

	// Out-of-range paging arguments are clamped rather than sent as-is.
	if _, err := Search(context.Background(), "attention", -5, 5000); err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := last.Get("start"); got != "0" {
		t.Errorf("negative start = %q, want 0", got)
	}
	if got := last.Get("max_results"); got != "100" {
		t.Errorf("oversized max_results = %q, want the API ceiling 100", got)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	serveFeed(t, searchFeed)
	if _, err := Search(context.Background(), "   ", 0, 25); err == nil {
		t.Error("empty query should be rejected before a request is made")
	}
}

// arXiv answers a no-match query with one contentless entry; it must not be
// reported as a result.
func TestSearchIgnoresEmptyEntries(t *testing.T) {
	serveFeed(t, `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opensearch="http://a9.com/-/spec/opensearch/1.1/">
  <opensearch:totalResults>0</opensearch:totalResults>
  <entry>
    <id></id>
    <title></title>
  </entry>
</feed>`)

	res, err := Search(context.Background(), "zzzznotathing", 0, 25)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Results) != 0 {
		t.Errorf("got %d results, want none", len(res.Results))
	}
	if res.Total != 0 {
		t.Errorf("total = %d, want 0", res.Total)
	}
}

func TestSearchServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down for maintenance"))
	}))
	t.Cleanup(srv.Close)
	prev := apiBaseURL
	apiBaseURL = srv.URL
	t.Cleanup(func() { apiBaseURL = prev })

	if _, err := Search(context.Background(), "attention", 0, 25); err == nil {
		t.Error("a 503 should surface as an error")
	}
}

// Fetch and Search share their parsing, so the by-ID path keeps reporting the
// ID the caller asked for rather than the versioned one from the feed.
func TestFetchUsesRequestedID(t *testing.T) {
	last := serveFeed(t, searchFeed)

	md, err := Fetch(context.Background(), "1706.03762")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if md.ID != "1706.03762" {
		t.Errorf("id = %q, want the requested id", md.ID)
	}
	if md.Title != "Attention Is All You Need" {
		t.Errorf("title = %q", md.Title)
	}
	if got := last.Get("id_list"); got != "1706.03762" {
		t.Errorf("id_list = %q", got)
	}
	// The download still targets the exact version the feed returned.
	if md.PDFURL != "http://arxiv.org/pdf/1706.03762v5" {
		t.Errorf("pdf url = %q", md.PDFURL)
	}
}

func TestFetchNoEntries(t *testing.T) {
	serveFeed(t, `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"></feed>`)
	if _, err := Fetch(context.Background(), "9999.99999"); err == nil {
		t.Error("a feed with no entries should be an error")
	}
}

func TestDownloadPDF(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("%PDF-1.4 fake"))
	}))
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "paper.pdf")
	if err := DownloadPDF(context.Background(), srv.URL, dest); err != nil {
		t.Fatalf("download: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(data) != "%PDF-1.4 fake" {
		t.Errorf("content = %q", data)
	}

	// No temp files are left beside the result.
	entries, err := os.ReadDir(filepath.Dir(dest))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d files, want only the finished PDF", len(entries))
	}
}

// Importing must never silently replace a paper already in the library.
func TestDownloadPDFRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "paper.pdf")
	if err := os.WriteFile(dest, []byte("original"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("replacement"))
	}))
	t.Cleanup(srv.Close)

	if err := DownloadPDF(context.Background(), srv.URL, dest); err == nil {
		t.Fatal("expected an error rather than an overwrite")
	}
	if data, _ := os.ReadFile(dest); string(data) != "original" {
		t.Errorf("existing file was modified: %q", data)
	}
}

// A failed download must not leave a partial file where a valid PDF is
// expected — the next :index would try to parse it.
func TestDownloadPDFLeavesNothingBehindOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	dest := filepath.Join(dir, "paper.pdf")
	if err := DownloadPDF(context.Background(), srv.URL, dest); err == nil {
		t.Fatal("expected an error on 404")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("dest exists after a failed download, stat err = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("directory holds %d leftover files, want none", len(entries))
	}
}

func TestDownloadPDFEmptyURL(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "paper.pdf")
	if err := DownloadPDF(context.Background(), "  ", dest); err == nil {
		t.Error("an empty URL should be rejected")
	}
}
