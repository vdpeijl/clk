package capture

import (
	"time"

	"github.com/vdpeijl/clk/internal/sessions"
)

// Command names exchanged over the daemon's Unix socket.
const (
	cmdEvent  = "event"
	cmdStatus = "status"
	cmdPing   = "ping"
)

// request is a single newline-delimited JSON message sent by a client to the
// daemon. Exactly one of the command payloads is meaningful per Cmd.
type request struct {
	Cmd   string `json:"cmd"`
	Event *Event `json:"event,omitempty"`
}

// response is the daemon's newline-delimited JSON reply to a request.
type response struct {
	OK     bool    `json:"ok"`
	Error  string  `json:"error,omitempty"`
	Status *Status `json:"status,omitempty"`
}

// Event is the wire representation of a captured activity signal. It mirrors
// sessions.Event but carries the timestamp as unix seconds so it serializes
// compactly and unambiguously across the socket.
type Event struct {
	Timestamp    int64  `json:"timestamp"`
	Type         string `json:"type"`
	Source       string `json:"source"`
	ProjectToken string `json:"project_token"`
	Branch       string `json:"branch,omitempty"`
	IssueID      string `json:"issue_id,omitempty"`
	Description  string `json:"description,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
}

// FromSessionEvent converts a domain event into its wire form, defaulting the
// timestamp to now when unset.
func FromSessionEvent(e sessions.Event) Event {
	ts := e.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	return Event{
		Timestamp:    ts.Unix(),
		Type:         e.Type,
		Source:       e.Source,
		ProjectToken: e.ProjectToken,
		Branch:       e.Branch,
		IssueID:      e.IssueID,
		Description:  e.Description,
		FilePath:     e.FilePath,
	}
}

// toSessionEvent converts a wire event back into a domain event.
func (e Event) toSessionEvent() sessions.Event {
	return sessions.Event{
		Timestamp:    time.Unix(e.Timestamp, 0),
		Type:         e.Type,
		Source:       e.Source,
		ProjectToken: e.ProjectToken,
		Branch:       e.Branch,
		IssueID:      e.IssueID,
		Description:  e.Description,
		FilePath:     e.FilePath,
	}
}

// Status is a snapshot of the daemon's runtime state, returned by the status
// command and surfaced by `clk status`.
type Status struct {
	PID           int   `json:"pid"`
	Buffered      int   `json:"buffered"`
	EventsTotal   int64 `json:"events_total"`
	UptimeSeconds int64 `json:"uptime_seconds"`
}
