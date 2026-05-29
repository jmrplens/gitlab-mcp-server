package evaluator

import (
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
