// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package goprogram

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestIgnoredGoFiles_KeepsTheGoFilesInACopyOfTheirOwn verifies only the Go
// files a load left out are returned, in the loader's order, and that the
// answer is a slice of the caller's own: narrowing it in place leaves the
// package the loader returned as it was.
func TestIgnoredGoFiles_KeepsTheGoFilesInACopyOfTheirOwn(t *testing.T) {
	pkg := &packages.Package{IgnoredFiles: []string{"/p/b_windows.go", "/p/asm_arm64.s", "/p/a_test.go"}}
	got := IgnoredGoFiles(pkg)
	if want := []string{"/p/b_windows.go", "/p/a_test.go"}; !slices.Equal(got, want) {
		t.Fatalf("IgnoredGoFiles = %v, want %v", got, want)
	}
	got[0] = "/p/changed.go"
	if pkg.IgnoredFiles[0] != "/p/b_windows.go" {
		t.Errorf("IgnoredFiles = %v after the answer was changed, want the loader's list untouched", pkg.IgnoredFiles)
	}
	if none := IgnoredGoFiles(&packages.Package{}); len(none) != 0 {
		t.Errorf("IgnoredGoFiles of a package that left nothing out = %v, want none", none)
	}
}

// TestFilesImporting_KeepsWhatImportsThePathAndWhatCannotBeRead verifies which
// hidden files are named: one that imports the path under its own name, under
// another name and as a raw string, and one that cannot be read, either
// because it is not there or because its imports do not parse; and not one
// importing something else, nor one importing a package whose path only
// begins like it. Every file but the missing one is served from the overlay,
// the way a test's own source is, and the answer keeps the order given.
func TestFilesImporting_KeepsWhatImportsThePathAndWhatCannotBeRead(t *testing.T) {
	root := t.TempDir()
	path := func(name string) string { return filepath.Join(root, name) }
	overlay := map[string][]byte{
		path("quiet.go"):     []byte("package p\n\nimport \"testing\"\n"),
		path("sub.go"):       []byte("package p\n\nimport \"example.com/served/sub\"\n"),
		path("plain.go"):     []byte("package p\n\nimport \"example.com/served\"\n"),
		path("aliased.go"):   []byte("package p\n\nimport other \"example.com/served\"\n\nvar _ = other.X\n"),
		path("raw.go"):       []byte("package p\n\nimport `example.com/served`\n"),
		path("broken.go"):    []byte("package p\n\nimport (\n"),
		path("unrelated.go"): []byte("package p\n"),
	}
	given := []string{
		path("quiet.go"), path("sub.go"), path("plain.go"), path("aliased.go"),
		path("raw.go"), path("broken.go"), path("missing.go"), path("unrelated.go"),
	}
	got := FilesImporting(given, "example.com/served", overlay)
	want := []string{path("plain.go"), path("aliased.go"), path("raw.go"), path("broken.go"), path("missing.go")}
	if !slices.Equal(got, want) {
		t.Errorf("FilesImporting =\n%v\nwant\n%v", got, want)
	}
	if none := FilesImporting(nil, "example.com/served", overlay); len(none) != 0 {
		t.Errorf("FilesImporting of no files = %v, want none", none)
	}
}

// TestImports_ReadsThePathNotTheName verifies the import is recognized by its
// path under any name, a raw string included, and that a path that only
// begins like it, or names another package, is not it.
func TestImports_ReadsThePathNotTheName(t *testing.T) {
	for _, tt := range []struct {
		name string
		path string
		want bool
	}{
		{"quoted", `"example.com/served"`, true},
		{"raw", "`example.com/served`", true},
		{"a subpackage", `"example.com/served/sub"`, false},
		{"something else", `"context"`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			file := &ast.File{Imports: []*ast.ImportSpec{
				{Path: &ast.BasicLit{Kind: token.STRING, Value: `"fmt"`}},
				{Path: &ast.BasicLit{Kind: token.STRING, Value: tt.path}},
			}}
			if got := imports(file, "example.com/served"); got != tt.want {
				t.Errorf("imports(%s) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
