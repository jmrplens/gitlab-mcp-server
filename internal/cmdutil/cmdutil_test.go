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

// TestRepositoryRoot_AbsError verifies RepositoryRoot surfaces the
// underlying error from filepath.Abs when the working directory is
// unreadable. The test chdirs into a fresh temp directory and then
// revokes all permissions, causing os.Getwd (called from filepath.Abs)
// to fail with EACCES. The error is wrapped in a PathError with op
// "stat" and path "."; RepositoryRoot must propagate that error.
func TestRepositoryRoot_AbsError(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Windows does not support directory read permission restriction via Chmod")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root, cannot revoke permissions to fail Getwd")
	}

	tmp := t.TempDir()
	t.Chdir(tmp)

	// Drop all permissions on the cwd so getcwd(3) cannot read "." to
	// resolve the path; it returns EACCES, which filepath.Abs surfaces
	// as a PathError { Op: "stat", Path: ".", Err: EACCES }.
	if err := os.Chmod(tmp, 0o000); err != nil {
		t.Fatalf("chmod tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(tmp, 0o700) }) //nolint:gosec // test fixture; needs exec bit for cleanup traversal

	_, err := RepositoryRoot("relative")
	if err == nil {
		t.Fatal("RepositoryRoot() error = nil, want error from filepath.Abs")
	}
	if strings.Contains(err.Error(), "go.mod not found") {
		t.Fatalf("RepositoryRoot() error = %q, want Abs error, not NotFound", err)
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
