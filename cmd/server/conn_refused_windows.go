//go:build windows

// conn_refused_windows.go answers "did the kernel say nothing is listening?" on
// Windows, where the answer arrives as a Winsock code that shares neither the
// value nor the identity of the POSIX errno of the same name.
package main

import (
	"errors"
	"syscall"
)

// wsaeConnRefused is Winsock's WSAECONNREFUSED, "No connection could be made
// because the target machine actively refused it".
//
// It is spelled out here because Go's syscall package does not export it on
// Windows: the E-prefixed names it does define, [syscall.ECONNREFUSED] among
// them, are a separate block based at APPLICATION_ERROR and never equal to what
// a socket operation returns. A dial that Windows refuses comes back as this
// value, so a check against the POSIX name alone answers false for the one case
// it exists to recognize. The number is Winsock's, fixed since Windows Sockets 2
// and part of its ABI.
const wsaeConnRefused = syscall.Errno(10061)

// isConnRefused reports whether err is the kernel's answer that nothing is
// listening on the address that was dialed.
//
// Both spellings are accepted. The POSIX one cannot reach a socket error here,
// but it costs nothing and keeps this honest if a future Go maps the two.
func isConnRefused(err error) bool {
	return errors.Is(err, wsaeConnRefused) || errors.Is(err, syscall.ECONNREFUSED)
}
