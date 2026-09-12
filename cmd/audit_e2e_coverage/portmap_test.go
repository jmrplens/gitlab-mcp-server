package main

import (
	"go/ast"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// portMapFixtures are the two trees the port map tests read.
func portMapFixtures() (oldDir, newDir string) {
	return filepath.Join("testdata", "portmap", "old"), filepath.Join("testdata", "portmap", "new")
}

// fixtureDrops is the declared-drops table the port map tests use: a valid
// drop, a stale one, one under an unknown category, and one that collides
// with a Replaces line.
var fixtureDrops = map[string]dropDeclaration{
	"TestMeta_Legacy": {Category: dropCopiedProduction, Reason: "reproduced the selector closure"},
	"TestMeta_Gone":   {Category: dropSuperseded, Reason: "stale: the old suite has no such test"},
	"TestMain":        {Category: "made-up", Reason: "unknown category"},
	"TestBoth":        {Category: dropSuperseded, Reason: "also replaced"},
}

// TestBuildPortMap_Fixtures_ResolvedAndUnresolved verifies the whole map over the
// fixture trees: the old Test functions found flat, the Replaces lines read
// off the new tests only, a subtest reference resolving to its parent, the
// drops applied, and every way the map can be wrong reported.
func TestBuildPortMap_Fixtures_ResolvedAndUnresolved(t *testing.T) {
	oldDir, newDir := portMapFixtures()
	oldTests, err := testFunctions(oldDir)
	if err != nil {
		t.Fatalf("testFunctions() error = %v", err)
	}
	replaces, err := replacesLines(newDir)
	if err != nil {
		t.Fatalf("replacesLines() error = %v", err)
	}
	m := resolvePortMap(oldTests, replaces, fixtureDrops)

	wantOld := []string{"TestBoth", "TestIndividual_Issues", "TestMain", "TestMeta_Issues", "TestMeta_Legacy", "TestMeta_Unresolved"}
	if !reflect.DeepEqual(m.Old, wantOld) {
		t.Errorf("Old = %q, want %q", m.Old, wantOld)
	}
	wantReplaced := map[string][]string{
		"TestBoth":              {"TestIssue_Both"},
		"TestIndividual_Issues": {"TestIssue_Lifecycle"},
		"TestMeta_Issues":       {"TestIssue_Lifecycle"},
	}
	if !reflect.DeepEqual(m.Replaced, wantReplaced) {
		t.Errorf("Replaced = %v, want %v", m.Replaced, wantReplaced)
	}
	if _, dropped := m.Dropped["TestMeta_Legacy"]; !dropped || len(m.Dropped) != 1 {
		t.Errorf("Dropped = %v, want only TestMeta_Legacy", m.Dropped)
	}
	if want := []string{"TestMain", "TestMeta_Unresolved"}; !reflect.DeepEqual(m.Unresolved, want) {
		t.Errorf("Unresolved = %q, want %q", m.Unresolved, want)
	}
	wantFindings := []string{
		"TestBoth is both dropped and replaced by TestIssue_Both",
		"TestIssue_Ghost replaces TestMeta_Ghost, which the old suite does not have",
		"drop declared for TestMain under the unknown category \"made-up\"",
		"drop declared for TestMeta_Gone, which the old suite does not have",
	}
	if !reflect.DeepEqual(m.Findings, wantFindings) {
		t.Errorf("Findings = %q, want %q", m.Findings, wantFindings)
	}
	if m.complete() {
		t.Error("complete() = true with unresolved tests and findings")
	}
}

// TestResolvePortMap_EveryTestResolved_Complete verifies the map that passes: every old test
// replaced or dropped, and nothing wrong with the map.
func TestResolvePortMap_EveryTestResolved_Complete(t *testing.T) {
	m := resolvePortMap(
		[]string{"TestA", "TestB"},
		map[string][]string{"TestNewA": {"TestA"}},
		map[string]dropDeclaration{"TestB": {Category: dropCoveredElsewhere, Reason: "test/e2e/http"}},
	)
	if !m.complete() {
		t.Errorf("complete() = false: unresolved %q, findings %q", m.Unresolved, m.Findings)
	}
}

// TestReplacesLines_MissingNewSuite_IsEmpty verifies that a new suite that
// does not exist yet reads as no Replaces line at all, which leaves every
// old test unresolved rather than breaking the command.
func TestReplacesLines_MissingNewSuite_IsEmpty(t *testing.T) {
	replaces, err := replacesLines(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("replacesLines() error = %v, want nil", err)
	}
	if len(replaces) != 0 {
		t.Errorf("replacesLines() = %v, want empty", replaces)
	}
}

// TestBuildPortMap_Fixtures_ReadsBothTrees verifies the directory-level
// entry point over the same fixtures, and that an old suite with no Test
// function is refused.
func TestBuildPortMap_Fixtures_ReadsBothTrees(t *testing.T) {
	oldDir, newDir := portMapFixtures()
	m, err := buildPortMap(oldDir, newDir)
	if err != nil {
		t.Fatalf("buildPortMap() error = %v", err)
	}
	if len(m.Old) != 6 {
		t.Errorf("buildPortMap() read %d old tests, want 6", len(m.Old))
	}
	if _, emptyErr := buildPortMap(t.TempDir(), newDir); emptyErr == nil {
		t.Error("buildPortMap() accepted an old suite with no Test function")
	}
}

// TestWalkTestFiles_UnparseableFile_IsAnError verifies that a test file the
// parser refuses stops the walk rather than being skipped as if it held no
// tests, on both sides of the map.
func TestWalkTestFiles_UnparseableFile_IsAnError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "broken_test.go"), "package broken\nfunc (")
	if _, err := testFunctions(dir); err == nil {
		t.Error("testFunctions() accepted a file that does not parse")
	}
	if _, err := replacesLines(dir); err == nil {
		t.Error("replacesLines() accepted a file that does not parse")
	}
	oldDir, _ := portMapFixtures()
	if _, err := buildPortMap(oldDir, dir); err == nil || !strings.Contains(err.Error(), "new suite") {
		t.Errorf("buildPortMap() error = %v, want the new suite's parse failure", err)
	}
	if _, err := buildPortMap(dir, dir); err == nil || !strings.Contains(err.Error(), "old suite") {
		t.Errorf("buildPortMap() error = %v, want the old suite's parse failure", err)
	}
}

// TestParseReplaces_Spelling_Read verifies how a Replaces line is read: names
// split on commas, blanks dropped, a subtest reference reduced to its
// parent, and other comment lines ignored.
func TestParseReplaces_Spelling_Read(t *testing.T) {
	doc := &ast.CommentGroup{List: []*ast.Comment{
		{Text: "// TestX covers something."},
		{Text: "// Replaces: TestA, , TestB/sub ,TestC"},
		{Text: "//Replaces: TestD"},
	}}
	if got, want := parseReplaces(doc), []string{"TestA", "TestB", "TestC", "TestD"}; !reflect.DeepEqual(got, want) {
		t.Errorf("parseReplaces() = %q, want %q", got, want)
	}
}
