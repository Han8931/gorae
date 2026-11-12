package meta_test

import (
	"context"
	"strings"
	"testing"
)

// filler builds body text with no overlap with any query term used here.
func filler(words int) string {
	return strings.TrimSpace(strings.Repeat("lorem ", words))
}

func TestRelevantPassagesReturnsShortDocumentWhole(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const body = "a short indexed document about attention"
	if err := store.IndexContent(ctx, "/tmp/short.pdf", body, 10); err != nil {
		t.Fatalf("index: %v", err)
	}

	got, err := store.RelevantPassages(ctx, "/tmp/short.pdf", "attention", nil, 8000)
	if err != nil {
		t.Fatalf("passages: %v", err)
	}
	if got != body {
		t.Errorf("got %q, want the whole body %q", got, body)
	}
}

// Without embeddings, selection falls back to query-term overlap — so focus
// still narrows to the relevant part of a long paper with vector search off.
func TestRelevantPassagesSelectsByTermOverlap(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	// The needle sits late in the document, well past any prefix cut.
	body := filler(1200) + " the memory hierarchy section discusses SRAM bandwidth in depth " + filler(1200)
	if err := store.IndexContent(ctx, "/tmp/long.pdf", body, 100); err != nil {
		t.Fatalf("index: %v", err)
	}

	got, err := store.RelevantPassages(ctx, "/tmp/long.pdf", "SRAM bandwidth memory hierarchy", nil, 1200)
	if err != nil {
		t.Fatalf("passages: %v", err)
	}
	if len([]rune(got)) > 1200 {
		t.Errorf("got %d chars, want at most 1200", len([]rune(got)))
	}
	if !strings.Contains(got, "SRAM bandwidth") {
		t.Errorf("selection missed the relevant passage; got %q…", truncateForLog(got))
	}
}

// With a query vector and stored embeddings, ranking uses cosine similarity.
func TestRelevantPassagesRanksByEmbedding(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const path = "/tmp/vec.pdf"
	// A body long enough that excerpting is required.
	if err := store.IndexContent(ctx, path, filler(4000), 100); err != nil {
		t.Fatalf("index: %v", err)
	}
	chunks := []string{"chunk zero about scaling", "chunk one about memory", "chunk two about parsing"}
	vecs := [][]float32{{0, 1, 0}, {1, 0, 0}, {0, 0, 1}}
	if err := store.StoreEmbeddings(ctx, path, chunks, vecs, 100, testEmbModel); err != nil {
		t.Fatalf("store embeddings: %v", err)
	}

	// Query vector points at chunk one.
	got, err := store.RelevantPassages(ctx, path, "memory", []float32{1, 0, 0}, 40)
	if err != nil {
		t.Fatalf("passages: %v", err)
	}
	if !strings.Contains(got, "chunk one") {
		t.Errorf("got %q, want the chunk nearest the query vector", got)
	}
}

// Stored vectors from a different embedding_model must not drive selection;
// the term-overlap path should take over instead.
func TestRelevantPassagesIgnoresMismatchedEmbeddings(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const path = "/tmp/mixed.pdf"
	body := filler(1200) + " distinctive marker phrase about quantization " + filler(1200)
	if err := store.IndexContent(ctx, path, body, 100); err != nil {
		t.Fatalf("index: %v", err)
	}
	// Three-wide stored vectors, five-wide query vector.
	if err := store.StoreEmbeddings(ctx, path, []string{"unrelated stored chunk"}, [][]float32{{1, 0, 0}}, 100, testEmbModel); err != nil {
		t.Fatalf("store embeddings: %v", err)
	}

	got, err := store.RelevantPassages(ctx, path, "quantization marker", []float32{1, 0, 0, 0, 0}, 1200)
	if err != nil {
		t.Fatalf("passages: %v", err)
	}
	if strings.Contains(got, "unrelated stored chunk") {
		t.Error("selection used embeddings of the wrong width")
	}
	if !strings.Contains(got, "quantization") {
		t.Errorf("term-overlap fallback missed the marker; got %q…", truncateForLog(got))
	}
}

// Excerpts must read forwards and mark omitted text, so the model can tell it
// is seeing selected passages rather than continuous prose.
func TestRelevantPassagesOrdersAndMarksGaps(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const path = "/tmp/gaps.pdf"
	if err := store.IndexContent(ctx, path, filler(4000), 100); err != nil {
		t.Fatalf("index: %v", err)
	}
	// Chunks 0 and 2 are the relevant ones; chunk 1 sits between them.
	chunks := []string{"alpha relevant text", "beta unrelated text", "gamma relevant text"}
	vecs := [][]float32{{1, 0}, {0, 1}, {1, 0}}
	if err := store.StoreEmbeddings(ctx, path, chunks, vecs, 100, testEmbModel); err != nil {
		t.Fatalf("store embeddings: %v", err)
	}

	got, err := store.RelevantPassages(ctx, path, "relevant", []float32{1, 0}, 45)
	if err != nil {
		t.Fatalf("passages: %v", err)
	}
	alpha, gamma := strings.Index(got, "alpha"), strings.Index(got, "gamma")
	if alpha < 0 || gamma < 0 {
		t.Fatalf("got %q, want both relevant chunks", got)
	}
	if alpha > gamma {
		t.Errorf("got %q, want document order", got)
	}
	if !strings.Contains(got, "[…]") {
		t.Errorf("got %q, want a gap marker between non-adjacent passages", got)
	}
}

// A budget smaller than any single passage must still yield grounding text.
func TestRelevantPassagesTinyBudget(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const path = "/tmp/tiny.pdf"
	if err := store.IndexContent(ctx, path, filler(2000), 100); err != nil {
		t.Fatalf("index: %v", err)
	}

	got, err := store.RelevantPassages(ctx, path, "lorem", nil, 20)
	if err != nil {
		t.Fatalf("passages: %v", err)
	}
	if got == "" {
		t.Fatal("got empty output, want a truncated passage")
	}
	if len([]rune(got)) > 20 {
		t.Errorf("got %d chars, want at most 20", len([]rune(got)))
	}
}

func TestRelevantPassagesEdgeCases(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if err := store.IndexContent(ctx, "/tmp/edge.pdf", "some body", 10); err != nil {
		t.Fatalf("index: %v", err)
	}

	// Unindexed path.
	if got, err := store.RelevantPassages(ctx, "/tmp/missing.pdf", "query", nil, 500); err != nil || got != "" {
		t.Errorf("unindexed: got %q, %v; want empty and no error", got, err)
	}
	// Non-positive budget.
	if got, err := store.RelevantPassages(ctx, "/tmp/edge.pdf", "query", nil, 0); err != nil || got != "" {
		t.Errorf("zero budget: got %q, %v; want empty and no error", got, err)
	}
	// A query with no content words still returns text rather than nothing.
	if got, err := store.RelevantPassages(ctx, "/tmp/edge.pdf", "why?", nil, 500); err != nil || got == "" {
		t.Errorf("contentless query: got %q, %v; want the body", got, err)
	}
}

func truncateForLog(s string) string {
	if r := []rune(s); len(r) > 120 {
		return string(r[:120])
	}
	return s
}
