package cmdutil

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fatalExit struct{}

// TestRepositoryRoot_FindsModuleRoot verifies RepositoryRoot walks from a
// nested directory to the nearest parent containing go.mod.
func TestRepositoryRoot_FindsModuleRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	nested := filepath.Join(root, "cmd", "tool")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	got, err := RepositoryRoot(nested)
	if err != nil {
		t.Fatalf("RepositoryRoot() error = %v", err)
	}
	if got != root {
		t.Fatalf("RepositoryRoot() = %q, want %q", got, root)
	}
}

// TestRepositoryRoot_NotFound verifies RepositoryRoot returns an actionable
// error when no parent directory contains go.mod.
func TestRepositoryRoot_NotFound(t *testing.T) {
	_, err := RepositoryRoot(t.TempDir())
	if err == nil {
		t.Fatal("RepositoryRoot() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "go.mod not found") {
		t.Fatalf("RepositoryRoot() error = %q, want go.mod not found", err)
	}
}

// TestFatalf_WritesMessageAndExits verifies Fatalf writes the formatted
// diagnostic to stderr and exits with status 1, matching command-line behavior.
func TestFatalf_WritesMessageAndExits(t *testing.T) {
	var stderr bytes.Buffer
	previousStderr := fatalStderr
	previousExit := exitProcess
	t.Cleanup(func() {
		fatalStderr = previousStderr
		exitProcess = previousExit
	})

	var exitCode int
	exited := false
	fatalStderr = &stderr
	exitProcess = func(code int) {
		exited = true
		exitCode = code
		panic(fatalExit{})
	}

	defer func() {
		recovered := recover()
		if _, ok := recovered.(fatalExit); !ok {
			t.Fatalf("Fatalf() panic = %v, want fatalExit", recovered)
		}
		if !exited || exitCode != 1 {
			t.Fatalf("Fatalf() exit = (%t, %d), want (true, 1)", exited, exitCode)
		}
		if got := stderr.String(); got != "failed: boom\n" {
			t.Fatalf("Fatalf() stderr = %q, want formatted message", got)
		}
	}()

	Fatalf("failed: %s", "boom")
	t.Fatal("Fatalf() returned without exiting")
}
