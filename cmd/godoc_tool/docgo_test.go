package main

import (
	"errors"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildHolder parses src as a package file and returns the holder moveToDocGo
// operates on, for the failure branches reached only by driving it directly.
func buildHolder(t *testing.T, path, src string) *packageDocHolder {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return &packageDocHolder{path: path, file: file, src: []byte(src), fset: fset}
}

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

// TestMovePackageDoc_ReportsErrors verifies the mover surfaces the two failures
// it can meet on well-formed input rather than swallowing them: a directory it
// cannot read, and a source file it cannot parse.
func TestMovePackageDoc_ReportsErrors(t *testing.T) {
	t.Run("a directory that does not exist", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nope")
		if err := movePackageDoc(missing); err == nil {
			t.Error("movePackageDoc(missing) error = nil, want a readdir failure")
		}
	})

	t.Run("a source file that does not parse", func(t *testing.T) {
		pkg := writePackageFixture(t, "broken", map[string]string{
			"broken.go": "package broken\nfunc (\n",
		})
		if err := movePackageDoc(pkg.Dir); err == nil {
			t.Error("movePackageDoc() over an unparseable file = nil, want a parse failure")
		}
	})
}

// TestMovePackageDoc_SkipsFilesOnceTheHolderIsFound covers the branch a second
// source file takes after an earlier one has already supplied the comment: the
// comment-bearing file sorts first, so the holder is set before the second file
// is reached and that file is passed over rather than read again.
func TestMovePackageDoc_SkipsFilesOnceTheHolderIsFound(t *testing.T) {
	pkg := writePackageFixture(t, "sample", map[string]string{
		"aaa.go": "// Package sample is documented here.\npackage sample\n",
		"zzz.go": "package sample\n\n// More is more.\nfunc More() {}\n",
	})
	if err := movePackageDoc(pkg.Dir); err != nil {
		t.Fatalf("movePackageDoc() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(pkg.Dir, "doc.go")); err != nil {
		t.Errorf("doc.go missing: %v", err)
	}
}

// TestMovePackageDoc_ReportsAFileItCannotRead covers the read-failure branch
// with a broken symlink, which os.ReadFile fails to follow even for root, where
// a permission bit would not.
func TestMovePackageDoc_ReportsAFileItCannotRead(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "broken.go")); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}
	if err := movePackageDoc(dir); err == nil {
		t.Error("movePackageDoc() over an unreadable file = nil, want a read failure")
	}
}

// TestMoveToDocGo_ReportsAMalformedPosition covers the guard against a package
// comment that does not sit above the package clause. A real parse never
// produces that, so the holder's source is truncated to put the package offset
// past the end of it.
func TestMoveToDocGo_ReportsAMalformedPosition(t *testing.T) {
	h := buildHolder(t, "sample.go", "// Package sample is documented.\npackage sample\n")
	h.src = h.src[:2]
	if err := h.moveToDocGo(t.TempDir()); err == nil {
		t.Error("moveToDocGo() with a truncated source = nil, want a position failure")
	}
}

// TestMoveToDocGo_ReportsFormatFailures covers both format steps, which cannot
// fail on bytes just parsed, through the formatSource seam.
func TestMoveToDocGo_ReportsFormatFailures(t *testing.T) {
	const src = "// Package sample is documented.\npackage sample\n"
	t.Run("formatting doc.go", func(t *testing.T) {
		h := buildHolder(t, "sample.go", src)
		formatSource = func([]byte) ([]byte, error) { return nil, errors.New("boom") }
		t.Cleanup(func() { formatSource = format.Source })
		if err := h.moveToDocGo(t.TempDir()); err == nil {
			t.Error("moveToDocGo() with the first format failing = nil")
		}
	})
	t.Run("formatting the holder without its comment", func(t *testing.T) {
		h := buildHolder(t, "sample.go", src)
		calls := 0
		formatSource = func(b []byte) ([]byte, error) {
			calls++
			if calls == 2 {
				return nil, errors.New("boom")
			}
			return format.Source(b)
		}
		t.Cleanup(func() { formatSource = format.Source })
		if err := h.moveToDocGo(t.TempDir()); err == nil {
			t.Error("moveToDocGo() with the second format failing = nil")
		}
	})
}

// TestMoveToDocGo_ReportsWriteFailures covers both writes, which a directory the
// test owns and runs as root never refuses, through the writeFile seam.
func TestMoveToDocGo_ReportsWriteFailures(t *testing.T) {
	const src = "// Package sample is documented.\npackage sample\n"
	for _, tc := range []struct {
		name   string
		failOn int
	}{
		{name: "writing doc.go", failOn: 1},
		{name: "writing the holder", failOn: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := buildHolder(t, filepath.Join(t.TempDir(), "sample.go"), src)
			calls := 0
			writeFile = func(name string, data []byte, perm fs.FileMode) error {
				calls++
				if calls == tc.failOn {
					return errors.New("boom")
				}
				return os.WriteFile(name, data, perm)
			}
			t.Cleanup(func() { writeFile = os.WriteFile })
			if err := h.moveToDocGo(t.TempDir()); err == nil {
				t.Errorf("moveToDocGo() with write %d failing = nil", tc.failOn)
			}
		})
	}
}

// TestMovePackageDocs_ReportsAWalkError covers the branch that surfaces an error
// the walk itself hands the callback, which a readable tree never produces, by
// driving the walk seam to report one.
func TestMovePackageDocs_ReportsAWalkError(t *testing.T) {
	walkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, errors.New("boom"))
	}
	t.Cleanup(func() { walkDir = filepath.WalkDir })
	if err := movePackageDocs(t.TempDir()); err == nil {
		t.Error("movePackageDocs() with a failing walk = nil, want the walk error surfaced")
	}
}

// TestMovePackageDocs_CollectsErrorsFromTheWalk verifies the walker does not
// stop at the first package it cannot process: a broken file in one directory
// is reported, and the joined error the walk returns carries it.
func TestMovePackageDocs_CollectsErrorsFromTheWalk(t *testing.T) {
	root := t.TempDir()
	writeAuditFile(t, root, "good/g.go", "// Package good is fine.\npackage good\n")
	writeAuditFile(t, root, "bad/b.go", "package bad\nfunc (\n")

	err := movePackageDocs(root)
	if err == nil {
		t.Fatal("movePackageDocs() error = nil, want the broken package reported")
	}
	if !strings.Contains(err.Error(), filepath.Join("bad", "b.go")) {
		t.Errorf("the error does not name the broken file: %v", err)
	}
	// The reachable package was still processed: the walk went on past the
	// failure rather than aborting at it.
	if _, statErr := os.Stat(filepath.Join(root, "good", "doc.go")); statErr != nil {
		t.Errorf("good/doc.go missing, so the walk stopped at the broken package: %v", statErr)
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
