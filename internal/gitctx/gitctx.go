// Package gitctx detects repository context — project token, git branch, and
// issue id — from a working directory. The filesystem/git probing is isolated
// in Detect; the issue-id extraction logic is pure and independently testable.
package gitctx

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Context is the repository context attached to a captured event.
type Context struct {
	// ProjectToken identifies the project, derived from the git repo root
	// (its base name) or, outside a repo, the directory base name.
	ProjectToken string
	// Branch is the current git branch, empty outside a repo.
	Branch string
	// IssueID is a PROJ-123-style id parsed from the branch, if present.
	IssueID string
}

// issueIDRe matches PROJ-123-style issue ids: uppercase letters, a hyphen, and
// digits.
var issueIDRe = regexp.MustCompile(`[A-Z][A-Z0-9]+-[0-9]+`)

// Detect inspects dir and returns its repository context. It never errors:
// fields that cannot be determined are left empty.
func Detect(dir string) Context {
	if dir == "" {
		return Context{}
	}

	ctx := Context{ProjectToken: filepath.Base(filepath.Clean(dir))}

	if root := gitOutput(dir, "rev-parse", "--show-toplevel"); root != "" {
		ctx.ProjectToken = filepath.Base(root)
	}
	if branch := gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD"); branch != "" && branch != "HEAD" {
		ctx.Branch = branch
		ctx.IssueID = IssueIDFromBranch(branch)
	}
	return ctx
}

// Root returns the absolute path to the git repository root containing dir, or
// an empty string when dir is not inside a repository.
func Root(dir string) string {
	if dir == "" {
		return ""
	}
	return gitOutput(dir, "rev-parse", "--show-toplevel")
}

// IssueIDFromBranch extracts a PROJ-123-style issue id from a branch name,
// returning an empty string when none is present. It is pure.
func IssueIDFromBranch(branch string) string {
	return issueIDRe.FindString(branch)
}

// Commit is the subset of a git commit captured as an event.
type Commit struct {
	SHA     string
	Subject string
}

// LastCommit returns the most recent commit in the repository containing dir.
// Both fields are empty when dir is not a repository or has no commits yet.
func LastCommit(dir string) Commit {
	return Commit{
		SHA:     gitOutput(dir, "rev-parse", "HEAD"),
		Subject: gitOutput(dir, "log", "-1", "--pretty=%s"),
	}
}

// gitOutput runs git in dir and returns trimmed stdout, or "" on any failure.
func gitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
