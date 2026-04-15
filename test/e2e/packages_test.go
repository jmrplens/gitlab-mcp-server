//go:build e2e

package e2e

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/packages"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func i64soi(v int64) toolutil.StringOrInt {
	return toolutil.StringOrInt(strconv.FormatInt(v, 10))
}

// TestPackages exercises the package registry lifecycle: publish, list, file-list,
// download, file-delete, and package-delete through both individual and meta-tool sessions.
func TestPackages(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// --- Individual tool session ---
	proj := createProject(ctx, t, sess.individual)
	commitFile(ctx, t, sess.individual, proj, "main", "init.txt", "init", "init commit")

	const (
		pkgName    = "e2e-test-pkg"
		pkgVersion = "1.0.0"
		fileName   = "data.txt"
	)
	fileContent := base64.StdEncoding.EncodeToString([]byte("hello package"))

	var packageID int64
	var packageFileID int64

	t.Run("Individual/Publish", func(t *testing.T) {
		out, err := callToolOn[packages.PublishOutput](ctx, sess.individual, "gitlab_package_publish", packages.PublishInput{
			ProjectID:      proj.pidOf(),
			PackageName:    pkgName,
			PackageVersion: pkgVersion,
			FileName:       fileName,
			ContentBase64:  fileContent,
		})
		if err != nil {
			t.Fatalf("publish: %v", err)
		}
		if out.PackageID == 0 {
			t.Fatal("expected non-zero package ID")
		}
		packageID = out.PackageID
		packageFileID = out.PackageFileID
		t.Logf("Published package ID=%d, fileID=%d", packageID, packageFileID)
	})

	t.Run("Individual/List", func(t *testing.T) {
		out, err := callToolOn[packages.ListOutput](ctx, sess.individual, "gitlab_package_list", packages.ListInput{
			ProjectID: proj.pidOf(),
		})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(out.Packages) == 0 {
			t.Fatal("expected at least one package")
		}
		found := false
		for _, p := range out.Packages {
			if p.Name == pkgName {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("package %q not found in list", pkgName)
		}
	})

	t.Run("Individual/FileList", func(t *testing.T) {
		out, err := callToolOn[packages.FileListOutput](ctx, sess.individual, "gitlab_package_file_list", packages.FileListInput{
			ProjectID: proj.pidOf(),
			PackageID: i64soi(packageID),
		})
		if err != nil {
			t.Fatalf("file list: %v", err)
		}
		if len(out.Files) == 0 {
			t.Fatal("expected at least one file")
		}
		if out.Files[0].FileName != fileName {
			t.Fatalf("expected file %q, got %q", fileName, out.Files[0].FileName)
		}
	})

	t.Run("Individual/Download", func(t *testing.T) {
		tmpDir := t.TempDir()
		outPath := tmpDir + "/downloaded.txt"
		out, err := callToolOn[packages.DownloadOutput](ctx, sess.individual, "gitlab_package_download", packages.DownloadInput{
			ProjectID:      proj.pidOf(),
			PackageName:    pkgName,
			PackageVersion: pkgVersion,
			FileName:       fileName,
			OutputPath:     outPath,
		})
		if err != nil {
			t.Fatalf("download: %v", err)
		}
		if out.Size == 0 {
			t.Fatal("expected non-zero file size")
		}
		t.Logf("Downloaded %d bytes to %s", out.Size, out.OutputPath)
	})

	t.Run("Individual/FileDelete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_package_file_delete", packages.FileDeleteInput{
			ProjectID:     proj.pidOf(),
			PackageID:     i64soi(packageID),
			PackageFileID: i64soi(packageFileID),
		})
		if err != nil {
			t.Fatalf("file delete: %v", err)
		}
	})

	t.Run("Individual/Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_package_delete", packages.DeleteInput{
			ProjectID: proj.pidOf(),
			PackageID: i64soi(packageID),
		})
		if err != nil {
			t.Fatalf("delete: %v", err)
		}
	})

	// --- Meta-tool session ---
	projM := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, projM, "main", "init.txt", "init", "init commit")

	var mPkgID int64
	var mFileID int64

	t.Run("Meta/Publish", func(t *testing.T) {
		out, err := callToolOn[packages.PublishOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "publish",
			"params": map[string]any{
				"project_id":      projM.pidStr(),
				"package_name":    pkgName,
				"package_version": pkgVersion,
				"file_name":       fileName,
				"content_base64":  fileContent,
			},
		})
		if err != nil {
			t.Fatalf("meta publish: %v", err)
		}
		mPkgID = out.PackageID
		mFileID = out.PackageFileID
		t.Logf("Meta published package ID=%d, fileID=%d", mPkgID, mFileID)
	})

	t.Run("Meta/List", func(t *testing.T) {
		out, err := callToolOn[packages.ListOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_id": projM.pidStr(),
			},
		})
		if err != nil {
			t.Fatalf("meta list: %v", err)
		}
		if len(out.Packages) == 0 {
			t.Fatal("expected at least one package (meta)")
		}
	})

	t.Run("Meta/FileList", func(t *testing.T) {
		out, err := callToolOn[packages.FileListOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "file_list",
			"params": map[string]any{
				"project_id": projM.pidStr(),
				"package_id": fmt.Sprintf("%d", mPkgID),
			},
		})
		if err != nil {
			t.Fatalf("meta file list: %v", err)
		}
		if len(out.Files) == 0 {
			t.Fatal("expected at least one file (meta)")
		}
	})

	t.Run("Meta/Download", func(t *testing.T) {
		tmpDir := t.TempDir()
		outPath := tmpDir + "/downloaded.txt"
		out, err := callToolOn[packages.DownloadOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "download",
			"params": map[string]any{
				"project_id":      projM.pidStr(),
				"package_name":    pkgName,
				"package_version": pkgVersion,
				"file_name":       fileName,
				"output_path":     outPath,
			},
		})
		if err != nil {
			t.Fatalf("meta download: %v", err)
		}
		if out.Size == 0 {
			t.Fatal("expected non-zero file size (meta)")
		}
	})

	t.Run("Meta/FileDelete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "file_delete",
			"params": map[string]any{
				"project_id":      projM.pidStr(),
				"package_id":      fmt.Sprintf("%d", mPkgID),
				"package_file_id": fmt.Sprintf("%d", mFileID),
			},
		})
		if err != nil {
			t.Fatalf("meta file delete: %v", err)
		}
	})

	t.Run("Meta/Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id": projM.pidStr(),
				"package_id": fmt.Sprintf("%d", mPkgID),
			},
		})
		if err != nil {
			t.Fatalf("meta delete: %v", err)
		}
	})
}
