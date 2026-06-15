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
	RoleTool      Role = "tool"
)

type Message struct {
	Role      Role   `json:"role"`
	Content   string `json:"content"`
	Thinking  string `json:"-"` // reasoning content from <think>…</think>; not sent to API
	IsSummary bool   `json:"-"` // true for compacted context summary messages

	// Tool-calling round-trip fields (OpenAI-compatible shape).
	// On role=assistant: ToolCalls is the list of calls the model asked us to run.
	// On role=tool: ToolCallID + Name + Content carry the tool's result back.
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// Tool describes a function the model is allowed to call. Schema is a raw
// JSON Schema document so callers don't have to depend on a particular
// reflection helper.
type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// ToolCall is one function invocation the model has requested.
// Arguments is the raw JSON string the model produced (OpenAI's wire format);
// callers should json.Unmarshal it into their own param struct.
type ToolCall struct {
	ID   string       `json:"id"`
	Type string       `json:"type"` // always "function" today
	Func ToolCallFunc `json:"function"`
}

type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
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
	// ToolCalls is delivered exactly once, on the final chunk before Done,
	// when the model finished with finish_reason=tool_calls.
	ToolCalls []ToolCall
	Err       error
	Done      bool
}

// StreamChat sends messages and streams response tokens. When tools is non-nil
// the model is told it may invoke them; the caller is responsible for running
// any returned ToolCalls and dispatching a follow-up StreamChat with the
// tool-result messages appended to history.
func (c *Client) StreamChat(ctx context.Context, messages []Message, tools []Tool) <-chan StreamToken {
	ch := make(chan StreamToken, 64)
	go func() {
		defer close(ch)
		if err := c.stream(ctx, messages, tools, ch); err != nil {
			ch <- StreamToken{Err: err}
		}
	}()
	return ch
}

type toolSpecFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type toolSpec struct {
	Type     string       `json:"type"` // "function"
	Function toolSpecFunc `json:"function"`
}

type chatRequest struct {
	Model    string     `json:"model"`
	Messages []Message  `json:"messages"`
	Stream   bool       `json:"stream"`
	Tools    []toolSpec `json:"tools,omitempty"`
}

// toolCallDelta mirrors the streaming fragment shape. Any of id/name/arguments
// may be empty in a given chunk; we accumulate them keyed by index.
type toolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

type streamDelta struct {
	Content   string          `json:"content"`
	ToolCalls []toolCallDelta `json:"tool_calls,omitempty"`
}

type streamChoice struct {
	Delta        streamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type streamChunk struct {
	Choices []streamChoice `json:"choices"`
}

func (c *Client) stream(ctx context.Context, messages []Message, tools []Tool, ch chan<- StreamToken) error {
	payload := chatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
	}
	if len(tools) > 0 {
		payload.Tools = make([]toolSpec, 0, len(tools))
		for _, t := range tools {
			payload.Tools = append(payload.Tools, toolSpec{
				Type: "function",
				Function: toolSpecFunc{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.Schema,
				},
			})
		}
	}
	body, err := json.Marshal(payload)
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

	// Accumulator for streamed tool calls. OpenAI fragments them across chunks
	// keyed by index; we stitch them back together before emitting.
	type pendingCall struct {
		id   string
		name string
		args strings.Builder
	}
	pending := map[int]*pendingCall{}
	var order []int // preserve first-seen order

	flushTools := func() []ToolCall {
		if len(pending) == 0 {
			return nil
		}
		out := make([]ToolCall, 0, len(pending))
		for _, idx := range order {
			p := pending[idx]
			args := p.args.String()
			if args == "" {
				args = "{}"
			}
			out = append(out, ToolCall{
				ID:   p.id,
				Type: "function",
				Func: ToolCallFunc{Name: p.name, Arguments: args},
			})
		}
		return out
	}

	// send delivers a token but bails out if the caller's context is cancelled,
	// so this producer goroutine can't block forever should the consumer stop
	// draining the channel mid-stream. Returns false once the stream is aborted.
	send := func(tok StreamToken) bool {
		select {
		case ch <- tok:
			return true
		case <-ctx.Done():
			return false
		}
	}

	// Buffer for SSE payloads larger than bufio.Scanner's default 64KB line cap.
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1<<16), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			send(StreamToken{ToolCalls: flushTools(), Done: true})
			return nil
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			if t := choice.Delta.Content; t != "" {
				if !send(StreamToken{Text: t}) {
					return ctx.Err()
				}
			}
			for _, tc := range choice.Delta.ToolCalls {
				p, ok := pending[tc.Index]
				if !ok {
					p = &pendingCall{}
					pending[tc.Index] = p
					order = append(order, tc.Index)
				}
				if tc.ID != "" {
					p.id = tc.ID
				}
				if tc.Function.Name != "" {
					p.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					p.args.WriteString(tc.Function.Arguments)
				}
			}
			if choice.FinishReason != nil {
				send(StreamToken{ToolCalls: flushTools(), Done: true})
				return nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	send(StreamToken{ToolCalls: flushTools(), Done: true})
	return nil
}
