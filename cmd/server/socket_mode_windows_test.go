//go:build windows

// socket_mode_windows_test.go covers the Windows half of unix socket binding,
// whose one condition is the listen it hands the path to.
package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestBindUnixSocket_AListenThatFails_IsReportedWithThePath covers the refusal
// the Windows half returns itself: a path whose directory does not exist is
// one Winsock cannot create a socket file at, and the error names the path the
// operator gave rather than surfacing the bind's own message alone.
//
// listenUnix checks the directory before it gets here, so this is the half's
// own answer, driven directly: it is what stands between a bind the kernel
// refused and a server reported as listening.
func TestBindUnixSocket_AListenThatFails_IsReportedWithThePath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "absent", "mcp.sock")
	listener, err := bindUnixSocket(t.Context(), path, 0o660)
	if err == nil {
		_ = listener.Close()
		t.Fatalf("bindUnixSocket(%q) listened under a directory that does not exist", path)
	}
	if !strings.Contains(err.Error(), "listening on unix socket") || !strings.Contains(err.Error(), "mcp.sock") {
		t.Errorf("error = %q, want it to say the listen failed and name the path", err)
	}
}

// TestBindUnixSocket_OnWindows_ListensAtThePathItWasGiven covers the other
// way out of the same condition: Windows binds the operator's path directly,
// with no staging directory, so the listener reports that path and closing it
// leaves nothing listening.
func TestBindUnixSocket_OnWindows_ListensAtThePathItWasGiven(t *testing.T) {
	t.Parallel()

	path := filepath.Join(socketDir(t), "mcp.sock")
	listener, err := bindUnixSocket(t.Context(), path, 0o660)
	if err != nil {
		t.Fatalf("bindUnixSocket(%q) error = %v", path, err)
	}
	if got := listener.Addr().String(); got != path {
		t.Errorf("Addr() = %q, want the path it was given, %q", got, path)
	}
	if closeErr := listener.Close(); closeErr != nil {
		t.Errorf("Close() error = %v", closeErr)
	}
}
