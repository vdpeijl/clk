package pushplanner

import (
	"strconv"
	"testing"
	"time"

	"github.com/vdpeijl/clk/internal/sessions"
)

// target is a tiny helper for building a planner Target by key/hash.
func target(key, hash string) Target {
	return Target{Key: key, ContentHash: hash, Session: sessions.Session{ProjectToken: key}}
}

func link(key, entryID, hash string) PushLink {
	return PushLink{SessionKey: key, ClockifyEntryID: entryID, ContentHash: hash}
}

// planByKey indexes a plan for assertions, since warn items are appended after
// the targets and order is otherwise deterministic.
func planByKey(plan []PlanItem) map[string]PlanItem {
	m := make(map[string]PlanItem, len(plan))
	for _, item := range plan {
		m[item.Key] = item
	}
	return m
}

func TestPlanActions(t *testing.T) {
	tests := []struct {
		name    string
		targets []Target
		links   []PushLink
		want    map[string]Action
	}{
		{
			name:    "new session is created",
			targets: []Target{target("a", "h1")},
			links:   nil,
			want:    map[string]Action{"a": ActionCreate},
		},
		{
			name:    "pushed and unchanged is skipped",
			targets: []Target{target("a", "h1")},
			links:   []PushLink{link("a", "e1", "h1")},
			want:    map[string]Action{"a": ActionSkip},
		},
		{
			name:    "pushed and changed is updated",
			targets: []Target{target("a", "h2")},
			links:   []PushLink{link("a", "e1", "h1")},
			want:    map[string]Action{"a": ActionUpdate},
		},
		{
			name:    "pushed then dropped locally warns",
			targets: nil,
			links:   []PushLink{link("a", "e1", "h1")},
			want:    map[string]Action{"a": ActionWarn},
		},
		{
			name:    "daemon merge after push: survivor updates, collapsed peer warns",
			targets: []Target{target("a", "h2")}, // 'a' grew (new hash); 'b' merged into 'a' and is gone
			links:   []PushLink{link("a", "e1", "h1"), link("b", "e2", "hb")},
			want:    map[string]Action{"a": ActionUpdate, "b": ActionWarn},
		},
		{
			name: "mixed batch covers every action at once",
			targets: []Target{
				target("new", "hn"),
				target("same", "hs"),
				target("changed", "hc2"),
			},
			links: []PushLink{
				link("same", "e-same", "hs"),
				link("changed", "e-changed", "hc1"),
				link("gone", "e-gone", "hg"),
			},
			want: map[string]Action{
				"new":     ActionCreate,
				"same":    ActionSkip,
				"changed": ActionUpdate,
				"gone":    ActionWarn,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := Plan(tt.targets, tt.links)
			if len(plan) != len(tt.want) {
				t.Fatalf("plan has %d items, want %d: %+v", len(plan), len(tt.want), plan)
			}
			got := planByKey(plan)
			for key, wantAction := range tt.want {
				item, ok := got[key]
				if !ok {
					t.Fatalf("no plan item for key %q", key)
				}
				if item.Action != wantAction {
					t.Errorf("key %q: action = %v, want %v", key, item.Action, wantAction)
				}
			}
		})
	}
}

func TestPlanLinkAttachment(t *testing.T) {
	links := []PushLink{link("a", "e1", "h1"), link("gone", "e9", "hg")}
	plan := Plan([]Target{target("a", "h2")}, links)
	got := planByKey(plan)

	// Update carries the prior link so the caller can reuse the entry id.
	upd := got["a"]
	if upd.Link == nil || upd.Link.ClockifyEntryID != "e1" {
		t.Errorf("update link = %+v, want entry e1", upd.Link)
	}
	// Warn carries the orphaned link so the caller can name the stranded entry.
	warn := got["gone"]
	if warn.Link == nil || warn.Link.ClockifyEntryID != "e9" {
		t.Errorf("warn link = %+v, want entry e9", warn.Link)
	}
	if warn.Action != ActionWarn {
		t.Errorf("gone action = %v, want warn", warn.Action)
	}
}

func TestPlanCreateAndSkipHaveExpectedLinks(t *testing.T) {
	plan := Plan([]Target{target("a", "h1"), target("b", "h2")},
		[]PushLink{link("b", "e2", "h2")})
	got := planByKey(plan)

	if got["a"].Action != ActionCreate || got["a"].Link != nil {
		t.Errorf("create item = %+v, want create with nil link", got["a"])
	}
	if got["b"].Action != ActionSkip || got["b"].Link == nil {
		t.Errorf("skip item = %+v, want skip with link", got["b"])
	}
}

func TestSessionKey(t *testing.T) {
	start := time.Date(2026, 6, 13, 9, 30, 0, 0, time.UTC)
	s := sessions.Session{ProjectToken: "clk", Start: start}

	want := "clk|" + strconv.FormatInt(start.Unix(), 10)
	if got := SessionKey(s); got != want {
		t.Errorf("SessionKey = %q, want %q", got, want)
	}

	// Distinct starts yield distinct keys; identical inputs are stable.
	other := sessions.Session{ProjectToken: "clk", Start: start.Add(time.Minute)}
	if SessionKey(s) == SessionKey(other) {
		t.Errorf("expected distinct keys for distinct starts")
	}
	twin := sessions.Session{ProjectToken: "clk", Start: start}
	if SessionKey(s) != SessionKey(twin) {
		t.Errorf("SessionKey not stable for identical input")
	}
}
