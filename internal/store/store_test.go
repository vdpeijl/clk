package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/vdpeijl/clk/internal/sessions"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInsertAndQueryEvents(t *testing.T) {
	s := openTemp(t)

	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	in := sessions.Event{
		Timestamp:    now,
		Type:         "tool_use",
		Source:       "claude_code",
		ProjectToken: "clk",
		Description:  "Edit sessions.go",
		FilePath:     "internal/sessions/sessions.go",
		Branch:       "main",
		IssueID:      "PROJ-123",
	}
	id, err := s.InsertEvent(in)
	if err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	if id == 0 {
		t.Fatalf("expected non-zero id")
	}

	got, err := s.EventsBetween(now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("EventsBetween: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	e := got[0]
	if e.Source != in.Source || e.ProjectToken != in.ProjectToken ||
		e.Branch != in.Branch || e.IssueID != in.IssueID ||
		e.Description != in.Description || e.FilePath != in.FilePath {
		t.Errorf("round-trip mismatch: got %+v want %+v", e, in)
	}
	if !e.Timestamp.Equal(now) {
		t.Errorf("timestamp = %v, want %v", e.Timestamp, now)
	}
}

func TestEventsBetweenRangeFiltering(t *testing.T) {
	s := openTemp(t)

	day := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	mk := func(hour int) sessions.Event {
		return sessions.Event{
			Timestamp:    day.Add(time.Duration(hour) * time.Hour),
			Type:         "tool_use",
			Source:       "claude_code",
			ProjectToken: "clk",
		}
	}
	for _, h := range []int{1, 10, 23} {
		if _, err := s.InsertEvent(mk(h)); err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
	}
	// Next day event should be excluded.
	if _, err := s.InsertEvent(mk(25)); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	got, err := s.EventsBetween(day, day.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("EventsBetween: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	// Confirm chronological ordering.
	for i := 1; i < len(got); i++ {
		if got[i].Timestamp.Before(got[i-1].Timestamp) {
			t.Errorf("events not ordered chronologically: %v", got)
		}
	}
}
