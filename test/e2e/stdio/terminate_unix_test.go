//go:build stdioe2e && !windows

// terminate_unix_test.go is the termination path a supervisor uses on every
// platform but Windows: SIGTERM, which reaches a process however it was
// started.
package stdioe2e

import (
	"os"
	"os/exec"
	"syscall"
)

// terminationSignalName names what signalTermination sends, for messages.
const terminationSignalName = "SIGTERM"

// prepareForTermination leaves the command as it is: a signal needs no
// preparation of the process that will receive it.
func prepareForTermination(*exec.Cmd) {}

// signalTermination asks the process to shut down the way a supervisor does
// here, with SIGTERM.
func signalTermination(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}

// withPlatformHome leaves the environment as it is: os.UserHomeDir reads HOME
// on Unix, which the caller already set.
func withPlatformHome(environ []string) []string { return environ }
