//go:build unix

package main

import (
	"testing"

	"golang.org/x/sys/unix"
)

// mkfifoForTest creates a FIFO at path so a test can point stdin at the shape
// `docker run -i` gives, and reports whether it succeeded.
//
// A FIFO rather than an os.Pipe because the inference stats a path: a pipe
// from os.Pipe has no name to stat, and the property under test is what
// os.Stat reports about file descriptor 0, not how the pipe was made.
func mkfifoForTest(t *testing.T, path string) bool {
	t.Helper()
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Logf("mkfifo %s: %v", path, err)
		return false
	}
	return true
}
