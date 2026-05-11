package ai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeSSEServer returns an httptest.Server that replies to POST /chat/completions
// with the provided SSE event lines (each will be wrapped with "data: ").
func fakeSSEServer(t *testing.T, events []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatalf("response writer does not support flushing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, ev := range events {
			fmt.Fprintf(w, "data: %s\n\n", ev)
			flusher.Flush()
		}
	}))
}

// TestStreamToolCallReassembly verifies that tool-call fragments arriving across
// multiple SSE chunks (id in one chunk, name in another, arguments in pieces)
// are stitched back into one ToolCall delivered on Done.
func TestStreamToolCallReassembly(t *testing.T) {
	events := []string{
		// First fragment: id + name, no args yet.
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"save_markdown","arguments":""}}]}}]}`,
		// Arguments arrive in three pieces.
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"file"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"name\":\"oct"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"-meeting\"}"}}]}}]}`,
		// Finish.
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	}
	srv := fakeSSEServer(t, events)
	defer srv.Close()

	c := &Client{
		baseURL:    srv.URL,
		model:      "test",
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := c.StreamChat(ctx, []Message{{Role: RoleUser, Content: "save it"}}, []Tool{{Name: "save_markdown"}})

	var (
		text     strings.Builder
		toolCall *ToolCall
	)
	for tok := range ch {
		if tok.Err != nil {
			t.Fatalf("stream error: %v", tok.Err)
		}
		if tok.Text != "" {
			text.WriteString(tok.Text)
		}
		if tok.Done {
			if len(tok.ToolCalls) == 1 {
				tc := tok.ToolCalls[0]
				toolCall = &tc
			} else if len(tok.ToolCalls) > 1 {
				t.Fatalf("expected 1 tool call, got %d", len(tok.ToolCalls))
			}
			break
		}
	}

	if toolCall == nil {
		t.Fatalf("expected one tool call, got none")
	}
	if toolCall.ID != "call_1" {
		t.Errorf("ID = %q, want %q", toolCall.ID, "call_1")
	}
	if toolCall.Func.Name != "save_markdown" {
		t.Errorf("name = %q, want %q", toolCall.Func.Name, "save_markdown")
	}
	if toolCall.Func.Arguments != `{"filename":"oct-meeting"}` {
		t.Errorf("arguments = %q, want %q", toolCall.Func.Arguments, `{"filename":"oct-meeting"}`)
	}
	if text.Len() != 0 {
		t.Errorf("expected no text content, got %q", text.String())
	}
}

// TestStreamPlainTextStillWorks confirms the existing happy path (content only,
// no tool calls) is unaffected by the tool plumbing.
func TestStreamPlainTextStillWorks(t *testing.T) {
	events := []string{
		`{"choices":[{"delta":{"content":"Hel"}}]}`,
		`{"choices":[{"delta":{"content":"lo"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}
	srv := fakeSSEServer(t, events)
	defer srv.Close()

	c := &Client{
		baseURL:    srv.URL,
		model:      "test",
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := c.StreamChat(ctx, []Message{{Role: RoleUser, Content: "hi"}}, nil)

	var text strings.Builder
	var calls []ToolCall
	for tok := range ch {
		if tok.Err != nil {
			t.Fatalf("stream error: %v", tok.Err)
		}
		text.WriteString(tok.Text)
		if tok.Done {
			calls = tok.ToolCalls
			break
		}
	}
	if got := text.String(); got != "Hello" {
		t.Errorf("text = %q, want %q", got, "Hello")
	}
	if len(calls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(calls))
	}
}

// guard against unused-import nag if the file is trimmed later.
var _ = io.Discard
var _ = bytes.NewReader
