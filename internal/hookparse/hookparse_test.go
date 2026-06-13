package hookparse

import "testing"

func TestParseClaudeCode(t *testing.T) {
	raw := []byte(`{
		"hook_event_name": "PostToolUse",
		"tool_name": "Edit",
		"cwd": "/home/dev/clk",
		"tool_input": {"file_path": "internal/sessions/sessions.go"}
	}`)

	e, err := Parse(raw, SourceClaudeCode)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if e.Source != "claude_code" {
		t.Errorf("source = %q, want claude_code", e.Source)
	}
	if e.Type != "tool_use" {
		t.Errorf("type = %q, want tool_use", e.Type)
	}
	if e.FilePath != "internal/sessions/sessions.go" {
		t.Errorf("file path = %q", e.FilePath)
	}
	if e.Description != "Edit internal/sessions/sessions.go" {
		t.Errorf("description = %q", e.Description)
	}
	if !e.Timestamp.IsZero() {
		t.Errorf("timestamp should be left unset by the parser, got %v", e.Timestamp)
	}
}

func TestParseClaudeCodeNoFilePath(t *testing.T) {
	raw := []byte(`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"ls"}}`)
	e, err := Parse(raw, SourceClaudeCode)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if e.FilePath != "" {
		t.Errorf("file path = %q, want empty", e.FilePath)
	}
	if e.Description != "Bash" {
		t.Errorf("description = %q, want Bash", e.Description)
	}
}

func TestParseClaudeCodeMissingToolName(t *testing.T) {
	raw := []byte(`{"hook_event_name":"PostToolUse"}`)
	if _, err := Parse(raw, SourceClaudeCode); err == nil {
		t.Fatal("expected error for missing tool_name")
	}
}

func TestParseCursorFileEdit(t *testing.T) {
	raw := []byte(`{
		"hook_event_name": "afterFileEdit",
		"workspace_roots": ["/home/dev/clk"],
		"file_path": "internal/store/store.go"
	}`)
	e, err := Parse(raw, SourceCursor)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if e.Source != "cursor" {
		t.Errorf("source = %q, want cursor", e.Source)
	}
	if e.Type != "tool_use" {
		t.Errorf("type = %q, want tool_use", e.Type)
	}
	if e.FilePath != "internal/store/store.go" {
		t.Errorf("file path = %q", e.FilePath)
	}
	if e.Description != "afterFileEdit internal/store/store.go" {
		t.Errorf("description = %q", e.Description)
	}
}

func TestParseCursorShellExecution(t *testing.T) {
	raw := []byte(`{"hook_event_name":"beforeShellExecution","command":"go test ./..."}`)
	e, err := Parse(raw, SourceCursor)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if e.FilePath != "" {
		t.Errorf("file path = %q, want empty", e.FilePath)
	}
	if e.Description != "beforeShellExecution go test ./..." {
		t.Errorf("description = %q", e.Description)
	}
}

func TestParseCursorMissingEventName(t *testing.T) {
	if _, err := Parse([]byte(`{"file_path":"x"}`), SourceCursor); err == nil {
		t.Fatal("expected error for missing hook_event_name")
	}
}

func TestParseCopilot(t *testing.T) {
	raw := []byte(`{
		"hook_event_name": "post_tool",
		"tool": "edit_file",
		"cwd": "/home/dev/clk",
		"arguments": {"path": "cmd/init.go"}
	}`)
	e, err := Parse(raw, SourceCopilot)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if e.Source != "copilot" {
		t.Errorf("source = %q, want copilot", e.Source)
	}
	if e.FilePath != "cmd/init.go" {
		t.Errorf("file path = %q", e.FilePath)
	}
	if e.Description != "edit_file cmd/init.go" {
		t.Errorf("description = %q", e.Description)
	}
}

func TestParseCopilotMissingTool(t *testing.T) {
	if _, err := Parse([]byte(`{"hook_event_name":"post_tool"}`), SourceCopilot); err == nil {
		t.Fatal("expected error for missing tool")
	}
}

func TestParseInvalidJSON(t *testing.T) {
	for _, src := range []Source{SourceClaudeCode, SourceCursor, SourceCopilot} {
		if _, err := Parse([]byte("not json"), src); err == nil {
			t.Errorf("expected error for invalid JSON from %s", src)
		}
	}
}

func TestParseUnknownSource(t *testing.T) {
	if _, err := Parse([]byte(`{}`), Source("nope")); err == nil {
		t.Fatal("expected error for unknown source")
	}
}

func TestGitEvent(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		sha     string
		want    string
	}{
		{name: "subject preferred", subject: "feat: add init", sha: "abcdef1234", want: "feat: add init"},
		{name: "short sha fallback", subject: "  ", sha: "abcdef1234567", want: "commit abcdef1"},
		{name: "bare commit fallback", subject: "", sha: "", want: "commit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := GitEvent(tt.subject, tt.sha)
			if e.Source != "git" {
				t.Errorf("source = %q, want git", e.Source)
			}
			if e.Type != "git_commit" {
				t.Errorf("type = %q, want git_commit", e.Type)
			}
			if e.Description != tt.want {
				t.Errorf("description = %q, want %q", e.Description, tt.want)
			}
		})
	}
}

func TestCWD(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		source Source
		want   string
	}{
		{name: "claude code cwd", raw: `{"tool_name":"Edit","cwd":"/home/dev/clk"}`, source: SourceClaudeCode, want: "/home/dev/clk"},
		{name: "cursor first workspace root", raw: `{"workspace_roots":["/a","/b"]}`, source: SourceCursor, want: "/a"},
		{name: "cursor no roots", raw: `{"hook_event_name":"stop"}`, source: SourceCursor, want: ""},
		{name: "copilot cwd", raw: `{"tool":"x","cwd":"/home/dev/clk"}`, source: SourceCopilot, want: "/home/dev/clk"},
		{name: "invalid json", raw: "nope", source: SourceClaudeCode, want: ""},
		{name: "git has no cwd", raw: `{}`, source: SourceGit, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CWD([]byte(tt.raw), tt.source); got != tt.want {
				t.Errorf("CWD = %q, want %q", got, tt.want)
			}
		})
	}
}
