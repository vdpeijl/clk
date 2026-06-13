// Package capture contains the background daemon, hook ingestion, and
// file-watch heartbeat logic. It is I/O-heavy and kept intentionally thin —
// all business logic lives in the pure packages it calls.
package capture

// Daemon manages the background capture process.
type Daemon struct {
	socketPath string
	dbPath     string
}

// New creates a Daemon configured to listen on socketPath and write to dbPath.
func New(socketPath, dbPath string) *Daemon {
	return &Daemon{
		socketPath: socketPath,
		dbPath:     dbPath,
	}
}

// Run starts the daemon and blocks until stopped.
func (d *Daemon) Run() error {
	// TODO: implement Unix socket listener, hook ingestion, file-watch, periodic merge
	return nil
}
