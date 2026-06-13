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
	default:
		return sessions.Event{}, fmt.Errorf("unknown source: %s", source)
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

// CWD extracts the working directory reported by a Claude Code payload. It
// returns an empty string if the field is absent or the payload is invalid.
func CWD(raw json.RawMessage) string {
	var p claudeCodePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return ""
	}
	return p.CWD
}
