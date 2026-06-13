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
