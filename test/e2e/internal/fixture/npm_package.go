// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// npm_package.go publishes an npm package to a project's registry, which is
// what a scenario needs to make GitLab refuse to move the project to another
// top-level namespace: the transfer worker fails a move that would leave an
// npm package scoped to the old top-level namespace behind
// (Projects::TransferService#project_has_namespaced_npm_packages?), and from
// GitLab 19.4 that failure happens in the background.

package fixture

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1" //nolint:gosec // npm's dist.shasum is a SHA-1 of the tarball, which GitLab stores as the file's sha1
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// NpmPackageVersion is the version every npm package fixture is published
// at. Each fixture has a project of its own, so a literal cannot collide.
const NpmPackageVersion = "1.0.0"

// npmPackageBaseName is the unscoped half of an npm package fixture's name.
const npmPackageBaseName = "e2e-transfer-package"

// How long the read-back waits for the package to be listed, and how often it
// asks. GitLab creates the package as processing and lists it only once its
// worker has read the tarball, which an idle instance does in a second or two.
const (
	npmPackageWait         = 2 * time.Minute
	npmPackagePollInterval = time.Second
)

// NpmPackage is an npm package a fixture published to a project's registry.
type NpmPackage struct {
	ID      int64
	Name    string
	Version string
}

// NewNpmPackage publishes an npm package named @scope/e2e-transfer-package to
// the project's registry through the project-level npm endpoint, registers its
// deletion, and waits for the registry to list it.
//
// The deletion is registered as soon as the publish is answered, before the
// read-back: GitLab holds the package from that moment, and a group whose
// project holds one scoped to it refuses its own deletion ("Group URL cannot
// change when group contains projects with NPM packages"), so a fixture that
// failed between the two would strand its group. The package counts as scoped
// to scope while it is still processing, so the transfer check sees it either
// way.
func NewNpmPackage(e *harness.Env, project Project, scope string) NpmPackage {
	e.T.Helper()

	name := "@" + scope + "/" + npmPackageBaseName
	if _, err := retryTransient(e, "publish npm package "+name, createRetries, func() (struct{}, error) {
		return struct{}{}, publishNpmPackage(e.Ctx, e.Client(), project.ID, name)
	}); err != nil {
		e.T.Fatalf("publishing npm package %q to project %d: %v", name, project.ID, err)
	}
	e.Defer("npm package "+name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteNpmPackages(ctx, e.Client(), project.ID, name)
	})

	pkg, err := waitForNpmPackage(e.Ctx, e.Client(), project.ID, name, npmPackagePollInterval, npmPackageWait)
	if err != nil {
		e.T.Fatalf("reading npm package %q of project %d back: %v", name, project.ID, err)
	}
	return pkg
}

// publishNpmPackage sends the npm client's publish document for name to the
// project's npm endpoint.
func publishNpmPackage(ctx context.Context, client *gitlabclient.Client, projectID int64, name string) error {
	document, err := npmPublishDocument(name, NpmPackageVersion)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("projects/%d/packages/npm/%s", projectID, gl.PathEscape(name))
	req, err := client.GL().NewRequest(http.MethodPut, path, document, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return fmt.Errorf("building the npm publish of %s: %w", name, err)
	}
	_, err = client.GL().Do(req, nil)
	return err
}

// npmPublishDocument is the body the npm client sends to publish one version
// of a package: the version's manifest with the tarball's SHA-1, the tag that
// points at it, and the tarball itself, base64-encoded and keyed by the file
// name GitLab looks it up under (Packages::Npm::CreatePackageService reads
// "<name>-<version>.tgz").
func npmPublishDocument(name, version string) (map[string]any, error) {
	manifest := map[string]any{"name": name, "version": version}
	tarball, err := npmTarball(manifest)
	if err != nil {
		return nil, err
	}
	sum := sha1.Sum(tarball) //nolint:gosec // see the import
	file := name + "-" + version + ".tgz"
	return map[string]any{
		"name": name,
		"versions": map[string]any{version: map[string]any{
			"name":    name,
			"version": version,
			"dist":    map[string]any{"shasum": hex.EncodeToString(sum[:]), "tarball": file},
		}},
		"dist-tags": map[string]any{"latest": version},
		"_attachments": map[string]any{file: map[string]any{
			"content_type": "application/octet-stream",
			"data":         base64.StdEncoding.EncodeToString(tarball),
			"length":       len(tarball),
		}},
	}, nil
}

// npmTarball is a package tarball as npm packs it: package/package.json
// holding the manifest, gzipped. GitLab reads that file back when it
// processes the package, and a manifest that names another package or version
// marks it failed, so the tarball says what the publish document says.
func npmTarball(manifest map[string]any) ([]byte, error) {
	packageJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encoding package.json: %w", err)
	}
	var buf bytes.Buffer
	zipped := gzip.NewWriter(&buf)
	archive := tar.NewWriter(zipped)
	header := &tar.Header{Name: "package/package.json", Mode: 0o644, Size: int64(len(packageJSON)), Typeflag: tar.TypeReg}
	if err = archive.WriteHeader(header); err != nil {
		return nil, fmt.Errorf("writing the tarball: %w", err)
	}
	if _, err = archive.Write(packageJSON); err != nil {
		return nil, fmt.Errorf("writing the tarball: %w", err)
	}
	if err = archive.Close(); err != nil {
		return nil, fmt.Errorf("closing the tarball: %w", err)
	}
	if err = zipped.Close(); err != nil {
		return nil, fmt.Errorf("compressing the tarball: %w", err)
	}
	return buf.Bytes(), nil
}

// errNpmPackageNotListed is what a read-back says while the registry does not
// list the package yet.
var errNpmPackageNotListed = errors.New("not listed yet")

// waitForNpmPackage reads the project's npm packages every interval until the
// fixture's is listed, for up to wait. A listing GitLab refuses ends the wait
// with its error.
func waitForNpmPackage(ctx context.Context, client *gitlabclient.Client, projectID int64, name string, interval, wait time.Duration) (NpmPackage, error) {
	var found NpmPackage
	err := harness.Poll(ctx, interval, wait, func() (bool, string, error) {
		pkg, err := findNpmPackage(ctx, client, projectID, name)
		if errors.Is(err, errNpmPackageNotListed) {
			return false, err.Error(), nil
		}
		found = pkg
		return err == nil, "", err
	})
	return found, err
}

// findNpmPackage reads the project's npm packages named name and returns the
// one at the fixture's version, or errNpmPackageNotListed.
func findNpmPackage(ctx context.Context, client *gitlabclient.Client, projectID int64, name string) (NpmPackage, error) {
	packages, err := listNpmPackages(ctx, client, projectID, name)
	if err != nil {
		return NpmPackage{}, err
	}
	for _, pkg := range packages {
		if pkg.Version == NpmPackageVersion {
			return NpmPackage{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version}, nil
		}
	}
	return NpmPackage{}, fmt.Errorf("project %d: npm package %s %s: %w", projectID, name, NpmPackageVersion, errNpmPackageNotListed)
}

// listNpmPackages returns the project's npm packages whose name is exactly
// name: GitLab's package_name filter matches a part of the name.
func listNpmPackages(ctx context.Context, client *gitlabclient.Client, projectID int64, name string) ([]*gl.Package, error) {
	packages, _, err := client.GL().Packages.ListProjectPackages(projectID, &gl.ListProjectPackagesOptions{
		PerPage:     packageListPageSize,
		PackageType: new("npm"),
		PackageName: new(name),
	}, gl.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("listing the npm packages of project %d: %w", projectID, err)
	}
	named := make([]*gl.Package, 0, len(packages))
	for _, pkg := range packages {
		if pkg != nil && pkg.Name == name {
			named = append(named, pkg)
		}
	}
	return named, nil
}

// deleteNpmPackages deletes every npm package of the project named name,
// whatever its version, which is the cleanup a publish registers before it
// knows the package's ID. A package GitLab is still processing is neither
// listed nor deleted, and neither can be: at v19.5.0-ee the listing and the
// delete route both read only the packages GitLab displays (default, error
// and deprecated: Packages::FinderHelper#filter_by_status and
// Packages::PackageFinder), so its ID could not be deleted even if a listing
// by status named it. Nor does it stop the group's deletion, whose npm check
// (Groups::UpdateService, through Packages::PackagesFinder) reads the same
// set, which is what registering the cleanup early protects.
func deleteNpmPackages(ctx context.Context, client *gitlabclient.Client, projectID int64, name string) error {
	packages, err := listNpmPackages(ctx, client, projectID, name)
	if err != nil {
		return err
	}
	for _, pkg := range packages {
		if deleteErr := deletePackage(ctx, client, projectID, pkg.ID); deleteErr != nil {
			return deleteErr
		}
	}
	return nil
}
