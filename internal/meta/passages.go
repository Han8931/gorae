package meta

import (
	"context"
	"sort"
	"strings"
)

// passageChunkWords / passageOverlapWords mirror the values used when building
// embeddings, so in-memory chunking produces comparable units to the stored
// ones.
const (
	passageChunkWords   = 400
	passageOverlapWords = 50
)

// passageGap marks omitted text between two non-adjacent passages, so the
// model can see that it is reading excerpts rather than continuous prose.
const passageGap = "\n\n[…]\n\n"

type passage struct {
	idx   int
	body  string
	score float64
}

// RelevantPassages returns the parts of one document that bear on query,
// totalling at most maxChars runes. A document already within budget is
// returned whole — excerpting it would only lose information.
//
// This exists because a focused paper is re-sent on every turn: the system
// prompt is rebuilt per request and never becomes part of the transcript, so
// "send it once" is not available. Sending the whole paper each time is what
// the budget avoids; sending the *relevant* part each time is what keeps the
// answer grounded.
//
// queryVec is optional. When present and the document has stored embeddings,
// passages are ranked by cosine similarity; otherwise they are ranked by how
// many of the query's content terms they contain.
func (s *Store) RelevantPassages(ctx context.Context, path, query string, queryVec []float32, maxChars int) (string, error) {
	if maxChars <= 0 {
		return "", nil
	}
	body, err := s.GetFileContent(ctx, path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(body) == "" {
		return "", nil
	}
	if len([]rune(body)) <= maxChars {
		return body, nil
	}

	candidates := s.embeddedPassages(ctx, path, queryVec)
	if len(candidates) == 0 {
		candidates = termScoredPassages(body, query)
	}
	if len(candidates) == 0 {
		// Nothing to rank on — fall back to the opening, as before.
		return string([]rune(body)[:maxChars]), nil
	}
	return assemblePassages(candidates, maxChars), nil
}

// embeddedPassages ranks a document's stored chunks against queryVec. It
// returns nil when there is no query vector, no stored chunks, or the stored
// vectors were written by a different embedding model.
func (s *Store) embeddedPassages(ctx context.Context, path string, queryVec []float32) []passage {
	if len(queryVec) == 0 {
		return nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT chunk_idx, body, embedding FROM vec_embeddings WHERE path = ?`, path)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []passage
	for rows.Next() {
		var idx int
		var chunk string
		var blob []byte
		if err := rows.Scan(&idx, &chunk, &blob); err != nil {
			continue
		}
		vec := bytesToFloat32s(blob)
		if len(vec) != len(queryVec) {
			continue // written by a different embedding_model
		}
		out = append(out, passage{idx: idx, body: chunk, score: float64(cosineSimilarity(queryVec, vec))})
	}
	if rows.Err() != nil {
		return nil
	}
	return out
}

// termScoredPassages chunks a body in memory and scores each chunk by the
// number of distinct query content terms it contains. This is the path taken
// when vector search is off, so focus still narrows to the relevant part of a
// long paper without requiring an embedding model.
func termScoredPassages(body, query string) []passage {
	chunks := ChunkText(body, passageChunkWords, passageOverlapWords)
	if len(chunks) == 0 {
		return nil
	}
	terms := ProseTerms(query)
	out := make([]passage, 0, len(chunks))
	for i, chunk := range chunks {
		lower := strings.ToLower(chunk)
		var hits float64
		for _, t := range terms {
			if strings.Contains(lower, t) {
				hits++
			}
		}
		out = append(out, passage{idx: i, body: chunk, score: hits})
	}
	return out
}

// assemblePassages greedily takes the highest-scoring passages that fit the
// budget, then restores document order so the excerpt reads forwards. Gaps
// between non-adjacent passages are marked.
func assemblePassages(candidates []passage, maxChars int) string {
	ranked := make([]passage, len(candidates))
	copy(ranked, candidates)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].idx < ranked[j].idx // earlier text breaks ties
	})

	var chosen []passage
	budget := maxChars
	for _, p := range ranked {
		n := len([]rune(p.body))
		if n > budget {
			// Always emit something: truncate the best passage to the budget
			// rather than returning nothing for a very small allowance.
			if len(chosen) == 0 {
				p.body = string([]rune(p.body)[:budget])
				chosen = append(chosen, p)
			}
			break
		}
		chosen = append(chosen, p)
		budget -= n
		if budget == 0 {
			break
		}
	}

	sort.Slice(chosen, func(i, j int) bool { return chosen[i].idx < chosen[j].idx })

	var sb strings.Builder
	for i, p := range chosen {
		if i > 0 {
			if chosen[i-1].idx+1 == p.idx {
				sb.WriteString("\n\n")
			} else {
				sb.WriteString(passageGap)
			}
		}
		sb.WriteString(strings.TrimSpace(p.body))
	}
	return sb.String()
}
