package evaluator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/eval_mcp_surfaces/internal/termio"
)

// TestShouldConfigureTerminalOutput_RespectsQuietCheckModes verifies terminal log
// setup is skipped for pure check commands unless explicitly requested.
func TestShouldConfigureTerminalOutput_RespectsQuietCheckModes(t *testing.T) {
	if !shouldConfigureTerminalOutput(options{}) {
		t.Fatal("shouldConfigureTerminalOutput(default) = false, want true")
	}
	if shouldConfigureTerminalOutput(options{CheckDocs: true}) {
		t.Fatal("shouldConfigureTerminalOutput(check docs) = true, want false")
	}
	if !shouldConfigureTerminalOutput(options{CheckDocs: true, PrintOutput: true}) {
		t.Fatal("shouldConfigureTerminalOutput(explicit print) = false, want true")
	}
}

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
