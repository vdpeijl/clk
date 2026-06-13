package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/vdpeijl/clk/internal/capture"
	"github.com/vdpeijl/clk/internal/gitctx"
	"github.com/vdpeijl/clk/internal/hookparse"
	"github.com/vdpeijl/clk/internal/sessions"
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
		return ingestStdinHook(cmd, hookparse.SourceClaudeCode)
	},
}

var hookCursorCmd = &cobra.Command{
	Use:   "cursor",
	Short: "Ingest a Cursor agent hook payload from stdin",
	Long: `Reads a Cursor agent hook JSON payload on stdin, attaches the current
git branch and PROJ-123-style issue id detected from the workspace, and stores
the resulting event in ~/.clk/state.db.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return ingestStdinHook(cmd, hookparse.SourceCursor)
	},
}

var hookCopilotCmd = &cobra.Command{
	Use:   "copilot",
	Short: "Ingest a Copilot CLI hook payload from stdin",
	Long: `Reads a Copilot CLI hook JSON payload on stdin, attaches the current
git branch and PROJ-123-style issue id detected from the working directory, and
stores the resulting event in ~/.clk/state.db.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return ingestStdinHook(cmd, hookparse.SourceCopilot)
	},
}

var hookGitCmd = &cobra.Command{
	Use:   "git",
	Short: "Capture the latest commit as an event (run from a git post-commit hook)",
	Long: `Inspects the repository in the current directory, reads the most recent
commit, and stores it as an event in ~/.clk/state.db. This is invoked by the
post-commit hook installed by 'clk init'.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("determine working directory: %w", err)
		}

		commit := gitctx.LastCommit(cwd)
		if commit.SHA == "" {
			return fmt.Errorf("no commit found in %q", cwd)
		}
		event := hookparse.GitEvent(commit.Subject, commit.SHA)
		return deliverEvent(cmd, event, cwd)
	},
}

// ingestStdinHook reads a JSON hook payload for source from stdin, parses it,
// attaches git context from the working directory it reports, and delivers the
// event to the daemon.
func ingestStdinHook(cmd *cobra.Command, source hookparse.Source) error {
	raw, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	event, err := hookparse.Parse(raw, source)
	if err != nil {
		return err
	}
	return deliverEvent(cmd, event, hookparse.CWD(raw, source))
}

// deliverEvent stamps the event with git context for cwd and the current time,
// then fires it to the daemon (auto-starting the daemon on the first hook).
func deliverEvent(cmd *cobra.Command, event sessions.Event, cwd string) error {
	ctx := gitctx.Detect(cwd)
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
}

func init() {
	hookCmd.AddCommand(hookClaudeCodeCmd, hookCursorCmd, hookCopilotCmd, hookGitCmd)
	rootCmd.AddCommand(hookCmd)
}
