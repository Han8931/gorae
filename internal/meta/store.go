package meta

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	_ "github.com/mattn/go-sqlite3"
)

type Metadata struct {
	Path      string
	Title     string
	Author    string
	Venue     string
	Year      string
	Published string
	URL       string
	DOI       string
	Abstract  string
	Tag       string
}

type Store struct {
	db *sql.DB
}

func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) initSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS metadata (
  path   TEXT PRIMARY KEY,
  title  TEXT,
  author TEXT,
  venue  TEXT,
  year   TEXT,
  published TEXT,
  url    TEXT,
  doi    TEXT,
  abstract TEXT,
  tag TEXT
);
`)
	if err != nil {
		return err
	}
	if err := s.ensureColumn("published", "TEXT"); err != nil {
		return err
	}
	if err := s.ensureColumn("url", "TEXT"); err != nil {
		return err
	}
	if err := s.ensureColumn("doi", "TEXT"); err != nil {
		return err
	}
	if err := s.ensureColumn("abstract", "TEXT"); err != nil {
		return err
	}
	return s.ensureColumn("tag", "TEXT")
}

func (s *Store) ensureColumn(name, typ string) error {
	query := fmt.Sprintf(`ALTER TABLE metadata ADD COLUMN %s %s`, name, typ)
	_, err := s.db.Exec(query)
	if err != nil {
		errLower := strings.ToLower(err.Error())
		if strings.Contains(errLower, "duplicate column name") {
			return nil
		}
	}
	return err
}

func (s *Store) Get(ctx context.Context, path string) (*Metadata, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT path, title, author, venue, year,
		        IFNULL(published, ''),
		        IFNULL(url, ''),
		        IFNULL(doi, ''),
		        IFNULL(abstract, ''),
		        IFNULL(tag, '')
		   FROM metadata WHERE path = ?`,
		path,
	)

	m := Metadata{}
	switch err := row.Scan(&m.Path, &m.Title, &m.Author, &m.Venue, &m.Year, &m.Published, &m.URL, &m.DOI, &m.Abstract, &m.Tag); err {
	case sql.ErrNoRows:
		return nil, nil
	case nil:
		return &m, nil
	default:
		return nil, err
	}
}

func (s *Store) Upsert(ctx context.Context, m *Metadata) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO metadata (path, title, author, venue, year, published, url, doi, abstract, tag)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(path) DO UPDATE SET
  title    = excluded.title,
  author   = excluded.author,
  venue    = excluded.venue,
  year     = excluded.year,
  published = excluded.published,
  url      = excluded.url,
  doi      = excluded.doi,
  abstract = excluded.abstract,
  tag      = excluded.tag
`,
		m.Path, m.Title, m.Author, m.Venue, m.Year, m.Published, m.URL, m.DOI, m.Abstract, m.Tag,
	)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) MovePath(ctx context.Context, oldPath, newPath string) error {
	if oldPath == newPath {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE metadata SET path = ? WHERE path = ?`, newPath, oldPath)
	return err
}

func (s *Store) MoveTree(ctx context.Context, oldDir, newDir string) error {
	oldPrefix, err := normalizeDirPrefix(oldDir)
	if err != nil {
		return err
	}
	newPrefix, err := normalizeDirPrefix(newDir)
	if err != nil {
		return err
	}
	if oldPrefix == newPrefix {
		return nil
	}
	start := utf8.RuneCountInString(oldPrefix) + 1
	pattern := escapeLike(oldPrefix) + "%"
	_, err = s.db.ExecContext(ctx, `
UPDATE metadata
SET path = ?1 || substr(path, ?2)
WHERE path LIKE ?3 ESCAPE '\'
`, newPrefix, start, pattern)
	return err
}

func normalizeDirPrefix(path string) (string, error) {
	cleaned := filepath.Clean(path)
	if cleaned == "" || cleaned == "." {
		return "", fmt.Errorf("path %q must not be empty", path)
	}
	return ensureTrailingSlash(cleaned), nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func ensureTrailingSlash(path string) string {
	if path == "" {
		return string(os.PathSeparator)
	}
	sep := string(os.PathSeparator)
	if strings.HasSuffix(path, sep) {
		return path
	}
	return path + sep
}
