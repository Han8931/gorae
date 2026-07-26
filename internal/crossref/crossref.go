package crossref

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// httpClient bounds every Crossref request so an unresponsive server can't
// hang the operation indefinitely (http.DefaultClient has no timeout).
var httpClient = &http.Client{Timeout: 15 * time.Second}

// Metadata describes the subset of Crossref fields used by Gorae.
type Metadata struct {
	DOI       string
	Title     string
	Authors   []string
	Published string
	Year      int
	URL       string
	Abstract  string
}

const userAgent = "github.com/Han8931/gorae/0.1 (https://github.com/Han8931/gorae)"

// Fetch retrieves metadata for the provided DOI via the Crossref Works API.
func Fetch(ctx context.Context, doi string) (*Metadata, error) {
	value := strings.TrimSpace(doi)
	if value == "" {
		return nil, fmt.Errorf("doi cannot be empty")
	}
	endpoint := "https://api.crossref.org/works/" + url.PathEscape(value)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("crossref status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Message workMessage `json:"message"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)) // limit to 8MB
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	msg := payload.Message
	meta := &Metadata{
		DOI:       strings.TrimSpace(firstNonEmpty(msg.DOI, value)),
		Title:     strings.TrimSpace(firstFrom(msg.Title)),
		Authors:   parseAuthors(msg.Author),
		Published: strings.TrimSpace(firstFrom(msg.ContainerTitle)),
		Year:      pickYear(msg.PublishedPrint, msg.PublishedOnline, msg.Issued),
		URL:       strings.TrimSpace(msg.URL),
		Abstract:  cleanAbstract(msg.Abstract),
	}
	return meta, nil
}

type workMessage struct {
	Title           []string  `json:"title"`
	Subtitle        []string  `json:"subtitle"`
	Author          []author  `json:"author"`
	ContainerTitle  []string  `json:"container-title"`
	PublishedPrint  dateParts `json:"published-print"`
	PublishedOnline dateParts `json:"published-online"`
	Issued          dateParts `json:"issued"`
	URL             string    `json:"URL"`
	DOI             string    `json:"DOI"`
	Abstract        string    `json:"abstract"`
}

type author struct {
	Given  string `json:"given"`
	Family string `json:"family"`
	Name   string `json:"name"`
}

type dateParts struct {
	DateParts [][]int `json:"date-parts"`
}

func firstFrom(values []string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func parseAuthors(items []author) []string {
	if len(items) == 0 {
		return nil
	}
	names := make([]string, 0, len(items))
	for _, a := range items {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			given := strings.TrimSpace(a.Given)
			family := strings.TrimSpace(a.Family)
			if given != "" && family != "" {
				name = given + " " + family
			} else if family != "" {
				name = family
			} else {
				name = given
			}
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func pickYear(parts ...dateParts) int {
	for _, p := range parts {
		if year := datePartsYear(p); year > 0 {
			return year
		}
	}
	return 0
}

func datePartsYear(parts dateParts) int {
	if len(parts.DateParts) == 0 {
		return 0
	}
	first := parts.DateParts[0]
	if len(first) == 0 {
		return 0
	}
	return first[0]
}

func cleanAbstract(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(raw))
	inTag := false
	for _, r := range raw {
		switch r {
		case '<':
			inTag = true
			continue
		case '>':
			inTag = false
			continue
		}
		if !inTag {
			builder.WriteRune(r)
		}
	}
	clean := html.UnescapeString(builder.String())
	return strings.TrimSpace(clean)
}
