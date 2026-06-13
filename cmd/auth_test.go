package cmd

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/vdpeijl/clk/internal/clockify"
)

func TestSelectWorkspace(t *testing.T) {
	ws := []clockify.Workspace{
		{ID: "a", Name: "Acme"},
		{ID: "b", Name: "Beta"},
		{ID: "c", Name: "Gamma"},
	}

	tests := []struct {
		name       string
		workspaces []clockify.Workspace
		input      string
		wantID     string
		wantErr    bool
	}{
		{name: "none is an error", workspaces: nil, wantErr: true},
		{name: "single auto-selects", workspaces: ws[:1], wantID: "a"},
		{name: "many reads a choice", workspaces: ws, input: "2\n", wantID: "b"},
		{name: "choice is trimmed", workspaces: ws, input: "  3  \n", wantID: "c"},
		{name: "zero is out of range", workspaces: ws, input: "0\n", wantErr: true},
		{name: "too high is out of range", workspaces: ws, input: "4\n", wantErr: true},
		{name: "non-numeric is rejected", workspaces: ws, input: "x\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			in := bufio.NewReader(strings.NewReader(tt.input))
			got, err := selectWorkspace(&out, in, tt.workspaces)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got workspace %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ID != tt.wantID {
				t.Errorf("selected %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

func TestPromptAPIKey(t *testing.T) {
	var out bytes.Buffer
	in := bufio.NewReader(strings.NewReader("  my-key  \n"))
	got, err := promptAPIKey(&out, in)
	if err != nil {
		t.Fatalf("promptAPIKey: %v", err)
	}
	if got != "my-key" {
		t.Errorf("got %q, want %q", got, "my-key")
	}
	if !strings.Contains(out.String(), "API key") {
		t.Errorf("prompt missing, got %q", out.String())
	}
}
