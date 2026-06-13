package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/vdpeijl/clk/internal/capture"
	"github.com/vdpeijl/clk/internal/gitctx"
	"github.com/vdpeijl/clk/internal/hookparse"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Ingest activity from an editor or tool hook",
}

var hookClaudeCodeCmd = &cobra.Command{
	Use:   "claude-code",
	Short: "Ingest a Claude Code PostToolUse payload from stdin",
	Long: `Reads a Claude Code PostToolUse JSON payload on stdin, attaches the
current git branch and PROJ-123-style issue id detected from the working
directory, and stores the resulting event in ~/.clk/state.db.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		raw, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}

		event, err := hookparse.Parse(raw, hookparse.SourceClaudeCode)
		if err != nil {
			return err
		}

		ctx := gitctx.Detect(hookparse.CWD(raw))
		event.ProjectToken = ctx.ProjectToken
		event.Branch = ctx.Branch
		event.IssueID = ctx.IssueID
		event.Timestamp = time.Now()

		p, err := resolveDaemonPaths()
		if err != nil {
			return err
		}
		exe, args, err := daemonSpawn()
		if err != nil {
			return err
		}

		// Fire-and-forget through the daemon, auto-starting it (with no loss of
		// this event) the first time a hook fires.
		if err := capture.EnsureRunningAndSend(
			exe, args, p.socket, p.pid, p.log, capture.FromSessionEvent(event),
		); err != nil {
			return fmt.Errorf("deliver event to daemon: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "captured %s event in %q\n", event.Source, event.ProjectToken)
		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookClaudeCodeCmd)
	rootCmd.AddCommand(hookCmd)
}
