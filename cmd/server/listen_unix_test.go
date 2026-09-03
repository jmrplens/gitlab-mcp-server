//go:build !windows

// listen_unix_test.go holds the parts of the listener's behavior that only
// exist where the filesystem carries POSIX permission bits.
//
// The split is a judgement about each assertion, not a blanket exclusion.
// Windows has AF_UNIX, binds it, serves HTTP over it and refuses the paths it
// should, all of which stays in listen_test.go and runs everywhere. What it has
// no equivalent of is the permission mode: access to the socket follows the
// directory's ACL, there is no chmod that would change it, and bindUnixSocket
// warns rather than pretending otherwise. An assertion that the socket carries
// 0660 is therefore a claim about POSIX, and asserting it there would be
// demanding the platform be something it is not.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestListenHTTP_UnixSocket_CarriesTheRequestedMode verifies the half of socket
// support that listen_test.go's serving test no longer covers.
//
// The mode matters because it is the difference between a proxy that can reach
// the server and one that cannot, and in the other direction between a socket
// only the proxy's group can open and one every local account can.
func TestListenHTTP_UnixSocket_CarriesTheRequestedMode(t *testing.T) {
	t.Parallel()

	path := filepath.Join(socketDir(t), "mode.sock")
	listener, err := listenHTTP(t.Context(), path, config.DefaultSocketMode)
	if err != nil {
		t.Fatalf("listenHTTP() error = %v", err)
	}
	defer listener.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if perm := info.Mode().Perm(); perm != config.DefaultSocketMode {
		t.Errorf("socket mode = %#o, want %#o", perm, config.DefaultSocketMode)
	}
}

// TestListenHTTP_UnixSocketWithoutAMode_TakesTheDefault covers the zero mode,
// which is what a Config assembled without the flag carries.
//
// The default is the one that matters for a same-host proxy: a socket only the
// proxy's group can open, rather than one every local account can.
func TestListenHTTP_UnixSocketWithoutAMode_TakesTheDefault(t *testing.T) {
	t.Parallel()

	path := filepath.Join(socketDir(t), "default-mode.sock")
	listener, err := listenHTTP(t.Context(), path, 0)
	if err != nil {
		t.Fatalf("listenHTTP: %v", err)
	}
	defer listener.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if got := info.Mode().Perm(); got != config.DefaultSocketMode {
		t.Errorf("socket mode = %#o, want the default %#o", got, config.DefaultSocketMode)
	}
}

// TestClearStaleSocket_AnUnreadablePath_IsRefusedRatherThanRemoved covers the
// lstat failing for a reason other than the path being absent.
//
// Absent is the ordinary case and means "nothing to clear". Anything else means
// the question went unanswered, and removing on an unanswered question is how a
// running deployment loses its socket to a racing restart.
//
// The refusal is not POSIX-specific; arranging for it is. This puts the socket
// under a regular file, which POSIX answers with ENOTDIR, an error that is
// plainly not "absent". Windows answers the same layout with
// ERROR_PATH_NOT_FOUND, which Errno.Is maps to fs.ErrNotExist, so there the
// question is answered rather than unanswered and nil is the right reply.
func TestClearStaleSocket_AnUnreadablePath_IsRefusedRatherThanRemoved(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	blocking := filepath.Join(dir, "file")
	if err := os.WriteFile(blocking, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("writing the blocking file: %v", err)
	}

	err := clearStaleSocket(t.Context(), filepath.Join(blocking, "mcp.sock"))

	if err == nil {
		t.Fatal("clearStaleSocket accepted a path it could not examine")
	}
	if !strings.Contains(err.Error(), "--http-addr") {
		t.Errorf("error = %q, want it to name the flag the operator typed", err)
	}
}
