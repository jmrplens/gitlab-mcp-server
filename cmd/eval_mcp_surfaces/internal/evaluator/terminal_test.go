package evaluator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/eval_mcp_surfaces/internal/termio"
)

// TestTerminalPrintHelpers_WriteToConfiguredLog verifies package-level terminal
// helpers write into the configured command output sink.
func TestTerminalPrintHelpers_WriteToConfiguredLog(t *testing.T) {
	var b strings.Builder
	restore := termio.SetOutputForTest(termio.NewOutput(&b, false))
	t.Cleanup(restore)
	terminalPrintf("hello %s", "there")
	terminalPrint("!")
	terminalLogPrintf(" log=%d", 1)
	if got := b.String(); got != "hello there! log=1" {
		t.Fatalf("terminal output = %q, want combined log", got)
	}
}

// TestConfigureTerminalOutput_DefaultAndOverride verifies configureTerminalOutput
// resolves the default log path and returns the close hook for cleanup, and
// that an invalid path produces an error and a nil cleanup hook.
func TestConfigureTerminalOutput_DefaultAndOverride(t *testing.T) {
	output := t.TempDir()
	restore := termio.SetOutputForTest(termio.NewOutput(&strings.Builder{}, false))
	t.Cleanup(restore)

	updated, closeHook, err := configureTerminalOutput(options{Output: output})
	if err != nil {
		t.Fatalf("configureTerminalOutput() error = %v", err)
	}
	if updated.TerminalLog == "" {
		t.Fatal("configureTerminalOutput() did not populate TerminalLog")
	}
	if closeHook == nil {
		t.Fatal("configureTerminalOutput() returned nil close hook")
	}
	if closeErr := closeHook(); closeErr != nil {
		t.Fatalf("closeHook() error = %v", closeErr)
	}

	// An invalid path should surface an error and a nil cleanup hook so the
	// caller can distinguish between success-with-cleanup and outright failure.
	// Using a regular file as the log path's parent directory forces MkdirAll
	// to fail with ENOTDIR.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if writeErr := os.WriteFile(blocker, []byte("not a dir"), 0o600); writeErr != nil {
		t.Fatalf("write blocker: %v", writeErr)
	}
	if _, _, invalidErr := configureTerminalOutput(options{Output: output, TerminalLog: filepath.Join(blocker, "log.txt")}); invalidErr == nil {
		t.Fatal("configureTerminalOutput(invalid) error = nil, want error")
	}
}

// TestConfigureTerminalOutput_WritesLogWithoutEcho verifies progress output is
// captured in the terminal log by default.
func TestConfigureTerminalOutput_WritesLogWithoutEcho(t *testing.T) {
	// configureTerminalOutput swaps the process-wide termio sink. Without this
	// restore the global keeps pointing at the log file closed below, and the
	// next test in the package to call terminalPrintf writes to a closed file —
	// a failure that only appears under some orderings.
	t.Cleanup(termio.SetOutputForTest(termio.NewOutput(&strings.Builder{}, false)))

	logPath := filepath.Join(t.TempDir(), "terminal.log")
	_, closeLog, err := configureTerminalOutput(options{TerminalLog: logPath})
	if err != nil {
		t.Fatalf("configureTerminalOutput() error = %v", err)
	}
	terminalPrintf("progress line %d\n", 1)
	if closeErr := closeLog(); closeErr != nil {
		t.Fatalf("close terminal log: %v", closeErr)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read terminal log: %v", err)
	}
	requireContainsAll(t, "terminal log", string(data), []string{"eval_mcp_surfaces terminal output", "progress line 1"})
}

// TestShouldConfigureTerminalOutput_SkipsCheckDocsWithoutExplicitOutput verifies
// report-checking modes avoid terminal log setup unless output is requested.
//
// The test covers check-docs, efficiency checks, and trace comparisons as quiet
// modes, then asserts that default options, an explicit log path, or a print flag
// all enable terminal output. This keeps validation commands from creating
// unnecessary artifacts.
func TestShouldConfigureTerminalOutput_SkipsCheckDocsWithoutExplicitOutput(t *testing.T) {
	for _, tt := range []struct {
		name string
		opts options
	}{
		{"check docs", options{CheckDocs: true}},
		{"check efficiency", options{CheckEfficiency: stringList{"dist/evaluation/efficiency.md"}}},
		{"compare traces", options{CompareTraces: stringList{"dist/evaluation/report.traces"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if shouldConfigureTerminalOutput(tt.opts) {
				t.Errorf("shouldConfigureTerminalOutput(%+v) = true, want false", tt.opts)
			}
		})
	}
	for _, tt := range []struct {
		name string
		opts options
	}{
		{"default options", options{}},
		{"check docs with explicit log path", options{CheckDocs: true, TerminalLog: "check.log"}},
		{"check docs with print output", options{CheckDocs: true, PrintOutput: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !shouldConfigureTerminalOutput(tt.opts) {
				t.Errorf("shouldConfigureTerminalOutput(%+v) = false, want true", tt.opts)
			}
		})
	}
}
