//go:build e2e

// package_release_test.go drives both halves of the package fixture: the
// local files a publish case uploads, and the publish-then-read-back that
// turns an upload into a package with an ID.

package fixture

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWritePackageFiles_Written_AreReadableAndListedInOrder checks that every
// declared file lands on disk with its content, and that the names and paths
// the fixture hands back describe what was written.
func TestWritePackageFiles_Written_AreReadableAndListedInOrder(t *testing.T) {
	dir := t.TempDir()

	got, err := writePackageFiles(dir)
	if err != nil {
		t.Fatalf("writePackageFiles() error = %v, want nil", err)
	}
	if len(got.Names) != len(packageFixtureFiles) || len(got.Paths) != len(packageFixtureFiles) {
		t.Fatalf("writePackageFiles() described %d names and %d paths, want %d of each",
			len(got.Names), len(got.Paths), len(packageFixtureFiles))
	}
	for i, file := range packageFixtureFiles {
		if got.Names[i] != file.name {
			t.Errorf("writePackageFiles() named file %d %q, want %q", i, got.Names[i], file.name)
		}
		if got.Paths[i] != filepath.Join(dir, file.name) {
			t.Errorf("writePackageFiles() placed file %d at %q, want it under %q", i, got.Paths[i], dir)
		}
		content, readErr := os.ReadFile(got.Paths[i])
		if readErr != nil {
			t.Fatalf("reading %s: %v", got.Paths[i], readErr)
		}
		if string(content) != file.content {
			t.Errorf("file %q holds %q, want %q", file.name, content, file.content)
		}
	}
	if got.Name != PackageName || got.Version != PackageVersion || got.Tag != PackageReleaseTag {
		t.Errorf("writePackageFiles() described the package as %s/%s (%s), want %s/%s (%s)",
			got.Name, got.Version, got.Tag, PackageName, PackageVersion, PackageReleaseTag)
	}
}

// TestPackageFilesDisplay_Names_RenderAsThePromptListsThem checks the one
// value a stimulus interpolates, so the directory and the sentence describing
// it cannot disagree.
func TestPackageFilesDisplay_Names_RenderAsThePromptListsThem(t *testing.T) {
	files := PackageFiles{Names: []string{"a.txt", "b.txt"}}
	if got := files.Display(); got != "a.txt, b.txt" {
		t.Errorf("Display() = %q, want %q", got, "a.txt, b.txt")
	}
}

// TestWritePackageFiles_UnwritableDirectory_NamesTheFile checks that a
// directory the fixture cannot write into is reported with the path, rather
// than handed back as an empty file list a case would upload nothing from.
func TestWritePackageFiles_UnwritableDirectory_NamesTheFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-directory")

	_, err := writePackageFiles(missing)
	if err == nil {
		t.Fatal("writePackageFiles() error = nil, want the unwritable directory reported")
	}
	if !strings.Contains(err.Error(), packageFixtureFiles[0].name) {
		t.Errorf("writePackageFiles() error = %q, want it to name the file it could not write", err)
	}
}

// publishPath is where the generic package endpoint takes the first fixture
// file.
const publishPath = "/api/v4/projects/9/packages/generic/" + PackageName + "/" + PackageVersion +
	"/gitlab-mcp-server-linux-amd64.txt"

// TestPublishPackage_Published_ReadsTheRegistryForTheID checks the read-back
// that turns an upload into a fixture: the publish answers about the file, and
// every package action takes the package's ID.
func TestPublishPackage_Published_ReadsTheRegistryForTheID(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPut, publishPath, stubCreated(map[string]any{"file_name": "gitlab-mcp-server-linux-amd64.txt"}))
	stub.answers(http.MethodGet, "/api/v4/projects/9/packages", stubOK([]any{
		map[string]any{"id": 2, "name": "something-else", "version": "9.9.9"},
		map[string]any{"id": 17, "name": PackageName, "version": PackageVersion},
	}))

	got, err := publishPackage(t.Context(), client, 9, "gitlab-mcp-server-linux-amd64.txt", "bytes\n")
	if err != nil {
		t.Fatalf("publishPackage() error = %v, want nil", err)
	}
	want := Package{ID: 17, Name: PackageName, Version: PackageVersion}
	if got != want {
		t.Errorf("publishPackage() = %+v, want %+v", got, want)
	}
}

// TestPublishPackage_NotInTheRegistry_IsReported checks that a registry which
// does not hold the package after the upload is a failure here rather than in
// the case that depended on it.
func TestPublishPackage_NotInTheRegistry_IsReported(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPut, publishPath, stubCreated(map[string]any{"file_name": "gitlab-mcp-server-linux-amd64.txt"}))
	stub.answers(http.MethodGet, "/api/v4/projects/9/packages", stubOK([]any{}))

	_, err := publishPackage(t.Context(), client, 9, "gitlab-mcp-server-linux-amd64.txt", "bytes\n")
	if err == nil {
		t.Fatal("publishPackage() error = nil, want the missing package reported")
	}
	if !strings.Contains(err.Error(), PackageName) {
		t.Errorf("publishPackage() error = %q, want it to name the package", err)
	}
}

// TestDeletePackage_Endings_ToleratesOneACaseDeleted checks that a package a
// case already deleted is not a cleanup failure and any other refusal is.
func TestDeletePackage_Endings_ToleratesOneACaseDeleted(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "deleted", answer: stubNoContent()},
		{name: "already gone", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/projects/9/packages/17", tc.answer)

			err := deletePackage(t.Context(), client, 9, 17)
			if (err != nil) != tc.wantErr {
				t.Errorf("deletePackage() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
