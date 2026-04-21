package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WebResult is a single result from a web search.
type WebResult struct {
	Title   string
	URL     string
	Snippet string
}

// WebSearcher is the interface for web search providers.
type WebSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]WebResult, error)
}

// NewWebSearcher creates a WebSearcher from the given provider name and API key.
// Supported providers: "brave", "tavily".
func NewWebSearcher(provider, apiKey string) (WebSearcher, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "brave":
		if strings.TrimSpace(apiKey) == "" {
			return nil, fmt.Errorf("brave search requires an api_key")
		}
		return NewBraveSearcher(apiKey), nil
	case "tavily":
		if strings.TrimSpace(apiKey) == "" {
			return nil, fmt.Errorf("tavily search requires an api_key")
		}
		return NewTavilySearcher(apiKey), nil
	default:
		return nil, fmt.Errorf("unknown web search provider %q — supported: brave, tavily", provider)
	}
}

// ── Brave Search ──────────────────────────────────────────────────────────────

// BraveSearcher calls the Brave Search API.
// Free tier: 2 000 requests/month. Docs: https://api.search.brave.com/
type BraveSearcher struct {
	apiKey string
	client *http.Client
}

func NewBraveSearcher(apiKey string) *BraveSearcher {
	return &BraveSearcher{
		apiKey: apiKey,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (b *BraveSearcher) Search(ctx context.Context, query string, limit int) ([]WebResult, error) {
	if limit <= 0 {
		limit = 5
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", fmt.Sprintf("%d", limit))
	params.Set("text_decorations", "false")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.search.brave.com/res/v1/web/search?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.apiKey)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brave search: HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	out := make([]WebResult, 0, len(payload.Web.Results))
	for _, r := range payload.Web.Results {
		out = append(out, WebResult{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return out, nil
}

// ── Tavily Search ─────────────────────────────────────────────────────────────

// TavilySearcher calls the Tavily API, which is purpose-built for AI/RAG use.
// Free tier: 1 000 requests/month. Docs: https://docs.tavily.com/
type TavilySearcher struct {
	apiKey string
	client *http.Client
}

func NewTavilySearcher(apiKey string) *TavilySearcher {
	return &TavilySearcher{
		apiKey: apiKey,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *TavilySearcher) Search(ctx context.Context, query string, limit int) ([]WebResult, error) {
	if limit <= 0 {
		limit = 5
	}
	body, err := json.Marshal(map[string]interface{}{
		"api_key":      t.apiKey,
		"query":        query,
		"max_results":  limit,
		"search_depth": "basic",
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily search: HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	out := make([]WebResult, 0, len(payload.Results))
	for _, r := range payload.Results {
		out = append(out, WebResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return out, nil
}
