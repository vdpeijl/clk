package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var followLogs bool

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show the daemon log, optionally following new output",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		path, err := logPath()
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(cmd.OutOrStdout(), "no daemon logs yet")
			return nil
		}
		if err != nil {
			return err
		}
		defer f.Close()

		out := cmd.OutOrStdout()
		if _, err := io.Copy(out, f); err != nil {
			return err
		}
		if !followLogs {
			return nil
		}
		return follow(f, out)
	},
}

// follow streams newly appended bytes from f until the user interrupts with
// Ctrl-C (SIGINT/SIGTERM).
func follow(f *os.File, out io.Writer) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	buf := make([]byte, 4096)
	for {
		select {
		case <-sigCh:
			return nil
		default:
		}

		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if errors.Is(err, io.EOF) || n == 0 {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
	}
}

func init() {
	logsCmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "follow the log output")
	rootCmd.AddCommand(logsCmd)
}
