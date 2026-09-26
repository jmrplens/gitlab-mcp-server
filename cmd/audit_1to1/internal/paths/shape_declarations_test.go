package paths

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeclaredShapeFields_NamesWhatTheTreeHolds verifies the real table
// against the real tree, since an entry is a claim about it: each names a
// package under internal/tools, a type that package declares, a field, a
// known category and a reason. Whether the finding it answers still exists
// is the stale check's question, asked on every run of the audit; what the
// reason claims about GitLab's source is not checked here, which is what the
// reason is written down for.
func TestDeclaredShapeFields_NamesWhatTheTreeHolds(t *testing.T) {
	root := repoRoot(t)
	known := map[string]bool{
		categoryRecordSilent:              true,
		categoryServerShape:               true,
		categoryAnnotationNotPresented:    true,
		categoryServerDerived:             true,
		categorySharedTypeFilledElsewhere: true,
	}
	declared := map[string]map[string]bool{}
	for _, declaration := range declaredShapeFields {
		t.Run(declaration.key(), func(t *testing.T) {
			types, cached := declared[declaration.Package]
			if !cached {
				types = packageTypeNames(t, filepath.Join(root, declaration.Package))
				declared[declaration.Package] = types
			}
			if !types[declaration.Type] {
				t.Errorf("package %s declares no type %s", declaration.Package, declaration.Type)
			}
			if !known[declaration.Category] || declaration.Reason == "" || declaration.Field == "" {
				t.Errorf("declaration %+v is missing its category, reason or field", declaration)
			}
		})
	}
}

// packageTypeNames is every type name the non-test files of one directory
// declare, aliases included, read with the parser rather than by matching
// text so a name in a comment does not count.
func packageTypeNames(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read package %s: %v", dir, err)
	}
	names := map[string]bool{}
	files := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(files, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		for _, decl := range file.Decls {
			general, isGeneral := decl.(*ast.GenDecl)
			if !isGeneral || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				names[spec.(*ast.TypeSpec).Name.Name] = true
			}
		}
	}
	return names
}

// TestPackageTypeNames_ReadsDeclarationsAndNothingElse verifies the helper the
// table test rests on: a type and an alias declared in a source file count, a
// type named only in a comment, a function, a constant or a test file does
// not, and a subdirectory is skipped rather than read as a file.
func TestPackageTypeNames_ReadsDeclarationsAndNothingElse(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("shapes.go", "package p\n\n// Mentioned is a comment, not a type.\ntype Output struct{}\n\ntype Alias = Output\n\nconst Constant = 1\n\nfunc Function() {}\n")
	write("shapes_test.go", "package p\n\ntype TestOnly struct{}\n")
	write("notes.txt", "type Text struct{}\n")
	if err := os.Mkdir(filepath.Join(dir, "sub.go"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got := packageTypeNames(t, dir)

	for _, want := range []string{"Output", "Alias"} {
		if !got[want] {
			t.Errorf("packageTypeNames() is missing %s: %v", want, got)
		}
	}
	for _, absent := range []string{"Mentioned", "Constant", "Function", "TestOnly", "Text"} {
		if got[absent] {
			t.Errorf("packageTypeNames() counts %s, which is not a type declared in a source file", absent)
		}
	}
}
