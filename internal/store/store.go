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
