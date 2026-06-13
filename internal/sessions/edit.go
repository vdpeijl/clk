package sessions

import (
	"sort"
	"strings"
	"time"
)

// Merge combines two or more sessions into a single contiguous session covering
// the full wall-clock span from the earliest start to the latest end. It is the
// review-time counterpart to reconstruction: where Reconstruct splits activity
// on gaps, Merge lets a human glue back together runs the gap heuristic wrongly
// separated.
//
// The result takes its project token, branch, issue, and source from the
// earliest session (falling back to the first non-empty value for branch and
// issue), unions the touched files in first-seen order, joins the distinct
// descriptions with "; ", and sums the event counts. Merge is pure and does not
// mutate its inputs. Merging zero sessions yields the zero Session; merging one
// returns it unchanged.
func Merge(ss ...Session) Session {
	switch len(ss) {
	case 0:
		return Session{}
	case 1:
		return ss[0]
	}

	ordered := make([]Session, len(ss))
	copy(ordered, ss)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Start.Before(ordered[j].Start)
	})

	out := Session{
		ProjectToken: ordered[0].ProjectToken,
		Start:        ordered[0].Start,
		End:          ordered[0].End,
		Branch:       ordered[0].Branch,
		IssueID:      ordered[0].IssueID,
		Source:       ordered[0].Source,
	}

	seenFile := make(map[string]bool)
	seenDesc := make(map[string]bool)
	var descs []string
	for _, s := range ordered {
		if s.End.After(out.End) {
			out.End = s.End
		}
		if out.Branch == "" {
			out.Branch = s.Branch
		}
		if out.IssueID == "" {
			out.IssueID = s.IssueID
		}
		out.EventCount += s.EventCount
		for _, f := range s.Files {
			if f != "" && !seenFile[f] {
				seenFile[f] = true
				out.Files = append(out.Files, f)
			}
		}
		if s.Description != "" && !seenDesc[s.Description] {
			seenDesc[s.Description] = true
			descs = append(descs, s.Description)
		}
	}
	out.Description = strings.Join(descs, "; ")
	return out
}

// SplitAt divides a session in two at the instant at, returning the earlier and
// later halves. The split succeeds only when at falls strictly inside the
// session's open interval (Start, End); otherwise ok is false and both returned
// sessions are zero, so the caller leaves the original untouched.
//
// Both halves inherit the parent's project token, branch, issue, description,
// source, and file list — the events themselves are not re-partitioned, since
// the review UI works with reconstructed sessions rather than raw events. The
// earlier half keeps the parent's event count; the later half starts at zero.
// SplitAt is pure and does not mutate its input.
func SplitAt(s Session, at time.Time) (early, late Session, ok bool) {
	if !at.After(s.Start) || !at.Before(s.End) {
		return Session{}, Session{}, false
	}

	files := append([]string(nil), s.Files...)
	early = Session{
		ProjectToken: s.ProjectToken,
		Start:        s.Start,
		End:          at,
		Branch:       s.Branch,
		IssueID:      s.IssueID,
		Description:  s.Description,
		Source:       s.Source,
		EventCount:   s.EventCount,
		Files:        files,
	}
	late = Session{
		ProjectToken: s.ProjectToken,
		Start:        at,
		End:          s.End,
		Branch:       s.Branch,
		IssueID:      s.IssueID,
		Description:  s.Description,
		Source:       s.Source,
		Files:        append([]string(nil), s.Files...),
	}
	return early, late, true
}
