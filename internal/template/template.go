// Package template expands description templates for time entries. It is pure.
package template

import (
	"strings"

	"github.com/vdpeijl/clk/internal/sessions"
)

// Expand replaces placeholders in tmpl with values from the session.
// Supported placeholders: {issue}, {branch}, {summary}, {files}. A placeholder
// whose source field is empty (e.g. a session with no issue id) expands to the
// empty string rather than being left literal.
func Expand(s sessions.Session, tmpl string) string {
	r := strings.NewReplacer(
		"{issue}", s.IssueID,
		"{branch}", s.Branch,
		"{summary}", s.Description,
		"{files}", strings.Join(s.Files, ", "),
	)
	return r.Replace(tmpl)
}
