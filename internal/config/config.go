// Package config loads and merges clk configuration from multiple sources
// with defined precedence: .clk.toml < ~/.clk/config.toml < environment.
//
// The pure merge logic (Merge) is deliberately separated from the file and
// environment reads (Load, LoadUser, SaveUser) so it can be unit-tested across
// all three layers without touching the filesystem.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// projectFileName is the committed, per-repo config file.
const projectFileName = ".clk.toml"

// userFileName is the personal config file inside the clk home directory.
const userFileName = "config.toml"

// envAPIKey is the environment variable that overrides the stored API key.
const envAPIKey = "CLOCKIFY_API_KEY"

// ProjectConfig is sourced from the committed .clk.toml in a repo. It carries
// the team-shared conventions: which Clockify project/task a repo maps to, the
// billable default, and the description template.
type ProjectConfig struct {
	ClockifyProject string
	ClockifyTask    string
	Billable        bool
	Template        string
}

// UserConfig is sourced from ~/.clk/config.toml (mode 0600, never committed).
// It carries the personal secret (API key), the pinned workspace, the push
// rounding mode, and optional personal mapping overrides that take precedence
// over the committed project mapping.
type UserConfig struct {
	APIKey      string
	WorkspaceID string
	PushRound   string
	// Personal overrides of the committed .clk.toml mapping. Empty means
	// "inherit the project value".
	ClockifyProject string
	ClockifyTask    string
}

// Env carries the configuration drawn from environment variables.
type Env struct {
	APIKey string
}

// Config is the merged runtime configuration.
type Config struct {
	ClockifyProject string
	ClockifyTask    string
	Billable        bool
	Template        string
	APIKey          string
	WorkspaceID     string
	PushRound       string
}

// Merge combines the three configuration layers with precedence
// project < user < env: each later layer overrides a non-empty value from an
// earlier one. It is pure — no file or environment access — so the precedence
// rules can be exercised directly in tests.
func Merge(project ProjectConfig, user UserConfig, env Env) Config {
	c := Config{
		ClockifyProject: project.ClockifyProject,
		ClockifyTask:    project.ClockifyTask,
		Billable:        project.Billable,
		Template:        project.Template,
	}

	// ~/.clk/config.toml: personal secrets, workspace, push rounding, and any
	// personal overrides of the committed project mapping.
	c.APIKey = user.APIKey
	c.WorkspaceID = user.WorkspaceID
	c.PushRound = user.PushRound
	if user.ClockifyProject != "" {
		c.ClockifyProject = user.ClockifyProject
	}
	if user.ClockifyTask != "" {
		c.ClockifyTask = user.ClockifyTask
	}

	// CLOCKIFY_API_KEY overrides the stored key.
	if env.APIKey != "" {
		c.APIKey = env.APIKey
	}

	return c
}

// projectFile mirrors the on-disk layout of .clk.toml.
type projectFile struct {
	Clockify struct {
		Project  string `toml:"project"`
		Task     string `toml:"task"`
		Billable bool   `toml:"billable"`
		Template string `toml:"template"`
	} `toml:"clockify"`
}

// userFile mirrors the on-disk layout of ~/.clk/config.toml.
type userFile struct {
	Clockify struct {
		APIKey    string `toml:"api_key"`
		Workspace string `toml:"workspace"`
		Project   string `toml:"project"`
		Task      string `toml:"task"`
	} `toml:"clockify"`
	Push struct {
		Round string `toml:"round"`
	} `toml:"push"`
}

// Load reads the committed .clk.toml from projectPath, the personal
// ~/.clk/config.toml, and the environment, then merges them with the defined
// precedence. Missing files are treated as empty, not errors.
func Load(projectPath string) (*Config, error) {
	project, err := loadProject(filepath.Join(projectPath, projectFileName))
	if err != nil {
		return nil, err
	}

	user, err := LoadUser()
	if err != nil {
		return nil, err
	}

	merged := Merge(project, user, EnvFromOS())
	return &merged, nil
}

// EnvFromOS reads the configuration-relevant environment variables.
func EnvFromOS() Env {
	return Env{APIKey: os.Getenv(envAPIKey)}
}

// loadProject reads a .clk.toml file. A missing file yields a zero ProjectConfig.
func loadProject(path string) (ProjectConfig, error) {
	var f projectFile
	if err := decodeTOML(path, &f); err != nil {
		return ProjectConfig{}, err
	}
	return ProjectConfig{
		ClockifyProject: f.Clockify.Project,
		ClockifyTask:    f.Clockify.Task,
		Billable:        f.Clockify.Billable,
		Template:        f.Clockify.Template,
	}, nil
}

// LoadUser reads ~/.clk/config.toml. A missing file yields a zero UserConfig.
func LoadUser() (UserConfig, error) {
	path, err := UserConfigPath()
	if err != nil {
		return UserConfig{}, err
	}
	var f userFile
	if err := decodeTOML(path, &f); err != nil {
		return UserConfig{}, err
	}
	return UserConfig{
		APIKey:          f.Clockify.APIKey,
		WorkspaceID:     f.Clockify.Workspace,
		PushRound:       f.Push.Round,
		ClockifyProject: f.Clockify.Project,
		ClockifyTask:    f.Clockify.Task,
	}, nil
}

// SaveUser writes the personal config to ~/.clk/config.toml at mode 0600,
// creating the clk home directory (0700) if necessary. Because the file holds
// the Clockify API key, the restrictive permissions are enforced on every write.
func SaveUser(uc UserConfig) error {
	path, err := UserConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	var f userFile
	f.Clockify.APIKey = uc.APIKey
	f.Clockify.Workspace = uc.WorkspaceID
	f.Clockify.Project = uc.ClockifyProject
	f.Clockify.Task = uc.ClockifyTask
	f.Push.Round = uc.PushRound

	// Open with 0600 up front so the secret is never briefly world-readable.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	// Re-assert the mode in case the file already existed with looser bits.
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	if err := toml.NewEncoder(file).Encode(f); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return nil
}

// UserConfigPath returns the path to ~/.clk/config.toml. The CLK_HOME
// environment variable overrides the directory, which is convenient for tests
// and dotfile-managed setups.
func UserConfigPath() (string, error) {
	dir := os.Getenv("CLK_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate home directory: %w", err)
		}
		dir = filepath.Join(home, ".clk")
	}
	return filepath.Join(dir, userFileName), nil
}

// decodeTOML decodes the TOML file at path into v. A non-existent file is not an
// error: v is left as its zero value.
func decodeTOML(path string, v any) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if _, err := toml.DecodeFile(path, v); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
