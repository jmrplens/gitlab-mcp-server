package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readFixtureFile returns the content of a file in a fixture directory.
func readFixtureFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name)) //#nosec G304 -- test fixture path
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// TestMovePackageDoc_MovesTheCommentIntoDocGo verifies the mechanical move:
// the package comment lands verbatim in a new doc.go above its own package
// clause, the file it came from keeps its build constraint and loses only the
// comment, and the audit is satisfied afterwards.
func TestMovePackageDoc_MovesTheCommentIntoDocGo(t *testing.T) {
	pkg := writePackageFixture(t, "sample", map[string]string{
		"sample.go": "//go:build linux\n\n// Package sample provides a fixture.\n//\n// It has a second paragraph.\npackage sample\n\n// Exported is documented.\nfunc Exported() {}\n",
		"other.go":  "package sample\n",
	})

	if err := movePackageDoc(pkg.Dir); err != nil {
		t.Fatalf("movePackageDoc() error = %v", err)
	}

	if got, want := readFixtureFile(t, pkg.Dir, "doc.go"), "// Package sample provides a fixture.\n//\n// It has a second paragraph.\npackage sample\n"; got != want {
		t.Errorf("doc.go = %q, want %q", got, want)
	}
	if got, want := readFixtureFile(t, pkg.Dir, "sample.go"), "//go:build linux\n\npackage sample\n\n// Exported is documented.\nfunc Exported() {}\n"; got != want {
		t.Errorf("sample.go = %q, want %q", got, want)
	}
	findings, err := auditPackage(pkg, false)
	if err != nil {
		t.Fatalf("auditPackage() error = %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("auditPackage() after the move = %#v, want none", findings)
	}
}

// TestMovePackageDoc_LeavesWhatItShould verifies the cases the mover must
// not touch: a package that already has a doc.go, a package with no package
// comment at all, a comment that is malformed (the audit reports it instead),
// and a comment that sits in a test file.
func TestMovePackageDoc_LeavesWhatItShould(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
	}{
		{name: "doc.go already exists", files: map[string]string{
			"doc.go":    "// Package sample provides a fixture.\npackage sample\n",
			"sample.go": "// Package sample is duplicated here.\npackage sample\n",
		}},
		{name: "no package comment", files: map[string]string{
			"sample.go": "package sample\n",
		}},
		{name: "malformed comment stays for the audit", files: map[string]string{
			"sample.go": "// sample.go is a file header.\npackage sample\n",
		}},
		{name: "a comment in a test file is not the package's", files: map[string]string{
			"sample.go":      "package sample\n",
			"sample_test.go": "// Package sample is documented in a test.\npackage sample\n",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := writePackageFixture(t, "sample", tt.files)
			before := map[string]string{}
			for name := range tt.files {
				before[name] = readFixtureFile(t, pkg.Dir, name)
			}
			if err := movePackageDoc(pkg.Dir); err != nil {
				t.Fatalf("movePackageDoc() error = %v", err)
			}
			for name, content := range before {
				if got := readFixtureFile(t, pkg.Dir, name); got != content {
					t.Errorf("%s changed to %q", name, got)
				}
			}
			if _, hasDoc := tt.files["doc.go"]; !hasDoc {
				if _, err := os.Stat(filepath.Join(pkg.Dir, "doc.go")); err == nil {
					t.Errorf("doc.go was created")
				}
			}
		})
	}
}

// TestMovePackageDoc_DryRunWritesNothing verifies --dry-run reports the move
// and leaves both files as they were.
func TestMovePackageDoc_DryRunWritesNothing(t *testing.T) {
	pkg := writePackageFixture(t, "sample", map[string]string{
		"sample.go": "// Package sample provides a fixture.\npackage sample\n",
	})
	dryRun = true
	t.Cleanup(func() { dryRun = false })

	if err := movePackageDoc(pkg.Dir); err != nil {
		t.Fatalf("movePackageDoc() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(pkg.Dir, "doc.go")); err == nil {
		t.Errorf("doc.go was created under --dry-run")
	}
	if got := readFixtureFile(t, pkg.Dir, "sample.go"); !strings.HasPrefix(got, "// Package sample") {
		t.Errorf("sample.go changed under --dry-run: %q", got)
	}
}

// TestMovePackageDocs_WalksDirectoriesAndSkipsTestdata verifies the walker
// reaches nested packages, takes a file path as its directory, and leaves
// testdata trees alone.
func TestMovePackageDocs_WalksDirectoriesAndSkipsTestdata(t *testing.T) {
	root := t.TempDir()
	writeAuditFile(t, root, "a/a.go", "// Package a is nested.\npackage a\n")
	writeAuditFile(t, root, "a/deeper/d.go", "// Package deeper is deeper.\npackage deeper\n")
	writeAuditFile(t, root, "a/testdata/t.go", "// Package t is a fixture.\npackage t\n")

	if err := movePackageDocs(root); err != nil {
		t.Fatalf("movePackageDocs() error = %v", err)
	}
	for _, dir := range []string{"a", "a/deeper"} {
		t.Run(dir, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir), "doc.go")); err != nil {
				t.Errorf("%s/doc.go missing: %v", dir, err)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, "a", "testdata", "doc.go")); err == nil {
		t.Errorf("testdata got a doc.go")
	}

	if err := movePackageDocs(filepath.Join(root, "missing")); err == nil {
		t.Errorf("movePackageDocs(missing) error = nil, want stat failure")
	}
	writeAuditFile(t, root, "b/b.go", "// Package b is reached through a file path.\npackage b\n")
	if err := movePackageDocs(filepath.Join(root, "b", "b.go")); err != nil {
		t.Fatalf("movePackageDocs(file) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "b", "doc.go")); err != nil {
		t.Errorf("b/doc.go missing after a file path: %v", err)
	}
}
