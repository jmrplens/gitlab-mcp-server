//go:build unix

// file_utils_unix_test.go contains Unix-only unit tests for file_utils.go that
// need OS-level resource-limit control unavailable on Windows.
package toolutil

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestOpenAndValidateFile_FileDescriptorLimitReached_ReturnsOpenError
// verifies that OpenAndValidateFile surfaces the os.Open error when the
// process runs out of file descriptors after os.Stat has already validated
// the path as a regular file within the size bound.
//
// This exercises the open-failure branch (the `open %s: %w` wrap) that
// cannot be reached with a missing-file or permission-bit trick: tests in
// this suite may run as root, which bypasses DAC permission checks (see
// TestOpenAndValidateFile_UnreadableFile_ReturnsOpenError, which skips in
// that case), and a missing path fails earlier at the os.Stat call instead
// of at os.Open. Temporarily pinning the process's own soft RLIMIT_NOFILE
// below its current descriptor count makes the very next os.Open call fail
// with EMFILE deterministically, regardless of privilege level, and the
// original limit is restored before the test returns.
func TestOpenAndValidateFile_FileDescriptorLimitReached_ReturnsOpenError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var orig syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &orig); err != nil {
		t.Skipf("cannot read RLIMIT_NOFILE: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &orig) })

	// A live Go test binary always holds well more than 3 descriptors
	// (stdio plus runtime-internal fds such as the netpoller), so pinning
	// the soft limit to 3 guarantees the next os.Open has no room left,
	// without needing to enumerate the exact current count.
	tight := syscall.Rlimit{Cur: 3, Max: orig.Max}
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &tight); err != nil {
		t.Skipf("cannot lower RLIMIT_NOFILE: %v", err)
	}

	f, info, err := OpenAndValidateFile(path, 0)

	// Restore immediately so any later test-framework activity in this
	// process is not affected by the tightened limit.
	if restoreErr := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &orig); restoreErr != nil {
		t.Fatalf("restore RLIMIT_NOFILE: %v", restoreErr)
	}

	if f != nil {
		f.Close()
	}
	if err == nil {
		t.Fatal("OpenAndValidateFile(fd limit reached) error = nil, want open error")
	}
	if info != nil {
		t.Errorf("OpenAndValidateFile(fd limit reached) info = %v, want nil", info)
	}
	if !strings.Contains(err.Error(), "open ") {
		t.Errorf("OpenAndValidateFile(fd limit reached) error = %q, want it to mention open", err)
	}
}

// TestOpenLeafNoFollow_SymlinkAtTheLeaf_Refused verifies that the two leaf-open
// primitives refuse a symlink at the last path component rather than following
// it.
//
// The path checks in file_utils.go run on a path string: EvalSymlinks resolves
// it, Lstat proves the result is a regular file, and only then does the open
// happen. A local principal able to write in an allowed root (the OS temp
// directory is always one, and /tmp is world-writable) can replace the leaf
// with a symlink in between, and os.Open and os.Create both follow one, so the
// file read or written is not the file that was checked. The refusal has to
// live in the open itself, which is what this pins.
func TestOpenLeafNoFollow_SymlinkAtTheLeaf_Refused(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	link := filepath.Join(dir, "swapped.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	tests := []struct {
		name    string
		open    func(string) (*os.File, error)
		path    string
		wantErr bool
	}{
		{name: "read refuses a symlink at the leaf", open: openLeafNoFollow, path: link, wantErr: true},
		{name: "read opens a regular file", open: openLeafNoFollow, path: target, wantErr: false},
		{name: "create refuses a symlink at the leaf", open: createLeafNoFollow, path: link, wantErr: true},
		{name: "create makes a new file", open: createLeafNoFollow, path: filepath.Join(dir, "new.txt"), wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := tt.open(tt.path)
			if err == nil {
				defer func() { _ = f.Close() }()
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("open(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}

	// os.ReadFile follows the symlink, which is exactly what the calls above
	// used to do, and it is what makes their refusals meaningful: the target is
	// readable, so they declined to follow rather than failing to find it.
	if _, err := os.ReadFile(link); err != nil {
		t.Errorf("the symlink target became unreadable, so the refusals above prove nothing: %v", err)
	}
}
