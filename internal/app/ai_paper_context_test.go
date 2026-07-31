package app

import "testing"

func TestAddFocusedPaperDedupAndCap(t *testing.T) {
	var m Model
	if !m.addFocusedPaper("/a.pdf") {
		t.Fatal("expected first add to report added")
	}
	if m.addFocusedPaper("/a.pdf") {
		t.Fatal("expected duplicate add to report not-added")
	}
	if got := len(m.aiFocusedFiles); got != 1 {
		t.Fatalf("expected 1 focused paper, got %d", got)
	}

	// Overflow the cap; only the most recent maxFocusedPapers should remain.
	paths := []string{"/b", "/c", "/d", "/e", "/f", "/g"}
	for _, p := range paths {
		m.addFocusedPaper(p)
	}
	if got := len(m.aiFocusedFiles); got != maxFocusedPapers {
		t.Fatalf("expected %d focused papers after overflow, got %d", maxFocusedPapers, got)
	}
	if m.primaryFocusedPaper() != "/g" {
		t.Fatalf("expected most recent paper to be primary, got %q", m.primaryFocusedPaper())
	}
	// Oldest ("/a.pdf") should have been dropped.
	for _, p := range m.aiFocusedFiles {
		if p == "/a.pdf" {
			t.Fatal("oldest paper should have been evicted past the cap")
		}
	}
}

func TestToggleFindMark(t *testing.T) {
	var m Model
	m.toggleFindMark("/a")
	m.toggleFindMark("/b")
	if !m.isFindMarked("/a") || !m.isFindMarked("/b") {
		t.Fatal("expected /a and /b to be marked")
	}
	if len(m.aiFindMarked) != 2 {
		t.Fatalf("expected 2 marks, got %d", len(m.aiFindMarked))
	}
	// Toggling again unmarks and preserves order of the rest.
	m.toggleFindMark("/a")
	if m.isFindMarked("/a") {
		t.Fatal("expected /a to be unmarked")
	}
	if len(m.aiFindMarked) != 1 || m.aiFindMarked[0] != "/b" {
		t.Fatalf("expected only /b to remain, got %v", m.aiFindMarked)
	}
}

func TestParseFocusedPaths(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"/one.pdf", 1},
		{"/one.pdf\n/two.pdf", 2},
		{"/one.pdf\n\n  \n/two.pdf\n", 2},
	}
	for _, c := range cases {
		if got := len(parseFocusedPaths(c.in)); got != c.want {
			t.Errorf("parseFocusedPaths(%q) = %d paths, want %d", c.in, got, c.want)
		}
	}
}

func TestPrependUnique(t *testing.T) {
	got := prependUnique([]string{"a", "b"}, []string{"b", "c", "a", "d"})
	want := []string{"a", "b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestIsGenericPaperRef(t *testing.T) {
	generic := []string{"this paper", "the study", "that", "  ", "this work", "the paper about"}
	for _, g := range generic {
		if !isGenericPaperRef(g) {
			t.Errorf("expected %q to be generic", g)
		}
	}
	specific := []string{"diffusion planning", "attention is all you need", "Vaswani 2017", "retrieval augmented"}
	for _, s := range specific {
		if isGenericPaperRef(s) {
			t.Errorf("expected %q to be specific", s)
		}
	}
}

func TestPaperRefCues(t *testing.T) {
	cued := []string{
		"summarize the paper on diffusion",
		"compare these two approaches",
		"what is the relationship between them",
		`open "Attention Is All You Need"`,
	}
	for _, c := range cued {
		if !paperRefCues(c) {
			t.Errorf("expected cue in %q", c)
		}
	}
	uncued := []string{"hello there", "what is 2+2", "thanks!"}
	for _, u := range uncued {
		if paperRefCues(u) {
			t.Errorf("did not expect cue in %q", u)
		}
	}
}

func TestBuildFTSPrefixQuery(t *testing.T) {
	cases := map[string]string{
		"":                  "",
		"   ":               "",
		"neural":            "neural*",
		"neural nets":       "neural* nets*",
		"long-context, GPT": "long* context* GPT*",
	}
	for in, want := range cases {
		if got := buildFTSPrefixQuery(in); got != want {
			t.Errorf("buildFTSPrefixQuery(%q) = %q, want %q", in, got, want)
		}
	}
}
