package meta_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Han8931/gorae/internal/meta"
)

func TestChunkTextOverlaps(t *testing.T) {
	text := "a b c d e f g h i j"

	chunks := meta.ChunkText(text, 4, 1)
	want := []string{"a b c d", "d e f g", "g h i j"}
	if len(chunks) != len(want) {
		t.Fatalf("got %d chunks %q, want %d", len(chunks), chunks, len(want))
	}
	for i := range want {
		if chunks[i] != want[i] {
			t.Errorf("chunk %d = %q, want %q", i, chunks[i], want[i])
		}
	}
}

func TestChunkTextEdgeCases(t *testing.T) {
	if got := meta.ChunkText("", 10, 2); got != nil {
		t.Errorf("empty text: got %q, want nil", got)
	}
	if got := meta.ChunkText("   \n\t ", 10, 2); got != nil {
		t.Errorf("whitespace-only text: got %q, want nil", got)
	}
	// A chunk wider than the input yields exactly one chunk.
	if got := meta.ChunkText("a b c", 100, 10); len(got) != 1 || got[0] != "a b c" {
		t.Errorf("short text: got %q, want [\"a b c\"]", got)
	}
	// overlap >= chunk size must still make forward progress rather than loop.
	if got := meta.ChunkText("a b c d", 2, 5); len(got) == 0 {
		t.Error("overlap exceeding chunk size produced no chunks")
	}
}

// Switching embedding_model leaves vectors of the old width in the table.
// Those rows must be ignored so the caller can fall back to keyword search,
// rather than coming back as top hits that merely scored zero.
func TestSearchSemanticIgnoresMismatchedDimensions(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	threeDim := "/tmp/three.pdf"
	chunks := []string{"body of the three-dimensional document"}
	vecs := [][]float32{{1, 0, 0}}
	if err := store.StoreEmbeddings(ctx, threeDim, chunks, vecs, 100); err != nil {
		t.Fatalf("store embeddings: %v", err)
	}

	// Same width — found.
	got, err := store.SearchSemantic(ctx, []float32{1, 0, 0}, 5)
	if err != nil {
		t.Fatalf("search (matching dims): %v", err)
	}
	if len(got) != 1 || got[0].Path != threeDim {
		t.Fatalf("matching dims: got %+v, want one hit for %s", got, threeDim)
	}

	// Different width — skipped entirely, not returned with score 0.
	got, err = store.SearchSemantic(ctx, []float32{1, 0, 0, 0, 0}, 5)
	if err != nil {
		t.Fatalf("search (mismatched dims): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("mismatched dims: got %+v, want no hits", got)
	}

	// An empty query vector is not a match-everything query.
	got, err = store.SearchSemantic(ctx, nil, 5)
	if err != nil {
		t.Fatalf("search (nil vector): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("nil vector: got %+v, want no hits", got)
	}
}

func TestSearchSemanticRanksByCosineSimilarity(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	// near points almost along the query axis; far points away from it.
	if err := store.StoreEmbeddings(ctx, "/tmp/near.pdf", []string{"near"}, [][]float32{{0.9, 0.1, 0}}, 10); err != nil {
		t.Fatalf("store near: %v", err)
	}
	if err := store.StoreEmbeddings(ctx, "/tmp/far.pdf", []string{"far"}, [][]float32{{0, 0.2, 0.9}}, 10); err != nil {
		t.Fatalf("store far: %v", err)
	}

	got, err := store.SearchSemantic(ctx, []float32{1, 0, 0}, 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d hits, want 2", len(got))
	}
	if got[0].Path != "/tmp/near.pdf" {
		t.Errorf("top hit = %s, want /tmp/near.pdf", got[0].Path)
	}
}

// Magnitude must not affect ranking — cosine similarity is scale-invariant, so
// a long vector pointing away loses to a short one pointing at the query.
func TestSearchSemanticIsScaleInvariant(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if err := store.StoreEmbeddings(ctx, "/tmp/aligned.pdf", []string{"aligned"}, [][]float32{{0.01, 0, 0}}, 10); err != nil {
		t.Fatalf("store aligned: %v", err)
	}
	if err := store.StoreEmbeddings(ctx, "/tmp/large.pdf", []string{"large"}, [][]float32{{50, 50, 0}}, 10); err != nil {
		t.Fatalf("store large: %v", err)
	}

	got, err := store.SearchSemantic(ctx, []float32{1, 0, 0}, 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) == 0 || got[0].Path != "/tmp/aligned.pdf" {
		t.Errorf("top hit = %+v, want /tmp/aligned.pdf", got)
	}
}

// Embeddings survive the float32 → blob → float32 round trip intact; a
// byte-order or packing bug here would quietly corrupt every stored vector.
func TestStoreEmbeddingsRoundTripsVectorValues(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	// An exactly-representable unit vector: if the round trip were lossy or
	// mis-packed, similarity against itself would drift off 1.0.
	vec := []float32{0.5, -0.5, 0.5, -0.5}
	if err := store.StoreEmbeddings(ctx, "/tmp/rt.pdf", []string{"round trip"}, [][]float32{vec}, 10); err != nil {
		t.Fatalf("store: %v", err)
	}

	got, err := store.SearchSemantic(ctx, vec, 1)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d hits, want 1", len(got))
	}
	if got[0].Snippet != "round trip" {
		t.Errorf("snippet = %q, want %q", got[0].Snippet, "round trip")
	}
}

// Re-indexing a file replaces its chunks rather than accumulating duplicates.
func TestStoreEmbeddingsReplacesPriorChunks(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	path := "/tmp/replace.pdf"

	first := []string{"old one", "old two", "old three"}
	if err := store.StoreEmbeddings(ctx, path, first, [][]float32{{1, 0}, {0, 1}, {1, 1}}, 10); err != nil {
		t.Fatalf("store first: %v", err)
	}
	second := []string{"new only"}
	if err := store.StoreEmbeddings(ctx, path, second, [][]float32{{1, 0}}, 20); err != nil {
		t.Fatalf("store second: %v", err)
	}

	// SearchSemantic returns the best chunk per path, so the surviving snippet
	// tells us which generation of chunks is in the table.
	got, err := store.SearchSemantic(ctx, []float32{1, 0}, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d hits, want 1", len(got))
	}
	if got[0].Snippet != "new only" {
		t.Errorf("snippet = %q, want the re-indexed chunk %q", got[0].Snippet, "new only")
	}

	state, err := store.GetVecIndexState(ctx, path)
	if err != nil {
		t.Fatalf("index state: %v", err)
	}
	if state == nil {
		t.Fatal("index state is nil after storing embeddings")
	}
	if state.FileSize != 20 {
		t.Errorf("file size = %d, want the updated 20", state.FileSize)
	}
}

func TestGetVecIndexStateUnindexed(t *testing.T) {
	store := openTestStore(t)

	state, err := store.GetVecIndexState(context.Background(), "/tmp/never-indexed.pdf")
	if err != nil {
		t.Fatalf("index state: %v", err)
	}
	if state != nil {
		t.Errorf("got %+v for an unindexed path, want nil", state)
	}
}

func openTestStore(t *testing.T) *meta.Store {
	t.Helper()
	store, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
