package meta

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	AddedAt      time.Time
	LastOpenedAt time.Time
}

const defaultReadingState = "unread"

type Store struct {
	db *sql.DB
}

const metadataSelectColumns = `
  path,
  IFNULL(title, ''),
  IFNULL(author, ''),
  IFNULL(year, ''),
  IFNULL(published, ''),
  IFNULL(url, ''),
  IFNULL(doi, ''),
  IFNULL(abstract, ''),
  IFNULL(tag, ''),
  COALESCE(reading_state, ''),
  COALESCE(favorite, 0),
  COALESCE(to_read, 0),
  COALESCE(added_at, 0),
  COALESCE(last_opened_at, 0)
`

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
  to_read INTEGER DEFAULT 0,
  added_at INTEGER,
  last_opened_at INTEGER
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
	if err := s.ensureColumn("tag", "TEXT"); err != nil {
		return err
	}
	if err := s.ensureColumn("added_at", "INTEGER"); err != nil {
		return err
	}
	return s.ensureColumn("last_opened_at", "INTEGER")
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
		`SELECT`+metadataSelectColumns+`
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
	addedAt := m.AddedAt
	if addedAt.IsZero() {
		addedAt = time.Now()
	}
	addedAtUnix := addedAt.Unix()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO metadata (path, title, author, year, published, url, doi, abstract, tag, reading_state, favorite, to_read, added_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
  to_read  = excluded.to_read,
  added_at = CASE
                WHEN COALESCE(metadata.added_at, 0) = 0 THEN excluded.added_at
                ELSE metadata.added_at
             END
`,
		m.Path, m.Title, m.Author, m.Year, m.Published, m.URL, m.DOI, m.Abstract, m.Tag, state, favorite, toRead, addedAtUnix,
	)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMetadataRow(scanner rowScanner) (Metadata, error) {
	md := Metadata{}
	var favorite, toRead int64
	var addedAt, openedAt sql.NullInt64
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
		&addedAt,
		&openedAt,
	)
	if err != nil {
		return Metadata{}, err
	}
	md.ReadingState = normalizeReadingState(md.ReadingState)
	md.Favorite = favorite != 0
	md.ToRead = toRead != 0
	if addedAt.Valid && addedAt.Int64 > 0 {
		md.AddedAt = time.Unix(addedAt.Int64, 0).UTC()
	}
	if openedAt.Valid && openedAt.Int64 > 0 {
		md.LastOpenedAt = time.Unix(openedAt.Int64, 0).UTC()
	}
	return md, nil
}

func (s *Store) listByFlag(ctx context.Context, column string) ([]Metadata, error) {
	query := fmt.Sprintf(`
SELECT%[1]s
  FROM metadata
 WHERE COALESCE(%s, 0) = 1
ORDER BY LOWER(title), path
`, metadataSelectColumns, column)
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
SELECT` + metadataSelectColumns + `
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

func (s *Store) RecordOpened(ctx context.Context, path string, openedAt time.Time) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if openedAt.IsZero() {
		openedAt = time.Now()
	}
	ts := openedAt.Unix()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO metadata (path, reading_state, added_at, last_opened_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(path) DO UPDATE SET
  last_opened_at = excluded.last_opened_at,
  added_at = CASE
                WHEN COALESCE(metadata.added_at, 0) = 0 THEN excluded.added_at
                ELSE metadata.added_at
             END
`,
		path, defaultReadingState, ts, ts,
	)
	return err
}

func (s *Store) ListRecentlyOpened(ctx context.Context, limit int) ([]Metadata, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
SELECT` + metadataSelectColumns + `
  FROM metadata
 WHERE COALESCE(last_opened_at, 0) > 0
 ORDER BY last_opened_at DESC, LOWER(title), path
 LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]Metadata, 0, limit)
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
