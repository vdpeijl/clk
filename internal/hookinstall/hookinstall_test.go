package hookinstall

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		in   Detection
		want []Tool
	}{
		{name: "nothing detected", in: Detection{}, want: nil},
		{
			name: "all sources",
			in:   Detection{ClaudeBin: true, CursorDir: true, CopilotBin: true, CodexDir: true, GitRepo: true},
			want: []Tool{ToolClaudeCode, ToolCursor, ToolCopilot, ToolCodex, ToolGit},
		},
		{
			name: "dir or bin each suffice",
			in:   Detection{ClaudeDir: true, CopilotDir: true},
			want: []Tool{ToolClaudeCode, ToolCopilot},
		},
		{name: "codex via dir", in: Detection{CodexDir: true}, want: []Tool{ToolCodex}},
		{name: "codex via bin", in: Detection{CodexBin: true}, want: []Tool{ToolCodex}},
		{name: "git only", in: Detection{GitRepo: true}, want: []Tool{ToolGit}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Detect(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Detect() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMergeClaudeSettingsFresh(t *testing.T) {
	out, changed, err := MergeClaudeSettings(nil, ClaudeCommand)
	if err != nil {
		t.Fatalf("MergeClaudeSettings: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true for fresh settings")
	}
	if !claudeHasCommand(t, out, ClaudeCommand) {
		t.Errorf("command not installed: %s", out)
	}
}

func TestMergeClaudeSettingsPreservesAndIsIdempotent(t *testing.T) {
	existing := []byte(`{"model":"opus","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[]}]}}`)

	out, changed, err := MergeClaudeSettings(existing, ClaudeCommand)
	if err != nil {
		t.Fatalf("MergeClaudeSettings: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}

	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if m["model"] != "opus" {
		t.Errorf("unrelated key not preserved: %v", m)
	}
	hooks := m["hooks"].(map[string]any)
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Errorf("existing hook category dropped: %v", hooks)
	}
	if !claudeHasCommand(t, out, ClaudeCommand) {
		t.Errorf("command not installed: %s", out)
	}

	// Second run is a no-op.
	out2, changed2, err := MergeClaudeSettings(out, ClaudeCommand)
	if err != nil {
		t.Fatalf("MergeClaudeSettings (2nd): %v", err)
	}
	if changed2 {
		t.Error("expected changed=false on second run")
	}
	if string(out2) != string(out) {
		t.Errorf("idempotent run changed bytes:\n%s\n---\n%s", out, out2)
	}
}

func TestMergeClaudeSettingsInvalidJSON(t *testing.T) {
	if _, _, err := MergeClaudeSettings([]byte("{not json"), ClaudeCommand); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestMergeEventHooks(t *testing.T) {
	events := []string{"afterFileEdit", "beforeShellExecution"}
	out, changed, err := MergeEventHooks(nil, events, CursorCommand)
	if err != nil {
		t.Fatalf("MergeEventHooks: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}

	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	hooks := m["hooks"].(map[string]any)
	for _, ev := range events {
		entries, ok := hooks[ev].([]any)
		if !ok || len(entries) != 1 {
			t.Fatalf("event %q not installed: %v", ev, hooks[ev])
		}
		if entries[0].(map[string]any)["command"] != CursorCommand {
			t.Errorf("event %q wrong command: %v", ev, entries[0])
		}
	}

	// Idempotent.
	out2, changed2, err := MergeEventHooks(out, events, CursorCommand)
	if err != nil {
		t.Fatalf("MergeEventHooks (2nd): %v", err)
	}
	if changed2 {
		t.Error("expected changed=false on second run")
	}
	if string(out2) != string(out) {
		t.Error("idempotent run changed bytes")
	}
}

func TestMergeEventHooksPreservesExistingEntries(t *testing.T) {
	existing := []byte(`{"hooks":{"afterFileEdit":[{"command":"other-tool"}]}}`)
	out, changed, err := MergeEventHooks(existing, []string{"afterFileEdit"}, CursorCommand)
	if err != nil {
		t.Fatalf("MergeEventHooks: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	entries := m["hooks"].(map[string]any)["afterFileEdit"].([]any)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (existing + ours), got %d", len(entries))
	}
}

func TestMergePostCommitHook(t *testing.T) {
	out, changed := MergePostCommitHook("", GitCommand)
	if !changed {
		t.Fatal("expected changed=true for fresh hook")
	}
	if !strings.HasPrefix(out, "#!/bin/sh") {
		t.Errorf("fresh hook missing shebang: %q", out)
	}
	if !strings.Contains(out, GitCommand) {
		t.Errorf("fresh hook missing command: %q", out)
	}

	// Idempotent.
	out2, changed2 := MergePostCommitHook(out, GitCommand)
	if changed2 || out2 != out {
		t.Errorf("expected no-op on second run, got changed=%v", changed2)
	}
}

func TestMergePostCommitHookAppendsToExisting(t *testing.T) {
	existing := "#!/bin/bash\nmake lint"
	out, changed := MergePostCommitHook(existing, GitCommand)
	if !changed {
		t.Fatal("expected changed=true")
	}
	if !strings.Contains(out, "make lint") {
		t.Errorf("existing body dropped: %q", out)
	}
	if !strings.Contains(out, GitCommand) {
		t.Errorf("command not appended: %q", out)
	}
	if !strings.HasPrefix(out, "#!/bin/bash") {
		t.Errorf("existing shebang lost: %q", out)
	}
}

// claudeHasCommand reports whether a Claude settings blob carries command in a
// PostToolUse matcher.
func claudeHasCommand(t *testing.T, raw []byte, command string) bool {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	hooks, _ := m["hooks"].(map[string]any)
	matchers, _ := hooks["PostToolUse"].([]any)
	for _, mc := range matchers {
		entry, _ := mc.(map[string]any)
		inner, _ := entry["hooks"].([]any)
		for _, h := range inner {
			hm, _ := h.(map[string]any)
			if hm["command"] == command {
				return true
			}
		}
	}
	return false
}
