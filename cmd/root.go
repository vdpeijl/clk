package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "clk",
	Short: "Auto-capture dev activity and push to Clockify",
	Long: `clk captures your dev activity automatically and lets you review
and push it to Clockify as time entries.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
