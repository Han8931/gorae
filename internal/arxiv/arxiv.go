package arxiv

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Metadata represents the subset of arXiv fields we care about.
type Metadata struct {
	ID       string
	Title    string
	Authors  []string
	Year     int
	DOI      string
	Abstract string
	// PDFURL is where the paper itself can be downloaded. The feed advertises
	// it per entry; when it is absent we derive it from the ID, which is the
	// documented layout of arxiv.org.
	PDFURL string
}

const userAgent = "github.com/Han8931/gorae/0.1 (https://github.com/Han8931/gorae)"

// apiBaseURL is a variable so tests can point the client at a local server
// instead of reaching out to arXiv.
var apiBaseURL = "https://export.arxiv.org/api/query"

// httpClient bounds every arXiv request so an unresponsive server can't hang
// the operation indefinitely (http.DefaultClient has no timeout).
var httpClient = &http.Client{Timeout: 15 * time.Second}

// downloadClient is separate from httpClient because a PDF is orders of
// magnitude larger than a metadata response: a timeout tight enough to keep
// the UI responsive on a query would abort legitimate downloads.
var downloadClient = &http.Client{Timeout: 5 * time.Minute}

type feed struct {
	TotalResults int     `xml:"http://a9.com/-/spec/opensearch/1.1/ totalResults"`
	StartIndex   int     `xml:"http://a9.com/-/spec/opensearch/1.1/ startIndex"`
	Entries      []entry `xml:"entry"`
}

type entry struct {
	ID        string        `xml:"id"`
	Title     string        `xml:"title"`
	Published string        `xml:"published"`
	Authors   []entryAuthor `xml:"author"`
	Summary   string        `xml:"summary"`
	// encoding/xml matches a namespaced element as "<namespace> <local>" —
	// the "{ns}local" spelling this used before is not a form it recognises,
	// so arXiv DOIs silently parsed as empty.
	DOI   string      `xml:"http://arxiv.org/schemas/atom doi"`
	Links []entryLink `xml:"link"`
}

type entryAuthor struct {
	Name string `xml:"name"`
}

type entryLink struct {
	Href  string `xml:"href,attr"`
	Rel   string `xml:"rel,attr"`
	Type  string `xml:"type,attr"`
	Title string `xml:"title,attr"`
}

func cleanAbstract(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// cleanTitle collapses the newlines and runs of spaces arXiv inserts when
// wrapping long titles, so a title is usable as one display line.
func cleanTitle(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// idFromEntryURL reduces the absolute entry URL the feed reports
// ("http://arxiv.org/abs/2301.00001v2") to the bare ID ("2301.00001v2"), which
// is what callers pass back to Fetch and what the PDF URL is built from.
func idFromEntryURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, "/abs/"); idx >= 0 {
		return strings.TrimSpace(raw[idx+len("/abs/"):])
	}
	return raw
}

// pdfURL picks the entry's advertised PDF link, falling back to the canonical
// arxiv.org layout when the feed omits it.
func pdfURL(e entry, id string) string {
	for _, l := range e.Links {
		if strings.EqualFold(l.Title, "pdf") || strings.EqualFold(l.Type, "application/pdf") {
			if h := strings.TrimSpace(l.Href); h != "" {
				return h
			}
		}
	}
	if id == "" {
		return ""
	}
	return "https://arxiv.org/pdf/" + id
}

// toMetadata converts one feed entry. id overrides the ID parsed from the
// entry when the caller already knows which ID it asked for, so a query for
// "2301.00001" reports that rather than the versioned "2301.00001v2".
func toMetadata(e entry, id string) Metadata {
	entryID := idFromEntryURL(e.ID)
	if id == "" {
		id = entryID
	}

	var year int
	if t, err := time.Parse(time.RFC3339, e.Published); err == nil {
		year = t.Year()
	}

	authors := make([]string, len(e.Authors))
	for i, a := range e.Authors {
		authors[i] = strings.TrimSpace(a.Name)
	}

	return Metadata{
		ID:       id,
		Title:    cleanTitle(e.Title),
		Authors:  authors,
		Year:     year,
		DOI:      strings.TrimSpace(e.DOI),
		Abstract: cleanAbstract(e.Summary),
		// The PDF link is built from the entry's own (versioned) ID so the
		// download matches the entry that was listed.
		PDFURL: pdfURL(e, entryID),
	}
}

// get performs a GET against the arXiv API and returns the decoded feed.
func get(ctx context.Context, queryURL string) (*feed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("arxiv status %s: %s", resp.Status, string(b))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var f feed
	if err := xml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("decode xml: %w", err)
	}
	return &f, nil
}

// Fetch retrieves metadata for a given arXiv ID using the official Atom API.
func Fetch(ctx context.Context, id string) (*Metadata, error) {
	f, err := get(ctx, apiBaseURL+"?id_list="+url.QueryEscape(id))
	if err != nil {
		return nil, err
	}
	if len(f.Entries) == 0 {
		return nil, fmt.Errorf("no entries returned for id %q", id)
	}
	md := toMetadata(f.Entries[0], id)
	return &md, nil
}

// SearchResult is one page of search hits plus enough context to page through
// the rest.
type SearchResult struct {
	Results []Metadata
	// Total is how many papers match the query overall, not how many are in
	// Results. It is what makes paging possible: without it a caller cannot
	// tell a last page from a full one.
	Total int
	Start int
}

// MaxResultsPerPage is the ceiling arXiv applies to a single query.
const MaxResultsPerPage = 100

// Search runs a free-text query against the arXiv API and returns one page of
// results, most relevant first.
//
// The query goes to the "all:" field, which covers title, abstract, authors,
// and comments — the behaviour someone typing a few words expects, rather than
// arXiv's default of matching only exact field-qualified terms. Queries are
// sent verbatim otherwise, so arXiv's own syntax (au:, ti:, AND/OR) still
// works for anyone who knows it.
//
// Results are ordered by relevance rather than submission date. Sorting by
// date answers "what is new in this field", but someone searching for a paper
// to import is almost always after a specific one, and newest-first buries it:
// a query for "attention is all you need" returns yesterday's preprints and
// not the paper. A caller wanting the date ordering will need a sort parameter.
func Search(ctx context.Context, query string, start, maxResults int) (*SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty search query")
	}
	if start < 0 {
		start = 0
	}
	if maxResults <= 0 {
		maxResults = 25
	}
	if maxResults > MaxResultsPerPage {
		maxResults = MaxResultsPerPage
	}

	var f *feed
	for _, searchQuery := range buildSearchQueries(query) {
		params := url.Values{}
		params.Set("search_query", searchQuery)
		params.Set("start", strconv.Itoa(start))
		params.Set("max_results", strconv.Itoa(maxResults))
		params.Set("sortBy", "relevance")
		params.Set("sortOrder", "descending")

		page, err := get(ctx, apiBaseURL+"?"+params.Encode())
		if err != nil {
			return nil, err
		}
		f = page
		// The decision to fall back is made on the total match count, never on
		// how many entries this page happens to hold: a page past the end of a
		// result set returns nothing, and falling back there would swap the
		// result set out midway through paging.
		if page.TotalResults > 0 {
			break
		}
	}

	results := make([]Metadata, 0, len(f.Entries))
	for _, e := range f.Entries {
		md := toMetadata(e, "")
		// A query with no matches can come back as a single empty entry rather
		// than an empty feed; a hit without an ID is not usable either way.
		if strings.TrimSpace(md.ID) == "" {
			continue
		}
		results = append(results, md)
	}

	return &SearchResult{Results: results, Total: f.TotalResults, Start: f.StartIndex}, nil
}

// searchStopwords are words arXiv's index does not match on. They have to be
// dropped from the AND fallback below, where a single unmatchable term reduces
// the whole conjunction to zero hits.
var searchStopwords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "of": true, "for": true, "and": true, "or": true,
	"to": true, "in": true, "on": true, "with": true, "at": true, "by": true,
	"from": true, "as": true, "all": true, "you": true, "your": true,
	"it": true, "its": true, "we": true, "this": true, "that": true,
}

// buildSearchQueries turns a user's words into the arXiv query forms to try, in
// order, stopping at the first that matches anything.
//
// Neither single form works alone, which is why there are two:
//
//   - Sending the words loose makes arXiv match them disjunctively. "attention
//     is all you need" then returns hundreds of thousands of papers containing
//     any of those words and not the paper itself, which is useless.
//   - A quoted phrase finds that paper immediately, but returns nothing at all
//     when the user's wording is a description rather than a literal title.
//   - A conjunction of the terms handles the descriptive case, but collapses to
//     zero as soon as one word is absent from arXiv's index — which is exactly
//     what happens with the function words in a real title.
//
// So: try the phrase, then the conjunction of content words. A query that
// already uses arXiv's field syntax is passed through untouched, since the
// user has said precisely what they want.
func buildSearchQueries(query string) []string {
	if strings.Contains(query, ":") {
		return []string{query}
	}
	words := strings.Fields(query)
	if len(words) == 0 {
		return nil
	}
	if len(words) == 1 {
		return []string{"all:" + words[0]}
	}

	// A quote inside the query would terminate the phrase early.
	phrase := `all:"` + strings.ReplaceAll(query, `"`, " ") + `"`

	terms := make([]string, 0, len(words))
	for _, w := range words {
		if searchStopwords[strings.ToLower(strings.Trim(w, `"`))] {
			continue
		}
		terms = append(terms, "all:"+w)
	}
	// Nothing but stopwords leaves no conjunction worth sending.
	if len(terms) == 0 {
		return []string{phrase}
	}
	return []string{phrase, strings.Join(terms, " AND ")}
}

// DownloadPDF saves the PDF at rawURL to dest, writing through a temporary
// file in the same directory so an interrupted download cannot leave a
// truncated PDF where a valid one is expected. It refuses to overwrite an
// existing dest.
func DownloadPDF(ctx context.Context, rawURL, dest string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("no PDF URL")
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists", filepath.Base(dest))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := downloadClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download status %s", resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".gorae-arxiv-*.pdf")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck // no-op once renamed into place

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("write pdf: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close pdf: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("finalize pdf: %w", err)
	}
	return nil
}
