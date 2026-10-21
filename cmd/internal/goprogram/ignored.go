package goprogram

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"golang.org/x/tools/go/packages"
)

// IgnoredGoFiles returns the Go files a load left out of a package, which is
// what a build constraint the load did not satisfy looks like from the
// platform and the tags it was made under. A file that is not Go, such as an
// assembly file a constraint left out beside them, is not part of the answer.
//
// The slice is the caller's own, so a gate may narrow it in place without
// touching the package the loader returned.
func IgnoredGoFiles(pkg *packages.Package) []string {
	var ignored []string
	for _, path := range pkg.IgnoredFiles {
		if strings.HasSuffix(path, ".go") {
			ignored = append(ignored, path)
		}
	}
	return ignored
}

// FilesImporting returns, in the order given, the files among paths that
// import importPath under any name, and the ones that cannot be read at all.
//
// It is how a gate says what a load hid from it. A file a build constraint
// left out holds source the gate never judged, and a gate that said nothing
// about it would be clean over source it never read; but most such files hold
// nothing the gate looks for, and naming every one would bury the few that
// matter. Only the imports are read, which is all the question needs: whether
// the hidden file could hold what the gate would judge. A file that cannot be
// read is kept, since nothing can be said about what it holds. A file the
// overlay supplies is read from the overlay, as the load itself read it.
func FilesImporting(paths []string, importPath string, overlay map[string][]byte) []string {
	var importing []string
	for _, path := range paths {
		var src any
		if content, overlaid := overlay[path]; overlaid {
			src = content
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ImportsOnly)
		if err == nil && !imports(file, importPath) {
			continue
		}
		importing = append(importing, path)
	}
	return importing
}

// imports reports whether a file imports the package at importPath, under any
// name: the path is read off the import, never the name it is bound to, and a
// raw string literal names it as much as a quoted one.
func imports(file *ast.File, importPath string) bool {
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, "\"`") == importPath {
			return true
		}
	}
	return false
}
