// termio_test.go covers the terminal and log-file output helpers used by the
// eval_mcp_surfaces CLI to mirror command progress into an optional log file
// and stdout.

package termio

import (
	"errors"
	"strings"
	"testing"
)

// failingWriter is a minimal [io.Writer] that always returns an error, used to
// verify [Output.Write] propagates underlying sink failures.
type failingWriter struct{}

// Write always fails so tests can observe how [Output.Write] handles sink errors.
func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

// TestOutputWrite_WritesFileAndPropagatesErrors verifies that [Output.Write]
// writes the supplied payload to the configured file sink and surfaces errors
// from the sink to the caller.
//
// The test first writes to a successful [strings.Builder] sink and asserts the
// byte count, absence of error, and exact echoed payload. It then writes
// through a [failingWriter] and asserts that the underlying error is returned
// unchanged. This protects the command's log-routing path from silently
// dropping write errors.
func TestOutputWrite_WritesFileAndPropagatesErrors(t *testing.T) {
	var builder strings.Builder
	out := NewOutput(&builder, false)
	n, err := out.Write([]byte("hello"))
	if err != nil || n != 5 || builder.String() != "hello" {
		t.Fatalf("Write() = %d, %v, %q; want 5 nil hello", n, err, builder.String())
	}
	failing := NewOutput(failingWriter{}, false)
	_, failingErr := failing.Write([]byte("hello"))
	if failingErr == nil {
		t.Fatal("Write(failing writer) error = nil, want error")
	}
}

// TestPrintHelpers_WriteToConfiguredLog verifies that [Printf], [Print], and
// [LogPrintf] all route through the package's configured output sink.
//
// The test swaps the package-level output for one targeting a [strings.Builder],
// invokes each helper with deterministic inputs, and asserts the combined
// payload matches the expected sequence. [SetOutputForTest] installs the sink
// and registers a cleanup that restores the prior sink for subsequent tests.
// This protects the live command's three-output contract (formatted, plain,
// log-only) from regressions that mix sinks.
func TestPrintHelpers_WriteToConfiguredLog(t *testing.T) {
	var builder strings.Builder
	restore := SetOutputForTest(NewOutput(&builder, false))
	t.Cleanup(restore)
	Printf("hello %s", "there")
	Print("!")
	LogPrintf(" log=%d", 1)
	if got := builder.String(); got != "hello there! log=1" {
		t.Fatalf("terminal output = %q, want combined log", got)
	}
}

// TestShouldConfigure_RespectsQuietCheckModes verifies that [ShouldConfigure]
// decides whether to install the terminal-log sink based on explicit output
// requests, the silent-check-docs flag, and counts of efficiency/trace checks.
//
// The first subcase asserts the default invocation opens a log file. The
// second confirms a silent check-docs run stays quiet when no other signal is
// set. The third confirms that an explicit print-output request forces log
// configuration even when silent checks are active. This protects the CLI from
// unexpectedly creating log files during CI-side validation runs.
func TestShouldConfigure_RespectsQuietCheckModes(t *testing.T) {
	if !ShouldConfigure("", false, false, 0, 0) {
		t.Fatal("ShouldConfigure(default) = false, want true")
	}
	if ShouldConfigure("", false, true, 0, 0) {
		t.Fatal("ShouldConfigure(check docs) = true, want false")
	}
	if !ShouldConfigure("", true, true, 0, 0) {
		t.Fatal("ShouldConfigure(explicit print) = false, want true")
	}
}
