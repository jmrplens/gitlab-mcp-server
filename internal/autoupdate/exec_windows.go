//go:build windows

package autoupdate

import "errors"

// ExecSelf is not supported on Windows. The rename trick places the new
// binary at the original path so it will be used on the next startup.
func ExecSelf() error {
	return errors.New("autoupdate: exec-self is not supported on Windows; update will take effect on next restart")
}
