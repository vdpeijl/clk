package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vdpeijl/clk/internal/upgrade"
)

// upgradeRepo is the GitHub repository releases are pulled from.
const upgradeRepo = "vdpeijl/clk"

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade clk to the latest release",
	Long: `Downloads the latest clk release for this platform from GitHub and
replaces the running binary in place. Does nothing when already up to date.

If the executable lives in a directory you cannot write to (for example a
Homebrew prefix), run "brew upgrade clk" instead, or re-run with sufficient
permissions.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runUpgrade(cmd)
	},
}

func runUpgrade(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate clk executable: %w", err)
	}
	// Resolve symlinks so we replace the real file, not a link to it.
	if resolved, err := os.Readlink(execPath); err == nil && resolved != "" {
		execPath = resolved
	}

	fmt.Fprintf(out, "Current version: %s\nChecking for updates…\n", Version)

	client := upgrade.New(upgradeRepo)
	res, err := client.Run(cmd.Context(), Version, execPath)
	if err != nil {
		return err
	}

	if !res.Upgraded {
		fmt.Fprintf(out, "Already up to date (%s).\n", res.To)
		return nil
	}

	fmt.Fprintf(out, "Upgraded clk %s → %s (%s)\n", res.From, res.To, res.BinaryPath)
	return nil
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
}
