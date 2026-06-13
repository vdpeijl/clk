package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vdpeijl/clk/internal/clockify"
	"github.com/vdpeijl/clk/internal/config"
	"github.com/vdpeijl/clk/internal/gitctx"
	"github.com/vdpeijl/clk/internal/pushplanner"
	"github.com/vdpeijl/clk/internal/rounding"
	"github.com/vdpeijl/clk/internal/sessions"
	"github.com/vdpeijl/clk/internal/store"
	"github.com/vdpeijl/clk/internal/template"
)

var pushMerge bool

var pushCmd = &cobra.Command{
	Use:   "push [today|yesterday|week|month]",
	Short: "Push reviewed sessions to Clockify",
	Long: `Reconstructs sessions for the given period (default today) and registers
them in Clockify as time entries. The push is idempotent: a new session is
created, a previously pushed session whose payload changed is updated, an
unchanged one is skipped, and a session that was pushed but no longer exists
locally is warned about — never deleted (use clk unpush for that).

By default each session becomes its own entry, preserving its real start. With
--merge the day's sessions for a project are collapsed into a single entry whose
duration is the rounded total.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPush(cmd, args)
	},
}

var unpushCmd = &cobra.Command{
	Use:   "unpush [today|yesterday|week|month]",
	Short: "Delete previously pushed entries from Clockify",
	Long: `Explicitly removes the Clockify entries clk created for sessions in the
given period (default today) and forgets their push links, so a later push
treats those sessions as new. This is the only command that deletes from
Clockify; push never does.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUnpush(cmd, args)
	},
}

var logCmd = &cobra.Command{
	Use:   "log <duration> <description>",
	Short: "Create a manual Clockify time entry",
	Long: `Creates a one-off Clockify time entry for the current project ending now and
starting <duration> ago. Duration accepts Go syntax such as 45m or 1h30m. The
description is used verbatim (no template expansion).`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLog(cmd, args)
	},
}

// pusher bundles the resolved push context shared across a single push run.
type pusher struct {
	root     string
	template string
	billable bool
	mode     rounding.Mode
}

func runPush(cmd *cobra.Command, args []string) error {
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

	p := newPusher(cfg, root)
	units := p.units(ss, pushMerge)

	// Render each unit into a payload, skipping (with a notice) any whose
	// project token is not yet mapped to a Clockify project.
	out := cmd.OutOrStdout()
	entries := make(map[string]clockify.NewTimeEntry, len(units))
	var targets []pushplanner.Target
	for _, u := range units {
		entry, ok, err := p.entryFor(u.session)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintf(out, "skip %s — no Clockify mapping (run `clk link %s`)\n", u.key, u.session.ProjectToken)
			continue
		}
		entries[u.key] = entry
		targets = append(targets, pushplanner.Target{
			Key:         u.key,
			ContentHash: contentHash(entry),
			Session:     u.session,
		})
	}

	links, err := st.PushLinks()
	if err != nil {
		return fmt.Errorf("read push links: %w", err)
	}

	plan := pushplanner.Plan(targets, toPlannerLinks(links))
	return execPlan(cmd.Context(), out, client, st, entries, plan)
}

// execPlan carries out a push plan: create/update entries in Clockify, record
// the resulting links, leave skips alone, and warn about locally dropped
// entries without deleting them.
func execPlan(
	ctx context.Context,
	out io.Writer,
	client *clockify.Client,
	st *store.Store,
	entries map[string]clockify.NewTimeEntry,
	plan []pushplanner.PlanItem,
) error {
	var created, updated, skipped, warned int
	for _, item := range plan {
		switch item.Action {
		case pushplanner.ActionCreate:
			entry := entries[item.Key]
			te, err := client.CreateTimeEntry(ctx, entry)
			if err != nil {
				return err
			}
			if err := recordLink(st, item.Key, te.ID, contentHash(entry)); err != nil {
				return err
			}
			created++
			fmt.Fprintf(out, "create %s — %s\n", item.Key, entry.Description)
		case pushplanner.ActionUpdate:
			entry := entries[item.Key]
			if _, err := client.UpdateTimeEntry(ctx, item.Link.ClockifyEntryID, entry); err != nil {
				return err
			}
			if err := recordLink(st, item.Key, item.Link.ClockifyEntryID, contentHash(entry)); err != nil {
				return err
			}
			updated++
			fmt.Fprintf(out, "update %s — %s\n", item.Key, entry.Description)
		case pushplanner.ActionSkip:
			skipped++
		case pushplanner.ActionWarn:
			warned++
			fmt.Fprintf(out, "warn   %s — pushed entry %s no longer exists locally; run `clk unpush` to remove it\n",
				item.Key, item.Link.ClockifyEntryID)
		}
	}
	fmt.Fprintf(out, "pushed: %d created, %d updated, %d skipped, %d warned\n", created, updated, skipped, warned)
	return nil
}

func runUnpush(cmd *cobra.Command, args []string) error {
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

	p := newPusher(cfg, root)
	candidates := make(map[string]bool)
	for _, u := range p.units(ss, false) {
		candidates[u.key] = true
	}
	for _, u := range p.units(ss, true) {
		candidates[u.key] = true
	}

	links, err := st.PushLinks()
	if err != nil {
		return fmt.Errorf("read push links: %w", err)
	}

	out := cmd.OutOrStdout()
	var removed int
	for _, l := range links {
		if !candidates[l.SessionKey] {
			continue
		}
		if err := client.DeleteTimeEntry(cmd.Context(), l.ClockifyEntryID); err != nil {
			return err
		}
		if err := st.DeletePushLink(l.SessionKey); err != nil {
			return err
		}
		removed++
		fmt.Fprintf(out, "unpushed %s (entry %s)\n", l.SessionKey, l.ClockifyEntryID)
	}
	fmt.Fprintf(out, "unpushed %d entr%s\n", removed, plural(removed))
	return nil
}

func runLog(cmd *cobra.Command, args []string) error {
	dur, err := time.ParseDuration(args[0])
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", args[0], err)
	}
	if dur <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	description := strings.Join(args[1:], " ")

	cfg, root, err := loadPushConfig()
	if err != nil {
		return err
	}

	token := gitctx.Detect(root).ProjectToken
	mapping, ok, err := config.ResolveMapping(root, token)
	if err != nil {
		return err
	}
	if !ok || mapping.Project == "" {
		return fmt.Errorf("no Clockify mapping for %q — run `clk link` first", token)
	}

	end := time.Now()
	entry := clockify.NewTimeEntry{
		Start:       end.Add(-dur),
		End:         end,
		Description: description,
		ProjectID:   mapping.Project,
		TaskID:      mapping.Task,
		Billable:    cfg.Billable,
	}

	client := clockify.New(cfg.APIKey, cfg.WorkspaceID)
	te, err := client.CreateTimeEntry(cmd.Context(), entry)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "logged %s — %s (entry %s)\n", dur, description, te.ID)
	return nil
}

// unit is one plannable push: a stable key and the (possibly synthetic) session
// rendered into a Clockify entry.
type unit struct {
	key     string
	session sessions.Session
}

func newPusher(cfg *config.Config, root string) *pusher {
	tmpl := cfg.Template
	if strings.TrimSpace(tmpl) == "" {
		tmpl = config.DefaultTemplate
	}
	return &pusher{
		root:     root,
		template: tmpl,
		billable: cfg.Billable,
		mode:     rounding.Parse(cfg.PushRound),
	}
}

// units turns reconstructed sessions into plannable push units. Per session by
// default; with merge, the day's sessions for a project are collapsed into one
// synthetic session whose duration is the sum of its parts.
func (p *pusher) units(ss []sessions.Session, merge bool) []unit {
	if !merge {
		units := make([]unit, 0, len(ss))
		for _, s := range ss {
			units = append(units, unit{key: pushplanner.SessionKey(s), session: s})
		}
		return units
	}

	type dayKey struct {
		token string
		day   string
	}
	groups := make(map[dayKey][]sessions.Session)
	var order []dayKey
	for _, s := range ss {
		k := dayKey{token: s.ProjectToken, day: s.Start.Format("2006-01-02")}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], s)
	}

	units := make([]unit, 0, len(order))
	for _, k := range order {
		merged := mergeSessions(groups[k])
		units = append(units, unit{key: k.token + "|" + k.day, session: merged})
	}
	return units
}

// mergeSessions collapses a project-day's sessions into one synthetic session:
// the earliest start, a span equal to the summed durations, the union of files,
// the first non-empty branch/issue, and the joined distinct descriptions.
func mergeSessions(group []sessions.Session) sessions.Session {
	out := sessions.Session{ProjectToken: group[0].ProjectToken, Start: group[0].Start}
	var total time.Duration
	seenFile := make(map[string]bool)
	seenDesc := make(map[string]bool)
	var descs []string
	for _, s := range group {
		if s.Start.Before(out.Start) {
			out.Start = s.Start
		}
		total += s.Duration()
		out.EventCount += s.EventCount
		if out.Branch == "" {
			out.Branch = s.Branch
		}
		if out.IssueID == "" {
			out.IssueID = s.IssueID
		}
		for _, f := range s.Files {
			if !seenFile[f] {
				seenFile[f] = true
				out.Files = append(out.Files, f)
			}
		}
		if s.Description != "" && !seenDesc[s.Description] {
			seenDesc[s.Description] = true
			descs = append(descs, s.Description)
		}
	}
	out.End = out.Start.Add(total)
	out.Description = strings.Join(descs, "; ")
	return out
}

// entryFor renders a session into a Clockify payload, applying the project
// mapping, rounding, and description template. ok is false when the session's
// project token is not yet mapped.
func (p *pusher) entryFor(s sessions.Session) (clockify.NewTimeEntry, bool, error) {
	mapping, ok, err := config.ResolveMapping(p.root, s.ProjectToken)
	if err != nil {
		return clockify.NewTimeEntry{}, false, err
	}
	if !ok || mapping.Project == "" {
		return clockify.NewTimeEntry{}, false, nil
	}
	return p.entryWith(s, mapping), true, nil
}

// entryWith renders a session into a Clockify payload against an explicit
// mapping, applying rounding and the description template. It is the shared
// core behind both the token-resolved push path and the review UI, where the
// user may reassign a session to a project/task that differs from its token's
// committed mapping.
func (p *pusher) entryWith(s sessions.Session, mapping config.Mapping) clockify.NewTimeEntry {
	dur := rounding.Round(s.Duration(), p.mode)
	if dur <= 0 {
		dur = sessions.MinDuration
	}
	return clockify.NewTimeEntry{
		Start:       s.Start,
		End:         s.Start.Add(dur),
		Description: template.Expand(s, p.template),
		ProjectID:   mapping.Project,
		TaskID:      mapping.Task,
		Billable:    p.billable,
	}
}

// contentHash is a stable digest of the fields that determine a Clockify entry,
// so an unchanged session skips and a changed one updates.
func contentHash(e clockify.NewTimeEntry) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		e.Start.UTC().Format(time.RFC3339),
		e.End.UTC().Format(time.RFC3339),
		e.Description,
		e.ProjectID,
		e.TaskID,
		strconv.FormatBool(e.Billable),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

// pushPeriod resolves the optional period argument (default today) into a
// half-open [start, end) window.
func pushPeriod(args []string) (start, end time.Time, err error) {
	period := "today"
	if len(args) == 1 {
		period = args[0]
	}
	return periodRange(period, time.Now())
}

// loadPushConfig resolves the current repo root, loads the merged config, and
// verifies the credentials needed to talk to Clockify are present.
func loadPushConfig() (*config.Config, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("determine working directory: %w", err)
	}
	root := gitctx.Root(cwd)
	if root == "" {
		root = cwd
	}
	cfg, err := config.Load(root)
	if err != nil {
		return nil, "", err
	}
	if cfg.APIKey == "" {
		return nil, "", fmt.Errorf("no Clockify API key configured — run `clk auth login` first")
	}
	if cfg.WorkspaceID == "" {
		return nil, "", fmt.Errorf("no Clockify workspace selected — run `clk auth login` first")
	}
	return cfg, root, nil
}

// openStore opens the shared SQLite state database.
func openStore() (*store.Store, error) {
	path, err := dbPath()
	if err != nil {
		return nil, err
	}
	return store.Open(path)
}

// reconstructPeriod reads the period's raw events and reconstructs them into
// sessions on the fly — the same source of truth as `clk list`, so push keys
// stay independent of the volatile materialized-session row ids.
func reconstructPeriod(st *store.Store, start, end time.Time) ([]sessions.Session, error) {
	events, err := st.EventsBetween(start, end)
	if err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}
	return sessions.Reconstruct(events), nil
}

// recordLink upserts the push link for a session key after a create or update.
func recordLink(st *store.Store, key, entryID, hash string) error {
	return st.UpsertPushLink(store.PushLink{
		SessionKey:      key,
		ClockifyEntryID: entryID,
		ContentHash:     hash,
		PushedAt:        time.Now(),
	})
}

// toPlannerLinks adapts stored links into the planner's link type.
func toPlannerLinks(links []store.PushLink) []pushplanner.PushLink {
	out := make([]pushplanner.PushLink, len(links))
	for i, l := range links {
		out[i] = pushplanner.PushLink{
			SessionKey:      l.SessionKey,
			ClockifyEntryID: l.ClockifyEntryID,
			ContentHash:     l.ContentHash,
		}
	}
	return out
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func init() {
	pushCmd.Flags().BoolVar(&pushMerge, "merge", false, "collapse the day's sessions per project into a single entry")
	rootCmd.AddCommand(pushCmd, unpushCmd, logCmd)
}
