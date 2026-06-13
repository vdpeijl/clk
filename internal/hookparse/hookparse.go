// Package hookparse contains pure per-source parsers that convert raw hook
// payloads into Event values.
package hookparse

import (
	"encoding/json"
	"fmt"
	"time"

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
func Parse(raw json.RawMessage, source Source) (sessions.Event, error) {
	switch source {
	case SourceClaudeCode:
		return parseClaudeCode(raw)
	default:
		return sessions.Event{}, fmt.Errorf("unknown source: %s", source)
	}
}

func parseClaudeCode(raw json.RawMessage) (sessions.Event, error) {
	// TODO: implement Claude Code PostToolUse payload parsing
	return sessions.Event{
		Timestamp: time.Now(),
		Source:    string(SourceClaudeCode),
	}, nil
}
