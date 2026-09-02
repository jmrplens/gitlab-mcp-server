// termio_test.go covers the terminal and log-file output helpers used by the
// eval_mcp_surfaces CLI to mirror command progress into an optional log file
// and stdout.

package termio

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
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

// captureStream swaps the process stream at target (os.Stdout or os.Stderr)
// for a temporary file until the test ends and returns a reader for what was
// written to it. The helpers under test write to the package-level os.Stdout
// and os.Stderr variables, so replacing them is what lets a test observe the
// echo and the diagnostic paths.
func captureStream(t *testing.T, target **os.File) func() string {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "stream"))
	if err != nil {
		t.Fatalf("create capture file: %v", err)
	}
	previous := *target
	*target = file
	t.Cleanup(func() {
		*target = previous
		_ = file.Close()
	})
	return func() string {
		data, readErr := os.ReadFile(file.Name())
		if readErr != nil {
			t.Fatalf("read capture file: %v", readErr)
		}
		return string(data)
	}
}

// TestOutputWrite_EchoEnabled_MirrorsToStdout verifies that an echoing sink
// writes the payload to both the file sink and os.Stdout and still reports
// the full length.
func TestOutputWrite_EchoEnabled_MirrorsToStdout(t *testing.T) {
	stdout := captureStream(t, &os.Stdout)
	var builder strings.Builder

	n, err := NewOutput(&builder, true).Write([]byte("echoed"))
	if err != nil || n != len("echoed") {
		t.Fatalf("Write() = %d, %v; want %d nil", n, err, len("echoed"))
	}
	if builder.String() != "echoed" || stdout() != "echoed" {
		t.Fatalf("file sink = %q, stdout = %q; want both echoed", builder.String(), stdout())
	}
}

// TestOutputWrite_StdoutClosed_ReturnsStdoutError verifies that a failure of
// the stdout echo is returned to the caller even though the file sink
// accepted the payload: the sink is a tee, and a tee that drops one leg
// silently would hide a broken terminal.
func TestOutputWrite_StdoutClosed_ReturnsStdoutError(t *testing.T) {
	captureStream(t, &os.Stdout)
	if err := os.Stdout.Close(); err != nil {
		t.Fatalf("close capture stdout: %v", err)
	}
	var builder strings.Builder

	_, err := NewOutput(&builder, true).Write([]byte("lost"))
	if err == nil {
		t.Fatal("Write() error = nil, want the closed-stdout error")
	}
	if builder.String() != "lost" {
		t.Fatalf("file sink = %q, want the payload written before the echo failed", builder.String())
	}
}

// TestPrintHelpers_EchoEnabled_WriteStdout verifies Printf and Print echo to
// os.Stdout when the sink is configured to, while LogPrintf never does.
func TestPrintHelpers_EchoEnabled_WriteStdout(t *testing.T) {
	tests := []struct {
		name       string
		call       func()
		wantStdout string
		wantFile   string
	}{
		{name: "Printf", call: func() { Printf("value=%d", 7) }, wantStdout: "value=7", wantFile: "value=7"},
		{name: "Print", call: func() { Print("plain") }, wantStdout: "plain", wantFile: "plain"},
		{name: "LogPrintf", call: func() { LogPrintf("log=%s", "only") }, wantStdout: "", wantFile: "log=only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := captureStream(t, &os.Stdout)
			var builder strings.Builder
			t.Cleanup(SetOutputForTest(NewOutput(&builder, true)))

			tt.call()
			if got := stdout(); got != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", got, tt.wantStdout)
			}
			if got := builder.String(); got != tt.wantFile {
				t.Errorf("file sink = %q, want %q", got, tt.wantFile)
			}
		})
	}
}

// TestPrintHelpers_FileSinkFails_ReportsOnStderr verifies each helper names
// itself on os.Stderr when the log-file sink rejects the write, so a full disk
// under the evaluator is visible instead of silently truncating the log.
func TestPrintHelpers_FileSinkFails_ReportsOnStderr(t *testing.T) {
	tests := []struct {
		name string
		call func()
	}{
		{name: "Printf", call: func() { Printf("value=%d", 7) }},
		{name: "Print", call: func() { Print("plain") }},
		{name: "LogPrintf", call: func() { LogPrintf("log=%s", "only") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stderr := captureStream(t, &os.Stderr)
			t.Cleanup(SetOutputForTest(NewOutput(failingWriter{}, false)))

			tt.call()
			want := "termio." + tt.name + ": write terminal log: write failed\n"
			if got := stderr(); got != want {
				t.Errorf("stderr = %q, want %q", got, want)
			}
		})
	}
}

// TestConfigure_ValidPath_RoutesOutputAndRestoresOnClose verifies Configure
// creates the log directory and file, writes its header lines, routes both
// the helpers and the default slog logger into the file, and that the
// returned closer restores the previous logger and sink and closes the file.
func TestConfigure_ValidPath_RoutesOutputAndRestoresOnClose(t *testing.T) {
	stdout := captureStream(t, &os.Stdout)
	logPath := filepath.Join(t.TempDir(), "nested", "terminal.log")
	previousLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	closer, err := Configure(logPath, true)
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	Printf("progress %d\n", 1)
	slog.Info("from slog")
	if closeErr := closer(); closeErr != nil {
		t.Fatalf("closer() error = %v", closeErr)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read terminal log: %v", err)
	}
	for _, want := range []string{
		"eval_mcp_surfaces terminal output\n",
		"terminal_log=" + logPath + "\n",
		"print_output=true\n",
		"progress 1\n",
		"msg=\"from slog\"",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(string(content), want) {
				t.Errorf("terminal log %q does not contain %q", content, want)
			}
		})
	}
	if got := stdout(); !strings.Contains(got, "progress 1\n") || strings.Contains(got, "terminal_log=") {
		t.Errorf("stdout = %q, want the echoed progress line and no log-only header", got)
	}
	if slog.Default() != previousLogger {
		t.Error("closer() did not restore the previous default logger")
	}
	if commandOutput.file != nil || commandOutput.echo {
		t.Errorf("commandOutput after close = %+v, want the zero sink", commandOutput)
	}
	if closer() == nil {
		t.Error("second closer() error = nil, want the already-closed file error")
	}
}

// TestConfigure_UnusableLogPath_ReturnsError verifies Configure reports the
// directory-creation and file-open failures instead of routing output into
// nothing: a log directory that collides with a file, and a log path that is
// a directory.
func TestConfigure_UnusableLogPath_ReturnsError(t *testing.T) {
	tests := []struct {
		name    string
		logPath func(t *testing.T) string
		wantErr string
	}{
		{
			name: "log directory is a file",
			logPath: func(t *testing.T) string {
				t.Helper()
				blocker := filepath.Join(t.TempDir(), "blocker")
				if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
					t.Fatalf("write blocker: %v", err)
				}
				return filepath.Join(blocker, "terminal.log")
			},
			wantErr: "create terminal log directory",
		},
		{
			name: "log path is a directory",
			logPath: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "open terminal log",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			closer, err := Configure(tt.logPath(t), false)
			if err == nil {
				_ = closer()
				t.Fatalf("Configure() error = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Configure() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
