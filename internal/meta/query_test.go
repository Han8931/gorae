package meta_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Han8931/gorae/internal/meta"
)

// Punctuation is FTS5 syntax, not text to match. A trailing "?" or an
// apostrophe used to reach MATCH verbatim and raise a syntax error, which the
// caller discarded — so every question the user asked retrieved nothing.
func TestProseQueryStripsFTSSyntax(t *testing.T) {
	for _, q := range []string{
		"how does FlashAttention reduce memory usage?",
		"what's the paper's approach to attention?",
		"attention -- in detail (really)",
		"compare \"FlashAttention\" AND the baseline",
		"NEAR memory^2 OR attention*",
		"tokens: 50% faster; why?",
	} {
		got := meta.ProseQuery(q)
		if got == "" {
			t.Errorf("ProseQuery(%q) = empty, want terms", q)
			continue
		}
		// Only quoted terms joined by OR may survive.
		for _, part := range strings.Split(got, " OR ") {
			if !strings.HasPrefix(part, `"`) || !strings.HasSuffix(part, `"`) {
				t.Errorf("ProseQuery(%q) produced unquoted term %q", q, part)
			}
			inner := strings.Trim(part, `"`)
			if strings.ContainsAny(inner, `"'?*^%():;-`) {
				t.Errorf("ProseQuery(%q) leaked punctuation in %q", q, part)
			}
		}
	}
}

func TestProseQueryDropsStopwordsAndJoinsWithOR(t *testing.T) {
	got := meta.ProseQuery("How does the model reduce memory?")
	want := `"model" OR "reduce" OR "memory"`
	if got != want {
		t.Errorf("ProseQuery = %q, want %q", got, want)
	}
}

func TestProseTermsDeduplicatesAndLowercases(t *testing.T) {
	got := meta.ProseTerms("Attention attention ATTENTION memory")
	if len(got) != 2 || got[0] != "attention" || got[1] != "memory" {
		t.Errorf("ProseTerms = %q, want [attention memory]", got)
	}
}

func TestProseTermsDropsSingleCharacters(t *testing.T) {
	got := meta.ProseTerms("a b transformer x")
	if len(got) != 1 || got[0] != "transformer" {
		t.Errorf("ProseTerms = %q, want [transformer]", got)
	}
}

func TestProseQueryEmptyWhenNothingSearchable(t *testing.T) {
	for _, q := range []string{"", "   ", "why?", "how is it?", "?!...", "the and of"} {
		if got := meta.ProseQuery(q); got != "" {
			t.Errorf("ProseQuery(%q) = %q, want empty", q, got)
		}
	}
}

func TestProseTermsCaps(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("term")
		sb.WriteString(string(rune('a' + i%26)))
		sb.WriteString(string(rune('a' + i/26)))
		sb.WriteString(" ")
	}
	if got := meta.ProseTerms(sb.String()); len(got) > 24 {
		t.Errorf("ProseTerms returned %d terms, want at most 24", len(got))
	}
}

// End-to-end regression: a natural-language question must retrieve from the
// index. Before ProseQuery, anything ending in "?" returned zero hits.
func TestSearchHybridRetrievesFromNaturalLanguageQuestion(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const path = "/tmp/flash.pdf"
	body := "FlashAttention is an IO-aware exact attention algorithm that reduces " +
		"memory reads and writes between GPU high bandwidth memory and on-chip SRAM, " +
		"achieving a speedup over the baseline transformer implementation."
	if err := store.IndexContent(ctx, path, body, 100); err != nil {
		t.Fatalf("index content: %v", err)
	}

	for _, q := range []string{
		"how does FlashAttention reduce memory usage?",
		"what's the paper's approach to memory?",
		"FlashAttention memory",
		"Tell me about SRAM and HBM -- how do they differ?",
	} {
		res, err := store.SearchHybrid(ctx, q, nil, 3)
		got := res.Matches
		if err != nil {
			t.Errorf("SearchHybrid(%q): unexpected error %v", q, err)
			continue
		}
		if len(got) == 0 {
			t.Errorf("SearchHybrid(%q) = no hits, want the indexed document", q)
			continue
		}
		if got[0].Path != path {
			t.Errorf("SearchHybrid(%q) top hit = %s, want %s", q, got[0].Path, path)
		}
	}
}

// OR-joining means a question whose terms only partly appear still matches,
// where the previous implicit AND required every word to be present.
func TestSearchHybridMatchesPartialTermOverlap(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if err := store.IndexContent(ctx, "/tmp/partial.pdf", "retrieval augmented generation for question answering", 100); err != nil {
		t.Fatalf("index content: %v", err)
	}

	// "sparse" and "benchmarks" appear nowhere in the document.
	res, err := store.SearchHybrid(ctx, "how does sparse retrieval do on benchmarks?", nil, 3)
	got := res.Matches
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	if len(got) == 0 {
		t.Error("got no hits, want a match on the overlapping term \"retrieval\"")
	}
}

// A question with no content words must not error or match everything.
func TestSearchHybridSkipsFTSWhenNoSearchableTerms(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if err := store.IndexContent(ctx, "/tmp/doc.pdf", "some indexed body text", 100); err != nil {
		t.Fatalf("index content: %v", err)
	}

	res, err := store.SearchHybrid(ctx, "why?", nil, 3)
	got := res.Matches
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d hits for a contentless query, want 0", len(got))
	}
}
