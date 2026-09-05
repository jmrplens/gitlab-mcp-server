//go:build httpe2e && !windows

// terminate_unix_test.go is the termination path a supervisor uses on every
// platform but Windows: SIGTERM, which reaches a process however it was
// started.
package httpe2e

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
