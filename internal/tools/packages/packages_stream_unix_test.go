//go:build !windows

// packages_stream_sync_unix_test.go covers the outFile.Sync() error branch of
// streamDownloadPackageFile (packages_stream.go) using a named pipe (FIFO) as
// the output path. fsync(2) on a FIFO genuinely fails with EINVAL on Linux and
// other Unix systems -- this is not a simulated/faked OS error, just a
// filesystem object type real code can encounter (e.g. a caller pointing
// output_path at a pipe). syscall.Mkfifo has no Windows equivalent, hence the
// build tag; the branch is exercised only where the underlying mechanism
// exists.
package packages

import (
	"net/http"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestStreamDownload_SyncErrorOnFIFO verifies that streamDownloadPackageFile
// reports the outFile.Sync() error when the output path is a named pipe
// instead of a regular file. A pipe accepts small writes into its kernel
// buffer without blocking (no reader required), so the download itself
// succeeds, but fsync(2) on a pipe is not supported and returns EINVAL. This
// is the only known, non-OS-faking way to reach that branch: the file handle
// is genuinely open and genuinely fails to sync, unlike a deleted or
// permission-denied path which fails earlier (at os.Create) or not at all
// (root bypasses permission checks). Losing the Sync error wrap would surface
// a bare, unattributed error instead of one naming "sync output file".
func TestStreamDownload_SyncErrorOnFIFO(t *testing.T) {
	fileBody := "small-fifo-payload"
	client := testutil.NewTestClient(t, testStreamServer(t, fileBody, http.StatusOK))

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
		t.Fatal("expected sync error when output_path is a FIFO, got nil")
	}
	if !strings.Contains(err.Error(), "sync output file") {
		t.Fatalf("error = %q, want it to mention 'sync output file'", err.Error())
	}
}
