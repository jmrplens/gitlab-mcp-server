// tempdir.go isolates the process temporary directory for a test, on every
// platform rather than only on the one the test was written on.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// tempDirVars are every environment variable [os.TempDir] consults, across all
// supported platforms: TMPDIR on Unix, then TMP and TEMP on Windows, which
// reads them through GetTempPath in that order.
var tempDirVars = []string{"TMPDIR", "TMP", "TEMP"}

// IsolateTempDir points [os.TempDir] at dir for the duration of the test.
//
// Setting TMPDIR alone is a POSIX-only instruction. On Windows [os.TempDir]
// never reads it, so a test that sets only TMPDIR keeps the real temporary
// directory and its isolation silently does nothing.
//
// That silence is expensive in exactly the tests this helper exists for. The
// upload, download and import allow-lists treat the OS temporary directory as
// an always-allowed root, so a fixture built with [testing.T.TempDir] and
// meant to sit outside every allowed root instead sat inside one. Every
// assertion that such a path is refused then passed a path that was correctly
// accepted, and the suite reported that containment worked while testing
// nothing. The allow-list itself was never wrong.
//
// It cannot be used from a parallel test, because [testing.T.Setenv] cannot.
func IsolateTempDir(t *testing.T, dir string) {
	t.Helper()
	for _, name := range tempDirVars {
		t.Setenv(name, dir)
	}
	// Verified rather than assumed, because the whole failure being fixed here
	// is an isolation that quietly did nothing. If some platform grows another
	// variable, this says so instead of letting a containment test pass while
	// asserting nothing.
	// os.TempDir is the subject of the assertion, not an alternative to
	// t.TempDir: what is being checked is that this process's temporary
	// directory is now dir.
	if got, want := filepath.Clean(os.TempDir()), filepath.Clean(dir); got != want { //nolint:usetesting // see above
		t.Fatalf("os.TempDir() = %q after isolating it to %q; this platform reads a variable %v does not cover", got, want, tempDirVars)
	}
}
