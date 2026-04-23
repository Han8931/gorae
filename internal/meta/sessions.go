package meta

import (
	"context"
	"time"
)

// ChatSession is a persisted AI conversation.
type ChatSession struct {
	ID           int64
	Title        string
	FocusedFile  string
	MessageCount int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ChatMessage is a single persisted message within a session.
type ChatMessage struct {
	Role      string
	Content   string
	Thinking  string
	IsSummary bool
}

func (s *Store) initSessions() error {
	if _, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS chat_sessions (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	title        TEXT    NOT NULL DEFAULT '',
	focused_file TEXT    NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL,
	updated_at   INTEGER NOT NULL
)`); err != nil {
		return err
	}
	// Migration: add focused_file for DBs created before this column existed.
	s.db.Exec(`ALTER TABLE chat_sessions ADD COLUMN focused_file TEXT NOT NULL DEFAULT ''`)
	if _, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS chat_messages (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id INTEGER NOT NULL,
	role       TEXT    NOT NULL,
	content    TEXT    NOT NULL DEFAULT '',
	thinking   TEXT    NOT NULL DEFAULT '',
	is_summary INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL,
	FOREIGN KEY (session_id) REFERENCES chat_sessions(id) ON DELETE CASCADE
)`); err != nil {
		return err
	}
	// Migration: add is_summary for DBs created before this column existed.
	s.db.Exec(`ALTER TABLE chat_messages ADD COLUMN is_summary INTEGER NOT NULL DEFAULT 0`)
	return nil
}

// CreateSession inserts a new session and returns its ID.
func (s *Store) CreateSession(ctx context.Context, title string) (int64, error) {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO chat_sessions (title, created_at, updated_at) VALUES (?, ?, ?)`,
		title, now, now,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListSessions returns all sessions ordered by most recently updated.
func (s *Store) ListSessions(ctx context.Context) ([]ChatSession, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT s.id, s.title, s.focused_file, s.created_at, s.updated_at,
       COUNT(m.id) AS message_count
FROM   chat_sessions s
LEFT JOIN chat_messages m ON m.session_id = s.id
GROUP BY s.id
ORDER BY s.updated_at DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ChatSession
	for rows.Next() {
		var ss ChatSession
		var createdAt, updatedAt int64
		if err := rows.Scan(&ss.ID, &ss.Title, &ss.FocusedFile, &createdAt, &updatedAt, &ss.MessageCount); err != nil {
			return nil, err
		}
		ss.CreatedAt = time.Unix(createdAt, 0)
		ss.UpdatedAt = time.Unix(updatedAt, 0)
		out = append(out, ss)
	}
	return out, rows.Err()
}

// UpdateSessionFocusedFile records the file that was focused during a session.
func (s *Store) UpdateSessionFocusedFile(ctx context.Context, id int64, path string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE chat_sessions SET focused_file = ? WHERE id = ?`, path, id,
	)
	return err
}

// DeleteSession removes a session and all its messages.
func (s *Store) DeleteSession(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chat_sessions WHERE id = ?`, id)
	return err
}

// SaveMessage appends a message to the given session.
func (s *Store) SaveMessage(ctx context.Context, sessionID int64, role, content, thinking string) error {
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO chat_messages (session_id, role, content, thinking, created_at) VALUES (?, ?, ?, ?, ?)`,
		sessionID, role, content, thinking, now,
	); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE chat_sessions SET updated_at = ? WHERE id = ?`,
		now, sessionID,
	)
	return err
}

// LoadMessages returns all messages for a session in chronological order.
func (s *Store) LoadMessages(ctx context.Context, sessionID int64) ([]ChatMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT role, content, thinking, is_summary
FROM   chat_messages
WHERE  session_id = ?
ORDER BY created_at ASC, id ASC
`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ChatMessage
	for rows.Next() {
		var cm ChatMessage
		var isSummary int
		if err := rows.Scan(&cm.Role, &cm.Content, &cm.Thinking, &isSummary); err != nil {
			return nil, err
		}
		cm.IsSummary = isSummary != 0
		out = append(out, cm)
	}
	return out, rows.Err()
}

// CompactSession replaces the oldest messages with a summary, keeping the
// most recent keepLast messages verbatim.
func (s *Store) CompactSession(ctx context.Context, sessionID int64, summary string, keepLast int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Find the created_at of the oldest message to keep, so we know the cutoff.
	var cutoff int64
	err = tx.QueryRowContext(ctx, `
SELECT created_at FROM chat_messages
WHERE  session_id = ?
ORDER BY created_at DESC, id DESC
LIMIT 1 OFFSET ?
`, sessionID, keepLast-1).Scan(&cutoff)
	if err != nil {
		return err
	}

	// Record the earliest timestamp so the summary sorts before everything else.
	var earliest int64
	if err := tx.QueryRowContext(ctx,
		`SELECT MIN(created_at) FROM chat_messages WHERE session_id = ?`, sessionID,
	).Scan(&earliest); err != nil {
		return err
	}

	// Delete the messages that are being summarised (older than the cutoff).
	if _, err := tx.ExecContext(ctx, `
DELETE FROM chat_messages
WHERE  session_id = ? AND created_at < ?
`, sessionID, cutoff); err != nil {
		return err
	}

	// Insert the summary before the remaining messages.
	if _, err := tx.ExecContext(ctx, `
INSERT INTO chat_messages (session_id, role, content, thinking, is_summary, created_at)
VALUES (?, 'assistant', ?, '', 1, ?)
`, sessionID, summary, earliest-1); err != nil {
		return err
	}

	return tx.Commit()
}

// ClearSessionMessages deletes all messages in a session without deleting the session itself.
func (s *Store) ClearSessionMessages(ctx context.Context, sessionID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chat_messages WHERE session_id = ?`, sessionID)
	return err
}
