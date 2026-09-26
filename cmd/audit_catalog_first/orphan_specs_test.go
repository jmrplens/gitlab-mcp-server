// orphan_specs_test.go covers the rule that holds every exported ActionSpecs
// under internal/tools to something that aggregates it.
package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// cachedAggregation reads the tree once and shares the answer: the rule loads
// and type-checks ./internal/... , which is the most expensive thing this
// package does, and every test below asks a different question of the same
// reading.
var (
	aggregationOnce sync.Once
	cachedAggregate map[string]bool
	errAggregate    error
)

func aggregationForTest(t *testing.T) map[string]bool {
	t.Helper()
	aggregationOnce.Do(func() {
		root, err := cmdutil.RepositoryRoot("../..")
		if err != nil {
			errAggregate = err
			return
		}
		cachedAggregate, errAggregate = actionSpecsAggregation(root)
	})
	if errAggregate != nil {
		t.Fatalf("actionSpecsAggregation() error = %v", errAggregate)
	}
	return cachedAggregate
}

// TestActionSpecsAreAggregated_OnThisTree is the gate itself, run over the
// repository rather than over a planted fixture, because the question it asks
// is about this tree: which packages declare model-facing metadata that no
// surface ever serves.
func TestActionSpecsAreAggregated_OnThisTree(t *testing.T) {
	root, err := cmdutil.RepositoryRoot("../..")
	if err != nil {
		t.Fatalf("cmdutil.RepositoryRoot() error = %v", err)
	}
	if assertErr := assertActionSpecsAreAggregated(root); assertErr != nil {
		t.Error(assertErr)
	}
}

// TestActionSpecsAggregation_ResolvesReferencesTheTypeCheckerSeesAndAGrepDoesNot
// pins the two references that decide whether this rule can be written as a
// text scan at all. It cannot.
//
// internal/tools/action_specs.go imports internal/tools/groups under the alias
// grouptools, so the call reads "grouptools.ActionSpecs" and a grep for the
// package's own name finds nothing; and internal/tools/register_mcp_meta.go
// names health.ActionSpecs as a function value with no call parentheses, so a
// grep for ".ActionSpecs(" misses it. Both are aggregated, and reading either
// one wrongly would put a live surface on the deletion list: the second is
// what made health look like a twenty-fourth orphan, and its specs back
// gitlab_server, the tool a model reaches for when the instance is already
// unreachable.
func TestActionSpecsAggregation_ResolvesReferencesTheTypeCheckerSeesAndAGrepDoesNot(t *testing.T) {
	aggregated := aggregationForTest(t)

	for _, pkg := range []string{"groups", "health"} {
		t.Run(pkg, func(t *testing.T) {
			called, declared := aggregated[pkg]
			if !declared {
				t.Fatalf("%s declares no exported ActionSpecs, so this test asserts nothing", pkg)
			}
			if !called {
				t.Errorf("%s reads as orphaned; its ActionSpecs is aggregated", pkg)
			}
		})
	}
}

// TestActionSpecsAggregation_ReadsEveryPackageThatDeclaresSpecs guards the
// other way a rule like this goes quiet: matching nothing. A load that
// resolved no package, or a prefix that stopped matching, would report no
// orphans and pass for ever.
func TestActionSpecsAggregation_ReadsEveryPackageThatDeclaresSpecs(t *testing.T) {
	aggregated := aggregationForTest(t)

	if len(aggregated) < 100 {
		t.Fatalf("packages declaring ActionSpecs = %d, want the whole domain family: the load or the path prefix stopped matching", len(aggregated))
	}
	if _, ok := aggregated["adminspecs"]; !ok {
		t.Error("adminspecs declares the instance-administration specs and is missing from the reading")
	}
}

// TestOrphanActionSpecGaps_ReportsBothDirections verifies that an orphan no
// declaration answers is a finding, that a declared one is not, and that a
// declaration matching nothing is a finding of its own.
//
// The last is the half that rots quietly: a package that is later aggregated
// or deleted leaves its declaration behind, and the next package to take that
// name inherits an excuse nobody wrote for it. The two declarations carry
// different categories so the stale finding is held to its own.
func TestOrphanActionSpecGaps_ReportsBothDirections(t *testing.T) {
	declaredOrphanActionSpecs["excused_example"] = orphanActionSpecsDeclaration{
		Category: "excused-fixture",
		Reason:   "declared by this test only.",
	}
	declaredOrphanActionSpecs["stale_example"] = orphanActionSpecsDeclaration{
		Category: "stale-fixture",
		Reason:   "declared by this test only, and matching nothing.",
	}
	t.Cleanup(func() {
		delete(declaredOrphanActionSpecs, "excused_example")
		delete(declaredOrphanActionSpecs, "stale_example")
	})

	gaps := orphanActionSpecGaps([]string{"excused_example", "reported_example"})

	want := []string{
		"declaration for stale_example (stale-fixture) matches nothing: its ActionSpecs is aggregated or gone",
		"reported_example declares an exported ActionSpecs that no production file calls; aggregate it into the catalog or delete it",
	}
	if !slices.Equal(gaps, want) {
		t.Errorf("gaps = %q, want %q", gaps, want)
	}
}

// TestOrphanActionSpecGaps_CleanTree_ReportsNothing verifies the quiet case:
// no orphans and no declarations is no finding, which is the state this
// repository is in and the state the empty table documents.
func TestOrphanActionSpecGaps_CleanTree_ReportsNothing(t *testing.T) {
	if gaps := orphanActionSpecGaps(nil); len(gaps) != 0 {
		t.Errorf("orphanActionSpecGaps(nil) = %v, want no findings", gaps)
	}
}

// orphanFixtureModule is a module the aggregation rule can be run over end to
// end: one package whose specs a production file calls, one whose specs only a
// test file names, and one outside internal/tools whose specs are called from
// production.
//
// The module path has to be this repository's own, because that prefix is how
// the rule decides which packages it is judging at all; the fixture is loaded
// from its own directory, so the two never meet.
func orphanFixtureModule() map[string]string {
	module := strings.TrimSuffix(toolsPathPrefix, "/internal/tools/")
	return map[string]string{
		"go.mod":                       "module " + module + "\n\ngo 1.27\n",
		"internal/tools/alpha/spec.go": "package alpha\n\nfunc ActionSpecs() {}\n",
		"internal/tools/beta/spec.go":  "package beta\n\nfunc ActionSpecs() {}\n",
		"internal/helper/spec.go":      "package helper\n\nfunc ActionSpecs() {}\n",
		"internal/tools/aggregate.go": fmt.Sprintf(
			"package tools\n\nimport (\n\t%q\n\t%q\n)\n\nfunc aggregate() {\n\tbeta.ActionSpecs()\n\thelper.ActionSpecs()\n}\n",
			module+"/internal/tools/beta", module+"/internal/helper",
		),
		"internal/tools/aggregate_test.go": fmt.Sprintf(
			"package tools\n\nimport %q\n\nvar _ = alpha.ActionSpecs\n", module+"/internal/tools/alpha",
		),
	}
}

// TestActionSpecsAggregation_FixtureModule_SeparatesTheThreeStates verifies
// the three answers the rule can give about a package, over a module planted
// for the purpose: called from production, named only from a test, and not a
// package this rule judges at all.
//
// The middle one is the state the rule exists to catch and the one this tree
// has never been in, so nothing had ever seen the reading that produces it;
// the last matters because the recorded uses include every ActionSpecs in the
// loaded program, and counting one declared outside internal/tools would make
// a genuine orphan read as aggregated.
//
// The whole map is compared: a package outside the prefix is keyed by its full
// import path, so looking it up by its short name found nothing either way.
func TestActionSpecsAggregation_FixtureModule_SeparatesTheThreeStates(t *testing.T) {
	root := writeCatalogFirstFixture(t, orphanFixtureModule())

	aggregated, err := actionSpecsAggregation(root)
	if err != nil {
		t.Fatalf("actionSpecsAggregation() error = %v", err)
	}

	want := map[string]bool{
		"alpha": false, // an orphan only a test file names
		"beta":  true,  // a package a production file calls
	}
	if !maps.Equal(aggregated, want) {
		t.Errorf("aggregation = %v, want %v: internal/helper is outside internal/tools", aggregated, want)
	}
}

// TestAssertActionSpecsAreAggregated_FixtureModule_ReportsBothDirections
// verifies the gate over a planted module reports the orphan it finds and the
// declaration that matches nothing, in one refusal.
//
// Over this repository the rule has nothing to say, which is the healthy state
// and also the reason the branch that reports anything at all was reachable
// from no test: a rule that stopped finding orphans would have looked exactly
// like the tree being clean.
func TestAssertActionSpecsAreAggregated_FixtureModule_ReportsBothDirections(t *testing.T) {
	declaredOrphanActionSpecs["gamma"] = orphanActionSpecsDeclaration{
		Category: "test-fixture",
		Reason:   "declared by this test only, and matching nothing in the fixture.",
	}
	t.Cleanup(func() { delete(declaredOrphanActionSpecs, "gamma") })

	root := writeCatalogFirstFixture(t, orphanFixtureModule())

	err := assertActionSpecsAreAggregated(root)
	if err == nil {
		t.Fatal("assertActionSpecsAreAggregated() = nil, want the orphan and the stale declaration reported")
	}
	findings := []struct {
		name string
		want string
	}{
		{name: "the orphan", want: "alpha declares an exported ActionSpecs"},
		{name: "the stale declaration", want: "declaration for gamma"},
	}
	for _, finding := range findings {
		t.Run(finding.name, func(t *testing.T) {
			if !strings.Contains(err.Error(), finding.want) {
				t.Errorf("assertActionSpecsAreAggregated() = %v, want it to contain %q", err, finding.want)
			}
		})
	}
}

// TestAssertActionSpecsAreAggregated_UnloadableTree_IsRefused verifies a tree
// the loader cannot read is a refusal rather than an empty answer.
//
// It is the one failure the rule can have that says nothing about the source:
// a load that resolved no package would otherwise report no orphans and pass,
// which is the silence a gate must never produce.
func TestAssertActionSpecsAreAggregated_UnloadableTree_IsRefused(t *testing.T) {
	if err := assertActionSpecsAreAggregated(t.TempDir()); err == nil {
		t.Fatal("assertActionSpecsAreAggregated() = nil over a directory that is not a module, want a refusal")
	}
}
