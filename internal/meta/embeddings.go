package meta

import (
	"context"
	"database/sql"
	"encoding/binary"
	"math"
	"sort"
	"strings"
	"time"
)

func (s *Store) initEmbeddings() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS vec_embeddings (
    path      TEXT    NOT NULL,
    chunk_idx INTEGER NOT NULL,
    body      TEXT    NOT NULL,
    embedding BLOB    NOT NULL,
    PRIMARY KEY (path, chunk_idx)
);

CREATE TABLE IF NOT EXISTS vec_index_state (
    path       TEXT PRIMARY KEY,
    file_size  INTEGER NOT NULL DEFAULT 0,
    indexed_at INTEGER NOT NULL DEFAULT 0
);
`)
	return err
}

// ChunkText splits text into overlapping word-based chunks suitable for embedding.
func ChunkText(text string, chunkWords, overlapWords int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	if chunkWords <= 0 {
		chunkWords = 400
	}
	if overlapWords < 0 {
		overlapWords = 0
	}
	var chunks []string
	step := chunkWords - overlapWords
	if step < 1 {
		step = 1
	}
	for start := 0; start < len(words); start += step {
		end := start + chunkWords
		if end > len(words) {
			end = len(words)
		}
		chunks = append(chunks, strings.Join(words[start:end], " "))
		if end == len(words) {
			break
		}
	}
	return chunks
}

// StoreEmbeddings persists all chunks and their vectors for a path.
func (s *Store) StoreEmbeddings(ctx context.Context, path string, chunks []string, vecs [][]float32, fileSize int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `DELETE FROM vec_embeddings WHERE path = ?`, path); err != nil {
		return err
	}
	for i, chunk := range chunks {
		if i >= len(vecs) {
			break
		}
		blob := float32sToBytes(vecs[i])
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO vec_embeddings(path, chunk_idx, body, embedding) VALUES(?,?,?,?)`,
			path, i, chunk, blob,
		); err != nil {
			return err
		}
	}

	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO vec_index_state(path, file_size, indexed_at) VALUES(?,?,?)
ON CONFLICT(path) DO UPDATE SET file_size=excluded.file_size, indexed_at=excluded.indexed_at
`, path, fileSize, now); err != nil {
		return err
	}

	return tx.Commit()
}

// GetVecIndexState returns the last known vector index state, or nil if not indexed.
func (s *Store) GetVecIndexState(ctx context.Context, path string) (*IndexState, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT file_size, indexed_at FROM vec_index_state WHERE path = ?`, path)
	var fileSize, indexedAt int64
	err := row.Scan(&fileSize, &indexedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &IndexState{
		FileSize:  fileSize,
		IndexedAt: time.Unix(indexedAt, 0).UTC(),
	}, nil
}

// SearchSemantic finds the top-k most relevant chunks by cosine similarity.
// It returns one result per unique path (the highest-scoring chunk).
func (s *Store) SearchSemantic(ctx context.Context, queryVec []float32, topK int) ([]FTSMatch, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path, body, embedding FROM vec_embeddings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		path  string
		body  string
		score float32
	}
	var candidates []scored

	for rows.Next() {
		var path, body string
		var blob []byte
		if err := rows.Scan(&path, &body, &blob); err != nil {
			continue
		}
		vec := bytesToFloat32s(blob)
		score := cosineSimilarity(queryVec, vec)
		candidates = append(candidates, scored{path, body, score})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// One result per path — take highest-scoring chunk per document.
	seen := map[string]bool{}
	var results []FTSMatch
	for _, c := range candidates {
		if len(results) >= topK {
			break
		}
		if seen[c.path] {
			continue
		}
		seen[c.path] = true
		snippet := c.body
		if len(snippet) > 400 {
			snippet = snippet[:400] + "…"
		}
		results = append(results, FTSMatch{Path: c.path, Snippet: snippet})
	}
	return results, nil
}

func float32sToBytes(vecs []float32) []byte {
	b := make([]byte, len(vecs)*4)
	for i, v := range vecs {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	return b
}

func bytesToFloat32s(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	vecs := make([]float32, len(b)/4)
	for i := range vecs {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		vecs[i] = math.Float32frombits(bits)
	}
	return vecs
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
