package arxiv

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
}

const userAgent = "gorae/0.1 (https://github.com/han/go-pdf)"

type feed struct {
	Entries []entry `xml:"entry"`
}

type entry struct {
	ID        string        `xml:"id"`
	Title     string        `xml:"title"`
	Published string        `xml:"published"`
	Authors   []entryAuthor `xml:"author"`
	Summary   string        `xml:"summary"`
	DOI       string        `xml:"{http://arxiv.org/schemas/atom}doi"`
}

type entryAuthor struct {
	Name string `xml:"name"`
}

func cleanAbstract(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// Fetch retrieves metadata for a given arXiv ID using the official Atom API.
func Fetch(ctx context.Context, id string) (*Metadata, error) {
	queryURL := "https://export.arxiv.org/api/query?id_list=" + url.QueryEscape(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
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
	if len(f.Entries) == 0 {
		return nil, fmt.Errorf("no entries returned for id %q", id)
	}
	e := f.Entries[0]

	var year int
	if t, err := time.Parse(time.RFC3339, e.Published); err == nil {
		year = t.Year()
	}

	authors := make([]string, len(e.Authors))
	for i, a := range e.Authors {
		authors[i] = strings.TrimSpace(a.Name)
	}

	return &Metadata{
		ID:       id,
		Title:    strings.TrimSpace(e.Title),
		Authors:  authors,
		Year:     year,
		DOI:      strings.TrimSpace(e.DOI),
		Abstract: cleanAbstract(e.Summary),
	}, nil
}
