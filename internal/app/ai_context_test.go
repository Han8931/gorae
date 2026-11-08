package app

import (
	"strings"
	"testing"

	"github.com/Han8931/gorae/internal/ai"
)

func TestNeedsCondensationFollowUps(t *testing.T) {
	followUps := []string{
		"how does it compare to the baseline?",
		"what about the second one?",
		"why?",
		"and the limitations?",
		"explain more",
		"so why does that help?",
		"compare them",
		"what did they use for evaluation?",
		"is the latter faster?",
		"tell me more about this",
	}
	for _, text := range followUps {
		if !needsCondensation(text) {
			t.Errorf("needsCondensation(%q) = false, want true", text)
		}
	}
}

// Self-contained questions must skip the rewrite entirely — it costs an API
// call, so a false positive here shows up as latency on every message.
func TestNeedsCondensationSkipsSelfContained(t *testing.T) {
	standalone := []string{
		"does FlashAttention beat Megatron on long sequences",
		"summarize the Chinchilla scaling laws paper",
		"Vaswani attention transformer architecture",
		"FlashAttention memory usage",
		// Starts with a continuation word, but names its own subject.
		"how does FlashAttention reduce memory usage?",
		"what evaluation benchmarks did Chinchilla report",
		"compare FlashAttention against Megatron throughput",
	}
	for _, text := range standalone {
		if needsCondensation(text) {
			t.Errorf("needsCondensation(%q) = true, want false", text)
		}
	}
}

// Social filler is short enough to look like a follow-up; rewriting it would
// invent a research question the user never asked.
func TestNeedsCondensationSkipsConversational(t *testing.T) {
	for _, text := range []string{"thanks", "Thanks!", "ok", "got it", "makes sense", "cool."} {
		if needsCondensation(text) {
			t.Errorf("needsCondensation(%q) = true, want false", text)
		}
		if !isConversational(text) {
			t.Errorf("isConversational(%q) = false, want true", text)
		}
	}
}

func TestIsConversationalRejectsSubstantive(t *testing.T) {
	for _, text := range []string{
		"thanks, now explain the attention mechanism",
		"ok so how does the decoder work?",
		"hello world dataset paper",
	} {
		if isConversational(text) {
			t.Errorf("isConversational(%q) = true, want false", text)
		}
	}
}

func TestNeedsCondensationEmpty(t *testing.T) {
	for _, text := range []string{"", "   ", "???"} {
		if needsCondensation(text) {
			t.Errorf("needsCondensation(%q) = true, want false", text)
		}
	}
}

func TestCleanCondensed(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{"plain", "FlashAttention memory usage versus baseline", "FlashAttention memory usage versus baseline", true},
		{"wrapped in quotes", "\"FlashAttention memory usage\"", "FlashAttention memory usage", true},
		{"labelled", "Standalone query: FlashAttention memory usage", "FlashAttention memory usage", true},
		{"preamble then query", "Sure! Here's the query:\nFlashAttention memory usage", "FlashAttention memory usage", true},
		{"think block", "<think>the user means the paper</think>\nFlashAttention memory", "FlashAttention memory", true},
		{"empty", "", "", false},
		{"whitespace", "   \n  ", "", false},
		{"no content words", "the and of it", "", false},
		{"runaway explanation", strings.Repeat("because the user is asking about memory ", 20), "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := cleanCondensed(tc.raw)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tc.ok, got)
			}
			if ok && got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// With no model reachable, the query must still gain the standing topic rather
// than going out as a bare pronoun.
func TestLocalCondenseSplicesPriorTopic(t *testing.T) {
	recent := []ai.Message{
		{Role: ai.RoleUser, Content: "What does the FlashAttention paper say about SRAM?"},
		{Role: ai.RoleAssistant, Content: "It keeps the attention computation in on-chip SRAM…"},
	}

	got := localCondense(recent, "how does it compare to the baseline?")
	if !strings.Contains(got, "how does it compare to the baseline?") {
		t.Errorf("got %q, want the original text retained", got)
	}
	for _, term := range []string{"flashattention", "sram"} {
		if !strings.Contains(strings.ToLower(got), term) {
			t.Errorf("got %q, want it to carry the prior topic %q", got, term)
		}
	}
	// Function words from the prior turn add nothing and are dropped.
	if strings.Contains(strings.ToLower(got), "what does the") {
		t.Errorf("got %q, want only content terms spliced from history", got)
	}
}

func TestLocalCondenseWithoutUsableHistory(t *testing.T) {
	const text = "why?"

	if got := localCondense(nil, text); got != text {
		t.Errorf("no history: got %q, want %q", got, text)
	}

	onlyAssistant := []ai.Message{{Role: ai.RoleAssistant, Content: "Here is an answer."}}
	if got := localCondense(onlyAssistant, text); got != text {
		t.Errorf("no user turn: got %q, want %q", got, text)
	}
}

// Only real dialogue helps resolve a referent; summaries and tool plumbing are
// noise, and the transcript must read oldest-first.
func TestRecentDialogueFiltersAndOrders(t *testing.T) {
	prior := []ai.Message{
		{Role: ai.RoleUser, Content: "oldest — should fall outside the window"},
		{Role: ai.RoleSystem, Content: "a system notice"},
		{Role: ai.RoleUser, Content: "first", IsSummary: true},
		{Role: ai.RoleAssistant, Content: "with a tool call", ToolCalls: []ai.ToolCall{{ID: "1"}}},
		{Role: ai.RoleTool, Content: "tool output"},
		{Role: ai.RoleUser, Content: "   "},
		{Role: ai.RoleUser, Content: "A"},
		{Role: ai.RoleAssistant, Content: "B"},
		{Role: ai.RoleUser, Content: "C"},
	}

	got := recentDialogue(prior)
	if len(got) > condenseHistoryTurns {
		t.Fatalf("got %d messages, want at most %d", len(got), condenseHistoryTurns)
	}
	var contents []string
	for _, m := range got {
		contents = append(contents, m.Content)
		if m.Role != ai.RoleUser && m.Role != ai.RoleAssistant {
			t.Errorf("non-dialogue role %q survived filtering", m.Role)
		}
		if m.IsSummary || len(m.ToolCalls) > 0 || strings.TrimSpace(m.Content) == "" {
			t.Errorf("message %+v should have been filtered", m)
		}
	}
	joined := strings.Join(contents, ",")
	if !strings.HasSuffix(joined, "A,B,C") {
		t.Errorf("got %q, want the tail to read oldest-first as A,B,C", joined)
	}
}

func TestRecentDialogueEmpty(t *testing.T) {
	if got := recentDialogue(nil); len(got) != 0 {
		t.Errorf("got %+v, want empty", got)
	}
	onlyNoise := []ai.Message{
		{Role: ai.RoleSystem, Content: "notice"},
		{Role: ai.RoleTool, Content: "output"},
	}
	if got := recentDialogue(onlyNoise); len(got) != 0 {
		t.Errorf("got %+v, want empty", got)
	}
}

// condenseQuery must never return empty, whatever goes wrong: a degraded
// retrieval query is recoverable, a blank one silently drops all grounding.
func TestCondenseQueryFallsBackWithoutClient(t *testing.T) {
	const text = "how does it compare?"

	if got := condenseQuery(nil, nil, text, nil); got != text { //nolint:staticcheck // nil ctx unused on this path
		t.Errorf("no history, no client: got %q, want %q", got, text)
	}

	recent := []ai.Message{{Role: ai.RoleUser, Content: "the Chinchilla scaling paper"}}
	got := condenseQuery(nil, recent, text, nil) //nolint:staticcheck // nil ctx unused on this path
	if got == "" {
		t.Fatal("got empty query, want a fallback")
	}
	if !strings.Contains(strings.ToLower(got), "chinchilla") {
		t.Errorf("got %q, want the prior topic spliced in", got)
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("short", 100); got != "short" {
		t.Errorf("got %q, want %q", got, "short")
	}
	// Whitespace is collapsed so a quoted message stays one transcript line.
	if got := truncateRunes("a\n\nb   c", 100); got != "a b c" {
		t.Errorf("got %q, want %q", got, "a b c")
	}
	got := truncateRunes(strings.Repeat("x", 50), 10)
	if r := []rune(got); len(r) != 11 || string(r[10]) != "…" {
		t.Errorf("got %q, want 10 runes plus an ellipsis", got)
	}
	// Multi-byte text must be cut on runes, not bytes.
	if got := truncateRunes(strings.Repeat("한", 50), 5); len([]rune(got)) != 6 {
		t.Errorf("got %q (%d runes), want 5 plus an ellipsis", got, len([]rune(got)))
	}
}
