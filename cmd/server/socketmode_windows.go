//go:build windows

// socketmode_windows.go is the Windows half of unix socket binding.
//
// Windows supports AF_UNIX sockets but not the POSIX permission bits the mode
// describes: access follows the ACL the socket inherits from its directory,
// and there is no umask and no fchmod to apply. Saying so out loud is the
// honest behaviour — quietly accepting the flag would let an operator believe
// a restriction is in force that the platform never applied.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
)

// bindUnixSocket listens on path, warning that the requested mode cannot be
// enforced on this platform.
func bindUnixSocket(ctx context.Context, path string, mode os.FileMode) (net.Listener, error) {
	slog.WarnContext(ctx, "unix socket permission mode is not enforceable on Windows; access follows the directory ACL",
		"path", path, "requested_mode", mode.Perm().String())

	listener, err := (&net.ListenConfig{}).Listen(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("listening on unix socket %q: %w", path, err)
	}
	return listener, nil
}
