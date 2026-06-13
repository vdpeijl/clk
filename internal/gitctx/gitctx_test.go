package gitctx

import "testing"

func TestIssueIDFromBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{"PROJ-123", "PROJ-123"},
		{"feature/PROJ-123-add-thing", "PROJ-123"},
		{"bugfix/ABC-9", "ABC-9"},
		{"PROJECT-4567/some-work", "PROJECT-4567"},
		{"main", ""},
		{"feature/no-issue-here", ""},
		{"", ""},
		{"proj-123", ""}, // lowercase keys are not issue ids
	}
	for _, tt := range tests {
		if got := IssueIDFromBranch(tt.branch); got != tt.want {
			t.Errorf("IssueIDFromBranch(%q) = %q, want %q", tt.branch, got, tt.want)
		}
	}
}

func TestDetectEmptyDir(t *testing.T) {
	got := Detect("")
	if got != (Context{}) {
		t.Errorf("Detect(\"\") = %+v, want zero", got)
	}
}

func TestDetectNonRepoUsesDirName(t *testing.T) {
	dir := t.TempDir()
	got := Detect(dir)
	if got.Branch != "" {
		t.Errorf("branch = %q, want empty outside a repo", got.Branch)
	}
	if got.ProjectToken == "" {
		t.Errorf("project token should fall back to the directory base name")
	}
}
