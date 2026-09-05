//go:build !windows

// conn_refused_unix.go answers "did the kernel say nothing is listening?" on the
// platforms where that answer is ECONNREFUSED and nothing else.

package main

import (
	"errors"
	"syscall"
)

// isConnRefused reports whether err is the kernel's answer that nothing is
// listening on the address that was dialed.
//
// See [isConnRefused] in conn_refused_windows.go for why this is a per-platform
// question rather than one errors.Is can settle on its own.
func isConnRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}
