// deferred_test.go contains unit tests for deferred auto-update file operations.
package autoupdate

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeBinary returns a byte slice with valid PE (MZ) magic header padded
// to at least minBinarySize so it passes all writeToFile validations.
func fakeBinary() []byte {
	buf := make([]byte, minBinarySize+1)
	buf[0] = 'M'
	buf[1] = 'Z'
	return buf
}

// TestHasPendingUpdate_NoFile verifies HasPendingUpdate returns false when
// no staged binary exists.
func TestHasPendingUpdate_NoFile(t *testing.T) {
	path, ok := HasPendingUpdate()
	if ok {
		t.Errorf("expected no pending update, got path: %s", path)
	}
}

// TestWriteToFile_ValidBinary verifies a valid binary is written and permissions set.
func TestWriteToFile_ValidBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-binary")

	if err := writeToFile(path, bytes.NewReader(fakeBinary())); err != nil {
		t.Fatalf("writeToFile: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() < minBinarySize {
		t.Errorf("size = %d, want >= %d", info.Size(), minBinarySize)
	}

	if runtime.GOOS != "windows" {
		if info.Mode()&0o111 == 0 {
			t.Error("expected executable permission on non-Windows")
		}
	}
}

// TestWriteToFile_InvalidPath verifies writeToFile returns an error for
// a non-existent directory.
func TestWriteToFile_InvalidPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "binary")
	err := writeToFile(path, bytes.NewReader(fakeBinary()))
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

// TestWriteToFile_TooSmall verifies writeToFile rejects files smaller than
// minBinarySize (the exact scenario from issue #2: a JSON error response).
func TestWriteToFile_TooSmall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small-file")

	data := []byte(`{"message":"404 Not found"}`)
	err := writeToFile(path, bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error for small file")
	}
	if !errors.Is(err, errNotBinary) {
		t.Errorf("err = %v, want errNotBinary", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("staging file should be removed after validation failure")
	}
}

// TestWriteToFile_InvalidMagicBytes verifies writeToFile rejects a large file
// that does not have valid executable magic bytes.
func TestWriteToFile_InvalidMagicBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-magic")

	buf := make([]byte, minBinarySize+1)
	buf[0] = 0xFF
	buf[1] = 0xFF

	err := writeToFile(path, bytes.NewReader(buf))
	if err == nil {
		t.Fatal("expected error for invalid magic bytes")
	}
	if !errors.Is(err, errNotBinary) {
		t.Errorf("err = %v, want errNotBinary", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("staging file should be removed after validation failure")
	}
}

// TestValidateBinaryMagic_AllFormats verifies all supported executable formats.
func TestValidateBinaryMagic_AllFormats(t *testing.T) {
	tests := []struct {
		name   string
		header []byte
	}{
		{name: "ELF", header: []byte{0x7f, 'E', 'L', 'F'}},
		{name: "MachO-64-LE", header: []byte{0xCF, 0xFA, 0xED, 0xFE}},
		{name: "MachO-64-BE", header: []byte{0xFE, 0xED, 0xFA, 0xCF}},
		{name: "MachO-Universal", header: []byte{0xCA, 0xFE, 0xBA, 0xBE}},
		{name: "PE-MZ", header: []byte{'M', 'Z', 0x00, 0x00}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "binary")
			if err := os.WriteFile(path, tt.header, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := validateBinaryMagic(path); err != nil {
				t.Errorf("validateBinaryMagic(%s) = %v, want nil", tt.name, err)
			}
		})
	}
}

// TestValidateBinaryMagic_Rejected verifies unknown magic bytes are rejected.
func TestValidateBinaryMagic_Rejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-binary")
	if err := os.WriteFile(path, []byte(`{"message":"404 Not found"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := validateBinaryMagic(path)
	if err == nil {
		t.Fatal("expected error for JSON content")
	}
	if !errors.Is(err, errNotBinary) {
		t.Errorf("err = %v, want errNotBinary", err)
	}
}

// TestValidateBinaryMagic_NonExistentFile verifies that validateBinaryMagic
// returns an error when the file does not exist (os.Open error path).
func TestValidateBinaryMagic_NonExistentFile(t *testing.T) {
	err := validateBinaryMagic(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

// TestValidateBinaryMagic_TooShortForHeader verifies that validateBinaryMagic
// rejects files shorter than 4 bytes where io.ReadFull cannot fill the header.
func TestValidateBinaryMagic_TooShortForHeader(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty file", data: []byte{}},
		{name: "1 byte", data: []byte{0x7f}},
		{name: "3 bytes", data: []byte{0x7f, 'E', 'L'}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "short")
			if err := os.WriteFile(path, tt.data, 0o644); err != nil {
				t.Fatal(err)
			}
			err := validateBinaryMagic(path)
			if err == nil {
				t.Fatal("expected error for short file")
			}
			if !errors.Is(err, errNotBinary) {
				t.Errorf("err = %v, want errNotBinary", err)
			}
		})
	}
}

// TestHasPendingUpdate_WithTmpFile verifies HasPendingUpdate detects a .tmp
// staged binary next to the current executable.
func TestHasPendingUpdate_WithTmpFile(t *testing.T) {
	exe := stubExecutablePath(t)
	tmpPath := exe + ".tmp"
	if err := os.WriteFile(tmpPath, []byte("staged"), 0o644); err != nil {
		t.Fatalf("cannot create .tmp file: %v", err)
	}

	path, ok := HasPendingUpdate()
	if !ok {
		t.Fatal("expected pending update for .tmp file")
	}
	if path != tmpPath {
		t.Errorf("path = %q, want %q", path, tmpPath)
	}
}

// TestHasPendingUpdate_WithNewFile verifies HasPendingUpdate detects a .new
// staged binary when no .tmp file exists.
func TestHasPendingUpdate_WithNewFile(t *testing.T) {
	exe := stubExecutablePath(t)

	newPath := exe + ".new"
	if err := os.WriteFile(newPath, []byte("staged"), 0o644); err != nil {
		t.Fatalf("cannot create .new file: %v", err)
	}

	path, ok := HasPendingUpdate()
	if !ok {
		t.Fatal("expected pending update for .new file")
	}
	if path != newPath {
		t.Errorf("path = %q, want %q", path, newPath)
	}
}

// failReader is an io.Reader that always returns an error.
type failReader struct{}

// Read implements [io.Reader] by failing before any bytes are copied.
func (failReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated read failure")
}

// TestWriteToFile_ReaderError verifies that writeToFile removes the file and
// returns an error when io.Copy fails due to a reader error.
func TestWriteToFile_ReaderError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary")
	err := writeToFile(path, failReader{})
	if err == nil {
		t.Fatal("expected error for reader failure")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("file should be removed after write failure")
	}
}

// TestWriteToFile_TooSmallAfterCopy verifies that writeToFile removes the
// file when the reader provides data smaller than minBinarySize but the copy
// itself succeeds without error.
func TestWriteToFile_TooSmallAfterCopy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiny")
	// 100 bytes of valid-looking data — well below minBinarySize.
	err := writeToFile(path, bytes.NewReader(make([]byte, 100)))
	if err == nil {
		t.Fatal("expected size validation error")
	}
	if !errors.Is(err, errNotBinary) {
		t.Errorf("err = %v, want errNotBinary", err)
	}
}

// partialReader returns n bytes successfully then fails with an error.
type partialReader struct {
	data []byte
	pos  int
	err  error
}

// Read implements [io.Reader] by returning buffered bytes and then the
// configured error once the buffer is exhausted.
func (r *partialReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos >= len(r.data) {
		return n, r.err
	}
	return n, nil
}

// TestWriteToFile_PartialCopyError verifies that writeToFile cleans up when
// the reader returns data followed by an unexpected error.
func TestWriteToFile_PartialCopyError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial")
	data := make([]byte, 512)
	err := writeToFile(path, &partialReader{data: data, err: io.ErrUnexpectedEOF})
	if err == nil {
		t.Fatal("expected error for partial read")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("file should be removed after partial write failure")
	}
}

// TestHasPendingUpdate_ResolveError verifies HasPendingUpdate returns false
// when resolveExecutable fails (e.g. binary deleted from under us).
func TestHasPendingUpdate_ResolveError(t *testing.T) {
	orig := resolveExecutable
	resolveExecutable = func() (string, error) {
		return "", errors.New("cannot resolve")
	}
	t.Cleanup(func() { resolveExecutable = orig })

	path, ok := HasPendingUpdate()
	if ok {
		t.Errorf("expected no pending update, got path=%q", path)
	}
}

// TestHasPendingUpdate_NoStagedFiles verifies HasPendingUpdate returns false
// when neither .tmp nor .new files exist next to the executable.
func TestHasPendingUpdate_NoStagedFiles(t *testing.T) {
	stubExecutablePath(t)
	path, ok := HasPendingUpdate()
	if ok {
		t.Errorf("expected no pending update, got path=%q", path)
	}
}
