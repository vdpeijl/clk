package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vdpeijl/clk/internal/capture"
)

// daemonPaths bundles the filesystem locations the daemon and its controllers
// share.
type daemonPaths struct {
	socket string
	pid    string
	log    string
	db     string
}

// resolveDaemonPaths computes every path the daemon depends on.
func resolveDaemonPaths() (daemonPaths, error) {
	socket, err := socketPath()
	if err != nil {
		return daemonPaths{}, err
	}
	pid, err := pidPath()
	if err != nil {
		return daemonPaths{}, err
	}
	logp, err := logPath()
	if err != nil {
		return daemonPaths{}, err
	}
	db, err := dbPath()
	if err != nil {
		return daemonPaths{}, err
	}
	return daemonPaths{socket: socket, pid: pid, log: logp, db: db}, nil
}

// daemonSpawn returns the executable and arguments used to launch a detached
// daemon process (this binary re-invoked with the hidden "daemon" command).
func daemonSpawn() (string, []string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", nil, fmt.Errorf("locate executable: %w", err)
	}
	return exe, []string{"daemon"}, nil
}

// daemonCmd is the hidden entry point for the background process itself. Users
// interact with the daemon through `clk up`/`down`/`status`, not this command.
var daemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Run the capture daemon in the foreground (internal)",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		p, err := resolveDaemonPaths()
		if err != nil {
			return err
		}
		d := capture.New(p.socket, p.db, p.log, p.pid)
		return d.Run()
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}
