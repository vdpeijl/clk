// Package config loads and merges clk configuration from multiple sources
// with defined precedence: .clk.toml < ~/.clk/config.toml < environment.
package config

// ProjectConfig is sourced from the committed .clk.toml in a repo.
type ProjectConfig struct {
	ClockifyProject string
	ClockifyTask    string
	Billable        bool
	Template        string
}

// UserConfig is sourced from ~/.clk/config.toml (mode 0600, never committed).
type UserConfig struct {
	APIKey      string
	WorkspaceID string
	PushRound   string
}

// Config is the merged runtime configuration.
type Config struct {
	ProjectConfig
	UserConfig
}

// Load merges project config, user config, and environment variables.
func Load(projectPath string) (*Config, error) {
	// TODO: implement config loading and merge
	return &Config{}, nil
}
