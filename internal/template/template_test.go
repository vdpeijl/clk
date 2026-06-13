package template

import (
	"testing"

	"github.com/vdpeijl/clk/internal/sessions"
)

func TestExpand(t *testing.T) {
	full := sessions.Session{
		IssueID:     "PROJ-123",
		Branch:      "feature/push",
		Description: "wire up push pipeline",
		Files:       []string{"cmd/push.go", "internal/pushplanner/pushplanner.go"},
	}

	tests := []struct {
		name    string
		session sessions.Session
		tmpl    string
		want    string
	}{
		{
			name:    "all placeholders expand",
			session: full,
			tmpl:    "{issue} {branch}: {summary} [{files}]",
			want:    "PROJ-123 feature/push: wire up push pipeline [cmd/push.go, internal/pushplanner/pushplanner.go]",
		},
		{
			name:    "missing issue expands to empty",
			session: sessions.Session{Branch: "main", Description: "tidy"},
			tmpl:    "{issue} {branch}: {summary}",
			want:    " main: tidy",
		},
		{
			name:    "missing branch expands to empty",
			session: sessions.Session{IssueID: "PROJ-1", Description: "tidy"},
			tmpl:    "{issue} {branch}: {summary}",
			want:    "PROJ-1 : tidy",
		},
		{
			name:    "missing files expands to empty",
			session: sessions.Session{Description: "no files touched"},
			tmpl:    "{summary} ({files})",
			want:    "no files touched ()",
		},
		{
			name:    "empty session and template stays empty",
			session: sessions.Session{},
			tmpl:    "",
			want:    "",
		},
		{
			name:    "literal text without placeholders is preserved",
			session: full,
			tmpl:    "manual entry",
			want:    "manual entry",
		},
		{
			name:    "single file renders without separator",
			session: sessions.Session{Files: []string{"main.go"}},
			tmpl:    "{files}",
			want:    "main.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Expand(tt.session, tt.tmpl); got != tt.want {
				t.Errorf("Expand() = %q, want %q", got, tt.want)
			}
		})
	}
}
