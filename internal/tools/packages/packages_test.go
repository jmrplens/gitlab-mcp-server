// packages_test.go contains unit tests for GitLab Generic Packages API
// operations (publish, download, list, file_list, delete, file_delete).
package packages

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
			t.Errorf("read upload body: %v", err)
			http.Error(w, "read upload body", http.StatusInternalServerError)
			return
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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

	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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

	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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
	if len(out.Packages[0].Tags) != 1 || out.Packages[0].Tags[0].Name != "latest" {
		t.Errorf("Packages[0].Tags = %v, want [latest]", out.Packages[0].Tags)
	}
	if out.Packages[0].Links == nil || out.Packages[0].Links.WebPath != "/project/-/packages/10" {
		t.Errorf("Packages[0].Links = %+v, want WebPath=/project/-/packages/10", out.Packages[0].Links)
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
	}, toolutil.PackageExtra{})

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

// TestPublish_SelectOverride verifies that an explicit select value is
// forwarded to the GitLab API as the select query parameter, overriding
// the default package_file selection.
func TestPublish_SelectOverride(t *testing.T) {
	var gotSelect string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			gotSelect = r.URL.Query().Get("select")
			testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"package_id":10,"file_name":"app.tar.gz","size":4,"file_sha256":"abc"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       "app.tar.gz",
		ContentBase64:  base64.StdEncoding.EncodeToString([]byte("data")),
		Select:         "package_file",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if gotSelect != "package_file" {
		t.Errorf("select query = %q, want package_file", gotSelect)
	}
}

// TestFileList_OrderingAndKeyset verifies that order_by, sort, and
// keyset pagination inputs are propagated to the GitLab API query.
func TestFileList_OrderingAndKeyset(t *testing.T) {
	var orderBy, sort, pagination, pageToken string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/packages/10/package_files" {
			q := r.URL.Query()
			orderBy, sort = q.Get("order_by"), q.Get("sort")
			pagination, pageToken = q.Get("pagination"), q.Get("page_token")
			testutil.RespondJSON(w, http.StatusOK, `[{"id":20,"package_id":10,"file_name":"app.bin","size":1,"file_sha256":"h"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	_, err := FileList(context.Background(), client, FileListInput{
		ProjectID:  "1",
		PackageID:  "10",
		OrderBy:    "created_at",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "20",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if orderBy != "created_at" || sort != "desc" {
		t.Errorf("order_by/sort = %q/%q, want created_at/desc", orderBy, sort)
	}
	if pagination != "keyset" || pageToken != "20" {
		t.Errorf("pagination/page_token = %q/%q, want keyset/20", pagination, pageToken)
	}
}

// TestList_KeysetPagination verifies that the package list keyset
// pagination inputs are propagated to the GitLab API query.
func TestList_KeysetPagination(t *testing.T) {
	var pagination, pageToken string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathAPIPkgs1 {
			q := r.URL.Query()
			pagination, pageToken = q.Get("pagination"), q.Get("page_token")
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))
	_, err := List(context.Background(), client, ListInput{
		ProjectID:  "1",
		Pagination: "keyset", PageToken: "55",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if pagination != "keyset" || pageToken != "55" {
		t.Errorf("pagination/page_token = %q/%q, want keyset/55", pagination, pageToken)
	}
}

// TestPackageToListItem_FullNestedObjects verifies that _links
// (delete_api_path), tag timestamps, and the full pipeline user object
// (state, avatar_url, created_at) are mirrored from the GitLab Package.
func TestPackageToListItem_FullNestedObjects(t *testing.T) {
	now := time.Date(2026, 5, 6, 11, 0, 0, 0, time.UTC)
	item := packageToListItem(&gl.Package{
		ID:          7,
		Name:        "pkg",
		PackageType: "generic",
		Status:      "default",
		Links:       &gl.PackageLinks{WebPath: "/p/7", DeleteAPIPath: "/api/v4/p/7"},
		Tags:        []gl.PackageTag{{ID: 1, PackageID: 7, Name: "latest", CreatedAt: &now, UpdatedAt: &now}},
		Pipeline: &gl.PackagePipeline{
			ID: 9, Status: "success",
			User: &gl.BasicUser{
				ID: 5, Username: "alice", Name: "Alice",
				State: "active", AvatarURL: "https://gitlab.example.com/a.png",
				WebURL: "https://gitlab.example.com/alice", CreatedAt: &now,
			},
		},
	}, toolutil.PackageExtra{})

	if item.Links == nil || item.Links.DeleteAPIPath != "/api/v4/p/7" {
		t.Errorf("Links = %+v, want DeleteAPIPath set", item.Links)
	}
	if len(item.Tags) != 1 || item.Tags[0].CreatedAt == "" || item.Tags[0].UpdatedAt == "" {
		t.Errorf("Tags = %+v, want timestamps populated", item.Tags)
	}
	if item.Pipeline.User == nil || item.Pipeline.User.State != "active" ||
		item.Pipeline.User.AvatarURL == "" || item.Pipeline.User.CreatedAt == "" {
		t.Errorf("pipeline user = %+v, want full user fields", item.Pipeline.User)
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestPackageGroupList_Success verifies GroupList returns packages across a
// group with the owning project id/path and the embedded project-scoped fields
// (pipeline, tags, _links) fully preserved.
func TestPackageGroupList_Success(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/99/packages" {
			gotPath = r.URL.Path
			gotQuery = r.URL.Query()
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[null,{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"generic","status":"default","project_id":7,"project_path":"grp/proj","tags":[{"id":1,"package_id":10,"name":"latest"}],"_links":{"web_path":"/grp/proj/-/packages/10"}}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GroupList(context.Background(), client, GroupListInput{
		GroupID:            "99",
		ExcludeSubgroups:   true,
		PackageName:        testPackageName,
		PackageType:        "generic",
		OrderBy:            "project_path",
		Sort:               "desc",
		IncludeVersionless: true,
		Status:             "default",
	})
	if err != nil {
		t.Fatalf("GroupList() unexpected error: %v", err)
	}
	if gotPath != "/api/v4/groups/99/packages" {
		t.Fatalf("request path = %q, want group packages path", gotPath)
	}
	if gotQuery.Get("exclude_subgroups") != "true" {
		t.Errorf("exclude_subgroups query = %q, want true", gotQuery.Get("exclude_subgroups"))
	}
	if gotQuery.Get("order_by") != "project_path" {
		t.Errorf("order_by query = %q, want project_path", gotQuery.Get("order_by"))
	}
	if len(out.Packages) != 1 {
		t.Fatalf("len(Packages) = %d, want 1", len(out.Packages))
	}
	pkg := out.Packages[0]
	if pkg.ID != 10 || pkg.Name != testPackageName {
		t.Errorf("Packages[0] = {ID:%d Name:%q}, want {10 %q}", pkg.ID, pkg.Name, testPackageName)
	}
	if pkg.ProjectID != 7 || pkg.ProjectPath != "grp/proj" {
		t.Errorf("Packages[0] project = {ID:%d Path:%q}, want {7 grp/proj}", pkg.ProjectID, pkg.ProjectPath)
	}
	if len(pkg.Tags) != 1 || pkg.Tags[0].Name != "latest" {
		t.Errorf("Packages[0].Tags = %v, want [latest]", pkg.Tags)
	}
	if pkg.Links == nil || pkg.Links.WebPath != "/grp/proj/-/packages/10" {
		t.Errorf("Packages[0].Links = %+v, want WebPath=/grp/proj/-/packages/10", pkg.Links)
	}
}

// TestPackageGroupList_MissingGroupID verifies GroupList rejects an empty group_id.
func TestPackageGroupList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := GroupList(context.Background(), client, GroupListInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id")
	}
	if !strings.Contains(err.Error(), "group_id is required") {
		t.Errorf("error = %v, want group_id required", err)
	}
}

// TestPackageGroupList_ContextCancelled verifies GroupList returns early when
// the context is already cancelled.
func TestPackageGroupList_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GroupList(ctx, client, GroupListInput{GroupID: "99"})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

// TestPackageGroupList_APIError verifies GroupList wraps API failures with a hint.
func TestPackageGroupList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	}))

	_, err := GroupList(context.Background(), client, GroupListInput{GroupID: "missing"})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "packageGroupList") {
		t.Errorf("error = %v, want packageGroupList prefix", err)
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	err := Delete(context.Background(), nil, client, DeleteInput{
		PackageID: "10",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestPackageDelete_MissingPackageID verifies PackageDelete when missing package ID.
func TestPackageDelete_MissingPackageID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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

// assertFileNameShapeResult checks one file-name-contract outcome for op
// (Publish/Download): success for valid names, or an ErrInvalidFileName
// carrying the segment-rule hint for rejected ones.
func assertFileNameShapeResult(t *testing.T, op, fileName, wantErr string, err error) {
	t.Helper()
	if wantErr == "" {
		if err != nil {
			t.Fatalf("%s() error = %v, want success for directory-structured file_name", op, err)
		}
		return
	}
	if err == nil {
		t.Fatalf("%s() = nil error, want ErrInvalidFileName for %q", op, fileName)
	}
	if !errors.Is(err, gl.ErrInvalidFileName) {
		t.Fatalf("error = %v, want errors.Is ErrInvalidFileName", err)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("error = %q, want segment-rule hint containing %q", err.Error(), wantErr)
	}
}

// TestPublish_FileNameShapes verifies the v2.58.2 file-name contract through
// the publish handler, as table-driven subtests:
//
//   - DirectoryStructure: a file_name with "/" separators reaches the API
//     with the separators preserved and each segment escaped individually.
//   - InvalidDotSegment: a "." segment is rejected client-side by client-go's
//     ErrInvalidFileName before any request, and the wrapped error carries
//     the segment-rule hint.
func TestPublish_FileNameShapes(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		wantErr  string // empty = expect success
	}{
		{name: "DirectoryStructure", fileName: "dist/linux-amd64/tool.bin"},
		{name: "InvalidDotSegment", fileName: "dist/./tool.bin", wantErr: "must not be empty"},
		{name: "InvalidParentSegment", fileName: "../escape.bin", wantErr: "must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var handler http.Handler
			if tt.wantErr == "" {
				handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// EscapedPath distinguishes real "/" separators from the
					// %2F-escaped flat name pre-2.58.2 client-go sent (Go
					// preserves %2F escapes, and canonicalizes needless ones
					// like %2E away).
					if got := r.URL.EscapedPath(); got != "/api/v4/projects/42/packages/generic/pkg/1.0.0/dist/linux-amd64/tool.bin" {
						t.Errorf("escaped path = %q, want directory separators preserved (no %%2F)", got)
					}
					testutil.RespondJSON(w, http.StatusCreated, `{"message":"201 Created"}`)
				})
			} else {
				// The SDK must reject the name before any request is issued.
				handler = testutil.ForbiddenHandler(t)
			}
			client := testutil.NewTestClient(t, handler)

			_, err := Publish(context.Background(), nil, client, PublishInput{
				ProjectID:      "42",
				PackageName:    "pkg",
				PackageVersion: "1.0.0",
				FileName:       tt.fileName,
				ContentBase64:  "aGVsbG8=",
			})
			assertFileNameShapeResult(t, "Publish", tt.fileName, tt.wantErr, err)
		})
	}
}

// TestDownload_FileNameShapes verifies the v2.58.2 file-name contract on the
// download path, as table-driven subtests:
//
//   - DirectoryStructure: a directory-structured file_name reaches the API
//     with real "/" separators and the file streams to disk.
//   - InvalidDotSegment: FormatPackageURL rejects the name client-side with
//     ErrInvalidFileName (no request leaves) and the wrapped error carries
//     the segment-rule hint.
func TestDownload_FileNameShapes(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		wantErr  string // empty = expect success
	}{
		{name: "DirectoryStructure", fileName: "dist/linux-amd64/tool.bin"},
		{name: "InvalidDotSegment", fileName: "dist/../escape.bin", wantErr: "must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var handler http.Handler
			if tt.wantErr == "" {
				handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if got := r.URL.EscapedPath(); got != "/api/v4/projects/42/packages/generic/pkg/1.0.0/dist/linux-amd64/tool.bin" {
						t.Errorf("escaped path = %q, want directory separators preserved (no %%2F)", got)
					}
					w.Header().Set("Content-Type", "application/octet-stream")
					_, _ = w.Write([]byte("binary-content"))
				})
			} else {
				handler = testutil.ForbiddenHandler(t)
			}
			client := testutil.NewTestClient(t, handler)

			out, err := Download(context.Background(), nil, client, DownloadInput{
				ProjectID:      "42",
				PackageName:    "pkg",
				PackageVersion: "1.0.0",
				FileName:       tt.fileName,
				OutputPath:     filepath.Join(t.TempDir(), "out", "tool.bin"),
			})
			if tt.wantErr == "" && err == nil && out.Size != int64(len("binary-content")) {
				t.Errorf("Size = %d, want %d", out.Size, len("binary-content"))
			}
			assertFileNameShapeResult(t, "Download", tt.fileName, tt.wantErr, err)
		})
	}
}

// ---------------------------------------------------------------------------
// Fields GitLab sends beside the ones the SDK's own Package models
// ---------------------------------------------------------------------------.

// packageSentJSON is one package as GitLab renders it, carrying the keys the
// SDK's own Package leaves out: who published it, the Conan recipe name, the
// owning project a group listing names, and the package's other versions with
// the tags and the pipeline each of those carries.
const packageSentJSON = `{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"conan",` +
	`"status":"default","creator_id":57,"conan_package_name":"my-pkg",` +
	`"project_id":42,"project_path":"group/project",` +
	`"versions":[{"id":9,"version":"0.9.0","created_at":"2026-01-02T03:04:05Z",` +
	`"tags":[{"id":3,"package_id":9,"name":"stable","created_at":"2026-01-02T03:04:05Z",` +
	`"updated_at":"2026-01-03T03:04:05Z"}],` +
	`"pipeline":{"id":77,"iid":4,"project_id":42,"sha":"abc123","ref":"main","status":"success",` +
	`"source":"push","created_at":"2026-01-02T03:04:05Z","updated_at":"2026-01-02T04:04:05Z",` +
	`"web_url":"https://gitlab.example.com/p/-/pipelines/77",` +
	`"user":{"id":5,"username":"alice","name":"Alice"}}}]}`

// packageWithoutConditionalJSON is a package rendered by the project listing,
// where GitLab names no Conan recipe, no owning project and no other version.
const packageWithoutConditionalJSON = `{"id":10,"name":"my-pkg","version":"1.0.0",` +
	`"package_type":"generic","status":"default","creator_id":57}`

// packagesClient answers the project and group listing endpoints with body.
func packagesClient(t *testing.T, body string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
}

// packageCalls are the two listing handlers that answer with packages.
var packageCalls = []struct {
	name string
	call func(client *gitlabclient.Client) (ListItem, error)
}{
	{name: "list", call: func(client *gitlabclient.Client) (ListItem, error) {
		out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
		if err != nil {
			return ListItem{}, err
		}
		if len(out.Packages) != 1 {
			return ListItem{}, errNoPackage
		}
		return out.Packages[0], nil
	}},
	{name: "group_list", call: func(client *gitlabclient.Client) (ListItem, error) {
		out, err := GroupList(context.Background(), client, GroupListInput{GroupID: "7"})
		if err != nil {
			return ListItem{}, err
		}
		if len(out.Packages) != 1 {
			return ListItem{}, errNoPackage
		}
		return out.Packages[0].ListItem, nil
	}},
}

// errNoPackage reports a listing handler that answered with no package.
var errNoPackage = errors.New("the handler published no package")

// TestPackages_PublishTheFieldsGitLabSendsBesideTheSDKs verifies both listing
// handlers publish the five keys the SDK's own Package does not model, read
// off the captured response, including every field of a nested version.
func TestPackages_PublishTheFieldsGitLabSendsBesideTheSDKs(t *testing.T) {
	for _, packageCall := range packageCalls {
		t.Run(packageCall.name, func(t *testing.T) {
			item, err := packageCall.call(packagesClient(t, `[`+packageSentJSON+`]`))
			if err != nil {
				t.Fatalf("%s: %v", packageCall.name, err)
			}
			if item.CreatorID != 57 {
				t.Errorf("creator_id = %d, want 57", item.CreatorID)
			}
			if item.ConanPackageName != "my-pkg" {
				t.Errorf("conan_package_name = %q, want my-pkg", item.ConanPackageName)
			}
			if item.ProjectID != 42 || item.ProjectPath != "group/project" {
				t.Errorf("owning project = %d / %q, want 42 / group/project", item.ProjectID, item.ProjectPath)
			}
			assertSentVersions(t, item.Versions)
		})
	}
}

// wantSentVersion is the one other version packageSentJSON carries, with the
// tag pointing at it and the pipeline that built it, as the handler should
// publish them.
var wantSentVersion = toolutil.PackageVersionOutput{
	ID:      9,
	Version: "0.9.0",
	Tags:    []toolutil.PackageTagOutput{{ID: 3, PackageID: 9, Name: "stable"}},
	Pipeline: &toolutil.PackagePipelineOutput{
		ID: 77, IID: 4, ProjectID: 42, SHA: "abc123", Ref: "main", Status: "success",
		Source: "push", WebURL: "https://gitlab.example.com/p/-/pipelines/77",
		User: &toolutil.UserBasicOutput{ID: 5, Username: "alice", Name: "Alice"},
	},
}

// assertSentVersions reports a package's other versions that lost any field of
// the version, its tags or the pipeline that built it. The timestamps are
// compared only for presence, since the fixture's are the only ones that could
// be there.
func assertSentVersions(t *testing.T, versions []toolutil.PackageVersionOutput) {
	t.Helper()
	if len(versions) != 1 {
		t.Fatalf("versions = %+v, want the one version GitLab sent", versions)
	}
	got := versions[0]
	if got.CreatedAt == nil || len(got.Tags) != 1 || got.Tags[0].CreatedAt == nil || got.Tags[0].UpdatedAt == nil {
		t.Fatalf("versions[0] = %+v, want the version and its tag with their timestamps", got)
	}
	if got.Pipeline == nil || got.Pipeline.CreatedAt == nil || got.Pipeline.UpdatedAt == nil {
		t.Fatalf("versions[0].pipeline = %+v, want the pipeline with its timestamps", got.Pipeline)
	}
	// Compared with the timestamps cleared, so one comparison covers every
	// other field of the version, the tag and the pipeline at once.
	got.CreatedAt, got.Tags[0].CreatedAt, got.Tags[0].UpdatedAt = nil, nil, nil
	pipeline := *got.Pipeline
	pipeline.CreatedAt, pipeline.UpdatedAt = nil, nil
	got.Pipeline = &pipeline
	if !reflect.DeepEqual(got, wantSentVersion) {
		t.Errorf("versions[0] = %+v, want %+v", got, wantSentVersion)
	}
}

// TestPackages_OmitTheFieldsGitLabDidNotSend verifies a package answered
// without the Conan name, the owning project and the other versions publishes
// none of them, while the creator GitLab always sends still arrives.
func TestPackages_OmitTheFieldsGitLabDidNotSend(t *testing.T) {
	for _, packageCall := range packageCalls {
		t.Run(packageCall.name, func(t *testing.T) {
			item, err := packageCall.call(packagesClient(t, `[`+packageWithoutConditionalJSON+`]`))
			if err != nil {
				t.Fatalf("%s: %v", packageCall.name, err)
			}
			if item.ConanPackageName != "" {
				t.Errorf("conan_package_name = %q, want none for a generic package", item.ConanPackageName)
			}
			if item.ProjectID != 0 || item.ProjectPath != "" {
				t.Errorf("owning project = %d / %q, want none", item.ProjectID, item.ProjectPath)
			}
			if item.Versions != nil {
				t.Errorf("versions = %+v, want none", item.Versions)
			}
			if item.CreatorID != 57 {
				t.Errorf("creator_id = %d, want 57", item.CreatorID)
			}
		})
	}
}

// TestPackages_UnreadableCapturedFields verifies both listing handlers report
// the captured response's decode failure rather than a package missing what
// GitLab sent. The SDK's own Package has no creator_id, so only the read
// beside it can notice GitLab sent a string there.
func TestPackages_UnreadableCapturedFields(t *testing.T) {
	const poisoned = `[{"id":10,"name":"my-pkg","version":"1.0.0","creator_id":"nobody"}]`
	cases := make([]testutil.CapturedCase, 0, len(packageCalls))
	for _, packageCall := range packageCalls {
		cases = append(cases, testutil.CapturedCase{Name: packageCall.name, Call: func() error {
			_, err := packageCall.call(packagesClient(t, poisoned))
			return err
		}})
	}
	testutil.AssertCapturedDecodeFailures(t, cases)
}

// TestGroupList_SkipsANullPackageAndKeepsTheCapturedFieldsPaired verifies a
// listing whose array carries a null entry publishes the packages beside it
// with the fields read off their own position in the captured answer.
func TestGroupList_SkipsANullPackageAndKeepsTheCapturedFieldsPaired(t *testing.T) {
	client := packagesClient(t, `[null,`+packageSentJSON+`]`)
	out, err := GroupList(t.Context(), client, GroupListInput{GroupID: "7"})
	if err != nil {
		t.Fatalf("GroupList: %v", err)
	}
	if len(out.Packages) != 1 {
		t.Fatalf("packages = %d, want the one package beside the null", len(out.Packages))
	}
	if out.Packages[0].CreatorID != 57 {
		t.Errorf("creator_id = %d, want 57 read off the second position", out.Packages[0].CreatorID)
	}
}

// TestPackageToListItem_LeavesOutTheCollectionsGitLabDidNotSend verifies a
// package carrying no pipeline history and no tags publishes neither, rather
// than publishing an empty list for each.
func TestPackageToListItem_LeavesOutTheCollectionsGitLabDidNotSend(t *testing.T) {
	item := packageToListItem(&gl.Package{ID: 1, Name: "pkg", Version: "1.0.0"}, toolutil.PackageExtra{})
	if item.Pipelines != nil {
		t.Errorf("Pipelines = %+v, want none", item.Pipelines)
	}
	if item.Tags != nil {
		t.Errorf("Tags = %+v, want none", item.Tags)
	}
	if item.Links != nil {
		t.Errorf("Links = %+v, want none", item.Links)
	}
}

// TestPublish_SelectAndURL verifies the publish request carries the default
// response selector unless one was given, and that the answer names the URL
// the file can be fetched from.
func TestPublish_SelectAndURL(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		select_ string
		want    string
	}{
		{name: "default", want: "select=package_file"},
		{name: "given", select_: "package_file_details", want: "select=package_file_details"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var query string
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query = r.URL.RawQuery
				testutil.RespondJSON(w, http.StatusCreated, publishResponseJSON)
			}))
			out, err := Publish(context.Background(), nil, client, PublishInput{
				ProjectID:      "42",
				PackageName:    testPackageName,
				PackageVersion: "1.0.0",
				FileName:       testFileName,
				ContentBase64:  testBase64Content,
				Select:         testCase.select_,
			})
			if err != nil {
				t.Fatalf("Publish: %v", err)
			}
			if !strings.Contains(query, testCase.want) {
				t.Errorf("publish query %q missing %q", query, testCase.want)
			}
			// The path is escaped the way the SDK builds it, dots included.
			if !strings.HasSuffix(out.URL, "/projects/42/packages/generic/my-pkg/1%2E0%2E0/app%2Etar%2Egz") {
				t.Errorf("URL = %q, want the generic package path", out.URL)
			}
		})
	}
}

// TestGroupList_FiltersReachTheRequest verifies every optional filter the
// group listing accepts is sent when it was given and left out when it was
// not, which only the query the handler built can say.
func TestGroupList_FiltersReachTheRequest(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		input GroupListInput
		want  []string
		omit  []string
	}{
		{
			name: "every filter",
			input: GroupListInput{
				GroupID: "7", ExcludeSubgroups: true, PackageName: "my-pkg", PackageType: "generic",
				OrderBy: "created_at", Sort: "desc", IncludeVersionless: true, Status: "hidden",
			},
			want: []string{
				"exclude_subgroups=true", "package_name=my-pkg", "package_type=generic",
				"order_by=created_at", "sort=desc", "include_versionless=true", "status=hidden",
			},
		},
		{
			name:  "none of them",
			input: GroupListInput{GroupID: "7"},
			omit: []string{
				"exclude_subgroups", "package_name", "package_type",
				"order_by", "sort", "include_versionless", "status",
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var query string
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query = r.URL.RawQuery
				testutil.RespondJSON(w, http.StatusOK, `[`+packageSentJSON+`]`)
			}))
			if _, err := GroupList(context.Background(), client, testCase.input); err != nil {
				t.Fatalf("GroupList: %v", err)
			}
			for _, want := range testCase.want {
				if !strings.Contains(query, want) {
					t.Errorf("query %q missing %q", query, want)
				}
			}
			for _, omit := range testCase.omit {
				if strings.Contains(query, omit) {
					t.Errorf("query %q carries %q it was not given", query, omit)
				}
			}
		})
	}
}

// TestPackageIdentifiers_RefuseAZeroID verifies the three handlers that take a
// package identifier refuse a zero as firmly as an identifier that is not a
// number, since GitLab numbers packages and files from one.
func TestPackageIdentifiers_RefuseAZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	for _, testCase := range []struct {
		name string
		call func() error
		want string
	}{
		{name: "file_list zero package", want: "package_id must be a positive integer", call: func() error {
			_, err := FileList(context.Background(), client, FileListInput{ProjectID: "42", PackageID: "0"})
			return err
		}},
		{name: "delete zero package", want: "package_id must be a positive integer", call: func() error {
			return Delete(context.Background(), nil, client, DeleteInput{ProjectID: "42", PackageID: "0"})
		}},
		{name: "file_delete zero package", want: "package_id must be a positive integer", call: func() error {
			return FileDelete(context.Background(), nil, client,
				FileDeleteInput{ProjectID: "42", PackageID: "0", PackageFileID: "20"})
		}},
		{name: "file_delete zero file", want: "package_file_id must be a positive integer", call: func() error {
			return FileDelete(context.Background(), nil, client,
				FileDeleteInput{ProjectID: "42", PackageID: "10", PackageFileID: "0"})
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.call()
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("error = %v, want one naming %q", err, testCase.want)
			}
		})
	}
}

// TestPackageOptions_UseTheMetadataTable verifies the spec builder takes the
// usage, aliases, related actions and description from the metadata entry for
// the action, and keeps the shared defaults for an action the table does not
// name. No action reaches that fallback today, so the builder is called
// directly rather than through ActionSpecs.
func TestPackageOptions_UseTheMetadataTable(t *testing.T) {
	options := packageOptions("publish", "gitlab_package_publish")
	meta := packageActionMetadata["publish"]
	if options.Usage != meta.usage {
		t.Errorf("Usage = %q, want the metadata usage", options.Usage)
	}
	if len(options.Aliases) != len(meta.aliases)+1 || options.Aliases[0] != "gitlab_package_publish" {
		t.Errorf("Aliases = %v, want the tool name followed by the metadata aliases", options.Aliases)
	}
	if len(options.RelatedActions) != len(meta.related) {
		t.Errorf("RelatedActions = %v, want the metadata related actions", options.RelatedActions)
	}
	if options.IndividualTool.Description != meta.description {
		t.Errorf("Description = %q, want the metadata description", options.IndividualTool.Description)
	}

	unlisted := packageOptions("unlisted", "gitlab_package_unlisted")
	if unlisted.Usage != "Use to execute packages domain action." {
		t.Errorf("Usage = %q, want the shared one", unlisted.Usage)
	}
	if len(unlisted.Aliases) != 1 || unlisted.Aliases[0] != "gitlab_package_unlisted" {
		t.Errorf("Aliases = %v, want the tool name alone", unlisted.Aliases)
	}
	if unlisted.RelatedActions != nil || unlisted.IndividualTool.Description != "" {
		t.Errorf("unlisted action carries %v / %q, want neither", unlisted.RelatedActions, unlisted.IndividualTool.Description)
	}
}
