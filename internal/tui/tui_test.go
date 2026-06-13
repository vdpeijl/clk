package tui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vdpeijl/clk/internal/sessions"
)

var base = time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC)

func at(min int) time.Time { return base.Add(time.Duration(min) * time.Minute) }

func session(startMin, endMin int, desc string) sessions.Session {
	return sessions.Session{
		ProjectToken: "clk",
		Start:        at(startMin),
		End:          at(endMin),
		Description:  desc,
	}
}

// key builds a KeyMsg from a single rune or a named special key.
func key(s string) tea.KeyMsg {
	switch s {
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// send drives the model through a sequence of keys and returns the result.
func send(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(Model)
	}
	return m
}

func newModel(deps Deps, ss ...sessions.Session) Model {
	return New(ss, deps)
}

func TestMergeSelected(t *testing.T) {
	m := newModel(Deps{},
		session(0, 30, "first"),
		session(60, 90, "second"),
		session(120, 150, "third"),
	)

	// Select rows 0 and 1, then merge.
	m = send(m, "space", "down", "space", "m")

	if len(m.items) != 2 {
		t.Fatalf("got %d items, want 2 after merge", len(m.items))
	}
	merged := m.items[0].session
	if !merged.Start.Equal(at(0)) || !merged.End.Equal(at(90)) {
		t.Errorf("merged span = [%v,%v], want [0,90]", merged.Start, merged.End)
	}
	if merged.Description != "first; second" {
		t.Errorf("merged description = %q", merged.Description)
	}
	if m.items[1].session.Description != "third" {
		t.Errorf("unmerged row = %q, want third", m.items[1].session.Description)
	}
}

func TestMergeNeedsTwoSelections(t *testing.T) {
	m := newModel(Deps{}, session(0, 30, "first"), session(60, 90, "second"))
	m = send(m, "space", "m") // only one selected

	if len(m.items) != 2 {
		t.Fatalf("items changed on a no-op merge: %d", len(m.items))
	}
	if m.status == "" {
		t.Error("expected a status hint when merging fewer than two")
	}
}

func TestSplitAtMidpoint(t *testing.T) {
	m := newModel(Deps{}, session(0, 60, "work"))

	m = send(m, "s") // enter split, defaults to midpoint (30m)
	if m.mode != modeSplit {
		t.Fatalf("mode = %v, want split", m.mode)
	}
	if m.splitOffset != 30*time.Minute {
		t.Errorf("default split offset = %v, want 30m", m.splitOffset)
	}

	m = send(m, "enter")
	if len(m.items) != 2 {
		t.Fatalf("got %d items, want 2 after split", len(m.items))
	}
	if !m.items[0].session.End.Equal(at(30)) || !m.items[1].session.Start.Equal(at(30)) {
		t.Errorf("split boundary wrong: %v / %v", m.items[0].session.End, m.items[1].session.Start)
	}
}

func TestSplitOffsetClampsInsideBounds(t *testing.T) {
	m := newModel(Deps{}, session(0, 4, "short")) // 4-minute session
	m = send(m, "s")

	// Hammer left past the lower bound; offset must stay >= 1 minute.
	m = send(m, "left", "left", "left", "left", "left")
	if m.splitOffset < time.Minute {
		t.Errorf("offset = %v, want >= 1m", m.splitOffset)
	}
	// Hammer right past the upper bound; offset must stay <= dur-1m.
	m = send(m, "right", "right", "right", "right", "right")
	if m.splitOffset > 3*time.Minute {
		t.Errorf("offset = %v, want <= 3m", m.splitOffset)
	}
}

func TestEditDescription(t *testing.T) {
	m := newModel(Deps{}, session(0, 30, "old"))

	m = send(m, "e")
	if m.mode != modeEdit {
		t.Fatalf("mode = %v, want edit", m.mode)
	}
	// Clear the field, type a new value, save.
	m.input.SetValue("")
	m = send(m, "n", "e", "w", "enter")

	if got := m.items[0].session.Description; got != "new" {
		t.Errorf("description = %q, want new", got)
	}
	if m.mode != modeList {
		t.Errorf("mode = %v, want list after save", m.mode)
	}
}

func TestEditCancelKeepsOriginal(t *testing.T) {
	m := newModel(Deps{}, session(0, 30, "keep"))
	m = send(m, "e")
	m.input.SetValue("discard")
	m = send(m, "esc")

	if m.items[0].session.Description != "keep" {
		t.Errorf("description = %q, want keep after cancel", m.items[0].session.Description)
	}
}

func TestDropExcludesFromPush(t *testing.T) {
	m := newModel(Deps{}, session(0, 30, "noise"), session(60, 90, "real"))

	m = send(m, "d") // drop row 0
	if !m.items[0].dropped {
		t.Fatal("row 0 not marked dropped")
	}
	items := m.pushItems()
	if len(items) != 1 || items[0].Session.Description != "real" {
		t.Errorf("pushItems = %+v, want only the non-dropped session", items)
	}

	m = send(m, "d") // toggle back
	if m.items[0].dropped {
		t.Error("drop did not toggle off")
	}
}

func TestReassignProjectAndTask(t *testing.T) {
	deps := Deps{
		Projects: func() ([]Project, error) {
			return []Project{{ID: "p1", Name: "Alpha"}, {ID: "p2", Name: "Beta"}}, nil
		},
		Tasks: func(projectID string) ([]Task, error) {
			if projectID != "p2" {
				t.Errorf("Tasks called with %q, want p2", projectID)
			}
			return []Task{{ID: "t1", Name: "Design"}}, nil
		},
	}
	m := newModel(deps, session(0, 30, "work"))

	m = send(m, "r") // open project picker
	if m.mode != modePickProject {
		t.Fatalf("mode = %v, want pick project", m.mode)
	}
	// Filter to Beta (typed key by key so the picker re-ranks) and select it.
	m = send(m, "B", "e", "t", "a", "enter")
	if m.mode != modePickTask {
		t.Fatalf("mode = %v, want pick task", m.mode)
	}

	// Pick the task (row 1; row 0 is "no task").
	m = send(m, "down", "enter")
	if m.mode != modeList {
		t.Fatalf("mode = %v, want list after reassign", m.mode)
	}
	it := m.items[0]
	if it.projectID != "p2" || it.taskID != "t1" || it.projectName != "Beta" {
		t.Errorf("reassign = %+v, want project p2/Beta task t1", it)
	}

	items := m.pushItems()
	if items[0].ProjectID != "p2" || items[0].TaskID != "t1" {
		t.Errorf("pushItem override = %+v", items[0])
	}
}

func TestReassignNoTask(t *testing.T) {
	deps := Deps{
		Projects: func() ([]Project, error) { return []Project{{ID: "p1", Name: "Alpha"}}, nil },
		Tasks:    func(string) ([]Task, error) { return []Task{{ID: "t1", Name: "Design"}}, nil },
	}
	m := newModel(deps, session(0, 30, "work"))
	m = send(m, "r", "enter") // pick first (only) project
	m = send(m, "enter")      // accept the "(no task)" row at cursor 0

	if m.items[0].taskID != "" {
		t.Errorf("taskID = %q, want empty (no task)", m.items[0].taskID)
	}
	if m.items[0].projectID != "p1" {
		t.Errorf("projectID = %q, want p1", m.items[0].projectID)
	}
}

func TestPushConfirmFlow(t *testing.T) {
	var got []PushItem
	deps := Deps{
		Push: func(items []PushItem) (PushSummary, error) {
			got = items
			return PushSummary{Created: 2}, nil
		},
	}
	m := newModel(deps, session(0, 30, "a"), session(60, 90, "b"))

	m = send(m, "p") // confirm prompt
	if m.mode != modeConfirm {
		t.Fatalf("mode = %v, want confirm", m.mode)
	}
	m = send(m, "y") // confirm
	if len(got) != 2 {
		t.Fatalf("push got %d items, want 2", len(got))
	}
	if m.status == "" {
		t.Error("expected a status summary after push")
	}
}

func TestPushConfirmCancel(t *testing.T) {
	called := false
	deps := Deps{Push: func(items []PushItem) (PushSummary, error) { called = true; return PushSummary{}, nil }}
	m := newModel(deps, session(0, 30, "a"))

	m = send(m, "p", "n") // open confirm, then decline
	if called {
		t.Error("push ran despite cancel")
	}
	if m.mode != modeList {
		t.Errorf("mode = %v, want list after cancel", m.mode)
	}
}

func TestPushSurfacesError(t *testing.T) {
	deps := Deps{Push: func([]PushItem) (PushSummary, error) { return PushSummary{}, errors.New("boom") }}
	m := newModel(deps, session(0, 30, "a"))
	m = send(m, "p", "y")
	if m.err == nil {
		t.Error("expected error to be recorded")
	}
}

func TestPushNothingWhenAllDropped(t *testing.T) {
	called := false
	deps := Deps{Push: func([]PushItem) (PushSummary, error) { called = true; return PushSummary{}, nil }}
	m := newModel(deps, session(0, 30, "a"))
	m = send(m, "d", "p") // drop the only session, then try to push

	if m.mode == modeConfirm {
		t.Error("entered confirm with nothing to push")
	}
	if called {
		t.Error("push ran with nothing to push")
	}
}

func TestQuit(t *testing.T) {
	m := newModel(Deps{}, session(0, 30, "a"))
	_, cmd := m.Update(key("q"))
	if cmd == nil {
		t.Fatal("q produced no command, want tea.Quit")
	}
	if msg := cmd(); msg == nil {
		t.Error("quit command produced no message")
	}
}

func TestPushSummaryString(t *testing.T) {
	s := PushSummary{Created: 1, Updated: 2, Skipped: 3, Warned: 4, Unmapped: 5}
	want := "1 created, 2 updated, 3 skipped, 4 warned, 5 unmapped"
	if s.String() != want {
		t.Errorf("String() = %q, want %q", s.String(), want)
	}
}

func TestViewRendersWithoutPanic(t *testing.T) {
	deps := Deps{
		Projects: func() ([]Project, error) { return []Project{{ID: "p1", Name: "Alpha"}}, nil },
		Tasks:    func(string) ([]Task, error) { return []Task{{ID: "t1", Name: "Design"}}, nil },
	}
	m := newModel(deps, session(0, 60, "work"))
	for _, mode := range []func(Model) Model{
		func(m Model) Model { return m },                         // list
		func(m Model) Model { return send(m, "e") },              // edit
		func(m Model) Model { return send(m, "s") },              // split
		func(m Model) Model { return send(m, "r") },              // pick project
		func(m Model) Model { return send(m, "r", "enter") },     // pick task
		func(m Model) Model { return send(m, "p") },              // confirm
	} {
		if mode(m).View() == "" {
			t.Error("a mode rendered an empty view")
		}
	}
}
