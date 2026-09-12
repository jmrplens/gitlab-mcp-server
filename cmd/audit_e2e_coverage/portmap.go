package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// replacesPrefix starts the comment line a new test carries to name the old
// tests it replaces: "// Replaces: TestMeta_Issues, TestIndividual_Issues".
const replacesPrefix = "Replaces:"

// dropDeclaration records why an old Test function has no successor.
type dropDeclaration struct {
	// Category says what kind of drop this is.
	Category string
	// Reason says why, in the words a reviewer needs.
	Reason string
}

// Drop categories.
const (
	// dropCoveredElsewhere is a test whose subject another module already
	// covers on the wire: the transport modules under test/e2e/http and
	// test/e2e/stdio.
	dropCoveredElsewhere = "covered-elsewhere"
	// dropCopiedProduction is a test that reproduced production wiring in
	// the test process and so tested its copy, which the binary-driven
	// suite has no equivalent of by construction.
	dropCopiedProduction = "copied-production"
	// dropSuperseded is a test whose subject the rebuild makes moot.
	dropSuperseded = "superseded"
)

// declaredDrops holds every old Test function that no new test replaces,
// each with a category and a reason.
//
// It ships empty and fills as the port proceeds. A drop naming a test the
// old suite does not have is a finding, and so is one naming a test that a
// Replaces line also claims: the two are different answers to where a
// scenario went, and both cannot be true.
var declaredDrops = map[string]dropDeclaration{}

// declaredDropCategories is the set a category must belong to.
var declaredDropCategories = map[string]bool{
	dropCoveredElsewhere: true,
	dropCopiedProduction: true,
	dropSuperseded:       true,
}

// portMap is the resolution of every old Test function.
type portMap struct {
	// Old lists every Test function of the old suite, sorted.
	Old []string `json:"old"`
	// Replaced maps an old test to the new tests that name it.
	Replaced map[string][]string `json:"replaced"`
	// Dropped maps an old test to its drop declaration.
	Dropped map[string]dropDeclaration `json:"dropped"`
	// Unresolved lists the old tests neither replaced nor dropped.
	Unresolved []string `json:"unresolved"`
	// Findings lists what is wrong with the map itself: a Replaces line or a
	// drop naming a test the old suite lacks, a test both replaced and
	// dropped, a drop with an unknown category.
	Findings []string `json:"findings"`
}

// complete reports whether every old test is resolved and nothing about the
// map is wrong.
func (m *portMap) complete() bool {
	return len(m.Unresolved) == 0 && len(m.Findings) == 0
}

// buildPortMap reads the old suite's Test functions and the new suite's
// Replaces lines, and resolves each old test.
func buildPortMap(oldDir, newDir string) (*portMap, error) {
	oldTests, err := testFunctions(oldDir)
	if err != nil {
		return nil, fmt.Errorf("old suite: %w", err)
	}
	if len(oldTests) == 0 {
		return nil, fmt.Errorf("old suite: no Test function under %s", oldDir)
	}
	replaces, err := replacesLines(newDir)
	if err != nil {
		return nil, fmt.Errorf("new suite: %w", err)
	}
	return resolvePortMap(oldTests, replaces, declaredDrops), nil
}

// resolvePortMap is [buildPortMap] once the two suites have been read, so a
// test can hand it lists instead of directories.
func resolvePortMap(oldTests []string, replaces map[string][]string, drops map[string]dropDeclaration) *portMap {
	known := map[string]bool{}
	for _, name := range oldTests {
		known[name] = true
	}
	m := &portMap{Old: oldTests, Replaced: map[string][]string{}, Dropped: map[string]dropDeclaration{}}
	for newTest, olds := range replaces {
		for _, old := range olds {
			if !known[old] {
				m.Findings = append(m.Findings, fmt.Sprintf("%s replaces %s, which the old suite does not have", newTest, old))
				continue
			}
			m.Replaced[old] = append(m.Replaced[old], newTest)
		}
	}
	for old, replacements := range m.Replaced {
		sort.Strings(replacements)
		m.Replaced[old] = replacements
	}
	for old, drop := range drops {
		m.noteDrop(old, drop, known)
	}
	for _, name := range oldTests {
		if _, replaced := m.Replaced[name]; replaced {
			continue
		}
		if _, dropped := m.Dropped[name]; dropped {
			continue
		}
		m.Unresolved = append(m.Unresolved, name)
	}
	sort.Strings(m.Unresolved)
	sort.Strings(m.Findings)
	return m
}

// noteDrop records one declared drop, or what is wrong with it.
func (m *portMap) noteDrop(old string, drop dropDeclaration, known map[string]bool) {
	switch {
	case !known[old]:
		m.Findings = append(m.Findings, fmt.Sprintf("drop declared for %s, which the old suite does not have", old))
	case !declaredDropCategories[drop.Category]:
		m.Findings = append(m.Findings, fmt.Sprintf("drop declared for %s under the unknown category %q", old, drop.Category))
	case len(m.Replaced[old]) > 0:
		m.Findings = append(m.Findings, fmt.Sprintf("%s is both dropped and replaced by %s", old, strings.Join(m.Replaced[old], ", ")))
	default:
		m.Dropped[old] = drop
	}
}

// testFunctions lists the Test functions declared in the _test.go files
// directly under dir, sorted.
//
// It does not descend: the old suite is one flat package, and a Test function
// one directory down would belong to another package the map is not about.
func testFunctions(dir string) ([]string, error) {
	var names []string
	err := walkTestFiles(dir, false, func(file *ast.File) {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if isFunc && fn.Recv == nil && isTestFunc(fn.Name.Name) {
				names = append(names, fn.Name.Name)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

// isTestFunc reports whether a function name is one the test binary runs as
// a test, TestMain included, since the old suite's TestMain is on the port
// map too.
func isTestFunc(name string) bool {
	return strings.HasPrefix(name, "Test")
}

// replacesLines reads every Replaces line off the Test functions under dir,
// recursively, as a map from the new test to the old tests it names.
//
// A directory that does not exist yields an empty map rather than an error:
// before the new suite is created every old test is unresolved, which is
// the truthful answer, and not a broken command.
func replacesLines(dir string) (map[string][]string, error) {
	replaces := map[string][]string{}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return replaces, nil
	}
	err := walkTestFiles(dir, true, func(file *ast.File) {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Recv != nil || !isTestFunc(fn.Name.Name) || fn.Doc == nil {
				continue
			}
			if olds := parseReplaces(fn.Doc); len(olds) > 0 {
				replaces[fn.Name.Name] = append(replaces[fn.Name.Name], olds...)
			}
		}
	})
	return replaces, err
}

// parseReplaces reads the old test names off a doc comment's Replaces lines.
func parseReplaces(doc *ast.CommentGroup) []string {
	var olds []string
	for _, comment := range doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
		if !strings.HasPrefix(text, replacesPrefix) {
			continue
		}
		for name := range strings.SplitSeq(strings.TrimPrefix(text, replacesPrefix), ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			// A subtest reference names its parent's function.
			olds = append(olds, topLevelTest(name))
		}
	}
	return olds
}

// walkTestFiles parses every _test.go file under dir and hands each to visit.
func walkTestFiles(dir string, recursive bool, visit func(*ast.File)) error {
	fset := token.NewFileSet()
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", dir, err)
		}
		if entry.IsDir() {
			if path != dir && !recursive {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		visit(file)
		return nil
	})
}
