// Package sessions reconstructs work sessions from raw events using a
// gap-based algorithm. It is pure: no I/O, no global state.
package sessions

import (
	"sort"
	"time"
)

// Event is a single captured dev activity signal.
type Event struct {
	ID           int64
	Timestamp    time.Time
	Type         string
	Source       string
	ProjectToken string
	Branch       string
	IssueID      string
	Description  string
	FilePath     string
}

// Session is a contiguous block of activity derived from events.
type Session struct {
	ID           int64
	ProjectToken string
	Start        time.Time
	End          time.Time
	Branch       string
	IssueID      string
	Description  string
	Source       string
	EventCount   int
	// Files lists the distinct file paths touched during the session, in
	// first-seen order. It feeds the {files} description placeholder.
	Files []string
}

// Duration returns the wall-clock length of the session.
func (s Session) Duration() time.Duration {
	return s.End.Sub(s.Start)
}

// GapThreshold is the inactivity duration that splits two sessions.
const GapThreshold = 25 * time.Minute

// MinDuration is the minimum session length; shorter sessions are extended to
// it so that single stray events do not become 0-minute noise entries.
const MinDuration = 1 * time.Minute

// sourcePriority ranks sources for primary-source tie-breaking. Lower wins.
// Sources not listed fall back to the lowest priority.
var sourcePriority = map[string]int{
	"claude_code": 0,
	"cursor":      1,
	"copilot":     2,
	"git":         3,
	"filewatch":   4,
}

// Reconstruct converts a slice of events into sessions. Events are grouped by
// project_token + branch, split into sessions wherever the gap between two
// consecutive events exceeds GapThreshold, padded up to MinDuration, and
// labelled with their primary source. The returned sessions are ordered by
// start time. Reconstruct is pure and does not mutate its input.
func Reconstruct(events []Event) []Session {
	if len(events) == 0 {
		return nil
	}

	// Group by project_token + branch, preserving first-seen order.
	type groupKey struct {
		project string
		branch  string
	}
	groups := make(map[groupKey][]Event)
	var order []groupKey
	for _, e := range events {
		k := groupKey{project: e.ProjectToken, branch: e.Branch}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], e)
	}

	var result []Session
	for _, k := range order {
		grouped := groups[k]
		sort.SliceStable(grouped, func(i, j int) bool {
			return grouped[i].Timestamp.Before(grouped[j].Timestamp)
		})

		// Split into runs separated by gaps larger than GapThreshold.
		start := 0
		for i := 1; i <= len(grouped); i++ {
			boundary := i == len(grouped)
			if !boundary && grouped[i].Timestamp.Sub(grouped[i-1].Timestamp) <= GapThreshold {
				continue
			}
			result = append(result, buildSession(grouped[start:i]))
			start = i
		}
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Start.Before(result[j].Start)
	})
	return result
}

// buildSession materializes a single session from a non-empty, time-ordered
// run of events that share a project_token and branch.
func buildSession(run []Event) Session {
	first := run[0]
	last := run[len(run)-1]

	end := last.Timestamp
	if end.Sub(first.Timestamp) < MinDuration {
		end = first.Timestamp.Add(MinDuration)
	}

	// Prefer the most recent non-empty description and issue id.
	description := latestNonEmpty(run, func(e Event) string { return e.Description })
	issueID := latestNonEmpty(run, func(e Event) string { return e.IssueID })

	return Session{
		ProjectToken: first.ProjectToken,
		Start:        first.Timestamp,
		End:          end,
		Branch:       first.Branch,
		IssueID:      issueID,
		Description:  description,
		Source:       primarySource(run),
		EventCount:   len(run),
		Files:        distinctFiles(run),
	}
}

// distinctFiles returns the file paths touched in run, de-duplicated and in
// first-seen order. Empty paths (e.g. heartbeats) are skipped.
func distinctFiles(run []Event) []string {
	seen := make(map[string]bool)
	var files []string
	for _, e := range run {
		if e.FilePath == "" || seen[e.FilePath] {
			continue
		}
		seen[e.FilePath] = true
		files = append(files, e.FilePath)
	}
	return files
}

// latestNonEmpty returns the last non-empty value produced by sel over run.
func latestNonEmpty(run []Event, sel func(Event) string) string {
	for i := len(run) - 1; i >= 0; i-- {
		if v := sel(run[i]); v != "" {
			return v
		}
	}
	return ""
}

// primarySource selects the dominant source of a run: the source with the most
// events, breaking ties by sourcePriority (then alphabetically for stability).
func primarySource(run []Event) string {
	counts := make(map[string]int)
	for _, e := range run {
		counts[e.Source]++
	}

	best := ""
	bestCount := -1
	for src, c := range counts {
		switch {
		case c > bestCount:
			best, bestCount = src, c
		case c == bestCount && betterSource(src, best):
			best = src
		}
	}
	return best
}

// betterSource reports whether a should win a tie over b.
func betterSource(a, b string) bool {
	pa, aok := sourcePriority[a]
	pb, bok := sourcePriority[b]
	switch {
	case aok && bok:
		return pa < pb
	case aok:
		return true
	case bok:
		return false
	default:
		return a < b
	}
}
