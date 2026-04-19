package meta

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// IndexState records when a file was last indexed and its size at that time.
type IndexState struct {
	FileSize  int64
	IndexedAt time.Time
}

// FTSMatch is a single result from a full-text search.
type FTSMatch struct {
	Path    string
	Snippet string
}

func (s *Store) initFTS() error {
	_, err := s.db.Exec(`
CREATE VIRTUAL TABLE IF NOT EXISTS fts_content USING fts5(
  path UNINDEXED,
  body,
  tokenize = 'porter unicode61'
);

CREATE TABLE IF NOT EXISTS fts_index_state (
  path       TEXT PRIMARY KEY,
  file_size  INTEGER NOT NULL DEFAULT 0,
  indexed_at INTEGER NOT NULL DEFAULT 0
);
`)
	return err
}

// IndexContent stores the extracted text for path in the FTS index.
// If fileSize matches the previously indexed size, the call is a no-op.
func (s *Store) IndexContent(ctx context.Context, path, text string, fileSize int64) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `DELETE FROM fts_content WHERE path = ?`, path); err != nil {
		return err
	}
	if strings.TrimSpace(text) != "" {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO fts_content(path, body) VALUES (?, ?)`, path, text); err != nil {
			return err
		}
	}

	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO fts_index_state(path, file_size, indexed_at) VALUES(?,?,?)
ON CONFLICT(path) DO UPDATE SET file_size=excluded.file_size, indexed_at=excluded.indexed_at
`, path, fileSize, now); err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteFromIndex removes path from both the FTS content table and index state.
func (s *Store) DeleteFromIndex(ctx context.Context, path string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `DELETE FROM fts_content WHERE path = ?`, path); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM fts_index_state WHERE path = ?`, path); err != nil {
		return err
	}
	return tx.Commit()
}

// GetIndexState returns the last known index state for path, or nil if never indexed.
func (s *Store) GetIndexState(ctx context.Context, path string) (*IndexState, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT file_size, indexed_at FROM fts_index_state WHERE path = ?`, path)
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

// SearchFTS performs a full-text search and returns matches with highlighted snippets.
// The FTS5 snippet markers are ** (bold markers) around matched terms.
func (s *Store) SearchFTS(ctx context.Context, query string, limit int) ([]FTSMatch, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT path, snippet(fts_content, 1, '**', '**', '...', 32)
FROM fts_content
WHERE fts_content MATCH ?
ORDER BY rank
LIMIT ?
`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []FTSMatch
	for rows.Next() {
		var m FTSMatch
		if err := rows.Scan(&m.Path, &m.Snippet); err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// IndexedCount returns how many files have been indexed.
func (s *Store) IndexedCount(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fts_index_state`)
	var n int
	return n, row.Scan(&n)
}
