// temp_dir_test.go covers the temporary-directory isolation helper.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsolateTempDir_EveryPlatformVariable_PointsOsTempDirAtTheDirectory
// verifies the helper's whole purpose: after it runs, os.TempDir is the
// directory it was given, and every variable a supported platform reads to
// decide that says so.
//
// The per-variable assertions are what keep the fix honest. Checking only
// os.TempDir would pass on Linux with TMPDIR alone, which is precisely the
// state that let Windows containment tests assert nothing: the isolation was a
// no-op there, so fixtures meant to sit outside every allowed root sat inside
// the real temporary directory, which is always allowed.
func TestIsolateTempDir_EveryPlatformVariable_PointsOsTempDirAtTheDirectory(t *testing.T) {
	dir := t.TempDir()
	IsolateTempDir(t, dir)

	// os.TempDir is the subject of the assertion, not an alternative to
	// t.TempDir.
	if got, want := filepath.Clean(os.TempDir()), filepath.Clean(dir); got != want { //nolint:usetesting // see above
		t.Errorf("os.TempDir() = %q, want %q", got, want)
	}
	for _, name := range tempDirVars {
		t.Run(name, func(t *testing.T) {
			if got := os.Getenv(name); got != dir {
				t.Errorf("%s = %q, want %q", name, got, dir)
			}
		})
	}
}

// TestIsolateTempDir_Restored_LeavesTheProcessAsItFoundIt verifies the cleanup
// t.Setenv registers, because a helper that leaked a temporary directory into
// the rest of the run would send every later test's scratch files somewhere
// already deleted.
func TestIsolateTempDir_Restored_LeavesTheProcessAsItFoundIt(t *testing.T) {
	// os.TempDir is the subject here, not a stand-in for t.TempDir: what is
	// asserted is the process-wide value before and after.
	before := os.TempDir() //nolint:usetesting // see above

	t.Run("isolated", func(t *testing.T) {
		IsolateTempDir(t, t.TempDir())
	})

	if after := os.TempDir(); after != before { //nolint:usetesting // see above
		t.Errorf("os.TempDir() = %q after the isolated subtest ended, want %q restored", after, before)
	}
}
