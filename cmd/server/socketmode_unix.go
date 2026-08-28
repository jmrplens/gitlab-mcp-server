//go:build !windows

// socketmode_unix.go creates the unix socket with its permission mode already
// set, rather than setting it afterwards.
//
// The obvious implementation — bind, then os.Chmod(path, mode) — has a window
// between the two calls. On a directory an untrusted local account can write
// (/tmp being the obvious one), that account can replace the socket with a
// symlink in the gap and the chmod then lands on whatever the link points at:
// CWE-367, time-of-check to time-of-use.
//
// Two fixes look plausible and only one works. Applying the mode to the bound
// descriptor with fchmod(2) would close the window by removing the name from
// the operation — but on Linux fchmod on a socket descriptor RETURNS SUCCESS
// AND CHANGES NOTHING. Measured: asking for 0660 leaves 0755. A silent no-op
// is worse than the race it was meant to fix, because it reports a restriction
// that was never applied.
//
// So the mode is set through the umask instead, around the bind. The socket is
// created with the right bits from the start: there is no second operation to
// race, and no path to re-point.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
)

// umaskMu serializes the umask window. umask is process-global, so two
// concurrent binds could otherwise restore each other's value. Binding happens
// once at startup, so this is never contended in practice — it is here so the
// invariant does not depend on that staying true.
var umaskMu sync.Mutex //nolint:gochecknoglobals // guards a process-global

// bindUnixSocket listens on path with the socket created under mode.
//
// mode is expressed as permission bits (0660); umask takes the complement, so
// 0660 becomes a umask of 0117.
func bindUnixSocket(ctx context.Context, path string, mode os.FileMode) (net.Listener, error) {
	umaskMu.Lock()
	previous := syscall.Umask(int(^mode.Perm() & 0o777))
	listener, err := (&net.ListenConfig{}).Listen(ctx, "unix", path)
	syscall.Umask(previous)
	umaskMu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("listening on unix socket %q: %w", path, err)
	}

	// Verified rather than assumed: a umask can only clear bits, and a
	// filesystem or a kernel that ignores it would leave the socket more open
	// than the operator asked for. Serving on a socket whose permissions
	// nobody checked is the failure this whole path exists to avoid.
	info, statErr := os.Stat(path)
	if statErr != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("checking the mode of unix socket %q: %w", path, statErr)
	}
	if got := info.Mode().Perm(); got != mode.Perm() {
		_ = listener.Close()
		return nil, fmt.Errorf(
			"unix socket %q was created with mode %#o, not the requested %#o; refusing to serve on it",
			path, got, mode.Perm())
	}
	return listener, nil
}
