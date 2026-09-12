//go:build e2e

// sources_test.go holds the parser-only gates over the rebuilt suite's source.
//
// They read files and never type-check them, so they cost a parse and run on
// every push with no GitLab and no build of the packages they judge. Each
// rule answers a question the suite this replaces got wrong: a test that
// assembles a server of its own is testing its assembly; a file behind a tag
// nothing names is analyzed by nothing; and a _ce_ or _ee_ suffix is the
// old runtime split, which the package layout replaced.

package harness

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The trees the gates read, relative to the repository root: the runtime
// packages, which the assembly rule is about, and the libraries they call
// through, which share the constraint and naming rules.
var (
	runtimeTestTree = filepath.Join("test", "e2e", "gitlab")
	harnessTree     = filepath.Join("test", "e2e", "internal")
)

// The import paths whose use a runtime package may not make.
const (
	sdkImportPath   = "github.com/modelcontextprotocol/go-sdk/mcp"
	toolsImportPath = "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
)

// forbiddenSDKCalls are the go-sdk constructors that build a server or a
// client in the test process. The harness builds the one client the suite
// uses; a test that built its own would be recorded by nothing.
var forbiddenSDKCalls = []string{"NewServer", "NewClient", "NewInMemoryTransports"}

// e2eConstraint is the one build constraint every file but doc.go carries.
const e2eConstraint = "//go:build e2e"

// raceSeamConstraints are the one pair a file may carry instead: the harness
// builds the server it drives with -race when the tests run under the
// detector and without it otherwise, and the two halves of that seam are
// selected by the race tag. Both still carry e2e, so nothing under this tree
// is ever outside the one tag the analysis names.
var raceSeamConstraints = []string{"//go:build e2e && race", "//go:build e2e && !race"}

// editionSuffixes are the file-name markers of the old suite's runtime split.
// A package decides its runtime now, and a file that said so in its name
// would be saying it twice, once out of step with the other.
var editionSuffixes = []string{"_ce_test.go", "_ee_test.go", "_ce.go", "_ee.go"}

// sourceFinding is one thing a gate refuses, positioned.
type sourceFinding struct {
	pos     string
	message string
}

// String spells the finding for a failure message.
func (f sourceFinding) String() string {
	if f.pos == "" {
		return f.message
	}
	return f.pos + ": " + f.message
}

// goFiles lists every .go file under root, sorted, and fails on a root that
// cannot be read: a gate that saw nothing would pass on the wrong tree.
func goFiles(t *testing.T, root string) []string {
	t.Helper()

	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name := entry.Name(); path != root && (name == "testdata" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	slices.Sort(files)
	return files
}

// assemblyFindings reports every call under root that would build a server
// or a client of its own, or reach tools/call without naming an action.
//
// A file's imports say which local name the SDK and the tools package go by,
// so an aliased import is judged the same as a plain one. CallTool is judged
// by name alone: the parser cannot see the receiver's type, and the only
// legitimate caller of ClientSession.CallTool is the harness, which is not
// under this root.
func assemblyFindings(fset *token.FileSet, files []string) ([]sourceFinding, error) {
	var findings []sourceFinding
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		sdkNames := localNames(file, sdkImportPath)
		toolsNames := localNames(file, toolsImportPath)

		ast.Inspect(file, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if !isSelector {
				return true
			}
			pos := fset.Position(call.Pos()).String()
			name := selector.Sel.Name
			if name == "CallTool" {
				findings = append(findings, sourceFinding{pos: pos, message: "CallTool is called directly; name an action and let the harness project it, or use Raw for a protocol test"})
				return true
			}
			receiver, isIdent := selector.X.(*ast.Ident)
			if !isIdent {
				return true
			}
			switch {
			case slices.Contains(sdkNames, receiver.Name) && slices.Contains(forbiddenSDKCalls, name):
				findings = append(findings, sourceFinding{pos: pos, message: "mcp." + name + " assembles a server or a client in the test process; the harness is the only route to a server"})
			case slices.Contains(toolsNames, receiver.Name) && strings.HasPrefix(name, "Register"):
				findings = append(findings, sourceFinding{pos: pos, message: "tools." + name + " registers a surface in the test process; the binary registers its own"})
			}
			return true
		})
	}
	return findings, nil
}

// localNames returns the identifiers one import path is known by in a file:
// its alias when it has one, and otherwise the last path segment.
func localNames(file *ast.File, importPath string) []string {
	var names []string
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) != importPath {
			continue
		}
		if spec.Name != nil {
			names = append(names, spec.Name.Name)
			continue
		}
		names = append(names, importPath[strings.LastIndex(importPath, "/")+1:])
	}
	return names
}

// constraintFindings reports every file whose build constraints are not
// exactly the one the suite uses: one "//go:build e2e" on every file, or one
// half of the race seam, and none on doc.go, which is what a plain build
// sees of the package.
func constraintFindings(files []string) ([]sourceFinding, error) {
	var findings []sourceFinding
	for _, path := range files {
		constraints, err := buildConstraints(path)
		if err != nil {
			return nil, err
		}
		switch {
		case filepath.Base(path) == "doc.go":
			if len(constraints) > 0 {
				findings = append(findings, sourceFinding{pos: path, message: "doc.go carries a build constraint, and it must be what a plain build sees of the package"})
			}
		case len(constraints) != 1 || (constraints[0] != e2eConstraint && !slices.Contains(raceSeamConstraints, constraints[0])):
			findings = append(findings, sourceFinding{pos: path, message: fmt.Sprintf("build constraints %q, want exactly %q", constraints, e2eConstraint)})
		}
	}
	return findings, nil
}

// buildConstraints returns the //go:build lines of a file's header, which is
// every line before the package clause. A constraint after it is not one, and
// the compiler ignores it too.
func buildConstraints(path string) ([]string, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var constraints []string
	for line := range strings.SplitSeq(string(source), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") {
			break
		}
		if strings.HasPrefix(trimmed, "//go:build") {
			constraints = append(constraints, trimmed)
		}
	}
	return constraints, nil
}

// suffixFindings reports every file named with the old suite's edition
// suffix.
func suffixFindings(files []string) []sourceFinding {
	var findings []sourceFinding
	for _, path := range files {
		name := filepath.Base(path)
		for _, suffix := range editionSuffixes {
			if strings.HasSuffix(name, suffix) {
				findings = append(findings, sourceFinding{pos: path, message: "the " + suffix + " suffix is the old runtime split; the package decides the runtime now"})
				break
			}
		}
	}
	return findings
}

// reportFindings fails the test with every finding, one per line.
func reportFindings(t *testing.T, findings []sourceFinding) {
	t.Helper()
	if len(findings) == 0 {
		return
	}
	lines := make([]string, 0, len(findings))
	for _, finding := range findings {
		lines = append(lines, finding.String())
	}
	t.Fatalf("%d finding(s):\n  %s", len(findings), strings.Join(lines, "\n  "))
}

// repositoryTree returns the absolute path of one tree under the repository
// root.
func repositoryTree(t *testing.T, tree string) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("resolving the repository root: %v", err)
	}
	return filepath.Join(root, tree)
}

// TestSources_RuntimePackages_NeverAssembleAServer is the gate behind issue
// 616's suite half: no test under test/e2e/gitlab builds a server, builds a
// client, registers a surface, or calls tools/call by hand.
func TestSources_RuntimePackages_NeverAssembleAServer(t *testing.T) {
	findings, err := assemblyFindings(token.NewFileSet(), goFiles(t, repositoryTree(t, runtimeTestTree)))
	if err != nil {
		t.Fatalf("parsing the runtime packages: %v", err)
	}
	reportFindings(t, findings)
}

// TestSources_EveryFile_CarriesExactlyTheE2EConstraint checks the one
// constraint on every file of the runtime packages and the libraries, and
// its absence on doc.go.
//
// One tag is what lets one analysis run and one compile see the whole suite.
// The old suite's two halves excluded each other, so nothing compiled its
// Enterprise files outside a licensed run for as long as they existed.
func TestSources_EveryFile_CarriesExactlyTheE2EConstraint(t *testing.T) {
	files := append(goFiles(t, repositoryTree(t, runtimeTestTree)), goFiles(t, repositoryTree(t, harnessTree))...)
	findings, err := constraintFindings(files)
	if err != nil {
		t.Fatalf("reading the constraints: %v", err)
	}
	reportFindings(t, findings)
}

// TestSources_NoFile_CarriesAnEditionSuffix checks that the runtime split
// lives in the package layout and nowhere in a file name.
func TestSources_NoFile_CarriesAnEditionSuffix(t *testing.T) {
	files := append(goFiles(t, repositoryTree(t, runtimeTestTree)), goFiles(t, repositoryTree(t, harnessTree))...)
	reportFindings(t, suffixFindings(files))
}

// plantFile writes one source file into a fixture tree and returns its path.
func plantFile(t *testing.T, dir, name, source string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("creating the directory of %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

// TestAssemblyFindings_PlantedDefects_AreEachReported drives the assembly
// rule over a fixture with every forbidden shape, aliased imports included,
// and one clean file, so a rule that silently matched nothing would fail
// here rather than pass on the real tree.
func TestAssemblyFindings_PlantedDefects_AreEachReported(t *testing.T) {
	dir := t.TempDir()
	planted := plantFile(t, dir, "bad_test.go", `//go:build e2e

package common

import (
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
)

func bad() {
	server := sdk.NewServer(nil, nil)
	_ = sdk.NewClient(nil, nil)
	_, _ = sdk.NewInMemoryTransports()
	gitlabtools.RegisterAll(server, nil)
	var session *sdk.ClientSession
	_, _ = session.CallTool(nil, nil)
}
`)
	plantFile(t, dir, "good_test.go", `//go:build e2e

package common

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

func good(e *harness.Env) {
	_ = harness.AllSurfaces()
}
`)

	findings, err := assemblyFindings(token.NewFileSet(), goFiles(t, dir))
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}

	wantMessages := []string{"mcp.NewServer", "mcp.NewClient", "mcp.NewInMemoryTransports", "tools.RegisterAll", "CallTool"}
	if len(findings) != len(wantMessages) {
		t.Fatalf("got %d findings, want %d:\n%v", len(findings), len(wantMessages), findings)
	}
	for i, want := range wantMessages {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(findings[i].message, want) {
				t.Errorf("finding %d is %q, want it to name %s", i, findings[i].message, want)
			}
			if !strings.HasPrefix(findings[i].pos, planted) {
				t.Errorf("finding %d is positioned at %q, want the planted file", i, findings[i].pos)
			}
		})
	}
}

// TestConstraintFindings_EveryShape_IsJudged drives the constraint rule over
// the four shapes a file can take: right, missing, wrong, and a doc.go that
// carries one.
func TestConstraintFindings_EveryShape_IsJudged(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name   string
		file   string
		source string
		want   bool
	}{
		{name: "tagged e2e", file: "right_test.go", source: "//go:build e2e\n\npackage p\n", want: false},
		{name: "race seam", file: "seam_race.go", source: "//go:build e2e && race\n\npackage p\n", want: false},
		{name: "race seam other half", file: "seam_norace.go", source: "//go:build e2e && !race\n\npackage p\n", want: false},
		{name: "untagged doc.go", file: "doc.go", source: "// Package p.\npackage p\n", want: false},
		{name: "missing tag", file: "missing_test.go", source: "package p\n", want: true},
		{name: "wrong tag", file: "wrong_test.go", source: "//go:build e2e && enterprise\n\npackage p\n", want: true},
		{name: "race without e2e", file: "bare_race.go", source: "//go:build race\n\npackage p\n", want: true},
		{name: "two tags", file: "two_test.go", source: "//go:build e2e\n//go:build e2e\n\npackage p\n", want: true},
		{name: "tagged doc.go", file: filepath.Join("sub", "doc.go"), source: "//go:build e2e\n\npackage sub\n", want: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			path := plantFile(t, dir, testCase.file, testCase.source)

			findings, err := constraintFindings([]string{path})
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}
			if got := len(findings) > 0; got != testCase.want {
				t.Errorf("finding = %t, want %t: %v", got, testCase.want, findings)
			}
		})
	}
}

// TestSuffixFindings_OldSplitNames_AreRefused drives the naming rule over the
// old suite's spellings and a name the new layout allows.
func TestSuffixFindings_OldSplitNames_AreRefused(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{name: "issues_ce_test.go", want: true},
		{name: "issues_ee_test.go", want: true},
		{name: "helpers_ce.go", want: true},
		{name: "helpers_ee.go", want: true},
		{name: "issues_test.go", want: false},
		{name: "license_test.go", want: false},
		{name: "doc.go", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			findings := suffixFindings([]string{filepath.Join("tree", testCase.name)})

			if got := len(findings) > 0; got != testCase.want {
				t.Errorf("finding = %t, want %t: %v", got, testCase.want, findings)
			}
		})
	}
}

// TestGoFiles_SkipsTestdataAndHiddenDirectories checks that a fixture tree
// planted by another gate is not judged by these.
func TestGoFiles_SkipsTestdataAndHiddenDirectories(t *testing.T) {
	dir := t.TempDir()
	plantFile(t, dir, filepath.Join("testdata", "file.go"), "package p\n")
	plantFile(t, dir, filepath.Join(".hidden", "file.go"), "package p\n")
	plantFile(t, dir, filepath.Join("kept", "file.go"), "package p\n")
	plantFile(t, dir, "top.go", "package p\n")

	files := goFiles(t, dir)

	want := []string{filepath.Join(dir, "kept", "file.go"), filepath.Join(dir, "top.go")}
	if !slices.Equal(files, want) {
		t.Errorf("goFiles() = %v, want %v", files, want)
	}
}
