//go:build !windows

// packages_stream_unix_test.go covers the destination-type rule of
// streamDownloadPackageFile (packages_stream.go) against a named pipe (FIFO),
// the one non-regular file a test can create without privileges.
// syscall.Mkfifo has no Windows equivalent, hence the build tag.
package packages

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestStreamDownload_FIFOOutputPath_Refused verifies that a download refuses a
// destination that already exists and is not a regular file, and that it
// refuses before requesting the package: the mock handler fails the test if it
// is reached at all.
//
// A FIFO is the benign member of that class. The dangerous one is a symlink,
// which os.Create follows to wherever it points — that is how an "output path"
// inside the workspace overwrote a file outside it. The rule is written once,
// on the type of the destination, so both are refused by the same check.
//
// This test used to assert the opposite: it pointed output_path at a pipe on
// purpose to reach the outFile.Sync() error branch, which fsync(2) fails with
// EINVAL. That branch is now unreachable from a caller-supplied path, and is
// documented as such in TestStreamDownload_DeadBranches.
func TestStreamDownload_FIFOOutputPath_Refused(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	fifoPath := filepath.Join(t.TempDir(), "output.fifo")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	_, _, err := streamDownloadPackageFile(
		t.Context(),
		nil,
		client,
		DownloadInput{
			ProjectID:      "42",
			PackageName:    testPackageName,
			PackageVersion: testPkgVersion,
			FileName:       testAppBin,
			OutputPath:     fifoPath,
		},
	)
	if err == nil {
		t.Fatal("streamDownloadPackageFile(fifo) error = nil, want the not-a-regular-file refusal")
	}
	if !strings.Contains(err.Error(), "is not a regular file") {
		t.Fatalf("error = %q, want the not-a-regular-file refusal", err.Error())
	}
	info, statErr := os.Lstat(fifoPath)
	if statErr != nil {
		t.Fatalf("os.Lstat(%q) error = %v", fifoPath, statErr)
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		t.Errorf("os.Lstat(%q) mode = %v, want the pipe left as it was", fifoPath, info.Mode())
	}
}
