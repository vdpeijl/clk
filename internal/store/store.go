// Package store is the SQLite repository for events, sessions, and push links.
// It uses the pure-Go modernc.org/sqlite driver (no CGO).
package store

import (
	"database/sql"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
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
	// TODO: implement schema migrations
	return nil
}
