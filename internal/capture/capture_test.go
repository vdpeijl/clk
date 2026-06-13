package capture

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vdpeijl/clk/internal/sessions"
	"github.com/vdpeijl/clk/internal/store"
)

// newTestDaemon builds a daemon on temp paths tuned for fast tests, and starts
// it in the background. It returns the daemon, its paths, and a stop func that
// shuts it down and waits for a clean exit.
func newTestDaemon(t *testing.T) (*Daemon, daemonPaths, func()) {
	t.Helper()
	dir := t.TempDir()
	p := daemonPaths{
		socket: filepath.Join(dir, "daemon.sock"),
		pid:    filepath.Join(dir, "daemon.pid"),
		log:    filepath.Join(dir, "daemon.log"),
		db:     filepath.Join(dir, "state.db"),
	}
	d := New(p.socket, p.db, p.log, p.pid)
	d.FlushInterval = 20 * time.Millisecond
	d.MergeInterval = time.Hour // keep the merge loop out of the way in tests

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- d.run(ctx) }()

	if !waitFor(t, func() bool { return Ping(p.socket) == nil }) {
		cancel()
		t.Fatal("daemon did not come up")
	}

	stop := func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("daemon exited with error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("daemon did not shut down in time")
		}
	}
	return d, p, stop
}

// daemonPaths mirrors the cmd-layer bundle for test convenience.
type daemonPaths struct {
	socket, pid, log, db string
}

// waitFor polls cond up to ~2s, returning whether it became true.
func waitFor(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

func sampleEvent(min int) Event {
	base := time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC)
	return Event{
		Timestamp:    base.Add(time.Duration(min) * time.Minute).Unix(),
		Type:         "tool_use",
		Source:       "claude_code",
		ProjectToken: "clk",
		Branch:       "main",
	}
}

func TestDaemonIngestsBatchesAndMaterializes(t *testing.T) {
	_, p, stop := newTestDaemon(t)
	defer stop()

	for _, min := range []int{0, 10, 20} {
		if err := SendEvent(p.socket, sampleEvent(min)); err != nil {
			t.Fatalf("SendEvent: %v", err)
		}
	}

	// Status reports the events the daemon has accepted.
	st, err := GetStatus(p.socket)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.EventsTotal != 3 {
		t.Errorf("EventsTotal = %d, want 3", st.EventsTotal)
	}
	if st.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", st.PID, os.Getpid())
	}

	// After a flush, the events are materialized into a single session
	// (all within the 25-minute gap window).
	var got []sessions.Session
	ok := waitFor(t, func() bool {
		s, oerr := store.Open(p.db)
		if oerr != nil {
			return false
		}
		defer s.Close()
		day := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
		got, _ = s.SessionsBetween(day, day.Add(24*time.Hour))
		return len(got) == 1 && got[0].EventCount == 3
	})
	if !ok {
		t.Fatalf("expected 1 materialized session with 3 events, got %+v", got)
	}
}

func TestDaemonFlushesBufferOnShutdown(t *testing.T) {
	dir := t.TempDir()
	p := daemonPaths{
		socket: filepath.Join(dir, "daemon.sock"),
		pid:    filepath.Join(dir, "daemon.pid"),
		log:    filepath.Join(dir, "daemon.log"),
		db:     filepath.Join(dir, "state.db"),
	}
	d := New(p.socket, p.db, p.log, p.pid)
	d.FlushInterval = time.Hour // never flush on the timer — only on shutdown
	d.MergeInterval = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- d.run(ctx) }()
	if !waitFor(t, func() bool { return Ping(p.socket) == nil }) {
		cancel()
		t.Fatal("daemon did not come up")
	}

	if err := SendEvent(p.socket, sampleEvent(0)); err != nil {
		t.Fatalf("SendEvent: %v", err)
	}

	// Shut down before the (1h) flush timer fires; graceful shutdown must drain.
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("daemon exited with error: %v", err)
	}

	s, err := store.Open(p.db)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	day := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	got, err := s.SessionsBetween(day, day.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("SessionsBetween: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("buffer not flushed on shutdown: got %d sessions, want 1", len(got))
	}
}

func TestDaemonRemovesSocketAndPidOnShutdown(t *testing.T) {
	_, p, stop := newTestDaemon(t)
	stop()

	if _, err := os.Stat(p.socket); !os.IsNotExist(err) {
		t.Errorf("socket not removed on shutdown: %v", err)
	}
	if _, err := os.Stat(p.pid); !os.IsNotExist(err) {
		t.Errorf("pid file not removed on shutdown: %v", err)
	}
}

func TestSecondDaemonRefusesToStart(t *testing.T) {
	_, p, stop := newTestDaemon(t)
	defer stop()

	other := New(p.socket, p.db, p.log, p.pid)
	if err := other.run(context.Background()); err == nil {
		t.Fatal("expected second daemon on same socket to refuse to start")
	}
}

func TestClientErrorsWhenDaemonDown(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "daemon.sock")
	if err := Ping(socket); err == nil {
		t.Error("Ping should fail when no daemon is listening")
	}
	if err := SendEvent(socket, sampleEvent(0)); err == nil {
		t.Error("SendEvent should fail when no daemon is listening")
	}
}

func TestIsRunningWithoutPidFile(t *testing.T) {
	if _, ok := IsRunning(filepath.Join(t.TempDir(), "daemon.pid")); ok {
		t.Error("IsRunning should be false with no pid file")
	}
}

func TestStopWithoutDaemon(t *testing.T) {
	if err := Stop(filepath.Join(t.TempDir(), "daemon.pid"), time.Second); err != ErrNotRunning {
		t.Errorf("Stop without daemon = %v, want ErrNotRunning", err)
	}
}
