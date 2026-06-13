package cmd

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/vdpeijl/clk/internal/clockify"
)

func projects() []clockify.Project {
	return []clockify.Project{
		{ID: "p1", Name: "Internal Tools"},
		{ID: "p2", Name: "Client Website"},
		{ID: "p3", Name: "clk"},
	}
}

func TestPickProject(t *testing.T) {
	tests := []struct {
		name    string
		input   string // filter line, then selection line
		wantID  string
		wantErr bool
	}{
		{name: "blank filter then pick first", input: "\n1\n", wantID: "p1"},
		{name: "filter narrows then pick", input: "client\n1\n", wantID: "p2"},
		{name: "filter to exact then pick", input: "clk\n1\n", wantID: "p3"},
		{name: "no matches errors", input: "zzz\n", wantErr: true},
		{name: "out of range errors", input: "\n9\n", wantErr: true},
		{name: "non-numeric errors", input: "\nx\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			in := bufio.NewReader(strings.NewReader(tt.input))
			got, err := pickProject(&out, in, projects())
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ID != tt.wantID {
				t.Errorf("picked %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

func TestPickProjectEmptyList(t *testing.T) {
	var out bytes.Buffer
	in := bufio.NewReader(strings.NewReader("\n1\n"))
	if _, err := pickProject(&out, in, nil); err == nil {
		t.Fatal("expected error for empty project list")
	}
}

func TestResolveProject(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		wantID  string
		wantErr bool
	}{
		{name: "exact id", arg: "p2", wantID: "p2"},
		{name: "exact name case-insensitive", arg: "internal tools", wantID: "p1"},
		{name: "unambiguous fuzzy", arg: "website", wantID: "p2"},
		{name: "no match errors", arg: "zzzzz", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveProject(projects(), tt.arg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ID != tt.wantID {
				t.Errorf("resolved %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

func TestResolveTask(t *testing.T) {
	tasks := []clockify.Task{
		{ID: "t1", Name: "Build"},
		{ID: "t2", Name: "Review"},
	}
	got, err := resolveTask(tasks, "review")
	if err != nil {
		t.Fatalf("resolveTask: %v", err)
	}
	if got.ID != "t2" {
		t.Errorf("resolved %q, want t2", got.ID)
	}
	if _, err := resolveTask(tasks, "deploy"); err == nil {
		t.Error("expected error for unknown task")
	}
}

func TestLinkArgs(t *testing.T) {
	token, project, err := linkArgs([]string{"my-token", "my-project"})
	if err != nil {
		t.Fatalf("linkArgs: %v", err)
	}
	if token != "my-token" || project != "my-project" {
		t.Errorf("got token=%q project=%q, want explicit pair", token, project)
	}
}
