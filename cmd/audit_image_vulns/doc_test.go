// doc_test.go holds the command to what its package comment promises about
// what it is built from.
package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestImports_AreTheStandardLibraryAlone: the release workflow runs this
// command in the docker job, which holds the registry push, so any code it
// builds runs with that credential, and cmd/audit_supply_chain refuses a Go
// command there that reaches a module other than this one. The command goes
// further and imports nothing outside the standard library, so it reaches no
// package of this module either and nothing a change elsewhere in the tree
// brings in. A standard library path has no dot in its first element; every
// module path does.
func TestImports_AreTheStandardLibraryAlone(t *testing.T) {
	t.Parallel()

	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	read := 0
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		body, readErr := os.ReadFile(source)
		if readErr != nil {
			t.Fatalf("read %s: %v", source, readErr)
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), source, body, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", source, parseErr)
		}
		read++
		for _, spec := range file.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			if first, _, _ := strings.Cut(path, "/"); strings.Contains(first, ".") {
				t.Errorf("%s imports %s, which is not the standard library", source, path)
			}
		}
	}
	if read == 0 {
		t.Fatal("no source file was read, so nothing was checked")
	}
}
