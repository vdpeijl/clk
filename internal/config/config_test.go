package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergePrecedence(t *testing.T) {
	tests := []struct {
		name    string
		project ProjectConfig
		user    UserConfig
		env     Env
		want    Config
	}{
		{
			name:    "project only supplies the team-shared mapping",
			project: ProjectConfig{ClockifyProject: "clk", ClockifyTask: "build", Billable: true, Template: "{branch}"},
			want:    Config{ClockifyProject: "clk", ClockifyTask: "build", Billable: true, Template: "{branch}"},
		},
		{
			name: "user supplies secrets, workspace and rounding",
			user: UserConfig{APIKey: "k", WorkspaceID: "w", PushRound: "15m"},
			want: Config{APIKey: "k", WorkspaceID: "w", PushRound: "15m"},
		},
		{
			name:    "user mapping overrides the committed project mapping",
			project: ProjectConfig{ClockifyProject: "team-proj", ClockifyTask: "team-task", Template: "{summary}"},
			user:    UserConfig{ClockifyProject: "my-proj", ClockifyTask: "my-task"},
			want:    Config{ClockifyProject: "my-proj", ClockifyTask: "my-task", Template: "{summary}"},
		},
		{
			name:    "empty user mapping inherits the project mapping",
			project: ProjectConfig{ClockifyProject: "team-proj", ClockifyTask: "team-task"},
			user:    UserConfig{APIKey: "k"},
			want:    Config{ClockifyProject: "team-proj", ClockifyTask: "team-task", APIKey: "k"},
		},
		{
			name: "env overrides the stored API key",
			user: UserConfig{APIKey: "stored", WorkspaceID: "w"},
			env:  Env{APIKey: "from-env"},
			want: Config{APIKey: "from-env", WorkspaceID: "w"},
		},
		{
			name: "all three layers combined",
			project: ProjectConfig{
				ClockifyProject: "team-proj",
				ClockifyTask:    "team-task",
				Billable:        true,
				Template:        "{issue} {branch}: {summary}",
			},
			user: UserConfig{
				APIKey:          "stored",
				WorkspaceID:     "ws-1",
				PushRound:       "5m",
				ClockifyProject: "my-proj",
			},
			env: Env{APIKey: "env-key"},
			want: Config{
				ClockifyProject: "my-proj",
				ClockifyTask:    "team-task",
				Billable:        true,
				Template:        "{issue} {branch}: {summary}",
				APIKey:          "env-key",
				WorkspaceID:     "ws-1",
				PushRound:       "5m",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Merge(tt.project, tt.user, tt.env)
			if got != tt.want {
				t.Errorf("Merge() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadMergesAllLayers(t *testing.T) {
	clkHome := t.TempDir()
	t.Setenv("CLK_HOME", clkHome)
	t.Setenv("CLOCKIFY_API_KEY", "env-key")

	projectDir := t.TempDir()
	writeFile(t, filepath.Join(projectDir, ".clk.toml"), `
[clockify]
project = "team-proj"
task = "team-task"
billable = true
template = "{issue} {branch}: {summary}"
`)
	writeFile(t, filepath.Join(clkHome, "config.toml"), `
[clockify]
api_key = "stored-key"
workspace = "ws-1"
project = "my-proj"

[push]
round = "5m"
`)

	cfg, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := Config{
		ClockifyProject: "my-proj", // user override wins over project
		ClockifyTask:    "team-task",
		Billable:        true,
		Template:        "{issue} {branch}: {summary}",
		APIKey:          "env-key", // env wins over stored
		WorkspaceID:     "ws-1",
		PushRound:       "5m",
	}
	if *cfg != want {
		t.Errorf("Load() = %+v, want %+v", *cfg, want)
	}
}

func TestLoadMissingFiles(t *testing.T) {
	t.Setenv("CLK_HOME", t.TempDir())
	t.Setenv("CLOCKIFY_API_KEY", "")

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load with no files: %v", err)
	}
	if (*cfg != Config{}) {
		t.Errorf("Load() = %+v, want zero Config", *cfg)
	}
}

func TestSaveUserRoundTripAndMode(t *testing.T) {
	clkHome := t.TempDir()
	t.Setenv("CLK_HOME", clkHome)

	in := UserConfig{
		APIKey:          "secret-key",
		WorkspaceID:     "ws-9",
		PushRound:       "15m",
		ClockifyProject: "personal-proj",
		ClockifyTask:    "personal-task",
	}
	if err := SaveUser(in); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	path := filepath.Join(clkHome, "config.toml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat saved config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("config file mode = %o, want 600", got)
	}

	got, err := LoadUser()
	if err != nil {
		t.Fatalf("LoadUser: %v", err)
	}
	if got != in {
		t.Errorf("LoadUser() = %+v, want %+v", got, in)
	}
}

func TestSaveUserTightensExistingMode(t *testing.T) {
	clkHome := t.TempDir()
	t.Setenv("CLK_HOME", clkHome)

	path := filepath.Join(clkHome, "config.toml")
	writeFile(t, path, "old = true\n")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if err := SaveUser(UserConfig{APIKey: "k"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("config file mode = %o, want 600", got)
	}
}

func TestScaffoldProjectCreatesAndLoadsBack(t *testing.T) {
	dir := t.TempDir()
	pc := ProjectConfig{ClockifyProject: "clk", Template: DefaultTemplate}

	created, err := ScaffoldProject(dir, pc)
	if err != nil {
		t.Fatalf("ScaffoldProject: %v", err)
	}
	if !created {
		t.Fatal("expected created=true for a fresh directory")
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ClockifyProject != "clk" {
		t.Errorf("project = %q, want clk", cfg.ClockifyProject)
	}
	if cfg.Template != DefaultTemplate {
		t.Errorf("template = %q, want %q", cfg.Template, DefaultTemplate)
	}
}

func TestScaffoldProjectDoesNotClobber(t *testing.T) {
	t.Setenv("CLK_HOME", t.TempDir())
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".clk.toml"), "[clockify]\nproject = \"team\"\n")

	created, err := ScaffoldProject(dir, ProjectConfig{ClockifyProject: "overwrite"})
	if err != nil {
		t.Fatalf("ScaffoldProject: %v", err)
	}
	if created {
		t.Fatal("expected created=false when .clk.toml already exists")
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ClockifyProject != "team" {
		t.Errorf("existing config clobbered: project = %q, want team", cfg.ClockifyProject)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
