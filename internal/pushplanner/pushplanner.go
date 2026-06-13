// Package pushplanner computes a push plan from local sessions and prior push
// links. It is pure: no I/O, no HTTP calls.
package pushplanner

import "github.com/vdpeijl/clk/internal/sessions"

// Action describes what should happen to a session on the next push.
type Action int

const (
	ActionCreate Action = iota
	ActionUpdate
	ActionSkip
	ActionWarn // pushed and then locally dropped
)

// PushLink records a prior push: session → Clockify entry, plus a content hash.
type PushLink struct {
	SessionID       int64
	ClockifyEntryID string
	ContentHash     string
}

// PlanItem is one entry in the push plan.
type PlanItem struct {
	Session sessions.Session
	Action  Action
	Link    *PushLink // non-nil for Update/Skip/Warn
}

// Plan returns the push plan for the given sessions and existing push links.
func Plan(ss []sessions.Session, links []PushLink) []PlanItem {
	// TODO: implement idempotency logic
	return nil
}
