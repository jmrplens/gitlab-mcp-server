// packages_stream_test.go contains unit tests for streaming package downloads.
package packages

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	testOctetStream   = "application/octet-stream"
	testOutputBin     = "out.bin"
	testAppBin        = "app.bin"
	testPkgVersion    = "1.0.0"
	headerContentType = "Content-Type"
)

// testStreamServer creates a handler that serves streaming downloads.
func testStreamServer(t *testing.T, fileBody string, statusCode int) http.HandlerFunc {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/packages/generic/"):
			w.Header().Set(headerContentType, testOctetStream)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileBody)))
			w.WriteHeader(statusCode)
			w.Write([]byte(fileBody))
		default:
			http.NotFound(w, r)
		}
	})
}

// TestStreamDownloadPackageFile_Success verifies the behavior of stream download package file success.
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

// TestStreamDownloadPackageFile_CreatesDirectory verifies the behavior of stream download package file creates directory.
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

// TestStreamDownloadPackageFile_ContextCancelled verifies the behavior of stream download package file context cancelled.
func TestStreamDownloadPackageFile_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testStreamServer(t, "data", http.StatusOK))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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

// TestStreamDownloadPackageFile_APIError verifies the behavior of stream download package file a p i error.
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

// TestComputeSHA256_ViaToolutil verifies the behavior of compute s h a256 via toolutil.
func TestComputeSHA256_ViaToolutil(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.bin")
	os.WriteFile(f, []byte("hello"), 0644)

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
	os.WriteFile(blocker, []byte("x"), 0644)
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
