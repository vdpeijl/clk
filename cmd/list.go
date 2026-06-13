package cmd

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/vdpeijl/clk/internal/sessions"
	"github.com/vdpeijl/clk/internal/store"
)

var listCmd = &cobra.Command{
	Use:   "list <today|yesterday|week|month>",
	Short: "List reconstructed work sessions for a period",
	Long: `Reads captured events for the given period from ~/.clk/state.db,
reconstructs them into work sessions, and prints them with start/end, duration,
project, and source.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		period := args[0]
		start, end, err := periodRange(period, time.Now())
		if err != nil {
			return err
		}

		path, err := dbPath()
		if err != nil {
			return err
		}
		st, err := store.Open(path)
		if err != nil {
			return err
		}
		defer st.Close()

		events, err := st.EventsBetween(start, end)
		if err != nil {
			return fmt.Errorf("read events: %w", err)
		}

		printSessions(cmd.OutOrStdout(), period, sessions.Reconstruct(events))
		return nil
	},
}

// periodRange maps a period name to a half-open [start, end) time window
// relative to now, in now's location. It is pure.
func periodRange(period string, now time.Time) (start, end time.Time, err error) {
	y, m, d := now.Date()
	loc := now.Location()
	startToday := time.Date(y, m, d, 0, 0, 0, 0, loc)
	startTomorrow := startToday.AddDate(0, 0, 1)

	switch period {
	case "today":
		return startToday, startTomorrow, nil
	case "yesterday":
		return startToday.AddDate(0, 0, -1), startToday, nil
	case "week":
		// Monday is treated as the first day of the week.
		offset := (int(now.Weekday()) + 6) % 7
		return startToday.AddDate(0, 0, -offset), startTomorrow, nil
	case "month":
		return time.Date(y, m, 1, 0, 0, 0, 0, loc), startTomorrow, nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unknown period %q (want today|yesterday|week|month)", period)
	}
}

func printSessions(out io.Writer, period string, ss []sessions.Session) {
	if len(ss) == 0 {
		fmt.Fprintf(out, "No sessions for %s.\n", period)
		return
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "DATE\tSTART\tEND\tDUR\tPROJECT\tBRANCH\tISSUE\tSOURCE")
	for _, s := range ss {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Start.Format("2006-01-02"),
			s.Start.Format("15:04"),
			s.End.Format("15:04"),
			formatDuration(s.Duration()),
			dash(s.ProjectToken),
			dash(s.Branch),
			dash(s.IssueID),
			dash(s.Source),
		)
	}
	tw.Flush()
}

// formatDuration renders a duration rounded to whole minutes, e.g. "28m" or
// "1h05m".
func formatDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := int(d / time.Hour)
	mins := int((d % time.Hour) / time.Minute)
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

// dash renders empty values as "-" for readability in the table.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func init() {
	rootCmd.AddCommand(listCmd)
}
