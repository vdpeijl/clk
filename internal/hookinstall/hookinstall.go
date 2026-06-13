// Package hookinstall holds the pure logic behind `clk init`: deciding which dev
// tools are in use, and computing the hook-configuration file contents that wire
// those tools into clk. All functions here are pure — they take the existing
// file bytes and return the new bytes — so the merge and idempotency rules can
// be unit-tested without touching the filesystem. The cmd layer owns the actual
// detection probes and file writes.
package hookinstall

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Tool identifies a dev tool clk can capture from.
type Tool string

const (
	ToolClaudeCode Tool = "claude_code"
	ToolCursor     Tool = "cursor"
	ToolCopilot    Tool = "copilot"
	ToolCodex      Tool = "codex"
	ToolGit        Tool = "git"
)

// Commands clk installs into each tool's hook configuration. They reference the
// binary by name (not an absolute path) so committed configs work for every
// teammate who clones the repo.
const (
	ClaudeCommand  = "clk hook claude-code"
	CursorCommand  = "clk hook cursor"
	CopilotCommand = "clk hook copilot"
	CodexCommand   = "clk hook codex"
	GitCommand     = "clk hook git"
)

// Detection describes what clk observed about the environment: which tool config
// directories exist in the repo and which tool binaries are on PATH.
type Detection struct {
	ClaudeDir  bool
	ClaudeBin  bool
	CursorDir  bool
	CursorBin  bool
	CopilotDir bool
	CopilotBin bool
	CodexDir   bool
	CodexBin   bool
	GitRepo    bool
}

// Detect returns the tools to install hooks for, in a stable order. A tool is
// selected when either its repo config directory exists or its binary is on
// PATH; git is selected whenever the working directory is a repository.
func Detect(d Detection) []Tool {
	var tools []Tool
	if d.ClaudeDir || d.ClaudeBin {
		tools = append(tools, ToolClaudeCode)
	}
	if d.CursorDir || d.CursorBin {
		tools = append(tools, ToolCursor)
	}
	if d.CopilotDir || d.CopilotBin {
		tools = append(tools, ToolCopilot)
	}
	if d.CodexDir || d.CodexBin {
		tools = append(tools, ToolCodex)
	}
	if d.GitRepo {
		tools = append(tools, ToolGit)
	}
	return tools
}

// MergeClaudeSettings installs command as a PostToolUse hook in a Claude Code
// settings.json, preserving every other key. It is idempotent: if the command
// is already registered, the bytes are returned unchanged with changed=false.
func MergeClaudeSettings(existing []byte, command string) (result []byte, changed bool, err error) {
	root, err := decodeObject(existing)
	if err != nil {
		return nil, false, err
	}

	hooks := childObject(root, "hooks")
	matchers := toArray(hooks["PostToolUse"])

	if commandPresentInMatchers(matchers, command) {
		return existing, false, nil
	}

	matchers = append(matchers, map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{"type": "command", "command": command},
		},
	})
	hooks["PostToolUse"] = matchers
	root["hooks"] = hooks

	out, err := encodeObject(root)
	return out, true, err
}

// MergeEventHooks installs command under each named event in a tool config whose
// shape is {"hooks": {"<event>": [{"command": "..."}]}}. Both the Cursor and
// Copilot configs share this shape. It is idempotent per event.
func MergeEventHooks(existing []byte, events []string, command string) (result []byte, changed bool, err error) {
	root, err := decodeObject(existing)
	if err != nil {
		return nil, false, err
	}

	hooks := childObject(root, "hooks")
	for _, event := range events {
		entries := toArray(hooks[event])
		if commandPresentInEntries(entries, command) {
			continue
		}
		entries = append(entries, map[string]any{"command": command})
		hooks[event] = entries
		changed = true
	}
	if !changed {
		return existing, false, nil
	}
	root["hooks"] = hooks

	out, err := encodeObject(root)
	return out, true, err
}

// MergePostCommitHook installs the clk capture invocation into a git
// post-commit hook script. A fresh hook gets a shebang and the invocation; an
// existing unrelated hook keeps its body and gains the invocation appended. It
// is idempotent: an already-wired hook is returned unchanged.
func MergePostCommitHook(existing, command string) (result string, changed bool) {
	if strings.Contains(existing, command) {
		return existing, false
	}
	if strings.TrimSpace(existing) == "" {
		return "#!/bin/sh\n# Installed by clk — captures each commit as a dev-activity event.\n" + command + "\n", true
	}
	suffix := "\n# Installed by clk — captures each commit as a dev-activity event.\n" + command + "\n"
	if !strings.HasSuffix(existing, "\n") {
		suffix = "\n" + suffix
	}
	return existing + suffix, true
}

// decodeObject parses raw JSON into a map, treating empty/blank input as a fresh
// object so callers need not special-case missing files.
func decodeObject(raw []byte) (map[string]any, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse settings: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// encodeObject renders an object as 2-space-indented JSON with a trailing
// newline, matching the formatting these tools use.
func encodeObject(m map[string]any) ([]byte, error) {
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode settings: %w", err)
	}
	return append(out, '\n'), nil
}

// childObject returns the nested object at key, creating an empty one when the
// key is absent or holds a non-object value.
func childObject(parent map[string]any, key string) map[string]any {
	if child, ok := parent[key].(map[string]any); ok {
		return child
	}
	return map[string]any{}
}

// toArray coerces a JSON value into a slice, returning an empty slice for
// missing or non-array values.
func toArray(v any) []any {
	if arr, ok := v.([]any); ok {
		return arr
	}
	return nil
}

// commandPresentInMatchers reports whether command already appears in a Claude
// PostToolUse matcher list (matcher -> hooks[] -> command).
func commandPresentInMatchers(matchers []any, command string) bool {
	for _, m := range matchers {
		entry, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if commandPresentInEntries(toArray(entry["hooks"]), command) {
			return true
		}
	}
	return false
}

// commandPresentInEntries reports whether any {"command": ...} entry matches.
func commandPresentInEntries(entries []any, command string) bool {
	for _, e := range entries {
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if cmd, _ := entry["command"].(string); cmd == command {
			return true
		}
	}
	return false
}
