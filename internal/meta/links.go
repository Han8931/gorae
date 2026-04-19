package meta

import (
	"context"
	"strings"
)

// Link is a directed reference from one document to another.
type Link struct {
	Target   string
	LinkText string
}

func (s *Store) initLinks() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS document_links (
  source    TEXT NOT NULL,
  target    TEXT NOT NULL,
  link_text TEXT,
  PRIMARY KEY (source, target)
);
CREATE INDEX IF NOT EXISTS idx_document_links_target ON document_links(target);
`)
	return err
}

// SetOutlinks replaces all outgoing links from source with the given slice.
func (s *Store) SetOutlinks(ctx context.Context, source string, links []Link) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `DELETE FROM document_links WHERE source = ?`, source); err != nil {
		return err
	}
	for _, l := range links {
		if strings.TrimSpace(l.Target) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO document_links(source, target, link_text) VALUES(?,?,?)`,
			source, l.Target, l.LinkText); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetOutlinks returns all outgoing links from source.
func (s *Store) GetOutlinks(ctx context.Context, source string) ([]Link, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT target, IFNULL(link_text,'') FROM document_links WHERE source = ? ORDER BY target`, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []Link
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.Target, &l.LinkText); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

// GetBacklinks returns the source paths of all documents that link to target.
func (s *Store) GetBacklinks(ctx context.Context, target string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT source FROM document_links WHERE target = ? ORDER BY source`, target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		sources = append(sources, p)
	}
	return sources, rows.Err()
}

// DeleteOutlinks removes all outgoing links from source.
func (s *Store) DeleteOutlinks(ctx context.Context, source string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM document_links WHERE source = ?`, source)
	return err
}
