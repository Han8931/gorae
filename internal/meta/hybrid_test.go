package meta_test

import (
	"context"
	"testing"

	"github.com/Han8931/gorae/internal/meta"
)

func paths(matches []meta.FTSMatch) []string {
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m.Path)
	}
	return out
}

func equalPaths(got []meta.FTSMatch, want ...string) bool {
	gotPaths := paths(got)
	if len(gotPaths) != len(want) {
		return false
	}
	for i := range want {
		if gotPaths[i] != want[i] {
			return false
		}
	}
	return true
}

// The central property of fusion: a document both retrievers rank modestly
// beats one that only a single retriever ranked first.
func TestFuseRRFPromotesAgreement(t *testing.T) {
	keyword := []meta.FTSMatch{
		{Path: "/kw-only.pdf", Snippet: "kw"},
		{Path: "/both.pdf", Snippet: "kw both"},
	}
	vector := []meta.FTSMatch{
		{Path: "/vec-only.pdf", Snippet: "vec"},
		{Path: "/both.pdf", Snippet: "vec both"},
	}

	got := meta.FuseRRF(3, keyword, vector)
	if len(got) != 3 {
		t.Fatalf("got %d results %q, want 3", len(got), paths(got))
	}
	if got[0].Path != "/both.pdf" {
		t.Errorf("top hit = %s, want /both.pdf (ranked by both retrievers)", got[0].Path)
	}
}

// A path found by both lists carries the snippet from wherever it ranked best.
func TestFuseRRFKeepsBestRankedSnippet(t *testing.T) {
	keyword := []meta.FTSMatch{
		{Path: "/filler.pdf", Snippet: "filler"},
		{Path: "/doc.pdf", Snippet: "worse-ranked snippet"},
	}
	vector := []meta.FTSMatch{
		{Path: "/doc.pdf", Snippet: "better-ranked snippet"},
	}

	got := meta.FuseRRF(5, keyword, vector)
	for _, m := range got {
		if m.Path != "/doc.pdf" {
			continue
		}
		if m.Snippet != "better-ranked snippet" {
			t.Errorf("snippet = %q, want the rank-0 snippet", m.Snippet)
		}
		return
	}
	t.Fatalf("/doc.pdf missing from results %q", paths(got))
}

func TestFuseRRFPreservesOrderWithOneList(t *testing.T) {
	only := []meta.FTSMatch{
		{Path: "/a.pdf"}, {Path: "/b.pdf"}, {Path: "/c.pdf"},
	}

	// A single list has nothing to fuse against, so its ranking must survive.
	got := meta.FuseRRF(5, only)
	if !equalPaths(got, "/a.pdf", "/b.pdf", "/c.pdf") {
		t.Errorf("got %q, want the input order preserved", paths(got))
	}

	// An empty companion list must not perturb it either.
	got = meta.FuseRRF(5, only, nil)
	if !equalPaths(got, "/a.pdf", "/b.pdf", "/c.pdf") {
		t.Errorf("with empty second list: got %q, want input order", paths(got))
	}
}

func TestFuseRRFRespectsTopK(t *testing.T) {
	list := []meta.FTSMatch{
		{Path: "/a.pdf"}, {Path: "/b.pdf"}, {Path: "/c.pdf"}, {Path: "/d.pdf"},
	}

	if got := meta.FuseRRF(2, list); len(got) != 2 {
		t.Errorf("topK=2: got %d results %q, want 2", len(got), paths(got))
	}
	if got := meta.FuseRRF(0, list); got != nil {
		t.Errorf("topK=0: got %q, want nil", paths(got))
	}
	if got := meta.FuseRRF(-1, list); got != nil {
		t.Errorf("topK=-1: got %q, want nil", paths(got))
	}
}

func TestFuseRRFDeduplicatesPaths(t *testing.T) {
	a := []meta.FTSMatch{{Path: "/same.pdf", Snippet: "a"}}
	b := []meta.FTSMatch{{Path: "/same.pdf", Snippet: "b"}}
	c := []meta.FTSMatch{{Path: "/same.pdf", Snippet: "c"}}

	got := meta.FuseRRF(5, a, b, c)
	if len(got) != 1 {
		t.Errorf("got %d results %q, want a single deduplicated entry", len(got), paths(got))
	}
}

func TestFuseRRFNoInputs(t *testing.T) {
	if got := meta.FuseRRF(5); got != nil {
		t.Errorf("no lists: got %q, want nil", paths(got))
	}
	if got := meta.FuseRRF(5, nil, nil); got != nil {
		t.Errorf("only empty lists: got %q, want nil", paths(got))
	}
}

// With no query vector, hybrid search must still return keyword results.
func TestSearchHybridFallsBackToFTS(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const path = "/tmp/transformer.pdf"
	if err := store.IndexContent(ctx, path, "attention is all you need", 100); err != nil {
		t.Fatalf("index content: %v", err)
	}

	got, err := store.SearchHybrid(ctx, "attention", nil, 3)
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	if len(got) != 1 || got[0].Path != path {
		t.Fatalf("got %+v, want one keyword hit for %s", got, path)
	}
}

// Documents reachable only by embedding still surface when the keyword query
// misses them entirely — the point of running both retrievers.
func TestSearchHybridSurfacesVectorOnlyHits(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const kwPath = "/tmp/keyword.pdf"
	const vecPath = "/tmp/semantic.pdf"

	if err := store.IndexContent(ctx, kwPath, "sparse retrieval with bm25", 100); err != nil {
		t.Fatalf("index content: %v", err)
	}
	if err := store.StoreEmbeddings(ctx, vecPath, []string{"dense passage retrieval"}, [][]float32{{1, 0, 0}}, 100); err != nil {
		t.Fatalf("store embeddings: %v", err)
	}

	got, err := store.SearchHybrid(ctx, "bm25", []float32{1, 0, 0}, 5)
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	found := map[string]bool{}
	for _, m := range got {
		found[m.Path] = true
	}
	if !found[kwPath] {
		t.Errorf("keyword-only hit %s missing from %q", kwPath, paths(got))
	}
	if !found[vecPath] {
		t.Errorf("vector-only hit %s missing from %q", vecPath, paths(got))
	}
}

func TestSearchHybridEmptyStore(t *testing.T) {
	store := openTestStore(t)

	got, err := store.SearchHybrid(context.Background(), "nothing here", []float32{1, 0, 0}, 3)
	if err != nil {
		t.Fatalf("hybrid search on empty store: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %q, want no results", paths(got))
	}
}
