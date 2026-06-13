// Package capture contains the background daemon, hook ingestion, and
// file-watch heartbeat logic. It is I/O-heavy and kept intentionally thin —
// all business logic lives in the pure packages it calls.
package capture

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/vdpeijl/clk/internal/sessions"
	"github.com/vdpeijl/clk/internal/store"
)

// Default daemon timing. Events are flushed to the store frequently so `clk
// list` sees recent activity quickly; the merge loop re-materializes less
// often to coalesce sessions that have grown adjacent over time.
const (
	defaultFlushInterval     = 2 * time.Second
	defaultMergeInterval     = 60 * time.Second
	defaultMaterializeWindow = 30 * 24 * time.Hour
)

// Daemon manages the background capture process.
type Daemon struct {
	socketPath string
	dbPath     string
	logPath    string
	pidPath    string

	// FlushInterval controls how often buffered events are written to the
	// store and materialized into sessions.
	FlushInterval time.Duration
	// MergeInterval controls how often the adjacency-merge loop re-materializes
	// sessions independently of new events.
	MergeInterval time.Duration
	// MaterializeWindow is how far back reconstruction reaches on each pass.
	MaterializeWindow time.Duration

	mu          sync.Mutex
	buffer      []sessions.Event
	eventsTotal int64
	startedAt   time.Time

	logger *log.Logger
}

// New creates a Daemon configured to listen on socketPath and write to dbPath.
func New(socketPath, dbPath, logPath, pidPath string) *Daemon {
	return &Daemon{
		socketPath:        socketPath,
		dbPath:            dbPath,
		logPath:           logPath,
		pidPath:           pidPath,
		FlushInterval:     defaultFlushInterval,
		MergeInterval:     defaultMergeInterval,
		MaterializeWindow: defaultMaterializeWindow,
	}
}

// Run starts the daemon and blocks until it receives SIGTERM or SIGINT, at
// which point it flushes any buffered events and shuts down gracefully.
func (d *Daemon) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	return d.run(ctx)
}

// run is the daemon's main loop, shutting down when ctx is cancelled. It is
// separated from Run so tests can drive the lifecycle without OS signals.
func (d *Daemon) run(ctx context.Context) error {
	logFile, err := os.OpenFile(d.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log %s: %w", d.logPath, err)
	}
	defer logFile.Close()
	d.logger = log.New(logFile, "", log.LstdFlags|log.Lmicroseconds)

	if d.alreadyRunning() {
		return fmt.Errorf("daemon already listening on %s", d.socketPath)
	}
	// Clear any socket left behind by a crashed predecessor.
	if err := os.Remove(d.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", d.socketPath, err)
	}
	defer ln.Close()

	if err := os.WriteFile(d.pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	defer os.Remove(d.pidPath)
	defer os.Remove(d.socketPath)

	st, err := store.Open(d.dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	d.startedAt = time.Now()
	d.logger.Printf("daemon started (pid %d), listening on %s", os.Getpid(), d.socketPath)

	// Accept connections in the background; the worker loop owns all store I/O.
	go d.acceptLoop(ln)

	flushT := time.NewTicker(d.FlushInterval)
	defer flushT.Stop()
	mergeT := time.NewTicker(d.MergeInterval)
	defer mergeT.Stop()

	for {
		select {
		case <-flushT.C:
			d.flush(st)
		case <-mergeT.C:
			// Re-materialize even without new events so sessions that have
			// become adjacent within the gap window are merged over time.
			d.materialize(st)
		case <-ctx.Done():
			d.logger.Printf("shutting down")
			ln.Close()
			d.flush(st) // drain remaining buffer — no data loss on shutdown
			return nil
		}
	}
}

// alreadyRunning reports whether a healthy daemon is already answering on the
// socket, so a second instance refuses to start rather than stealing it.
func (d *Daemon) alreadyRunning() bool {
	return Ping(d.socketPath) == nil
}

// acceptLoop hands each incoming connection to a handler goroutine until the
// listener is closed.
func (d *Daemon) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Closed listener (shutdown) or transient error: stop accepting.
			return
		}
		go d.handle(conn)
	}
}

// handle reads a single request from conn, applies it, and writes one response.
func (d *Daemon) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return
	}

	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		d.reply(conn, response{OK: false, Error: "invalid request"})
		return
	}

	switch req.Cmd {
	case cmdEvent:
		if req.Event == nil {
			d.reply(conn, response{OK: false, Error: "missing event"})
			return
		}
		d.enqueue(req.Event.toSessionEvent())
		d.reply(conn, response{OK: true})
	case cmdStatus:
		s := d.snapshot()
		d.reply(conn, response{OK: true, Status: &s})
	case cmdPing:
		d.reply(conn, response{OK: true})
	default:
		d.reply(conn, response{OK: false, Error: "unknown command: " + req.Cmd})
	}
}

// reply writes a newline-delimited JSON response, logging write failures.
func (d *Daemon) reply(conn net.Conn, resp response) {
	b, err := json.Marshal(resp)
	if err != nil {
		d.logger.Printf("marshal response: %v", err)
		return
	}
	if _, err := conn.Write(append(b, '\n')); err != nil {
		d.logger.Printf("write response: %v", err)
	}
}

// enqueue buffers an event for the next flush.
func (d *Daemon) enqueue(e sessions.Event) {
	d.mu.Lock()
	d.buffer = append(d.buffer, e)
	d.eventsTotal++
	d.mu.Unlock()
}

// snapshot returns the current runtime status.
func (d *Daemon) snapshot() Status {
	d.mu.Lock()
	buffered := len(d.buffer)
	total := d.eventsTotal
	d.mu.Unlock()
	return Status{
		PID:           os.Getpid(),
		Buffered:      buffered,
		EventsTotal:   total,
		UptimeSeconds: int64(time.Since(d.startedAt).Seconds()),
	}
}

// flush drains the buffer into the store and re-materializes sessions.
func (d *Daemon) flush(st *store.Store) {
	d.mu.Lock()
	batch := d.buffer
	d.buffer = nil
	d.mu.Unlock()

	if len(batch) == 0 {
		return
	}

	for _, e := range batch {
		if _, err := st.InsertEvent(e); err != nil {
			d.logger.Printf("insert event: %v", err)
		}
	}
	d.logger.Printf("flushed %d event(s)", len(batch))
	d.materialize(st)
}

// materialize reconstructs sessions for the rolling window and writes them to
// the store. It is idempotent: running it repeatedly over unchanged events
// yields the same sessions, which is what makes the adjacency-merge loop safe.
func (d *Daemon) materialize(st *store.Store) {
	end := time.Now().Add(time.Second)
	start := end.Add(-d.MaterializeWindow)

	events, err := st.EventsBetween(start, end)
	if err != nil {
		d.logger.Printf("read events: %v", err)
		return
	}
	ss := sessions.Reconstruct(events)
	if err := st.ReplaceSessionsBetween(start, end, ss); err != nil {
		d.logger.Printf("materialize sessions: %v", err)
	}
}
