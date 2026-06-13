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

func TestReplaceAndQuerySessions(t *testing.T) {
	s := openTemp(t)

	day := time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC)
	window := []sessions.Session{
		{
			ProjectToken: "clk",
			Start:        day,
			End:          day.Add(30 * time.Minute),
			Branch:       "main",
			IssueID:      "PROJ-1",
			Description:  "work",
			Source:       "claude_code",
			EventCount:   4,
		},
	}
	winStart, winEnd := day.Add(-time.Hour), day.Add(24*time.Hour)
	if err := s.ReplaceSessionsBetween(winStart, winEnd, window); err != nil {
		t.Fatalf("ReplaceSessionsBetween: %v", err)
	}

	got, err := s.SessionsBetween(winStart, winEnd)
	if err != nil {
		t.Fatalf("SessionsBetween: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	g := got[0]
	if g.ProjectToken != "clk" || g.Branch != "main" || g.IssueID != "PROJ-1" ||
		g.Description != "work" || g.Source != "claude_code" || g.EventCount != 4 {
		t.Errorf("round-trip mismatch: %+v", g)
	}
	if !g.Start.Equal(day) || !g.End.Equal(day.Add(30*time.Minute)) {
		t.Errorf("times = %v..%v, want %v..%v", g.Start, g.End, day, day.Add(30*time.Minute))
	}

	// Replacing the window must overwrite, not append.
	replacement := []sessions.Session{
		{ProjectToken: "clk", Start: day, End: day.Add(time.Hour), Source: "git", EventCount: 9},
	}
	if err := s.ReplaceSessionsBetween(winStart, winEnd, replacement); err != nil {
		t.Fatalf("ReplaceSessionsBetween (replace): %v", err)
	}
	got, err = s.SessionsBetween(winStart, winEnd)
	if err != nil {
		t.Fatalf("SessionsBetween: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("after replace got %d sessions, want 1", len(got))
	}
	if got[0].EventCount != 9 || got[0].Source != "git" {
		t.Errorf("replacement not applied: %+v", got[0])
	}
}

func TestSessionsBetweenRangeFiltering(t *testing.T) {
	s := openTemp(t)

	day := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	in := []sessions.Session{
		{ProjectToken: "clk", Start: day.Add(-2 * time.Hour), End: day.Add(-time.Hour)},
		{ProjectToken: "clk", Start: day.Add(2 * time.Hour), End: day.Add(3 * time.Hour)},
		{ProjectToken: "clk", Start: day.Add(48 * time.Hour), End: day.Add(49 * time.Hour)},
	}
	wide := day.Add(-72 * time.Hour)
	if err := s.ReplaceSessionsBetween(wide, day.Add(72*time.Hour), in); err != nil {
		t.Fatalf("ReplaceSessionsBetween: %v", err)
	}

	got, err := s.SessionsBetween(day, day.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("SessionsBetween: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions in window, want 1", len(got))
	}
	if !got[0].Start.Equal(day.Add(2 * time.Hour)) {
		t.Errorf("wrong session selected: %+v", got[0])
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
