// fileutils_test.go contains unit tests for shared file utility functions
// including file validation, SHA-256 checksum computation, progress tracking,
// and package name validation.
package toolutil

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/progress"
)

// TestOpenAndValidateFile_RegularFile verifies that a regular file is accepted.
func TestOpenAndValidateFile_RegularFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, info, err := OpenAndValidateFile(path, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer f.Close()

	if info.Size() != 5 {
		t.Errorf("expected size 5, got %d", info.Size())
	}
}

// TestOpenAndValidateFile_Directory verifies directories are rejected.
func TestOpenAndValidateFile_Directory(t *testing.T) {
	tmp := t.TempDir()

	_, _, err := OpenAndValidateFile(tmp, 1024)
	if err == nil {
		t.Fatal("expected error for directory, got nil")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("expected 'not a regular file' error, got: %v", err)
	}
}

// TestOpenAndValidateFile_NotFound verifies missing files return an error.
func TestOpenAndValidateFile_NotFound(t *testing.T) {
	_, _, err := OpenAndValidateFile("/nonexistent/path/file.txt", 1024)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestOpenAndValidateFile_TooLarge verifies files exceeding maxSize are rejected.
func TestOpenAndValidateFile_TooLarge(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "large.bin")
	if err := os.WriteFile(path, make([]byte, 2048), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := OpenAndValidateFile(path, 1024)
	if err == nil {
		t.Fatal("expected error for too-large file, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("expected 'exceeds maximum' error, got: %v", err)
	}
}

// TestOpenAndValidateFile_EmptyPath verifies empty path is rejected.
func TestOpenAndValidateFile_EmptyPath(t *testing.T) {
	_, _, err := OpenAndValidateFile("", 1024)
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
	if !strings.Contains(err.Error(), "file path is required") {
		t.Errorf("expected 'file path is required' error, got: %v", err)
	}
}

// TestOpenAndValidateFile_ZeroMaxSize verifies maxSize=0 skips size check.
func TestOpenAndValidateFile_ZeroMaxSize(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "any.bin")
	if err := os.WriteFile(path, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}

	f, _, err := OpenAndValidateFile(path, 0)
	if err != nil {
		t.Fatalf("unexpected error with maxSize=0: %v", err)
	}
	f.Close()
}

// TestCanonicalImportArchivePath_TempArchive verifies that a canonical .tar.gz
// archive under the OS temp directory is accepted.
func TestCanonicalImportArchivePath_TempArchive(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "project-export.tar.gz")
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", path, err)
	}

	got, err := CanonicalImportArchivePath(path)
	if err != nil {
		t.Fatalf("CanonicalImportArchivePath() error: %v", err)
	}
	if got != want {
		t.Fatalf("canonical path = %q, want %q", got, want)
	}
}

// TestCanonicalImportArchivePath_RejectsWrongExtension verifies local import
// archives are constrained to GitLab export archive filenames.
func TestCanonicalImportArchivePath_RejectsWrongExtension(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "project-export.zip")
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := CanonicalImportArchivePath(path)
	if err == nil {
		t.Fatal("expected extension validation error")
	}
	if !strings.Contains(err.Error(), ".tar.gz") {
		t.Fatalf("error = %v, want .tar.gz mention", err)
	}
}

// TestCanonicalImportArchivePath_RejectsSymlinkEscape verifies that a symlink
// inside an allowed directory cannot point to an archive outside that directory.
func TestCanonicalImportArchivePath_RejectsSymlinkEscape(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	t.Setenv("TMPDIR", allowed)
	t.Chdir(allowed)

	target := filepath.Join(outside, "project-export.tar.gz")
	if err := os.WriteFile(target, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(allowed, "linked-export.tar.gz")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, err := CanonicalImportArchivePath(link)
	if err == nil {
		t.Fatal("expected symlink escape validation error")
	}
	if !strings.Contains(err.Error(), "outside allowed import directories") {
		t.Fatalf("error = %v, want allowed-directory mention", err)
	}
}

// TestCanonicalImportArchivePath_AllowsConfiguredDirectory verifies that
// GITLAB_MCP_ALLOWED_IMPORT_DIRS extends the allowed local archive roots.
func TestCanonicalImportArchivePath_AllowsConfiguredDirectory(t *testing.T) {
	cwd := t.TempDir()
	configured := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv(ImportArchiveAllowlistEnv, configured)
	t.Chdir(cwd)

	path := filepath.Join(configured, "project-export.tar.gz")
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", path, err)
	}

	got, err := CanonicalImportArchivePath(path)
	if err != nil {
		t.Fatalf("CanonicalImportArchivePath() error: %v", err)
	}
	if got != want {
		t.Fatalf("canonical path = %q, want %q", got, want)
	}
}

// TestCanonicalImportArchivePath_RejectsInvalidInputs verifies archive path
// validation branches before allowlist checks.
func TestCanonicalImportArchivePath_RejectsInvalidInputs(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		_, err := CanonicalImportArchivePath("")
		if err == nil || !strings.Contains(err.Error(), "archive path is required") {
			t.Fatalf("error = %v, want required path error", err)
		}
	})

	t.Run("missing path", func(t *testing.T) {
		_, err := CanonicalImportArchivePath(filepath.Join(t.TempDir(), "missing.tar.gz"))
		if err == nil || !strings.Contains(err.Error(), "resolve archive symlinks") {
			t.Fatalf("error = %v, want symlink resolution error", err)
		}
	})

	t.Run("directory archive", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "project-export.tar.gz")
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		_, err := CanonicalImportArchivePath(dir)
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("error = %v, want regular-file validation error", err)
		}
	})
}

// TestCanonicalImportArchivePath_RejectsUnsafePermissions verifies import
// archives cannot be group- or world-writable on Unix-like systems.
func TestCanonicalImportArchivePath_RejectsUnsafePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced on Windows")
	}
	path := filepath.Join(t.TempDir(), "project-export.tar.gz")
	if err := os.WriteFile(path, []byte("archive"), 0o666); err != nil { //nolint:gosec // Intentionally creates unsafe permissions for validation coverage.
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil { //nolint:gosec // Intentionally creates unsafe permissions for validation coverage.
		t.Fatal(err)
	}

	_, err := CanonicalImportArchivePath(path)
	if err == nil || !strings.Contains(err.Error(), "group/world-writable") {
		t.Fatalf("error = %v, want unsafe permissions error", err)
	}
}

// TestAllowedImportArchiveDirs_SkipsInvalidConfiguredDirectory verifies invalid
// allowlist entries are ignored while valid roots remain available.
func TestAllowedImportArchiveDirs_SkipsInvalidConfiguredDirectory(t *testing.T) {
	base := t.TempDir()
	invalid := filepath.Join(base, "not-a-directory")
	if err := os.WriteFile(invalid, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ImportArchiveAllowlistEnv, invalid)

	allowed := allowedImportArchiveDirs()
	for _, dir := range allowed {
		if dir == invalid {
			t.Fatalf("allowedImportArchiveDirs() included invalid configured file %q", invalid)
		}
	}
}

// TestCanonicalDirPath_RejectsInvalidDirectories covers direct directory
// canonicalization errors used by import-archive allowlisting.
func TestCanonicalDirPath_RejectsInvalidDirectories(t *testing.T) {
	if _, err := canonicalDirPath(""); err == nil {
		t.Fatal("canonicalDirPath(empty) error = nil, want error")
	}
	if _, err := canonicalDirPath(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("canonicalDirPath(missing) error = nil, want error")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := canonicalDirPath(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("canonicalDirPath(file) error = %v, want not-a-directory", err)
	}
}

// TestPathWithinBase verifies direct allowlist path containment checks for
// the base directory itself, child paths, and sibling escapes.
func TestPathWithinBase(t *testing.T) {
	base := t.TempDir()
	child := filepath.Join(base, "nested", "archive.tar.gz")
	sibling := filepath.Join(filepath.Dir(base), filepath.Base(base)+"-sibling", "archive.tar.gz")

	if !pathWithinBase(base, base) {
		t.Fatal("pathWithinBase(base, base) = false, want true")
	}
	if !pathWithinBase(child, base) {
		t.Fatal("pathWithinBase(child, base) = false, want true")
	}
	if pathWithinBase(sibling, base) {
		t.Fatal("pathWithinBase(sibling, base) = true, want false")
	}
}

// TestComputeSHA256_KnownHash verifies a known content produces the expected SHA-256.
func TestComputeSHA256_KnownHash(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "known.txt")
	content := []byte("Hello, World!")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256(content)
	expected := hex.EncodeToString(h[:])

	got, err := ComputeSHA256(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

// TestComputeSHA256_EmptyFile verifies SHA-256 of an empty file.
func TestComputeSHA256_EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "empty.bin")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ComputeSHA256(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != expected {
		t.Errorf("expected empty hash %s, got %s", expected, got)
	}
}

// TestComputeSHA256Reader_FromBytes verifies checksum from a byte reader.
func TestComputeSHA256Reader_FromBytes(t *testing.T) {
	data := []byte("test data for checksum")
	h := sha256.Sum256(data)
	expected := hex.EncodeToString(h[:])

	got, err := ComputeSHA256Reader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

// TestProgressReader_ReportsProgress verifies the progress reader counts bytes
// and calls the progress tracker.
func TestProgressReader_ReportsProgress(t *testing.T) {
	data := make([]byte, 256*1024) // 256 KB
	for i := range data {
		data[i] = byte(i % 256)
	}

	tracker := progress.Tracker{} // inactive tracker — no-op
	pr := NewProgressReader(context.Background(), bytes.NewReader(data), int64(len(data)), tracker)

	buf := make([]byte, 32*1024)
	var totalRead int64
	for {
		n, err := pr.Read(buf)
		totalRead += int64(n)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}
	}

	if totalRead != int64(len(data)) {
		t.Errorf("expected to read %d bytes, got %d", len(data), totalRead)
	}
	if pr.BytesRead() != int64(len(data)) {
		t.Errorf("progressReader.BytesRead() = %d, want %d", pr.BytesRead(), len(data))
	}
}

// TestProgressWriter_ReportsProgress verifies the progress writer counts bytes.
func TestProgressWriter_ReportsProgress(t *testing.T) {
	var buf bytes.Buffer
	data := []byte("some download data for writer test")

	tracker := progress.Tracker{} // inactive tracker
	pw := NewProgressWriter(context.Background(), &buf, int64(len(data)), tracker)

	n, err := pw.Write(data)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected to write %d bytes, got %d", len(data), n)
	}
	if pw.BytesWritten() != int64(len(data)) {
		t.Errorf("progressWriter.BytesWritten() = %d, want %d", pw.BytesWritten(), len(data))
	}
	if buf.String() != string(data) {
		t.Error("written content does not match input")
	}
}

// TestProgressReportInterval verifies the interval calculation logic.
func TestProgressReportInterval(t *testing.T) {
	tests := []struct {
		name    string
		total   int64
		wantMin int64
		wantMax int64
	}{
		{"small file (100KB)", 100 * 1024, 64 * 1024, 64 * 1024},
		{"medium file (10MB)", 10 * 1024 * 1024, 64 * 1024, 1024 * 1024},
		{"large file (100MB)", 100 * 1024 * 1024, 1024 * 1024, 1024 * 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProgressReportInterval(tt.total)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("ProgressReportInterval(%d) = %d, want between %d and %d",
					tt.total, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestValidatePackageName contains table-driven tests for package name validation.
func TestValidatePackageName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "my-package", false},
		{"valid with version path", "my-org/my-package", false},
		{"valid alphanumeric", "pkg123", false},
		{"valid with dots", "com.example.pkg", false},
		{"valid with plus", "my+package", false},
		{"valid with tilde", "my~package", false},
		{"valid with at", "my@package", false},
		{"empty", "", true},
		{"starts with dot", ".hidden", true},
		{"starts with dash", "-invalid", true},
		{"contains space", "my package", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePackageName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// TestValidatePackageFileName contains table-driven tests for filename validation.
func TestValidatePackageFileName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "file.txt", false},
		{"valid complex", "my-pkg_v1.0+build.tar.gz", false},
		{"valid with dots", "archive.tar.gz", false},
		{"empty", "", true},
		{"contains space", "my file.txt", true},
		{"starts with tilde", "~tempfile", true},
		{"starts with at", "@scope", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageFileName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePackageFileName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// TestSetGet_UploadConfig verifies that SetUploadConfig stores custom values
// and GetUploadConfig retrieves them. Restores defaults after the test.
func TestSetGet_UploadConfig(t *testing.T) {
	orig := GetUploadConfig()
	t.Cleanup(func() {
		SetUploadConfig(orig.MaxFileSize)
	})

	SetUploadConfig(4096)
	got := GetUploadConfig()

	if got.MaxFileSize != 4096 {
		t.Errorf("MaxFileSize = %d, want 4096", got.MaxFileSize)
	}
}

// TestComputeSHA256_NonexistentFile verifies that ComputeSHA256 returns an
// error when the file does not exist (covers the os.Open error branch).
func TestComputeSHA256_NonexistentFile(t *testing.T) {
	_, err := ComputeSHA256(filepath.Join(t.TempDir(), "does-not-exist.bin"))
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

// TestComputeSHA256Reader_ErrorReader verifies that ComputeSHA256Reader
// propagates errors from a failing io.Reader (covers the io.Copy error branch).
func TestComputeSHA256Reader_ErrorReader(t *testing.T) {
	failReader := &errReader{err: io.ErrUnexpectedEOF}
	_, err := ComputeSHA256Reader(failReader)
	if err == nil {
		t.Fatal("expected error from failing reader, got nil")
	}
}

// errReader is a test helper that always returns the configured error.
type errReader struct{ err error }

// Read streams data from errReader into p.
func (r *errReader) Read([]byte) (int, error) { return 0, r.err }

// TestProgressWriter_ReportsAtInterval verifies that the progress report
// branch triggers when written bytes exceed the report interval threshold.
func TestProgressWriter_ReportsAtInterval(t *testing.T) {
	var buf bytes.Buffer
	total := int64(100)

	tracker := progress.Tracker{}
	pw := NewProgressWriter(context.Background(), &buf, total, tracker)

	// Force a very small interval so the write triggers the report branch.
	pw.interval = 1

	data := []byte("hello world")
	n, err := pw.Write(data)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("wrote %d bytes, want %d", n, len(data))
	}
	if pw.lastReport != int64(len(data)) {
		t.Errorf("lastReport = %d, want %d (should update after interval)", pw.lastReport, len(data))
	}
}
