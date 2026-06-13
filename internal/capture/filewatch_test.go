package capture

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vdpeijl/clk/internal/sessions"
)

func TestThrottleAllow(t *testing.T) {
	th := newThrottle(time.Minute)
	base := time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC)

	if !th.allow("a", base) {
		t.Fatal("first event for a key should be allowed")
	}
	if th.allow("a", base.Add(30*time.Second)) {
		t.Error("second event within the interval should be throttled")
	}
	if !th.allow("a", base.Add(90*time.Second)) {
		t.Error("event after the interval should be allowed")
	}
	// A different key is gated independently.
	if !th.allow("b", base.Add(30*time.Second)) {
		t.Error("a distinct key should not be throttled by another key")
	}
}

func TestIgnoreDir(t *testing.T) {
	for _, name := range []string{".git", "node_modules", ".clk", "vendor"} {
		if !ignoreDir(name) {
			t.Errorf("ignoreDir(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"internal", "cmd", "src"} {
		if ignoreDir(name) {
			t.Errorf("ignoreDir(%q) = true, want false", name)
		}
	}
}

func TestIgnoreFile(t *testing.T) {
	for _, name := range []string{"main.go~", "store.go.swp", ".#main.go", "build.tmp"} {
		if !ignoreFile(name) {
			t.Errorf("ignoreFile(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"main.go", "README.md", ".clk.toml"} {
		if ignoreFile(name) {
			t.Errorf("ignoreFile(%q) = true, want false", name)
		}
	}
}

func TestProjectFor(t *testing.T) {
	projects := []watchProject{
		{root: "/home/dev/clk", token: "clk"},
		{root: "/home/dev/clk/vendored", token: "vendored"},
		{root: "/home/dev/other", token: "other"},
	}
	tests := []struct {
		path      string
		wantToken string
		wantOK    bool
	}{
		{path: "/home/dev/clk/internal/store/store.go", wantToken: "clk", wantOK: true},
		{path: "/home/dev/clk/vendored/pkg/x.go", wantToken: "vendored", wantOK: true}, // longest prefix wins
		{path: "/home/dev/clk", wantToken: "clk", wantOK: true},
		{path: "/home/dev/unrelated/x.go", wantOK: false},
		{path: "/home/dev/clksibling/x.go", wantOK: false}, // not a path-segment prefix
	}
	for _, tt := range tests {
		got, ok := projectFor(tt.path, projects)
		if ok != tt.wantOK {
			t.Errorf("projectFor(%q) ok = %v, want %v", tt.path, ok, tt.wantOK)
			continue
		}
		if ok && got.token != tt.wantToken {
			t.Errorf("projectFor(%q) token = %q, want %q", tt.path, got.token, tt.wantToken)
		}
	}
}

func TestFileWatcherEmitsHeartbeatOnChange(t *testing.T) {
	root := t.TempDir()

	var (
		mu     sync.Mutex
		events []sessions.Event
	)
	emit := func(e sessions.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	logger := log.New(io.Discard, "", 0)
	w, err := newFileWatcher([]watchProject{{root: root, token: "clk"}}, 10*time.Millisecond, emit, logger)
	if err != nil {
		t.Fatalf("newFileWatcher: %v", err)
	}
	defer w.Close()
	go w.run()

	// Give the watcher a moment to be ready, then edit a file.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		n := len(events)
		mu.Unlock()
		if n > 0 {
			break
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Fatal("expected at least one heartbeat event")
	}
	got := events[0]
	if got.Source != "filewatch" {
		t.Errorf("source = %q, want filewatch", got.Source)
	}
	if got.ProjectToken != "clk" {
		t.Errorf("project token = %q, want clk", got.ProjectToken)
	}
	if got.Type != "file_change" {
		t.Errorf("type = %q, want file_change", got.Type)
	}
}
