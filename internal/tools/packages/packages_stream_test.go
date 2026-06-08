// packages_stream_test.go contains unit tests for streaming package downloads.
package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
// returns the os.Create error when the requested output path is a directory.
func TestStreamDownload_OutputPathIsDirectory(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected API call: %s %s", r.Method, r.URL.Path)
	}))

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
	if !strings.Contains(err.Error(), "create output file") {
		t.Fatalf("error = %q, want create output file message", err.Error())
	}
}

// ----- branch coverage -----

// TestStreamDownload_DeadBranches documents why the four error-return
// branches inside streamDownloadPackageFile are unreachable through any
// public call path:
//
//  1. FormatPackageURL error: the function only fails on invalid pid
//     types in parseID. streamDownloadPackageFile always feeds it
//     string(input.ProjectID), which parseID accepts unconditionally.
//  2. NewRequest error: the only error path is url.PathUnescape on a
//     malformed percent-encoded path. FormatPackageURL generates the
//     path with PathEscape, so the result is always well-formed.
//  3. outFile.Sync error: the file handle is still open (deferred Close
//     has not run) and the writer has finished writing before Sync is
//     called. The only way Sync would fail is on a filesystem-level
//     error, which cannot be simulated in a unit test.
//  4. outFile.Stat error: the file handle is still open, so Stat
//     succeeds unconditionally under normal conditions.
//
// We assert the documented contract below: a happy-path download
// streams the payload to disk, syncs the file, and reports its size
// without invoking any of the four unreachable branches.
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
