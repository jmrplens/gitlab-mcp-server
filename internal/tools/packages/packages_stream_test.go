// packages_stream_test.go contains unit tests for streaming package downloads.
package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// testOctetStream identifies the test octet stream constant used by this package.
	testOctetStream = "application/octet-stream"
	// testOutputBin identifies the test output bin constant used by this package.
	testOutputBin = "out.bin"
	// testAppBin identifies the test app bin constant used by this package.
	testAppBin = "app.bin"
	// testPkgVersion identifies the test pkg version constant used by this package.
	testPkgVersion = "1.0.0"
	// headerContentType identifies the header content type constant used by this package.
	headerContentType = "Content-Type"
)

// testStreamServer creates a handler that serves streaming downloads.
func testStreamServer(t *testing.T, fileBody string, statusCode int) http.HandlerFunc {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/packages/generic/"):
			w.Header().Set(headerContentType, testOctetStream)
			w.Header().Set("Content-Length", strconv.Itoa(len(fileBody)))
			w.WriteHeader(statusCode)
			w.Write([]byte(fileBody))
		default:
			http.NotFound(w, r)
		}
	})
}

// TestStreamDownloadPackageFile_Success verifies StreamDownloadPackageFile when success.
func TestStreamDownloadPackageFile_Success(t *testing.T) {
	fileBody := strings.Repeat("streaming-data-block-", 1000)
	client := testutil.NewTestClient(t, testStreamServer(t, fileBody, http.StatusOK))

	outPath := filepath.Join(t.TempDir(), testOutputBin)
	out, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     outPath,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.OutputPath != outPath {
		t.Errorf("OutputPath = %q, want %q", out.OutputPath, outPath)
	}
	if out.Size != int64(len(fileBody)) {
		t.Errorf("Size = %d, want %d", out.Size, len(fileBody))
	}
	data, _ := os.ReadFile(outPath)
	if string(data) != fileBody {
		t.Error("downloaded content does not match")
	}
	expectedSHA := fmt.Sprintf("%x", sha256.Sum256([]byte(fileBody)))
	if out.SHA256 != expectedSHA {
		t.Errorf("SHA256 = %q, want %q", out.SHA256, expectedSHA)
	}
}

// makeDirs creates each directory or fails the test, so a containment fixture
// reads as one line instead of a loop that asserts.
func makeDirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatalf("Mkdir(%q) error = %v", dir, err)
		}
	}
}

// confineDownloadRoots points the default local-path allow-list roots (the
// working directory and the OS temp directory) at dir, so a sibling of dir is
// genuinely outside every allowed destination.
func confineDownloadRoots(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("TMPDIR", dir)
	t.Chdir(dir)
}

// TestDownload_OutputPathOutsideAllowedDirs_Rejected verifies that a download
// destination is confined to the allow-listed roots: a path outside them, a
// parent-traversal escape and a symlinked parent are all refused, nothing is
// created on the way, and the package is never even requested. The one
// legitimate destination inside the workspace still works.
func TestDownload_OutputPathOutsideAllowedDirs_Rejected(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "home")
	makeDirs(t, allowed, outside)
	linkDir := filepath.Join(allowed, "escape")
	symlinked := os.Symlink(outside, linkDir) == nil
	confineDownloadRoots(t, allowed)

	tests := []struct {
		name    string
		path    string
		wantErr bool
		skip    bool
	}{
		{name: "destination inside the workspace is written", path: filepath.Join(allowed, "artifact.bin")},
		{name: "destination outside every allowed root is refused", path: filepath.Join(outside, ".gitlab-mcp-server.env"), wantErr: true},
		{name: "parent traversal out of the workspace is refused", path: filepath.Join(allowed, "..", "home", "stolen.bin"), wantErr: true},
		{name: "symlinked parent directory pointing outside is refused", path: filepath.Join(linkDir, "planted.env"), wantErr: true, skip: !symlinked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("symlinks unsupported on this platform")
			}
			if tt.wantErr {
				assertDownloadRefused(t, tt.path, outside)
				return
			}
			assertDownloadWritten(t, tt.path)
		})
	}
}

// assertDownloadRefused runs a download to path and asserts it is refused
// without reaching the package endpoint and without creating anything, at the
// destination or under outside.
func assertDownloadRefused(t *testing.T, path, outside string) {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("package file was requested for a refused output_path %q", path)
		w.WriteHeader(http.StatusForbidden)
	}))

	if _, err := downloadTo(t, client, path); err == nil {
		t.Fatalf("Download(%q) error = nil, want refusal", path)
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("os.Lstat(%q) error = %v after refusal, want nothing written", path, statErr)
	}
	escaped := filepath.Join(outside, filepath.Base(path))
	if _, statErr := os.Lstat(escaped); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("os.Lstat(%q) error = %v, want no file outside the allowed roots", escaped, statErr)
	}
}

// assertDownloadWritten runs a download to path and asserts it reaches the
// package endpoint and lands on disk.
func assertDownloadWritten(t *testing.T, path string) {
	t.Helper()
	var served atomic.Bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served.Store(true)
		w.Header().Set(headerContentType, testOctetStream)
		_, _ = w.Write([]byte("payload"))
	}))

	if _, err := downloadTo(t, client, path); err != nil {
		t.Fatalf("Download(%q) error = %v, want success", path, err)
	}
	if !served.Load() {
		t.Error("the legitimate download did not reach the package endpoint")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("os.Stat(%q) error = %v, want the file written", path, statErr)
	}
}

// downloadTo issues one package download to path against client.
func downloadTo(t *testing.T, client *gitlabclient.Client, path string) (DownloadOutput, error) {
	t.Helper()
	return Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     path,
	})
}

// TestStreamDownloadPackageFile_CreatesDirectory verifies StreamDownloadPackageFile creates directory.
func TestStreamDownloadPackageFile_CreatesDirectory(t *testing.T) {
	fileBody := "hello-stream"
	client := testutil.NewTestClient(t, testStreamServer(t, fileBody, http.StatusOK))

	outPath := filepath.Join(t.TempDir(), "sub", "deep", testOutputBin)
	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     outPath,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if _, statErr := os.Stat(outPath); os.IsNotExist(statErr) {
		t.Error("expected output file to be created, but it does not exist")
	}
}

// TestStreamDownloadPackageFile_WithProgressToken verifies Download wraps the
// streaming writer when the MCP request includes a progress token.
func TestStreamDownloadPackageFile_WithProgressToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fileBody := strings.Repeat("stream-progress-block-", 4096)
	client := testutil.NewTestClient(t, testStreamServer(t, fileBody, http.StatusOK))
	outPath := filepath.Join(t.TempDir(), testOutputBin)

	server := mcp.NewServer(&mcp.Implementation{Name: "package-download-test", Version: "0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "package_download_with_progress"}, func(ctx context.Context, req *mcp.CallToolRequest, input DownloadInput) (*mcp.CallToolResult, DownloadOutput, error) {
		out, err := Download(ctx, req, client, input)
		if err != nil {
			return nil, DownloadOutput{}, err
		}
		return &mcp.CallToolResult{}, out, nil
	})

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	progressSeen := make(chan struct{}, 1)
	var once sync.Once
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "package-download-client"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, _ *mcp.ProgressNotificationClientRequest) {
			once.Do(func() { progressSeen <- struct{}{} })
		},
	})
	clientSession, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		_ = serverSession.Wait()
	})

	_, err = clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "package_download_with_progress",
		Arguments: map[string]any{
			"project_id":      "42",
			"package_name":    testPackageName,
			"package_version": testPkgVersion,
			"file_name":       testAppBin,
			"output_path":     outPath,
		},
		Meta: mcp.Meta{"progressToken": "package-download-progress-token"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}

	select {
	case <-progressSeen:
	case <-ctx.Done():
		t.Fatal("timed out waiting for progress notification")
	}
}

// TestStreamDownloadPackageFile_ContextCancelled verifies StreamDownloadPackageFile when context cancelled.
func TestStreamDownloadPackageFile_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testStreamServer(t, "data", http.StatusOK))

	ctx := testutil.CancelledCtx(t)

	_, err := Download(ctx, nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     filepath.Join(t.TempDir(), testOutputBin),
	})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestStreamDownloadPackageFile_APIError verifies StreamDownloadPackageFile when API error.
func TestStreamDownloadPackageFile_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"Package Not Found"}`)
	}))

	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     filepath.Join(t.TempDir(), testOutputBin),
	})
	if err == nil {
		t.Fatal("expected error for API error, got nil")
	}
}

// TestComputeSHA256_ViaToolutil verifies ComputeSHA256 when via toolutil.
func TestComputeSHA256_ViaToolutil(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.bin")
	os.WriteFile(f, []byte("hello"), 0o600)

	hash, err := toolutil.ComputeSHA256(f)
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte("hello")))
	if hash != expected {
		t.Errorf("SHA256 = %q, want %q", hash, expected)
	}
}

// TestStreamDownload_UnwritablePath verifies that streamDownloadPackageFile
// returns an error when the output file cannot be created (e.g. parent is a file).
func TestStreamDownload_UnwritablePath(t *testing.T) {
	client := testutil.NewTestClient(t, testStreamServer(t, "data", http.StatusOK))

	// Create a file where a directory is expected, so os.Create fails.
	blocker := filepath.Join(t.TempDir(), "blocker")
	os.WriteFile(blocker, []byte("x"), 0o600)
	badPath := filepath.Join(blocker, "sub", testOutputBin)

	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     badPath,
	})
	if err == nil {
		t.Fatal("expected error for unwritable output path, got nil")
	}
}

// TestStreamDownload_OutputPathIsDirectory verifies streamDownloadPackageFile
// refuses an output path that already exists as something other than a regular
// file, before it opens anything. A directory is the harmless instance of that
// rule; a symlink is the one it exists for.
func TestStreamDownload_OutputPathIsDirectory(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error when output_path is a directory")
	}
	if !strings.Contains(err.Error(), "is not a regular file") {
		t.Fatalf("error = %q, want the not-a-regular-file refusal", err.Error())
	}
}

// ----- branch coverage -----

// TestStreamDownload_DeadBranches documents why the error-return branches
// inside streamDownloadPackageFile are unreachable through any public call
// path:
//
//  1. FormatPackageURL error: the function only fails on invalid pid
//     types in parseID. streamDownloadPackageFile always feeds it
//     string(input.ProjectID), which parseID accepts unconditionally.
//  2. NewRequest error: the only error path is url.PathUnescape on a
//     malformed percent-encoded path. FormatPackageURL generates the
//     path with PathEscape, so the result is always well-formed.
//  3. outFile.Stat error: the file handle is still open, so Stat
//     succeeds unconditionally under normal conditions.
//  4. outFile.Sync error: fsync(2) genuinely fails with EINVAL on a named
//     pipe, which is how TestStreamDownload_SyncErrorOnFIFO used to reach
//     this branch. Confining output_path now refuses any destination that
//     already exists and is not a regular file, so no caller-supplied path
//     reaches Sync on a pipe any more; what is left are real I/O failures
//     (a full or failing filesystem), which a unit test cannot stage. The
//     wrap stays because those failures are what it names.
//
// We assert the documented contract below: a happy-path download
// streams the payload to disk, syncs the file, and reports its size
// without invoking any of the unreachable branches.
func TestStreamDownload_DeadBranches(t *testing.T) {
	fileBody := "dead-branch-fixture"
	client := testutil.NewTestClient(t, testStreamServer(t, fileBody, http.StatusOK))

	outPath := filepath.Join(t.TempDir(), "dead-branches.bin")
	size, checksum, err := streamDownloadPackageFile(
		context.Background(),
		nil,
		client,
		DownloadInput{
			ProjectID:      "42",
			PackageName:    testPackageName,
			PackageVersion: testPkgVersion,
			FileName:       testAppBin,
			OutputPath:     outPath,
		},
	)
	if err != nil {
		t.Fatalf("streamDownloadPackageFile() error = %v", err)
	}
	if size != int64(len(fileBody)) {
		t.Fatalf("size = %d, want %d", size, len(fileBody))
	}
	expected := sha256.Sum256([]byte(fileBody))
	want := hex.EncodeToString(expected[:])
	if checksum != want {
		t.Fatalf("checksum = %q, want %q", checksum, want)
	}
}
