//go:build !windows

package autoupdate

import (
	"fmt"
	"os"
	"syscall"
)

// ExecSelf replaces the current process with a new instance of the same
// binary. On Unix, this uses syscall.Exec which preserves the PID and all
// open file descriptors, making it transparent to the MCP client.
// This function does not return on success.
func ExecSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("autoupdate: resolving executable path: %w", err)
	}

	return syscall.Exec(exe, os.Args, os.Environ()) //#nosec G204 G702 -- intentional re-exec of our own binary after self-update
}
