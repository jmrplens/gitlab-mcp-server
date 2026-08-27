//go:build unix

// fileutils_unix_test.go contains Unix-only unit tests for fileutils.go that
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
