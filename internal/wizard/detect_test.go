package wizard

import (
	"os"
	"testing"
)

// TestIsInteractiveTerminal_ClosedStdin verifies stat errors are treated as
// non-interactive terminals.
func TestIsInteractiveTerminal_ClosedStdin(t *testing.T) {
	originalStdin := os.Stdin
	stdin, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatal(err)
	}
	if err = stdin.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdin = stdin
	t.Cleanup(func() { os.Stdin = originalStdin })

	if IsInteractiveTerminal() {
		t.Fatal("IsInteractiveTerminal() = true for closed stdin")
	}
}
