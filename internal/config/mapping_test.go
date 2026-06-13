package config

import (
	"path/filepath"
	"testing"
)

func TestMergeMappingsUserWins(t *testing.T) {
	project := map[string]Mapping{
		"clk":  {Project: "team-clk", Task: "team-task"},
		"docs": {Project: "team-docs"},
	}
	user := map[string]Mapping{
		"clk": {Project: "my-clk"},
	}

	merged := MergeMappings(project, user)
	if merged["clk"].Project != "my-clk" {
		t.Errorf("user override lost: clk = %+v", merged["clk"])
	}
	if merged["docs"].Project != "team-docs" {
		t.Errorf("unoverridden token lost: docs = %+v", merged["docs"])
	}
	// Inputs must not be mutated.
	if project["clk"].Project != "team-clk" {
		t.Errorf("MergeMappings mutated its project input")
	}
}

func TestResolveMappingPrecedenceAndAbsence(t *testing.T) {
	clkHome := t.TempDir()
	t.Setenv("CLK_HOME", clkHome)

	projectDir := t.TempDir()
	writeFile(t, filepath.Join(projectDir, ".clk.toml"), `
[clockify]
template = "{summary}"

[mappings.clk]
project = "team-clk"
task = "team-task"

[mappings.docs]
project = "team-docs"
`)
	writeFile(t, filepath.Join(clkHome, "config.toml"), `
[clockify]
api_key = "secret"

[mappings.clk]
project = "my-clk"
`)

	// Personal override wins for clk.
	m, ok, err := ResolveMapping(projectDir, "clk")
	if err != nil {
		t.Fatalf("ResolveMapping clk: %v", err)
	}
	if !ok || m.Project != "my-clk" {
		t.Errorf("clk resolved to %+v (ok=%v), want my-clk", m, ok)
	}

	// docs only exists in the committed file.
	m, ok, err = ResolveMapping(projectDir, "docs")
	if err != nil {
		t.Fatalf("ResolveMapping docs: %v", err)
	}
	if !ok || m.Project != "team-docs" {
		t.Errorf("docs resolved to %+v (ok=%v), want team-docs", m, ok)
	}

	// Unmapped token is the prompt-once signal.
	if _, ok, err := ResolveMapping(projectDir, "unknown"); err != nil || ok {
		t.Errorf("unknown token should be unmapped, got ok=%v err=%v", ok, err)
	}
}

func TestSetProjectMappingPreservesTemplateAndOthers(t *testing.T) {
	t.Setenv("CLK_HOME", t.TempDir())
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".clk.toml"), `
[clockify]
template = "{issue} {branch}: {summary}"
billable = true

[mappings.other]
project = "keep-me"
`)

	if err := SetProjectMapping(dir, "clk", Mapping{Project: "new-proj", Task: "new-task"}); err != nil {
		t.Fatalf("SetProjectMapping: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Template != "{issue} {branch}: {summary}" {
		t.Errorf("template clobbered: %q", cfg.Template)
	}
	if !cfg.Billable {
		t.Error("billable default lost")
	}

	mappings, err := ProjectMappings(dir)
	if err != nil {
		t.Fatalf("ProjectMappings: %v", err)
	}
	if mappings["clk"].Project != "new-proj" || mappings["clk"].Task != "new-task" {
		t.Errorf("new mapping not written: %+v", mappings["clk"])
	}
	if mappings["other"].Project != "keep-me" {
		t.Errorf("existing mapping clobbered: %+v", mappings["other"])
	}
}

func TestSetUserMappingPreservesCredentials(t *testing.T) {
	clkHome := t.TempDir()
	t.Setenv("CLK_HOME", clkHome)

	if err := SaveUser(UserConfig{APIKey: "secret", WorkspaceID: "ws-1", PushRound: "15m"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	if err := SetUserMapping("clk", Mapping{Project: "personal-clk"}); err != nil {
		t.Fatalf("SetUserMapping: %v", err)
	}

	uc, err := LoadUser()
	if err != nil {
		t.Fatalf("LoadUser: %v", err)
	}
	if uc.APIKey != "secret" || uc.WorkspaceID != "ws-1" || uc.PushRound != "15m" {
		t.Errorf("credentials lost after SetUserMapping: %+v", uc)
	}

	mappings, err := UserMappings()
	if err != nil {
		t.Fatalf("UserMappings: %v", err)
	}
	if mappings["clk"].Project != "personal-clk" {
		t.Errorf("personal mapping not written: %+v", mappings["clk"])
	}
}

func TestSaveUserPreservesExistingMappings(t *testing.T) {
	clkHome := t.TempDir()
	t.Setenv("CLK_HOME", clkHome)

	if err := SetUserMapping("clk", Mapping{Project: "personal-clk"}); err != nil {
		t.Fatalf("SetUserMapping: %v", err)
	}
	// Simulate `clk auth login` overwriting credentials only.
	if err := SaveUser(UserConfig{APIKey: "rotated", WorkspaceID: "ws-2"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	mappings, err := UserMappings()
	if err != nil {
		t.Fatalf("UserMappings: %v", err)
	}
	if mappings["clk"].Project != "personal-clk" {
		t.Errorf("auth login wiped personal mapping: %+v", mappings)
	}
}
