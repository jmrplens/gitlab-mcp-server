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
	// sysfs is nobody's workspace, so the allow-list has to name it before the
	// short-read branch is reachable at all.
	t.Setenv(UploadDirAllowlistEnv, "/sys")

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

	// Far past the limit, which is the case the cheap pre-decode check exists
	// for: base64.DecodedLen overestimates by up to two bytes, so a payload
	// only just over the limit falls through to the exact check after
	// decoding, and only a clearly oversized one is refused without allocating
	// its decoded form at all.
	t.Run("an oversized payload is refused before it is decoded", func(t *testing.T) {
		huge := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 1024)))
		if _, _, _, err := OpenFileOrBase64Source("op", "", huge); err == nil ||
			!strings.Contains(err.Error(), "would exceed maximum allowed size") {
			t.Errorf("OpenFileOrBase64Source oversized err = %v, want the pre-decode size refusal", err)
		}
	})
}

// TestOpenFileOrBase64Source_ZeroStatSizeFile_Bounded verifies that the
// streaming branch bounds what it reads by the configured limit rather than by
// the size os.Stat reports. A procfs entry is a regular file whose reported
// size is zero and whose content is not, which is how /proc/self/environ — the
// server's own credentials — streamed past a size cap that only ever looked at
// os.Stat.
func TestOpenFileOrBase64Source_ZeroStatSizeFile_Bounded(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("procfs zero-size regular files exist only on Linux")
	}
	const procDir = "/proc"
	if _, err := os.Stat("/proc/self/environ"); err != nil {
		t.Skipf("procfs unavailable: %v", err)
	}
	t.Setenv(UploadDirAllowlistEnv, procDir)

	const limit = 8
	original := GetUploadConfig()
	SetUploadConfig(limit)
	t.Cleanup(func() { SetUploadConfig(original.MaxFileSize) })

	reader, size, cleanup, err := OpenFileOrBase64Source("op", "/proc/self/environ", "")
	if err != nil {
		// Not a skip. The environment gates above already ran, so an error here
		// is the allow-list or the open path refusing for some new reason, and
		// skipping would report a regression in the containment as a pass.
		t.Fatalf("OpenFileOrBase64Source() error = %v, want the procfs entry opened through the allow-list", err)
	}
	defer cleanup()
	if size != 0 {
		t.Fatalf("procfs entry reported size %d, want 0: the zero-size shape is what this test exists to bound", size)
	}

	read, err := io.ReadAll(reader)
	if err == nil {
		t.Fatalf("io.ReadAll() read %d bytes with no error, want the configured limit to stop it", len(read))
	}
	if !strings.Contains(err.Error(), "exceeds maximum allowed size") {
		t.Errorf("io.ReadAll() error = %q, want the size-limit refusal", err)
	}
	if len(read) > limit {
		t.Errorf("io.ReadAll() read %d bytes, want no more than the configured limit of %d", len(read), limit)
	}
}

// TestNewLimitedFileReader_BoundsWhatItYields verifies the streaming bound on a
// synthetic source, so the limit is proven without procfs and stays proven on
// the platforms and in the sandboxes where procfs is not there to prove it.
//
// The boundary is the whole point: a source of exactly the limit reads to EOF,
// and one byte past it is what the reader refuses, because a source that lies
// about its size is only detectable by reading further than it claimed.
func TestNewLimitedFileReader_BoundsWhatItYields(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		maxSize  int64
		wantRead int
		wantErr  bool
	}{
		{name: "empty source", content: "", maxSize: 8, wantRead: 0},
		{name: "under the limit", content: "abc", maxSize: 8, wantRead: 3},
		{name: "exactly the limit", content: "abcdefgh", maxSize: 8, wantRead: 8},
		{name: "one byte over the limit", content: "abcdefghi", maxSize: 8, wantRead: 8, wantErr: true},
		{name: "far over the limit", content: strings.Repeat("x", 4096), maxSize: 8, wantRead: 8, wantErr: true},
		{name: "no limit configured", content: strings.Repeat("x", 4096), maxSize: 0, wantRead: 4096},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newLimitedFileReader("op", strings.NewReader(tt.content), tt.maxSize)
			read, err := io.ReadAll(reader)
			if (err != nil) != tt.wantErr {
				t.Fatalf("io.ReadAll() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), "op: file exceeds maximum allowed size") {
				t.Errorf("io.ReadAll() error = %q, want the op-prefixed size refusal", err)
			}
			if len(read) > tt.wantRead {
				t.Errorf("io.ReadAll() yielded %d bytes, want no more than %d", len(read), tt.wantRead)
			}
		})
	}
}
