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

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestFormatPublishMarkdown_WithChecksumAndURL verifies publish markdown includes
// file identifiers, checksum, URL, and follow-up hints for common workflows.
func TestFormatPublishMarkdown_WithChecksumAndURL(t *testing.T) {
	out := PublishOutput{
		PackageFileID: 10,
		PackageID:     20,
		FileName:      "app.tar.gz",
		Size:          4096,
		SHA256:        "0123456789abcdef",
		URL:           "https://gitlab.example.com/api/v4/projects/1/packages/generic/pkg/1.0.0/app.tar.gz",
	}

	got := FormatPublishMarkdown(out)
	for _, want := range []string{
		"## Package Published",
		"**Package File ID**: 10",
		"**Package ID**: 20",
		"**File Name**: app.tar.gz",
		"**Size**: 4096 bytes",
		"**SHA256**: 0123456789abcdef",
		"**URL**: [https://gitlab.example.com/api/v4/projects/1/packages/generic/pkg/1.0.0/app.tar.gz](https://gitlab.example.com/api/v4/projects/1/packages/generic/pkg/1.0.0/app.tar.gz)",
		"publish_and_link",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatPublishMarkdown() = %q, want %q", got, want)
		}
	}
}

// TestFormatDownloadMarkdown_WithChecksum verifies download markdown reports the
// local output path, byte count, checksum, and next actions.
func TestFormatDownloadMarkdown_WithChecksum(t *testing.T) {
	out := DownloadOutput{OutputPath: "/tmp/app.tar.gz", Size: 2048, SHA256: "abcdef"}

	got := FormatDownloadMarkdown(out)
	for _, want := range []string{
		"## Package Downloaded",
		"**Output Path**: /tmp/app.tar.gz",
		"**Size**: 2048 bytes",
		"**SHA256**: abcdef",
		"file_list",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatDownloadMarkdown() = %q, want %q", got, want)
		}
	}
}

// TestFormatPublishAndLinkMarkdown_RendersBothSections verifies the composite
// formatter keeps package and release-link details visible after a successful workflow.
func TestFormatPublishAndLinkMarkdown_RendersBothSections(t *testing.T) {
	out := PublishAndLinkOutput{
		Package: PublishOutput{
			PackageFileID: 11,
			FileName:      "app.tar.gz",
			Size:          1024,
			URL:           "https://gitlab.example.com/package/app.tar.gz",
		},
		ReleaseLink: releaselinks.Output{ID: 22, Name: "app.tar.gz", URL: "https://gitlab.example.com/release/app.tar.gz"},
	}

	got := FormatPublishAndLinkMarkdown(out)
	for _, want := range []string{
		"## Package Published & Linked",
		"### Package",
		"**Package File ID**: 11",
		"**File Name**: app.tar.gz",
		"**URL**: [https://gitlab.example.com/package/app.tar.gz](https://gitlab.example.com/package/app.tar.gz)",
		"### Release Link",
		"**ID**: 22",
		"**Name**: app.tar.gz",
		"**URL**: [https://gitlab.example.com/release/app.tar.gz](https://gitlab.example.com/release/app.tar.gz)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatPublishAndLinkMarkdown() = %q, want %q", got, want)
		}
	}
}

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

// TestFormatListMarkdown_IncludesPreserveLinksHint verifies that package list
// markdown links pipeline summaries and emits the preserve-links hint.
func TestFormatListMarkdown_IncludesPreserveLinksHint(t *testing.T) {
	out := ListOutput{
		Packages: []ListItem{{
			ID:      1,
			Name:    "pkg",
			Version: "1.0.0",
			Pipeline: &PipelineItem{
				ID:     7,
				Status: "success",
				Ref:    "main",
				WebURL: "https://gitlab.example.com/project/-/pipelines/7",
			},
		}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	got := FormatListMarkdown(out)
	if !strings.Contains(got, "[7 success main](https://gitlab.example.com/project/-/pipelines/7)") {
		t.Fatalf("FormatListMarkdown() = %q, want linked pipeline", got)
	}
	if !strings.Contains(got, toolutil.HintPreserveLinks) {
		t.Fatalf("FormatListMarkdown() = %q, want preserve-links hint", got)
	}
}

// TestPipelineSummary_Variants verifies package pipeline summaries for primary,
// historical, linked, and empty pipeline data.
func TestPipelineSummary_Variants(t *testing.T) {
	tests := []struct {
		name string
		pkg  ListItem
		want string
	}{
		{name: "none", pkg: ListItem{}, want: ""},
		{name: "primary", pkg: ListItem{Pipeline: &PipelineItem{ID: 7, Status: "success", Ref: "main"}}, want: "7 success main"},
		{name: "primary link", pkg: ListItem{Pipeline: &PipelineItem{ID: 7, Status: "success", Ref: "main", WebURL: "https://gitlab.example.com/pipelines/7"}}, want: "[7 success main](https://gitlab.example.com/pipelines/7)"},
		{name: "single history", pkg: ListItem{Pipelines: []PipelineItem{{ID: 8, Status: "failed", Ref: "release"}}}, want: "8 failed release"},
		{name: "multiple history", pkg: ListItem{Pipelines: []PipelineItem{{ID: 9, Status: "running", Ref: "dev"}, {ID: 10}}}, want: "9 running dev (+1)"},
		{name: "multiple history link", pkg: ListItem{Pipelines: []PipelineItem{{ID: 9, Status: "running", Ref: "dev", WebURL: "https://gitlab.example.com/pipelines/9"}, {ID: 10}}}, want: "[9 running dev](https://gitlab.example.com/pipelines/9) (+1)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pipelineSummary(tt.pkg); got != tt.want {
				t.Fatalf("pipelineSummary() = %q, want %q", got, tt.want)
			}
		})
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

// errNoReachAPI identifies the err no reach API constant used by this package.
const errNoReachAPI = "should not reach API"

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

const (
	// pathPutPkg1 identifies the path put pkg 1 constant used by this package.
	pathPutPkg1 = "PUT /api/v4/projects/1/packages/generic/my-pkg/1.0.0/app.tar.gz"
	// hdrContentType identifies the hdr content type constant used by this package.
	hdrContentType = "Content-Type"
	// mimeOctetStream identifies the mime octet stream constant used by this package.
	mimeOctetStream = "application/octet-stream"
	// fmtExpPkgVersionErr identifies the fmt exp pkg version err constant used by this package.
	fmtExpPkgVersionErr = "expected package_version error, got: %v"
	// pathTmpOutBin identifies the path tmp out bin constant used by this package.
	pathTmpOutBin = "/tmp/out.bin"
	// fmtExpProjectIDErr identifies the fmt exp project ID err constant used by this package.
	fmtExpProjectIDErr = "expected project_id error, got: %v"
	// testCtxCancelled identifies the test ctx cancelled constant used by this package.
	testCtxCancelled = "context canceled"
	// fmtExpCtxCancelErr identifies the fmt exp ctx cancel err constant used by this package.
	fmtExpCtxCancelErr = "expected context canceled error, got: %v"
	// pathAPIPkgs1 identifies the path API pkgs 1 constant used by this package.
	pathAPIPkgs1 = "/api/v4/projects/1/packages"
	// testFileAppBin identifies the test file app bin constant used by this package.
	testFileAppBin = "app.bin"
	// testFileOutBin identifies the test file out bin constant used by this package.
	testFileOutBin = "out.bin"
	// fmtExpCtxCancelGot identifies the fmt exp ctx cancel got constant used by this package.
	fmtExpCtxCancelGot = "expected context canceled, got: %v"
)

// ---------------------------------------------------------------------------
// Publish — missing package_version
// ---------------------------------------------------------------------------.

// TestPublish_MissingVersion verifies Publish when missing version.
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

// TestPublish_InvalidFileName verifies Publish when invalid file name.
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

// TestPublish_InvalidBase64 verifies Publish when invalid base 64.
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

// TestDownload_MissingProjectID verifies Download when missing project ID.
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

// TestDownload_MissingPackageName verifies Download when missing package name.
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

// TestDownload_MissingVersion verifies Download when missing version.
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

// TestDownload_MissingFileName verifies Download when missing file name.
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

// TestList_APIError verifies List when API error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestList_ContextCancelled verifies List when context cancelled.
func TestList_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := List(ctx, client, ListInput{ProjectID: "1"})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelErr, err)
	}
}

// TestList_WithSortAndOrderBy verifies List when with sort and order by.
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

// TestList_WithEmptyPackage verifies list output when a package has no tags or links.
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

// TestFileList_APIError verifies FileList when API error.
func TestFileList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := FileList(context.Background(), client, FileListInput{ProjectID: "1", PackageID: "10"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestFileList_ContextCancelled verifies FileList when context cancelled.
func TestFileList_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := FileList(ctx, client, FileListInput{ProjectID: "1", PackageID: "10"})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelErr, err)
	}
}

// TestFileList_MissingProjectID verifies FileList when missing project ID.
func TestFileList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := FileList(context.Background(), client, FileListInput{PackageID: "10"})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// TestFileList_WithCreatedAt verifies file list output when created_at is present.
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

// TestDelete_APIError verifies Delete when API error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	err := Delete(context.Background(), nil, client, DeleteInput{ProjectID: "1", PackageID: "10"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_ContextCancelled verifies Delete when context cancelled.
func TestDelete_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)
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

// TestFileDelete_APIError verifies FileDelete when API error.
func TestFileDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	err := FileDelete(context.Background(), nil, client, FileDeleteInput{ProjectID: "1", PackageID: "10", PackageFileID: "20"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestFileDelete_ContextCancelled verifies FileDelete when context cancelled.
func TestFileDelete_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := FileDelete(ctx, nil, client, FileDeleteInput{ProjectID: "1", PackageID: "10", PackageFileID: "20"})
	if err == nil || !strings.Contains(err.Error(), testCtxCancelled) {
		t.Fatalf(fmtExpCtxCancelErr, err)
	}
}

// TestFileDelete_MissingProjectID verifies FileDelete when missing project ID.
func TestFileDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	err := FileDelete(context.Background(), nil, client, FileDeleteInput{PackageID: "10", PackageFileID: "20"})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// TestFileDelete_MissingPackageID verifies FileDelete when missing package ID.
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

// TestPublishDirectory_MissingProjectID verifies PublishDirectory when missing project ID.
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

// TestPublishDirectory_InvalidPackageName verifies PublishDirectory when invalid package name.
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

// TestPublishDirectory_MissingVersion verifies PublishDirectory when missing version.
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

// TestPublishDirectory_NonexistentDir verifies PublishDirectory when nonexistent dir.
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

// TestStreamDownload_ContextCancelled verifies StreamDownload when context cancelled.
func TestStreamDownload_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)
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

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// streamDownloadPackageFile — successful download
// ---------------------------------------------------------------------------.

// TestStreamDownload_Success verifies StreamDownload when success.
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

// TestStreamDownload_APIError verifies StreamDownload when API error.
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

// TestPublish_BothFileAndBase64 verifies Publish when both file and base 64.
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

// TestPublish_NeitherFileNorBase64 verifies Publish when neither file nor base 64.
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

// TestPublish_APIError verifies Publish when API error.
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

// TestPublish_ContextCancelled verifies Publish when context cancelled.
func TestPublish_ContextCancelled(t *testing.T) {
	ctx := testutil.CancelledCtx(t)
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

// TestPublish_FilePathSmallFile verifies Publish when file path small file.
func TestPublish_FilePathSmallFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "small.bin")
	if err := os.WriteFile(tmpFile, []byte("small-data"), 0o600); err != nil {
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

// TestPublish_InvalidPackageName verifies Publish when invalid package name.
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

// TestPublish_MissingProjectID verifies Publish when missing project ID.
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

// TestList_WithNameAndTypeFilter verifies List when with name and type filter.
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

// TestPublishDirectory_EmptyDir verifies PublishDirectory when empty dir.
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

// TestPublish_FilePathNonexistent verifies Publish when file path nonexistent.
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
