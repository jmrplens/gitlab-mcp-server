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

// TestRepositoryRoot_AbsError_RemovedCwd verifies RepositoryRoot surfaces the
// underlying error from filepath.Abs when the working directory has been
// deleted out from under the process. TestRepositoryRoot_AbsError revokes
// read permission on the cwd, which is a no-op for a root-owned test
// process (root bypasses discretionary permission checks, as this test
// binary's own t.Skip acknowledges) and so leaves the filepath.Abs error
// branch uncovered in CI environments that run as root. Deleting the
// directory the process is chdir'd into fails getcwd(3) with ENOENT
// regardless of privilege, exercising the same RepositoryRoot error path
// without depending on permission enforcement.
//
// It depends instead on the platform letting the removal happen and then
// reporting it, and there are two different ways for that to fail. Both are
// checked as premises rather than predicted from the platform name, and both
// skip rather than asserting an error the operating system was never going to
// produce.
func TestRepositoryRoot_AbsError_RemovedCwd(t *testing.T) {
	tmp := t.TempDir()
	nested := filepath.Join(tmp, "gone")
	if err := os.Mkdir(nested, 0o750); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	t.Chdir(nested)

	// First premise: the removal itself. Windows refuses it, because a
	// directory that is a process's working directory is a directory in use,
	// and says so ("The process cannot access the file because it is being
	// used by another process"). A platform that will not remove the working
	// directory cannot be made to fail filepath.Abs this way at all, so the
	// removal failing is a reason to skip and not a reason to fail: a t.Fatalf
	// here reported the operating system's design as a defect in this package.
	if err := os.RemoveAll(nested); err != nil {
		t.Skipf("this platform will not remove the working directory, so filepath.Abs cannot be made to fail here: %v", err)
	}

	// Second premise, for the platforms that do remove it. Removal fails
	// getcwd(3) on Linux and does not on macOS, where Darwin answers from the
	// path it remembers, so RepositoryRoot walks for a go.mod and returns the
	// not-found error instead. Where getcwd still answers there is no
	// filepath.Abs failure to cover.
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform's getcwd still answers after the working directory is removed, so filepath.Abs cannot fail here")
	}

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

func TestProgressf_WritesToStderrWithNewline(t *testing.T) {
	var stderr bytes.Buffer
	previous := progressStderr
	t.Cleanup(func() { progressStderr = previous })
	progressStderr = &stderr

	Progressf("step %d/%d: %s", 2, 3, "working")
	if got, want := stderr.String(), "step 2/3: working\n"; got != want {
		t.Fatalf("Progressf() stderr = %q, want %q", got, want)
	}
}

// TestMust_ReturnsValueWhenErrIsNil verifies that the happy path, the only one
// a correct caller ever takes, returns the value untouched and does not panic.
func TestMust_ReturnsValueWhenErrIsNil(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  func() any
		want any
	}{
		{
			name: "a value is returned unchanged",
			got:  func() any { return Must("catalog", nil) },
			want: "catalog",
		},
		{
			name: "the zero value is returned as readily as any other",
			got:  func() any { return Must(0, nil) },
			want: 0,
		},
		{
			name: "MustDo returns without panicking",
			got:  func() any { MustDo(nil); return "returned" },
			want: "returned",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.got(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMust_PanicsWithTheError verifies that a failure reaching Must stops the
// program and says why. The message has to carry the error itself, because the
// call site cannot add context: Go's f(g()) rule leaves no room for an extra
// argument, so the error text is the only explanation a reader gets.
func TestMust_PanicsWithTheError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		invoke func()
		want   string
	}{
		{
			name:   "Must names itself and carries the error",
			invoke: func() { _ = Must("", os.ErrNotExist) },
			want:   "cmdutil.Must: file does not exist",
		},
		{
			name:   "MustDo names itself and carries the error",
			invoke: func() { MustDo(os.ErrPermission) },
			want:   "cmdutil.MustDo: permission denied",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				raised := recover()
				if raised == nil {
					t.Fatal("no panic; a non-nil error must stop the program")
				}
				if got, ok := raised.(string); !ok || got != tc.want {
					t.Errorf("panic = %v, want %q", raised, tc.want)
				}
			}()
			tc.invoke()
		})
	}
}
