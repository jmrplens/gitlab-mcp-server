// orphan_specs_test.go covers the rule that holds every exported ActionSpecs
// under internal/tools to something that aggregates it.
package main

import (
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
// name inherits an excuse nobody wrote for it.
func TestOrphanActionSpecGaps_ReportsBothDirections(t *testing.T) {
	declaredOrphanActionSpecs["excused_example"] = orphanActionSpecsDeclaration{
		Category: "test-fixture",
		Reason:   "declared by this test only.",
	}
	declaredOrphanActionSpecs["stale_example"] = orphanActionSpecsDeclaration{
		Category: "test-fixture",
		Reason:   "declared by this test only, and matching nothing.",
	}
	t.Cleanup(func() {
		delete(declaredOrphanActionSpecs, "excused_example")
		delete(declaredOrphanActionSpecs, "stale_example")
	})

	gaps := orphanActionSpecGaps([]string{"excused_example", "reported_example"})

	if !slices.IsSorted(gaps) {
		t.Errorf("gaps = %v, want them sorted so a run reports the same order twice", gaps)
	}
	if !hasGapNaming(gaps, "reported_example") {
		t.Errorf("gaps = %v, want the undeclared orphan reported", gaps)
	}
	if hasGapNaming(gaps, "excused_example") {
		t.Errorf("gaps = %v, want the declared orphan excused", gaps)
	}
	if !hasGapNaming(gaps, "stale_example") {
		t.Errorf("gaps = %v, want the declaration that matches nothing reported", gaps)
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

// hasGapNaming reports whether any finding names the package.
func hasGapNaming(gaps []string, packageName string) bool {
	for _, gap := range gaps {
		if strings.Contains(gap, packageName) {
			return true
		}
	}
	return false
}
