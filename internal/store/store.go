// Package store is the SQLite repository for events, sessions, and push links.
// It uses the pure-Go modernc.org/sqlite driver (no CGO).
package store

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver

	"github.com/vdpeijl/clk/internal/sessions"
)

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at the given path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// A busy timeout lets a reader (e.g. clk list) and the daemon writer share
	// the database without spurious "database is locked" errors.
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS events (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp     INTEGER NOT NULL,
	event_type    TEXT NOT NULL,
	source        TEXT NOT NULL,
	project_token TEXT NOT NULL,
	tool_name     TEXT NOT NULL DEFAULT '',
	description   TEXT NOT NULL DEFAULT '',
	file_path     TEXT NOT NULL DEFAULT '',
	branch        TEXT NOT NULL DEFAULT '',
	issue_id      TEXT NOT NULL DEFAULT '',
	topic         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);

CREATE TABLE IF NOT EXISTS sessions (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	project_token    TEXT NOT NULL,
	start_ts         INTEGER NOT NULL,
	end_ts           INTEGER NOT NULL,
	duration_seconds INTEGER NOT NULL,
	source           TEXT NOT NULL DEFAULT '',
	branch           TEXT NOT NULL DEFAULT '',
	issue_id         TEXT NOT NULL DEFAULT '',
	description      TEXT NOT NULL DEFAULT '',
	event_count      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_sessions_start ON sessions(start_ts);

CREATE TABLE IF NOT EXISTS projects (
	root          TEXT PRIMARY KEY,
	token         TEXT NOT NULL,
	registered_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS push_links (
	session_key  TEXT PRIMARY KEY,
	entry_id     TEXT NOT NULL,
	content_hash TEXT NOT NULL,
	pushed_at    INTEGER NOT NULL
);
`
	_, err := s.db.Exec(schema)
	return err
}

// InsertEvent persists a single event and returns its assigned id.
func (s *Store) InsertEvent(e sessions.Event) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO events
			(timestamp, event_type, source, project_token, description, file_path, branch, issue_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Timestamp.Unix(),
		e.Type,
		e.Source,
		e.ProjectToken,
		e.Description,
		e.FilePath,
		e.Branch,
		e.IssueID,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Project is a local repository registered with `clk init`. The daemon watches
// each registered root for file-change heartbeats.
type Project struct {
	Root         string
	Token        string
	RegisteredAt time.Time
}

// RegisterProject records (or refreshes) a project root in the local registry,
// keyed by its absolute root path so re-running `clk init` updates rather than
// duplicates the entry.
func (s *Store) RegisterProject(root, token string, at time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO projects (root, token, registered_at) VALUES (?, ?, ?)
		 ON CONFLICT(root) DO UPDATE SET token = excluded.token, registered_at = excluded.registered_at`,
		root, token, at.Unix(),
	)
	return err
}

// Projects returns every registered project, ordered by root for stability.
func (s *Store) Projects() ([]Project, error) {
	rows, err := s.db.Query(`SELECT root, token, registered_at FROM projects ORDER BY root ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Project
	for rows.Next() {
		var (
			p  Project
			at int64
		)
		if err := rows.Scan(&p.Root, &p.Token, &at); err != nil {
			return nil, err
		}
		p.RegisteredAt = time.Unix(at, 0)
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// EventsBetween returns all events whose timestamp falls in [start, end),
// ordered chronologically.
func (s *Store) EventsBetween(start, end time.Time) ([]sessions.Event, error) {
	rows, err := s.db.Query(
		`SELECT id, timestamp, event_type, source, project_token, description, file_path, branch, issue_id
		 FROM events
		 WHERE timestamp >= ? AND timestamp < ?
		 ORDER BY timestamp ASC, id ASC`,
		start.Unix(),
		end.Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []sessions.Event
	for rows.Next() {
		var (
			e  sessions.Event
			ts int64
		)
		if err := rows.Scan(
			&e.ID, &ts, &e.Type, &e.Source, &e.ProjectToken,
			&e.Description, &e.FilePath, &e.Branch, &e.IssueID,
		); err != nil {
			return nil, err
		}
		e.Timestamp = time.Unix(ts, 0)
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

// ReplaceSessionsBetween atomically rewrites the materialized sessions whose
// start falls in [start, end) with the supplied ones. The daemon calls this
// after reconstructing events for a window, so re-materialization is
// idempotent and adjacency merges (which extend or coalesce sessions) are
// reflected in place rather than duplicated.
func (s *Store) ReplaceSessionsBetween(start, end time.Time, ss []sessions.Session) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	if _, err := tx.Exec(
		`DELETE FROM sessions WHERE start_ts >= ? AND start_ts < ?`,
		start.Unix(), end.Unix(),
	); err != nil {
		return err
	}

	stmt, err := tx.Prepare(
		`INSERT INTO sessions
			(project_token, start_ts, end_ts, duration_seconds, source, branch, issue_id, description, event_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sess := range ss {
		if _, err := stmt.Exec(
			sess.ProjectToken,
			sess.Start.Unix(),
			sess.End.Unix(),
			int64(sess.Duration().Seconds()),
			sess.Source,
			sess.Branch,
			sess.IssueID,
			sess.Description,
			sess.EventCount,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// PushLink records that a session (identified by its stable key) was pushed to
// Clockify as a particular entry, plus a content hash of the pushed payload.
// The hash drives the create/update/skip decision on the next push; deletion is
// only ever explicit via `clk unpush`.
type PushLink struct {
	SessionKey      string
	ClockifyEntryID string
	ContentHash     string
	PushedAt        time.Time
}

// UpsertPushLink records (or refreshes) the link for a session key, keyed so a
// re-push updates the existing row rather than duplicating it.
func (s *Store) UpsertPushLink(l PushLink) error {
	_, err := s.db.Exec(
		`INSERT INTO push_links (session_key, entry_id, content_hash, pushed_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(session_key) DO UPDATE SET
			entry_id = excluded.entry_id,
			content_hash = excluded.content_hash,
			pushed_at = excluded.pushed_at`,
		l.SessionKey, l.ClockifyEntryID, l.ContentHash, l.PushedAt.Unix(),
	)
	return err
}

// PushLinks returns every recorded push link, ordered by session key for
// stability. It is the prior-push state the pushplanner diffs against.
func (s *Store) PushLinks() ([]PushLink, error) {
	rows, err := s.db.Query(
		`SELECT session_key, entry_id, content_hash, pushed_at
		 FROM push_links ORDER BY session_key ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PushLink
	for rows.Next() {
		var (
			l        PushLink
			pushedAt int64
		)
		if err := rows.Scan(&l.SessionKey, &l.ClockifyEntryID, &l.ContentHash, &pushedAt); err != nil {
			return nil, err
		}
		l.PushedAt = time.Unix(pushedAt, 0)
		result = append(result, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// DeletePushLink removes the link for a session key. `clk unpush` calls it after
// deleting the entry from Clockify so the next push treats the session as new.
func (s *Store) DeletePushLink(sessionKey string) error {
	_, err := s.db.Exec(`DELETE FROM push_links WHERE session_key = ?`, sessionKey)
	return err
}

// SessionsBetween returns the materialized sessions whose start falls in
// [start, end), ordered chronologically.
func (s *Store) SessionsBetween(start, end time.Time) ([]sessions.Session, error) {
	rows, err := s.db.Query(
		`SELECT id, project_token, start_ts, end_ts, source, branch, issue_id, description, event_count
		 FROM sessions
		 WHERE start_ts >= ? AND start_ts < ?
		 ORDER BY start_ts ASC, id ASC`,
		start.Unix(),
		end.Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []sessions.Session
	for rows.Next() {
		var (
			sess           sessions.Session
			startTS, endTS int64
		)
		if err := rows.Scan(
			&sess.ID, &sess.ProjectToken, &startTS, &endTS,
			&sess.Source, &sess.Branch, &sess.IssueID, &sess.Description, &sess.EventCount,
		); err != nil {
			return nil, err
		}
		sess.Start = time.Unix(startTS, 0)
		sess.End = time.Unix(endTS, 0)
		result = append(result, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
