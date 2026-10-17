//go:build e2e

// npm_package_test.go drives the npm package fixture against the stub: the
// publish document npm's client would send, the read-back that waits for the
// registry to list the package, and the cleanup that finds it by name.

package fixture

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1" //nolint:gosec // the test recomputes npm's dist.shasum
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The npm fixture's name under scope grp, and where the stub takes its
// publish: the project-level npm endpoint, with the scoped name as one
// escaped segment, which the stub reads back decoded.
const (
	npmFixtureName   = "@grp/" + npmPackageBaseName
	npmPublishRoute  = "/api/v4/projects/9/packages/npm/" + npmFixtureName
	npmPackagesRoute = "/api/v4/projects/9/packages"
)

// TestPublishNpmPackage_Published_SendsNpmsDocument checks that the publish
// is the document GitLab's npm endpoint takes, which is what makes the
// package exist.
func TestPublishNpmPackage_Published_SendsNpmsDocument(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPut, npmPublishRoute, stubOK(map[string]any{"message": "201 Created"}))

	if err := publishNpmPackage(t.Context(), client, 9, npmFixtureName); err != nil {
		t.Fatalf("publishNpmPackage() error = %v, want nil", err)
	}
	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("the stub was sent %d scripted requests, want the publish alone", len(requests))
	}
	assertNpmPublishDocument(t, requests[0].Body)
}

// TestPublishNpmPackage_Refused_IsTheError checks that a publish GitLab
// refuses is the fixture's error rather than a package it waits for.
func TestPublishNpmPackage_Refused_IsTheError(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPut, npmPublishRoute, stubRefusal(http.StatusForbidden, "403 Forbidden"))

	if err := publishNpmPackage(t.Context(), client, 9, npmFixtureName); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("publishNpmPackage() error = %v, want GitLab's refusal", err)
	}
}

// assertNpmPublishDocument holds the publish body to what
// Packages::Npm::CreatePackageService reads: the name, one version whose dist
// carries the tarball's SHA-1, a dist-tag, and the tarball under
// "<name>-<version>.tgz", whose package/package.json names the same package.
func assertNpmPublishDocument(t *testing.T, body map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("re-encoding the publish body: %v", err)
	}
	var document struct {
		Name     string `json:"name"`
		Versions map[string]struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Dist    struct {
				Shasum string `json:"shasum"`
			} `json:"dist"`
		} `json:"versions"`
		DistTags    map[string]string `json:"dist-tags"`
		Attachments map[string]struct {
			Data   string `json:"data"`
			Length int    `json:"length"`
		} `json:"_attachments"`
	}
	if err = json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("the publish body %s: %v", encoded, err)
	}
	version, known := document.Versions[NpmPackageVersion]
	if document.Name != npmFixtureName || !known || version.Name != npmFixtureName || version.Version != NpmPackageVersion {
		t.Errorf("the publish body names %q with versions %+v, want %s at %s", document.Name, document.Versions, npmFixtureName, NpmPackageVersion)
	}
	if document.DistTags["latest"] != NpmPackageVersion {
		t.Errorf("the publish body tags %v, want latest at %s", document.DistTags, NpmPackageVersion)
	}
	attachment, attached := document.Attachments[npmFixtureName+"-"+NpmPackageVersion+".tgz"]
	if !attached {
		t.Fatalf("the publish body attaches %v, want the tarball under %s-%s.tgz", document.Attachments, npmFixtureName, NpmPackageVersion)
	}
	tarball, err := base64.StdEncoding.DecodeString(attachment.Data)
	if err != nil || attachment.Length != len(tarball) {
		t.Fatalf("the attachment decodes to %d bytes (%v) and says %d", len(tarball), err, attachment.Length)
	}
	if sum := sha1.Sum(tarball); version.Dist.Shasum != hex.EncodeToString(sum[:]) { //nolint:gosec // see the import
		t.Errorf("dist.shasum = %s, want the SHA-1 of the attached tarball", version.Dist.Shasum)
	}
	if manifest := tarballManifest(t, tarball); manifest["name"] != npmFixtureName || manifest["version"] != NpmPackageVersion {
		t.Errorf("the tarball's package.json = %v, want %s at %s", manifest, npmFixtureName, NpmPackageVersion)
	}
}

// tarballManifest reads package/package.json out of a gzipped tarball.
func tarballManifest(t *testing.T, tarball []byte) map[string]any {
	t.Helper()
	zipped, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		t.Fatalf("the tarball is not gzipped: %v", err)
	}
	archive := tar.NewReader(zipped)
	header, err := archive.Next()
	if err != nil || header.Name != "package/package.json" {
		t.Fatalf("the tarball's first entry is %v (%v), want package/package.json", header, err)
	}
	raw, err := io.ReadAll(archive)
	if err != nil {
		t.Fatalf("reading package/package.json: %v", err)
	}
	var manifest map[string]any
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("package/package.json %s: %v", raw, err)
	}
	return manifest
}

// TestWaitForNpmPackage_ListedLater_IsTheFixtureVersion checks the read-back
// waits while GitLab still processes the package, which the registry does not
// list, and then takes the package of the fixture's name and version: not
// another version of it, and not a package whose name only contains the
// fixture's, which GitLab's package_name filter also matches.
func TestWaitForNpmPackage_ListedLater_IsTheFixtureVersion(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodGet, npmPackagesRoute, stubOK([]any{}), stubOK([]any{
		map[string]any{"id": 8, "name": npmFixtureName, "version": "0.9.0"},
		map[string]any{"id": 9, "name": npmFixtureName + "-extra", "version": NpmPackageVersion},
		map[string]any{"id": 21, "name": npmFixtureName, "version": NpmPackageVersion},
	}))

	got, err := waitForNpmPackage(t.Context(), client, 9, npmFixtureName, time.Millisecond, time.Second)
	if err != nil {
		t.Fatalf("waitForNpmPackage() error = %v, want nil", err)
	}
	if want := (NpmPackage{ID: 21, Name: npmFixtureName, Version: NpmPackageVersion}); got != want {
		t.Errorf("waitForNpmPackage() = %+v, want %+v", got, want)
	}
	requests := stub.recordedRequests()
	if len(requests) != 2 {
		t.Fatalf("the registry was read %d times, want twice", len(requests))
	}
	if query := requests[0].Query; query.Get("package_type") != "npm" || query.Get("package_name") != npmFixtureName {
		t.Errorf("the read-back asked %v, want npm packages named %s", query, npmFixtureName)
	}
}

// TestWaitForNpmPackage_Failures_AreReported checks the two ways the read-back
// gives up: a listing GitLab refuses, at once, and a package the registry
// never lists, once the wait is spent, each naming what it was waiting for.
func TestWaitForNpmPackage_Failures_AreReported(t *testing.T) {
	cases := []struct {
		name    string
		listing scriptedAnswer
		want    string
	}{
		{name: "listing refused", listing: stubRefusal(http.StatusInternalServerError, "500 Internal Server Error"), want: "listing the npm packages of project 9"},
		{name: "never listed", listing: stubOK([]any{}), want: npmFixtureName},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, npmPackagesRoute, tc.listing)

			_, err := waitForNpmPackage(t.Context(), client, 9, npmFixtureName, time.Millisecond, 20*time.Millisecond)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("waitForNpmPackage() error = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

// TestDeleteNpmPackages_DeletesEveryVersionOfTheName checks the cleanup the
// publish registers before it knows an ID: every version listed under the
// fixture's exact name is deleted, and a package whose name only contains it
// is left alone.
func TestDeleteNpmPackages_DeletesEveryVersionOfTheName(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodGet, npmPackagesRoute, stubOK([]any{
		map[string]any{"id": 8, "name": npmFixtureName, "version": "0.9.0"},
		map[string]any{"id": 9, "name": npmFixtureName + "-extra", "version": NpmPackageVersion},
		map[string]any{"id": 21, "name": npmFixtureName, "version": NpmPackageVersion},
	}))
	stub.answers(http.MethodDelete, npmPackagesRoute+"/8", stubNoContent())
	stub.answers(http.MethodDelete, npmPackagesRoute+"/21", stubNoContent())

	if err := deleteNpmPackages(t.Context(), client, 9, npmFixtureName); err != nil {
		t.Fatalf("deleteNpmPackages() error = %v, want nil", err)
	}
	var deleted []string
	for _, request := range stub.recordedRequests() {
		if request.Method == http.MethodDelete {
			deleted = append(deleted, request.Path)
		}
	}
	if want := []string{npmPackagesRoute + "/8", npmPackagesRoute + "/21"}; strings.Join(deleted, " ") != strings.Join(want, " ") {
		t.Errorf("deleted %v, want %v", deleted, want)
	}
}

// TestDeleteNpmPackages_Failures_AreReported checks that a cleanup that cannot
// list the packages, or cannot delete one, says so rather than reporting the
// group's deletion refused later for a package it left behind.
func TestDeleteNpmPackages_Failures_AreReported(t *testing.T) {
	t.Run("listing refused", func(t *testing.T) {
		stub, client := newStubGitLab(t)
		stub.answers(http.MethodGet, npmPackagesRoute, stubRefusal(http.StatusInternalServerError, "500 Internal Server Error"))

		if err := deleteNpmPackages(t.Context(), client, 9, npmFixtureName); err == nil || !strings.Contains(err.Error(), "listing") {
			t.Errorf("deleteNpmPackages() error = %v, want the listing's", err)
		}
	})
	t.Run("deletion refused", func(t *testing.T) {
		stub, client := newStubGitLab(t)
		stub.answers(http.MethodGet, npmPackagesRoute, stubOK([]any{
			map[string]any{"id": 21, "name": npmFixtureName, "version": NpmPackageVersion},
		}))
		stub.answers(http.MethodDelete, npmPackagesRoute+"/21", stubRefusal(http.StatusForbidden, "403 Forbidden"))

		if err := deleteNpmPackages(t.Context(), client, 9, npmFixtureName); err == nil || !strings.Contains(err.Error(), "deleting package 21") {
			t.Errorf("deleteNpmPackages() error = %v, want the deletion's", err)
		}
	})
}
