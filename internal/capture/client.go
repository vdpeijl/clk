package capture

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ErrNotRunning is returned by lifecycle operations when no daemon is running.
var ErrNotRunning = errors.New("daemon is not running")

// ErrAlreadyRunning is returned by Start when a daemon is already running.
var ErrAlreadyRunning = errors.New("daemon is already running")

// dialTimeout bounds how long a client waits to connect to the socket.
const dialTimeout = 2 * time.Second

// roundtrip dials the daemon, sends one request, and returns its response.
func roundtrip(socketPath string, req request) (response, error) {
	conn, err := net.DialTimeout("unix", socketPath, dialTimeout)
	if err != nil {
		return response{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(dialTimeout))

	b, err := json.Marshal(req)
	if err != nil {
		return response{}, err
	}
	if _, err := conn.Write(append(b, '\n')); err != nil {
		return response{}, err
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return response{}, err
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		return response{}, fmt.Errorf("decode response: %w", err)
	}
	if !resp.OK {
		return resp, fmt.Errorf("daemon error: %s", resp.Error)
	}
	return resp, nil
}

// Ping reports whether a daemon is answering on the socket.
func Ping(socketPath string) error {
	_, err := roundtrip(socketPath, request{Cmd: cmdPing})
	return err
}

// SendEvent delivers a single event to the daemon. It is the transport used by
// hooks; delivery is acknowledged so the caller knows the event was buffered.
func SendEvent(socketPath string, e Event) error {
	_, err := roundtrip(socketPath, request{Cmd: cmdEvent, Event: &e})
	return err
}

// GetStatus queries the daemon for its current runtime status.
func GetStatus(socketPath string) (Status, error) {
	resp, err := roundtrip(socketPath, request{Cmd: cmdStatus})
	if err != nil {
		return Status{}, err
	}
	if resp.Status == nil {
		return Status{}, errors.New("daemon returned no status")
	}
	return *resp.Status, nil
}

// IsRunning reports the daemon's PID and whether its process is alive, reading
// the PID file and probing the process with signal 0.
func IsRunning(pidPath string) (int, bool) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		// ESRCH: no such process. EPERM: alive but not ours to signal.
		return pid, errors.Is(err, syscall.EPERM)
	}
	return pid, true
}

// Start launches a detached daemon process by running cmdPath with args (e.g.
// the "daemon" subcommand) and waits until it is answering on the socket. It
// returns ErrAlreadyRunning if a daemon is already up.
func Start(cmdPath string, args []string, socketPath, pidPath, logPath string) error {
	if Ping(socketPath) == nil {
		return ErrAlreadyRunning
	}
	if _, ok := IsRunning(pidPath); ok {
		return ErrAlreadyRunning
	}

	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer logFile.Close()

	cmd := exec.Command(cmdPath, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Detach into a new session so the daemon outlives the spawning process and
	// its terminal.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	// Reap nothing here: the child is reparented to init once we exit.
	_ = cmd.Process.Release()

	// Wait for the socket to come up.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if Ping(socketPath) == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("daemon did not come up within timeout")
}

// Stop signals the daemon to shut down gracefully (SIGTERM), waiting up to
// grace for it to exit before escalating to SIGKILL. It returns ErrNotRunning
// if no daemon is running.
func Stop(pidPath string, grace time.Duration) error {
	pid, ok := IsRunning(pidPath)
	if !ok {
		return ErrNotRunning
	}

	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return ErrNotRunning
		}
		return fmt.Errorf("send SIGTERM: %w", err)
	}

	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Grace elapsed; force kill.
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("send SIGKILL: %w", err)
	}
	return nil
}

// EnsureRunningAndSend delivers an event to the daemon, auto-starting it first
// if it is down and retrying once the socket is up. This is how hooks avoid
// losing the event that triggered the (cold) daemon start.
func EnsureRunningAndSend(cmdPath string, args []string, socketPath, pidPath, logPath string, e Event) error {
	if err := SendEvent(socketPath, e); err == nil {
		return nil
	}
	if err := Start(cmdPath, args, socketPath, pidPath, logPath); err != nil && !errors.Is(err, ErrAlreadyRunning) {
		return fmt.Errorf("auto-start daemon: %w", err)
	}
	return SendEvent(socketPath, e)
}
