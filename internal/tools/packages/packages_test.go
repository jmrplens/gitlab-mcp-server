// packages_test.go contains unit tests for GitLab Generic Packages API
// operations (publish, download, list, file_list, delete, file_delete).
package packages

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const (
	pathPackagePublish  = "/api/v4/projects/42/packages/generic/my-pkg/1.0.0/app.tar.gz"
	pathPackageDownload = "/api/v4/projects/42/packages/generic/my-pkg/1.0.0/app.tar.gz"
	pathPackageList     = "/api/v4/projects/42/packages"
	pathPackageFileList = "/api/v4/projects/42/packages/10/package_files"
	pathPackageDelete   = "/api/v4/projects/42/packages/10"
	pathFileDelete      = "/api/v4/projects/42/packages/10/package_files/20"

	testPackageName   = "my-pkg"
	testFileName      = "app.tar.gz"
	testBase64Content = "dGVzdA=="
)

const publishResponseJSON = `{
	"id": 1,
	"package_id": 10,
	"file_name": "app.tar.gz",
	"size": 1024,
	"file_sha256": "abc123hash",
	"file_md5": "md5hash",
	"file_sha1": "sha1hash",
	"file_store": 1,
	"created_at": "2026-06-01T10:00:00Z",
	"updated_at": "2026-06-01T11:00:00Z"
}`

// TestPackagePublishBase64_Success verifies the behavior of package publish base64 success.
func TestPackagePublishBase64_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathPackagePublish {
			testutil.RespondJSON(w, http.StatusCreated, publishResponseJSON)
			return
		}
		http.NotFound(w, r)
	}))

	content := base64.StdEncoding.EncodeToString([]byte("hello-package-data"))
	out, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  content,
	})
	if err != nil {
		t.Fatalf("Publish() unexpected error: %v", err)
	}
	if out.PackageFileID != 1 {
		t.Errorf("PackageFileID = %d, want 1", out.PackageFileID)
	}
	if out.PackageID != 10 {
		t.Errorf("PackageID = %d, want 10", out.PackageID)
	}
	if out.FileName != testFileName {
		t.Errorf("FileName = %q, want %q", out.FileName, testFileName)
	}
	if out.Size != 1024 {
		t.Errorf("Size = %d, want 1024", out.Size)
	}
	if out.FileMD5 != "md5hash" {
		t.Errorf("FileMD5 = %q, want %q", out.FileMD5, "md5hash")
	}
	if out.FileSHA1 != "sha1hash" {
		t.Errorf("FileSHA1 = %q, want %q", out.FileSHA1, "sha1hash")
	}
	if out.FileStore != 1 {
		t.Errorf("FileStore = %d, want 1", out.FileStore)
	}
	if out.SHA256 != "abc123hash" {
		t.Errorf("SHA256 = %q, want %q", out.SHA256, "abc123hash")
	}
	if out.CreatedAt == "" {
		t.Error("CreatedAt should not be empty")
	}
	if out.UpdatedAt == "" {
		t.Error("UpdatedAt should not be empty")
	}
}

// TestPackagePublishFilePath_Success verifies the behavior of package publish file path success.
func TestPackagePublishFilePath_Success(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "testpkg.bin")
	if err := os.WriteFile(tmpFile, []byte("binary-file-content"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathPackagePublish {
			testutil.RespondJSON(w, http.StatusCreated, publishResponseJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		FilePath:       tmpFile,
	})
	if err != nil {
		t.Fatalf("Publish() unexpected error: %v", err)
	}
	if out.PackageFileID != 1 {
		t.Errorf("PackageFileID = %d, want 1", out.PackageFileID)
	}
	if out.PackageID != 10 {
		t.Errorf("PackageID = %d, want 10", out.PackageID)
	}
	if out.FileName != testFileName {
		t.Errorf("FileName = %q, want %q", out.FileName, testFileName)
	}
	if out.Size != 1024 {
		t.Errorf("Size = %d, want 1024", out.Size)
	}
	if out.SHA256 != "abc123hash" {
		t.Errorf("SHA256 = %q, want %q", out.SHA256, "abc123hash")
	}
}

// TestPackagePublishBothParams_Error verifies the behavior of package publish both params error.
func TestPackagePublishBothParams_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		FilePath:       "/some/path",
		ContentBase64:  testBase64Content,
	})
	if err == nil {
		t.Fatal("expected error when both file_path and content_base64 provided")
	}
}

// TestPackagePublishNeitherParams_Error verifies the behavior of package publish neither params error.
func TestPackagePublishNeitherParams_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
	})
	if err == nil {
		t.Fatal("expected error when neither file_path nor content_base64 provided")
	}
}

// TestPackagePublish_InvalidPackageName verifies the behavior of package publish invalid package name.
func TestPackagePublish_InvalidPackageName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    ".invalid",
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  testBase64Content,
	})
	if err == nil {
		t.Fatal("expected error for invalid package name")
	}
}

// TestPackagePublish_MissingProjectID verifies the behavior of package publish missing project i d.
func TestPackagePublish_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := Publish(context.Background(), nil, client, PublishInput{
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  testBase64Content,
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestPackagePublish_ContextCancelled verifies the behavior of package publish context cancelled.
func TestPackagePublish_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := Publish(ctx, nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  testBase64Content,
	})
	if err == nil {
		t.Fatal("expected context canceled error")
	}
}

// TestPackageDownload_Success verifies the behavior of package download success.
func TestPackageDownload_Success(t *testing.T) {
	fileContent := []byte("downloaded-binary-data")
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPackageDownload {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(fileContent)
			return
		}
		http.NotFound(w, r)
	}))

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "downloaded.bin")

	out, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		OutputPath:     outPath,
	})
	if err != nil {
		t.Fatalf("Download() unexpected error: %v", err)
	}
	if out.OutputPath != outPath {
		t.Errorf("OutputPath = %q, want %q", out.OutputPath, outPath)
	}
	if out.Size != int64(len(fileContent)) {
		t.Errorf("Size = %d, want %d", out.Size, len(fileContent))
	}
	expectedSHA := fmt.Sprintf("%x", sha256.Sum256(fileContent))
	if out.SHA256 != expectedSHA {
		t.Errorf("SHA256 = %q, want %q", out.SHA256, expectedSHA)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(data) != string(fileContent) {
		t.Errorf("file content = %q, want %q", string(data), string(fileContent))
	}
}

// TestPackageDownload_MissingOutputPath verifies the behavior of package download missing output path.
func TestPackageDownload_MissingOutputPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
	})
	if err == nil {
		t.Fatal("expected error for missing output_path")
	}
}

// TestPackageDownload_ContextCancelled verifies the behavior of package download context cancelled.
func TestPackageDownload_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := Download(ctx, nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		OutputPath:     filepath.Join(t.TempDir(), "out.bin"),
	})
	if err == nil {
		t.Fatal("expected context canceled error")
	}
}

// TestPackageList_Success verifies the behavior of package list success.
func TestPackageList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPackageList {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"generic","status":"default","last_downloaded_at":"2026-06-01T12:00:00Z","tags":[{"id":1,"package_id":10,"name":"latest"}],"_links":{"web_path":"/project/-/packages/10"}}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Packages) != 1 {
		t.Fatalf("len(Packages) = %d, want 1", len(out.Packages))
	}
	if out.Packages[0].ID != 10 {
		t.Errorf("Packages[0].ID = %d, want 10", out.Packages[0].ID)
	}
	if out.Packages[0].Name != testPackageName {
		t.Errorf("Packages[0].Name = %q, want %q", out.Packages[0].Name, testPackageName)
	}
	if out.Packages[0].PackageType != "generic" {
		t.Errorf("Packages[0].PackageType = %q, want %q", out.Packages[0].PackageType, "generic")
	}
	if out.Packages[0].LastDownloadedAt == "" {
		t.Error("Packages[0].LastDownloadedAt should not be empty")
	}
	if len(out.Packages[0].Tags) != 1 || out.Packages[0].Tags[0] != "latest" {
		t.Errorf("Packages[0].Tags = %v, want [latest]", out.Packages[0].Tags)
	}
	if out.Packages[0].WebPath != "/project/-/packages/10" {
		t.Errorf("Packages[0].WebPath = %q, want %q", out.Packages[0].WebPath, "/project/-/packages/10")
	}
}

// TestPackageList_WithFilters verifies the behavior of package list with filters.
func TestPackageList_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPackageList {
			testutil.AssertQueryParam(t, r, "package_name", testPackageName)
			testutil.AssertQueryParam(t, r, "package_type", "generic")
			testutil.AssertQueryParam(t, r, "include_versionless", "true")
			testutil.AssertQueryParam(t, r, "status", "hidden")
			testutil.RespondJSONWithPagination(w, http.StatusOK, "[]",
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:          "42",
		PackageName:        testPackageName,
		PackageType:        "generic",
		IncludeVersionless: true,
		Status:             "hidden",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Packages) != 0 {
		t.Errorf("len(Packages) = %d, want 0", len(out.Packages))
	}
}

// TestPackageList_MissingProjectID verifies the behavior of package list missing project i d.
func TestPackageList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestPackageFileList_Success verifies the behavior of package file list success.
func TestPackageFileList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPackageFileList {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":20,"package_id":10,"file_name":"app.tar.gz","size":1024,"file_sha256":"abc123","file_md5":"md5file","file_sha1":"sha1file"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := FileList(context.Background(), client, FileListInput{
		ProjectID: "42",
		PackageID: "10",
	})
	if err != nil {
		t.Fatalf("FileList() unexpected error: %v", err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(out.Files))
	}
	if out.Files[0].PackageFileID != 20 {
		t.Errorf("Files[0].PackageFileID = %d, want 20", out.Files[0].PackageFileID)
	}
	if out.Files[0].FileName != testFileName {
		t.Errorf("Files[0].FileName = %q, want %q", out.Files[0].FileName, testFileName)
	}
	if out.Files[0].SHA256 != "abc123" {
		t.Errorf("Files[0].SHA256 = %q, want %q", out.Files[0].SHA256, "abc123")
	}
	if out.Files[0].FileMD5 != "md5file" {
		t.Errorf("Files[0].FileMD5 = %q, want %q", out.Files[0].FileMD5, "md5file")
	}
	if out.Files[0].FileSHA1 != "sha1file" {
		t.Errorf("Files[0].FileSHA1 = %q, want %q", out.Files[0].FileSHA1, "sha1file")
	}
	if out.Files[0].Size != 1024 {
		t.Errorf("Files[0].Size = %d, want 1024", out.Files[0].Size)
	}
	if out.Files[0].PackageID != 10 {
		t.Errorf("Files[0].PackageID = %d, want 10", out.Files[0].PackageID)
	}
}

// TestPackageFileList_MissingPackageID verifies the behavior of package file list missing package i d.
func TestPackageFileList_MissingPackageID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := FileList(context.Background(), client, FileListInput{
		ProjectID: "42",
	})
	if err == nil {
		t.Fatal("expected error for missing package_id")
	}
}

// TestPackageDelete_Success verifies the behavior of package delete success.
func TestPackageDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathPackageDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), nil, client, DeleteInput{
		ProjectID: "42",
		PackageID: "10",
	})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestPackageDelete_MissingProjectID verifies the behavior of package delete missing project i d.
func TestPackageDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	err := Delete(context.Background(), nil, client, DeleteInput{
		PackageID: "10",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestPackageDelete_MissingPackageID verifies the behavior of package delete missing package i d.
func TestPackageDelete_MissingPackageID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	err := Delete(context.Background(), nil, client, DeleteInput{
		ProjectID: "42",
	})
	if err == nil {
		t.Fatal("expected error for missing package_id")
	}
}

// TestPackageFileDelete_Success verifies the behavior of package file delete success.
func TestPackageFileDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathFileDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := FileDelete(context.Background(), nil, client, FileDeleteInput{
		ProjectID:     "42",
		PackageID:     "10",
		PackageFileID: "20",
	})
	if err != nil {
		t.Fatalf("FileDelete() unexpected error: %v", err)
	}
}

// TestPackageFileDelete_MissingFileID verifies the behavior of package file delete missing file i d.
func TestPackageFileDelete_MissingFileID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	err := FileDelete(context.Background(), nil, client, FileDeleteInput{
		ProjectID: "42",
		PackageID: "10",
	})
	if err == nil {
		t.Fatal("expected error for missing package_file_id")
	}
}

// TestPackagePublish_APIError verifies the behavior of package publish a p i error.
func TestPackagePublish_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  base64.StdEncoding.EncodeToString([]byte("data")),
	})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestPackageDelete403_Maintainer verifies that Delete returns a clear
// permission message when the user lacks Maintainer role.
func TestPackageDelete403_Maintainer(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	err := Delete(context.Background(), nil, client, DeleteInput{
		ProjectID: "42",
		PackageID: "10",
	})
	if err == nil {
		t.Fatal("Delete() expected error for 403, got nil")
	}
	if !strings.Contains(err.Error(), "Maintainer") {
		t.Errorf("Delete() error should mention Maintainer role, got: %v", err)
	}
}

// TestPtrString verifies the behavior of ptr string.
func TestPtrString(t *testing.T) {
	if p := ptrString(""); p != nil {
		t.Error("ptrString empty should return nil")
	}
	if p := ptrString("hello"); p == nil || *p != "hello" {
		t.Error("ptrString hello should return pointer to hello")
	}
}

// Ensure fmt is referenced to avoid unused import error.
var _ = fmt.Sprintf
