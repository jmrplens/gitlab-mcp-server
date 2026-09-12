package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

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
// made through a helper, a Premium id in ee.
func TestRunStatic_PlantedDefects_EachReported(t *testing.T) {
	result := runFakeStatic(t, fakeStaticConfig(t))

	cases := []struct {
		kind string
		want []string
	}{
		{kind: findingUnknownID, want: []string{
			"test/e2e/gitlab/common/planted_test.go nope.action is not a catalog action",
		}},
		{kind: findingTierPlacement, want: []string{
			"test/e2e/gitlab/common/planted_test.go merge_train.list is premium and common runs on every runtime",
		}},
		{kind: findingDiscardedResult, want: []string{
			"test/e2e/gitlab/common/planted_test.go the result of Do(issue.get) is discarded: read it, or call DoVoid",
			"test/e2e/gitlab/common/planted_test.go the result of Do(issue.get) is discarded: read it, or call DoVoid",
			"test/e2e/gitlab/common/planted_test.go the result of Eventually(issue.get) is discarded: read it, or call DoVoid",
			"test/e2e/gitlab/common/planted_test.go the result of Try(issue.get) is discarded: read it, or call DoVoid",
		}},
		{kind: findingUltimateUndeclared, want: []string{
			"test/e2e/gitlab/ee/planted_test.go vulnerability.get is Ultimate and TestPlanted_HelperReachedWithoutNeeds_Reported does not declare Needs(Tier(edition.Ultimate))",
			"test/e2e/gitlab/ee/planted_test.go vulnerability.list is Ultimate and TestPlanted_UltimateWithoutNeeds_Reported does not declare Needs(Tier(edition.Ultimate))",
		}},
		{kind: findingDeadExport, want: nil},
		{kind: findingUnexercised, want: nil},
		{kind: findingStaleExemption, want: nil},
		{kind: findingPlacement, want: nil},
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
	if want := []string{"common", "ee"}; !reflect.DeepEqual(sortedKeys(placements), want) {
		t.Errorf("placements = %q, want %q", sortedKeys(placements), want)
	}
}

// TestRunStatic_NonConstantSites_Listed verifies that a verb called with a
// helper parameter or a table field is listed and not failed: both are
// constants one step away, and listing is what lets a reader check that.
func TestRunStatic_NonConstantSites_Listed(t *testing.T) {
	result := runFakeStatic(t, fakeStaticConfig(t))

	var got []string
	for _, note := range result.NonConstant {
		got = append(got, note.Text)
	}
	sort.Strings(got)
	want := []string{
		"DoVoid is called with the non-constant id id",
		"DoVoid is called with the non-constant id tc.id",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("non-constant sites = %q, want %q", got, want)
	}
}

// TestRunStatic_DeadExports_ListedNotFailedUntilRatchet verifies both halves
// of the dead-export rule: the unused symbols are named, a type used only
// through what hands it out is not among them, and without the ratchet they
// are a note rather than a finding.
func TestRunStatic_DeadExports_ListedNotFailedUntilRatchet(t *testing.T) {
	result := runFakeStatic(t, fakeStaticConfig(t))

	want := []string{"Env.Label", "Failure.String", "Label", "Session.Close", "Unused"}
	if !reflect.DeepEqual(result.DeadExports, want) {
		t.Errorf("DeadExports = %q, want %q", result.DeadExports, want)
	}
	if lines := findingLines(result, findingDeadExport); len(lines) != 0 {
		t.Errorf("dead exports failed the gate with the ratchet off: %q", lines)
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
			"Env.Label is exported by the harness and used by nothing",
			"Failure.String is exported by the harness and used by nothing",
			"Label is exported by the harness and used by nothing",
			"Session.Close is exported by the harness and used by nothing",
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
// helpers included.
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
		{test: "TestPlanted_KeptResult_Clean", want: []string{"issue.list"}},
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
