package main

import (
	"go/ast"
	"os"
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

// fixtureRetired is the retired list the port map tests use: a test the old
// tree no longer declares and the new tree replaces, and one the old tree
// still declares, which retiring was wrong about.
var fixtureRetired = []string{"TestMeta_Retired", "TestMeta_Issues"}

// TestIsTestFunc_Names_GoTestRule verifies the name rule against go test's
// own: Test alone and Test followed by anything but a lowercase letter run,
// and a helper named Testhelper does not.
func TestIsTestFunc_Names_GoTestRule(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{name: "Test", want: true},
		{name: "TestMain", want: true},
		{name: "TestIssue_List", want: true},
		{name: "Test_underscore", want: true},
		{name: "Test1", want: true},
		{name: "Testhelper", want: false},
		{name: "testIssue", want: false},
		{name: "Benchmark", want: false},
		{name: "helper", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTestFunc(tc.name); got != tc.want {
				t.Errorf("isTestFunc(%q) = %t, want %t", tc.name, got, tc.want)
			}
		})
	}
}

// TestBuildPortMap_Fixtures_ResolvedAndUnresolved verifies the whole map over the
// fixture trees: the old Test functions found flat, the retired one merged
// in, the Replaces lines read off the new tests only, a subtest reference
// resolving to its parent, the drops applied, and every way the map can be
// wrong reported. Both trees hold a Testhelper: the old one must not be on
// the map, and the new one's Replaces line must retire nothing.
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
	m := resolvePortMap(oldTests, fixtureRetired, replaces, fixtureDrops)

	wantOld := []string{"TestBoth", "TestIndividual_Issues", "TestMain", "TestMeta_Issues", "TestMeta_Legacy", "TestMeta_Retired", "TestMeta_Unresolved"}
	if !reflect.DeepEqual(m.Old, wantOld) {
		t.Errorf("Old = %q, want %q", m.Old, wantOld)
	}
	if want := []string{"TestMeta_Retired"}; !reflect.DeepEqual(m.Retired, want) {
		t.Errorf("Retired = %q, want %q", m.Retired, want)
	}
	wantReplaced := map[string][]string{
		"TestBoth":              {"TestIssue_Both"},
		"TestIndividual_Issues": {"TestIssue_Lifecycle"},
		"TestMeta_Issues":       {"TestIssue_Lifecycle"},
		"TestMeta_Retired":      {"TestIssue_Retired"},
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
		"TestMeta_Issues is retired and still declared in the old suite",
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
		nil,
		map[string][]string{"TestNewA": {"TestA"}},
		map[string]dropDeclaration{"TestB": {Category: dropCoveredElsewhere, Reason: "test/e2e/http"}},
	)
	if !m.complete() {
		t.Errorf("complete() = false: unresolved %q, findings %q", m.Unresolved, m.Findings)
	}
}

// TestResolvePortMap_RetiredTests_HeldToTheSameRule verifies what the
// retired list does: a retired test is an old test, so one the new suite
// replaces is replaced, one a drop names is dropped, and one nothing names
// is unresolved rather than forgotten; the merged list is sorted with the
// declared ones; and the two ways the list can be wrong are findings, a
// name the old suite still declares and a name retired twice, each
// reported once and neither counted as old twice.
func TestResolvePortMap_RetiredTests_HeldToTheSameRule(t *testing.T) {
	m := resolvePortMap(
		[]string{"TestZ_Declared", "TestA_Declared"},
		[]string{"TestR_Replaced", "TestR_Unresolved", "TestR_Dropped", "TestA_Declared", "TestR_Replaced"},
		map[string][]string{"TestNew": {"TestR_Replaced", "TestZ_Declared"}},
		map[string]dropDeclaration{"TestR_Dropped": {Category: dropSuperseded, Reason: "moot"}},
	)
	wantOld := []string{"TestA_Declared", "TestR_Dropped", "TestR_Replaced", "TestR_Unresolved", "TestZ_Declared"}
	if !reflect.DeepEqual(m.Old, wantOld) {
		t.Errorf("Old = %q, want %q", m.Old, wantOld)
	}
	wantRetired := []string{"TestR_Dropped", "TestR_Replaced", "TestR_Unresolved"}
	if !reflect.DeepEqual(m.Retired, wantRetired) {
		t.Errorf("Retired = %q, want %q", m.Retired, wantRetired)
	}
	if got := m.Replaced["TestR_Replaced"]; !reflect.DeepEqual(got, []string{"TestNew"}) {
		t.Errorf("Replaced[TestR_Replaced] = %q, want TestNew", got)
	}
	if _, dropped := m.Dropped["TestR_Dropped"]; !dropped {
		t.Errorf("Dropped = %v, want TestR_Dropped", m.Dropped)
	}
	if want := []string{"TestA_Declared", "TestR_Unresolved"}; !reflect.DeepEqual(m.Unresolved, want) {
		t.Errorf("Unresolved = %q, want %q", m.Unresolved, want)
	}
	wantFindings := []string{
		"TestA_Declared is retired and still declared in the old suite",
		"TestR_Replaced is retired twice",
	}
	if !reflect.DeepEqual(m.Findings, wantFindings) {
		t.Errorf("Findings = %q, want %q", m.Findings, wantFindings)
	}
	if m.complete() {
		t.Error("complete() = true with an unresolved retired test and findings")
	}
}

// TestRetiredTests_Table_NamesDeletedTests verifies the production list on
// the tree it describes: every entry is a name go test would run, none is
// listed twice, and none is declared under test/e2e/suite any more, since
// each was retired on the claim that its file is gone. The list goes with
// the old suite: once test/e2e/suite is deleted there is nothing for it to
// be held against, and this test says so rather than failing.
func TestRetiredTests_Table_NamesDeletedTests(t *testing.T) {
	oldDir := filepath.Join("..", "..", "test", "e2e", "suite")
	if _, err := os.Stat(oldDir); err != nil {
		t.Skipf("%s is gone, and the retired list goes with it", oldDir)
	}
	declared, err := testFunctions(oldDir)
	if err != nil {
		t.Fatalf("testFunctions(%s) error = %v", oldDir, err)
	}
	stillDeclared := map[string]bool{}
	for _, name := range declared {
		stillDeclared[name] = true
	}
	seen := map[string]bool{}
	for _, name := range retiredTests {
		t.Run(name, func(t *testing.T) {
			if !isTestFunc(name) {
				t.Errorf("%s is not a name go test runs", name)
			}
			if seen[name] {
				t.Errorf("%s is listed twice", name)
			}
			seen[name] = true
			if stillDeclared[name] {
				t.Errorf("%s is retired and still declared under %s", name, oldDir)
			}
		})
	}
	if len(retiredTests) != 74 {
		t.Errorf("retiredTests holds %d names, want the 74 Test functions of the deleted EE half", len(retiredTests))
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
// entry point over the same fixtures, with and without a retired list, and
// that an old suite with no Test function is refused.
func TestBuildPortMap_Fixtures_ReadsBothTrees(t *testing.T) {
	oldDir, newDir := portMapFixtures()
	m, err := buildPortMap(oldDir, newDir, nil, fixtureDrops)
	if err != nil {
		t.Fatalf("buildPortMap() error = %v", err)
	}
	if len(m.Old) != 6 || len(m.Retired) != 0 {
		t.Errorf("buildPortMap() read %d old tests (%d retired), want 6 and none", len(m.Old), len(m.Retired))
	}
	withRetired, err := buildPortMap(oldDir, newDir, fixtureRetired, fixtureDrops)
	if err != nil {
		t.Fatalf("buildPortMap() with the retired list error = %v", err)
	}
	if len(withRetired.Old) != 7 || len(withRetired.Retired) != 1 {
		t.Errorf("buildPortMap() with the retired list read %d old tests (%d retired), want 7 and 1", len(withRetired.Old), len(withRetired.Retired))
	}
	if _, emptyErr := buildPortMap(t.TempDir(), newDir, nil, fixtureDrops); emptyErr == nil {
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
	if _, err := buildPortMap(oldDir, dir, nil, fixtureDrops); err == nil || !strings.Contains(err.Error(), "new suite") {
		t.Errorf("buildPortMap() error = %v, want the new suite's parse failure", err)
	}
	if _, err := buildPortMap(dir, dir, nil, fixtureDrops); err == nil || !strings.Contains(err.Error(), "old suite") {
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
