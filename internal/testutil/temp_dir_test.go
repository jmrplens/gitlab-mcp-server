// temp_dir_test.go covers the temporary-directory isolation helper.
package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// TestVerifyTempDir_APlatformThatReadsAnotherVariable_IsToldSo verifies the
// refusal no supported platform reaches: with every variable set and the
// process temporary directory still somewhere else, the helper says the
// isolation did not take rather than letting a containment test assert nothing.
func TestVerifyTempDir_APlatformThatReadsAnotherVariable_IsToldSo(t *testing.T) {
	original := processTempDir
	processTempDir = func() string { return filepath.Join("somewhere", "else") }
	t.Cleanup(func() { processTempDir = original })
	reporter := &recordingTempDirReporter{}

	verifyTempDir(reporter, filepath.Join("the", "isolated", "one"))

	if !strings.Contains(reporter.message, "does not cover") {
		t.Errorf("report = %q, want it to name the variables it covers", reporter.message)
	}
}

// recordingTempDirReporter stands in for *testing.T so the refusal is recorded
// rather than failing the test that provoked it.
type recordingTempDirReporter struct {
	message string
}

// Helper satisfies the reporter and does nothing.
func (*recordingTempDirReporter) Helper() {}

// Fatalf records what would have been reported.
func (r *recordingTempDirReporter) Fatalf(format string, args ...any) {
	r.message = fmt.Sprintf(format, args...)
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
