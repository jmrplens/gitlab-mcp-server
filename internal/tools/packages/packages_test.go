// packages_test.go contains unit tests for GitLab Generic Packages API
// operations (publish, download, list, file_list, delete, file_delete).
package packages

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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

// publishResponseJSON is one published file as GitLab answers with it. No two
// numbers in it agree: while the file's id and its store were both 1, the
// handler could publish the store as the file's id and every test stayed
// green, since an assignment has no branch either gate can flip.
const publishResponseJSON = `{
	"id": 1,
	"package_id": 10,
	"file_name": "app.tar.gz",
	"size": 1024,
	"file_sha256": "abc123hash",
	"file_md5": "md5hash",
	"file_sha1": "sha1hash",
	"file_store": 4,
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
	if out.FileStore != 4 {
		t.Errorf("FileStore = %d, want 4", out.FileStore)
	}
	if out.SHA256 != "abc123hash" {
		t.Errorf("SHA256 = %q, want %q", out.SHA256, "abc123hash")
	}
	// Both are published in RFC 3339, the form GitLab sent them in; they used to
	// be Go's time.String form, which no display helper here can parse.
	if out.CreatedAt != "2026-06-01T10:00:00Z" {
		t.Errorf("CreatedAt = %q, want 2026-06-01T10:00:00Z", out.CreatedAt)
	}
	if out.UpdatedAt != "2026-06-01T11:00:00Z" {
		t.Errorf("UpdatedAt = %q, want 2026-06-01T11:00:00Z", out.UpdatedAt)
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
				`[{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"generic","status":"default","pipeline":{"id":77,"iid":4,"project_id":42,"source":"push","status":"success","ref":"main","sha":"abc123","web_url":"https://gitlab.example.com/project/-/pipelines/77","user":{"id":5,"username":"alice","name":"Alice","public_email":"alice@example.com","locked":true,"web_url":"https://gitlab.example.com/alice"}},"pipelines":[],"last_downloaded_at":"2026-06-01T12:00:00.123Z","tags":[{"id":1,"package_id":10,"name":"latest"}],"_links":{"web_path":"/project/-/packages/10"}}]`,
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
	assertListedPipeline(t, out.Packages[0])
	// RFC 3339, seconds and UTC, which is what the card's time helper reads.
	if out.Packages[0].LastDownloadedAt != "2026-06-01T12:00:00Z" {
		t.Errorf("Packages[0].LastDownloadedAt = %q, want 2026-06-01T12:00:00Z", out.Packages[0].LastDownloadedAt)
	}
	if len(out.Packages[0].Tags) != 1 || out.Packages[0].Tags[0].Name != "latest" {
		t.Errorf("Packages[0].Tags = %v, want [latest]", out.Packages[0].Tags)
	}
	if out.Packages[0].Links == nil || out.Packages[0].Links.WebPath != "/project/-/packages/10" {
		t.Errorf("Packages[0].Links = %+v, want WebPath=/project/-/packages/10", out.Packages[0].Links)
	}
}

// assertListedPipeline holds the listed package of TestPackageList_Success to
// the pipeline that last built it. The pipeline's iid, project and source, and
// its user's address and lock, are keys client-go does not decode: the listing
// reads them off the captured answer. It also holds the item to publishing
// neither pipelines, which GitLab sends as its constant empty list, nor
// versions, which it sends only to a request for one package.
func assertListedPipeline(t *testing.T, item ListItem) {
	t.Helper()
	pipeline := item.Pipeline
	if pipeline == nil || pipeline.ID != 77 || pipeline.IID != 4 || pipeline.ProjectID != 42 || pipeline.Source != "push" {
		t.Fatalf("Pipeline = %+v, want 77 with iid 4, project 42 and source push", pipeline)
	}
	if pipeline.User == nil || pipeline.User.Username != "alice" || pipeline.User.PublicEmail != "alice@example.com" || !pipeline.User.Locked {
		t.Fatalf("Pipeline.User = %+v, want alice, alice@example.com, locked", pipeline.User)
	}
	published, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal item: %v", err)
	}
	for _, key := range []string{`"pipelines"`, `"versions"`} {
		if strings.Contains(string(published), key) {
			t.Errorf("item published as %s, want no %s key", published, key)
		}
	}
}

// TestPackageToListItem_OptionalPipelineFields verifies the pipeline that last
// built a package keeps its timestamps, in RFC 3339 to the second as the
// package's own are although GitLab writes them with milliseconds, its user,
// and the keys the capture read beside it.
func TestPackageToListItem_OptionalPipelineFields(t *testing.T) {
	now := time.Date(2026, 5, 6, 11, 0, 0, 123000000, time.UTC)
	updated := now.Add(time.Minute)
	extra := toolutil.PackageExtra{Pipeline: &toolutil.PackagePipelineExtra{IID: 2, Source: "push"}}
	item := packageToListItem(&gl.Package{
		ID:          99,
		Name:        "pkg",
		Version:     "1.2.3",
		PackageType: "generic",
		Status:      "default",
		CreatedAt:   &now,
		Pipeline: &gl.PackagePipeline{
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
	}, extra)

	if item.CreatedAt != "2026-05-06T11:00:00Z" {
		t.Fatalf("CreatedAt = %q, want 2026-05-06T11:00:00Z", item.CreatedAt)
	}
	pipeline := item.Pipeline
	if pipeline == nil || pipeline.ID != 77 || pipeline.IID != 2 || pipeline.Source != "push" {
		t.Fatalf("pipeline = %+v, want 77 with the keys the capture read", pipeline)
	}
	if pipeline.CreatedAt != "2026-05-06T11:00:00Z" || pipeline.UpdatedAt != "2026-05-06T11:01:00Z" {
		t.Fatalf("pipeline timestamps = %q / %q, want 2026-05-06T11:00:00Z / 2026-05-06T11:01:00Z", pipeline.CreatedAt, pipeline.UpdatedAt)
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
// (delete_api_path), tag timestamps, and the pipeline user object (state,
// avatar_url) are mirrored from the GitLab Package, and that the user's
// created_at, which client-go's BasicUser decodes and GitLab's UserBasic never
// sends, is not published.
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
	if len(item.Tags) != 1 || item.Tags[0].CreatedAt != "2026-05-06T11:00:00Z" || item.Tags[0].UpdatedAt != "2026-05-06T11:00:00Z" {
		t.Errorf("Tags = %+v, want timestamps populated in RFC 3339", item.Tags)
	}
	if item.Pipeline.User == nil || item.Pipeline.User.State != "active" || item.Pipeline.User.AvatarURL == "" {
		t.Errorf("pipeline user = %+v, want full user fields", item.Pipeline.User)
	}
	published, err := json.Marshal(item.Pipeline.User)
	if err != nil {
		t.Fatalf("marshal user: %v", err)
	}
	if strings.Contains(string(published), "created_at") {
		t.Errorf("pipeline user published as %s, want no created_at", published)
	}
}

// TestPackagePipelineToOutput_NilPipeline_ReturnsNil verifies that nil package
// pipeline pointers are converted to nil output values.
func TestPackagePipelineToOutput_NilPipeline_ReturnsNil(t *testing.T) {
	if got := packagePipelineToOutput(nil, &toolutil.PackagePipelineExtra{IID: 4}); got != nil {
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
				`[null,{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"generic","status":"default","project_id":7,"project_path":"grp/proj","pipeline":{"id":77,"iid":5,"project_id":7,"source":"merge_request_event","status":"success","ref":"main","sha":"abc123","web_url":"https://gitlab.example.com/grp/proj/-/pipelines/77"},"tags":[{"id":1,"package_id":10,"name":"latest"}],"_links":{"web_path":"/grp/proj/-/packages/10"}}]`,
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
	// The null ahead of the package holds its own position in the capture, so
	// the pipeline keys client-go does not decode are the ones read beside this
	// package rather than beside the null.
	wantPipeline := toolutil.PackagePipelineOutput{
		ID: 77, IID: 5, ProjectID: 7, Source: "merge_request_event", Status: "success", Ref: "main", SHA: "abc123",
		WebURL: "https://gitlab.example.com/grp/proj/-/pipelines/77",
	}
	if pkg.Pipeline == nil || *pkg.Pipeline != wantPipeline {
		t.Errorf("Packages[0].Pipeline = %+v, want %+v", pkg.Pipeline, wantPipeline)
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
	// The hint is what the comment promises and what a model acts on, so it is
	// asserted rather than left to the operation prefix alone.
	if !strings.Contains(err.Error(), "verify group_id with group.get") {
		t.Errorf("error = %v, want the hint naming how to check the group", err)
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
// The fields client-go's Package models since v3.14.0, and the ones a listing
// never carries
// ---------------------------------------------------------------------------.

// packageSentJSON is one package carrying every key GitLab's package entity
// can send: who published it, the Conan recipe name, the owning project a
// group listing names, and the package's other versions with the tags and the
// pipeline each of those carries. No single response carries all of them,
// since the versions come only with a request for one package and the owning
// project only with a group's listing, which is what lets one fixture show
// which of them each handler publishes.
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

// TestPackages_PublishTheFieldsClientGoDecodes verifies both listing handlers
// publish who published the package and the Conan recipe name, which
// client-go's Package decodes as of v3.14.0, and publish on the package's own
// item no versions key, even from an answer carrying them. A listing never
// carries versions, since the entity leaves them out of a collection, so the
// key belongs to the item package.get fills alone; the owning project reaches a
// reader through the group listing's own item, which the next test holds.
func TestPackages_PublishTheFieldsClientGoDecodes(t *testing.T) {
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
			assertPublishesNoKey(t, item, "versions")
		})
	}
}

// assertPublishesNoKey reports v serializing with any of keys at any depth.
func assertPublishesNoKey(t *testing.T, v any, keys ...string) {
	t.Helper()
	published, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	for _, key := range keys {
		if strings.Contains(string(published), `"`+key+`"`) {
			t.Errorf("%T published as %s, want no %s key", v, published, key)
		}
	}
}

// TestGroupList_PublishesTheOwningProjectOnItsOwnItem verifies the group
// listing publishes the owning project GitLab names there, which client-go
// decodes on the GroupPackage wrapping the package, under the item's own keys.
func TestGroupList_PublishesTheOwningProjectOnItsOwnItem(t *testing.T) {
	out, err := GroupList(t.Context(), packagesClient(t, `[`+packageSentJSON+`]`), GroupListInput{GroupID: "7"})
	if err != nil {
		t.Fatalf("GroupList: %v", err)
	}
	if len(out.Packages) != 1 {
		t.Fatalf("packages = %d, want one", len(out.Packages))
	}
	if got := out.Packages[0]; got.ProjectID != 42 || got.ProjectPath != "group/project" {
		t.Errorf("owning project = %d / %q, want 42 / group/project", got.ProjectID, got.ProjectPath)
	}
}

// TestPackages_OmitTheFieldsGitLabDidNotSend verifies a package answered
// without the Conan name publishes none, while the creator GitLab always sends
// still arrives.
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
			if item.CreatorID != 57 {
				t.Errorf("creator_id = %d, want 57", item.CreatorID)
			}
		})
	}
}

// TestPackages_UnreadableCreator_Refused verifies both listing handlers refuse
// a package whose creator_id is not a number rather than publishing one
// missing what GitLab sent. client-go models creator_id on its own Package as
// of v3.14.0, so its decoder is what refuses it, and the listings read nothing
// beside it any more.
func TestPackages_UnreadableCreator_Refused(t *testing.T) {
	const poisoned = `[{"id":10,"name":"my-pkg","version":"1.0.0","creator_id":"nobody"}]`
	cases := make([]testutil.CapturedCase, 0, len(packageCalls))
	for _, packageCall := range packageCalls {
		cases = append(cases, testutil.CapturedCase{Name: packageCall.name, Call: func() error {
			_, err := packageCall.call(packagesClient(t, poisoned))
			return err
		}})
	}
	testutil.AssertUnreadableBodyRefused(t, cases)
}

// TestPackages_SkipANullPackage verifies both listings, whose array carries a
// null entry, publish the package beside it, whole, with the pipeline keys the
// capture read at the package's own position rather than the null's. The
// project listing used to dereference the null and take the server down.
func TestPackages_SkipANullPackage(t *testing.T) {
	const withPipeline = `{"id":10,"name":"my-pkg","creator_id":57,"pipeline":{"id":77,"iid":4,"source":"push"}}`
	for _, packageCall := range packageCalls {
		t.Run(packageCall.name, func(t *testing.T) {
			item, err := packageCall.call(packagesClient(t, `[null,`+withPipeline+`]`))
			if err != nil {
				t.Fatalf("%s: %v", packageCall.name, err)
			}
			if item.CreatorID != 57 {
				t.Errorf("creator_id = %d, want 57", item.CreatorID)
			}
			if item.Pipeline == nil || item.Pipeline.IID != 4 || item.Pipeline.Source != "push" {
				t.Errorf("pipeline = %+v, want iid 4 and source push", item.Pipeline)
			}
		})
	}
}

// TestPackages_UnreadablePipelineKey_RefusedByTheCapture verifies every
// handler that renders a package refuses an answer whose pipeline carries an
// iid that is not a number rather than publishing the pipeline without it.
// client-go does not decode iid, so only the read beside it can notice.
func TestPackages_UnreadablePipelineKey_RefusedByTheCapture(t *testing.T) {
	const poisoned = `{"id":10,"name":"my-pkg","pipeline":{"id":77,"iid":"four"}}`
	cases := make([]testutil.CapturedCase, 0, len(packageCalls)+1)
	for _, packageCall := range packageCalls {
		cases = append(cases, testutil.CapturedCase{Name: packageCall.name, Call: func() error {
			_, err := packageCall.call(packagesClient(t, `[`+poisoned+`]`))
			return err
		}})
	}
	cases = append(cases, testutil.CapturedCase{Name: "get", Call: func() error {
		_, err := Get(t.Context(), packagesClient(t, poisoned), GetInput{ProjectID: "42", PackageID: "10"})
		return err
	}})
	testutil.AssertCapturedDecodeFailures(t, cases)
}

// TestPackageToListItem_LeavesOutTheCollectionsGitLabDidNotSend verifies a
// package carrying an empty list of tags, and GitLab's constant empty list of
// pipelines, publishes neither key, rather than an empty list for each.
func TestPackageToListItem_LeavesOutTheCollectionsGitLabDidNotSend(t *testing.T) {
	item := packageToListItem(&gl.Package{
		ID: 1, Name: "pkg", Version: "1.0.0",
		Tags: []gl.PackageTag{}, Pipelines: []*gl.PackagePipeline{},
	}, toolutil.PackageExtra{})
	assertPublishesNoKey(t, item, "tags", "pipelines", "pipeline")
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
//
// The URL assertion is also what stands in for the branch that leaves it
// empty. The same coordinates already built the request GitLab accepted, so
// FormatPackageURL cannot fail here and gobco reports its `err == nil` as
// never false; the property behind the guard is that a publish which
// succeeded always publishes a URL, and that is what is asserted.
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
//
// The two flags are also driven one at a time, because a pair of booleans has
// no fixture where no two values agree: with only an all-on and an all-off
// case, the two guards could set each other's parameter and both cases would
// still pass, which is what they did.
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
		{
			name:  "only exclude_subgroups",
			input: GroupListInput{GroupID: "7", ExcludeSubgroups: true},
			want:  []string{"exclude_subgroups=true"},
			omit:  []string{"include_versionless"},
		},
		{
			name:  "only include_versionless",
			input: GroupListInput{GroupID: "7", IncludeVersionless: true},
			want:  []string{"include_versionless=true"},
			omit:  []string{"exclude_subgroups"},
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

// TestPackageToListItem_EveryFieldComesFromItsOwnSource verifies the whole
// converted package against one built by hand, over a fixture where no two
// values agree. An assignment carries no branch, so neither gate can see a
// field read from its neighbor: measured by hand against the suite as it
// stood, the package's version and status could be swapped, a pipeline's ref
// and sha could be swapped, and the publishing user's avatar and profile URLs
// could be swapped, with every test still green.
//
// Every timestamp carries a fraction of a second, as GitLab writes them, and
// every one is published to the second: the package's own, its tag's and its
// pipeline's used to be published at two precisions in one answer.
func TestPackageToListItem_EveryFieldComesFromItsOwnSource(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 100000000, time.UTC)
	downloaded := time.Date(2026, 2, 3, 4, 5, 6, 200000000, time.UTC)
	tagCreated := time.Date(2026, 3, 4, 5, 6, 7, 300000000, time.UTC)
	tagUpdated := time.Date(2026, 4, 5, 6, 7, 8, 400000000, time.UTC)
	pipeCreated := time.Date(2026, 5, 6, 7, 8, 9, 500000000, time.UTC)
	pipeUpdated := time.Date(2026, 6, 7, 8, 9, 10, 600000000, time.UTC)
	userCreated := time.Date(2026, 7, 8, 9, 10, 11, 0, time.UTC)

	// The pipeline that last built the package, and one GitLab lists beside
	// it, differ in every field. GitLab has rendered that list as a constant
	// empty one since 16.1, so nothing of it is published even where client-go
	// decoded an entry, and reading it where the pipeline belongs is visible.
	current := &gl.PackagePipeline{
		ID:        71,
		Status:    "success",
		Ref:       "main",
		SHA:       "aaa111",
		WebURL:    "https://gitlab.example.com/p/-/pipelines/71",
		CreatedAt: &pipeCreated,
		UpdatedAt: &pipeUpdated,
		User: &gl.BasicUser{
			ID:        5,
			Username:  "alice",
			Name:      "Alice Liddell",
			State:     "active",
			AvatarURL: "https://gitlab.example.com/uploads/avatar.png",
			WebURL:    "https://gitlab.example.com/alice",
			CreatedAt: &userCreated,
		},
	}
	previous := &gl.PackagePipeline{
		ID: 62, Status: "failed", Ref: "release-1", SHA: "bbb222",
		WebURL: "https://gitlab.example.com/p/-/pipelines/62",
	}

	extra := toolutil.PackageExtra{
		Pipeline: &toolutil.PackagePipelineExtra{
			IID: 8, ProjectID: 42, Source: "push",
			User: &toolutil.UserBasicExtra{Locked: true, PublicEmail: "alice@example.com"},
		},
	}

	got := packageToListItem(&gl.Package{
		ID:          10,
		Name:        "my-pkg",
		Version:     "1.0.0",
		PackageType: "conan",
		Status:      "hidden",
		Links: &gl.PackageLinks{
			WebPath:       "/grp/proj/-/packages/10",
			DeleteAPIPath: "/api/v4/projects/42/packages/10",
		},
		Pipeline:         current,
		Pipelines:        []*gl.PackagePipeline{previous},
		CreatedAt:        &created,
		LastDownloadedAt: &downloaded,
		CreatorID:        57,
		ConanPackageName: "recipe-name",
		Tags: []gl.PackageTag{{
			ID: 3, PackageID: 10, Name: "latest",
			CreatedAt: &tagCreated, UpdatedAt: &tagUpdated,
		}},
		// A listing never carries versions, so the conversion a listing runs
		// leaves them out even where the struct holds some.
		Versions: []*gl.PackageVersion{{ID: 9, Version: "0.9.0"}},
	}, extra)

	want := ListItem{
		ID:               10,
		Name:             "my-pkg",
		Version:          "1.0.0",
		PackageType:      "conan",
		Status:           "hidden",
		ConanPackageName: "recipe-name",
		Links: &LinksItem{
			WebPath:       "/grp/proj/-/packages/10",
			DeleteAPIPath: "/api/v4/projects/42/packages/10",
		},
		// The user's created_at, which client-go decodes, is not a key GitLab's
		// UserBasic sends, so the published user has none.
		Pipeline: &toolutil.PackagePipelineOutput{
			ID:        71,
			IID:       8,
			ProjectID: 42,
			SHA:       "aaa111",
			Ref:       "main",
			Status:    "success",
			Source:    "push",
			CreatedAt: "2026-05-06T07:08:09Z",
			UpdatedAt: "2026-06-07T08:09:10Z",
			WebURL:    "https://gitlab.example.com/p/-/pipelines/71",
			User: &toolutil.UserBasicOutput{
				ID:          5,
				Username:    "alice",
				PublicEmail: "alice@example.com",
				Name:        "Alice Liddell",
				State:       "active",
				Locked:      true,
				AvatarURL:   "https://gitlab.example.com/uploads/avatar.png",
				WebURL:      "https://gitlab.example.com/alice",
			},
		},
		// RFC 3339, which is what the card's time helper reads; Go's String
		// form, published here before, reached the card unparsed.
		CreatedAt:        "2026-01-02T03:04:05Z",
		LastDownloadedAt: "2026-02-03T04:05:06Z",
		CreatorID:        57,
		Tags: []toolutil.PackageTagOutput{{
			ID: 3, PackageID: 10, Name: "latest",
			CreatedAt: "2026-03-04T05:06:07Z", UpdatedAt: "2026-04-05T06:07:08Z",
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("packageToListItem() =\n%+v\nwant:\n%+v", got, want)
	}
}

// TestPackages_PaginationComesFromTheResponseHeaders verifies each of the three
// listings fills every pagination field from the header GitLab sent for it.
// Nothing asserted the block at all, so all three could hand a model a page and
// no way to ask for the next one while staying green; the headers carry six
// distinct numbers, so a field read off a neighboring header is visible too.
func TestPackages_PaginationComesFromTheResponseHeaders(t *testing.T) {
	headers := testutil.PaginationHeaders{
		Page: "3", PerPage: "20", Total: "97", TotalPages: "5", NextPage: "4", PrevPage: "2",
	}
	want := toolutil.PaginationOutput{
		Page: 3, PerPage: 20, TotalItems: 97, TotalPages: 5, NextPage: 4, PrevPage: 2, HasMore: true,
	}
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":10,"name":"my-pkg"}]`, headers)
	}))

	for _, testCase := range []struct {
		name string
		call func() (toolutil.PaginationOutput, error)
	}{
		{name: "list", call: func() (toolutil.PaginationOutput, error) {
			out, err := List(t.Context(), client, ListInput{ProjectID: "42"})
			return out.Pagination, err
		}},
		{name: "group_list", call: func() (toolutil.PaginationOutput, error) {
			out, err := GroupList(t.Context(), client, GroupListInput{GroupID: "7"})
			return out.Pagination, err
		}},
		{name: "file_list", call: func() (toolutil.PaginationOutput, error) {
			out, err := FileList(t.Context(), client, FileListInput{ProjectID: "42", PackageID: "10"})
			return out.Pagination, err
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := testCase.call()
			if err != nil {
				t.Fatalf("%s: %v", testCase.name, err)
			}
			if got != want {
				t.Errorf("pagination = %+v, want %+v", got, want)
			}
		})
	}
}

// TestPackages_ContextCancelledMidFlight_AbandonsTheRequest verifies that the
// upload, the file listing and the two deletes hand the caller's context to
// the request they send, so the action deadline and an abandoned HTTP POST end
// them rather than leaving them running against GitLab.
//
// client-go takes that context only as the gl.WithContext request option and
// builds the request from context.Background() without it, which is how all
// four shipped; the upload is the one that matters most, since its body may
// run to the configured upload ceiling and nothing else bounds the transfer.
// The download, which builds its request by hand, is held in
// packages_stream_test.go. The context is cancelled once the request has
// arrived, since the guard at the top of each handler answers one cancelled up
// front before any request exists.
func TestPackages_ContextCancelledMidFlight_AbandonsTheRequest(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		call   func(context.Context, *gitlabclient.Client) error
	}{
		{"publish", http.StatusCreated, publishResponseJSON, func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Publish(ctx, nil, c, PublishInput{
				ProjectID:      "42",
				PackageName:    testPackageName,
				PackageVersion: "1.0.0",
				FileName:       testFileName,
				ContentBase64:  testBase64Content,
			})
			return err
		}},
		{"file_list", http.StatusOK, `[]`, func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := FileList(ctx, c, FileListInput{ProjectID: "42", PackageID: "10"})
			return err
		}},
		{"get", http.StatusOK, packageGetJSON, func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Get(ctx, c, GetInput{ProjectID: "42", PackageID: "10"})
			return err
		}},
		{"delete", http.StatusNoContent, "", func(ctx context.Context, c *gitlabclient.Client) error {
			return Delete(ctx, nil, c, DeleteInput{ProjectID: "42", PackageID: "10"})
		}},
		{"file_delete", http.StatusNoContent, "", func(ctx context.Context, c *gitlabclient.Client) error {
			return FileDelete(ctx, nil, c, FileDeleteInput{ProjectID: "42", PackageID: "10", PackageFileID: "20"})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, client := testutil.CancelOnArrival(t, func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, tt.body)
			})
			if err := tt.call(ctx, client); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v after the context was cancelled mid-flight, want context.Canceled", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------.

// pathPackageGet is the one package's own route, which the delete shares.
const pathPackageGet = pathPackageDelete

// packageGetJSON is one package as the single-package endpoint renders it:
// packageSentJSON's keys less the owning project, which only a group listing
// is sent, plus the pipeline that last built the package, with every value
// distinct from its neighbors so a field read from the wrong key is visible,
// and the users who ran both pipelines carrying the two UserBasic keys
// client-go's BasicUser leaves out.
const packageGetJSON = `{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"conan",` +
	`"status":"default","creator_id":57,"conan_package_name":"recipe-name",` +
	`"pipeline":{"id":78,"iid":5,"project_id":42,"sha":"def456","ref":"release","status":"running",` +
	`"source":"web","created_at":"2026-01-09T03:04:05.123Z","updated_at":"2026-01-10T03:04:05Z",` +
	`"web_url":"https://gitlab.example.com/p/-/pipelines/78",` +
	`"user":{"id":6,"username":"bob","public_email":"bob@example.com","name":"Bob",` +
	`"state":"active","locked":false,"avatar_url":"https://gitlab.example.com/b.png",` +
	`"web_url":"https://gitlab.example.com/bob"}},"pipelines":[],` +
	`"tags":[{"id":2,"package_id":10,"name":"latest","created_at":"2026-01-03T03:04:05.5Z",` +
	`"updated_at":"2026-01-04T03:04:05.25Z"}],` +
	`"versions":[{"id":9,"version":"0.9.0","created_at":"2026-01-02T03:04:05.75Z",` +
	`"tags":[{"id":3,"package_id":9,"name":"stable","created_at":"2026-01-05T03:04:05Z",` +
	`"updated_at":"2026-01-06T03:04:05Z"}],` +
	`"pipeline":{"id":77,"iid":4,"project_id":42,"sha":"abc123","ref":"main","status":"success",` +
	`"source":"push","created_at":"2026-01-07T03:04:05Z","updated_at":"2026-01-08T03:04:05Z",` +
	`"web_url":"https://gitlab.example.com/p/-/pipelines/77",` +
	`"user":{"id":5,"username":"alice","public_email":"alice@example.com","name":"Alice Liddell",` +
	`"state":"active","locked":true,"avatar_url":"https://gitlab.example.com/a.png",` +
	`"web_url":"https://gitlab.example.com/alice"}}}]}`

// wantGotVersions is how Get publishes the one other version packageGetJSON
// carries, as a caller reads it: the version and its tag from client-go's
// PackageVersion, eight keys of the pipeline from its PackagePipeline and the
// other three from the capture, and six keys of the user from BasicUser and
// the other two from the capture.
const wantGotVersions = `[{"id":9,"version":"0.9.0","created_at":"2026-01-02T03:04:05Z",` +
	`"tags":[{"id":3,"package_id":9,"name":"stable","created_at":"2026-01-05T03:04:05Z",` +
	`"updated_at":"2026-01-06T03:04:05Z"}],` +
	`"pipeline":{"id":77,"iid":4,"project_id":42,"sha":"abc123","ref":"main","status":"success",` +
	`"source":"push","created_at":"2026-01-07T03:04:05Z","updated_at":"2026-01-08T03:04:05Z",` +
	`"web_url":"https://gitlab.example.com/p/-/pipelines/77",` +
	`"user":{"id":5,"username":"alice","public_email":"alice@example.com","name":"Alice Liddell",` +
	`"state":"active","locked":true,"avatar_url":"https://gitlab.example.com/a.png",` +
	`"web_url":"https://gitlab.example.com/alice"}}}]`

// wantGotPipeline is how Get publishes the pipeline that last built the
// package packageGetJSON carries: the same eleven keys, and the same eight of
// its user, as each version's pipeline, which is what a caller reading both in
// one answer is owed. Its time is RFC 3339 to the second, as every other time
// in the answer is, although GitLab wrote it with milliseconds.
const wantGotPipeline = `{"id":78,"iid":5,"project_id":42,"sha":"def456","ref":"release","status":"running",` +
	`"source":"web","created_at":"2026-01-09T03:04:05Z","updated_at":"2026-01-10T03:04:05Z",` +
	`"web_url":"https://gitlab.example.com/p/-/pipelines/78",` +
	`"user":{"id":6,"username":"bob","public_email":"bob@example.com","name":"Bob",` +
	`"state":"active","locked":false,"avatar_url":"https://gitlab.example.com/b.png",` +
	`"web_url":"https://gitlab.example.com/bob"}}`

// wantGotTags is how Get publishes the tag pointing at the package
// packageGetJSON carries: in the shape and to the precision a version's tag is
// published in beside it, although GitLab wrote its times with fractions.
const wantGotTags = `[{"id":2,"package_id":10,"name":"latest","created_at":"2026-01-03T03:04:05Z",` +
	`"updated_at":"2026-01-04T03:04:05Z"}]`

// assertPublishedAs reports a value serializing to anything but want, which is
// the form a caller reads it in.
func assertPublishedAs(t *testing.T, what string, value any, want string) {
	t.Helper()
	got, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", what, err)
	}
	if string(got) != want {
		t.Errorf("%s published as\n%s\nwant\n%s", what, got, want)
	}
}

// assertVersionsPublishedAs reports versions serializing to anything but want.
func assertVersionsPublishedAs(t *testing.T, versions []toolutil.PackageVersionOutput, want string) {
	t.Helper()
	assertPublishedAs(t, "versions", versions, want)
}

// TestGet_PublishesThePackageWithItsOtherVersions verifies Get asks for the one
// package at its own route and publishes every field package.list does, the
// pipeline that last built the package completed exactly as each version's
// pipeline is, and the package's other versions, each version's pipeline
// completed with the keys client-go does not decode. Its tags, its versions'
// tags and every time in it share one shape and one precision. The owning
// project is not among the keys, since GitLab sends it only to a group's
// listing, and neither is pipelines, which GitLab sends as a constant empty
// list.
func TestGet_PublishesThePackageWithItsOtherVersions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != pathPackageGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, packageGetJSON)
	}))
	out, err := Get(t.Context(), client, GetInput{ProjectID: "42", PackageID: "10"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	p := out.Package
	if p.ID != 10 || p.Name != testPackageName || p.Version != "1.0.0" || p.PackageType != "conan" || p.Status != "default" {
		t.Errorf("package = %d %q %q %q %q, want 10 my-pkg 1.0.0 conan default", p.ID, p.Name, p.Version, p.PackageType, p.Status)
	}
	if p.CreatorID != 57 || p.ConanPackageName != "recipe-name" {
		t.Errorf("creator and recipe = %d / %q, want 57 / recipe-name", p.CreatorID, p.ConanPackageName)
	}
	assertPublishedAs(t, "pipeline", p.Pipeline, wantGotPipeline)
	assertPublishedAs(t, "tags", p.Tags, wantGotTags)
	assertVersionsPublishedAs(t, p.Versions, wantGotVersions)

	published, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var envelope struct {
		Package map[string]json.RawMessage `json:"package"`
	}
	if err = json.Unmarshal(published, &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"project_id", "project_path", "pipelines"} {
		t.Run(key, func(t *testing.T) {
			if _, ok := envelope.Package[key]; ok {
				t.Errorf("package publishes %s, which this route is never sent", key)
			}
		})
	}
}

// TestGet_VersionShapes verifies each shape an other version can arrive in:
// no version at all, a version whose tag list is empty and one GitLab sent
// without tags, both of which publish no tags key, as the package's own item
// does, a version sent without a time or a pipeline, a pipeline without the
// user who ran it, and a null entry, whose neighbor keeps the pipeline keys
// read at its own position rather than the null's.
func TestGet_VersionShapes(t *testing.T) {
	for _, tt := range []struct {
		name     string
		versions string
		want     string
	}{
		{name: "no other version", versions: `[]`, want: `[]`},
		{
			name:     "an empty tag list and no pipeline",
			versions: `[{"id":9,"version":"0.9.0","tags":[]}]`,
			want:     `[{"id":9,"version":"0.9.0","pipeline":null}]`,
		},
		{
			name:     "no tags sent",
			versions: `[{"id":9,"version":"0.9.0"}]`,
			want:     `[{"id":9,"version":"0.9.0","pipeline":null}]`,
		},
		{
			name: "a pipeline without its user",
			versions: `[{"id":9,"version":"0.9.0","tags":[],"pipeline":{"id":77,"iid":4,"project_id":42,` +
				`"sha":"abc123","ref":"main","status":"success","source":"push","web_url":"https://g/p/77","user":null}}]`,
			want: `[{"id":9,"version":"0.9.0","pipeline":{"id":77,"iid":4,"project_id":42,` +
				`"sha":"abc123","ref":"main","status":"success","source":"push",` +
				`"web_url":"https://g/p/77","user":null}}]`,
		},
		{
			name:     "a null version beside one",
			versions: `[null,{"id":9,"version":"0.9.0","tags":[],"pipeline":{"id":77,"iid":4,"project_id":42,"source":"push"}}]`,
			want: `[{"id":9,"version":"0.9.0","pipeline":{"id":77,"iid":4,"project_id":42,` +
				`"sha":"","ref":"","status":"","source":"push","web_url":"","user":null}}]`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"id":10,"name":"my-pkg","version":"1.0.0","versions":` + tt.versions + `}`
			out, err := Get(t.Context(), packagesClient(t, body), GetInput{ProjectID: "42", PackageID: "10"})
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			assertVersionsPublishedAs(t, out.Package.Versions, tt.want)
		})
	}
}

// TestGet_InvalidInput_RefusedBeforeAnyRequest verifies Get refuses a
// cancelled context, a missing project and a package_id that is not a
// positive integer without sending anything.
func TestGet_InvalidInput_RefusedBeforeAnyRequest(t *testing.T) {
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	for _, tt := range []struct {
		name  string
		ctx   context.Context
		input GetInput
		want  string
	}{
		{name: "cancelled context", ctx: cancelled, input: GetInput{ProjectID: "42", PackageID: "10"}, want: "context canceled"},
		{name: "no project", ctx: t.Context(), input: GetInput{PackageID: "10"}, want: "project_id is required"},
		{name: "no package", ctx: t.Context(), input: GetInput{ProjectID: "42"}, want: "package_id must be a positive integer"},
		{name: "zero package", ctx: t.Context(), input: GetInput{ProjectID: "42", PackageID: "0"}, want: "package_id must be a positive integer"},
		{name: "negative package", ctx: t.Context(), input: GetInput{ProjectID: "42", PackageID: "-3"}, want: "package_id must be a positive integer"},
		{name: "package name", ctx: t.Context(), input: GetInput{ProjectID: "42", PackageID: testPackageName}, want: "package_id must be a positive integer"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Get(tt.ctx, testutil.NewTestClient(t, testutil.ForbiddenHandler(t)), tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Get error = %v, want one mentioning %q", err, tt.want)
			}
		})
	}
}

// TestGet_GitLabRefusal_Wrapped verifies a 404 keeps its status for the route
// to turn into the not-found result and carries the hint naming both of
// GitLab's reasons, and any other refusal is reported with its own status. It
// asks for another
// package than the other tests do, and holds the path to it, so a handler that
// sent one fixed id would be seen here and in the request inventory alike.
func TestGet_GitLabRefusal_Wrapped(t *testing.T) {
	for _, tt := range []struct {
		name     string
		status   int
		wantHint bool
	}{
		{name: "not found", status: http.StatusNotFound, wantHint: true},
		{name: "forbidden", status: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v4/projects/42/packages/11" {
					t.Errorf("request path = %q, want package 11's", r.URL.Path)
				}
				testutil.RespondJSON(w, tt.status, `{"message":"refused"}`)
			}))
			_, err := Get(t.Context(), client, GetInput{ProjectID: "42", PackageID: "11"})
			if !toolutil.IsHTTPStatus(err, tt.status) {
				t.Fatalf("Get error = %v, want status %d", err, tt.status)
			}
			if got := strings.Contains(err.Error(), packageNotFoundHint); got != tt.wantHint {
				t.Errorf("error %q carries the not-found hint: %t, want %t", err, got, tt.wantHint)
			}
		})
	}
}

// TestGet_UnreadablePipelineKey_RefusedByTheCapture verifies Get refuses an
// answer whose version pipeline carries an iid that is not a number. client-go
// does not decode iid, so only the read beside it can notice.
func TestGet_UnreadablePipelineKey_RefusedByTheCapture(t *testing.T) {
	const poisoned = `{"id":10,"versions":[{"id":9,"pipeline":{"id":77,"iid":"four"}}]}`
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{{Name: "get", Call: func() error {
		_, err := Get(t.Context(), packagesClient(t, poisoned), GetInput{ProjectID: "42", PackageID: "10"})
		return err
	}}})
}

// TestPackagePipelineToOutput_WithoutTheCapturedKeys verifies a pipeline, and a
// user, the capture read nothing beside publish what client-go decoded and
// leave the rest at zero rather than failing. Every handler always has both
// halves, since they decode the same bytes; this holds the converters to not
// depending on it.
func TestPackagePipelineToOutput_WithoutTheCapturedKeys(t *testing.T) {
	got := packagePipelineToOutput(&gl.PackagePipeline{
		ID: 77, SHA: "abc123", User: &gl.BasicUser{ID: 5, Username: "alice"},
	}, nil)
	if got.ID != 77 || got.SHA != "abc123" || got.IID != 0 || got.Source != "" {
		t.Errorf("pipeline = %+v, want the decoded keys and no captured ones", got)
	}
	if got.User == nil || got.User.ID != 5 || got.User.Username != "alice" || got.User.Locked || got.User.PublicEmail != "" {
		t.Errorf("user = %+v, want the decoded keys and no captured ones", got.User)
	}
	user := packagePipelineUserToOutput(&gl.BasicUser{ID: 6}, &toolutil.UserBasicExtra{Locked: true, PublicEmail: "bob@example.com"})
	if user.ID != 6 || !user.Locked || user.PublicEmail != "bob@example.com" {
		t.Errorf("user = %+v, want 6, locked, bob@example.com", user)
	}
}

// TestExtraAt_ReadsOnlyWhatTheCaptureHolds verifies the positional read hands
// back the extra at a position the capture holds, the element itself rather
// than a copy, and nil at the first position past the end and for no capture
// at all, which is what lets a conversion made without a capture still run.
func TestExtraAt_ReadsOnlyWhatTheCaptureHolds(t *testing.T) {
	extras := []toolutil.PackagePipelineExtra{{IID: 3}, {IID: 4}}
	if got := extraAt(extras, 1); got != &extras[1] {
		t.Errorf("extraAt(extras, 1) = %p, want the second element %p", got, &extras[1])
	}
	if got := extraAt(extras, 2); got != nil {
		t.Errorf("extraAt(extras, 2) = %+v, want nil past the end", got)
	}
	if got := extraAt[toolutil.PackagePipelineExtra](nil, 0); got != nil {
		t.Errorf("extraAt(nil, 0) = %+v, want nil", got)
	}
}

// TestPackageConversions_WithoutACapture verifies a package's own pipeline
// and its other versions convert without a capture, each publishing what
// client-go decoded and no captured key, rather than dereferencing or indexing
// past the extras they were not given.
func TestPackageConversions_WithoutACapture(t *testing.T) {
	pipeline := &gl.PackagePipeline{ID: 62, Status: "failed"}
	item := packageToDetailItem(&gl.Package{ID: 10, Pipeline: pipeline, Versions: []*gl.PackageVersion{{ID: 9}}}, toolutil.PackageExtra{})
	if item.Pipeline == nil || item.Pipeline.ID != 62 || item.Pipeline.IID != 0 {
		t.Errorf("pipeline = %+v, want 62 without a captured iid", item.Pipeline)
	}
	if len(item.Versions) != 1 || item.Versions[0].ID != 9 {
		t.Errorf("versions = %+v, want 9", item.Versions)
	}
	versions := packageVersionsToOutput([]*gl.PackageVersion{{ID: 9, Pipeline: pipeline}}, nil)
	if len(versions) != 1 || versions[0].Pipeline == nil || versions[0].Pipeline.ID != 62 || versions[0].Pipeline.IID != 0 {
		t.Errorf("versions = %+v, want 9 built by 62 without a captured iid", versions)
	}
}
