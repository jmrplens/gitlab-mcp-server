package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// auditedPattern is a small, real package to run the whole command over. The
// command's own source publishes no action IDs, so the run is a complete one
// that finds nothing, which is exactly the shape a clean run has to take.
const auditedPattern = "./cmd/audit_action_ids/..."

// TestRun_WholeCommand_ReportsAndWritesTheWorkList drives run end to end and
// holds the two things it owes a caller: a report on stdout and the work list
// on disk for the layer that fixes the cross-links.
func TestRun_WholeCommand_ReportsAndWritesTheWorkList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "action-ids.json")
	var stdout, stderr bytes.Buffer

	if code := run(auditConfig{dir: repoRoot(t), patterns: []string{auditedPattern}, jsonPath: path}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), toolName+":") {
		t.Errorf("stdout = %q, want the summary line", stdout.String())
	}
	if !strings.Contains(stdout.String(), "wrote "+path) {
		t.Errorf("stdout = %q, want the work list named", stdout.String())
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("work list: %v", err)
	}
}

// TestRun_NoWorkListPath_WritesNothing holds that an empty -json is a report
// and nothing else, which is how the audit is run over one package without
// overwriting the tree's work list.
func TestRun_NoWorkListPath_WritesNothing(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer

	if code := run(auditConfig{dir: repoRoot(t), patterns: []string{auditedPattern}}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "wrote ") {
		t.Errorf("stdout = %q, want no work list written", stdout.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Errorf("temporary directory holds %d entries (err %v), want none", len(entries), err)
	}
}

// TestRun_UnloadableSource_ExitsOne holds that a run that could not be made is
// a failure rather than a clean report. The audit does not fail on a finding,
// so this is the only thing that can send it home with a 1, and a silent
// success here would be a report over source nobody loaded.
func TestRun_UnloadableSource_ExitsOne(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run(auditConfig{dir: repoRoot(t), patterns: []string{"./cmd/audit_action_ids/nothing/..."}}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), toolName+":") {
		t.Errorf("stderr = %q, want the failure named", stderr.String())
	}
}

// TestRun_UnwritableWorkList_ExitsOne holds that a work list that could not be
// written fails the run, since the layer downstream reads that file and an
// absent one would read as no work to do.
func TestRun_UnwritableWorkList_ExitsOne(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	var stdout, stderr bytes.Buffer

	code := run(auditConfig{dir: repoRoot(t), patterns: []string{auditedPattern}, jsonPath: filepath.Join(blocker, "action-ids.json")}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), toolName+":") {
		t.Errorf("stderr = %q, want the failure named", stderr.String())
	}
}

// TestRun_Check_DeadCrossLink_FailsAndNamesIt drives the gate over a fixture
// that publishes an ID the catalog does not have, which is the whole point of
// the flip: the command reported this and now refuses it. A gate only ever
// exercised on a clean tree is one nobody has watched fail.
func TestRun_Check_DeadCrossLink_FailsAndNamesIt(t *testing.T) {
	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte(`package fixture

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"

// Spec publishes a cross-link to an action the catalog does not have.
var Spec = toolutil.ActionSpecOptions{RelatedActions: []string{"project.no_such_action"}}
`),
	}
	var stdout, stderr bytes.Buffer

	code := run(auditConfig{dir: root, patterns: fixturePatterns, overlay: overlay, check: true}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("run with -check = %d over a dead cross-link, want 1", code)
	}
	if !strings.Contains(stdout.String(), "project.no_such_action") {
		t.Errorf("stdout = %q, want the dead ID named", stdout.String())
	}
	if !strings.Contains(stderr.String(), "ERROR:") {
		t.Errorf("stderr = %q, want the gate's own failure line", stderr.String())
	}
}

// TestRun_Check_FailureLine_CountsEachRefusalUnderItsOwnName drives the gate
// over a fixture that trips three of its four refusals by different amounts,
// and holds the one stderr line a CI log is read by. The line is one Fprintf
// over four counters, and a fixture with one dead ID and nothing else, which
// is what the gate was first watched fail on, cannot tell "2 resolve to no
// action, 1 names an alias, 3 not folded" from the same numbers in another
// order. A prose alias is what fills the second slot, since a Usage line may
// spell one and only the two declared spellings are excused.
func TestRun_Check_FailureLine_CountsEachRefusalUnderItsOwnName(t *testing.T) {
	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte(`package fixture

import (
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Spec publishes two dead cross-links and a Usage line naming a registered
// alias, deploy_key.create, which resolves to access.deploy_key_add.
var Spec = toolutil.ActionSpecOptions{
	RelatedActions: []string{"project.no_such_action", "issue.no_such_action"},
	Usage:          "Chain deploy_key.create after this.",
}

// Unfolded publishes three lists nothing can fold.
var Unfolded = toolutil.ActionSpecOptions{
	RelatedActions: []string{os.Getenv("A"), os.Getenv("B"), os.Getenv("C")},
}
`),
	}
	var stdout, stderr bytes.Buffer

	// The fixture package alone, since the counts are the whole assertion and
	// toolutil publishes sites of its own that would move them.
	code := run(auditConfig{dir: root, patterns: []string{"./" + fixtureDir + "/..."}, overlay: overlay, check: true}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("run with -check = %d, want 1; stdout %q", code, stdout.String())
	}
	const want = "\nERROR: 2 published ID(s) resolve to no action, 1 name a registered alias rather than a catalog ID, 3 site(s) could not be folded, 0 declaration(s) excuse nothing, 0 hint(s) name a tool rather than an action, 0 e2e assertion(s) name a tool rather than an action, 0 assertion helper declaration(s) match no call\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if !strings.Contains(stdout.String(), `usage "deploy_key.create" alias of access.deploy_key_add`) {
		t.Errorf("stdout = %q, want the prose alias named with what it resolves to", stdout.String())
	}
}

// TestRun_Check_CleanPackage_Passes holds the other side of the switch: with
// nothing to refuse, -check is silent and exits 0, so the flag cannot be one
// that fails on everything.
func TestRun_Check_CleanPackage_Passes(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run(auditConfig{dir: repoRoot(t), patterns: []string{auditedPattern}, check: true}, &stdout, &stderr); code != 0 {
		t.Fatalf("run with -check = %d over a package publishing no IDs, stderr %q", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "ERROR:") {
		t.Errorf("stderr = %q, want nothing from a clean run", stderr.String())
	}
}

// TestRun_Check_HintNamingATool_IsReportedAndFails drives the whole command
// over a fixture whose only defect is a hint, and holds the flip end to end:
// the row is in the report, the count is in the summary, and the gate exits
// non-zero.
//
// It passed until the tree was clean, which was the staging: the class opened
// at 785 findings, and a gate refusing them then would have refused every push
// with no run left to measure the tree with. It fails now, which is what stops
// the 786th being written.
func TestRun_Check_HintNamingATool_IsReportedAndFails(t *testing.T) {
	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte(`package fixture

import (
	"errors"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

// Get hands a model a tool name the dynamic surface does not register.
func Get() error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, "verify project_id with gitlab_project_get")
}
`),
	}
	var stdout, stderr bytes.Buffer

	code := run(auditConfig{dir: root, patterns: []string{"./" + fixtureDir + "/..."}, overlay: overlay, check: true, verbose: true}, &stdout, &stderr)

	if code == 0 {
		t.Fatalf("run with -check = 0 over a hint finding, want a refusal; stdout %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "gitlab_project_get") {
		t.Errorf("stdout = %q, want the tool name named", stdout.String())
	}
	if !strings.Contains(stdout.String(), "error hints: 1 finding(s) in 1 package(s) over 1 hint(s) read") {
		t.Errorf("stdout = %q, want the count of what the rule read", stdout.String())
	}
	if !strings.Contains(stderr.String(), "hint(s) name a tool rather than an action") {
		t.Errorf("stderr = %q, want the failure line to name this rule rather than only the count", stderr.String())
	}
}

// TestSplitPatterns_EachSpelling_GoesToItsLoad holds what a run covers: a bare
// run both whole trees, and a run naming patterns exactly what it names, each
// sorted to the load that reads it whatever spelling a caller typed.
//
// go list loads a package by three spellings, and each has a case: the
// relative pattern with its leading ./, in either separator; an absolute path;
// and an import path. The last two used to be sorted as typed, which the
// suite's prefix never matches, so the whole suite named either way went to
// the served load and came back clean with nothing judged. They are read as
// the relative pattern they name, which is also the pattern the load is
// handed. An absolute path outside the root names nothing of either tree and
// is handed on as it was given.
func TestSplitPatterns_EachSpelling_GoesToItsLoad(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name     string
		patterns []string
		served   []string
		suite    []string
	}{
		{name: "none", served: defaultPatterns, suite: defaultSuitePatterns},
		{name: "one served package", patterns: []string{"./internal/tools/issues"}, served: []string{"./internal/tools/issues"}},
		{name: "the served tree named", patterns: []string{"./internal/tools/..."}, served: []string{"./internal/tools/..."}},
		{name: "a suite package", patterns: []string{"./test/e2e/gitlab/ee"}, suite: []string{"./test/e2e/gitlab/ee"}},
		{
			name:     "a suite package in the platform's spelling",
			patterns: []string{platformPattern("test", "e2e", "gitlab", "ee")},
			suite:    []string{platformPattern("test", "e2e", "gitlab", "ee")},
		},
		{
			name:     "both, in the order given",
			patterns: []string{"./test/e2e/gitlab/common", "./internal/tools/issues", "./test/e2e/gitlab/ee", "./cmd/audit_action_ids"},
			served:   []string{"./internal/tools/issues", "./cmd/audit_action_ids"},
			suite:    []string{"./test/e2e/gitlab/common", "./test/e2e/gitlab/ee"},
		},
		{
			name:     "the suite as an absolute path",
			patterns: []string{filepath.Join(root, "test", "e2e", "gitlab", "...")},
			suite:    []string{"./test/e2e/gitlab/..."},
		},
		{
			name:     "a served package as an absolute path",
			patterns: []string{filepath.Join(root, "internal", "tools", "issues")},
			served:   []string{"./internal/tools/issues"},
		},
		{
			name:     "the suite as an import path",
			patterns: []string{goprogram.ModulePath + "/test/e2e/gitlab/..."},
			suite:    []string{"./test/e2e/gitlab/..."},
		},
		{
			name:     "a served package as an import path",
			patterns: []string{goprogram.ModulePath + "/internal/tools/issues"},
			served:   []string{"./internal/tools/issues"},
		},
		{
			name:     "an absolute path outside the root",
			patterns: []string{filepath.Join(filepath.Dir(root), "elsewhere", "test", "e2e", "gitlab", "...")},
			served:   []string{filepath.Join(filepath.Dir(root), "elsewhere", "test", "e2e", "gitlab", "...")},
		},
		{name: "the whole module", patterns: []string{"./..."}, served: []string{"./..."}, suite: defaultSuitePatterns},
		{name: "a tree enclosing the suite", patterns: []string{"./test/..."}, served: []string{"./test/..."}, suite: defaultSuitePatterns},
		{
			name:     "an enclosing tree before a served package",
			patterns: []string{"./...", "./internal/tools/issues"},
			served:   []string{"./...", "./internal/tools/issues"},
			suite:    defaultSuitePatterns,
		},
		{
			name:     "an enclosing tree as an absolute path",
			patterns: []string{filepath.Join(root, "...")},
			served:   []string{"./..."},
			suite:    defaultSuitePatterns,
		},
		{name: "the root's parent as an absolute path", patterns: []string{filepath.Dir(root)}, served: []string{filepath.Dir(root)}},
		{
			name:     "a directory of the root whose name begins with two dots",
			patterns: []string{filepath.Join(root, "..data", "x")},
			served:   []string{"./..data/x"},
		},
		{name: "a single package above the suite", patterns: []string{"./test"}, served: []string{"./test"}},
		{name: "a tree beside the suite", patterns: []string{"./cmd/..."}, served: []string{"./cmd/..."}},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			served, suite := splitPatterns(root, one.patterns)
			if !slices.Equal(served, one.served) || !slices.Equal(suite, one.suite) {
				t.Errorf("splitPatterns(%q) = %q, %q, want %q, %q", one.patterns, served, suite, one.served, one.suite)
			}
		})
	}
	if !slices.Equal(defaultPatterns, []string{"./internal/tools/...", "./internal/toolutil"}) {
		t.Errorf("defaultPatterns = %v, want the tree that publishes action IDs and the helpers its hints are handed to", defaultPatterns)
	}
	if !slices.Equal(defaultSuitePatterns, []string{"./test/e2e/gitlab/..."}) {
		t.Errorf("defaultSuitePatterns = %v, want the suite that quotes it", defaultSuitePatterns)
	}
	if served, suite := splitPatterns(root, defaultSuitePatterns); len(served) != 0 || !slices.Equal(suite, defaultSuitePatterns) {
		t.Errorf("the default suite named explicitly splits into %q and %q, want all of it read as suite", served, suite)
	}
}

// TestRun_Check_SuiteNamedByAbsolutePathOrImportPath_IsJudgedAsTheSuite drives
// the whole command over the planted suite named in the two spellings that
// used to be sorted to the served load, which read three doc.go files, judged
// nothing and exited 0 over a quotation naming a tool. Named either way, the
// run reads the quotation and the gate fails on it.
func TestRun_Check_SuiteNamedByAbsolutePathOrImportPath_IsJudgedAsTheSuite(t *testing.T) {
	root := repoRoot(t)
	overlay := suiteOverlay(t, map[string]string{"planted_test.go": plantedToDoRefusal})
	for _, one := range []struct {
		name    string
		pattern string
	}{
		{name: "an absolute path", pattern: filepath.Join(root, filepath.FromSlash(suiteFixtureDir), "...")},
		{name: "an import path", pattern: goprogram.ModulePath + "/" + suiteFixtureDir + "/..."},
	} {
		t.Run(one.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := run(auditConfig{dir: root, patterns: []string{one.pattern}, overlay: overlay, check: true}, &stdout, &stderr)

			if code != 1 {
				t.Fatalf("run with -check = %d over a quotation naming a tool, want 1; stdout %q", code, stdout.String())
			}
			if want := "  e2e assertions: 1 finding(s) in 1 package(s) over 1 assertion(s) read;"; !strings.Contains(stdout.String(), want) {
				t.Errorf("stdout = %q, want %q", stdout.String(), want)
			}
		})
	}
}

// TestRun_NoWorkingDirectory_ExitsOne holds the one failure resolving the root
// can have: filepath.Abs fails only when the process has no working
// directory, which is reachable only by replacing it, and the run stops there
// rather than sorting patterns against a root it does not have.
func TestRun_NoWorkingDirectory_ExitsOne(t *testing.T) {
	restore := absolutePath
	absolutePath = func(string) (string, error) { return "", errors.New("no working directory") }
	t.Cleanup(func() { absolutePath = restore })
	var stdout, stderr bytes.Buffer

	if code := run(auditConfig{dir: ".", patterns: []string{auditedPattern}}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d without a working directory, want 1", code)
	}
	if want := "audit_action_ids: no working directory\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing reported for a run that could not start", stdout.String())
	}
}

// TestNamesWhole_EachSpelling_JudgesTheTableOnlyOverTheWholeTree holds the
// condition a declaration table is judged under: the run covered the whole
// tree the table describes, in any spelling go list loads. Compared
// literally, the suite named with Windows' separator, .\test\e2e\gitlab\...,
// loaded every package of it and left the helper table unjudged, which is a
// clean report over a run that never asked the question; and a part of the
// tree, or the tree plus something beside it, is a run whose unused entries
// say nothing about the table.
//
// A relative pattern without the leading ./ has no case here, because no run
// reaches this comparison with one: go list reads it as an import path, which
// matches no package of this module, and the load refuses the run first. An
// absolute path or an import path has none either, since splitPatterns has
// already read it as the relative pattern it names.
func TestNamesWhole_EachSpelling_JudgesTheTableOnlyOverTheWholeTree(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		whole    []string
		want     bool
	}{
		{name: "the suite as the bare run names it", patterns: []string{"./test/e2e/gitlab/..."}, whole: defaultSuitePatterns, want: true},
		{
			name:     "the suite in the platform's spelling",
			patterns: []string{platformPattern("test", "e2e", "gitlab", "...")},
			whole:    defaultSuitePatterns,
			want:     true,
		},
		{
			name:     "the served tree in the platform's spelling",
			patterns: []string{platformPattern("internal", "tools", "..."), platformPattern("internal", "toolutil")},
			whole:    defaultPatterns,
			want:     true,
		},
		{name: "the served tree without the helpers it hands hints to", patterns: []string{"./internal/tools/..."}, whole: defaultPatterns},
		{
			name:     "the served tree in another order",
			patterns: []string{"./internal/toolutil", "./internal/tools/..."},
			whole:    defaultPatterns,
			want:     true,
		},
		{
			name:     "the served tree and a package it encloses",
			patterns: []string{"./internal/tools/...", "./internal/tools/issues", "./internal/toolutil"},
			whole:    defaultPatterns,
			want:     true,
		},
		{name: "one package of the suite", patterns: []string{"./test/e2e/gitlab/ee"}, whole: defaultSuitePatterns},
		{
			name:     "the suite and a package it encloses",
			patterns: []string{"./test/e2e/gitlab/...", "./test/e2e/gitlab/ee"},
			whole:    defaultSuitePatterns,
			want:     true,
		},
		{
			name:     "the suite and a package it encloses, that package first",
			patterns: []string{"./test/e2e/gitlab/ee", "./test/e2e/gitlab/..."},
			whole:    defaultSuitePatterns,
			want:     true,
		},
		{
			name:     "the suite and the directory its wildcard names",
			patterns: []string{"./test/e2e/gitlab", "./test/e2e/gitlab/..."},
			whole:    defaultSuitePatterns,
			want:     true,
		},
		{
			name:     "the suite and a tree below it",
			patterns: []string{"./test/e2e/gitlab/...", "./test/e2e/gitlab/ee/..."},
			whole:    defaultSuitePatterns,
			want:     true,
		},
		{name: "the suite twice", patterns: []string{"./test/e2e/gitlab/...", "./test/e2e/gitlab/..."}, whole: defaultSuitePatterns, want: true},
		{
			name:     "the suite and a package beside it",
			patterns: []string{"./test/e2e/gitlab/...", "./test/e2e/internal/harness"},
			whole:    defaultSuitePatterns,
		},
		{
			name:     "the suite and a directory whose name only begins like it",
			patterns: []string{"./test/e2e/gitlab/...", "./test/e2e/gitlabx"},
			whole:    defaultSuitePatterns,
		},
		{name: "a package of the suite twice", patterns: []string{"./test/e2e/gitlab/ee", "./test/e2e/gitlab/ee"}, whole: defaultSuitePatterns},
		{
			// These load what the suite's wildcard loads, and the patterns
			// are compared, not the packages: no wildcard of the list
			// encloses the others, so the run is not judged whole.
			name:     "the suite's packages one by one",
			patterns: []string{"./test/e2e/gitlab/ce", "./test/e2e/gitlab/common", "./test/e2e/gitlab/ee"},
			whole:    defaultSuitePatterns,
		},
		{name: "nothing", whole: defaultSuitePatterns},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			if got := namesWhole(one.patterns, one.whole); got != one.want {
				t.Errorf("namesWhole(%q, %q) = %t, want %t", one.patterns, one.whole, got, one.want)
			}
		})
	}
}

// TestNamesWhole_AWildcardBesideASuitePackage_JudgesTheSuiteWhole holds the
// two halves together, as run does: a wildcard enclosing the suite brings the
// whole of it into the suite load, so naming a suite package beside it, or the
// suite itself again, loads the suite and nothing else and is judged whole.
// Compared as listed, both runs loaded the whole suite and reported the helper
// table unjudged. A suite package outside ./test/e2e/gitlab/... is a load over
// more than the suite, and stays unjudged.
func TestNamesWhole_AWildcardBesideASuitePackage_JudgesTheSuiteWhole(t *testing.T) {
	root := repoRoot(t)
	cases := []struct {
		name     string
		patterns []string
		want     bool
	}{
		{name: "the module and a suite package", patterns: []string{"./...", "./test/e2e/gitlab/ee"}, want: true},
		{name: "the module and the suite", patterns: []string{"./...", "./test/e2e/gitlab/..."}, want: true},
		{name: "a tree enclosing the suite and a suite package", patterns: []string{"./test/...", "./test/e2e/gitlab/common"}, want: true},
		{name: "the module and the harness", patterns: []string{"./...", "./test/e2e/internal/harness"}},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			_, suite := splitPatterns(root, one.patterns)
			if got := namesWhole(suite, defaultSuitePatterns); got != one.want {
				t.Errorf("namesWhole(%q) after splitting %q = %t, want %t", suite, one.patterns, got, one.want)
			}
		})
	}
}

// platformPattern spells a relative pattern the way a caller on this platform
// types it: the leading dot go list needs to read it as a path, and the
// platform's separator, so on Windows it is .\test\e2e\gitlab\ee. On a
// platform whose separator is the slash it is the pattern the code declares.
func platformPattern(elems ...string) string {
	return "." + string(filepath.Separator) + filepath.Join(elems...)
}

// plantedToDoRefusal is the line issue 901 was made of, as the suite wrote it
// before the fix: a quotation of a hint that had named a tool until issue 883
// moved it to the action's catalog ID, so the test went on asserting the old
// spelling and failed only when the licensed run reached it a month later.
const plantedToDoRefusal = `//go:build e2e

package actionidsfixture

import "testing"

func TestToDo(t *testing.T) {
	refusal := "the refusal"
	assertMentions(t, "the refusal of a second to-do", refusal, "gitlab_todo_list")
}
`

// TestRun_Check_SuiteAssertionNamingATool_FailsAndNamesIt drives the whole
// command over a suite whose only defect is issue 901's line, and holds the
// gate that would have caught it on the push that introduced it: the row
// names the file, the line and the needle, the count is in the report, and
// the failure line names the rule. The run passes no -v, as check-action-ids
// does not, so the row is what a red job's log carries.
func TestRun_Check_SuiteAssertionNamingATool_FailsAndNamesIt(t *testing.T) {
	overlay := suiteOverlay(t, map[string]string{"planted_test.go": plantedToDoRefusal})
	var stdout, stderr bytes.Buffer

	code := run(auditConfig{dir: repoRoot(t), patterns: []string{suiteFixturePattern}, overlay: overlay, check: true}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("run with -check = %d over a quotation naming a tool, want 1; stdout %q", code, stdout.String())
	}
	for _, want := range []string{
		assertionRowsHeading + "\n=== " + suiteFixtureDir + " ===\n",
		`  ` + suiteFixtureDir + `/planted_test.go:9 assertion "gitlab_todo_list" is a tool name; the dynamic surface registers no such tool` + "\n",
		"  e2e assertions: 1 finding(s) in 1 package(s) over 1 assertion(s) read; 0 not folded (reported, not gated)\n",
	} {
		t.Run(strings.TrimSpace(want), func(t *testing.T) {
			if !strings.Contains(stdout.String(), want) {
				t.Errorf("stdout = %q, want %q", stdout.String(), want)
			}
		})
	}
	if !strings.Contains(stderr.String(), "1 e2e assertion(s) name a tool rather than an action") {
		t.Errorf("stderr = %q, want the failure line to name the suite's rule", stderr.String())
	}
}

// TestRun_Check_SuiteQuotingOnlyActionIDs_Passes holds the other side: the
// same line quoting the catalog ID, as issue 906 left it, beside prose and a
// dotted file name, passes. Two of the planted helpers are called nowhere,
// and a run over part of the suite does not hold that against the table.
func TestRun_Check_SuiteQuotingOnlyActionIDs_Passes(t *testing.T) {
	overlay := suiteOverlay(t, map[string]string{"planted_test.go": `//go:build e2e

package actionidsfixture

import "testing"

func TestToDo(t *testing.T) {
	refusal := "the refusal"
	assertMentions(t, "the refusal of a second to-do", refusal, "user.todo_list", "already has a to-do", ".gitlab-ci.yml")
	if !mentionsAny(refusal, "issue.create", "not found") {
		t.Error("absent")
	}
}
`})
	var stdout, stderr bytes.Buffer

	code := run(auditConfig{dir: repoRoot(t), patterns: []string{suiteFixturePattern}, overlay: overlay, check: true}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run with -check = %d over canonical quotations, want 0; stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "  e2e assertions: 0 finding(s) in 0 package(s) over 5 assertion(s) read;") {
		t.Errorf("stdout = %q, want the five needles counted as read", stdout.String())
	}
}

// TestRun_Check_FailureLine_CountsTheSuiteRefusalsApart holds the two counts
// the suite adds to the one stderr line a CI log is read by, with figures that
// differ so a swap between them shows.
func TestRun_Check_FailureLine_CountsTheSuiteRefusalsApart(t *testing.T) {
	overlay := suiteOverlay(t, map[string]string{
		"planted_test.go": `//go:build e2e

package actionidsfixture

import "testing"

func TestTwoTools(t *testing.T) {
	assertMentions(t, "a refusal", "text", "gitlab_issue_list", "gitlab_project_get")
}
`,
		"renamed/doc.go": "// Package renamed renames a helper's parameter.\npackage renamed\n",
		"renamed/renamed_test.go": `//go:build e2e

package renamed

import "testing"

func containsAny(text string, wants ...string) bool { return len(wants) > 0 && text != "" }

func TestRenamed(t *testing.T) {
	if !containsAny("text", "not read") {
		t.Error("absent")
	}
}
`,
	})
	var stdout, stderr bytes.Buffer

	code := run(auditConfig{dir: repoRoot(t), patterns: []string{suiteFixturePattern}, overlay: overlay, check: true}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("run with -check = %d, want 1; stdout %q", code, stdout.String())
	}
	const want = "\nERROR: 0 published ID(s) resolve to no action, 0 name a registered alias rather than a catalog ID, 0 site(s) could not be folded, 0 declaration(s) excuse nothing, 0 hint(s) name a tool rather than an action, 2 e2e assertion(s) name a tool rather than an action, 1 assertion helper declaration(s) match no call\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	const staleRow = "=== assertion helpers that describe no call ===\n" +
		"  test/e2e/actionidsfixture/renamed: containsAny takes no parameter named needles (servedTextAssertions). " +
		"The entry names one parameter for every copy of containsAny, so rename this copy's parameter to needles, " +
		"or the entry and every copy together.\n"
	if !strings.Contains(stdout.String(), staleRow) {
		t.Errorf("stdout = %q, want the mismatched helper printed without -v", stdout.String())
	}
}

// TestRun_ServedPatternsOnly_PrintsNoAssertionSection holds what a run over
// the served tree alone says about the suite, which is nothing: it loaded
// none, and a count over nothing would read as a clean suite. The work list
// says the same thing in the one field about it.
func TestRun_ServedPatternsOnly_PrintsNoAssertionSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "action-ids.json")
	var stdout, stderr bytes.Buffer

	if code := run(auditConfig{dir: repoRoot(t), patterns: []string{auditedPattern}, jsonPath: path, verbose: true}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr %q", code, stderr.String())
	}
	for _, unwanted := range []string{"e2e assertions", "assertion helpers", "assertion calls by helper"} {
		t.Run(unwanted, func(t *testing.T) {
			if strings.Contains(stdout.String(), unwanted) {
				t.Errorf("stdout = %q, want nothing about a suite the run did not load", stdout.String())
			}
		})
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the work list: %v", err)
	}
	if !strings.Contains(string(data), `"suite_judged": false`) {
		t.Errorf("work list = %s, want suite_judged false", data)
	}
}

// TestRun_SuitePatternsOnly_LoadsNoServedTree holds the other narrowing: a run
// naming a suite package reads that package and no served source, so it says
// the published-ID and hint rules were not run rather than printing their
// counts over nothing, which read as a clean tree, while its suite section is
// printed and counted, and both the report and the work list say the served
// tree was not judged and the helper table was not judged whole: over one
// package an empty stale list says nothing about the entries it does not call.
func TestRun_SuitePatternsOnly_LoadsNoServedTree(t *testing.T) {
	overlay := suiteOverlay(t, map[string]string{"planted_test.go": `//go:build e2e

package actionidsfixture

import "testing"

func TestOne(t *testing.T) {
	assertMentions(t, "a refusal", "text", "issue.create")
}
`})
	var stdout, stderr bytes.Buffer
	path := filepath.Join(t.TempDir(), "action-ids.json")

	if code := run(auditConfig{dir: repoRoot(t), patterns: []string{suiteFixturePattern}, overlay: overlay, jsonPath: path, verbose: true}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr %q", code, stderr.String())
	}
	for _, want := range []string{
		toolName + ": no served source loaded: the published-ID and hint rules were not run\n",
		"  e2e assertions: 0 finding(s) in 0 package(s) over 1 assertion(s) read;",
		"  the assertion helper table was not judged whole: only a run over the whole suite can tell an entry nothing calls from a narrowed run\n",
		"    assertion calls by helper: assertMentions 1\n",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(stdout.String(), want) {
				t.Errorf("stdout = %q, want %q", stdout.String(), want)
			}
		})
	}
	for _, unwanted := range []string{"published ID(s)", "error hints:", "the declaration tables were not judged"} {
		t.Run(unwanted, func(t *testing.T) {
			if strings.Contains(stdout.String(), unwanted) {
				t.Errorf("stdout = %q, want nothing about a served tree the run did not load", stdout.String())
			}
		})
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the work list: %v", err)
	}
	for _, key := range []string{`"served_judged": false`, `"helpers_judged": false`} {
		t.Run(key, func(t *testing.T) {
			if !strings.Contains(string(data), key) {
				t.Errorf("work list = %s, want %s", data, key)
			}
		})
	}
}

// TestRun_ASuiteThatDoesNotLoad_ExitsOne holds that a suite the load refuses
// fails the run, as a served tree that does not load does: a report over a
// suite nobody read would read as a clean one.
func TestRun_ASuiteThatDoesNotLoad_ExitsOne(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run(auditConfig{dir: repoRoot(t), patterns: []string{"./" + suiteFixtureDir + "/nothing/..."}}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), toolName+":") {
		t.Errorf("stderr = %q, want the failure named", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want no report over a suite that did not load", stdout.String())
	}
}

// TestRun_FixHints_NeverLoadsTheSuite holds the one mode that must leave the
// suite alone: it rewrites what the server writes, and a suite pattern beside
// a served one is neither read nor rewritten. The planted suite does not
// type-check, which is what shows it was never loaded.
func TestRun_FixHints_NeverLoadsTheSuite(t *testing.T) {
	root := repoRoot(t)
	overlay := suiteOverlay(t, map[string]string{"planted_test.go": "//go:build e2e\n\npackage actionidsfixture\n\nfunc broken() { undefined() }\n"})
	overlay[filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go")] = []byte("package fixture\n\n// Usage names an action.\nconst Usage = \"Read one with issue.get.\"\n")
	var stdout, stderr bytes.Buffer

	code := run(auditConfig{dir: root, patterns: []string{"./" + fixtureDir + "/...", suiteFixturePattern}, overlay: overlay, fixHints: true}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run with -fix-hints = %d, want 0 without loading the suite; stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "rewrote 0 tool name(s) in 0 file(s)") {
		t.Errorf("stdout = %q, want the report of a run with nothing to move", stdout.String())
	}
}

// TestRun_FixHints_ReportsWhatMovedAndJudgesNothing holds the shape of the
// fixing mode: it rewrites and reports, and it does not also answer the
// question -check answers.
//
// The two are deliberately not one flag. The gate says whether the tree is
// clean and this changes the tree, so a flag that could do either is one
// somebody eventually runs in CI.
func TestRun_FixHints_ReportsWhatMovedAndJudgesNothing(t *testing.T) {
	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte(`package fixture

// Usage names an action and nothing hands a model a hint.
const Usage = "Read one with issue.get."
`),
	}
	var stdout, stderr bytes.Buffer

	code := run(auditConfig{dir: root, patterns: []string{"./" + fixtureDir + "/..."}, overlay: overlay, fixHints: true}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run with -fix-hints = %d, want 0; stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "rewrote 0 tool name(s) in 0 file(s)") {
		t.Errorf("stdout = %q, want the report of a run with nothing to move", stdout.String())
	}
	if strings.Contains(stdout.String(), "published ID(s) to fix") {
		t.Errorf("stdout = %q, want the fixing run to report rather than judge", stdout.String())
	}
}

// TestRun_FixHints_ASourceItCannotRewrite_FailsTheRun holds the failure this
// mode must not absorb. A rewrite that could not open what it was asked to
// rewrite has not done the job, and a run reporting success there would leave
// a tree half moved with a clean line above it.
//
// The arrangement is the fixture tree itself, which lives in an overlay and
// not on disk: the walk folds its hints and the rewrite then has no directory
// to open.
func TestRun_FixHints_ASourceItCannotRewrite_FailsTheRun(t *testing.T) {
	root := repoRoot(t)
	overlay := map[string][]byte{
		filepath.Join(root, filepath.FromSlash(fixtureDir), "fixture.go"): []byte(`package fixture

import (
	"errors"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

var errDemo = errors.New("demo")

// Get hands a model a tool name the dynamic surface does not register.
func Get() error {
	return toolutil.WrapErrWithHint("demo_get", errDemo, "verify project_id with gitlab_project_get")
}
`),
	}
	var stdout, stderr bytes.Buffer

	code := run(auditConfig{dir: root, patterns: []string{"./" + fixtureDir + "/..."}, overlay: overlay, fixHints: true}, &stdout, &stderr)

	if code == 0 {
		t.Fatalf("run with -fix-hints = 0 over source it cannot open, want a refusal; stdout %q", stdout.String())
	}
	// The path is compared in the spelling the platform prints. The message
	// carries what the filesystem was asked for, which is separated by
	// backslashes on Windows, so a slash-separated expectation would pass on
	// Unix and fail there for the separator rather than for the message.
	if !strings.Contains(stderr.String(), filepath.FromSlash(fixtureDir)) {
		t.Errorf("stderr = %q, want it to name what could not be read", stderr.String())
	}
}

// TestRun_ACatalogItCannotBuild_IsReportedAndStops holds the first thing the
// run does and the only way it can fail before any source is read.
//
// Building the catalog needs no network and no credentials, so nothing a test
// can arrange makes it fail; the seam is what makes the branch reachable, and
// the branch matters because a run that reported no findings after failing to
// build the oracle would read as a clean tree.
func TestRun_ACatalogItCannotBuild_IsReportedAndStops(t *testing.T) {
	restore := buildCatalogIDs
	buildCatalogIDs = func() (*actionids.IDs, error) { return nil, errors.New("no catalog today") }
	defer func() { buildCatalogIDs = restore }()

	var stdout, stderr bytes.Buffer
	if code := run(auditConfig{dir: "."}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no catalog today") {
		t.Errorf("stderr = %q, want the reason the catalog could not be built", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing printed before the catalog exists", stdout.String())
	}
}
