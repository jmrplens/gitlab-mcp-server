//go:build stdioe2e && windows

// terminate_windows_test.go is the termination path a Windows console uses:
// a CTRL_BREAK event to the server's process group, which the Go runtime
// delivers as os.Interrupt, the signal the server stops on. Windows has no
// SIGTERM to send, and TerminateProcess is a kill, which would test nothing
// about shutdown.
package stdioe2e

import (
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// terminationSignalName names what signalTermination sends, for messages.
const terminationSignalName = "CTRL_BREAK"

// prepareForTermination puts the server in a console process group of its
// own, so a CTRL_BREAK aimed at that group reaches the server and nothing
// else, the test binary included.
func prepareForTermination(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// signalTermination sends CTRL_BREAK to the process group the server was
// started in, whose id is the server's pid.
func signalTermination(p *os.Process) error {
	return windows.GenerateConsoleCtrlEvent(syscall.CTRL_BREAK_EVENT, uint32(p.Pid)) //#nosec G115 -- a pid is a non-negative 32-bit value on Windows
}

// withPlatformHome mirrors the effective HOME into USERPROFILE, which is what
// os.UserHomeDir reads on Windows. Without it a test that set HOME leaves the
// server resolving the real account's profile instead of the one the test
// built.
func withPlatformHome(environ []string) []string {
	home := ""
	for _, kv := range environ {
		if strings.HasPrefix(kv, "HOME=") {
			home = strings.TrimPrefix(kv, "HOME=")
		}
	}
	return append(environ, "USERPROFILE="+home)
}
