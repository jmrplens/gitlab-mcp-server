// fileutils_errors_test.go contains unit tests for rare filesystem error
// branches in fileutils.go: opening an unreadable file after a successful
// stat, resolving relative paths when the working directory has been
// deleted (os.Getwd failure), and filepath.Rel errors in pathWithinBase.
package toolutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestOpenAndValidateFile_UnreadableFile_ReturnsOpenError verifies that
// [OpenAndValidateFile] surfaces the os.Open error when the file exists and
// passes the stat/regular/size checks but cannot be opened for reading
// (mode 0o000). This exercises the open-failure branch that runs after all
// validations succeed.
func TestOpenAndValidateFile_UnreadableFile_ReturnsOpenError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}

	path := filepath.Join(t.TempDir(), "secret.bin")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	f, info, err := OpenAndValidateFile(path, 0)
	if f != nil {
		f.Close()
	}
	if err == nil {
		t.Fatal("OpenAndValidateFile(unreadable) error = nil, want open error")
	}
	if info != nil {
		t.Errorf("OpenAndValidateFile(unreadable) info = %v, want nil", info)
	}
	if !strings.Contains(err.Error(), "open ") {
		t.Errorf("OpenAndValidateFile(unreadable) error = %q, want it to mention open", err)
	}
}

// TestCanonicalImportArchivePath_DeletedCwd_ReturnsResolveError verifies that
// [CanonicalImportArchivePath] fails cleanly for a relative archive path when
// the current working directory has been removed. On Linux os.Getwd fails and
// the filepath.Abs error branch ("resolve archive path") is exercised; on
// macOS getcwd still succeeds for a deleted cwd (vnode name cache), so the
// failure surfaces at symlink resolution instead — both are resolve errors.
// The same deleted-cwd state verifies that canonicalDirPath also fails for a
// relative allowlist directory.
func TestCanonicalImportArchivePath_DeletedCwd_ReturnsResolveError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("removing the current working directory is not supported on Windows")
	}

	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Skipf("cannot remove current working directory: %v", err)
	}

	_, err := CanonicalImportArchivePath("export.tar.gz")
	if err == nil {
		t.Fatal("CanonicalImportArchivePath(relative, deleted cwd) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "resolve archive") {
		t.Errorf("CanonicalImportArchivePath() error = %q, want a 'resolve archive' error", err)
	}

	if _, dirErr := canonicalDirPath("relative-allowlist-dir"); dirErr == nil {
		t.Error("canonicalDirPath(relative, deleted cwd) error = nil, want error")
	}
}

// TestPathWithinBase_RelError_ReturnsFalse verifies that pathWithinBase
// returns false when filepath.Rel cannot relate the two paths (an absolute
// target against a relative base), instead of panicking or misclassifying.
func TestPathWithinBase_RelError_ReturnsFalse(t *testing.T) {
	if pathWithinBase("/absolute/target", "relative-base") {
		t.Error(`pathWithinBase("/absolute/target", "relative-base") = true, want false`)
	}
}
