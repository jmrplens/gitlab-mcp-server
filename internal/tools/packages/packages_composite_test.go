// packages_composite_test.go contains unit tests for composite package
// operations: publish-and-link and publish-directory.
package packages

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// PublishAndLink tests.

// TestPackagePublishAndLink_Success verifies that PackagePublishAndLink handles the success scenario correctly.
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

// TestPackagePublishAndLink_DefaultLinkName verifies that PackagePublishAndLink handles the default link name scenario correctly.
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

// TestPackagePublishAndLink_CustomLinkType verifies that PackagePublishAndLink handles the custom link type scenario correctly.
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

// TestPackagePublishAndLink_MissingTagName verifies that PackagePublishAndLink handles the missing tag name scenario correctly.
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

// TestPackagePublishAndLink_PublishFails verifies that PackagePublishAndLink handles the publish fails scenario correctly.
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

// TestPackagePublishAndLink_LinkFails_ReturnsPackageInfo verifies that PackagePublishAndLink handles the link fails_ returns package info scenario correctly.
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

// TestPackagePublishAndLink_ContextCancelled verifies that PackagePublishAndLink handles the context cancelled scenario correctly.
func TestPackagePublishAndLink_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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

// TestPackagePublishDirectory_Success verifies that PackagePublishDirectory handles the success scenario correctly.
func TestPackagePublishDirectory_Success(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.tar.gz", "b.tar.gz", "readme.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content-"+name), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
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

// TestPackagePublishDirectory_WithPattern verifies that PackagePublishDirectory handles the with pattern scenario correctly.
func TestPackagePublishDirectory_WithPattern(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.tar.gz", "b.tar.gz", "readme.md", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
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

// TestPackagePublishDirectory_NoMatchingFiles verifies that PackagePublishDirectory handles the no matching files scenario correctly.
func TestPackagePublishDirectory_NoMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("text"), 0644)

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

// TestPackagePublishDirectory_EmptyDir verifies that PackagePublishDirectory handles the empty dir scenario correctly.
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

// TestPackagePublishDirectory_NotADirectory verifies that PackagePublishDirectory handles the not a directory scenario correctly.
func TestPackagePublishDirectory_NotADirectory(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "not-a-dir.txt")
	os.WriteFile(tmpFile, []byte("file"), 0644)

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

// TestPackagePublishDirectory_MissingDirectoryPath verifies that PackagePublishDirectory handles the missing dir path scenario correctly.
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

// TestPackagePublishDirectory_InvalidGlobPattern verifies that PackagePublishDirectory handles the invalid glob pattern scenario correctly.
func TestPackagePublishDirectory_InvalidGlobPattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("data"), 0644)

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

// TestPackagePublishDirectory_PartialFailure verifies that PackagePublishDirectory handles the partial failure scenario correctly.
func TestPackagePublishDirectory_PartialFailure(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"good.bin", "bad.bin"} {
		os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644)
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

// TestPackagePublishDirectory_ContextCancelled verifies that PackagePublishDirectory handles the context cancelled scenario correctly.
func TestPackagePublishDirectory_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.bin"), []byte("data"), 0644)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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

// TestPackagePublishDirectory_SkipsSubdirectories verifies that PackagePublishDirectory handles the skips subdirectories scenario correctly.
func TestPackagePublishDirectory_SkipsSubdirectories(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.bin"), []byte("content"), 0644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)
	os.WriteFile(filepath.Join(dir, "subdir", "nested.bin"), []byte("nested"), 0644)

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
