//go:build !unix

package main

import "testing"

// mkfifoForTest reports that this platform has no FIFO for the pipe case, so
// the case skips rather than failing on a platform whose containers this
// inference was never about.
func mkfifoForTest(t *testing.T, _ string) bool {
	t.Helper()
	return false
}
