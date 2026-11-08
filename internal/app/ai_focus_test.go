package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Han8931/gorae/internal/meta"
)

func focusTestStore(t *testing.T) *meta.Store {
	t.Helper()
	store, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func indexBody(t *testing.T, store *meta.Store, path string, runes int) {
	t.Helper()
	body := strings.TrimSpace(strings.Repeat("lorem ", runes/6+1))
	if err := store.IndexContent(context.Background(), path, body, 100); err != nil {
		t.Fatalf("index %s: %v", path, err)
	}
}

func totalRunes(blocks []string) int {
	n := 0
	for _, b := range blocks {
		n += len([]rune(b))
	}
	return n
}

// A single focused paper must behave exactly as before: capped by perPaper,
// not by the larger shared budget.
func TestBuildFocusContextSinglePaperUsesPerPaperCap(t *testing.T) {
	store := focusTestStore(t)
	indexBody(t, store, "/tmp/one.pdf", 40000)

	blocks, sources := buildFocusContext(
		context.Background(), store, []string{"/tmp/one.pdf"}, "lorem", nil, 8000, 12000)

	if len(blocks) != 1 || len(sources) != 1 {
		t.Fatalf("got %d blocks / %d sources, want 1 each", len(blocks), len(sources))
	}
	// The block carries a title header on top of the 8000-char allowance.
	if n := len([]rune(blocks[0])); n > 8000+200 {
		t.Errorf("block is %d chars, want roughly the 8000 per-paper cap", n)
	}
}

// The whole point of the budget: focusing more papers must not keep growing
// the per-turn cost. Four large papers previously cost 4 x 8000.
func TestBuildFocusContextSharesBudgetAcrossPapers(t *testing.T) {
	store := focusTestStore(t)
	var paths []string
	for _, name := range []string{"a", "b", "c", "d"} {
		p := "/tmp/" + name + ".pdf"
		indexBody(t, store, p, 40000)
		paths = append(paths, p)
	}

	blocks, sources := buildFocusContext(
		context.Background(), store, paths, "lorem", nil, 8000, 12000)

	if len(blocks) != 4 || len(sources) != 4 {
		t.Fatalf("got %d blocks / %d sources, want 4 each", len(blocks), len(sources))
	}
	// 12000 budget shared four ways, plus one title header per block.
	if n := totalRunes(blocks); n > 12000+800 {
		t.Errorf("focus context is %d chars, want it held near the 12000 budget", n)
	}
	// Every paper still appears, so none is silently dropped from the answer.
	for i, p := range paths {
		if sources[i] != p {
			t.Errorf("sources[%d] = %s, want %s", i, sources[i], p)
		}
	}
}

// Papers small enough to fit their share are sent whole rather than excerpted.
func TestBuildFocusContextKeepsSmallPapersWhole(t *testing.T) {
	store := focusTestStore(t)
	ctx := context.Background()

	const body = "a compact note about retrieval augmented generation"
	if err := store.IndexContent(ctx, "/tmp/small.pdf", body, 10); err != nil {
		t.Fatalf("index: %v", err)
	}

	blocks, _ := buildFocusContext(ctx, store, []string{"/tmp/small.pdf"}, "retrieval", nil, 8000, 12000)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if !strings.Contains(blocks[0], body) {
		t.Errorf("got %q, want the full body", blocks[0])
	}
}

// Unindexed papers contribute nothing but must not drop the ones that work,
// and must not be listed as sources for an answer they did not inform.
func TestBuildFocusContextSkipsUnindexed(t *testing.T) {
	store := focusTestStore(t)
	indexBody(t, store, "/tmp/real.pdf", 500)

	blocks, sources := buildFocusContext(
		context.Background(), store,
		[]string{"/tmp/missing.pdf", "/tmp/real.pdf"}, "lorem", nil, 8000, 12000)

	if len(blocks) != 1 || len(sources) != 1 {
		t.Fatalf("got %d blocks / %d sources, want 1 each", len(blocks), len(sources))
	}
	if sources[0] != "/tmp/real.pdf" {
		t.Errorf("sources = %q, want only the indexed paper", sources)
	}
}

func TestBuildFocusContextEmptyFocusSet(t *testing.T) {
	store := focusTestStore(t)

	blocks, sources := buildFocusContext(context.Background(), store, nil, "q", nil, 8000, 12000)
	if blocks != nil || sources != nil {
		t.Errorf("got %v / %v, want nil", blocks, sources)
	}
}

// A misconfigured budget must not silently zero out focus context.
func TestBuildFocusContextDegenerateBudget(t *testing.T) {
	store := focusTestStore(t)
	indexBody(t, store, "/tmp/x.pdf", 40000)

	blocks, _ := buildFocusContext(
		context.Background(), store, []string{"/tmp/x.pdf"}, "lorem", nil, 8000, 0)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1 (fall back to the per-paper cap)", len(blocks))
	}
	if strings.TrimSpace(blocks[0]) == "" {
		t.Error("got an empty block")
	}
}
