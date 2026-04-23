package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gorae/internal/config"
)

const (
	defaultOpenAIBase = "https://api.openai.com/v1"
	defaultOllamaBase = "http://localhost:11434/v1"
	defaultModel      = "gpt-4o-mini"
	defaultTopK       = 3
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role      Role   `json:"role"`
	Content   string `json:"content"`
	Thinking  string `json:"-"` // reasoning content from <think>…</think>; not sent to API
	IsSummary bool   `json:"-"` // true for compacted context summary messages
}

type Client struct {
	baseURL    string
	apiKey     string
	model      string
	provider   string
	httpClient *http.Client
}

// NewClient builds a Client from the app config.
func NewClient(cfg *config.AIConfig) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("no AI config found — add an \"ai\" block to config.json")
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		switch provider {
		case "ollama":
			baseURL = defaultOllamaBase
		case "openai", "":
			baseURL = defaultOpenAIBase
		default:
			return nil, fmt.Errorf("provider %q requires base_url to be set", cfg.Provider)
		}
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}
	return &Client{
		baseURL:  baseURL,
		apiKey:   strings.TrimSpace(cfg.APIKey),
		model:    model,
		provider: provider,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

func (c *Client) Model() string { return c.model }

// GetEmbedding returns a vector embedding for text using the given model.
// Supports Ollama (/api/embeddings) and OpenAI-compatible (/v1/embeddings) APIs.
func (c *Client) GetEmbedding(ctx context.Context, embModel, text string) ([]float32, error) {
	if c.provider == "ollama" {
		return c.ollamaEmbedding(ctx, embModel, text)
	}
	return c.openaiEmbedding(ctx, embModel, text)
}

func (c *Client) ollamaEmbedding(ctx context.Context, model, text string) ([]float32, error) {
	// Ollama base is http://localhost:11434/v1 — strip the /v1 for native API
	base := strings.TrimSuffix(c.baseURL, "/v1")
	reqBody, _ := json.Marshal(map[string]string{"model": model, "prompt": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/embeddings", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embeddings: status %d", resp.StatusCode)
	}
	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Embedding, nil
}

func (c *Client) openaiEmbedding(ctx context.Context, model, text string) ([]float32, error) {
	reqBody, _ := json.Marshal(map[string]string{"model": model, "input": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai embeddings: status %d", resp.StatusCode)
	}
	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("openai embeddings: empty response")
	}
	return result.Data[0].Embedding, nil
}

type StreamToken struct {
	Text string
	Err  error
	Done bool
}

// StreamChat sends messages and streams response tokens to the returned channel.
// The caller must drain the channel until Done or Err is received.
func (c *Client) StreamChat(ctx context.Context, messages []Message) <-chan StreamToken {
	ch := make(chan StreamToken, 64)
	go func() {
		defer close(ch)
		if err := c.stream(ctx, messages, ch); err != nil {
			ch <- StreamToken{Err: err}
		}
	}()
	return ch
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type streamDelta struct {
	Content string `json:"content"`
}

type streamChoice struct {
	Delta        streamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type streamChunk struct {
	Choices []streamChoice `json:"choices"`
}

func (c *Client) stream(ctx context.Context, messages []Message, ch chan<- StreamToken) error {
	body, err := json.Marshal(chatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			ch <- StreamToken{Done: true}
			return nil
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			if t := choice.Delta.Content; t != "" {
				ch <- StreamToken{Text: t}
			}
			if choice.FinishReason != nil {
				ch <- StreamToken{Done: true}
				return nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	ch <- StreamToken{Done: true}
	return nil
}
