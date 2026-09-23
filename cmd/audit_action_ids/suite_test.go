package main

import (
	"errors"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/packages"
)

// suiteFixtureDir is where the planted e2e suite pretends to live. Like the
// served fixture it exists only in the loader overlay, and it sits under
// test/e2e/ because that is the one place a helper call is read at all: the
// walk matches a helper by the package its function is declared in, and a
// suite planted anywhere else would be a lookalike.
const suiteFixtureDir = "test/e2e/actionidsfixture"

// suiteFixturePattern loads the planted suite and every package under it.
const suiteFixturePattern = "./" + suiteFixtureDir + "/..."

// suiteHarnessImport is the import path of the planted harness.
const suiteHarnessImport = `"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/actionidsfixture/harness"`

// The files every planted suite starts from: a package clause the plain
// package keeps, a harness with the real ExpectToolError's signature, and the
// three package-local helpers with the real ones' parameter names.
//
// The harness is a stand-in rather than the real one on purpose. Importing
// test/e2e/internal/harness would pull the whole of internal/tools into every
// load these tests make, and the one thing the real harness adds, its
// signature, is held where it matters: by the gate over the real suite, whose
// mismatch rule names a helper that stops taking contains.
const (
	suiteFixtureDoc = `// Package actionidsfixture is a planted e2e suite.
package actionidsfixture
`
	suiteFixtureHarness = `//go:build e2e

// Package harness stands in for the suite's harness.
package harness

// Session is a connection nobody opens.
type Session struct{}

// ActionID names a catalog action.
type ActionID string

// Option adjusts one call.
type Option func()

// FeatureFlag is a constant another package quotes.
const FeatureFlag = "from a qualified constant"

// For is an option, which the walk must not read as a needle.
func For(purpose string) Option { return func() {} }

// ExpectToolError has the real helper's signature.
func ExpectToolError(s *Session, id ActionID, params map[string]any, contains string, opts ...Option) string {
	return contains
}

// Raw names a tool on purpose.
func Raw(s *Session, name string) {}
`
	suiteFixtureHelpers = `//go:build e2e

package actionidsfixture

import (
	"strings"
	"testing"
)

func assertMentions(t *testing.T, what, text string, substrings ...string) {
	for _, want := range substrings {
		if !strings.Contains(text, want) {
			t.Errorf("%s does not mention %q", what, want)
		}
	}
}

func mentionsAny(text string, substrings ...string) bool {
	for _, want := range substrings {
		if strings.Contains(text, want) {
			return true
		}
	}
	return false
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
`
)

// suiteHelpersIn is the helper file for another planted package, since the
// suite declares its helpers once per package.
func suiteHelpersIn(pkg string) string {
	return strings.Replace(suiteFixtureHelpers, "package actionidsfixture", "package "+pkg, 1)
}

// suiteOverlay plants the base suite plus files, each keyed by its path below
// the planted suite, a file of the base replaced by one of the same name.
func suiteOverlay(t *testing.T, files map[string]string) map[string][]byte {
	t.Helper()
	root := repoRoot(t)
	planted := map[string]string{
		"doc.go":             suiteFixtureDoc,
		"harness/harness.go": suiteFixtureHarness,
		"helpers_test.go":    suiteFixtureHelpers,
	}
	maps.Copy(planted, files)
	overlay := map[string][]byte{}
	for rel, source := range planted {
		overlay[filepath.Join(root, filepath.FromSlash(suiteFixtureDir), filepath.FromSlash(rel))] = []byte(source)
	}
	return overlay
}

// collectPlanted walks the planted suite the overlay describes.
func collectPlanted(t *testing.T, overlay map[string][]byte) suiteRead {
	t.Helper()
	read, err := collectAssertionSites(repoRoot(t), []string{suiteFixturePattern}, overlay)
	if err != nil {
		t.Fatalf("collect assertion sites: %v", err)
	}
	return read
}

// sitesIn keeps the sites one planted package produced.
func sitesIn(sites []site, pkg string) []site {
	var own []site
	for _, at := range sites {
		if at.Package == pkg {
			own = append(own, at)
		}
	}
	return own
}

// The planted positions: every shape a quotation is written in, beside every
// shape that is not one. Each value says which it is, so a read that picked up
// the wrong argument shows in the list of what was read.
const (
	plantedPositions = `//go:build e2e

package actionidsfixture

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_action_ids/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	` + suiteHarnessImport + `
)

const (
	toolName      = "gitlab_search_code"
	excludedGroup = "gitlab_issue"
)

var searchTypes = []string{"from a package var"}

func TestPlanted(t *testing.T) {
	s := &harness.Session{}
	text := harness.ExpectToolError(s, "not read: an id", map[string]any{"id": "not read: a parameter"}, "from ExpectToolError", harness.For("not read: an option"))
	assertMentions(t, toolName, text, "from a literal")
	hint := []string{"from a local slice", "from its second element"}
	assertMentions(t, "not read: what", text, hint...)
	assertMentions(t, "not read: what", text, searchTypes...)
	assertMentions(t, "not read: what", text, append([]string{"from an append"}, searchTypes...)...)
	assertMentions(t, "not read: what", text, harness.FeatureFlag)
	assertMentions(t, "not read: what", text, toolutil.HintAction("from a HintAction's action ID", "not read: its purpose"))
	if !mentionsAny(text, "from a negated mentionsAny") {
		t.Error("absent")
	}
	if !(containsAny(text, "from a negated containsAny in parentheses")) {
		t.Error("absent")
	}
	if containsAny(text, "not read: an absence check", excludedGroup) {
		t.Error("present")
	}
	switch {
	case containsAny(text, "not read: a classification"):
		t.Log("classified")
	}
	harness.Raw(s, "gitlab_interactive_issue_create")
	fixture.ExpectToolError("not read: a lookalike outside the suite")
	ok := strings.Contains(text, "not read: a direct strings.Contains")
	if !ok || !strings.Contains(text, "not read: a negated strings.Contains") {
		t.Log(&text, len(text))
	}
	var err error
	if err != nil {
		assertMentions(t, "not read: what", err.Error(), "from beside an err.Error()")
	}
}
`
	plantedBothVariants = `//go:build e2e

package actionidsfixture

import ` + suiteHarnessImport + `

// Planted quotes a served text from a file both variants carry.
func Planted(s *harness.Session) string {
	return harness.ExpectToolError(s, "not read: an id", nil, "from a file both variants carry")
}
`
	plantedUntested = `//go:build e2e

// Package untested has no tests, so there is no variant to prefer.
package untested

import ` + suiteHarnessImport + `

// Refusal quotes a served text from a package with no tests.
func Refusal(s *harness.Session) string {
	return harness.ExpectToolError(s, "not read: an id", nil, "from a package with no tests")
}
`
	plantedLookalike = `package fixture

// ExpectToolError shares its name with the harness helper and lives outside
// the suite, so a call of it quotes nothing the rule reads.
func ExpectToolError(contains string) string { return contains }
`
)

// plantedSuite is the one load of the planted positions the tests below
// share, made once because each of them asks a different question of the same
// walk and a load is the slow part.
var plantedSuite = struct {
	once sync.Once
	read suiteRead
	err  error
}{}

// readPlantedSuite walks the planted positions, once per test binary.
func readPlantedSuite(t *testing.T) suiteRead {
	t.Helper()
	root := repoRoot(t)
	overlay := suiteOverlay(t, map[string]string{
		"planted_test.go":      plantedPositions,
		"planted.go":           plantedBothVariants,
		"untested/untested.go": plantedUntested,
	})
	overlay[filepath.Join(root, filepath.FromSlash(fixtureDir), "lookalike.go")] = []byte(plantedLookalike)
	plantedSuite.once.Do(func() {
		plantedSuite.read, plantedSuite.err = collectAssertionSites(root, []string{suiteFixturePattern}, overlay)
	})
	if plantedSuite.err != nil {
		t.Fatalf("collect assertion sites: %v", plantedSuite.err)
	}
	return plantedSuite.read
}

// TestCollectAssertionSites_EveryQuotingPosition_IsRead holds what the walk
// reads out of a suite, as a whole list: the substring of every declared
// helper however it is spelled, and nothing from any other position.
//
// The shapes are the ones the suite writes. A needle is a literal, an element
// of a local slice spread into the call, a package variable spread whole or
// through an append, a constant of another package, the one contains of
// ExpectToolError, or the needles of a negated predicate, parenthesised or
// not. One more is a shape no suite file writes yet, although sixteen of them
// import toolutil already: a needle built with toolutil.HintAction, read as the
// action ID it names, since the served walk judges that ID where HintAction
// is called and the suite walk has no such visit. Beside them sit the
// positions that must stay silent: what a helper is told it is checking, the
// text it is checking, ExpectToolError's id, parameters and options,
// HintAction's purpose, a predicate used as an absence check or as a
// classification, a tool named on purpose to Raw, a lookalike helper outside
// the suite, and the direct string checks a test writes without a helper.
//
// All but one of those sites are in a test file of a directory that exists
// only in the overlay, and the last assertion names one of them by its file,
// which settles the one thing about this load no other test in the tree
// shows: go list hands back the test variant of such a package.
func TestCollectAssertionSites_EveryQuotingPosition_IsRead(t *testing.T) {
	sites := sitesIn(readPlantedSuite(t).sites, suiteFixtureDir)

	want := []string{
		"from ExpectToolError",
		"from a HintAction's action ID",
		"from a file both variants carry",
		"from a literal",
		"from a local slice",
		"from a negated containsAny in parentheses",
		"from a negated mentionsAny",
		"from a package var",
		"from a qualified constant",
		"from an append",
		"from beside an err.Error()",
		"from its second element",
	}
	if got := valuesOfKind(sites, kindAssertion); !slices.Equal(got, want) {
		t.Errorf("assertion values = %q\nwant %q", got, want)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none", got)
	}
	if !slices.ContainsFunc(sites, func(at site) bool {
		return at.File == suiteFixtureDir+"/planted_test.go" && at.Value == "from a literal"
	}) {
		t.Errorf("sites = %+v, want the literal stamped with the test file it was written in", sites)
	}
}

// TestCollectAssertionSites_AFileBothVariantsCarry_IsReadOnce holds the choice
// of variant. A file that is not a test file is parsed into the package and
// into its test variant as two trees, and a site is one per expression, so
// walking both would report every call in such a file twice.
func TestCollectAssertionSites_AFileBothVariantsCarry_IsReadOnce(t *testing.T) {
	var found []site
	for _, at := range readPlantedSuite(t).sites {
		if at.Value == "from a file both variants carry" {
			found = append(found, at)
		}
	}
	want := []site{{
		Package: suiteFixtureDir, File: suiteFixtureDir + "/planted.go", Line: 9,
		Kind: kindAssertion, Value: "from a file both variants carry", Resolved: true,
	}}
	if !slices.Equal(found, want) {
		t.Errorf("sites = %+v, want %+v", found, want)
	}
}

// TestCollectAssertionSites_APackageWithNoTests_IsRead holds the other half of
// that choice: a package no test variant was loaded for is walked as itself,
// or a quotation in it would never be read.
func TestCollectAssertionSites_APackageWithNoTests_IsRead(t *testing.T) {
	sites := sitesIn(readPlantedSuite(t).sites, suiteFixtureDir+"/untested")

	if got := valuesOfKind(sites, kindAssertion); !slices.Equal(got, []string{"from a package with no tests"}) {
		t.Errorf("assertion values = %q, want the one quotation of the untested package", got)
	}
}

// TestCollectAssertionSites_ANeedleListParameter_IsReadElementByElement holds
// the one parameter shape no helper of the suite has today: a copy of a
// declared helper that takes its needles as a []string of its own rather than
// as a variadic tail. The argument is then one list, and read as one needle it
// folded to nothing, which fails nothing, so every quotation handed to such a
// copy would have passed unread. A literal list, a local one and an append to
// a package list are the shapes the served walk already folds a list of hints
// in, and each is read here element by element.
func TestCollectAssertionSites_ANeedleListParameter_IsReadElementByElement(t *testing.T) {
	read := collectPlanted(t, suiteOverlay(t, map[string]string{
		"listed/doc.go": "// Package listed takes its needles as a list.\npackage listed\n",
		"listed/listed_test.go": `//go:build e2e

package listed

import (
	"strings"
	"testing"
)

func assertMentions(t *testing.T, what, text string, substrings []string) {
	for _, want := range substrings {
		if !strings.Contains(text, want) {
			t.Errorf("%s does not mention %q", what, want)
		}
	}
}

var shared = []string{"from a package list"}

func TestListed(t *testing.T) {
	text := "text"
	assertMentions(t, "not read: what", text, []string{"from a literal list", "from its second entry"})
	local := []string{"from a local list"}
	assertMentions(t, "not read: what", text, local)
	assertMentions(t, "not read: what", text, append(shared, "from an appended entry"))
}
`,
	}))
	sites := sitesIn(read.sites, suiteFixtureDir+"/listed")

	want := []string{
		"from a literal list",
		"from a local list",
		"from a package list",
		"from an appended entry",
		"from its second entry",
	}
	if got := valuesOfKind(sites, kindAssertion); !slices.Equal(got, want) {
		t.Errorf("assertion values = %q\nwant %q", got, want)
	}
	if got := unresolvedExprs(sites); len(got) != 0 {
		t.Errorf("unresolved = %v, want none", got)
	}
}

// TestCollectAssertionSites_APackageWhoseTestsAreAllExternal_IsReadOnce holds
// the shape go list builds no internal variant for. The package's only tests
// are in package external_test, which names it in ForTest without compiling
// its files, so the package itself is the one copy of them: dropped because a
// variant named it, a quotation in it would be read by nothing and reported
// nowhere.
func TestCollectAssertionSites_APackageWhoseTestsAreAllExternal_IsReadOnce(t *testing.T) {
	read := collectPlanted(t, suiteOverlay(t, map[string]string{
		"external/external.go": `//go:build e2e

// Package external is tested only from outside.
package external

import ` + suiteHarnessImport + `

// Refusal quotes a served text from a package whose tests are all external.
func Refusal(s *harness.Session) string {
	return harness.ExpectToolError(s, "not read: an id", nil, "from a package whose tests are all external")
}
`,
		"external/external_test.go": `//go:build e2e

package external_test

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/actionidsfixture/external"
)

func TestRefusal(t *testing.T) {
	t.Log(external.Refusal != nil)
}
`,
	}))

	sites := sitesIn(read.sites, suiteFixtureDir+"/external")
	if got := valuesOfKind(sites, kindAssertion); !slices.Equal(got, []string{"from a package whose tests are all external"}) {
		t.Errorf("assertion values = %q, want the one quotation of the package, read once from the package itself", got)
	}
}

// TestCollectAssertionSites_EveryHelperCall_IsCountedOnce holds the figure the
// helper table is judged by. A negated predicate is met twice by the walk, as
// the operand of the `!` and as a call, and counted once; a predicate that is
// not negated is counted and not judged; a function of a declared name outside
// the suite is not counted at all.
//
// A figure off by the double visit would never be zero where a helper is
// used, so the staleness rule would read the same; it is the count published
// in the report, and a reader comparing it with the suite would find it wrong.
func TestCollectAssertionSites_EveryHelperCall_IsCountedOnce(t *testing.T) {
	read := readPlantedSuite(t)

	want := map[string]int{"assertMentions": 7, "ExpectToolError": 3, "mentionsAny": 1, "containsAny": 3}
	if !maps.Equal(read.calls, want) {
		t.Errorf("calls = %v, want %v", read.calls, want)
	}
	if len(read.mismatches) != 0 {
		t.Errorf("mismatches = %v, want none: every planted helper takes its declared parameter", read.mismatches)
	}
}

// TestCollectAssertionSites_AHelperWithoutItsParameter_IsNamed holds the rule
// that keeps a renamed parameter from switching the reading off. The helper is
// called as a bare predicate, whose needles would not be judged anyway, and it
// is named all the same: the parameter is looked up on every call, so a
// helper used only in the unjudged position is not a way past the rule.
//
// It is named as the copy it is. The planted suite's own mentionsAny takes
// the declared parameter and is called too, which is the shape the real suite
// has in assertMentions, declared in common and again in ee: the one entry
// covers both, so only the copy's package says which of them has to change,
// and the copy that agrees is read as before.
func TestCollectAssertionSites_AHelperWithoutItsParameter_IsNamed(t *testing.T) {
	read := collectPlanted(t, suiteOverlay(t, map[string]string{
		"kept_test.go": `//go:build e2e

package actionidsfixture

import "testing"

func TestKept(t *testing.T) {
	if !mentionsAny("text", "read: the copy that takes its parameter") {
		t.Error("absent")
	}
}
`,
		"renamed/doc.go": "// Package renamed renames a helper's parameter.\npackage renamed\n",
		"renamed/renamed_test.go": `//go:build e2e

package renamed

import (
	"strings"
	"testing"
)

func mentionsAny(text string, wants ...string) bool {
	for _, want := range wants {
		if strings.Contains(text, want) {
			return true
		}
	}
	return false
}

func TestRenamed(t *testing.T) {
	if mentionsAny("text", "not read: a helper without its parameter") {
		t.Log("present")
	}
}
`,
	}))

	want := map[helperCopy]string{{pkg: suiteFixtureDir + "/renamed", name: "mentionsAny"}: "substrings"}
	if !maps.Equal(read.mismatches, want) {
		t.Errorf("mismatches = %v, want %v", read.mismatches, want)
	}
	if got := sitesIn(read.sites, suiteFixtureDir+"/renamed"); len(got) != 0 {
		t.Errorf("sites = %+v, want none from a helper whose parameter is not there", got)
	}
	wantKept := []string{"read: the copy that takes its parameter"}
	if got := valuesOfKind(sitesIn(read.sites, suiteFixtureDir), kindAssertion); !slices.Equal(got, wantKept) {
		t.Errorf("assertion values of the copy that agrees = %q, want %q", got, wantKept)
	}
	if read.calls["mentionsAny"] != 2 {
		t.Errorf("calls = %v, want both copies' calls counted, the one that could not be read included", read.calls)
	}
}

// TestCollectAssertionSites_TheParameterIsFoundByName_WhereverItSits holds
// that the argument is read at the declared parameter's own position, the
// first included, and in the shape the parameter has. A copy of a helper that
// takes its needles first, or one needle rather than a list, is still that
// helper: a rule that took the first position for "no such parameter" would
// name it stale and read none of its calls, and one that assumed a list would
// read a single needle as the start of one. A unary operator other than `!`
// over a call negates nothing, and is passed over as such.
func TestCollectAssertionSites_TheParameterIsFoundByName_WhereverItSits(t *testing.T) {
	read := collectPlanted(t, suiteOverlay(t, map[string]string{
		"reordered/doc.go": "// Package reordered takes the needles first.\npackage reordered\n",
		"reordered/reordered_test.go": `//go:build e2e

package reordered

import "testing"

func containsAny(needles ...string) bool { return len(needles) > 1 }

func mentionsAny(text, substrings string) bool { return text == substrings }

func TestReordered(t *testing.T) {
	if !containsAny("from a helper whose needles come first") {
		t.Error("absent")
	}
	if !mentionsAny("text", "from a helper that takes one needle") {
		t.Error("absent")
	}
	if -len("text") > 0 {
		t.Error("a negative length")
	}
}
`,
	}))

	want := []string{"from a helper that takes one needle", "from a helper whose needles come first"}
	if got := valuesOfKind(sitesIn(read.sites, suiteFixtureDir+"/reordered"), kindAssertion); !slices.Equal(got, want) {
		t.Errorf("assertion values = %q, want %q", got, want)
	}
	if len(read.mismatches) != 0 {
		t.Errorf("mismatches = %v, want none: the helper takes its declared parameter", read.mismatches)
	}
}

// TestCollectAssertionSites_WhatNothingFolds_IsReportedRatherThanPassedOver
// holds the needles the walk cannot read, each named rather than dropped.
//
// A call that hands over one multi-valued call for its whole argument list has
// no argument at the parameter's position, and a variadic helper called with
// no needle asserts nothing; both are named. So is a needle read off a test
// table's hint field, which is the one place this walk departs from the served
// one: there a read of a hint field is a copy of prose recorded where the field
// is written, and the suite walk records no field write, so passing the read
// over as a copy would pass it in silence.
func TestCollectAssertionSites_WhatNothingFolds_IsReportedRatherThanPassedOver(t *testing.T) {
	read := collectPlanted(t, suiteOverlay(t, map[string]string{
		"unfolded/doc.go":          "// Package unfolded quotes what nothing folds.\npackage unfolded\n",
		"unfolded/helpers_test.go": suiteHelpersIn("unfolded"),
		"unfolded/unfolded_test.go": `//go:build e2e

package unfolded

import (
	"testing"

	` + suiteHarnessImport + `
)

func fourValues() (*harness.Session, harness.ActionID, map[string]any, string) {
	return nil, "not read: an id", nil, "not read: behind a multi-valued call"
}

type refusalCase struct {
	name  string
	hint  string
	hints []string
}

func describe(value string) string { return value }

func TestUnfolded(t *testing.T) {
	harness.ExpectToolError(fourValues())
	text := "a refusal"
	if !mentionsAny(text) {
		t.Log("a predicate asked about nothing")
	}
	for _, c := range []refusalCase{{name: "one", hint: "not read: a hint field", hints: []string{"not read: a hint list field"}}} {
		assertMentions(t, c.name, text, c.hint)
		assertMentions(t, c.name, text, c.hints...)
		assertMentions(t, c.name, text, describe(c.hint))
	}
}
`,
	}))
	sites := sitesIn(read.sites, suiteFixtureDir+"/unfolded")

	want := []string{"c.hint", "c.hints", "describe(c.hint)", "harness.ExpectToolError(fourValues())", "mentionsAny(text)"}
	if got := unresolvedExprs(sites); !slices.Equal(got, want) {
		t.Errorf("unresolved = %v\nwant %v", got, want)
	}
	if got := valuesOfKind(sites, kindAssertion); len(got) != 0 {
		t.Errorf("assertion values = %q, want nothing folded", got)
	}
}

// TestCollectAssertionSites_AWrapperForwardingItsNeedles_IsFollowedToItsCallers
// holds the parameter rule for the suite. A wrapper that takes the needles
// under a declared helper's own parameter name, or under a hint's, and hands
// them on is followed out to its callers, where the needles are written; one
// that takes them under any other name is named as a needle nothing folds,
// because following every string parameter judges whatever any caller passes.
//
// The forwarding caller concatenates, which is what tells the prose rule from
// the ID rule here: the prose rule keeps the literal half of a sentence built
// at run time and names the half nothing folds, and the ID rule has nothing to
// keep.
func TestCollectAssertionSites_AWrapperForwardingItsNeedles_IsFollowedToItsCallers(t *testing.T) {
	read := collectPlanted(t, suiteOverlay(t, map[string]string{
		"wrappers/doc.go":          "// Package wrappers forwards needles through wrappers.\npackage wrappers\n",
		"wrappers/helpers_test.go": suiteHelpersIn("wrappers"),
		"wrappers/wrappers_test.go": `//go:build e2e

package wrappers

import "testing"

func expectMentions(t *testing.T, text string, substrings ...string) {
	assertMentions(t, "the wrapped refusal", text, substrings...)
}

func expectHint(t *testing.T, text, hint string) {
	assertMentions(t, "the wrapped hint", text, hint)
}

func expectWanted(t *testing.T, text string, wants ...string) {
	assertMentions(t, "the wrapped wants", text, wants...)
}

func quoteThroughWrappers(t *testing.T, scope string) {
	expectMentions(t, "text", "from a forwarding wrapper's caller", "from a concatenation in the "+scope)
	expectHint(t, "text", "from a hint wrapper's caller")
	expectWanted(t, "text", "not read: a wrapper whose parameter says nothing")
}
`,
	}))
	sites := sitesIn(read.sites, suiteFixtureDir+"/wrappers")

	want := []string{"from a concatenation in the  ", "from a forwarding wrapper's caller", "from a hint wrapper's caller"}
	if got := valuesOfKind(sites, kindAssertion); !slices.Equal(got, want) {
		t.Errorf("assertion values = %q\nwant %q", got, want)
	}
	if got := unresolvedExprs(sites); !slices.Equal(got, []string{"scope", "wants"}) {
		t.Errorf("unresolved = %v, want the unfolded half and the parameter no rule follows each named once", got)
	}
}

// TestCollectAssertionSites_ANeedleConcatenatedFromAName_ReadsTheNameToo holds
// the half of a concatenated needle the fold does not keep. A needle written
// as a literal plus a local is a claim about the whole sentence, so the local
// is read out to the value it is given, where a tool name is judged, and a
// half no rule can read is named rather than dropped: a needle that kept only
// its literal half was counted as folded, and the tool name it asserted was
// judged nowhere.
func TestCollectAssertionSites_ANeedleConcatenatedFromAName_ReadsTheNameToo(t *testing.T) {
	read := collectPlanted(t, suiteOverlay(t, map[string]string{
		"halves/doc.go":          "// Package halves quotes needles built from two halves.\npackage halves\n",
		"halves/helpers_test.go": suiteHelpersIn("halves"),
		"halves/halves_test.go": `//go:build e2e

package halves

import (
	"fmt"
	"testing"
)

func TestHalves(t *testing.T) {
	tool := "gitlab_demo_list"
	assertMentions(t, "a refusal", "text", "use "+tool)
	assertMentions(t, "a refusal", "text", fmt.Sprint(1)+" was refused")
}
`,
	}))
	sites := sitesIn(read.sites, suiteFixtureDir+"/halves")

	want := []string{"  was refused", "gitlab_demo_list", "use  "}
	if got := valuesOfKind(sites, kindAssertion); !slices.Equal(got, want) {
		t.Errorf("assertion values = %q\nwant %q", got, want)
	}
	if got := unresolvedExprs(sites); !slices.Equal(got, []string{"fmt.Sprint(1)"}) {
		t.Errorf("unresolved = %v, want the half no rule reads named once", got)
	}
}

// TestCollectAssertionSites_NoPatternMatch_IsAnError holds that a suite
// pattern matching nothing is refused rather than reported as a clean suite.
func TestCollectAssertionSites_NoPatternMatch_IsAnError(t *testing.T) {
	if _, err := collectAssertionSites(repoRoot(t), []string{"./" + suiteFixtureDir + "/nothing/..."}, nil); err == nil {
		t.Fatal("collectAssertionSites over a pattern that matches nothing returned no error")
	}
}

// TestCollectAssertionSites_RootThatCannotBeResolved_IsAnError holds the one
// failure between loading the suite and walking it, reached through the same
// seam the served walk's is.
func TestCollectAssertionSites_RootThatCannotBeResolved_IsAnError(t *testing.T) {
	overlay := suiteOverlay(t, nil)
	restore := absolutePath
	absolutePath = func(string) (string, error) { return "", errors.New("no working directory") }
	t.Cleanup(func() { absolutePath = restore })

	read, err := collectAssertionSites(repoRoot(t), []string{suiteFixturePattern}, overlay)
	if err == nil {
		t.Fatalf("collectAssertionSites() = %+v, want the root failure reported", read)
	}
	if !strings.Contains(err.Error(), "no working directory") {
		t.Errorf("error = %v, want the reason the root could not be resolved", err)
	}
}

// TestSuiteVariants_EachShapeALoadReturns_KeepsOneCopy holds the selection on
// the metadata go list fills: a variant names the package it tests, the
// package itself names none, and only the internal variant, whose path is the
// tested package's own, stands in for it. A package whose tests are all
// external has no internal variant, so it is kept beside its external test
// package.
func TestSuiteVariants_EachShapeALoadReturns_KeepsOneCopy(t *testing.T) {
	plain := &packages.Package{PkgPath: "m/p"}
	variant := &packages.Package{PkgPath: "m/p", ForTest: "m/p"}
	external := &packages.Package{PkgPath: "m/p_test", ForTest: "m/p"}
	testMain := &packages.Package{PkgPath: "m/p.test"}
	untested := &packages.Package{PkgPath: "m/q"}

	cases := []struct {
		name   string
		loaded []*packages.Package
		want   []*packages.Package
	}{
		{name: "a tested package keeps its variant only", loaded: []*packages.Package{plain, variant}, want: []*packages.Package{variant}},
		{name: "a package with no tests is kept", loaded: []*packages.Package{untested}, want: []*packages.Package{untested}},
		{
			name:   "a package whose tests are all external is kept",
			loaded: []*packages.Package{plain, external, testMain},
			want:   []*packages.Package{plain, external, testMain},
		},
		{
			name:   "the external test package and the test main are kept",
			loaded: []*packages.Package{plain, variant, external, testMain, untested},
			want:   []*packages.Package{variant, external, testMain, untested},
		},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			if got := suiteVariants(one.loaded); !slices.Equal(got, one.want) {
				t.Errorf("suiteVariants = %v, want %v", pkgPaths(got), pkgPaths(one.want))
			}
		})
	}
}

// pkgPaths renders a list of packages for a failure message.
func pkgPaths(pkgs []*packages.Package) []string {
	paths := make([]string, 0, len(pkgs))
	for _, pkg := range pkgs {
		paths = append(paths, pkg.PkgPath+" for "+pkg.ForTest)
	}
	return paths
}

// TestStaleHelpers_EachRun_NamesWhatItCanJudge holds the helper table's two
// staleness rules, the run each is allowed on, and the remedy each row
// carries: a mismatch names the copy of the helper that disagrees and sends
// the reader to its parameter, an entry nothing calls sends them to the entry.
// Two copies of one helper in two packages are two rows, since each is a
// parameter to rename.
func TestStaleHelpers_EachRun_NamesWhatItCanJudge(t *testing.T) {
	everyHelperOnce := map[string]int{"assertMentions": 1, "ExpectToolError": 1, "mentionsAny": 1, "containsAny": 1}
	mismatchRow := func(pkg, name, param string) string {
		return pkg + ": " + name + " takes no parameter named " + param + " (servedTextAssertions). " +
			"The entry names one parameter for every copy of " + name + ", so rename this copy's parameter to " +
			param + ", or the entry and every copy together"
	}
	cases := []struct {
		name       string
		calls      map[string]int
		mismatches map[helperCopy]string
		wholeSuite bool
		want       []string
	}{
		{
			name:       "a helper nothing calls, over the whole suite",
			calls:      map[string]int{"assertMentions": 3, "ExpectToolError": 1},
			wholeSuite: true,
			want: []string{
				"containsAny is called nowhere in the suite (servedTextAssertions). Fix the entry",
				"mentionsAny is called nowhere in the suite (servedTextAssertions). Fix the entry",
			},
		},
		{name: "a helper nothing calls, over part of it", calls: map[string]int{"assertMentions": 3}, wholeSuite: false},
		{name: "every helper called", calls: everyHelperOnce, wholeSuite: true},
		{
			name:       "a mismatch, over part of it",
			calls:      map[string]int{"mentionsAny": 1},
			mismatches: map[helperCopy]string{{pkg: "test/e2e/gitlab/common", name: "mentionsAny"}: "substrings"},
			want:       []string{mismatchRow("test/e2e/gitlab/common", "mentionsAny", "substrings")},
		},
		{
			name:  "two copies of one helper, each its own row",
			calls: map[string]int{"assertMentions": 2},
			mismatches: map[helperCopy]string{
				{pkg: "test/e2e/gitlab/ee", name: "assertMentions"}:     "substrings",
				{pkg: "test/e2e/gitlab/common", name: "assertMentions"}: "substrings",
			},
			want: []string{
				mismatchRow("test/e2e/gitlab/common", "assertMentions", "substrings"),
				mismatchRow("test/e2e/gitlab/ee", "assertMentions", "substrings"),
			},
		},
		{
			name:       "both, in order",
			calls:      map[string]int{"assertMentions": 1, "ExpectToolError": 1, "mentionsAny": 1},
			mismatches: map[helperCopy]string{{pkg: "test/e2e/internal/harness", name: "ExpectToolError"}: "contains"},
			wholeSuite: true,
			want: []string{
				"containsAny is called nowhere in the suite (servedTextAssertions). Fix the entry",
				mismatchRow("test/e2e/internal/harness", "ExpectToolError", "contains"),
			},
		},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			if got := staleHelpers(one.calls, one.mismatches, one.wholeSuite); !slices.Equal(got, one.want) {
				t.Errorf("staleHelpers = %q\nwant %q", got, one.want)
			}
		})
	}
}

// TestParamIndex_ByName_FindsThePosition holds the lookup the argument is read
// by: the position of the first parameter of that name, and -1 for none.
func TestParamIndex_ByName_FindsThePosition(t *testing.T) {
	param := func(name string) *types.Var {
		return types.NewParam(token.NoPos, nil, name, types.Typ[types.String])
	}
	params := types.NewTuple(param("text"), param("contains"), param("opts"), param("contains"))

	cases := map[string]int{"text": 0, "contains": 1, "opts": 2, "substrings": -1}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := paramIndex(params, name); got != want {
				t.Errorf("paramIndex(%q) = %d, want %d", name, got, want)
			}
		})
	}
}

// TestIsAssertionParamName_DeclaredNames_AreFollowed holds the names a suite
// wrapper's parameter is followed under beyond a hint's: the ones the helper
// table declares, exactly.
func TestIsAssertionParamName_DeclaredNames_AreFollowed(t *testing.T) {
	cases := map[string]bool{
		"substrings": true,
		"needles":    true,
		"contains":   true,
		"wants":      false,
		"hint":       false,
		"Substrings": false,
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isAssertionParamName(name); got != want {
				t.Errorf("isAssertionParamName(%q) = %t, want %t", name, got, want)
			}
		})
	}
}

// TestServedTextAssertions_EveryEntry_NamesAParameter holds the table's own
// shape: an entry with no parameter would match no signature and be reported
// as a mismatch on its first call, which is a rule firing on the table rather
// than on the suite.
func TestServedTextAssertions_EveryEntry_NamesAParameter(t *testing.T) {
	names := slices.Collect(maps.Keys(servedTextAssertions))
	sort.Strings(names)
	if want := []string{"ExpectToolError", "assertMentions", "containsAny", "mentionsAny"}; !slices.Equal(names, want) {
		t.Errorf("helpers = %v, want %v", names, want)
	}
	for name, helper := range servedTextAssertions {
		t.Run(name, func(t *testing.T) {
			if helper.param == "" {
				t.Errorf("%s declares no parameter", name)
			}
		})
	}
}
