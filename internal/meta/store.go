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
	Path         string
	Title        string
	Author       string
	Year         string
	Published    string
	URL          string
	DOI          string
	Abstract     string
	Tag          string
	Favorite     bool
	ToRead       bool
	ReadingState string
}

const defaultReadingState = "unread"

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
  year   TEXT,
  published TEXT,
  url    TEXT,
  doi    TEXT,
  abstract TEXT,
  tag TEXT,
  reading_state TEXT,
  favorite INTEGER DEFAULT 0,
  to_read INTEGER DEFAULT 0
);
`)
	if err != nil {
		return err
	}
	if err := s.ensureColumn("favorite", "INTEGER DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn("to_read", "INTEGER DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn("reading_state", "TEXT"); err != nil {
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
		`SELECT path,
		        title,
		        author,
		        year,
		        IFNULL(published, ''),
		        IFNULL(url, ''),
		        IFNULL(doi, ''),
                IFNULL(abstract, ''),
		        IFNULL(tag, ''),
		        COALESCE(reading_state, ''),
		        COALESCE(favorite, 0),
		        COALESCE(to_read, 0)
		   FROM metadata WHERE path = ?`,
		path,
	)

	m, err := scanMetadataRow(row)
	switch err {
	case sql.ErrNoRows:
		return nil, nil
	case nil:
		return &m, nil
	default:
		return nil, err
	}
}

func (s *Store) Upsert(ctx context.Context, m *Metadata) error {
	favorite := 0
	if m.Favorite {
		favorite = 1
	}
	toRead := 0
	if m.ToRead {
		toRead = 1
	}
	state := normalizeReadingState(m.ReadingState)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO metadata (path, title, author, year, published, url, doi, abstract, tag, reading_state, favorite, to_read)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(path) DO UPDATE SET
  title    = excluded.title,
  author   = excluded.author,
  year     = excluded.year,
  published = excluded.published,
  url      = excluded.url,
  doi      = excluded.doi,
  abstract = excluded.abstract,
  tag      = excluded.tag,
  reading_state = excluded.reading_state,
  favorite = excluded.favorite,
  to_read  = excluded.to_read
`,
		m.Path, m.Title, m.Author, m.Year, m.Published, m.URL, m.DOI, m.Abstract, m.Tag, state, favorite, toRead,
	)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMetadataRow(scanner rowScanner) (Metadata, error) {
	md := Metadata{}
	var favorite, toRead int64
	err := scanner.Scan(
		&md.Path,
		&md.Title,
		&md.Author,
		&md.Year,
		&md.Published,
		&md.URL,
		&md.DOI,
		&md.Abstract,
		&md.Tag,
		&md.ReadingState,
		&favorite,
		&toRead,
	)
	if err != nil {
		return Metadata{}, err
	}
	md.ReadingState = normalizeReadingState(md.ReadingState)
	md.Favorite = favorite != 0
	md.ToRead = toRead != 0
	return md, nil
}

func (s *Store) listByFlag(ctx context.Context, column string) ([]Metadata, error) {
	query := fmt.Sprintf(`
SELECT path,
       title,
       author,
       year,
       IFNULL(published, ''),
       IFNULL(url, ''),
       IFNULL(doi, ''),
       IFNULL(abstract, ''),
       IFNULL(tag, ''),
       IFNULL(reading_state, ''),
       COALESCE(favorite, 0),
       COALESCE(to_read, 0)
  FROM metadata
 WHERE COALESCE(%s, 0) = 1
ORDER BY LOWER(title), path
`, column)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]Metadata, 0)
	for rows.Next() {
		md, err := scanMetadataRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, md)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *Store) ListFavorites(ctx context.Context) ([]Metadata, error) {
	return s.listByFlag(ctx, "favorite")
}

func (s *Store) ListToRead(ctx context.Context) ([]Metadata, error) {
	return s.listByFlag(ctx, "to_read")
}

func (s *Store) ListByReadingState(ctx context.Context, state string) ([]Metadata, error) {
	state = normalizeReadingState(state)
	query := `
SELECT path,
       title,
       author,
       year,
       IFNULL(published, ''),
       IFNULL(url, ''),
       IFNULL(doi, ''),
       IFNULL(abstract, ''),
       IFNULL(tag, ''),
       IFNULL(reading_state, ''),
       COALESCE(favorite, 0),
       COALESCE(to_read, 0)
  FROM metadata
 WHERE LOWER(IFNULL(reading_state, '')) = LOWER(?)
 ORDER BY LOWER(title), path`
	rows, err := s.db.QueryContext(ctx, query, state)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]Metadata, 0)
	for rows.Next() {
		md, err := scanMetadataRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, md)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func normalizeReadingState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "reading":
		return "reading"
	case "read":
		return "read"
	default:
		return defaultReadingState
	}
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
