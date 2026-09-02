// packages_composite_test.go contains unit tests for composite package
// operations: publish-and-link and publish-directory, including the
// progress scale a single tool call reports against.
package packages

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// PublishAndLink tests.

// TestPackagePublishAndLink_Success verifies PackagePublishAndLink when success.
func TestPackagePublishAndLink_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == pathPackagePublish:
			testutil.RespondJSON(w, http.StatusCreated, publishResponseJSON)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/releases/v1.0.0/assets/links":
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 50,
				"name": "app.tar.gz",
				"url": "https://example.com/packages/generic/my-pkg/1.0.0/app.tar.gz",
				"link_type": "package",
				"external": true
			}`)
		default:
			http.NotFound(w, r)
		}
	}))

	content := base64.StdEncoding.EncodeToString([]byte("publish-and-link-data"))
	out, err := PublishAndLink(context.Background(), nil, client, PublishAndLinkInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		FileName:       "app.tar.gz",
		ContentBase64:  content,
		TagName:        "v1.0.0",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Package.PackageFileID != 1 {
		t.Errorf("Package.PackageFileID = %d, want 1", out.Package.PackageFileID)
	}
	if out.ReleaseLink.ID != 50 {
		t.Errorf("ReleaseLink.ID = %d, want 50", out.ReleaseLink.ID)
	}
	if out.ReleaseLink.LinkType != "package" {
		t.Errorf("ReleaseLink.LinkType = %q, want %q", out.ReleaseLink.LinkType, "package")
	}
}

// TestPackagePublishAndLink_DefaultLinkName verifies PackagePublishAndLink when default link name.
func TestPackagePublishAndLink_DefaultLinkName(t *testing.T) {
	var capturedLinkName string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == pathPackagePublish:
			testutil.RespondJSON(w, http.StatusCreated, publishResponseJSON)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/assets/links"):
			capturedLinkName = "captured"
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 51,
				"name": "app.tar.gz",
				"url": "https://example.com/pkg",
				"link_type": "package",
				"external": true
			}`)
		default:
			http.NotFound(w, r)
		}
	}))

	content := base64.StdEncoding.EncodeToString([]byte("data"))
	out, err := PublishAndLink(context.Background(), nil, client, PublishAndLinkInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		FileName:       "app.tar.gz",
		ContentBase64:  content,
		TagName:        "v1.0.0",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if capturedLinkName == "" {
		t.Fatal("release link creation was not called")
	}
	if out.ReleaseLink.Name != "app.tar.gz" {
		t.Errorf("ReleaseLink.Name = %q, want %q (defaulted from file_name)", out.ReleaseLink.Name, "app.tar.gz")
	}
}

// TestPackagePublishAndLink_CustomLinkType verifies PackagePublishAndLink when custom link type.
func TestPackagePublishAndLink_CustomLinkType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == pathPackagePublish:
			testutil.RespondJSON(w, http.StatusCreated, publishResponseJSON)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/assets/links"):
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 52,
				"name": "Runbook",
				"url": "https://example.com/pkg",
				"link_type": "runbook",
				"external": true
			}`)
		default:
			http.NotFound(w, r)
		}
	}))

	content := base64.StdEncoding.EncodeToString([]byte("data"))
	out, err := PublishAndLink(context.Background(), nil, client, PublishAndLinkInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		FileName:       "app.tar.gz",
		ContentBase64:  content,
		TagName:        "v1.0.0",
		LinkName:       "Runbook",
		LinkType:       "runbook",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ReleaseLink.LinkType != "runbook" {
		t.Errorf("ReleaseLink.LinkType = %q, want %q", out.ReleaseLink.LinkType, "runbook")
	}
}

// TestPackagePublishAndLink_MissingTagName verifies PackagePublishAndLink when missing tag name.
func TestPackagePublishAndLink_MissingTagName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	content := base64.StdEncoding.EncodeToString([]byte("data"))
	_, err := PublishAndLink(context.Background(), nil, client, PublishAndLinkInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		FileName:       "app.tar.gz",
		ContentBase64:  content,
	})
	if err == nil {
		t.Fatal("expected error for missing tag_name, got nil")
	}
	if !strings.Contains(err.Error(), "tag_name") {
		t.Errorf("error should mention tag_name, got: %v", err)
	}
}

// TestPackagePublishAndLink_PublishFails verifies PackagePublishAndLink when publish fails.
func TestPackagePublishAndLink_PublishFails(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	content := base64.StdEncoding.EncodeToString([]byte("data"))
	_, err := PublishAndLink(context.Background(), nil, client, PublishAndLinkInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		FileName:       "app.tar.gz",
		ContentBase64:  content,
		TagName:        "v1.0.0",
	})
	if err == nil {
		t.Fatal("expected error when publish fails, got nil")
	}
	if !strings.Contains(err.Error(), "packagePublishAndLink/publish") {
		t.Errorf("error should mention packagePublishAndLink/publish, got: %v", err)
	}
}

// TestPackagePublishAndLink_LinkFails_ReturnsPackageInfo verifies PackagePublishAndLink returns package info with link fails.
func TestPackagePublishAndLink_LinkFails_ReturnsPackageInfo(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == pathPackagePublish:
			testutil.RespondJSON(w, http.StatusCreated, publishResponseJSON)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/assets/links"):
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"Release Not Found"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	content := base64.StdEncoding.EncodeToString([]byte("data"))
	out, err := PublishAndLink(context.Background(), nil, client, PublishAndLinkInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		FileName:       "app.tar.gz",
		ContentBase64:  content,
		TagName:        "v1.0.0",
	})
	if err == nil {
		t.Fatal("expected error when link creation fails, got nil")
	}
	if !strings.Contains(err.Error(), "packagePublishAndLink/link") {
		t.Errorf("error should mention packagePublishAndLink/link, got: %v", err)
	}
	if out.Package.PackageFileID != 1 {
		t.Errorf("Package.PackageFileID = %d, want 1 (should be returned even on link failure)", out.Package.PackageFileID)
	}
}

// TestPackagePublishAndLink_ContextCancelled verifies PackagePublishAndLink when context cancelled.
func TestPackagePublishAndLink_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := PublishAndLink(ctx, nil, client, PublishAndLinkInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		FileName:       "app.tar.gz",
		ContentBase64:  base64.StdEncoding.EncodeToString([]byte("data")),
		TagName:        "v1.0.0",
	})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// PublishDirectory tests.

// TestPackagePublishDirectory_Success verifies PackagePublishDirectory when success.
func TestPackagePublishDirectory_Success(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.tar.gz", "b.tar.gz", "readme.md"} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("content-"+name), 0o600); err != nil {
				t.Fatalf("write file: %v", err)
			}
		})
	}

	publishCount := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/packages/generic/") {
			publishCount++
			testutil.RespondJSON(w, http.StatusCreated, fmt.Sprintf(`{
				"id": %d,
				"package_id": 10,
				"file_name": "file%d.tar.gz",
				"size": 100,
				"file_sha256": "hash%d",
				"file_md5": "md5",
				"file_sha1": "sha1",
				"file_store": 1
			}`, publishCount, publishCount, publishCount))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		DirectoryPath:  dir,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.TotalFiles != 3 {
		t.Errorf("TotalFiles = %d, want 3", out.TotalFiles)
	}
	if len(out.Published) != 3 {
		t.Errorf("Published count = %d, want 3", len(out.Published))
	}
	if len(out.Errors) != 0 {
		t.Errorf("unexpected errors: %v", out.Errors)
	}
}

// TestPackagePublishDirectory_WithPattern verifies PackagePublishDirectory when with pattern.
func TestPackagePublishDirectory_WithPattern(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.tar.gz", "b.tar.gz", "readme.md", "notes.txt"} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0o600); err != nil {
				t.Fatalf("write file: %v", err)
			}
		})
	}

	publishCount := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/packages/generic/") {
			publishCount++
			testutil.RespondJSON(w, http.StatusCreated, fmt.Sprintf(`{
				"id": %d, "package_id": 10, "file_name": "file.tar.gz",
				"size": 50, "file_sha256": "hash", "file_md5": "md5", "file_sha1": "sha1", "file_store": 1
			}`, publishCount))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		DirectoryPath:  dir,
		IncludePattern: "*.tar.gz",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2 (only *.tar.gz)", out.TotalFiles)
	}
}

// TestPackagePublishDirectory_WithProgressToken verifies PublishDirectory sends
// progress notifications when invoked through MCP with a progress token.
func TestPackagePublishDirectory_WithProgressToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "artifact.bin"), []byte("progress-data"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/packages/generic/") {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 9, "package_id": 10, "file_name": "artifact.bin",
				"size": 13, "file_sha256": "hash", "file_md5": "md5", "file_sha1": "sha1", "file_store": 1
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "package-directory-test", Version: "0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "package_directory_with_progress"}, func(ctx context.Context, req *mcp.CallToolRequest, input PublishDirInput) (*mcp.CallToolResult, PublishDirOutput, error) {
		out, err := PublishDirectory(ctx, req, client, input)
		if err != nil {
			return nil, PublishDirOutput{}, err
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
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "package-directory-client"}, &mcp.ClientOptions{
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
		Name: "package_directory_with_progress",
		Arguments: map[string]any{
			"project_id":      "42",
			"package_name":    "my-pkg",
			"package_version": "1.0.0",
			"directory_path":  dir,
		},
		Meta: mcp.Meta{"progressToken": "package-directory-progress-token"},
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

// TestPackagePublishDirectory_NoMatchingFiles verifies PackagePublishDirectory when no matching files.
func TestPackagePublishDirectory_NoMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("text"), 0o600)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		DirectoryPath:  dir,
		IncludePattern: "*.tar.gz",
	})
	if err == nil {
		t.Fatal("expected error for no matching files, got nil")
	}
	if !strings.Contains(err.Error(), "no matching files") {
		t.Errorf("error should mention no matching files, got: %v", err)
	}
}

// TestPackagePublishDirectory_EmptyDir verifies PackagePublishDirectory when empty dir.
func TestPackagePublishDirectory_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		DirectoryPath:  dir,
	})
	if err == nil {
		t.Fatal("expected error for empty directory, got nil")
	}
}

// TestPackagePublishDirectory_NotADirectory verifies PackagePublishDirectory when not a directory.
func TestPackagePublishDirectory_NotADirectory(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "not-a-dir.txt")
	os.WriteFile(tmpFile, []byte("file"), 0o600)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		DirectoryPath:  tmpFile,
	})
	if err == nil {
		t.Fatal("expected error for non-directory path, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error should mention not a directory, got: %v", err)
	}
}

// TestPackagePublishDirectory_OutsideAllowedDirs_Rejected verifies that the
// directory a publish walks is confined to the allow-listed roots. It is the
// widest local read in the tool surface — one call ships every file it finds
// to a project the caller names — so a directory outside the workspace, a
// parent-traversal escape and a symlinked directory are all refused, and no
// upload is attempted: the mock handler fails the test if it is reached.
func TestPackagePublishDirectory_OutsideAllowedDirs_Rejected(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "home")
	makeDirs(t, allowed, outside)
	if err := os.WriteFile(filepath.Join(outside, "id_rsa"), []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	link := filepath.Join(allowed, "linked")
	symlinked := os.Symlink(outside, link) == nil
	confineDownloadRoots(t, allowed)

	tests := []struct {
		name string
		path string
		skip bool
	}{
		{name: "directory outside every allowed root is refused", path: outside},
		{name: "parent traversal out of the workspace is refused", path: filepath.Join(allowed, "..", "home")},
		{name: "symlinked directory pointing outside is refused", path: link, skip: !symlinked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("symlinks unsupported on this platform")
			}
			client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

			_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
				ProjectID:      "42",
				PackageName:    "my-pkg",
				PackageVersion: "1.0.0",
				DirectoryPath:  tt.path,
			})
			if err == nil {
				t.Fatalf("PublishDirectory(%q) error = nil, want refusal", tt.path)
			}
			if !strings.Contains(err.Error(), "outside allowed") {
				t.Errorf("PublishDirectory(%q) error = %q, want it to name the allow-list", tt.path, err)
			}
		})
	}
}

// TestPackagePublishDirectory_MissingDirectoryPath verifies PackagePublishDirectory when missing directory path.
func TestPackagePublishDirectory_MissingDirectoryPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
	})
	if err == nil {
		t.Fatal("expected error for missing directory_path, got nil")
	}
	if !strings.Contains(err.Error(), "directory_path") {
		t.Errorf("error should mention directory_path, got: %v", err)
	}
}

// TestPackagePublishDirectory_InvalidGlobPattern verifies PackagePublishDirectory when invalid glob pattern.
func TestPackagePublishDirectory_InvalidGlobPattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("data"), 0o600)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		DirectoryPath:  dir,
		IncludePattern: "[invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid glob pattern, got nil")
	}
	if !strings.Contains(err.Error(), "invalid glob") {
		t.Errorf("error should mention invalid glob, got: %v", err)
	}
}

// TestPackagePublishDirectory_PartialFailure verifies PackagePublishDirectory when partial failure.
func TestPackagePublishDirectory_PartialFailure(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"good.bin", "bad.bin"} {
		os.WriteFile(filepath.Join(dir, name), []byte("content"), 0o600)
	}

	callCount := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/packages/generic/") {
			callCount++
			if strings.Contains(r.URL.Path, "bad.bin") {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
				return
			}
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 1, "package_id": 10, "file_name": "good.bin",
				"size": 7, "file_sha256": "hash", "file_md5": "md5", "file_sha1": "sha1", "file_store": 1
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		DirectoryPath:  dir,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1 (only good.bin succeeded)", out.TotalFiles)
	}
	if len(out.Errors) != 1 {
		t.Errorf("Errors count = %d, want 1", len(out.Errors))
	}
	if len(out.Errors) > 0 && !strings.Contains(out.Errors[0], "bad.bin") {
		t.Errorf("error should mention bad.bin, got: %s", out.Errors[0])
	}
}

// TestPackagePublishDirectory_ContextCancelled verifies PackagePublishDirectory when context cancelled.
func TestPackagePublishDirectory_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.bin"), []byte("data"), 0o600)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx := testutil.CancelledCtx(t)
	_, err := PublishDirectory(ctx, nil, client, PublishDirInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		DirectoryPath:  dir,
	})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestPackagePublishDirectory_CancelledDuringLoop verifies PublishDirectory
// returns partial output if cancellation happens after one file is published.
func TestPackagePublishDirectory_CancelledDuringLoop(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.bin", "b.bin"} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0o600); err != nil {
				t.Fatalf("write file: %v", err)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/packages/generic/") {
			callCount++
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 1, "package_id": 10, "file_name": "a.bin",
				"size": 7, "file_sha256": "hash", "file_md5": "md5", "file_sha1": "sha1", "file_store": 1
			}`)
			cancel()
			return
		}
		http.NotFound(w, r)
	}))

	out, err := PublishDirectory(ctx, nil, client, PublishDirInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		DirectoryPath:  dir,
	})
	if err == nil {
		t.Fatal("expected cancellation error after first file")
	}
	if !strings.Contains(err.Error(), "context canceled after 1 of 2 files") {
		t.Fatalf("error = %q, want partial cancellation message", err.Error())
	}
	if out.TotalFiles != 0 || len(out.Published) != 1 {
		t.Fatalf("partial output = %+v, want one published item before totals", out)
	}
	if callCount != 1 {
		t.Fatalf("callCount = %d, want 1", callCount)
	}
}

// TestPackagePublishDirectory_SkipsSubdirectories verifies PackagePublishDirectory when skips subdirectories.
func TestPackagePublishDirectory_SkipsSubdirectories(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.bin"), []byte("content"), 0o600)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o750)
	os.WriteFile(filepath.Join(dir, "subdir", "nested.bin"), []byte("nested"), 0o600)

	publishCount := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/packages/generic/") {
			publishCount++
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 1, "package_id": 10, "file_name": "file.bin",
				"size": 7, "file_sha256": "hash", "file_md5": "md5", "file_sha1": "sha1", "file_store": 1
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		DirectoryPath:  dir,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1 (subdirectories should be skipped)", out.TotalFiles)
	}
	if publishCount != 1 {
		t.Errorf("publishCount = %d, want 1", publishCount)
	}
}

// TestPackagePublishDirectory_SkipsSymlinks verifies that symlinks are excluded
// because shouldIncludeFile checks info.Mode().IsRegular().
func TestPackagePublishDirectory_SkipsSymlinks(t *testing.T) {
	dir := t.TempDir()

	// Create a regular file.
	regular := filepath.Join(dir, "real.bin")
	os.WriteFile(regular, []byte("real"), 0o600)

	// Create a symlink to that file.
	symlink := filepath.Join(dir, "link.bin")
	if err := os.Symlink(regular, symlink); err != nil {
		t.Skip("symlinks not supported on this filesystem:", err)
	}

	publishCount := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/packages/generic/") {
			publishCount++
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 1, "package_id": 10, "file_name": "real.bin",
				"size": 4, "file_sha256": "h", "file_md5": "m", "file_sha1": "s", "file_store": 1
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := PublishDirectory(context.Background(), nil, client, PublishDirInput{
		ProjectID:      "42",
		PackageName:    "my-pkg",
		PackageVersion: "1.0.0",
		DirectoryPath:  dir,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1 (symlink should be excluded)", out.TotalFiles)
	}
	if publishCount != 1 {
		t.Errorf("publishCount = %d, want 1 (only real.bin)", publishCount)
	}
}

// TestCollectMatchingFiles_NonexistentDir verifies that collectMatchingFiles
// returns an error when the directory does not exist.
func TestCollectMatchingFiles_NonexistentDir(t *testing.T) {
	_, err := collectMatchingFiles("/nonexistent-path-42", "")
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}
}

type failingDirEntry struct{}

func (failingDirEntry) Name() string { return "missing.bin" }

func (failingDirEntry) IsDir() bool { return false }

func (failingDirEntry) Type() fs.FileMode { return 0 }

func (failingDirEntry) Info() (fs.FileInfo, error) { return nil, errors.New("stat missing.bin") }

// TestShouldIncludeFile_InfoError verifies shouldIncludeFile returns stat errors.
func TestShouldIncludeFile_InfoError(t *testing.T) {
	included, err := shouldIncludeFile(failingDirEntry{}, "")
	if err == nil {
		t.Fatal("expected info error, got nil")
	}
	if included {
		t.Fatal("included = true, want false")
	}
}

// TestPublishDirectory_ProgressSequenceOnlyIncreases pins the progress
// invariant across the whole tool call.
//
// "The progress value MUST increase with each notification, even if the total
// is unknown."
//
// PublishDirectory used to build one Tracker to count files and call Publish,
// which built a second one from the same request to count bytes. Each Tracker
// carried its own monotonic state, so the guard inside Update compared a
// Tracker only against itself and neither could see the other. On one progress
// token a client received a sequence that ran 200000 -> 1 -> 0, with `total`
// oscillating between the file count and a file's byte count.
//
// No existing test could see it. The publish_directory tests pass a nil
// request, so no Tracker is ever active; the monotonicity test in
// internal/progress builds a single Tracker, which is exactly the case that
// worked; and the e2e progress case asserts only that some frame arrives.
// Watching the real notification stream of one call is what it takes.
func TestPublishDirectory_ProgressSequenceOnlyIncreases(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/packages/generic/") {
			// Drained so the ProgressReader reaches EOF and reports.
			buf := make([]byte, 32*1024)
			for {
				if _, err := r.Body.Read(buf); err != nil {
					break
				}
			}
			testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"file_name":"f","size":1,"file_sha256":"abc"}`)
			return
		}
		http.NotFound(w, r)
	}))

	// Two files of different sizes, each well past the reader's report
	// interval, so both levels of the old design would have emitted.
	dir := t.TempDir()
	files := map[string]int{"a.bin": 200 * 1024, "b.bin": 320 * 1024}
	for name, size := range files {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Repeat("x", size)), 0o600); err != nil {
				t.Fatalf("writing %s: %v", name, err)
			}
		})
	}

	var wantTotal float64
	for _, size := range files {
		wantTotal += float64(size)
	}

	got := progressOfOneCall(t, func(ctx context.Context, req *mcp.CallToolRequest) error {
		_, err := PublishDirectory(ctx, req, client, PublishDirInput{
			ProjectID:      "42",
			PackageName:    "pkg",
			PackageVersion: "1.0.0",
			DirectoryPath:  dir,
		})
		return err
	}, func(seen []mcp.ProgressNotificationParams) bool {
		// The job is done when the last frame reports the whole total, which
		// is the same thing the assertions below check for.
		return len(seen) > 0 && seen[len(seen)-1].Progress == wantTotal
	})

	if len(got) < 2 {
		t.Fatalf("got %d progress notifications, want several: the call reported almost nothing", len(got))
	}

	prev := -1.0
	for i, n := range got {
		if n.Progress <= prev {
			t.Errorf("notification %d went backwards: progress %v after %v (message %q)",
				i, n.Progress, prev, n.Message)
		}
		// A total that changes meaning mid-call is the same defect seen from
		// the other side: the client cannot render a bar against it.
		if n.Total != wantTotal {
			t.Errorf("notification %d has total %v, want %v (the whole job, on one scale)",
				i, n.Total, wantTotal)
		}
		prev = n.Progress
	}

	if last := got[len(got)-1]; last.Progress != wantTotal {
		t.Errorf("the last notification reports %v of %v, want the job finished", last.Progress, wantTotal)
	}
}

// progressOfOneCall runs handler as one MCP tool call with a progress token and
// returns every progress notification the client received, in order.
//
// The whole sequence is what matters: "the progress value MUST increase with
// each notification" is a property of the series a client observes on one
// token, so a test that inspects notifications one at a time, or that reads the
// tracker's own state, cannot check it.
func progressOfOneCall(
	t *testing.T,
	handler func(context.Context, *mcp.CallToolRequest) error,
	complete func([]mcp.ProgressNotificationParams) bool,
) []mcp.ProgressNotificationParams {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	var mu sync.Mutex
	var got []mcp.ProgressNotificationParams

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "progress-test", Version: "v0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "under_test", Description: "the handler under test"},
		func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			if err := handler(ctx, req); err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		})

	ss, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "progress-client", Version: "v0"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, r *mcp.ProgressNotificationClientRequest) {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, *r.Params)
		},
	})
	cs, connErr := mcpClient.Connect(ctx, clientTransport, nil)
	if connErr != nil {
		t.Fatalf("client connect: %v", connErr)
	}
	t.Cleanup(func() { _ = cs.Close(); ss.Wait() })

	if _, callErr := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "under_test",
		Arguments: map[string]any{},
		Meta:      mcp.Meta{"progressToken": "tok"},
	}); callErr != nil {
		t.Fatalf("CallTool: %v", callErr)
	}

	// Progress notifications are one-way messages: the client queues them for
	// its handler while CallTool waits only for the tool response, so the call
	// can return before the last of them has been delivered. Waiting for the
	// caller's completion condition is what makes the sequence assertions
	// deterministic — a quiet period would only be a guess at how long delivery
	// takes, and a slow machine would make it wrong.
	waitFor(t, &mu, &got, complete)

	mu.Lock()
	defer mu.Unlock()
	return append([]mcp.ProgressNotificationParams(nil), got...)
}

// waitFor blocks until the notifications received satisfy complete, or fails
// the test with what it did see.
//
// Naming the terminal condition rather than waiting out a silence is the
// difference between a deterministic test and one that passes on a fast machine.
func waitFor(
	t *testing.T,
	mu *sync.Mutex,
	got *[]mcp.ProgressNotificationParams,
	complete func([]mcp.ProgressNotificationParams) bool,
) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		snapshot := append([]mcp.ProgressNotificationParams(nil), *got...)
		mu.Unlock()

		if complete(snapshot) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("the expected final progress notification never arrived; received %d: %+v", len(*got), *got)
}
