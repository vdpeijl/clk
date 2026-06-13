package sessions

import (
	"testing"
	"time"
)

// base is a fixed reference time so fixtures stay deterministic.
var base = time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC)

// at returns base offset by the given number of minutes.
func at(min int) time.Time {
	return base.Add(time.Duration(min) * time.Minute)
}

func ev(min int, project, branch, source string) Event {
	return Event{
		Timestamp:    at(min),
		Source:       source,
		ProjectToken: project,
		Branch:       branch,
	}
}

func TestReconstruct(t *testing.T) {
	tests := []struct {
		name   string
		events []Event
		want   []Session
	}{
		{
			name:   "no events",
			events: nil,
			want:   nil,
		},
		{
			name: "single event padded to minimum duration",
			events: []Event{
				ev(0, "clk", "main", "claude_code"),
			},
			want: []Session{
				{
					ProjectToken: "clk",
					Start:        at(0),
					End:          at(0).Add(MinDuration),
					Branch:       "main",
					Source:       "claude_code",
					EventCount:   1,
				},
			},
		},
		{
			name: "events within gap form one session",
			events: []Event{
				ev(0, "clk", "main", "claude_code"),
				ev(20, "clk", "main", "claude_code"),
				ev(40, "clk", "main", "claude_code"),
			},
			want: []Session{
				{
					ProjectToken: "clk",
					Start:        at(0),
					End:          at(40),
					Branch:       "main",
					Source:       "claude_code",
					EventCount:   3,
				},
			},
		},
		{
			name: "gap larger than threshold splits sessions",
			events: []Event{
				ev(0, "clk", "main", "claude_code"),
				ev(10, "clk", "main", "claude_code"),
				// 26-minute gap > 25-minute threshold.
				ev(36, "clk", "main", "claude_code"),
				ev(46, "clk", "main", "claude_code"),
			},
			want: []Session{
				{
					ProjectToken: "clk",
					Start:        at(0),
					End:          at(10),
					Branch:       "main",
					Source:       "claude_code",
					EventCount:   2,
				},
				{
					ProjectToken: "clk",
					Start:        at(36),
					End:          at(46),
					Branch:       "main",
					Source:       "claude_code",
					EventCount:   2,
				},
			},
		},
		{
			name: "gap exactly at threshold stays one session",
			events: []Event{
				ev(0, "clk", "main", "claude_code"),
				ev(25, "clk", "main", "claude_code"),
			},
			want: []Session{
				{
					ProjectToken: "clk",
					Start:        at(0),
					End:          at(25),
					Branch:       "main",
					Source:       "claude_code",
					EventCount:   2,
				},
			},
		},
		{
			name: "different branches are separate sessions",
			events: []Event{
				ev(0, "clk", "main", "claude_code"),
				ev(5, "clk", "feature", "claude_code"),
				ev(10, "clk", "main", "claude_code"),
			},
			want: []Session{
				{
					ProjectToken: "clk",
					Start:        at(0),
					End:          at(10),
					Branch:       "main",
					Source:       "claude_code",
					EventCount:   2,
				},
				{
					ProjectToken: "clk",
					Start:        at(5),
					End:          at(5).Add(MinDuration),
					Branch:       "feature",
					Source:       "claude_code",
					EventCount:   1,
				},
			},
		},
		{
			name: "different projects are separate sessions",
			events: []Event{
				ev(0, "clk", "main", "claude_code"),
				ev(5, "other", "main", "claude_code"),
			},
			want: []Session{
				{
					ProjectToken: "clk",
					Start:        at(0),
					End:          at(0).Add(MinDuration),
					Branch:       "main",
					Source:       "claude_code",
					EventCount:   1,
				},
				{
					ProjectToken: "other",
					Start:        at(5),
					End:          at(5).Add(MinDuration),
					Branch:       "main",
					Source:       "claude_code",
					EventCount:   1,
				},
			},
		},
		{
			name: "primary source is the most frequent",
			events: []Event{
				ev(0, "clk", "main", "git"),
				ev(5, "clk", "main", "claude_code"),
				ev(10, "clk", "main", "claude_code"),
			},
			want: []Session{
				{
					ProjectToken: "clk",
					Start:        at(0),
					End:          at(10),
					Branch:       "main",
					Source:       "claude_code",
					EventCount:   3,
				},
			},
		},
		{
			name: "primary source tie broken by priority",
			events: []Event{
				ev(0, "clk", "main", "git"),
				ev(5, "clk", "main", "claude_code"),
			},
			want: []Session{
				{
					ProjectToken: "clk",
					Start:        at(0),
					End:          at(5),
					Branch:       "main",
					Source:       "claude_code",
					EventCount:   2,
				},
			},
		},
		{
			name: "unsorted input is ordered before reconstruction",
			events: []Event{
				ev(40, "clk", "main", "claude_code"),
				ev(0, "clk", "main", "claude_code"),
				ev(20, "clk", "main", "claude_code"),
			},
			want: []Session{
				{
					ProjectToken: "clk",
					Start:        at(0),
					End:          at(40),
					Branch:       "main",
					Source:       "claude_code",
					EventCount:   3,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Reconstruct(tt.events)
			assertSessions(t, got, tt.want)
		})
	}
}

func TestReconstructPrefersLatestContext(t *testing.T) {
	events := []Event{
		{Timestamp: at(0), Source: "claude_code", ProjectToken: "clk", Branch: "main", Description: "first", IssueID: "PROJ-1"},
		{Timestamp: at(5), Source: "claude_code", ProjectToken: "clk", Branch: "main", Description: "second", IssueID: ""},
	}
	got := Reconstruct(events)
	if len(got) != 1 {
		t.Fatalf("expected 1 session, got %d", len(got))
	}
	if got[0].Description != "second" {
		t.Errorf("description = %q, want %q", got[0].Description, "second")
	}
	if got[0].IssueID != "PROJ-1" {
		t.Errorf("issue id = %q, want %q", got[0].IssueID, "PROJ-1")
	}
}

func TestReconstructDoesNotMutateInput(t *testing.T) {
	events := []Event{
		ev(40, "clk", "main", "claude_code"),
		ev(0, "clk", "main", "claude_code"),
	}
	_ = Reconstruct(events)
	if !events[0].Timestamp.Equal(at(40)) {
		t.Errorf("input was reordered: events[0] = %v", events[0].Timestamp)
	}
}

func assertSessions(t *testing.T, got, want []Session) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d sessions, want %d:\n got=%+v\nwant=%+v", len(got), len(want), got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.ProjectToken != w.ProjectToken ||
			!g.Start.Equal(w.Start) ||
			!g.End.Equal(w.End) ||
			g.Branch != w.Branch ||
			g.Source != w.Source ||
			g.EventCount != w.EventCount {
			t.Errorf("session[%d] mismatch:\n got=%+v\nwant=%+v", i, g, w)
		}
	}
}
