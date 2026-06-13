package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vdpeijl/clk/internal/clockify"
	"github.com/vdpeijl/clk/internal/config"
	"github.com/vdpeijl/clk/internal/pushplanner"
	"github.com/vdpeijl/clk/internal/store"
	"github.com/vdpeijl/clk/internal/tui"
)

var reviewCmd = &cobra.Command{
	Use:   "review [today|yesterday|week|month]",
	Short: "Interactively review sessions before pushing",
	Long: `Opens an interactive terminal UI listing the period's reconstructed
sessions (default today) and lets you correct them before anything reaches
Clockify: merge sessions the gap heuristic split too eagerly, split one that
covers two tasks, edit a description, re-assign a session's Clockify
project/task, drop noise, and trigger the push from within the UI.

review orchestrates the same reconstruction, templating, planning, and Clockify
machinery as push; it adds the human-in-the-loop editing step in front of it.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReview(cmd, args)
	},
}

func runReview(cmd *cobra.Command, args []string) error {
	start, end, err := pushPeriod(args)
	if err != nil {
		return err
	}

	cfg, root, err := loadPushConfig()
	if err != nil {
		return err
	}
	client := clockify.New(cfg.APIKey, cfg.WorkspaceID)

	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	ss, err := reconstructPeriod(st, start, end)
	if err != nil {
		return err
	}

	r := &reviewer{
		pusher: newPusher(cfg, root),
		client: client,
		store:  st,
		ctx:    cmd.Context(),
	}

	return tui.Run(tui.New(ss, r.deps()))
}

// reviewer bundles the resolved context the review TUI orchestrates: rendering
// via the shared pusher, Clockify access, and the local push-link store. It
// adapts those into the tui.Deps callbacks so the UI itself stays free of
// planning and I/O logic.
type reviewer struct {
	pusher *pusher
	client *clockify.Client
	store  *store.Store
	ctx    context.Context
}

// deps wires the reviewer's capabilities into the TUI's injection points.
func (r *reviewer) deps() tui.Deps {
	return tui.Deps{
		Projects: r.projects,
		Tasks:    r.tasks,
		Push:     r.push,
	}
}

// projects lists the workspace's Clockify projects for the reassign picker.
func (r *reviewer) projects() ([]tui.Project, error) {
	ps, err := r.client.Projects(r.ctx)
	if err != nil {
		return nil, err
	}
	out := make([]tui.Project, len(ps))
	for i, p := range ps {
		out[i] = tui.Project{ID: p.ID, Name: p.Name}
	}
	return out, nil
}

// tasks lists a project's tasks for the second reassign step.
func (r *reviewer) tasks(projectID string) ([]tui.Task, error) {
	ts, err := r.client.Tasks(r.ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]tui.Task, len(ts))
	for i, t := range ts {
		out[i] = tui.Task{ID: t.ID, Name: t.Name}
	}
	return out, nil
}

// push renders the reviewed items into payloads, plans the push against the
// stored links, and executes it — reusing the exact idempotent pipeline behind
// `clk push`. A reassigned item is rendered against its chosen project/task; an
// item left on its token's mapping resolves the normal way.
func (r *reviewer) push(items []tui.PushItem) (tui.PushSummary, error) {
	entries := make(map[string]clockify.NewTimeEntry, len(items))
	var targets []pushplanner.Target
	var unmapped int

	for _, it := range items {
		entry, ok, err := r.entryFor(it)
		if err != nil {
			return tui.PushSummary{}, err
		}
		if !ok {
			unmapped++
			continue
		}
		key := pushplanner.SessionKey(it.Session)
		entries[key] = entry
		targets = append(targets, pushplanner.Target{
			Key:         key,
			ContentHash: contentHash(entry),
			Session:     it.Session,
		})
	}

	links, err := r.store.PushLinks()
	if err != nil {
		return tui.PushSummary{}, fmt.Errorf("read push links: %w", err)
	}

	plan := pushplanner.Plan(targets, toPlannerLinks(links))
	created, updated, skipped, warned, err := r.execPlan(entries, plan)
	if err != nil {
		return tui.PushSummary{}, err
	}
	return tui.PushSummary{
		Created:  created,
		Updated:  updated,
		Skipped:  skipped,
		Warned:   warned,
		Unmapped: unmapped,
	}, nil
}

// entryFor renders one reviewed item, honoring an explicit reassignment when
// present and otherwise resolving the session's token mapping.
func (r *reviewer) entryFor(it tui.PushItem) (clockify.NewTimeEntry, bool, error) {
	if it.ProjectID != "" {
		mapping := config.Mapping{Project: it.ProjectID, Task: it.TaskID}
		return r.pusher.entryWith(it.Session, mapping), true, nil
	}
	return r.pusher.entryFor(it.Session)
}

// execPlan carries out a review push: create or update entries in Clockify,
// record the resulting links, leave skips alone, and warn (never delete) about
// links whose session is no longer present. It mirrors the command-line
// execPlan but tallies counts for the UI instead of writing a report.
func (r *reviewer) execPlan(
	entries map[string]clockify.NewTimeEntry,
	plan []pushplanner.PlanItem,
) (created, updated, skipped, warned int, err error) {
	for _, item := range plan {
		switch item.Action {
		case pushplanner.ActionCreate:
			entry := entries[item.Key]
			te, err := r.client.CreateTimeEntry(r.ctx, entry)
			if err != nil {
				return created, updated, skipped, warned, err
			}
			if err := recordLink(r.store, item.Key, te.ID, contentHash(entry)); err != nil {
				return created, updated, skipped, warned, err
			}
			created++
		case pushplanner.ActionUpdate:
			entry := entries[item.Key]
			if _, err := r.client.UpdateTimeEntry(r.ctx, item.Link.ClockifyEntryID, entry); err != nil {
				return created, updated, skipped, warned, err
			}
			if err := recordLink(r.store, item.Key, item.Link.ClockifyEntryID, contentHash(entry)); err != nil {
				return created, updated, skipped, warned, err
			}
			updated++
		case pushplanner.ActionSkip:
			skipped++
		case pushplanner.ActionWarn:
			warned++
		}
	}
	return created, updated, skipped, warned, nil
}

func init() {
	rootCmd.AddCommand(reviewCmd)
}
