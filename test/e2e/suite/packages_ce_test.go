//go:build e2e && !enterprise

// packages_ce_test.go tests the package registry MCP tools against a live GitLab instance.
// Covers the full generic package lifecycle: publish, list, file-list, download,
// file-delete, and package-delete for both individual tools and the gitlab_package meta-tool.
package suite

import (
	"context"
	"encoding/base64"
	"strconv"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/packages"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// i64soi converts an int64 to a StringOrInt for use in package tool inputs.
func i64soi(v int64) toolutil.StringOrInt {
	return toolutil.StringOrInt(strconv.FormatInt(v, 10))
}

// TestPackages exercises the package registry lifecycle: publish, list, file-list,
// download, file-delete, and package-delete through both individual and meta-tool sessions.
func TestPackages(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	ctx := context.Background()

	proj := createProject(ctx, t, sess.individual)
	commitFile(ctx, t, sess.individual, proj, "main", "init.txt", "init", "init commit")

	const (
		pkgName    = "e2e-test-pkg"
		pkgVersion = "1.0.0"
		fileName   = "data.txt"
	)
	fileContent := base64.StdEncoding.EncodeToString([]byte("hello package"))
	fixture := packageLifecycleFixture{pkgName: pkgName, pkgVersion: pkgVersion, fileName: fileName, fileContent: fileContent}

	testIndividualPackageLifecycle(ctx, t, proj, fixture)

	projM := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, projM, "main", "init.txt", "init", "init commit")
	testMetaPackageLifecycle(ctx, t, projM, fixture)
}

type packageLifecycleFixture struct {
	pkgName     string
	pkgVersion  string
	fileName    string
	fileContent string
}

func testIndividualPackageLifecycle(ctx context.Context, t *testing.T, proj ProjectFixture, fixture packageLifecycleFixture) {
	t.Helper()
	var packageID int64
	var packageFileID int64

	t.Run("Individual/Publish", func(t *testing.T) {
		packageID, packageFileID = publishIndividualPackage(ctx, t, proj, fixture)
	})

	t.Run("Individual/List", func(t *testing.T) {
		assertIndividualPackageListed(ctx, t, proj, fixture)
	})

	t.Run("Individual/FileList", func(t *testing.T) {
		listedFileID := assertIndividualPackageFileListed(ctx, t, proj, fixture, packageID)
		if sess.enterprise {
			packageFileID = listedFileID
		}
	})

	t.Run("Individual/Download", func(t *testing.T) {
		downloadIndividualPackage(ctx, t, proj, fixture)
	})

	t.Run("Individual/FileDelete", func(t *testing.T) {
		deleteIndividualPackageFile(ctx, t, proj, packageID, packageFileID)
	})

	t.Run("Individual/Delete", func(t *testing.T) {
		deleteIndividualPackage(ctx, t, proj, packageID)
	})
}

func publishIndividualPackage(ctx context.Context, t *testing.T, proj ProjectFixture, fixture packageLifecycleFixture) (int64, int64) {
	t.Helper()
	out, err := callToolOn[packages.PublishOutput](ctx, sess.individual, "gitlab_package_publish", packages.PublishInput{
		ProjectID:      proj.pidOf(),
		PackageName:    fixture.pkgName,
		PackageVersion: fixture.pkgVersion,
		FileName:       fixture.fileName,
		ContentBase64:  fixture.fileContent,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if out.PackageID == 0 {
		t.Fatal("expected non-zero package ID")
	}
	t.Logf("Published package ID=%d, fileID=%d", out.PackageID, out.PackageFileID)
	return out.PackageID, out.PackageFileID
}

func assertIndividualPackageListed(ctx context.Context, t *testing.T, proj ProjectFixture, fixture packageLifecycleFixture) {
	t.Helper()
	out, err := callToolOn[packages.ListOutput](ctx, sess.individual, "gitlab_package_list", packages.ListInput{ProjectID: proj.pidOf()})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, p := range out.Packages {
		if p.Name == fixture.pkgName {
			return
		}
	}
	t.Fatalf("package %q not found in list", fixture.pkgName)
}

func assertIndividualPackageFileListed(ctx context.Context, t *testing.T, proj ProjectFixture, fixture packageLifecycleFixture, packageID int64) int64 {
	t.Helper()
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
	if out.Files[0].FileName != fixture.fileName {
		t.Fatalf("expected file %q, got %q", fixture.fileName, out.Files[0].FileName)
	}
	return out.Files[0].PackageFileID
}

func downloadIndividualPackage(ctx context.Context, t *testing.T, proj ProjectFixture, fixture packageLifecycleFixture) {
	t.Helper()
	outPath := t.TempDir() + "/downloaded.txt"
	out, err := callToolOn[packages.DownloadOutput](ctx, sess.individual, "gitlab_package_download", packages.DownloadInput{
		ProjectID:      proj.pidOf(),
		PackageName:    fixture.pkgName,
		PackageVersion: fixture.pkgVersion,
		FileName:       fixture.fileName,
		OutputPath:     outPath,
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if out.Size == 0 {
		t.Fatal("expected non-zero file size")
	}
	t.Logf("Downloaded %d bytes to %s", out.Size, out.OutputPath)
}

func deleteIndividualPackageFile(ctx context.Context, t *testing.T, proj ProjectFixture, packageID, packageFileID int64) {
	t.Helper()
	err := callToolVoidOn(ctx, sess.individual, "gitlab_package_file_delete", packages.FileDeleteInput{
		ProjectID:     proj.pidOf(),
		PackageID:     i64soi(packageID),
		PackageFileID: i64soi(packageFileID),
	})
	if err != nil {
		t.Fatalf("file delete: %v", err)
	}
}

func deleteIndividualPackage(ctx context.Context, t *testing.T, proj ProjectFixture, packageID int64) {
	t.Helper()
	err := callToolVoidOn(ctx, sess.individual, "gitlab_package_delete", packages.DeleteInput{
		ProjectID: proj.pidOf(),
		PackageID: i64soi(packageID),
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func testMetaPackageLifecycle(ctx context.Context, t *testing.T, projM ProjectFixture, fixture packageLifecycleFixture) {
	t.Helper()
	var mPkgID int64
	var mFileID int64

	t.Run("Meta/Publish", func(t *testing.T) {
		out, err := callToolOn[packages.PublishOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "publish",
			"params": map[string]any{
				"project_id":      projM.pidStr(),
				"package_name":    fixture.pkgName,
				"package_version": fixture.pkgVersion,
				"file_name":       fixture.fileName,
				"content_base64":  fixture.fileContent,
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
				"package_id": strconv.FormatInt(mPkgID, 10),
			},
		})
		if err != nil {
			t.Fatalf("meta file list: %v", err)
		}
		if len(out.Files) == 0 {
			t.Fatal("expected at least one file (meta)")
		}
		if sess.enterprise {
			mFileID = out.Files[0].PackageFileID
		}
	})

	t.Run("Meta/Download", func(t *testing.T) {
		tmpDir := t.TempDir()
		outPath := tmpDir + "/downloaded.txt"
		out, err := callToolOn[packages.DownloadOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "download",
			"params": map[string]any{
				"project_id":      projM.pidStr(),
				"package_name":    fixture.pkgName,
				"package_version": fixture.pkgVersion,
				"file_name":       fixture.fileName,
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
				"package_id":      strconv.FormatInt(mPkgID, 10),
				"package_file_id": strconv.FormatInt(mFileID, 10),
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
				"package_id": strconv.FormatInt(mPkgID, 10),
			},
		})
		if err != nil {
			t.Fatalf("meta delete: %v", err)
		}
	})
}
