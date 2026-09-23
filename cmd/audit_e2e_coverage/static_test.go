package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// fakeHarnessPath is the harness import path inside the fixture module.
const fakeHarnessPath = "example.com/e2efake/test/e2e/internal/harness"

// fakeCatalog is the catalog the fixture's planted tests are judged against.
var fakeCatalog = map[string]edition.Tier{
	"issue.list":         edition.Free,
	"issue.get":          edition.Free,
	"project.get":        edition.Free,
	"project.list":       edition.Free,
	"server.status":      edition.Free,
	"merge_train.list":   edition.Premium,
	"vulnerability.list": edition.Ultimate,
	"vulnerability.get":  edition.Ultimate,
}

// fakeModuleDir is the fixture module the static gate runs against.
func fakeModuleDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "static"))
	if err != nil {
		t.Fatalf("fixture module: %v", err)
	}
	return dir
}

// fakeStaticConfig is the gate's configuration for the fixture module.
func fakeStaticConfig(t *testing.T) staticConfig {
	t.Helper()
	return staticConfig{
		dir: fakeModuleDir(t), harnessPath: fakeHarnessPath, patterns: staticPatterns,
		catalog: fakeCatalog, exemptions: map[string]actionExemption{},
	}
}

// runFakeStatic runs the gate over the fixture module once per test that
// needs it.
func runFakeStatic(t *testing.T, cfg staticConfig) *staticResult {
	t.Helper()
	result, err := runStatic(cfg)
	if err != nil {
		t.Fatalf("runStatic() error = %v, want nil", err)
	}
	if result.Skipped {
		t.Fatal("runStatic() skipped the fixture module")
	}
	return result
}

// findingLines spells the findings of one kind, positions stripped to the
// file so a line number shift does not fail the test.
func findingLines(result *staticResult, kind string) []string {
	var lines []string
	for _, finding := range result.Findings {
		if finding.Kind != kind {
			continue
		}
		file := finding.Pos
		if colon := strings.LastIndex(file, ":"); colon >= 0 {
			file = file[:colon]
		}
		lines = append(lines, strings.TrimSpace(file+" "+finding.Message))
	}
	sort.Strings(lines)
	return lines
}

// TestRunStatic_PlantedDefects_EachReported verifies that the fixture module
// produces exactly the findings its planted defects call for, kind by kind,
// and that the clean shapes beside them produce none: a typed constant, a
// constant through a helper parameter, a table of constants, a declaration
// made through one helper and through two, a helper cycle, a Premium id in
// ee, a tier declared beside requirements that are not tiers, and a method of
// an interface literal on the path to an Ultimate site. The Ultimate findings
// cover the three ways a test can reach an id without declaring the tier:
// through one helper, through two, and through a pointer-receiver method, and
// the one way a Tier call declares nothing, which is outside Needs. The
// discards cover a result thrown away through a helper's parameter, which is
// a finding that cannot name the id.
func TestRunStatic_PlantedDefects_EachReported(t *testing.T) {
	result := runFakeStatic(t, fakeStaticConfig(t))

	cases := []struct {
		kind string
		want []string
	}{
		{kind: findingUnknownID, want: []string{
			"test/e2e/gitlab/common/planted_test.go alpha.action is not a catalog action",
			"test/e2e/gitlab/common/planted_test.go nope.action is not a catalog action",
			"test/e2e/gitlab/common/planted_test.go zeta.action is not a catalog action",
		}},
		{kind: findingTierPlacement, want: []string{
			"test/e2e/gitlab/common/planted_test.go merge_train.list is premium and common runs on every runtime",
		}},
		{kind: findingDiscardedResult, want: []string{
			// Once through a helper's parameter, once through a multi-value
			// call handing the verb every argument.
			"test/e2e/gitlab/common/planted_test.go the result of Do(a non-constant id) is discarded: read it, or call DoVoid",
			"test/e2e/gitlab/common/planted_test.go the result of Do(a non-constant id) is discarded: read it, or call DoVoid",
			"test/e2e/gitlab/common/planted_test.go the result of Do(issue.get) is discarded: read it, or call DoVoid",
			"test/e2e/gitlab/common/planted_test.go the result of Do(issue.get) is discarded: read it, or call DoVoid",
			"test/e2e/gitlab/common/planted_test.go the result of Eventually(issue.get) is discarded: read it, or call DoVoid",
			"test/e2e/gitlab/common/planted_test.go the result of Try(issue.get) is discarded: read it, or call DoVoid",
		}},
		{kind: findingUltimateUndeclared, want: []string{
			"test/e2e/gitlab/ee/planted_test.go vulnerability.get is Ultimate and TestPlanted_HelperReachedWithoutNeeds_Reported does not declare Needs(Tier(edition.Ultimate))",
			"test/e2e/gitlab/ee/planted_test.go vulnerability.get is Ultimate and TestPlanted_PointerMethodWithoutNeeds_Reported does not declare Needs(Tier(edition.Ultimate))",
			"test/e2e/gitlab/ee/planted_test.go vulnerability.get is Ultimate and TestPlanted_TwoLevelHelperWithoutNeeds_Reported does not declare Needs(Tier(edition.Ultimate))",
			"test/e2e/gitlab/ee/planted_test.go vulnerability.list is Ultimate and TestPlanted_TierOutsideNeeds_Reported does not declare Needs(Tier(edition.Ultimate))",
			"test/e2e/gitlab/ee/planted_test.go vulnerability.list is Ultimate and TestPlanted_UltimateWithoutNeeds_Reported does not declare Needs(Tier(edition.Ultimate))",
		}},
		{kind: findingDeadExport, want: nil},
		{kind: findingUnexercised, want: nil},
		{kind: findingStaleExemption, want: nil},
		{kind: findingPlacement, want: []string{
			"package example.com/e2efake/test/e2e/gitlab/legacy is under test/e2e/gitlab and is not common, ce or ee",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			if got := findingLines(result, tc.kind); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s findings = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
	if !result.failed() {
		t.Error("failed() = false with planted defects")
	}
}

// TestRunStatic_Sites_ReadThroughHelpersAndTables verifies the collection
// the whole gate rests on: every constant id is found, including the one
// passed to a helper's ActionID parameter and the ones in a table, and each
// is attributed to the function it sits in.
func TestRunStatic_Sites_ReadThroughHelpersAndTables(t *testing.T) {
	result := runFakeStatic(t, fakeStaticConfig(t))

	byFunc := map[string]map[string]bool{}
	for _, site := range result.Sites {
		if byFunc[site.Func] == nil {
			byFunc[site.Func] = map[string]bool{}
		}
		byFunc[site.Func][site.ID] = true
	}
	cases := []struct {
		fn   string
		want []string
	}{
		{fn: "", want: []string{"issue.list"}},
		{fn: "TestPlanted_HelperConstant_Resolved", want: []string{"project.get"}},
		{fn: "TestPlanted_TableConstants_Resolved", want: []string{"issue.list", "server.status"}},
		{fn: "readVulnerability", want: []string{"vulnerability.get"}},
		{fn: "reader.list", want: []string{"issue.list"}},
		{fn: "reader.get", want: []string{"vulnerability.get"}},
		{fn: "cycleB", want: []string{"vulnerability.get"}},
		{fn: "TestPlanted_OtherShapes_WalkedPast", want: []string{"issue.list"}},
		{fn: "TestPlanted_DiscardedThroughHelper_Reported", want: []string{"issue.get"}},
		{fn: "TestPlanted_TierAmongOtherNeeds_Clean", want: []string{"vulnerability.list"}},
	}
	for _, tc := range cases {
		t.Run(tc.fn, func(t *testing.T) {
			got := sortedKeys(byFunc[tc.fn])
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ids in %q = %q, want %q", tc.fn, got, tc.want)
			}
		})
	}
	placements := map[string]bool{}
	for _, site := range result.Sites {
		placements[site.Placement] = true
	}
	if want := []string{"ce", "common", "ee", "legacy"}; !reflect.DeepEqual(sortedKeys(placements), want) {
		t.Errorf("placements = %q, want %q", sortedKeys(placements), want)
	}
}

// TestRunStatic_SiteShapes_OnceAndOnlyFromTheHarnessType verifies two edges of
// the site collection. One constant named twice on one line is one site: a
// site is an id at a position, and the type checker records both uses. A
// constant of a lookalike type is no site at all: the gate reads the type
// checker's identity of harness.ActionID and never a name, so the fixture's
// own actionID type, spelling the same name over the same underlying type,
// contributes nothing, and the unknown action it names is no finding either.
func TestRunStatic_SiteShapes_OnceAndOnlyFromTheHarnessType(t *testing.T) {
	result := runFakeStatic(t, fakeStaticConfig(t))

	twice := 0
	for _, site := range result.Sites {
		if site.Func == "TestPlanted_SameIDTwiceOnOneLine_CollectedOnce" {
			twice++
		}
		if site.ID == "ghost.lookalike" {
			t.Errorf("site %+v names the lookalike type's constant", site)
		}
	}
	if twice != 1 {
		t.Errorf("sites in TestPlanted_SameIDTwiceOnOneLine_CollectedOnce = %d, want the one line's one site", twice)
	}
	for _, finding := range result.Findings {
		if strings.Contains(finding.Message, "ghost.lookalike") {
			t.Errorf("finding %s is about the lookalike type's constant", finding)
		}
	}
}

// TestRunStatic_Sites_PositionNamesTheLine verifies that a site's position is
// the line the id sits on, spelled relative to the module root. The line is
// read off the fixture's own text rather than pinned, so the assertion moves
// with the fixture; what it holds is that the number is the line and not, say,
// the column, which nothing else here can tell apart on a site of its own.
func TestRunStatic_Sites_PositionNamesTheLine(t *testing.T) {
	result := runFakeStatic(t, fakeStaticConfig(t))

	file := "test/e2e/gitlab/common/planted_test.go"
	want := idSite{
		ID: "issue.list", Placement: placementCommon,
		Pos: fmt.Sprintf("%s:%d", file, fixtureLine(t, file, `const listIssues harness.ActionID = "issue.list"`)),
	}
	if !slices.Contains(result.Sites, want) {
		t.Errorf("sites = %+v, want %+v among them", result.Sites, want)
	}
}

// fixtureLine is the line number of the first line of a fixture file that
// holds text.
func fixtureLine(t *testing.T, rel, text string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fakeModuleDir(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, text) {
			return i + 1
		}
	}
	t.Fatalf("%s holds no line with %q", rel, text)
	return 0
}

// TestRunStatic_Sites_OrderedByPositionThenID verifies the order the site
// list is published in, which is the order everything downstream reads it in.
//
// A site is spelled file:line with no column, so two ids named on one line
// share a position and the id is what separates them. The fixture's
// TestPlanted_TwoIDsOnOneLine_BothCollected is that line, and without a pair
// at one position the second half of the comparison is never reached at all.
func TestRunStatic_Sites_OrderedByPositionThenID(t *testing.T) {
	result := runFakeStatic(t, fakeStaticConfig(t))

	var samePos []string
	// sequential: each step compares one site with the one before it, so the
	// list is walked in order rather than judged case by case.
	for i, site := range result.Sites {
		if i > 0 {
			previous := result.Sites[i-1]
			if previous.Pos > site.Pos || (previous.Pos == site.Pos && previous.ID >= site.ID) {
				t.Errorf("site %d (%s %s) comes after (%s %s), which is not the published order",
					i, site.Pos, site.ID, previous.Pos, previous.ID)
			}
			if previous.Pos == site.Pos {
				samePos = append(samePos, previous.ID+" "+site.ID)
			}
		}
	}
	if want := []string{"alpha.action zeta.action"}; !reflect.DeepEqual(samePos, want) {
		t.Errorf("ids sharing a position = %q, want %q: the one planted line names two", samePos, want)
	}
}

// TestCollect_SitesOutOfOrder_SortedByPositionThenID verifies the same order
// against input that needs reordering, which the loaded fixture cannot
// provide: each package's sites are sorted before they are merged, so the
// merge only ever sees groups that are already in order.
func TestCollect_SitesOutOfOrder_SortedByPositionThenID(t *testing.T) {
	result := &staticResult{unassertedIDs: map[string]bool{}, testIDs: map[string]map[string]bool{}}
	result.collect([]*packageScan{{
		pkgPath: "example.com/e2efake/test/e2e/gitlab/common", placement: placementCommon,
		sites: []idSite{
			{ID: "zeta.action", Pos: "common/a_test.go:11", Placement: placementCommon},
			{ID: "beta.action", Pos: "common/a_test.go:12", Placement: placementCommon},
			{ID: "alpha.action", Pos: "common/a_test.go:11", Placement: placementCommon},
		},
		resultSites: map[string]int{}, discardedSites: map[string]int{}, funcs: map[string]*funcScan{},
	}})

	want := []idSite{
		{ID: "alpha.action", Pos: "common/a_test.go:11", Placement: placementCommon},
		{ID: "zeta.action", Pos: "common/a_test.go:11", Placement: placementCommon},
		{ID: "beta.action", Pos: "common/a_test.go:12", Placement: placementCommon},
	}
	if !reflect.DeepEqual(result.Sites, want) {
		t.Errorf("collect() sites = %+v, want %+v", result.Sites, want)
	}
}

// TestCollect_NonConstantOutOfOrder_SortedByPositionThenText verifies the
// order the non-constant notes are printed in: by position, and two notes on
// one line, which two verb calls on one line make, by their text, so a run
// prints them the same way whatever order the scans appended them in.
func TestCollect_NonConstantOutOfOrder_SortedByPositionThenText(t *testing.T) {
	result := &staticResult{unassertedIDs: map[string]bool{}, testIDs: map[string]map[string]bool{}}
	result.collect([]*packageScan{
		{
			nonConstant: []staticNote{
				{Pos: "common/a_test.go:12", Text: "DoVoid is called with the non-constant id id"},
				{Pos: "common/a_test.go:11", Text: "Try is called with the non-constant id b"},
			},
			resultSites: map[string]int{}, discardedSites: map[string]int{}, funcs: map[string]*funcScan{},
		},
		{
			nonConstant: []staticNote{{Pos: "common/a_test.go:11", Text: "Do is called with the non-constant id a"}},
			resultSites: map[string]int{}, discardedSites: map[string]int{}, funcs: map[string]*funcScan{},
		},
	})

	want := []staticNote{
		{Pos: "common/a_test.go:11", Text: "Do is called with the non-constant id a"},
		{Pos: "common/a_test.go:11", Text: "Try is called with the non-constant id b"},
		{Pos: "common/a_test.go:12", Text: "DoVoid is called with the non-constant id id"},
	}
	if !reflect.DeepEqual(result.NonConstant, want) {
		t.Errorf("collect() non-constant notes = %+v, want %+v", result.NonConstant, want)
	}
}

// TestJudge_Findings_SortedByPositionThenMessage verifies the order the
// findings are printed and published in: by position, and two findings at
// one position by their message.
func TestJudge_Findings_SortedByPositionThenMessage(t *testing.T) {
	result := &staticResult{}
	result.judge(staticConfig{}, []*packageScan{{
		pkgPath: "example.com/e2efake/test/e2e/gitlab/common", placement: placementCommon,
		discarded: []staticNote{
			{Pos: "common/a_test.go:12", Text: "the result of Do(b) is discarded"},
			{Pos: "common/a_test.go:11", Text: "the result of Try(z) is discarded"},
			{Pos: "common/a_test.go:11", Text: "the result of Do(a) is discarded"},
		},
	}})

	var got []string
	for _, finding := range result.Findings {
		got = append(got, finding.Pos+" "+finding.Message)
	}
	want := []string{
		"common/a_test.go:11 the result of Do(a) is discarded",
		"common/a_test.go:11 the result of Try(z) is discarded",
		"common/a_test.go:12 the result of Do(b) is discarded",
	}
	if !slices.Equal(got, want) {
		t.Errorf("judge() findings = %q, want %q", got, want)
	}
}

// TestRunStatic_NonConstantSites_Listed verifies that a verb called with a
// helper parameter or a table field is listed and not failed: both are
// constants one step away, and listing is what lets a reader check that. A
// verb called through a variable is not a verb call at all, so the constant
// it takes is a site and no note. A verb handed every argument by one
// multi-value call has fewer arguments than parameters, and is listed by that
// call rather than passed over.
func TestRunStatic_NonConstantSites_Listed(t *testing.T) {
	result := runFakeStatic(t, fakeStaticConfig(t))

	var got []string
	for _, note := range result.NonConstant {
		got = append(got, note.Text)
	}
	sort.Strings(got)
	want := []string{
		"Do is called with the non-constant id id",
		"Do is called with the non-constant id verbArguments(s)",
		"DoVoid is called with the non-constant id each",
		"DoVoid is called with the non-constant id id",
		"DoVoid is called with the non-constant id tc.id",
		"DoVoid is called with the non-constant id verbArguments(s)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("non-constant sites = %q, want %q", got, want)
	}
}

// TestRunStatic_DeadExports_ListedNotFailedUntilRatchet verifies both halves
// of the dead-export rule: the unused symbols are named, a type used only
// through what hands it out is not among them, a field set by keyed literal
// is used while the alias sharing its name is not, and without the ratchet
// they are a note rather than a finding. The fake harness carries an
// in-package Test function, which must not be listed: the scan reads the
// plain package and not its test variant, or every test of the real harness
// would be a dead export the ratchet then fails on.
//
// Two members have no type to be keyed under: the field of the anonymous
// struct Anon and the method of the interface literal Stopper. A scenario
// reads one and calls the other, and each is a use of the variable alone: the
// package-level Field and Stop that share their names stay dead, since a bare
// name credited on their account would be a function nothing calls read as
// called. The constant a command under test/e2e/internal reads is live, since
// a main package is passed over only when it is the generated test main.
func TestRunStatic_DeadExports_ListedNotFailedUntilRatchet(t *testing.T) {
	result := runFakeStatic(t, fakeStaticConfig(t))

	want := []string{"Failure.String", "Field", "Label", "Session.Close", "Stop", "Timeout", "Unused"}
	if !reflect.DeepEqual(result.DeadExports, want) {
		t.Errorf("DeadExports = %q, want %q", result.DeadExports, want)
	}
	for _, used := range []string{"Anon", "Stopper", "Version"} {
		t.Run(used, func(t *testing.T) {
			if slices.Contains(result.DeadExports, used) {
				t.Errorf("DeadExports = %q, want %s read as used", result.DeadExports, used)
			}
		})
	}
	// The unexported function and method beside them are read by nothing
	// either, and belong to staticcheck's unused check rather than to this
	// rule: a harness may keep whatever private helpers it likes.
	for _, unexported := range []string{"reset", "Session.close", "close"} {
		t.Run(unexported, func(t *testing.T) {
			if slices.Contains(result.DeadExports, unexported) {
				t.Errorf("DeadExports = %q, want the unexported %s left out", result.DeadExports, unexported)
			}
		})
	}
	if lines := findingLines(result, findingDeadExport); len(lines) != 0 {
		t.Errorf("dead exports failed the gate with the ratchet off: %q", lines)
	}
}

// TestRunStatic_UsedOnlyFromModelEval_IsNotDead verifies that a package
// outside test/e2e/gitlab counts as a consumer.
//
// The model evaluation package is loaded for this rule and no other: it names
// no catalog action, so it earns no ratchet credit and is never scanned for
// placement, and what loading it buys is that a harness export it is the first
// and only user of can be seen to have a user at all. Without it every seam
// added for the model harness would be a finding on the day it landed, and the
// gate runs on every push.
func TestRunStatic_UsedOnlyFromModelEval_IsNotDead(t *testing.T) {
	result := runFakeStatic(t, fakeStaticConfig(t))

	if slices.Contains(result.DeadExports, "ModelOnly") {
		t.Errorf("DeadExports = %q, want ModelOnly absent: test/e2e/modeleval calls it", result.DeadExports)
	}
	loaded := slices.ContainsFunc(result.Packages, func(pkg string) bool {
		return strings.HasSuffix(pkg, "/test/e2e/modeleval")
	})
	if !loaded {
		t.Errorf("packages = %q, want the model evaluation package among them", result.Packages)
	}
	// Loaded, and judged by nothing else: a placement finding here would mean
	// the gate had started reading it as a runtime package.
	for _, finding := range result.Findings {
		if strings.Contains(finding.Pos, "/modeleval/") {
			t.Errorf("the gate reported %s at %s: the package is loaded as a consumer and scanned for nothing",
				finding.Kind, finding.Pos)
		}
	}
}

// TestRunStatic_Ratchet_UnexercisedAndStaleExemptions verifies the ratchet:
// a catalog action no package that can run it names is a finding unless
// exempted, a Premium action named only in common is not exercised, an
// exemption for an action that is named is stale, and dead exports fail.
func TestRunStatic_Ratchet_UnexercisedAndStaleExemptions(t *testing.T) {
	cfg := fakeStaticConfig(t)
	cfg.ratchet = true
	cfg.exemptions = map[string]actionExemption{
		"project.list": {Category: categoryNotYetWritten, Reason: "owed"},
		"issue.list":   {Category: categoryNoFixture, Reason: "stale: common names it"},
		"ghost.action": {Category: categoryGitLabComOnly, Reason: "stale: not a catalog action"},
		"issue.get":    {Category: "made-up", Reason: "stale: unknown category"},
	}
	result := runFakeStatic(t, cfg)

	cases := []struct {
		kind string
		want []string
	}{
		{kind: findingUnexercised, want: nil},
		{kind: findingStaleExemption, want: []string{
			"ghost.action is exempted and is not a catalog action",
			"issue.get is exempted under the unknown category \"made-up\"",
			"issue.list is exempted and a package that can run it names it",
		}},
		{kind: findingDeadExport, want: []string{
			"Failure.String is exported by the harness and used by nothing",
			"Field is exported by the harness and used by nothing",
			"Label is exported by the harness and used by nothing",
			"Session.Close is exported by the harness and used by nothing",
			"Stop is exported by the harness and used by nothing",
			"Timeout is exported by the harness and used by nothing",
			"Unused is exported by the harness and used by nothing",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			if got := findingLines(result, tc.kind); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s findings = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}

// TestRunStatic_Ratchet_UnexercisedAction_Reported verifies the finding the
// ratchet exists for: an action the catalog has that no scenario names, with
// no exemption, fails the gate; and the Premium action common names does not
// count as exercised, since common cannot run it.
func TestRunStatic_Ratchet_UnexercisedAction_Reported(t *testing.T) {
	cfg := fakeStaticConfig(t)
	cfg.ratchet = true
	cfg.catalog = map[string]edition.Tier{
		"project.list":     edition.Free,
		"merge_train.list": edition.Premium,
		"merge_train.get":  edition.Premium,
		"issue.list":       edition.Free,
	}
	result := runFakeStatic(t, cfg)

	want := []string{
		"merge_train.get has no scenario in a package that can run it and no entry in exemptions.go",
		"project.list has no scenario in a package that can run it and no entry in exemptions.go",
	}
	if got := findingLines(result, findingUnexercised); !reflect.DeepEqual(got, want) {
		t.Errorf("unexercised findings = %q, want %q", got, want)
	}
}

// TestRunStatic_UnassertedAndTestIDs_Derived verifies the two things the
// classification reads off the static result: the ids whose every
// result-bearing site discards the answer, and the ids each test names,
// through one helper, through two, through a method, through a cycle, and
// past a method of an interface literal, which the walk keys by a name no
// declaration answers to and passes over.
//
// A discard whose id arrives through a helper's parameter counts no site
// against the id, since the helper cannot say which constant it was, so
// issue.get stays the one unasserted id however many times discardVia runs.
// A test name two packages declare folds into one entry carrying both
// packages' ids, since the map is keyed by the name and the skip line read
// through it names no package.
func TestRunStatic_UnassertedAndTestIDs_Derived(t *testing.T) {
	result := runFakeStatic(t, fakeStaticConfig(t))

	if want := []string{"issue.get"}; !reflect.DeepEqual(sortedKeys(result.unassertedIDs), want) {
		t.Errorf("unassertedIDs = %q, want %q", sortedKeys(result.unassertedIDs), want)
	}
	cases := []struct {
		test string
		want []string
	}{
		{test: "TestPlanted_HelperConstant_Resolved", want: []string{"project.get"}},
		{test: "TestPlanted_UltimateViaHelper_Clean", want: []string{"vulnerability.get"}},
		{test: "TestPlanted_UltimateViaTwoHelpers_Clean", want: []string{"vulnerability.get"}},
		{test: "TestPlanted_HelperCycle_Terminates", want: []string{"vulnerability.get"}},
		{test: "TestPlanted_MethodSite_Listed", want: []string{"issue.list"}},
		{test: "TestPlanted_PointerMethodWithoutNeeds_Reported", want: []string{"vulnerability.get"}},
		{test: "TestPlanted_KeptResult_Clean", want: []string{"issue.list"}},
		{test: "TestPlanted_InterfaceMethodOnTheWay_Clean", want: []string{"vulnerability.get"}},
		{test: "TestPlanted_DiscardedThroughHelper_Reported", want: []string{"issue.get"}},
		{test: "TestPlanted_SharedName_FoldsBothPackages", want: []string{"issue.get", "server.status"}},
	}
	for _, tc := range cases {
		t.Run(tc.test, func(t *testing.T) {
			if got := sortedKeys(result.testIDs[tc.test]); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("testIDs[%s] = %q, want %q", tc.test, got, tc.want)
			}
		})
	}
}

// TestRunStatic_NoSuite_Skips verifies the answer the gate gives on a tree
// where test/e2e/gitlab does not exist yet: nothing to check, no error, so
// the push gate passes before the first new-suite package lands.
func TestRunStatic_NoSuite_Skips(t *testing.T) {
	cfg := fakeStaticConfig(t)
	cfg.dir = t.TempDir()
	result, err := runStatic(cfg)
	if err != nil {
		t.Fatalf("runStatic() error = %v, want nil", err)
	}
	if !result.Skipped || result.failed() {
		t.Errorf("runStatic() = skipped %t, failed %t; want skipped and clean", result.Skipped, result.failed())
	}
}

// TestRunStatic_HarnessMissing_IsAnError verifies that a load that never
// reaches the harness is refused: without it there is no ActionID type to
// look for, and a clean report would be a report over nothing.
func TestRunStatic_HarnessMissing_IsAnError(t *testing.T) {
	cfg := fakeStaticConfig(t)
	cfg.harnessPath = "example.com/e2efake/test/e2e/internal/absent"
	if _, err := runStatic(cfg); err == nil || !strings.Contains(err.Error(), "harness package") {
		t.Errorf("runStatic() error = %v, want the missing-harness refusal", err)
	}
}

// TestRunStatic_UnloadableTree_IsAnError verifies that a tree the loader
// refuses is reported rather than read as empty.
func TestRunStatic_UnloadableTree_IsAnError(t *testing.T) {
	cfg := fakeStaticConfig(t)
	cfg.dir = t.TempDir()
	if err := os.MkdirAll(filepath.Join(cfg.dir, gitlabTestDir), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := runStatic(cfg); err == nil {
		t.Error("runStatic() error = nil on a directory with no module")
	}
}

// TestPlacementOf_Paths_Classified verifies how a package path maps onto the
// three runtime packages, external test packages included.
func TestPlacementOf_Paths_Classified(t *testing.T) {
	cases := []struct {
		name      string
		pkgPath   string
		want      string
		underTree bool
	}{
		{name: "common", pkgPath: "example.com/e2efake/test/e2e/gitlab/common", want: "common", underTree: true},
		{name: "ee external test package", pkgPath: "example.com/e2efake/test/e2e/gitlab/ee_test", want: "ee", underTree: true},
		{name: "nested under ce", pkgPath: "example.com/e2efake/test/e2e/gitlab/ce/sub", want: "ce", underTree: true},
		{name: "the harness", pkgPath: fakeHarnessPath, want: "", underTree: false},
		{name: "another module", pkgPath: "example.com/other/test/e2e/gitlab/common", want: "", underTree: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, under := placementOf(tc.pkgPath, fakeHarnessPath)
			if got != tc.want || under != tc.underTree {
				t.Errorf("placementOf(%q) = (%q, %t), want (%q, %t)", tc.pkgPath, got, under, tc.want, tc.underTree)
			}
		})
	}
}

// TestCanRun_TiersAndPlacements_Decided verifies which placements count as able to
// run an action of each tier.
func TestCanRun_TiersAndPlacements_Decided(t *testing.T) {
	cases := []struct {
		name       string
		tier       edition.Tier
		placements map[string]bool
		want       bool
	}{
		{name: "free in common", tier: edition.Free, placements: map[string]bool{placementCommon: true}, want: true},
		{name: "free in ce", tier: edition.Free, placements: map[string]bool{placementCE: true}, want: true},
		{name: "free in ee", tier: edition.Free, placements: map[string]bool{placementEE: true}, want: true},
		{name: "free nowhere", tier: edition.Free, placements: nil, want: false},
		{name: "premium in common only", tier: edition.Premium, placements: map[string]bool{placementCommon: true}, want: false},
		{name: "ultimate in ee", tier: edition.Ultimate, placements: map[string]bool{placementEE: true}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canRun(tc.tier, tc.placements); got != tc.want {
				t.Errorf("canRun(%s, %v) = %t, want %t", tc.tier, tc.placements, got, tc.want)
			}
		})
	}
}

// TestReceiverTypeName_Receivers_Normalized verifies that a receiver spells
// its type name the same way whatever wraps it: a pointer, parentheses, type
// parameters, or nothing, and that an expression that is none of these is
// spelled as written.
func TestReceiverTypeName_Receivers_Normalized(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{src: "reader", want: "reader"},
		{src: "*reader", want: "reader"},
		{src: "(*reader)", want: "reader"},
		{src: "*box[T]", want: "box"},
		{src: "pair[K, V]", want: "pair"},
		{src: "pkg.T", want: "pkg.T"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.src)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			if got := receiverTypeName(expr); got != tc.want {
				t.Errorf("receiverTypeName(%s) = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestDeclarations_BodylessFunction_PassedOver verifies that a function
// declared without a body, which Go allows for one implemented elsewhere, is
// neither walked nor indexed: there is no call in it to read, and indexing it
// would give the reference walk a function whose body it cannot reach.
func TestDeclarations_BodylessFunction_PassedOver(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "external.go", "package p\n\nfunc external()\n\nfunc TestLocal() {}\n", 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	scan := &packageScan{funcs: map[string]*funcScan{}}

	decls := (&packageScanner{}).declarations(&packages.Package{Syntax: []*ast.File{file}}, scan)

	if len(decls) != 1 || decls[0].Name.Name != "TestLocal" {
		t.Errorf("declarations() = %d declarations, want only TestLocal", len(decls))
	}
	if got := sortedKeys(scan.funcs); !slices.Equal(got, []string{"TestLocal"}) || !scan.funcs["TestLocal"].isTest {
		t.Errorf("funcs = %q, want TestLocal alone, marked a test", got)
	}
}

// TestSelectPackages_Variants_TheOneWithMoreFilesKept verifies which variant
// of a package the scan reads, in either order the loader hands them over:
// the one compiling more files, which is the test variant, since that is the
// one holding the test files; a later variant with fewer files does not
// replace it, and the generated test main is never kept at all.
func TestSelectPackages_Variants_TheOneWithMoreFilesKept(t *testing.T) {
	plain := &packages.Package{PkgPath: "m/p", ID: "m/p", CompiledGoFiles: []string{"p.go"}}
	withTests := &packages.Package{PkgPath: "m/p", ID: "m/p [m/p.test]", CompiledGoFiles: []string{"p.go", "p_test.go"}}
	testMain := &packages.Package{PkgPath: "m/p.test", Name: "main", CompiledGoFiles: []string{"main.go", "a.go", "b.go"}}
	// A package whose only tests are external has a test variant compiling
	// the same files as the plain one, so neither has more: the first the
	// loader hands over is kept, rather than whichever came last.
	external := &packages.Package{PkgPath: "m/e", ID: "m/e", CompiledGoFiles: []string{"e.go"}}
	externalVariant := &packages.Package{PkgPath: "m/e", ID: "m/e [m/e.test]", CompiledGoFiles: []string{"e.go"}}
	cases := []struct {
		name   string
		loaded []*packages.Package
		want   []*packages.Package
	}{
		{name: "the plain variant first", loaded: []*packages.Package{plain, withTests, testMain}, want: []*packages.Package{withTests}},
		{name: "the test variant first", loaded: []*packages.Package{withTests, testMain, plain}, want: []*packages.Package{withTests}},
		{
			name:   "two variants of one size, and two packages out of order",
			loaded: []*packages.Package{withTests, external, externalVariant},
			want:   []*packages.Package{external, withTests},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if selected := selectPackages(tc.loaded); !slices.Equal(selected, tc.want) {
				t.Errorf("selectPackages() = %+v, want %+v", selected, tc.want)
			}
		})
	}
}

// TestFindHarness_Variants_ThePlainOneInEitherOrder verifies that the harness
// is read as its consumers see it, the plain variant, whichever order the
// loader hands the two over in, and that a harness loaded only as its test
// variant is still found.
func TestFindHarness_Variants_ThePlainOneInEitherOrder(t *testing.T) {
	plain := &packages.Package{PkgPath: fakeHarnessPath, ID: fakeHarnessPath}
	withTests := &packages.Package{PkgPath: fakeHarnessPath, ID: fakeHarnessPath + " [" + fakeHarnessPath + ".test]"}
	other := &packages.Package{PkgPath: "example.com/other", ID: "example.com/other"}
	cases := []struct {
		name   string
		loaded []*packages.Package
		want   *packages.Package
	}{
		{name: "the plain variant first", loaded: []*packages.Package{other, plain, withTests}, want: plain},
		{name: "the test variant first", loaded: []*packages.Package{withTests, other, plain}, want: plain},
		{name: "the test variant alone", loaded: []*packages.Package{other, withTests}, want: withTests},
		{name: "no harness", loaded: []*packages.Package{other}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := findHarness(tc.loaded, fakeHarnessPath); got != tc.want {
				t.Errorf("findHarness() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestIsActionIDType_Types_OnlyTheHarnessOne verifies the identity the site
// collection rests on: the harness's ActionID is one, a type of the same
// name in another package is not, an unnamed type is not, and a named type
// declared in no package, which is what the universe's error is, is not
// either rather than being looked up in a package that is not there.
func TestIsActionIDType_Types_OnlyTheHarnessOne(t *testing.T) {
	named := func(path string) types.Type {
		pkg := types.NewPackage(path, "harness")
		return types.NewNamed(types.NewTypeName(token.NoPos, pkg, actionIDTypeName, nil), types.Typ[types.String], nil)
	}
	cases := []struct {
		name string
		typ  types.Type
		want bool
	}{
		{name: "the harness ActionID", typ: named(fakeHarnessPath), want: true},
		{name: "an ActionID of another package", typ: named("example.com/other"), want: false},
		{name: "an unnamed type", typ: types.Typ[types.String], want: false},
		{name: "a named type of no package", typ: types.Universe.Lookup("error").Type(), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isActionIDType(tc.typ, fakeHarnessPath); got != tc.want {
				t.Errorf("isActionIDType(%s) = %t, want %t", tc.typ, got, tc.want)
			}
		})
	}
}

// TestPackageScan_Reachable_ClosureWithCycles verifies the walk the three
// attributions share: every function at any depth is reached, a cycle is
// walked once and brings the starting function into its own set, a function
// nothing declares is passed over, and the answer is memoized.
func TestPackageScan_Reachable_ClosureWithCycles(t *testing.T) {
	scan := &packageScan{funcs: map[string]*funcScan{
		"TestA":   {name: "TestA", isTest: true, refs: map[string]bool{"a": true}},
		"a":       {name: "a", refs: map[string]bool{"b": true, "ghost": true}},
		"b":       {name: "b", refs: map[string]bool{"a": true, "c": true}},
		"c":       {name: "c", refs: map[string]bool{}},
		"TestNil": {name: "TestNil", isTest: true, refs: map[string]bool{}},
	}}
	if got, want := sortedKeys(scan.reachable("TestA")), []string{"a", "b", "c", "ghost"}; !reflect.DeepEqual(got, want) {
		t.Errorf("reachable(TestA) = %q, want %q", got, want)
	}
	if got, want := sortedKeys(scan.reachable("a")), []string{"a", "b", "c", "ghost"}; !reflect.DeepEqual(got, want) {
		t.Errorf("reachable(a) = %q, want the cycle to bring a into its own set: %q", got, want)
	}
	if got := scan.reachable("TestNil"); len(got) != 0 {
		t.Errorf("reachable(TestNil) = %q, want nothing", sortedKeys(got))
	}
	if got := scan.reachable("absent"); len(got) != 0 {
		t.Errorf("reachable(absent) = %q, want nothing for a function the package does not declare", sortedKeys(got))
	}
	if scan.declaresUltimate("absent") {
		t.Error("declaresUltimate(absent) = true for a test the package does not declare")
	}
	if got := scan.testsReaching(idSite{Func: "c"}); !reflect.DeepEqual(got, []string{"TestA"}) {
		t.Errorf("testsReaching(c) = %q, want TestA through a and b", got)
	}
	if got := scan.testsReaching(idSite{Func: "absent"}); got != nil {
		t.Errorf("testsReaching(absent) = %q, want nil", got)
	}
}

// TestStaticFinding_String_NamesPositionWhenKnown verifies the two spellings
// of a finding: with its site, and about the tree.
func TestStaticFinding_String_NamesPositionWhenKnown(t *testing.T) {
	cases := []struct {
		name    string
		finding staticFinding
		want    string
	}{
		{name: "with a site", finding: staticFinding{Kind: "k", Pos: "a.go:3", Message: "m"}, want: "a.go:3: k: m"},
		{name: "about the tree", finding: staticFinding{Kind: "k", Message: "m"}, want: "k: m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.finding.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCollect_ZeroResultCounter_NotUnasserted verifies the guard on the
// unasserted rule against a scan holding a zero counter for an id: unasserted
// means every result-bearing site discards the answer, and an id with no such
// site has nothing to discard. The scanner only ever counts upwards, so a
// hand-built scan is the one input that reaches the guard.
func TestCollect_ZeroResultCounter_NotUnasserted(t *testing.T) {
	result := &staticResult{unassertedIDs: map[string]bool{}, testIDs: map[string]map[string]bool{}}
	result.collect([]*packageScan{{
		pkgPath: "example.com/e2efake/test/e2e/gitlab/common", placement: placementCommon,
		resultSites: map[string]int{"issue.get": 0, "issue.list": 2}, discardedSites: map[string]int{"issue.list": 2},
		funcs: map[string]*funcScan{},
	}})

	if want := []string{"issue.list"}; !reflect.DeepEqual(sortedKeys(result.unassertedIDs), want) {
		t.Errorf("unassertedIDs = %q, want %q: an id with no result-bearing site cannot be unasserted", sortedKeys(result.unassertedIDs), want)
	}
}

// TestMarkNamedTypes_Depth_Bounded verifies the bound on the walk into a type's
// structure, which is what keeps the closure over the harness's types finite
// whatever a signature nests: a named type at the bound is marked, and one a
// level past it is not.
func TestMarkNamedTypes_Depth_Bounded(t *testing.T) {
	pkg := types.NewPackage("example.com/e2efake/test/e2e/internal/harness", "harness")
	env := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Env", nil), types.NewStruct(nil, nil), nil)
	param := func(typ types.Type) *types.Tuple {
		return types.NewTuple(types.NewParam(token.NoPos, pkg, "v", typ))
	}
	// Every constructor the walk descends through counts one level, whichever
	// half of it holds the type: a map's key and its value, a signature's
	// parameters and its results.
	wrappers := map[string]func(types.Type) types.Type{
		"pointers":   func(inner types.Type) types.Type { return types.NewPointer(inner) },
		"slices":     func(inner types.Type) types.Type { return types.NewSlice(inner) },
		"map keys":   func(inner types.Type) types.Type { return types.NewMap(inner, types.Typ[types.Int]) },
		"map values": func(inner types.Type) types.Type { return types.NewMap(types.Typ[types.Int], inner) },
		"parameters": func(inner types.Type) types.Type {
			return types.NewSignatureType(nil, nil, nil, param(inner), nil, false)
		},
		"results": func(inner types.Type) types.Type {
			return types.NewSignatureType(nil, nil, nil, nil, param(inner), false)
		},
	}
	cases := []struct {
		name  string
		depth int
		want  bool
	}{
		{name: "at the bound", depth: typeDepth, want: true},
		{name: "past the bound", depth: typeDepth + 1, want: false},
	}
	for _, through := range sortedKeys(wrappers) {
		for _, tc := range cases {
			t.Run(through+" "+tc.name, func(t *testing.T) {
				var wrapped types.Type = env
				for range tc.depth {
					wrapped = wrappers[through](wrapped)
				}
				marked := map[string]bool{}
				markNamedTypes(wrapped, pkg, func(key string) { marked[key] = true }, 0)
				if marked["Env"] != tc.want {
					t.Errorf("Env marked = %t under %d %s, want %t", marked["Env"], tc.depth, through, tc.want)
				}
			})
		}
	}
}

// TestPackageScanner_Position_OutsideTheModuleStaysAbsolute verifies the two
// ways a file is not spelled relative to the module root: a scanner with no
// root, which is what a harness with no files leaves it, and a file outside
// the root, whose relative spelling would climb through "..". Both keep the
// absolute name, so a position never names a file that is not where it says.
func TestPackageScanner_Position_OutsideTheModuleStaysAbsolute(t *testing.T) {
	if got := moduleDir(&packages.Package{}); got != "" {
		t.Errorf("moduleDir() of a harness with no files = %q, want no root", got)
	}
	fileDir, moduleRoot := t.TempDir(), t.TempDir()
	name := filepath.Join(fileDir, "planted_test.go")
	fset := token.NewFileSet()
	pos := fset.AddFile(name, -1, 8).Pos(0)
	pkg := &packages.Package{Fset: fset}
	cases := []struct {
		name string
		dir  string
		want string
	}{
		{name: "no module root", dir: "", want: name + ":1"},
		{name: "outside the module", dir: moduleRoot, want: name + ":1"},
		{name: "inside the module", dir: fileDir, want: "planted_test.go:1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (&packageScanner{dir: tc.dir}).position(pkg, pos); got != tc.want {
				t.Errorf("position() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestIsUltimate_Constants_Judged verifies the reading of a Tier argument: the
// Ultimate integer is the tier, a lower one is not, and a value that is not an
// integer constant, or no constant at all, declares nothing.
func TestIsUltimate_Constants_Judged(t *testing.T) {
	cases := []struct {
		name string
		tv   types.TypeAndValue
		want bool
	}{
		{name: "ultimate", tv: types.TypeAndValue{Value: constant.MakeInt64(int64(edition.Ultimate))}, want: true},
		{name: "premium", tv: types.TypeAndValue{Value: constant.MakeInt64(int64(edition.Premium))}},
		{name: "a string constant", tv: types.TypeAndValue{Value: constant.MakeString("ultimate")}},
		{name: "not a constant"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUltimate(tc.tv); got != tc.want {
				t.Errorf("isUltimate() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestUnwrapCallee_Shapes_Stripped verifies that parentheses and type
// arguments come off a callee, one type argument or several, and that a callee
// that is none of these is handed back as it is.
func TestUnwrapCallee_Shapes_Stripped(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{src: "Do(s)", want: "Do"},
		{src: "Do[A](s)", want: "Do"},
		{src: "Do[A, B](s)", want: "Do"},
		{src: "(Do)(s)", want: "Do"},
		{src: "((Do[A]))(s)", want: "Do"},
		{src: "f()(s)", want: "f()"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.src)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			call, isCall := expr.(*ast.CallExpr)
			if !isCall {
				t.Fatalf("%q parsed as %T, want a call", tc.src, expr)
			}
			if got := types.ExprString(unwrapCallee(call.Fun)); got != tc.want {
				t.Errorf("unwrapCallee(%s) = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
