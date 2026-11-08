package meta

import (
	"context"
	"sort"
)

// rrfK dampens the contribution of low-ranked hits. 60 is the value from the
// original reciprocal-rank-fusion paper and behaves well without tuning.
const rrfK = 60.0

// FuseRRF merges ranked result lists with reciprocal rank fusion: every list
// contributes 1/(k+rank) to each path it returns, and paths are re-sorted by
// the summed score. Fusion happens on *rank*, never on the raw scores, because
// BM25 relevance and cosine similarity are on incomparable scales — a document
// found by both retrievers outranks one found by either alone regardless of how
// the two scored it.
//
// Each path appears once, carrying the snippet from the list where it ranked
// highest; ties fall to the earlier list. Empty lists are ignored, so this is
// safe to call when one retriever returned nothing.
func FuseRRF(topK int, lists ...[]FTSMatch) []FTSMatch {
	if topK <= 0 {
		return nil
	}

	type entry struct {
		match    FTSMatch
		score    float64
		bestRank int
		order    int // first-seen position, for a stable tiebreak
	}
	fused := map[string]*entry{}
	var order []string

	for _, list := range lists {
		for rank, m := range list {
			e, ok := fused[m.Path]
			if !ok {
				e = &entry{match: m, bestRank: rank, order: len(order)}
				fused[m.Path] = e
				order = append(order, m.Path)
			} else if rank < e.bestRank {
				// A better rank in this list means a better snippet too.
				e.match = m
				e.bestRank = rank
			}
			e.score += 1.0 / (rrfK + float64(rank) + 1.0)
		}
	}

	if len(fused) == 0 {
		return nil
	}

	ranked := make([]*entry, 0, len(fused))
	for _, path := range order {
		ranked = append(ranked, fused[path])
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].order < ranked[j].order
	})

	if len(ranked) > topK {
		ranked = ranked[:topK]
	}
	results := make([]FTSMatch, 0, len(ranked))
	for _, e := range ranked {
		results = append(results, e.match)
	}
	return results
}

// HybridSearch is the outcome of a fused search.
type HybridSearch struct {
	Matches []FTSMatch

	// Consensus is the path that keyword and semantic retrieval independently
	// ranked first. It is "" unless both retrievers ran, both returned results,
	// and their top hits agree.
	//
	// Agreement is a much stronger signal than either ranking alone: the two
	// retrievers fail in unrelated ways — keyword search on paraphrase, vector
	// search on exact terms and rare names — so one document winning both is
	// unlikely to be a coincidence of either method.
	Consensus string
}

// SearchHybrid runs keyword and vector retrieval together and fuses the two
// rankings. queryVec may be nil, in which case this degrades to a plain FTS
// search; either retriever returning nothing is not an error.
//
// query is natural-language prose, so it is converted with ProseQuery before
// reaching FTS5 — passing a question through verbatim is a syntax error on the
// first "?" or apostrophe.
//
// Both retrievers are asked for more than topK candidates so that fusion has
// room to promote documents the other one ranked highly.
func (s *Store) SearchHybrid(ctx context.Context, query string, queryVec []float32, topK int) (HybridSearch, error) {
	if topK <= 0 {
		topK = 3
	}
	candidates := topK * 3

	// Retrieval is best-effort per retriever: if one backend fails the other
	// still grounds the answer, so only a total failure is worth reporting.
	var ftsResults []FTSMatch
	var ftsErr error
	if match := ProseQuery(query); match != "" {
		ftsResults, ftsErr = s.SearchFTS(ctx, match, candidates)
	}

	var vecResults []FTSMatch
	var vecErr error
	if len(queryVec) > 0 {
		vecResults, vecErr = s.SearchSemantic(ctx, queryVec, candidates)
	}

	if len(ftsResults) == 0 && len(vecResults) == 0 {
		if ftsErr != nil {
			return HybridSearch{}, ftsErr
		}
		if vecErr != nil {
			return HybridSearch{}, vecErr
		}
		return HybridSearch{}, nil
	}

	out := HybridSearch{Matches: FuseRRF(topK, ftsResults, vecResults)}
	if len(ftsResults) > 0 && len(vecResults) > 0 && ftsResults[0].Path == vecResults[0].Path {
		out.Consensus = ftsResults[0].Path
	}
	return out, nil
}
