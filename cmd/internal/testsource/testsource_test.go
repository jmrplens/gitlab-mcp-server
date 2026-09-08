package testsource

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestIsTestFunction_NameShapes_FollowGoRules verifies the Test* entry-point
// rule this package settles for every command that counts tests: a bare Test
// and any name whose next rune is not lower case are tests, TestMain alone is
// not, and a lowercase follower or another prefix is not. The Test_ and Test9
// cases are the ones the two generators used to disagree about, since a rule
// written as "the next rune is upper case" rejects both.
func TestIsTestFunction_NameShapes_FollowGoRules(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{name: "Test", want: true},
		{name: "TestWidget", want: true},
		{name: "Test_Widget", want: true},
		{name: "Test9Widget", want: true},
		{name: "TestMain_Flags_Parse", want: true},
		{name: "TestÉtat", want: true},
		{name: "TestMain", want: false},
		{name: "Testwidget", want: false},
		{name: "Testéquipe", want: false},
		{name: "BenchmarkWidget", want: false},
		{name: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTestFunction(tc.name); got != tc.want {
				t.Errorf("IsTestFunction(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestClassifyTestName_Shapes_SelectTheBucket verifies the four naming buckets:
// the coverage-helper prefix wins over the underscore count, three or more
// segments are the compliant shape, two are tolerated, and a name with no
// underscore at all is the legacy shape.
func TestClassifyTestName_Shapes_SelectTheBucket(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{name: "TestCreate_ValidInput_ReturnsIssue", want: Pattern3Part},
		{name: "TestCreate_ReturnsIssue", want: Pattern2Part},
		{name: "TestCreateIssue", want: PatternNoUnderscore},
		{name: "TestCovBuildCatalog", want: PatternTestCov},
		{name: "TestCovBuild_Catalog", want: PatternTestCov},
		{name: "TestCovered", want: PatternNoUnderscore},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyTestName(tc.name); got != tc.want {
				t.Errorf("ClassifyTestName(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestSkipDir_Names_PruneGeneratedFixtureAndHiddenTrees verifies the one skip
// list: the vendored and generated trees, a tool's own fixtures, and every dot
// directory are pruned, while the walk roots themselves and a directory that
// merely starts with those letters are kept.
func TestSkipDir_Names_PruneGeneratedFixtureAndHiddenTrees(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{name: "node_modules", want: true},
		{name: "dist", want: true},
		{name: "testdata", want: true},
		{name: ".hidden", want: true},
		{name: ".", want: false},
		{name: "..", want: false},
		{name: "internal", want: false},
		{name: "distribution", want: false},
		{name: "testdata_helpers", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SkipDir(tc.name); got != tc.want {
				t.Errorf("SkipDir(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestWalkFiles_Policies_SelectTheirCorpus verifies that each policy visits the
// files it names and no others, that the walk descends into ordinary
// subdirectories, and that it enters none of the pruned trees.
func TestWalkFiles_Policies_SelectTheirCorpus(t *testing.T) {
	root := t.TempDir()
	// sequential: setup steps building one tree, asserted by the walk below
	for _, rel := range []string{
		"widget.go",
		"widget_test.go",
		"README.md",
		"sub/nested_test.go",
		"sub/nested.go",
		"testdata/fixture_test.go",
		"testdata/fixture.go",
		"node_modules/pkg_test.go",
		"dist/built.go",
		".cache/hidden_test.go",
	} {
		writeFile(t, root, rel)
	}

	cases := []struct {
		name   string
		policy Policy
		want   []string
	}{
		{name: "test files", policy: TestFiles, want: []string{"sub/nested_test.go", "widget_test.go"}},
		{name: "non-test go files", policy: NonTestGoFiles, want: []string{"sub/nested.go", "widget.go"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			if err := WalkFiles([]string{root}, tc.policy, func(path string) error {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					return relErr
				}
				got = append(got, filepath.ToSlash(rel))
				return nil
			}); err != nil {
				t.Fatalf("WalkFiles: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("WalkFiles visited %v, want %v", got, tc.want)
			}
			for i, want := range tc.want {
				if got[i] != want {
					t.Errorf("visit %d = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

// TestWalkFiles_Failures_StopAtTheFirstError verifies that a root that does not
// exist and an error the visitor returns both reach the caller, so a command
// reports a corpus it could not read instead of a short one.
func TestWalkFiles_Failures_StopAtTheFirstError(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "widget_test.go")
	sentinel := errors.New("visit failed")

	cases := []struct {
		name  string
		roots []string
		visit func(string) error
		want  error
	}{
		{
			name:  "absent root",
			roots: []string{filepath.Join(root, "absent")},
			visit: func(string) error { return nil },
			want:  os.ErrNotExist,
		},
		{
			name:  "visitor error",
			roots: []string{root},
			visit: func(string) error { return sentinel },
			want:  sentinel,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := WalkFiles(tc.roots, TestFiles, tc.visit); !errors.Is(err, tc.want) {
				t.Errorf("WalkFiles error = %v, want %v", err, tc.want)
			}
		})
	}
}

// writeFile creates an empty file under root at the slash-separated rel,
// creating the directories it needs.
func writeFile(t *testing.T, root, rel string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte("package fixture\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
