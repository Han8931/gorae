package meta

import (
	"context"
	"strings"
)

func (s *Store) initTags() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS document_tags (
  path TEXT NOT NULL,
  tag  TEXT NOT NULL,
  PRIMARY KEY (path, tag)
);
CREATE INDEX IF NOT EXISTS idx_document_tags_tag ON document_tags(tag);
`)
	return err
}

// SyncTags replaces all tags for path with those parsed from the comma-separated tagCSV.
func (s *Store) SyncTags(ctx context.Context, path, tagCSV string) error {
	tags := parseTags(tagCSV)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `DELETE FROM document_tags WHERE path = ?`, path); err != nil {
		return err
	}
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO document_tags(path, tag) VALUES(?,?)`, path, tag); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetTags returns the tags for path as a sorted slice.
func (s *Store) GetTags(ctx context.Context, path string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tag FROM document_tags WHERE path = ? ORDER BY tag`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// AllTags returns all unique tags across all documents, sorted.
func (s *Store) AllTags(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT tag FROM document_tags ORDER BY tag`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// ListByTag returns all metadata records that have the given tag.
// If tag ends with '/', it matches all tags sharing that prefix (e.g. "ml/" matches "ml/cnn").
func (s *Store) ListByTag(ctx context.Context, tag string) ([]Metadata, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, nil
	}

	var paths []string
	if strings.HasSuffix(tag, "/") {
		prefix := escapeLike(tag) + "%"
		rows, err := s.db.QueryContext(ctx,
			`SELECT DISTINCT path FROM document_tags WHERE tag LIKE ? ESCAPE '\'`, prefix)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return nil, err
			}
			paths = append(paths, p)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	} else {
		rows, err := s.db.QueryContext(ctx,
			`SELECT DISTINCT path FROM document_tags WHERE tag = ?`, tag)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return nil, err
			}
			paths = append(paths, p)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	results := make([]Metadata, 0, len(paths))
	for _, p := range paths {
		md, err := s.Get(ctx, p)
		if err != nil || md == nil {
			continue
		}
		results = append(results, *md)
	}
	return results, nil
}

func parseTags(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}
