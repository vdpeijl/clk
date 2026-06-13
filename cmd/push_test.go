package cmd

import (
	"testing"
	"time"

	"github.com/vdpeijl/clk/internal/pushplanner"
	"github.com/vdpeijl/clk/internal/sessions"
)

func day(d, h, m int) time.Time {
	return time.Date(2026, 6, d, h, m, 0, 0, time.UTC)
}

func TestUnitsPerSession(t *testing.T) {
	p := &pusher{}
	ss := []sessions.Session{
		{ProjectToken: "clk", Start: day(13, 9, 0), End: day(13, 9, 30)},
		{ProjectToken: "clk", Start: day(13, 11, 0), End: day(13, 11, 45)},
	}

	units := p.units(ss, false)
	if len(units) != 2 {
		t.Fatalf("got %d units, want 2", len(units))
	}
	for i, u := range units {
		if u.key != pushplanner.SessionKey(ss[i]) {
			t.Errorf("unit[%d] key = %q, want %q", i, u.key, pushplanner.SessionKey(ss[i]))
		}
	}
}

func TestUnitsMergeCollapsesPerProjectDay(t *testing.T) {
	p := &pusher{}
	ss := []sessions.Session{
		// clk, day 13: two sessions → one merged unit.
		{ProjectToken: "clk", Start: day(13, 9, 0), End: day(13, 9, 30), Branch: "main", IssueID: "PROJ-1", Description: "morning", Files: []string{"a.go"}, EventCount: 3},
		{ProjectToken: "clk", Start: day(13, 14, 0), End: day(13, 14, 45), Description: "afternoon", Files: []string{"a.go", "b.go"}, EventCount: 2},
		// clk, day 14: separate day → separate unit.
		{ProjectToken: "clk", Start: day(14, 9, 0), End: day(14, 9, 15), Description: "next day"},
		// other project, day 13: separate project → separate unit.
		{ProjectToken: "other", Start: day(13, 10, 0), End: day(13, 10, 30), Description: "elsewhere"},
	}

	units := p.units(ss, true)
	if len(units) != 3 {
		t.Fatalf("got %d merged units, want 3", len(units))
	}

	byKey := make(map[string]unit)
	for _, u := range units {
		byKey[u.key] = u
	}

	merged, ok := byKey["clk|2026-06-13"]
	if !ok {
		t.Fatalf("missing merged unit for clk|2026-06-13: %+v", units)
	}
	m := merged.session
	// Span equals the summed durations (30m + 45m), not wall-clock.
	if got := m.Duration(); got != 75*time.Minute {
		t.Errorf("merged duration = %v, want 75m", got)
	}
	if !m.Start.Equal(day(13, 9, 0)) {
		t.Errorf("merged start = %v, want earliest 09:00", m.Start)
	}
	if m.Branch != "main" || m.IssueID != "PROJ-1" {
		t.Errorf("merged branch/issue = %q/%q, want main/PROJ-1", m.Branch, m.IssueID)
	}
	if m.Description != "morning; afternoon" {
		t.Errorf("merged description = %q, want %q", m.Description, "morning; afternoon")
	}
	if len(m.Files) != 2 || m.Files[0] != "a.go" || m.Files[1] != "b.go" {
		t.Errorf("merged files = %v, want [a.go b.go]", m.Files)
	}
	if m.EventCount != 5 {
		t.Errorf("merged event count = %d, want 5", m.EventCount)
	}

	if _, ok := byKey["clk|2026-06-14"]; !ok {
		t.Errorf("missing merged unit for clk|2026-06-14")
	}
	if _, ok := byKey["other|2026-06-13"]; !ok {
		t.Errorf("missing merged unit for other|2026-06-13")
	}
}
