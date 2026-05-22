// packages_test.go contains unit tests for GitLab Generic Packages API
// operations (publish, download, list, file_list, delete, file_delete).
package packages

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

const (
	// pathPackagePublish identifies the path package publish constant used by this package.
	pathPackagePublish = "/api/v4/projects/42/packages/generic/my-pkg/1.0.0/app.tar.gz"
	// pathPackageDownload identifies the path package download constant used by this package.
	pathPackageDownload = "/api/v4/projects/42/packages/generic/my-pkg/1.0.0/app.tar.gz"
	// pathPackageList identifies the path package list constant used by this package.
	pathPackageList = "/api/v4/projects/42/packages"
	// pathPackageFileList identifies the path package file list constant used by this package.
	pathPackageFileList = "/api/v4/projects/42/packages/10/package_files"
	// pathPackageDelete identifies the path package delete constant used by this package.
	pathPackageDelete = "/api/v4/projects/42/packages/10"
	// pathFileDelete identifies the path file delete constant used by this package.
	pathFileDelete = "/api/v4/projects/42/packages/10/package_files/20"

	// testPackageName identifies the test package name constant used by this package.
	testPackageName = "my-pkg"
	// testFileName identifies the test file name constant used by this package.
	testFileName = "app.tar.gz"
	// testBase64Content identifies the test base 64 content constant used by this package.
	testBase64Content = "dGVzdA=="
)

// publishResponseJSON identifies the publish response JSON constant used by this package.
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

// TestPackagePublishBase64_Success verifies PackagePublishBase64 when success.
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

// TestPackagePublishFilePath_Success verifies PackagePublishFilePath when success.
func TestPackagePublishFilePath_Success(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "testpkg.bin")
	if err := os.WriteFile(tmpFile, []byte("binary-file-content"), 0o600); err != nil {
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

// TestPackagePublish_WithProgressToken verifies Publish wraps the upload body
// when the MCP request includes a progress token.
func TestPackagePublish_WithProgressToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != pathPackagePublish {
			http.NotFound(w, r)
			return
		}
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Fatalf("read upload body: %v", err)
		}
		testutil.RespondJSON(w, http.StatusCreated, publishResponseJSON)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "package-publish-test", Version: "0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "package_publish_with_progress"}, func(ctx context.Context, req *mcp.CallToolRequest, input PublishInput) (*mcp.CallToolResult, PublishOutput, error) {
		out, err := Publish(ctx, req, client, input)
		if err != nil {
			return nil, PublishOutput{}, err
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
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "package-publish-client"}, &mcp.ClientOptions{
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
		Name: "package_publish_with_progress",
		Arguments: map[string]any{
			"project_id":      "42",
			"package_name":    testPackageName,
			"package_version": "1.0.0",
			"file_name":       testFileName,
			"content_base64":  base64.StdEncoding.EncodeToString([]byte("progress package payload")),
		},
		Meta: mcp.Meta{"progressToken": "package-publish-progress-token"},
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

// TestPackagePublishBothParams_Error verifies PackagePublishBothParams when error.
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

// TestPackagePublishNeitherParams_Error verifies PackagePublishNeitherParams when error.
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

// TestPackagePublish_InvalidPackageName verifies PackagePublish when invalid package name.
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

// TestPackagePublish_MissingProjectID verifies PackagePublish when missing project ID.
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

// TestPackagePublish_ContextCancelled verifies PackagePublish when context cancelled.
func TestPackagePublish_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

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

// TestPackageDownload_Success verifies PackageDownload when success.
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

// TestPackageDownload_MissingOutputPath verifies PackageDownload when missing output path.
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

// TestPackageDownload_ContextCancelled verifies PackageDownload when context cancelled.
func TestPackageDownload_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)

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

// TestPackageList_Success verifies PackageList when success.
func TestPackageList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPackageList {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"generic","status":"default","pipeline":{"id":77,"status":"success","ref":"main","sha":"abc123","web_url":"https://gitlab.example.com/project/-/pipelines/77","user":{"id":5,"username":"alice","name":"Alice","web_url":"https://gitlab.example.com/alice"}},"pipelines":[{"id":77,"status":"success","ref":"main","sha":"abc123","web_url":"https://gitlab.example.com/project/-/pipelines/77"}],"last_downloaded_at":"2026-06-01T12:00:00Z","tags":[{"id":1,"package_id":10,"name":"latest"}],"_links":{"web_path":"/project/-/packages/10"}}]`,
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
	if out.Packages[0].Pipeline == nil || out.Packages[0].Pipeline.ID != 77 {
		t.Fatalf("Packages[0].Pipeline.ID = %v, want 77", out.Packages[0].Pipeline)
	}
	if out.Packages[0].Pipeline.User == nil || out.Packages[0].Pipeline.User.Username != "alice" {
		t.Fatalf("Packages[0].Pipeline.User.Username = %v, want alice", out.Packages[0].Pipeline.User)
	}
	if len(out.Packages[0].Pipelines) != 1 || out.Packages[0].Pipelines[0].Status != "success" {
		t.Fatalf("Packages[0].Pipelines = %v, want one success pipeline", out.Packages[0].Pipelines)
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

// TestPackageToListItem_OptionalPipelineFields verifies that package conversion
// preserves optional pipeline timestamps and user metadata while skipping nil entries.
func TestPackageToListItem_OptionalPipelineFields(t *testing.T) {
	now := time.Date(2026, 5, 6, 11, 0, 0, 0, time.UTC)
	updated := now.Add(time.Minute)
	item := packageToListItem(&gl.Package{
		ID:          99,
		Name:        "pkg",
		Version:     "1.2.3",
		PackageType: "generic",
		Status:      "default",
		CreatedAt:   &now,
		Pipelines: []*gl.PackagePipeline{
			nil,
			{
				ID:        77,
				Status:    "success",
				Ref:       "main",
				SHA:       "abc123",
				WebURL:    "https://gitlab.example.com/pipelines/77",
				CreatedAt: &now,
				UpdatedAt: &updated,
				User: &gl.BasicUser{
					ID:       5,
					Username: "alice",
					Name:     "Alice",
					WebURL:   "https://gitlab.example.com/alice",
				},
			},
		},
	})

	if item.CreatedAt == "" {
		t.Fatal("CreatedAt should be preserved")
	}
	if len(item.Pipelines) != 1 {
		t.Fatalf("Pipelines = %+v, want one non-nil pipeline", item.Pipelines)
	}
	pipeline := item.Pipelines[0]
	if pipeline.ID != 77 {
		t.Fatalf("pipeline ID = %d, want 77", pipeline.ID)
	}
	if pipeline.CreatedAt == "" || pipeline.UpdatedAt == "" {
		t.Fatalf("pipeline timestamps = %+v, want created and updated values", pipeline)
	}
	if pipeline.User == nil || pipeline.User.Username != "alice" {
		t.Fatalf("pipeline user = %+v, want alice", pipeline.User)
	}
}

// TestPackagePipelineToOutput_NilPipeline_ReturnsNil verifies that nil package
// pipeline pointers are converted to nil output values.
func TestPackagePipelineToOutput_NilPipeline_ReturnsNil(t *testing.T) {
	if got := packagePipelineToOutput(nil); got != nil {
		t.Fatalf("packagePipelineToOutput(nil) = %+v, want nil", got)
	}
}

// TestPackageList_WithFilters verifies PackageList when with filters.
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

// TestPackageList_MissingProjectID verifies PackageList when missing project ID.
func TestPackageList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestPackageFileList_Success verifies PackageFileList when success.
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

// TestPackageFileList_MissingPackageID verifies PackageFileList when missing package ID.
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

// TestPackageDelete_Success verifies PackageDelete when success.
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

// TestPackageDelete_MissingProjectID verifies PackageDelete when missing project ID.
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

// TestPackageDelete_MissingPackageID verifies PackageDelete when missing package ID.
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

// TestPackageFileDelete_Success verifies PackageFileDelete when success.
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

// TestPackageFileDelete_MissingFileID verifies PackageFileDelete when missing file ID.
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

// TestPackagePublish_APIError verifies PackagePublish when API error.
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

// TestPtrString verifies PtrString.
func TestPtrString(t *testing.T) {
	if ptrString("") != nil {
		t.Error("ptrString empty should return nil")
	}
	if p := ptrString("hello"); p == nil || *p != "hello" {
		t.Error("ptrString hello should return pointer to hello")
	}
}

// Ensure fmt is referenced to avoid unused import error.
var _ = fmt.Sprintf
