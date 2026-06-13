// Package pushplanner computes a push plan from local sessions and prior push
// links. It is pure: no I/O, no HTTP calls, no knowledge of config, rounding,
// or templates — callers render each session into a payload, hash it, and hand
// the planner the key/hash pairs.
package pushplanner

import (
	"strconv"

	"github.com/vdpeijl/clk/internal/sessions"
)

// Action describes what should happen for one push target on the next push.
type Action int

const (
	ActionCreate Action = iota // no prior link → create a new Clockify entry
	ActionUpdate               // linked but the payload changed → update in place
	ActionSkip                 // linked and unchanged → do nothing
	ActionWarn                 // linked but the session is gone locally → warn, never delete
)

// String renders an Action for log/CLI output.
func (a Action) String() string {
	switch a {
	case ActionCreate:
		return "create"
	case ActionUpdate:
		return "update"
	case ActionSkip:
		return "skip"
	case ActionWarn:
		return "warn"
	default:
		return "unknown"
	}
}

// PushLink records a prior push: a stable session key → Clockify entry, plus a
// content hash of the payload that was pushed. The hash drives the
// create-vs-update-vs-skip decision.
type PushLink struct {
	SessionKey      string
	ClockifyEntryID string
	ContentHash     string
}

// Target is one plannable push unit: a stable identity Key, the ContentHash of
// the rendered payload, and the representative Session the caller will render
// and execute against. For per-session pushes the Session is the real session;
// for `--merge` it is a synthetic per-project-per-day session.
type Target struct {
	Key         string
	ContentHash string
	Session     sessions.Session
}

// PlanItem is one entry in the push plan. Session is the representative session
// (zero for ActionWarn, whose session no longer exists locally). Link is
// non-nil for Update, Skip, and Warn.
type PlanItem struct {
	Key     string
	Action  Action
	Session sessions.Session
	Link    *PushLink
}

// SessionKey returns the stable per-session identity used for push idempotency:
// the project token plus the session start. It is independent of the volatile
// autoincrement row id, so it is stable across daemon re-materialization and
// matches the on-the-fly reconstruction `clk push` performs from raw events.
func SessionKey(s sessions.Session) string {
	return s.ProjectToken + "|" + strconv.FormatInt(s.Start.Unix(), 10)
}

// Plan computes the push plan for the given targets and existing push links.
//
// Each target is classified against its matching link by Key:
//   - no link            → ActionCreate
//   - link, hashes equal → ActionSkip
//   - link, hashes differ→ ActionUpdate
//
// Any link whose key is absent from the targets describes an entry that was
// pushed and then dropped locally (e.g. its events aged out, or a daemon merge
// collapsed it into an adjacent session): it yields ActionWarn and is never
// auto-deleted. Plan is pure and does not mutate its inputs.
func Plan(targets []Target, links []PushLink) []PlanItem {
	byKey := make(map[string]PushLink, len(links))
	for _, l := range links {
		byKey[l.SessionKey] = l
	}

	seen := make(map[string]bool, len(targets))
	var plan []PlanItem
	for _, t := range targets {
		seen[t.Key] = true
		link, ok := byKey[t.Key]
		switch {
		case !ok:
			plan = append(plan, PlanItem{Key: t.Key, Action: ActionCreate, Session: t.Session})
		case link.ContentHash == t.ContentHash:
			l := link
			plan = append(plan, PlanItem{Key: t.Key, Action: ActionSkip, Session: t.Session, Link: &l})
		default:
			l := link
			plan = append(plan, PlanItem{Key: t.Key, Action: ActionUpdate, Session: t.Session, Link: &l})
		}
	}

	for _, l := range links {
		if seen[l.SessionKey] {
			continue
		}
		ll := l
		plan = append(plan, PlanItem{Key: l.SessionKey, Action: ActionWarn, Link: &ll})
	}

	return plan
}
