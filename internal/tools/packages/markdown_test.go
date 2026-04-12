// markdown_test.go contains unit tests for the Markdown formatting functions
// in the packages package.
package packages

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestFormatPublishDirMarkdown_WithPublishedFiles verifies that
// [FormatPublishDirMarkdown] renders a table of published files with SHA256
// truncation when hashes exceed 12 characters.
func TestFormatPublishDirMarkdown_WithPublishedFiles(t *testing.T) {
	out := PublishDirOutput{
		TotalFiles: 2,
		TotalBytes: 1024,
		Published: []PublishDirItem{
			{FileName: "file1.txt", Size: 512, SHA256: "abcdef1234567890abcdef"},
			{FileName: "file2.txt", Size: 512, SHA256: "short"},
		},
	}
	got := FormatPublishDirMarkdown(out)
	if !strings.Contains(got, "## Directory Published") {
		t.Error("missing header")
	}
	if !strings.Contains(got, "| file1.txt | 512 |") {
		t.Error("missing file1.txt row")
	}
	if !strings.Contains(got, "abcdef123456…") {
		t.Error("SHA256 should be truncated to 12 chars + ellipsis")
	}
	if !strings.Contains(got, "| file2.txt | 512 | short |") {
		t.Error("short SHA256 should not be truncated")
	}
}

// TestFormatPublishDirMarkdown_WithErrors verifies that
// [FormatPublishDirMarkdown] includes error entries in the output.
func TestFormatPublishDirMarkdown_WithErrors(t *testing.T) {
	out := PublishDirOutput{
		TotalFiles: 1,
		TotalBytes: 100,
		Errors:     []string{"upload failed: timeout", "checksum mismatch"},
	}
	got := FormatPublishDirMarkdown(out)
	if !strings.Contains(got, "### Errors (2)") {
		t.Error("missing Errors section")
	}
	if !strings.Contains(got, "- upload failed: timeout") {
		t.Error("missing first error")
	}
}

// TestFormatPublishDirMarkdown_Empty verifies that [FormatPublishDirMarkdown]
// handles zero files and no errors gracefully.
func TestFormatPublishDirMarkdown_Empty(t *testing.T) {
	out := PublishDirOutput{}
	got := FormatPublishDirMarkdown(out)
	if !strings.Contains(got, "**Total Files**: 0") {
		t.Error("missing total files")
	}
	if strings.Contains(got, "| File |") {
		t.Error("should not contain table when no files published")
	}
}

// TestFormatListMarkdown_EmptyPackages verifies that [FormatListMarkdown]
// renders "No packages found." when the list is empty.
func TestFormatListMarkdown_EmptyPackages(t *testing.T) {
	out := ListOutput{
		Packages:   nil,
		Pagination: toolutil.PaginationOutput{TotalItems: 0},
	}
	got := FormatListMarkdown(out)
	if !strings.Contains(got, "No packages found.") {
		t.Error("missing 'No packages found.' message")
	}
}

// TestFormatFileListMarkdown_EmptyFiles verifies that [FormatFileListMarkdown]
// renders "No package files found." when the list is empty.
func TestFormatFileListMarkdown_EmptyFiles(t *testing.T) {
	out := FileListOutput{
		Files:      nil,
		Pagination: toolutil.PaginationOutput{TotalItems: 0},
	}
	got := FormatFileListMarkdown(out)
	if !strings.Contains(got, "No package files found.") {
		t.Error("missing 'No package files found.' message")
	}
}

// TestFormatFileListMarkdown_LongSHA verifies that [FormatFileListMarkdown]
// truncates SHA256 values longer than 12 characters.
func TestFormatFileListMarkdown_LongSHA(t *testing.T) {
	out := FileListOutput{
		Files: []FileListItem{
			{PackageFileID: 1, FileName: "pkg.tar.gz", Size: 2048, SHA256: "0123456789abcdef01234567"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	got := FormatFileListMarkdown(out)
	if !strings.Contains(got, "0123456789ab…") {
		t.Error("SHA256 should be truncated to 12 chars + ellipsis")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errNoReachAPI = "should not reach API"

const errExpectedAPI = "expected API error, got nil"

const fmtUnexpErr = "unexpected error: %v"

const (
	pathPutPkg1         = "PUT /api/v4/projects/1/packages/generic/my-pkg/1.0.0/app.tar.gz"
	hdrContentType      = "Content-Type"
	mimeOctetStream     = "application/octet-stream"
	fmtExpPkgVersionErr = "expected package_version error, got: %v"
	pathTmpOutBin       = "/tmp/out.bin"
	fmtExpProjectIDErr  = "expected project_id error, got: %v"
	testCtxCancelled    = "context canceled"
	fmtExpCtxCancelErr  = "expected context canceled error, got: %v"
	pathAPIPkgs1        = "/api/v4/projects/1/packages"
	testFileDataBin     = "data.bin"
	testPkgTestPkg      = "test-pkg"
	testFileAppBin      = "app.bin"
	testFileOutBin      = "out.bin"
	fmtExpCtxCancelGot  = "expected context canceled, got: %v"
	fmtCallToolErr      = "CallTool error: %v"
	msgCallToolIsError  = "CallTool returned IsError=true"
)

// ---------------------------------------------------------------------------
// RegisterTools / RegisterMeta — no-panic smoke tests
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip for all registered tools
// ---------------------------------------------------------------------------.

// newPackagesMCPSession is an internal helper for the packages package.
func newPackagesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()

	// Publish (PUT)
	handler.HandleFunc(pathPutPkg1, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 1, "package_id": 10, "file_name": "app.tar.gz",
			"size": 1024, "file_sha256": "abc", "file_md5": "md5",
			"file_sha1": "sha1", "file_store": 1,
			"created_at": "2026-01-01T00:00:00Z"
		}`)
	})

	// Download (GET)
	handler.HandleFunc("GET /api/v4/projects/1/packages/generic/my-pkg/1.0.0/app.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(hdrContentType, mimeOctetStream)
		w.Write([]byte("file-data"))
	})

	// List packages
	handler.HandleFunc("GET /api/v4/projects/1/packages", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"generic","status":"default"}]`)
	})

	// List package files
	handler.HandleFunc("GET /api/v4/projects/1/packages/10/package_files", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":20,"package_id":10,"file_name":"app.tar.gz","size":1024,"file_sha256":"abc"}]`)
	})

	// Delete package
	handler.HandleFunc("DELETE /api/v4/projects/1/packages/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Delete package file
	handler.HandleFunc("DELETE /api/v4/projects/1/packages/10/package_files/20", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Release link for publish_and_link
	handler.HandleFunc("POST /api/v4/projects/1/releases/v1.0.0/assets/links", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 50, "name": "app.tar.gz",
			"url": "https://example.com/pkg", "link_type": "package", "external": true
		}`)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// assertCallToolSuccess validates that a CallTool invocation succeeded without errors.
func assertCallToolSuccess(t *testing.T, result *mcp.CallToolResult, err error, toolName string) {
	t.Helper()
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", toolName, err)
	}
	if result.IsError {
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				t.Fatalf("CallTool(%s) returned error: %s", toolName, tc.Text)
			}
		}
		t.Fatalf("CallTool(%s) returned IsError=true", toolName)
	}
}

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newPackagesMCPSession(t)
	ctx := context.Background()

	content64 := base64.StdEncoding.EncodeToString([]byte("test-data"))
	outDir := t.TempDir()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"publish", "gitlab_package_publish", map[string]any{
			"project_id": "1", "package_name": testPackageName, "package_version": "1.0.0",
			"file_name": testFileName, "content_base64": content64,
		}},
		{"download", "gitlab_package_download", map[string]any{
			"project_id": "1", "package_name": testPackageName, "package_version": "1.0.0",
			"file_name": testFileName, "output_path": filepath.Join(outDir, "dl.bin"),
		}},
		{"list", "gitlab_package_list", map[string]any{"project_id": "1"}},
		{"file_list", "gitlab_package_file_list", map[string]any{"project_id": "1", "package_id": "10"}},
		{"delete", "gitlab_package_delete", map[string]any{"project_id": "1", "package_id": "10"}},
		{"file_delete", "gitlab_package_file_delete", map[string]any{"project_id": "1", "package_id": "10", "package_file_id": "20"}},
		{"publish_and_link", "gitlab_package_publish_and_link", map[string]any{
			"project_id": "1", "package_name": testPackageName, "package_version": "1.0.0",
			"file_name": testFileName, "content_base64": content64, "tag_name": "v1.0.0",
		}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			assertCallToolSuccess(t, result, err, tt.tool)
		})
	}
}

// ---------------------------------------------------------------------------
// Publish — missing package_version
// ---------------------------------------------------------------------------.

// TestPublish_MissingVersion verifies the behavior of publish missing version.
func TestPublish_MissingVersion(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:     "42",
		PackageName:   testPackageName,
		FileName:      testFileName,
		ContentBase64: testBase64Content,
	})
	if err == nil || !strings.Contains(err.Error(), "package_version") {
		t.Fatalf(fmtExpPkgVersionErr, err)
	}
}

// ---------------------------------------------------------------------------
// Publish — invalid file name (starts with ~)
// ---------------------------------------------------------------------------.

// TestPublish_InvalidFileName verifies the behavior of publish invalid file name.
func TestPublish_InvalidFileName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       "~badname.tar.gz",
		ContentBase64:  testBase64Content,
	})
	if err == nil {
		t.Fatal("expected error for invalid file name")
	}
}

// ---------------------------------------------------------------------------
// Publish — invalid base64 content
// ---------------------------------------------------------------------------.

// TestPublish_InvalidBase64 verifies the behavior of publish invalid base64.
func TestPublish_InvalidBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  "!!!not-base64!!!",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid base64") {
		t.Fatalf("expected invalid base64 error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Download — missing required fields
// ---------------------------------------------------------------------------.

// TestDownload_MissingProjectID verifies the behavior of download missing project i d.
func TestDownload_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Download(context.Background(), nil, client, DownloadInput{
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		OutputPath:     pathTmpOutBin,
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// TestDownload_MissingPackageName verifies the behavior of download missing package name.
func TestDownload_MissingPackageName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		OutputPath:     pathTmpOutBin,
	})
	if err == nil || !strings.Contains(err.Error(), "package_name") {
		t.Fatalf("expected package_name error, got: %v", err)
	}
}

// TestDownload_MissingVersion verifies the behavior of download missing version.
func TestDownload_MissingVersion(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:   "42",
		PackageName: testPackageName,
		FileName:    testFileName,
		OutputPath:  pathTmpOutBin,
	})
	if err == nil || !strings.Contains(err.Error(), "package_version") {
		t.Fatalf(fmtExpPkgVersionErr, err)
	}
}

// TestDownload_MissingFileName verifies the behavior of download missing file name.
func TestDownload_MissingFileName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		OutputPath:     pathTmpOutBin,
	})
	if err == nil || !strings.Contains(err.Error(), "file_name") {
		t.Fatalf("expected file_name error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// List — API error, context canceled, with sort/order_by/version filters
// ---------------------------------------------------------------------------.

// TestList_APIError verifies the behavior of list a p i error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestList_ContextCancelled verifies the behavior of list context cancelled.
func TestList_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := List(ctx, client, ListInput{ProjectID: "1"})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelErr, err)
	}
}

// TestList_WithSortAndOrderBy verifies the behavior of list with sort and order by.
func TestList_WithSortAndOrderBy(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathAPIPkgs1 {
			q := r.URL.Query()
			if q.Get("order_by") != "created_at" {
				t.Errorf("expected order_by=created_at, got %q", q.Get("order_by"))
			}
			if q.Get("sort") != "desc" {
				t.Errorf("expected sort=desc, got %q", q.Get("sort"))
			}
			if q.Get("package_version") != "2.0.0" {
				t.Errorf("expected package_version=2.0.0, got %q", q.Get("package_version"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))
	_, err := List(context.Background(), client, ListInput{
		ProjectID:      "1",
		OrderBy:        "created_at",
		Sort:           "desc",
		PackageVersion: "2.0.0",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// Empty list with no tags and no links.
func TestList_WithEmptyPackage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathAPIPkgs1 {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"pkg","version":"1.0.0","package_type":"generic","status":"default"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(out.Packages))
	}
	if out.Packages[0].CreatedAt != "" {
		t.Errorf("CreatedAt should be empty when nil, got %q", out.Packages[0].CreatedAt)
	}
	if out.Packages[0].WebPath != "" {
		t.Errorf("WebPath should be empty when Links is nil, got %q", out.Packages[0].WebPath)
	}
	if len(out.Packages[0].Tags) != 0 {
		t.Errorf("Tags should be empty, got %v", out.Packages[0].Tags)
	}
}

// ---------------------------------------------------------------------------
// FileList — API error, context canceled, missing project_id
// ---------------------------------------------------------------------------.

// TestFileList_APIError verifies the behavior of file list a p i error.
func TestFileList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := FileList(context.Background(), client, FileListInput{ProjectID: "1", PackageID: "10"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestFileList_ContextCancelled verifies the behavior of file list context cancelled.
func TestFileList_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := FileList(ctx, client, FileListInput{ProjectID: "1", PackageID: "10"})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelErr, err)
	}
}

// TestFileList_MissingProjectID verifies the behavior of file list missing project i d.
func TestFileList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := FileList(context.Background(), client, FileListInput{PackageID: "10"})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// FileList with created_at in response.
func TestFileList_WithCreatedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/packages/10/package_files" {
			testutil.RespondJSON(w, http.StatusOK, `[{
				"id":20,"package_id":10,"file_name":"app.bin","size":100,
				"file_sha256":"hash","created_at":"2026-06-01T10:00:00Z"
			}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := FileList(context.Background(), client, FileListInput{ProjectID: "1", PackageID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(out.Files))
	}
	if out.Files[0].CreatedAt == "" {
		t.Error("CreatedAt should not be empty")
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, context canceled
// ---------------------------------------------------------------------------.

// TestDelete_APIError verifies the behavior of delete a p i error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	err := Delete(context.Background(), nil, client, DeleteInput{ProjectID: "1", PackageID: "10"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_ContextCancelled verifies the behavior of delete context cancelled.
func TestDelete_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := Delete(ctx, nil, client, DeleteInput{ProjectID: "1", PackageID: "10"})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelErr, err)
	}
}

// ---------------------------------------------------------------------------
// FileDelete — API error, context canceled, missing project_id, missing package_id
// ---------------------------------------------------------------------------.

// TestFileDelete_APIError verifies the behavior of file delete a p i error.
func TestFileDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	err := FileDelete(context.Background(), nil, client, FileDeleteInput{ProjectID: "1", PackageID: "10", PackageFileID: "20"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestFileDelete_ContextCancelled verifies the behavior of file delete context cancelled.
func TestFileDelete_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := FileDelete(ctx, nil, client, FileDeleteInput{ProjectID: "1", PackageID: "10", PackageFileID: "20"})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelErr, err)
	}
}

// TestFileDelete_MissingProjectID verifies the behavior of file delete missing project i d.
func TestFileDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := FileDelete(context.Background(), nil, client, FileDeleteInput{PackageID: "10", PackageFileID: "20"})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// TestFileDelete_MissingPackageID verifies the behavior of file delete missing package i d.
func TestFileDelete_MissingPackageID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := FileDelete(context.Background(), nil, client, FileDeleteInput{ProjectID: "1", PackageFileID: "20"})
	if err == nil || !strings.Contains(err.Error(), "package_id") {
		t.Fatalf("expected package_id error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// PublishDirectory — missing project_id, invalid package name, nonexistent dir
// ---------------------------------------------------------------------------.

// TestPublishDirectory_MissingProjectID verifies the behavior of publish directory missing project i d.
func TestPublishDirectory_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		DirectoryPath:  t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// TestPublishDirectory_InvalidPackageName verifies the behavior of publish directory invalid package name.
func TestPublishDirectory_InvalidPackageName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "1",
		PackageName:    ".invalid",
		PackageVersion: "1.0.0",
		DirectoryPath:  t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for invalid package name")
	}
}

// TestPublishDirectory_MissingVersion verifies the behavior of publish directory missing version.
func TestPublishDirectory_MissingVersion(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:     "1",
		PackageName:   testPackageName,
		DirectoryPath: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "package_version") {
		t.Fatalf(fmtExpPkgVersionErr, err)
	}
}

// TestPublishDirectory_NonexistentDir verifies the behavior of publish directory nonexistent dir.
func TestPublishDirectory_NonexistentDir(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "1",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		DirectoryPath:  filepath.Join(t.TempDir(), "nonexistent"),
	})
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

// ---------------------------------------------------------------------------
// streamDownloadPackageFile — context canceled
// ---------------------------------------------------------------------------.

// TestStreamDownload_ContextCancelled verifies the behavior of stream download context cancelled.
func TestStreamDownload_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, _, err := streamDownloadPackageFile(ctx, nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileAppBin,
		OutputPath:     filepath.Join(t.TempDir(), testFileOutBin),
	})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelGot, err)
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip via meta-tool
// ---------------------------------------------------------------------------.

// newPackagesMetaMCPSession is an internal helper for the packages package.
func newPackagesMetaMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/1/packages", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"generic","status":"default"}]`)
	})

	handler.HandleFunc("GET /api/v4/projects/1/packages/10/package_files", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":20,"package_id":10,"file_name":"app.tar.gz","size":1024,"file_sha256":"abc"}]`)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// TestMetaTool_ListAction verifies the behavior of meta tool list action.
func TestMetaTool_ListAction(t *testing.T) {
	session := newPackagesMetaMCPSession(t)
	ctx := context.Background()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_package",
		Arguments: map[string]any{
			"action": "list",
			"params": map[string]any{"project_id": "1"},
		},
	})
	if err != nil {
		t.Fatalf(fmtCallToolErr, err)
	}
	if result.IsError {
		t.Fatal(msgCallToolIsError)
	}
}

// TestMetaTool_FileListAction verifies the behavior of meta tool file list action.
func TestMetaTool_FileListAction(t *testing.T) {
	session := newPackagesMetaMCPSession(t)
	ctx := context.Background()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_package",
		Arguments: map[string]any{
			"action": "file_list",
			"params": map[string]any{"project_id": "1", "package_id": "10"},
		},
	})
	if err != nil {
		t.Fatalf(fmtCallToolErr, err)
	}
	if result.IsError {
		t.Fatal(msgCallToolIsError)
	}
}

// ---------------------------------------------------------------------------
// Additional meta-tool actions to increase RegisterMeta coverage
// ---------------------------------------------------------------------------.

// TestMetaTool_PublishAction verifies the behavior of meta tool publish action.
func TestMetaTool_PublishAction(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc(pathPutPkg1, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 1, "package_id": 10, "file_name": "app.tar.gz",
			"size": 1024, "file_sha256": "abc", "file_md5": "md5",
			"file_sha1": "sha1", "file_store": 1,
			"created_at": "2026-01-01T00:00:00Z"
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_package",
		Arguments: map[string]any{
			"action": "publish",
			"params": map[string]any{
				"project_id":      "1",
				"package_name":    testPackageName,
				"package_version": "1.0.0",
				"file_name":       testFileName,
				"content_base64":  base64.StdEncoding.EncodeToString([]byte("test")),
			},
		},
	})
	if err != nil {
		t.Fatalf(fmtCallToolErr, err)
	}
	if result.IsError {
		t.Fatal(msgCallToolIsError)
	}
}

// TestMetaTool_DeleteAction verifies the behavior of meta tool delete action.
func TestMetaTool_DeleteAction(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("DELETE /api/v4/projects/1/packages/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_package",
		Arguments: map[string]any{
			"action": "delete",
			"params": map[string]any{"project_id": "1", "package_id": "10"},
		},
	})
	if err != nil {
		t.Fatalf(fmtCallToolErr, err)
	}
	if result.IsError {
		t.Fatal(msgCallToolIsError)
	}
}

// TestMetaTool_FileDeleteAction verifies the behavior of meta tool file delete action.
func TestMetaTool_FileDeleteAction(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("DELETE /api/v4/projects/1/packages/10/package_files/20", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_package",
		Arguments: map[string]any{
			"action": "file_delete",
			"params": map[string]any{"project_id": "1", "package_id": "10", "package_file_id": "20"},
		},
	})
	if err != nil {
		t.Fatalf(fmtCallToolErr, err)
	}
	if result.IsError {
		t.Fatal(msgCallToolIsError)
	}
}

// TestMetaTool_DownloadAction verifies the behavior of meta tool download action.
func TestMetaTool_DownloadAction(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/1/packages/generic/my-pkg/1.0.0/app.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(hdrContentType, mimeOctetStream)
		w.Write([]byte("file-content"))
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_package",
		Arguments: map[string]any{
			"action": "download",
			"params": map[string]any{
				"project_id":      "1",
				"package_name":    testPackageName,
				"package_version": "1.0.0",
				"file_name":       testFileName,
				"output_path":     filepath.Join(t.TempDir(), "dl.bin"),
			},
		},
	})
	if err != nil {
		t.Fatalf(fmtCallToolErr, err)
	}
	if result.IsError {
		t.Fatal(msgCallToolIsError)
	}
}

// TestMetaTool_PublishAndLinkAction verifies the behavior of meta tool publish and link action.
func TestMetaTool_PublishAndLinkAction(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc(pathPutPkg1, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 1, "package_id": 10, "file_name": "app.tar.gz",
			"size": 1024, "file_sha256": "abc"
		}`)
	})
	handler.HandleFunc("POST /api/v4/projects/1/releases/v1.0.0/assets/links", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 50, "name": "app.tar.gz", "url": "https://example.com/pkg",
			"link_type": "package", "external": true
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_package",
		Arguments: map[string]any{
			"action": "publish_and_link",
			"params": map[string]any{
				"project_id":      "1",
				"package_name":    testPackageName,
				"package_version": "1.0.0",
				"file_name":       testFileName,
				"content_base64":  base64.StdEncoding.EncodeToString([]byte("test")),
				"tag_name":        "v1.0.0",
			},
		},
	})
	if err != nil {
		t.Fatalf(fmtCallToolErr, err)
	}
	if result.IsError {
		t.Fatal(msgCallToolIsError)
	}
}

// TestMetaTool_PublishDirectoryAction verifies the behavior of meta tool publish directory action.
func TestMetaTool_PublishDirectoryAction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, testFileName), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := http.NewServeMux()
	handler.HandleFunc(pathPutPkg1, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 1, "package_id": 10, "file_name": "app.tar.gz",
			"size": 4, "file_sha256": "abc"
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_package",
		Arguments: map[string]any{
			"action": "publish_directory",
			"params": map[string]any{
				"project_id":      "1",
				"package_name":    testPackageName,
				"package_version": "1.0.0",
				"directory_path":  dir,
			},
		},
	})
	if err != nil {
		t.Fatalf(fmtCallToolErr, err)
	}
	if result.IsError {
		t.Fatal(msgCallToolIsError)
	}
}

// TestMetaTool_InvalidAction verifies the behavior of meta tool invalid action.
func TestMetaTool_InvalidAction(t *testing.T) {
	session := newPackagesMetaMCPSession(t)
	ctx := context.Background()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_package",
		Arguments: map[string]any{
			"action": "nonexistent",
			"params": map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf(fmtCallToolErr, err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for unknown action")
	}
}

// ---------------------------------------------------------------------------
// streamDownloadPackageFile — successful download
// ---------------------------------------------------------------------------.

// TestStreamDownload_Success verifies the behavior of stream download success.
func TestStreamDownload_Success(t *testing.T) {
	fileData := []byte("streaming-download-content")
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(hdrContentType, mimeOctetStream)
		w.Write(fileData)
	}))

	outputPath := filepath.Join(t.TempDir(), testFileOutBin)
	size, checksum, err := streamDownloadPackageFile(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileAppBin,
		OutputPath:     outputPath,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if size != int64(len(fileData)) {
		t.Errorf("expected size %d, got %d", len(fileData), size)
	}
	if checksum == "" {
		t.Error("expected non-empty checksum")
	}
	data, _ := os.ReadFile(outputPath)
	if string(data) != string(fileData) {
		t.Errorf("file content mismatch")
	}
}

// ---------------------------------------------------------------------------
// streamDownloadPackageFile — API error on Do()
// ---------------------------------------------------------------------------.

// TestStreamDownload_APIError verifies the behavior of stream download a p i error.
func TestStreamDownload_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))

	outputPath := filepath.Join(t.TempDir(), testFileOutBin)
	_, _, err := streamDownloadPackageFile(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileAppBin,
		OutputPath:     outputPath,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Publish — both file_path and content_base64
// ---------------------------------------------------------------------------.

// TestPublish_BothFileAndBase64 verifies the behavior of publish both file and base64.
func TestPublish_BothFileAndBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		FilePath:       "/tmp/file.bin",
		ContentBase64:  testBase64Content,
	})
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("expected 'not both' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Publish — neither file_path nor content_base64
// ---------------------------------------------------------------------------.

// TestPublish_NeitherFileNorBase64 verifies the behavior of publish neither file nor base64.
func TestPublish_NeitherFileNorBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
	})
	if err == nil || !strings.Contains(err.Error(), "either file_path or content_base64") {
		t.Fatalf("expected 'either' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Publish — API error on publish call
// ---------------------------------------------------------------------------.

// TestPublish_APIError verifies the behavior of publish a p i error.
func TestPublish_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  base64.StdEncoding.EncodeToString([]byte("test")),
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Publish — context canceled
// ---------------------------------------------------------------------------.

// TestPublish_ContextCancelled verifies the behavior of publish context cancelled.
func TestPublish_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Publish(ctx, nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  testBase64Content,
	})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelGot, err)
	}
}

// ---------------------------------------------------------------------------
// Publish — file_path with small file
// ---------------------------------------------------------------------------.

// TestPublish_FilePathSmallFile verifies the behavior of publish file path small file.
func TestPublish_FilePathSmallFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "small.bin")
	if err := os.WriteFile(tmpFile, []byte("small-data"), 0644); err != nil {
		t.Fatal(err)
	}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 1, "package_id": 10, "file_name": "small.bin",
				"size": 10, "file_sha256": "abc", "file_md5": "md5",
				"file_sha1": "sha1", "file_store": 1,
				"created_at": "2026-01-01T00:00:00Z",
				"updated_at": "2026-01-02T00:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       "small.bin",
		FilePath:       tmpFile,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.PackageFileID != 1 {
		t.Errorf("expected PackageFileID=1, got %d", out.PackageFileID)
	}
	if out.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}
}

// ---------------------------------------------------------------------------
// Publish — invalid package name
// ---------------------------------------------------------------------------.

// TestPublish_InvalidPackageName verifies the behavior of publish invalid package name.
func TestPublish_InvalidPackageName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

// ---------------------------------------------------------------------------
// Publish — missing project_id
// ---------------------------------------------------------------------------.

// TestPublish_MissingProjectID verifies the behavior of publish missing project i d.
func TestPublish_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		ContentBase64:  testBase64Content,
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// ---------------------------------------------------------------------------
// List — with package_name and package_type filter
// ---------------------------------------------------------------------------.

// TestList_WithNameAndTypeFilter verifies the behavior of list with name and type filter.
func TestList_WithNameAndTypeFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathAPIPkgs1 {
			q := r.URL.Query()
			if q.Get("package_name") != testPackageName {
				t.Errorf("expected package_name=my-pkg, got %q", q.Get("package_name"))
			}
			if q.Get("package_type") != "generic" {
				t.Errorf("expected package_type=generic, got %q", q.Get("package_type"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{
				"id":10,"name":"my-pkg","version":"1.0.0","package_type":"generic","status":"default",
				"_links": {"web_path": "/packages/10"},
				"tags": [{"name": "latest"}],
				"created_at": "2026-01-01T00:00:00Z"
			}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{
		ProjectID:   "1",
		PackageName: testPackageName,
		PackageType: "generic",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(out.Packages))
	}
	if out.Packages[0].WebPath == "" {
		t.Error("expected non-empty WebPath")
	}
	if len(out.Packages[0].Tags) != 1 {
		t.Errorf("expected 1 tag, got %d", len(out.Packages[0].Tags))
	}
}

// ---------------------------------------------------------------------------
// PublishDirectory — empty dir (no matching files)
// ---------------------------------------------------------------------------.

// TestPublishDirectory_EmptyDir verifies the behavior of publish directory empty dir.
func TestPublishDirectory_EmptyDir(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	dir := t.TempDir()
	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "1",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		DirectoryPath:  dir,
	})
	if err == nil || !strings.Contains(err.Error(), "no matching files") {
		t.Fatalf("expected 'no matching files' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Publish — file_path with nonexistent file
// ---------------------------------------------------------------------------.

// TestPublish_FilePathNonexistent verifies the behavior of publish file path nonexistent.
func TestPublish_FilePathNonexistent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := Publish(context.Background(), nil, client, PublishInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: "1.0.0",
		FileName:       testFileName,
		FilePath:       filepath.Join(t.TempDir(), "nonexistent.bin"),
	})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
