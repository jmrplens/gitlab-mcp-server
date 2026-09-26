package tenancy

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// The package's own import path, as the toolchain lists it.
const tenancyImportPath = "github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"

// productionFiles parses every non-test Go file of the package, by name.
//
// The files gobco writes into its instrumented copy of the package are left
// out: they carry the counters `make coverage-conditions` reads, and are no
// more part of the leaf than the copy is.
func productionFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the package directory: %v", err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
			strings.HasPrefix(name, "gobco_") {
			continue
		}
		f, parseErr := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		files[name] = f
	}
	if len(files) == 0 {
		t.Fatal("found no production file to read")
	}
	return files
}

// TestPackage_ImportsTheStandardLibraryOnly holds the leaf's imports to the
// four standard packages the server binary already links. It is the source
// half of the condition the proof of an unchanged binary rests on.
func TestPackage_ImportsTheStandardLibraryOnly(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range productionFiles(t) {
		for _, spec := range f.Imports {
			seen[strings.Trim(spec.Path.Value, `"`)] = true
		}
	}
	var imports []string
	for path := range seen {
		imports = append(imports, path)
	}
	slices.Sort(imports)
	if got, want := strings.Join(imports, ","), "errors,fmt,strings,time"; got != want {
		t.Errorf("the package imports %s, want exactly %s", got, want)
	}
}

// goListLines runs a go list command and returns its output as lines.
func goListLines(t *testing.T, cmd *exec.Cmd) []string {
	t.Helper()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v: %v", cmd.Args, err)
	}
	var lines []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// TestPackage_DependsOnTheStandardLibraryOnly asks the toolchain rather than
// the parser, because an indirect import is exactly what a source scan misses:
// the only dependency outside the standard library is the package itself.
func TestPackage_DependsOnTheStandardLibraryOnly(t *testing.T) {
	nonStandard := goListLines(t, exec.CommandContext(t.Context(),
		"go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "."))
	if len(nonStandard) != 1 || nonStandard[0] != tenancyImportPath {
		t.Errorf("dependencies outside the standard library: %v, want only %s", nonStandard, tenancyImportPath)
	}
}

// TestPackage_LinksNothingTheServerDoesNot holds every dependency of the leaf
// to one the server binary already has, which is the condition under which a
// layer that makes the server import it links nothing new, so that its binary
// is the one its parent builds once the parent imports the leaf where the
// layer does.
func TestPackage_LinksNothingTheServerDoesNot(t *testing.T) {
	server := map[string]bool{}
	for _, pkg := range goListLines(t, exec.CommandContext(t.Context(), "go", "list", "-deps", "../../cmd/server")) {
		server[pkg] = true
	}
	if !server["github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"] {
		t.Fatalf("go list -deps ../../cmd/server named %d packages and not internal/tools", len(server))
	}
	for _, pkg := range goListLines(t, exec.CommandContext(t.Context(), "go", "list", "-deps", ".")) {
		if pkg != tenancyImportPath && !server[pkg] {
			t.Errorf("the leaf depends on %s, which the server does not link", pkg)
		}
	}
}

// TestPackage_DeclaresNoPackageLevelVariable holds the leaf to constants,
// types and functions: a package-level variable is initialization work that a
// binary importing the leaf would carry, and a table held in one is kept alive
// whether anything reads it or not.
func TestPackage_DeclaresNoPackageLevelVariable(t *testing.T) {
	for name, f := range productionFiles(t) {
		for _, decl := range f.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
				t.Errorf("%s declares a package-level variable", name)
			}
		}
	}
}
