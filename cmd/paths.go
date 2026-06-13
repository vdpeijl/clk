package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

// clkDir returns the per-user clk state directory (~/.clk), creating it with
// 0700 permissions if necessary. The CLK_HOME environment variable overrides
// the location, which is convenient for tests and dotfile-managed setups.
func clkDir() (string, error) {
	dir := os.Getenv("CLK_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate home directory: %w", err)
		}
		dir = filepath.Join(home, ".clk")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	return dir, nil
}

// dbPath returns the path to the SQLite state database (~/.clk/state.db).
func dbPath() (string, error) {
	dir, err := clkDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.db"), nil
}
