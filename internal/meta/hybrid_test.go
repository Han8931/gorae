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

	res, err := store.SearchHybrid(ctx, "attention", nil, 3)
	got := res.Matches
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

	res, err := store.SearchHybrid(ctx, "bm25", []float32{1, 0, 0}, 5)
	got := res.Matches
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

// Consensus is the auto-focus signal: both retrievers, ranking independently,
// put the same document first.
func TestSearchHybridConsensusWhenRetrieversAgree(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const agreed = "/tmp/agreed.pdf"
	const other = "/tmp/other.pdf"
	if err := store.IndexContent(ctx, agreed, "attention and memory bandwidth", 100); err != nil {
		t.Fatalf("index agreed: %v", err)
	}
	if err := store.IndexContent(ctx, other, "attention and parsing grammars", 100); err != nil {
		t.Fatalf("index other: %v", err)
	}
	if err := store.StoreEmbeddings(ctx, agreed, []string{"memory"}, [][]float32{{1, 0, 0}}, 100); err != nil {
		t.Fatalf("embed agreed: %v", err)
	}
	if err := store.StoreEmbeddings(ctx, other, []string{"parsing"}, [][]float32{{0, 1, 0}}, 100); err != nil {
		t.Fatalf("embed other: %v", err)
	}

	res, err := store.SearchHybrid(ctx, "memory bandwidth", []float32{1, 0, 0}, 5)
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	if res.Consensus != agreed {
		t.Errorf("Consensus = %q, want %q", res.Consensus, agreed)
	}
}

// Disagreement must not auto-focus anything — that is the whole safeguard.
func TestSearchHybridNoConsensusWhenRetrieversDisagree(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const kwTop = "/tmp/keyword-top.pdf"
	const vecTop = "/tmp/vector-top.pdf"
	if err := store.IndexContent(ctx, kwTop, "memory bandwidth discussion", 100); err != nil {
		t.Fatalf("index kwTop: %v", err)
	}
	if err := store.IndexContent(ctx, vecTop, "unrelated grammar parsing", 100); err != nil {
		t.Fatalf("index vecTop: %v", err)
	}
	// Embeddings point the vector retriever at the *other* document.
	if err := store.StoreEmbeddings(ctx, kwTop, []string{"kw"}, [][]float32{{0, 1, 0}}, 100); err != nil {
		t.Fatalf("embed kwTop: %v", err)
	}
	if err := store.StoreEmbeddings(ctx, vecTop, []string{"vec"}, [][]float32{{1, 0, 0}}, 100); err != nil {
		t.Fatalf("embed vecTop: %v", err)
	}

	res, err := store.SearchHybrid(ctx, "memory bandwidth", []float32{1, 0, 0}, 5)
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	if res.Consensus != "" {
		t.Errorf("Consensus = %q, want empty when the retrievers disagree", res.Consensus)
	}
}

// One retriever cannot agree with itself. With vector search off there is no
// consensus signal at all, so auto-focus stays dormant.
func TestSearchHybridNoConsensusFromOneRetriever(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const path = "/tmp/solo.pdf"
	if err := store.IndexContent(ctx, path, "memory bandwidth discussion", 100); err != nil {
		t.Fatalf("index: %v", err)
	}
	if err := store.StoreEmbeddings(ctx, path, []string{"memory"}, [][]float32{{1, 0, 0}}, 100); err != nil {
		t.Fatalf("embed: %v", err)
	}

	// Keyword only — no query vector.
	res, err := store.SearchHybrid(ctx, "memory bandwidth", nil, 5)
	if err != nil {
		t.Fatalf("keyword-only search: %v", err)
	}
	if len(res.Matches) == 0 {
		t.Fatal("keyword-only search returned no matches")
	}
	if res.Consensus != "" {
		t.Errorf("keyword only: Consensus = %q, want empty", res.Consensus)
	}

	// Vector only — the query's terms appear nowhere in the body.
	res, err = store.SearchHybrid(ctx, "zygomorphic palimpsest", []float32{1, 0, 0}, 5)
	if err != nil {
		t.Fatalf("vector-only search: %v", err)
	}
	if len(res.Matches) == 0 {
		t.Fatal("vector-only search returned no matches")
	}
	if res.Consensus != "" {
		t.Errorf("vector only: Consensus = %q, want empty", res.Consensus)
	}
}

func TestSearchHybridEmptyStore(t *testing.T) {
	store := openTestStore(t)

	res, err := store.SearchHybrid(context.Background(), "nothing here", []float32{1, 0, 0}, 3)
	got := res.Matches
	if err != nil {
		t.Fatalf("hybrid search on empty store: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %q, want no results", paths(got))
	}
}
