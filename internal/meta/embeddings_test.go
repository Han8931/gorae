package meta_test

import (
	"context"
	"database/sql"
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
	if err := store.StoreEmbeddings(ctx, threeDim, chunks, vecs, 100, testEmbModel); err != nil {
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
	if err := store.StoreEmbeddings(ctx, "/tmp/near.pdf", []string{"near"}, [][]float32{{0.9, 0.1, 0}}, 10, testEmbModel); err != nil {
		t.Fatalf("store near: %v", err)
	}
	if err := store.StoreEmbeddings(ctx, "/tmp/far.pdf", []string{"far"}, [][]float32{{0, 0.2, 0.9}}, 10, testEmbModel); err != nil {
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

	if err := store.StoreEmbeddings(ctx, "/tmp/aligned.pdf", []string{"aligned"}, [][]float32{{0.01, 0, 0}}, 10, testEmbModel); err != nil {
		t.Fatalf("store aligned: %v", err)
	}
	if err := store.StoreEmbeddings(ctx, "/tmp/large.pdf", []string{"large"}, [][]float32{{50, 50, 0}}, 10, testEmbModel); err != nil {
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
	if err := store.StoreEmbeddings(ctx, "/tmp/rt.pdf", []string{"round trip"}, [][]float32{vec}, 10, testEmbModel); err != nil {
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
	if err := store.StoreEmbeddings(ctx, path, first, [][]float32{{1, 0}, {0, 1}, {1, 1}}, 10, testEmbModel); err != nil {
		t.Fatalf("store first: %v", err)
	}
	second := []string{"new only"}
	if err := store.StoreEmbeddings(ctx, path, second, [][]float32{{1, 0}}, 20, testEmbModel); err != nil {
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

// The recorded model is what lets :index notice that embedding_model changed
// while the file on disk did not — the one case where identical file size
// still means the stored vectors are unusable.
func TestGetVecIndexStateReportsEmbeddingModel(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	path := "/tmp/provenance.pdf"

	if err := store.StoreEmbeddings(ctx, path, []string{"body"}, [][]float32{{1, 0}}, 42, "nomic-embed-text"); err != nil {
		t.Fatalf("store: %v", err)
	}
	state, err := store.GetVecIndexState(ctx, path)
	if err != nil {
		t.Fatalf("index state: %v", err)
	}
	if state == nil {
		t.Fatal("index state is nil after storing embeddings")
	}
	if state.EmbeddingModel != "nomic-embed-text" {
		t.Errorf("embedding model = %q, want %q", state.EmbeddingModel, "nomic-embed-text")
	}

	// Re-embedding under a different model overwrites the provenance, so the
	// staleness check stops firing once the vectors have been rebuilt.
	if err := store.StoreEmbeddings(ctx, path, []string{"body"}, [][]float32{{1, 0, 0, 0}}, 42, "mxbai-embed-large"); err != nil {
		t.Fatalf("re-store: %v", err)
	}
	state, err = store.GetVecIndexState(ctx, path)
	if err != nil {
		t.Fatalf("index state after re-store: %v", err)
	}
	if state.EmbeddingModel != "mxbai-embed-large" {
		t.Errorf("embedding model = %q, want the new %q", state.EmbeddingModel, "mxbai-embed-large")
	}
}

// A DB written before vec_index_state carried a model reports "", which must
// not compare equal to any configured model: vectors of unknown provenance are
// exactly the ones that need rebuilding.
func TestGetVecIndexStateLegacyRowsHaveNoModel(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	path := "/tmp/legacy.pdf"

	if err := store.StoreEmbeddings(ctx, path, []string{"body"}, [][]float32{{1, 0}}, 42, ""); err != nil {
		t.Fatalf("store: %v", err)
	}
	state, err := store.GetVecIndexState(ctx, path)
	if err != nil {
		t.Fatalf("index state: %v", err)
	}
	if state == nil {
		t.Fatal("index state is nil after storing embeddings")
	}
	if state.EmbeddingModel != "" {
		t.Errorf("embedding model = %q, want empty", state.EmbeddingModel)
	}
}

// testEmbModel stands in for a configured embedding_model in tests whose
// subject is retrieval rather than vector provenance.
const testEmbModel = "test-embed"

func openTestStore(t *testing.T) *meta.Store {
	t.Helper()
	store, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// Opening a database written before vec_index_state had an embedding_model
// column must add it rather than fail: CREATE TABLE IF NOT EXISTS leaves the
// legacy three-column table in place, so only the ALTER brings it forward.
func TestOpenMigratesLegacyVecIndexState(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	// Build the pre-migration schema directly, bypassing Store so the old
	// shape is what is actually on disk.
	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacy.Exec(`
CREATE TABLE vec_index_state (
    path       TEXT PRIMARY KEY,
    file_size  INTEGER NOT NULL DEFAULT 0,
    indexed_at INTEGER NOT NULL DEFAULT 0
);
INSERT INTO vec_index_state(path, file_size, indexed_at) VALUES('/tmp/old.pdf', 77, 1700000000);
`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store, err := meta.Open(dbPath)
	if err != nil {
		t.Fatalf("open store on legacy db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	state, err := store.GetVecIndexState(context.Background(), "/tmp/old.pdf")
	if err != nil {
		t.Fatalf("index state: %v", err)
	}
	if state == nil {
		t.Fatal("legacy row disappeared across migration")
	}
	if state.FileSize != 77 {
		t.Errorf("file size = %d, want the preserved 77", state.FileSize)
	}
	// Unknown provenance: the row must not claim to match any live model.
	if state.EmbeddingModel != "" {
		t.Errorf("embedding model = %q, want empty for a migrated row", state.EmbeddingModel)
	}
}
