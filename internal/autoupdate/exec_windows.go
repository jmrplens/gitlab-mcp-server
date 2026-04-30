//go:build windows

// exec_windows.go provides the Windows stub for ExecSelf.
// Windows has no true exec syscall — Go's syscall.Exec on Windows spawns
// a new process, losing the MCP stdio pipes. On Windows, the binary is
// replaced via the rename trick and takes effect on the next startup.
package autoupdate

import "errors"

// ExecSelf is not supported on Windows. The rename trick places the new
// binary at the original path so it will be used on the next startup.
func ExecSelf() error {
	return errors.New("autoupdate: exec-self is not supported on Windows; update will take effect on next restart")
}
