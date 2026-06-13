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

func TestParseInvalidJSON(t *testing.T) {
	if _, err := Parse([]byte("not json"), SourceClaudeCode); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseUnknownSource(t *testing.T) {
	if _, err := Parse([]byte(`{}`), Source("nope")); err == nil {
		t.Fatal("expected error for unknown source")
	}
}

func TestCWD(t *testing.T) {
	raw := []byte(`{"tool_name":"Edit","cwd":"/home/dev/clk"}`)
	if got := CWD(raw); got != "/home/dev/clk" {
		t.Errorf("CWD = %q, want /home/dev/clk", got)
	}
	if got := CWD([]byte("nope")); got != "" {
		t.Errorf("CWD = %q, want empty for invalid json", got)
	}
}
