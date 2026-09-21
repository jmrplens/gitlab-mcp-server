package testsource

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

// TestClassifyTestName_Buckets_KeepTheirPublishedSpellings pins the four bucket
// names to the strings they are published under, which no test comparing a
// classification against its own constant can do. The naming auditor prints
// these verbatim as the labels of its summary and the testing reference joins
// its table rows on them, so two of them trading values would relabel every
// count in both while every other test here stayed green.
func TestClassifyTestName_Buckets_KeepTheirPublishedSpellings(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "three part", input: "TestCreate_ValidInput_ReturnsIssue", want: "3-part"},
		{name: "two part", input: "TestCreate_ReturnsIssue", want: "2-part"},
		{name: "no underscore", input: "TestCreateIssue", want: "no-underscore"},
		{name: "coverage helper", input: "TestCovBuildCatalog", want: "TestCov"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyTestName(tc.input); got != tc.want {
				t.Errorf("ClassifyTestName(%q) = %q, want the published spelling %q", tc.input, got, tc.want)
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

// TestWalkFiles_PrunedRoot_IsStillWalked verifies that the skip list judges
// what lies below a root and never the root itself: a scan pointed straight at
// a fixtures or dot directory scans it, and prunes one of the same name found
// inside it.
func TestWalkFiles_PrunedRoot_IsStillWalked(t *testing.T) {
	base := t.TempDir()
	// sequential: setup steps building one tree, asserted by the walks below
	for _, rel := range []string{
		"testdata/fixture_test.go",
		"testdata/testdata/deeper_test.go",
		".github/hook_test.go",
	} {
		writeFile(t, base, rel)
	}

	cases := []struct {
		name string
		root string
		want []string
	}{
		{name: "fixtures root", root: "testdata", want: []string{"fixture_test.go"}},
		{name: "dot root", root: ".github", want: []string{"hook_test.go"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(base, tc.root)
			var got []string
			if err := WalkFiles([]string{root}, TestFiles, func(path string) error {
				got = append(got, filepath.Base(path))
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

// TestWalkFiles_NestedCheckout_IsNotPartOfTheCorpus verifies that a git
// worktree or clone living inside the tree contributes none of its files, and
// that a root which is itself a checkout is still walked.
//
// The name rule already prunes the .claude/worktrees the parallel-agent
// tooling uses, so the case that matters here is the worktree under an
// ordinary name: its files parse, they are real test files, and folding them
// in would move every count this corpus feeds without saying so.
func TestWalkFiles_NestedCheckout_IsNotPartOfTheCorpus(t *testing.T) {
	base := t.TempDir()
	// sequential: setup steps building one tree, asserted by the walks below
	writeFile(t, base, "widget_test.go")
	writeFile(t, base, "scratch/copy_test.go")
	writeFile(t, base, "scratch/.git")
	writeFile(t, base, ".claude/worktrees/agent-1/other_test.go")
	writeFile(t, base, ".claude/worktrees/agent-1/.git")

	cases := []struct {
		name string
		root string
		want []string
	}{
		{name: "the tree above them", root: ".", want: []string{"widget_test.go"}},
		{name: "a checkout named as the root", root: "scratch", want: []string{"copy_test.go"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			if err := WalkFiles([]string{filepath.Join(base, tc.root)}, TestFiles, func(path string) error {
				got = append(got, filepath.Base(path))
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

// TestWalkFiles_SymlinkedRoot_IsFollowedAndReportedUnderItsOwnName verifies
// that a root which is a symlink to a directory is walked as that directory,
// with every path reported under the name the caller gave. filepath.WalkDir
// lstats its root, so without resolving it the walk would visit the link name
// alone and hand back an empty corpus, which a gate would read as a clean
// tree.
func TestWalkFiles_SymlinkedRoot_IsFollowedAndReportedUnderItsOwnName(t *testing.T) {
	base := t.TempDir()
	// sequential: setup steps building one tree, asserted by the walk below
	writeFile(t, base, "real/widget_test.go")
	writeFile(t, base, "real/sub/nested_test.go")
	writeFile(t, base, "real/testdata/fixture_test.go")
	link := filepath.Join(base, "link")
	if err := os.Symlink(filepath.Join(base, "real"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	var got []string
	if err := WalkFiles([]string{link}, TestFiles, func(path string) error {
		got = append(got, filepath.ToSlash(path))
		return nil
	}); err != nil {
		t.Fatalf("WalkFiles: %v", err)
	}

	want := []string{
		filepath.ToSlash(filepath.Join(link, "sub", "nested_test.go")),
		filepath.ToSlash(filepath.Join(link, "widget_test.go")),
	}
	if !slices.Equal(got, want) {
		t.Errorf("WalkFiles visited %v, want %v", got, want)
	}
}

// TestWalkFiles_SymlinkedRootTargets_DecideWhatIsWalked verifies the two
// remaining shapes a symlinked root can take: a link to a regular file is
// walked as the file it is, so a link named after a test file is visited and
// one named otherwise is not, and a link that resolves to nothing is an error
// rather than an empty corpus.
func TestWalkFiles_SymlinkedRootTargets_DecideWhatIsWalked(t *testing.T) {
	base := t.TempDir()
	// sequential: setup steps building one tree, asserted by the walks below
	writeFile(t, base, "widget_test.go")
	links := map[string]string{
		"file_test.go": filepath.Join(base, "widget_test.go"),
		"plain.go":     filepath.Join(base, "widget_test.go"),
		"broken":       filepath.Join(base, "absent"),
	}
	// sequential: setup steps building one tree, asserted by the walks below
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(base, name)); err != nil {
			t.Fatalf("symlink %s: %v", name, err)
		}
	}

	cases := []struct {
		name    string
		root    string
		want    int
		wantErr error
	}{
		{name: "link to a test file", root: "file_test.go", want: 1},
		{name: "link to a file under another name", root: "plain.go"},
		{name: "broken link", root: "broken", wantErr: os.ErrNotExist},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			visits := 0
			err := WalkFiles([]string{filepath.Join(base, tc.root)}, TestFiles, func(string) error {
				visits++
				return nil
			})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("WalkFiles error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("WalkFiles: %v", err)
			}
			if visits != tc.want {
				t.Errorf("WalkFiles visited %d file(s), want %d", visits, tc.want)
			}
		})
	}
}

// TestWalkFiles_UncleanRoot_ReportsPathsThatNameTheFileVisited verifies that
// the caller-facing rebuild is confined to a root the walk did not itself use.
// Below a root WalkDir walks directly, every path must arrive exactly as WalkDir
// spelled it, because rebuilding it there would join the root onto a path that
// already carries it: a root spelled with a redundant element, which no
// filepath.Join produces and every caller may write, would then hand the visitor
// a path naming nothing. Each visited path is therefore stated, not compared,
// since a gate's next act is to open it.
func TestWalkFiles_UncleanRoot_ReportsPathsThatNameTheFileVisited(t *testing.T) {
	base := t.TempDir()
	// sequential: setup steps building one tree, asserted by the walk below
	writeFile(t, base, "tree/widget_test.go")
	writeFile(t, base, "tree/sub/nested_test.go")

	sep := string(filepath.Separator)
	root := base + sep + "." + sep + "tree"

	var got []string
	if err := WalkFiles([]string{root}, TestFiles, func(path string) error {
		if _, statErr := os.Stat(path); statErr != nil {
			return fmt.Errorf("visited path %q names no file: %w", path, statErr)
		}
		got = append(got, filepath.Base(path))
		return nil
	}); err != nil {
		t.Fatalf("WalkFiles: %v", err)
	}

	want := []string{"nested_test.go", "widget_test.go"}
	if !slices.Equal(got, want) {
		t.Errorf("WalkFiles visited %v, want %v", got, want)
	}
}

// TestWalkFiles_NestedCheckoutBelowASymlinkedRoot_IsPruned verifies that the
// checkout rule still applies below a root reached through a symlink. The rule
// asks the filesystem rather than the name, so it has to be asked about the path
// the walk is really at; the tree below a symlinked root is the one place where
// that path and the name reported to the caller are spelled differently.
func TestWalkFiles_NestedCheckoutBelowASymlinkedRoot_IsPruned(t *testing.T) {
	base := t.TempDir()
	// sequential: setup steps building one tree, asserted by the walk below
	writeFile(t, base, "real/widget_test.go")
	writeFile(t, base, "real/scratch/copy_test.go")
	writeFile(t, base, "real/scratch/.git")
	link := filepath.Join(base, "link")
	if err := os.Symlink(filepath.Join(base, "real"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	var got []string
	if err := WalkFiles([]string{link}, TestFiles, func(path string) error {
		got = append(got, filepath.ToSlash(path))
		return nil
	}); err != nil {
		t.Fatalf("WalkFiles: %v", err)
	}

	want := []string{filepath.ToSlash(filepath.Join(link, "widget_test.go"))}
	if !slices.Equal(got, want) {
		t.Errorf("WalkFiles visited %v, want %v", got, want)
	}
}

// TestWalkFiles_SymlinkedRootThatCannotBeStated_IsWalkedAsTheLink verifies the
// branch a filesystem the tests own never produces: a root whose symlink
// resolves and whose target then refuses to be stated. Resolution has already
// succeeded there, so the answer cannot be an error, and it must not be a nil
// dereference either — the walk falls back to the link itself, exactly as it
// does for a link to a plain file.
func TestWalkFiles_SymlinkedRootThatCannotBeStated_IsWalkedAsTheLink(t *testing.T) {
	base := t.TempDir()
	// sequential: setup steps building one tree, asserted by the walk below
	writeFile(t, base, "real/widget_test.go")
	link := filepath.Join(base, "link_test.go")
	if err := os.Symlink(filepath.Join(base, "real"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	original := stat
	t.Cleanup(func() { stat = original })
	stat = func(string) (os.FileInfo, error) { return nil, errors.New("permission denied") }

	var got []string
	if err := WalkFiles([]string{link}, TestFiles, func(path string) error {
		got = append(got, filepath.ToSlash(path))
		return nil
	}); err != nil {
		t.Fatalf("WalkFiles: %v", err)
	}

	want := []string{filepath.ToSlash(link)}
	if !slices.Equal(got, want) {
		t.Errorf("WalkFiles visited %v, want %v", got, want)
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

// TestWalkFiles_UnreadableDirectory_StopsAndReports verifies the half of the
// stop-at-the-first-error contract no visitor can raise: a directory below the
// root whose contents the process may not list. The walk must surface that
// error rather than skip the directory, because a caller that only sees the
// files collected before it cannot tell a clean tree from an unread one.
//
// Directory permissions are a POSIX mechanism, and a sufficiently privileged
// process ignores them, so the test skips when the chmod did not in fact deny
// the read.
func TestWalkFiles_UnreadableDirectory_StopsAndReports(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory read permission is not a Windows file mode")
	}
	root := t.TempDir()
	writeFile(t, root, "aaa_test.go")
	writeFile(t, root, "locked/deep_test.go")
	writeFile(t, root, "zzz_test.go")

	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("chmod %s: %v", locked, err)
	}
	// t.TempDir removes the tree itself, and cannot descend into a directory
	// left unreadable.
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) }) //#nosec G302 -- a directory needs its search bit back or t.TempDir cannot remove it
	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("this process reads a 0o000 directory; permissions cannot be tested here")
	}

	var visited []string
	err := WalkFiles([]string{root}, TestFiles, func(path string) error {
		visited = append(visited, filepath.Base(path))
		return nil
	})
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("WalkFiles error = %v, want a permission error", err)
	}
	if slices.Contains(visited, "zzz_test.go") {
		t.Errorf("visited = %v, want the walk stopped at the unreadable directory", visited)
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
