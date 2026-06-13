// Package sessions reconstructs work sessions from raw events using a
// gap-based algorithm. It is pure: no I/O, no global state.
package sessions

import "time"

// Event is a single captured dev activity signal.
type Event struct {
	ID          int64
	Timestamp   time.Time
	Type        string
	Source      string
	ProjectToken string
	Branch      string
	IssueID     string
	Description string
	FilePath    string
}

// Session is a contiguous block of activity derived from events.
type Session struct {
	ID           int64
	ProjectToken string
	Start        time.Time
	End          time.Time
	Branch       string
	IssueID      string
	Description  string
	Source       string
	EventCount   int
}

// GapThreshold is the inactivity duration that splits two sessions.
const GapThreshold = 25 * time.Minute

// MinDuration is the minimum session length; shorter sessions are dropped.
const MinDuration = 1 * time.Minute

// Reconstruct converts a chronologically sorted slice of events into sessions.
func Reconstruct(events []Event) []Session {
	// TODO: implement gap-based reconstruction
	return nil
}
