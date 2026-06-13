package cmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/vdpeijl/clk/internal/capture"
)

// shutdownGrace is how long `clk down` waits for a graceful exit before SIGKILL.
const shutdownGrace = 5 * time.Second

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the background capture daemon",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := resolveDaemonPaths()
		if err != nil {
			return err
		}
		exe, args, err := daemonSpawn()
		if err != nil {
			return err
		}
		switch err := capture.Start(exe, args, p.socket, p.pid, p.log); {
		case errors.Is(err, capture.ErrAlreadyRunning):
			fmt.Fprintln(cmd.OutOrStdout(), "daemon already running")
			return nil
		case err != nil:
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "daemon started")
		return nil
	},
}

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the background capture daemon",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := resolveDaemonPaths()
		if err != nil {
			return err
		}
		switch err := capture.Stop(p.pid, shutdownGrace); {
		case errors.Is(err, capture.ErrNotRunning):
			fmt.Fprintln(cmd.OutOrStdout(), "daemon not running")
			return nil
		case err != nil:
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "daemon stopped")
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the daemon state and buffered events",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := resolveDaemonPaths()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()

		pid, alive := capture.IsRunning(p.pid)
		if !alive {
			fmt.Fprintln(out, "daemon: stopped")
			return nil
		}

		st, err := capture.GetStatus(p.socket)
		if err != nil {
			// Process is alive but not answering — report what we know.
			fmt.Fprintf(out, "daemon: running (pid %d), not responding on socket: %v\n", pid, err)
			return nil
		}
		fmt.Fprintf(out, "daemon:   running (pid %d)\n", st.PID)
		fmt.Fprintf(out, "uptime:   %s\n", (time.Duration(st.UptimeSeconds) * time.Second).String())
		fmt.Fprintf(out, "buffered: %d event(s)\n", st.Buffered)
		fmt.Fprintf(out, "captured: %d event(s) since start\n", st.EventsTotal)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(upCmd, downCmd, statusCmd)
}
