//go:build e2e

// package_release.go builds the two halves of the package workflow: the local
// files a publish case uploads, and a package already in the registry for the
// cases that read or delete one.
//
// The local files go into the test's own temporary directory rather than into
// a directory under the repository, which is what the evaluator this replaces
// did. A path under the checkout is shared by every attempt running at once,
// so two of them would write the same bytes to the same place and a case
// asserting on a file's content would be asserting about whichever wrote last;
// and the tool that reads it enforces an allowlist of directories, of which
// the OS temporary directory is always one.

package fixture

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// What a package fixture is called in the registry. The name and version are
// literals rather than run-scoped, because a generic package is addressed by
// name and version inside one project and every attempt gets a project of its
// own.
const (
	PackageName    = "e2e-release-package"
	PackageVersion = "0.1.0"
	// PackageReleaseTag is the tag a publish case attaches the package to,
	// which is a tag of the project's own and so may also be a literal.
	PackageReleaseTag = "v0.0.0-e2e-packages"
)

// packageListPageSize is how many packages one registry page carries. A
// fixture project holds one; the page size is what keeps the read-back from
// paging on a project a case has published several to.
const packageListPageSize = 100

// packageFixtureFiles are the files a publish case uploads, small enough that
// the upload is about the request rather than about the bytes.
var packageFixtureFiles = []struct {
	name    string
	content string
}{
	{name: "gitlab-mcp-server-linux-amd64.txt", content: "linux amd64 e2e package\n"},
	{name: "gitlab-mcp-server-darwin-arm64.txt", content: "darwin arm64 e2e package\n"},
	{name: "checksums.txt", content: "sha256  gitlab-mcp-server-linux-amd64.txt\nsha256  gitlab-mcp-server-darwin-arm64.txt\n"},
}

// PackageFiles is the local directory a publish case uploads from.
type PackageFiles struct {
	// Dir is the directory holding them all, which a directory upload takes.
	Dir string
	// Names are the file names inside it, in the order they were written.
	Names []string
	// Paths are the absolute paths of the same files, which a single-file
	// upload takes.
	Paths []string
	// Name, Version and Tag are what the package is published as.
	Name    string
	Version string
	Tag     string
}

// Display renders the file names the way a prompt lists them, so the stimulus
// and the directory cannot disagree about what is in it.
func (p PackageFiles) Display() string { return strings.Join(p.Names, ", ") }

// Package is a generic package already in a project's registry.
type Package struct {
	// ID is what the package actions take.
	ID int64
	// Name and Version are how the registry addresses it.
	Name    string
	Version string
}

// NewPackageFiles writes the fixture files into the test's own temporary
// directory and returns where they went.
//
// Nothing is registered: the directory is the testing package's, and it
// removes it when the test ends.
func NewPackageFiles(e *harness.Env) PackageFiles {
	e.T.Helper()

	dir := e.T.TempDir()
	files, err := writePackageFiles(dir)
	if err != nil {
		e.T.Fatalf("writing the package fixture files: %v", err)
	}
	return files
}

// writePackageFiles writes each fixture file into dir and returns what was
// written.
func writePackageFiles(dir string) (PackageFiles, error) {
	files := PackageFiles{Dir: dir, Name: PackageName, Version: PackageVersion, Tag: PackageReleaseTag}
	for _, file := range packageFixtureFiles {
		path := filepath.Join(dir, file.name)
		if err := os.WriteFile(path, []byte(file.content), 0o600); err != nil {
			return PackageFiles{}, fmt.Errorf("writing %s: %w", path, err)
		}
		files.Names = append(files.Names, file.name)
		files.Paths = append(files.Paths, path)
	}
	return files, nil
}

// NewPackage publishes one generic package file to the project's registry and
// registers the package's deletion.
//
// It publishes one file rather than all three: what the cases that read or
// delete a package need is a package with an ID, and each extra file is
// another upload the fixture waits for.
func NewPackage(e *harness.Env, project Project) Package {
	e.T.Helper()

	first := packageFixtureFiles[0]
	pkg, err := retryTransient(e, "publish package "+PackageName, createRetries, func() (Package, error) {
		return publishPackage(e.Ctx, e.Client(), project.ID, first.name, first.content)
	})
	if err != nil {
		e.T.Fatalf("publishing package %q to project %d: %v", PackageName, project.ID, err)
	}

	e.Defer(fmt.Sprintf("package %d", pkg.ID), func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deletePackage(ctx, e.Client(), project.ID, pkg.ID)
	})
	return pkg
}

// publishPackage uploads the file and reads the registry back for the package
// it landed in.
//
// The read-back is what turns an upload into a fixture: the publish endpoint
// answers about the file rather than about the package, and the package ID is
// what every package action takes.
func publishPackage(ctx context.Context, client *gitlabclient.Client, projectID int64, fileName, content string) (Package, error) {
	_, _, err := client.GL().GenericPackages.PublishPackageFile(projectID, PackageName, PackageVersion, fileName,
		bytes.NewReader([]byte(content)), nil, gl.WithContext(ctx))
	if err != nil {
		return Package{}, err
	}
	return findPackage(ctx, client, projectID)
}

// findPackage reads the project's registry and returns the fixture's package.
func findPackage(ctx context.Context, client *gitlabclient.Client, projectID int64) (Package, error) {
	packages, _, err := client.GL().Packages.ListProjectPackages(projectID,
		&gl.ListProjectPackagesOptions{PerPage: packageListPageSize}, gl.WithContext(ctx))
	if err != nil {
		return Package{}, fmt.Errorf("listing the packages of project %d: %w", projectID, err)
	}
	for _, pkg := range packages {
		if pkg != nil && pkg.Name == PackageName && pkg.Version == PackageVersion {
			return Package{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version}, nil
		}
	}
	return Package{}, fmt.Errorf("package %s/%s is not in the registry of project %d after being published",
		PackageName, PackageVersion, projectID)
}

// deletePackage removes the package and tolerates one a case deleted.
func deletePackage(ctx context.Context, client *gitlabclient.Client, projectID, packageID int64) error {
	_, err := client.GL().Packages.DeleteProjectPackage(projectID, packageID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting package %d of project %d: %w", packageID, projectID, err)
	}
	return nil
}
