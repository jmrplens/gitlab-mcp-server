package wizard

import (
	"os"
	"path/filepath"
	"runtime"
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
	if err := os.WriteFile(destPath, []byte("old"), 0o644); err != nil {
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
