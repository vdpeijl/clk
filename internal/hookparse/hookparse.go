// Package hookparse contains pure per-source parsers that convert raw hook
// payloads into Event values. Parsers map only what the payload itself carries;
// environmental context (timestamp, project, git branch, issue id) is attached
// by the caller.
package hookparse

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vdpeijl/clk/internal/sessions"
)

// Source identifies the hook origin.
type Source string

const (
	SourceClaudeCode Source = "claude_code"
	SourceCursor     Source = "cursor"
	SourceCopilot    Source = "copilot"
	SourceGit        Source = "git"
	SourceFileWatch  Source = "filewatch"
)

// Parse converts a raw JSON hook payload from the given source into an Event.
// The returned Event has no Timestamp or project/git context set; the caller is
// responsible for stamping those.
func Parse(raw json.RawMessage, source Source) (sessions.Event, error) {
	switch source {
	case SourceClaudeCode:
		return parseClaudeCode(raw)
	case SourceCursor:
		return parseCursor(raw)
	case SourceCopilot:
		return parseCopilot(raw)
	default:
		return sessions.Event{}, fmt.Errorf("unknown source: %s", source)
	}
}

// CWD extracts the working directory a payload reports, used by the caller to
// detect git context. It returns an empty string when the field is absent, the
// source has no notion of a working directory, or the payload is invalid.
func CWD(raw json.RawMessage, source Source) string {
	switch source {
	case SourceClaudeCode:
		var p claudeCodePayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return ""
		}
		return p.CWD
	case SourceCursor:
		var p cursorPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return ""
		}
		if len(p.WorkspaceRoots) > 0 {
			return p.WorkspaceRoots[0]
		}
		return ""
	case SourceCopilot:
		var p copilotPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return ""
		}
		return p.CWD
	default:
		return ""
	}
}

// claudeCodePayload mirrors the fields of a Claude Code PostToolUse hook
// payload that are relevant to capture.
type claudeCodePayload struct {
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	CWD           string          `json:"cwd"`
	ToolInput     json.RawMessage `json:"tool_input"`
}

// cursorPayload mirrors the fields of a Cursor agent hook payload. Cursor
// delivers different events (afterFileEdit, beforeShellExecution, …); only the
// fields common across them and useful for capture are mapped here.
type cursorPayload struct {
	HookEventName  string   `json:"hook_event_name"`
	WorkspaceRoots []string `json:"workspace_roots"`
	FilePath       string   `json:"file_path"`
	Command        string   `json:"command"`
}

// copilotPayload mirrors the fields of a Copilot CLI hook payload. It follows
// the same tool/arguments shape as Claude Code, so the path-extraction logic is
// shared.
type copilotPayload struct {
	HookEventName string          `json:"hook_event_name"`
	Tool          string          `json:"tool"`
	CWD           string          `json:"cwd"`
	Arguments     json.RawMessage `json:"arguments"`
}

// toolInputPaths captures the path-like fields a tool may report.
type toolInputPaths struct {
	FilePath     string `json:"file_path"`
	Path         string `json:"path"`
	NotebookPath string `json:"notebook_path"`
}

func parseClaudeCode(raw json.RawMessage) (sessions.Event, error) {
	var p claudeCodePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return sessions.Event{}, fmt.Errorf("parse claude code payload: %w", err)
	}
	if p.ToolName == "" {
		return sessions.Event{}, fmt.Errorf("claude code payload missing tool_name")
	}

	filePath := extractFilePath(p.ToolInput)

	return sessions.Event{
		Type:        "tool_use",
		Source:      string(SourceClaudeCode),
		Description: describe(p.ToolName, filePath),
		FilePath:    filePath,
	}, nil
}

func parseCursor(raw json.RawMessage) (sessions.Event, error) {
	var p cursorPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return sessions.Event{}, fmt.Errorf("parse cursor payload: %w", err)
	}
	if p.HookEventName == "" {
		return sessions.Event{}, fmt.Errorf("cursor payload missing hook_event_name")
	}

	// A file edit names a path; a shell execution names a command. Prefer the
	// path when both are present.
	description := p.HookEventName
	switch {
	case p.FilePath != "":
		description = describe(p.HookEventName, p.FilePath)
	case p.Command != "":
		description = describe(p.HookEventName, p.Command)
	}

	return sessions.Event{
		Type:        "tool_use",
		Source:      string(SourceCursor),
		Description: description,
		FilePath:    p.FilePath,
	}, nil
}

func parseCopilot(raw json.RawMessage) (sessions.Event, error) {
	var p copilotPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return sessions.Event{}, fmt.Errorf("parse copilot payload: %w", err)
	}
	if p.Tool == "" {
		return sessions.Event{}, fmt.Errorf("copilot payload missing tool")
	}

	filePath := extractFilePath(p.Arguments)

	return sessions.Event{
		Type:        "tool_use",
		Source:      string(SourceCopilot),
		Description: describe(p.Tool, filePath),
		FilePath:    filePath,
	}, nil
}

// GitEvent builds the event for a captured commit. It is pure: the caller
// gathers the commit subject and sha (and the surrounding git context) and
// passes them in. An empty subject falls back to the short sha so the entry is
// never blank.
func GitEvent(subject, sha string) sessions.Event {
	return sessions.Event{
		Type:        "git_commit",
		Source:      string(SourceGit),
		Description: commitDescription(subject, sha),
	}
}

// commitDescription renders a commit subject, falling back to "commit <short
// sha>" and finally a bare "commit" when neither is available.
func commitDescription(subject, sha string) string {
	if s := strings.TrimSpace(subject); s != "" {
		return s
	}
	if short := shortSHA(sha); short != "" {
		return "commit " + short
	}
	return "commit"
}

// shortSHA abbreviates a commit sha to its first seven characters.
func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// extractFilePath pulls a path-like field out of a tool_input object, if any.
func extractFilePath(toolInput json.RawMessage) string {
	if len(toolInput) == 0 {
		return ""
	}
	var paths toolInputPaths
	if err := json.Unmarshal(toolInput, &paths); err != nil {
		return ""
	}
	switch {
	case paths.FilePath != "":
		return paths.FilePath
	case paths.NotebookPath != "":
		return paths.NotebookPath
	default:
		return paths.Path
	}
}

// describe builds a short human-readable description of a tool invocation.
func describe(toolName, filePath string) string {
	if filePath == "" {
		return toolName
	}
	return strings.TrimSpace(toolName + " " + filePath)
}
