package toolutil

import (
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestOpenFileOrBase64Source_Validation pins the mutually-exclusive input
// contract: both sources set and neither source set fail with the op-prefixed
// messages shared by every upload tool.
func TestOpenFileOrBase64Source_Validation(t *testing.T) {
	_, _, _, err := OpenFileOrBase64Source("myOp", "/tmp/x", "aGk=")
	if err == nil || err.Error() != "myOp: provide either file_path or content_base64, not both" {
		t.Errorf("both-set err = %v, want op-prefixed not-both message", err)
	}

	_, _, _, err = OpenFileOrBase64Source("myOp", "", "")
	if err == nil || err.Error() != "myOp: either file_path or content_base64 is required" {
		t.Errorf("neither-set err = %v, want op-prefixed required message", err)
	}
}

// TestOpenFileOrBase64Source_File verifies the streaming file branch: the
// reader yields the file content, size matches, and cleanup closes the file.
func TestOpenFileOrBase64Source_File(t *testing.T) {
	path := filepath.Join(t.TempDir(), "src.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	reader, size, cleanup, err := OpenFileOrBase64Source("myOp", path, "")
	if err != nil {
		t.Fatalf("file branch err = %v, want nil", err)
	}
	defer cleanup()
	if size != 5 {
		t.Errorf("size = %d, want 5", size)
	}
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "hello" {
		t.Errorf("read = %q (err %v), want hello", data, err)
	}
}

// TestOpenFileOrBase64Source_Base64 verifies the base64 branch: valid content
// decodes with the right size, invalid content fails with the op-prefixed
// base64 message.
func TestOpenFileOrBase64Source_Base64(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("payload"))
	reader, size, cleanup, err := OpenFileOrBase64Source("myOp", "", encoded)
	if err != nil {
		t.Fatalf("base64 branch err = %v, want nil", err)
	}
	defer cleanup()
	if size != int64(len("payload")) {
		t.Errorf("size = %d, want %d", size, len("payload"))
	}
	data, _ := io.ReadAll(reader)
	if string(data) != "payload" {
		t.Errorf("read = %q, want payload", data)
	}

	_, _, _, err = OpenFileOrBase64Source("myOp", "", "!!!not-base64!!!")
	if err == nil || !strings.Contains(err.Error(), "myOp: invalid base64 content") {
		t.Errorf("invalid base64 err = %v, want op-prefixed base64 message", err)
	}
}

// TestReadFileOrBase64 covers the in-memory variant: validation errors,
// file read, missing file, and base64 decode.
func TestReadFileOrBase64(t *testing.T) {
	if _, err := ReadFileOrBase64("op", "", ""); err == nil {
		t.Error("neither-set err = nil, want required message")
	}

	path := filepath.Join(t.TempDir(), "src.bin")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := ReadFileOrBase64("op", path, "")
	if err != nil || r.Len() != 3 {
		t.Errorf("file branch = len %v err %v, want len 3 nil", r, err)
	}

	if _, err = ReadFileOrBase64("op", "/nonexistent/file", ""); err == nil || !strings.HasPrefix(err.Error(), "op: ") {
		t.Errorf("missing file err = %v, want op-prefixed error", err)
	}

	r, err = ReadFileOrBase64("op", "", base64.StdEncoding.EncodeToString([]byte("zz")))
	if err != nil || r.Len() != 2 {
		t.Errorf("base64 branch = len %v err %v, want len 2 nil", r, err)
	}
	if _, err = ReadFileOrBase64("op", "", "!!!"); err == nil || !strings.Contains(err.Error(), "invalid base64 content") {
		t.Errorf("invalid base64 err = %v, want base64 message", err)
	}
}

// TestReadFileOrBase64_TruncatedReadReturnsWrappedError verifies that
// ReadFileOrBase64 wraps an io.ReadFull failure (fewer bytes available than
// os.Stat reported) instead of silently returning a short/zero-padded
// buffer, so callers never mistake a truncated read for the real content.
//
// Real short reads of an already-stat'd, already-opened regular file are a
// TOCTOU race in general, but Linux sysfs attribute files give a
// deterministic, non-racy way to trigger the exact same code path: sysfs
// reports a fixed st_size of one page (4096 bytes) for attribute files
// regardless of their actual content length, so os.Stat over-promises the
// size and the subsequent io.ReadFull genuinely hits io.ErrUnexpectedEOF.
// /sys/class/net/lo/mtu (the loopback interface's MTU, present on every
// Linux system with loopback networking) is used as a stable instance of
// this kernel behavior.
func TestReadFileOrBase64_TruncatedReadReturnsWrappedError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("relies on a Linux sysfs attribute-file size quirk")
	}
	const path = "/sys/class/net/lo/mtu"
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Skipf("cannot stat %s: %v", path, statErr)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Skipf("cannot read %s: %v", path, readErr)
	}
	if info.Size() <= int64(len(data)) {
		t.Skipf("%s no longer over-reports its size (stat=%d, actual=%d); sysfs quirk not present", path, info.Size(), len(data))
	}

	_, err := ReadFileOrBase64("op", path, "")
	if err == nil {
		t.Fatal("ReadFileOrBase64() error = nil, want wrapped read error for a short read")
	}
	if !strings.Contains(err.Error(), "op: reading file:") {
		t.Errorf("ReadFileOrBase64() error = %q, want it prefixed with %q", err, "op: reading file:")
	}
}

// TestFileOrBase64_Base64SizeLimit verifies the content_base64 branch honors
// the configured MaxFileSize, so inline payloads cannot bypass the limit that
// OpenAndValidateFile enforces on file_path sources.
func TestFileOrBase64_Base64SizeLimit(t *testing.T) {
	original := GetUploadConfig()
	SetUploadConfig(4)
	t.Cleanup(func() { SetUploadConfig(original.MaxFileSize) })

	oversized := base64.StdEncoding.EncodeToString([]byte("12345"))
	if _, _, _, err := OpenFileOrBase64Source("op", "", oversized); err == nil ||
		!strings.Contains(err.Error(), "exceeds maximum allowed size") {
		t.Errorf("OpenFileOrBase64Source oversized err = %v, want size-limit error", err)
	}
	if _, err := ReadFileOrBase64("op", "", oversized); err == nil ||
		!strings.Contains(err.Error(), "exceeds maximum allowed size") {
		t.Errorf("ReadFileOrBase64 oversized err = %v, want size-limit error", err)
	}

	within := base64.StdEncoding.EncodeToString([]byte("1234"))
	if _, _, _, err := OpenFileOrBase64Source("op", "", within); err != nil {
		t.Errorf("OpenFileOrBase64Source within-limit err = %v, want nil", err)
	}
}
