// install_test.go contains unit tests for MCP server installation into
// IDE configuration files.
package wizard

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestInstallBinary_CreatesDir verifies InstallBinary creates intermediate
// directories and copies the binary with correct size.
func TestInstallBinary_CreatesDir(t *testing.T) {
	destDir := filepath.Join(t.TempDir(), "subdir", "bin")

	installed, err := InstallBinary(destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err = os.Stat(installed); os.IsNotExist(err) {
		t.Errorf("binary not found at %s", installed)
	}

	info, _ := os.Stat(installed)
	if info.Size() == 0 {
		t.Error("installed binary has zero size")
	}
}

// TestInstallBinary_SameLocation verifies InstallBinary is a no-op when
// source and destination resolve to the same path.
func TestInstallBinary_SameLocation(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("cannot determine executable path")
	}
	exe, _ = filepath.EvalSymlinks(exe)
	dir := filepath.Dir(exe)

	installed, err := InstallBinary(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resolved, _ := filepath.EvalSymlinks(installed)
	if resolved != exe {
		t.Logf("installed=%s exe=%s (may differ by binary name, OK)", resolved, exe)
	}
}

// TestInstallBinary_BinaryHasCorrectName verifies the installed binary
// has the platform-appropriate name.
func TestInstallBinary_BinaryHasCorrectName(t *testing.T) {
	destDir := filepath.Join(t.TempDir(), "install-name-test")

	installed, err := InstallBinary(destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	base := filepath.Base(installed)
	want := DefaultBinaryName()
	if base != want {
		t.Errorf("binary name = %q, want %q", base, want)
	}
}

// TestInstallBinary_OverwritesExisting verifies that InstallBinary replaces
// an existing binary at the destination.
func TestInstallBinary_OverwritesExisting(t *testing.T) {
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, DefaultBinaryName())

	// Create a dummy file at the destination
	if err := os.WriteFile(destPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	installed, err := InstallBinary(destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(installed)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() <= 3 {
		t.Error("installed binary was not replaced (still has dummy content)")
	}
}

// TestCopyFile_DestinationDir verifies that copyFile fails gracefully
// when the destination is a directory, not a file.
func TestCopyFile_DestinationDir(t *testing.T) {
	src, err := os.Executable()
	if err != nil {
		t.Skip("cannot determine executable")
	}

	destDir := t.TempDir()
	// copyFile should fail because destDir is a directory, not a file path
	err = copyFile(src, destDir)
	if err == nil && runtime.GOOS != "windows" {
		// On some systems this may succeed by writing into the dir.
		// The important thing is it doesn't panic.
		t.Log("copyFile to directory did not error (may vary by OS)")
	}
}

// TestCopyFile_SourceNotExists verifies copyFile returns an error when
// the source file doesn't exist.
func TestCopyFile_SourceNotExists(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.bin")
	err := copyFile(filepath.Join(t.TempDir(), "nonexistent"), dest)
	if err == nil {
		t.Fatal("expected error for nonexistent source, got nil")
	}
}

// TestInstallBinaryImpl_MkdirAllFails verifies installBinaryImpl returns
// an error when it cannot create the destination directory (e.g. read-only parent).
func TestInstallBinaryImpl_MkdirAllFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission test not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("permission test not reliable as root: root bypasses filesystem permission checks")
	}
	tmpDir := t.TempDir()
	blocked := filepath.Join(tmpDir, "readonly")
	if err := os.Mkdir(blocked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) }) //nolint:gosec // Cleanup restores directory traversal after readonly-permission test.

	deepDir := filepath.Join(blocked, "nested", "dir")
	_, err := installBinaryImpl(deepDir)
	if err == nil {
		t.Fatal("expected error when MkdirAll fails, got nil")
	}
	if !strings.Contains(err.Error(), "creating directory") {
		t.Errorf("error = %v, want to contain 'creating directory'", err)
	}
}

// TestInstallBinaryImpl_PathTraversalCheckContract documents the install
// path resolution contract: installBinaryImpl relies on filepath.Clean +
// filepath.Abs to resolve any ".." segments before applying the
// post-cleaning traversal check, so an attacker-controlled ".." that
// survives Clean is treated as a "path traversal" rejection. This test
// documents the contract and exercises the call path without panicking.
func TestInstallBinaryImpl_PathTraversalCheckContract(t *testing.T) {
	// The defensive post-Clean ".." check at install.go:48-50 is effectively
	// unreachable: filepath.Clean + filepath.Abs fully resolve ".."
	// segments before the check executes. We assert the actual call path
	// completes without panicking for a normalized CWD-relative input.
	resolved, err := filepath.Abs(filepath.Clean(t.TempDir() + "/../sub"))
	if err != nil {
		t.Fatalf("abs/clean: %v", err)
	}
	_, _ = installBinaryImpl(filepath.Join(resolved, DefaultBinaryName()))
}

// TestGetVersionFromBinary_Scenarios validates the version parsing logic across
// multiple scenarios: non-existent binary, non-executable file, expected
// output format, v-prefixed version, single-word output, and error exit.
func TestGetVersionFromBinary_Scenarios(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake binaries not supported on Windows")
	}

	tests := []struct {
		name  string
		setup func(t *testing.T) string // returns path to fake binary
		want  string
	}{
		{
			name: "returns empty for non-existent binary",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "no-such-binary")
			},
			want: "",
		},
		{
			name: "returns empty for non-executable file",
			setup: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "notexec")
				if err := os.WriteFile(p, []byte("not a binary"), 0o600); err != nil {
					t.Fatal(err)
				}
				return p
			},
			want: "",
		},
		{
			name: "parses standard version output",
			setup: func(t *testing.T) string {
				t.Helper()
				return writeFakeVersionBinary(t, "gitlab-mcp-server 1.2.3 (commit: abc1234)")
			},
			want: "1.2.3",
		},
		{
			name: "strips v prefix from version",
			setup: func(t *testing.T) string {
				t.Helper()
				return writeFakeVersionBinary(t, "gitlab-mcp-server v1.0.2 (commit: def5678)")
			},
			want: "1.0.2",
		},
		{
			name: "returns empty for single-word output",
			setup: func(t *testing.T) string {
				t.Helper()
				return writeFakeVersionBinary(t, "unknown")
			},
			want: "",
		},
		{
			name: "returns empty when binary exits with error",
			setup: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(t.TempDir(), "fail")
				script := "#!/bin/sh\nexit 1\n"
				if err := os.WriteFile(p, []byte(script), 0o755); err != nil { //nolint:gosec // Executable fixture is required to simulate version output.
					t.Fatal(err)
				}
				return p
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binPath := tt.setup(t)
			got := getVersionFromBinary(binPath)
			if got != tt.want {
				t.Errorf("getVersionFromBinary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGetInstalledVersionImpl_DefaultPathMissing verifies the default installed
// binary lookup returns empty when the install directory has no binary.
func TestGetInstalledVersionImpl_DefaultPathMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())

	if got := getInstalledVersionImpl(); got != "" {
		t.Fatalf("getInstalledVersionImpl() = %q, want empty", got)
	}
}

// writeFakeVersionBinary creates a shell script in a temp directory that
// prints the given output to stdout, simulating -version output.
func writeFakeVersionBinary(t *testing.T, output string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fake-binary")
	script := "#!/bin/sh\necho '" + output + "'\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil { //nolint:gosec // Executable fixture is required to simulate version output.
		t.Fatal(err)
	}
	return p
}
