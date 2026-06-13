package sessions

import (
	"testing"
	"time"
)

func sess(startMin, endMin int, opts ...func(*Session)) Session {
	s := Session{
		ProjectToken: "clk",
		Start:        at(startMin),
		End:          at(endMin),
	}
	for _, o := range opts {
		o(&s)
	}
	return s
}

func TestMerge(t *testing.T) {
	t.Run("zero sessions yields zero session", func(t *testing.T) {
		if got := Merge(); !got.Start.IsZero() || !got.End.IsZero() || got.Files != nil {
			t.Errorf("Merge() = %+v, want zero", got)
		}
	})

	t.Run("single session returned unchanged", func(t *testing.T) {
		s := sess(0, 30, func(s *Session) { s.Description = "solo" })
		if got := Merge(s); got.Description != "solo" || !got.Start.Equal(at(0)) || !got.End.Equal(at(30)) {
			t.Errorf("Merge(single) = %+v, want unchanged", got)
		}
	})

	t.Run("spans earliest start to latest end", func(t *testing.T) {
		a := sess(0, 30, func(s *Session) {
			s.Description = "morning"
			s.Branch = "main"
			s.Files = []string{"a.go"}
			s.EventCount = 3
			s.Source = "claude_code"
		})
		b := sess(60, 90, func(s *Session) {
			s.Description = "later"
			s.IssueID = "PROJ-1"
			s.Files = []string{"a.go", "b.go"}
			s.EventCount = 2
		})

		// Pass out of order to prove Merge sorts by start.
		got := Merge(b, a)

		if !got.Start.Equal(at(0)) {
			t.Errorf("Start = %v, want %v", got.Start, at(0))
		}
		if !got.End.Equal(at(90)) {
			t.Errorf("End = %v, want %v", got.End, at(90))
		}
		if got.Branch != "main" {
			t.Errorf("Branch = %q, want main", got.Branch)
		}
		if got.IssueID != "PROJ-1" {
			t.Errorf("IssueID = %q, want PROJ-1 (first non-empty)", got.IssueID)
		}
		if got.Source != "claude_code" {
			t.Errorf("Source = %q, want claude_code (earliest)", got.Source)
		}
		if got.EventCount != 5 {
			t.Errorf("EventCount = %d, want 5", got.EventCount)
		}
		if want := []string{"a.go", "b.go"}; !equalStrings(got.Files, want) {
			t.Errorf("Files = %v, want %v (union, first-seen)", got.Files, want)
		}
		if got.Description != "morning; later" {
			t.Errorf("Description = %q, want %q", got.Description, "morning; later")
		}
	})

	t.Run("does not mutate inputs", func(t *testing.T) {
		a := sess(0, 30, func(s *Session) { s.Files = []string{"a.go"} })
		b := sess(60, 90)
		_ = Merge(a, b)
		if !a.End.Equal(at(30)) || len(a.Files) != 1 {
			t.Errorf("input mutated: %+v", a)
		}
	})

	t.Run("deduplicates identical descriptions", func(t *testing.T) {
		a := sess(0, 30, func(s *Session) { s.Description = "same" })
		b := sess(60, 90, func(s *Session) { s.Description = "same" })
		if got := Merge(a, b); got.Description != "same" {
			t.Errorf("Description = %q, want %q", got.Description, "same")
		}
	})
}

func TestSplitAt(t *testing.T) {
	parent := sess(0, 60, func(s *Session) {
		s.Description = "work"
		s.Branch = "main"
		s.IssueID = "PROJ-1"
		s.Source = "git"
		s.EventCount = 4
		s.Files = []string{"a.go", "b.go"}
	})

	t.Run("splits at an interior point", func(t *testing.T) {
		early, late, ok := SplitAt(parent, at(25))
		if !ok {
			t.Fatal("ok = false, want true for interior split")
		}
		if !early.Start.Equal(at(0)) || !early.End.Equal(at(25)) {
			t.Errorf("early = [%v,%v], want [0,25]", early.Start, early.End)
		}
		if !late.Start.Equal(at(25)) || !late.End.Equal(at(60)) {
			t.Errorf("late = [%v,%v], want [25,60]", late.Start, late.End)
		}
		// Metadata is inherited by both halves.
		for _, h := range []Session{early, late} {
			if h.Description != "work" || h.Branch != "main" || h.IssueID != "PROJ-1" || h.Source != "git" {
				t.Errorf("half lost metadata: %+v", h)
			}
			if !equalStrings(h.Files, []string{"a.go", "b.go"}) {
				t.Errorf("half files = %v, want both", h.Files)
			}
		}
		if early.EventCount != 4 || late.EventCount != 0 {
			t.Errorf("event counts = %d/%d, want 4/0", early.EventCount, late.EventCount)
		}
	})

	t.Run("rejects split at or outside the bounds", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			at   time.Time
		}{
			{"at start", at(0)},
			{"at end", at(60)},
			{"before start", at(-10)},
			{"after end", at(120)},
		} {
			if _, _, ok := SplitAt(parent, tc.at); ok {
				t.Errorf("%s: ok = true, want false", tc.name)
			}
		}
	})

	t.Run("split halves carry independent file slices", func(t *testing.T) {
		early, late, _ := SplitAt(parent, at(30))
		early.Files[0] = "mutated"
		if late.Files[0] == "mutated" {
			t.Error("late half shares early half's file backing array")
		}
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
