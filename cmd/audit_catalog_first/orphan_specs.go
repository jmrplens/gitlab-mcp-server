package main

import (
	"fmt"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
)

// toolsPathPrefix is the import-path prefix of the packages this rule reads.
const toolsPathPrefix = goprogram.ModulePath + "/internal/tools/"

// actionSpecsFuncName is the exported function every domain package used to
// declare and that the catalog aggregates by calling.
const actionSpecsFuncName = "ActionSpecs"

// assertActionSpecsAreAggregated fails when a package under internal/tools
// declares an exported ActionSpecs that no production file calls.
//
// It is the rule the rest of this command could not express. Everything else
// here reads the source for a function of that name and treats its presence as
// health: HasActionSpecsFunction is set by seeing the declaration, and it
// feeds both HasMetaSpecs and HasIndividualTools. A package whose specs
// nothing aggregates therefore satisfied the gate, and deleting the dead file
// would have made the gate less happy rather than more. Twenty-three packages
// were in exactly that state, each declaring a full set of specs with usage
// lines, aliases, tags and parameter guidance that no surface ever served,
// while the served copies sat in internal/tools/adminspecs and had drifted
// from them word for word. A maintainer correcting the text in the obvious
// place changed nothing a model reads, and got a green build and green tests
// for it.
//
// The join is the call, resolved through the type checker, and never the
// action's OwnerPackage. Those are different questions and the second one
// cannot answer this: an admin action's owner is the domain package whose
// handler it routes to, so the twenty-three would all report actions owned by
// them while their own ActionSpecs stayed unreachable. A text scan is worse
// still, and wrong in both directions here: internal/tools/action_specs.go
// imports internal/tools/groups under the alias grouptools, so a grep for
// "groups.ActionSpecs" reports a package that is aggregated; and
// "settings.ActionSpecs" is a substring of "mrapprovalsettings.ActionSpecs",
// so the same grep misses a package that is not. The type checker's record has
// neither problem, and it also sees a reference that is not a call:
// internal/tools/register_mcp_meta.go names health.ActionSpecs as a function
// value with no parentheses, which is what made health look orphaned in the
// first reading of this class.
func assertActionSpecsAreAggregated(root string) error {
	orphans, err := orphanActionSpecPackages(root)
	if err != nil {
		return err
	}
	if gaps := orphanActionSpecGaps(orphans); len(gaps) > 0 {
		return fmt.Errorf("action spec aggregation invariants failed: %s", strings.Join(gaps, "; "))
	}
	return nil
}

// orphanActionSpecGaps is the verdict on one reading of the tree: every orphan
// no declaration answers, and every declaration no orphan matches.
func orphanActionSpecGaps(orphans []string) []string {
	var gaps []string
	for _, pkg := range orphans {
		if _, declared := declaredOrphanActionSpecs[pkg]; declared {
			continue
		}
		gaps = append(gaps, pkg+" declares an exported "+actionSpecsFuncName+" that no production file calls; aggregate it into the catalog or delete it")
	}
	gaps = append(gaps, staleOrphanDeclarations(orphans)...)
	sort.Strings(gaps)
	return gaps
}

// staleOrphanDeclarations names every declaration that no longer describes the
// tree, which is a finding on the same terms as the state it excuses: a
// declaration nothing matches is a claim about a package that has since been
// aggregated or deleted, and leaving it in place would silently excuse the
// next package to take that name.
func staleOrphanDeclarations(orphans []string) []string {
	found := make(map[string]struct{}, len(orphans))
	for _, pkg := range orphans {
		found[pkg] = struct{}{}
	}

	var stale []string
	for pkg, declaration := range declaredOrphanActionSpecs {
		if _, ok := found[pkg]; ok {
			continue
		}
		stale = append(stale, fmt.Sprintf("declaration for %s (%s) matches nothing: its %s is aggregated or gone", pkg, declaration.Category, actionSpecsFuncName))
	}
	return stale
}

// orphanActionSpecPackages returns, sorted, the import paths under
// internal/tools whose exported ActionSpecs is referenced by no production
// file anywhere in internal/... .
//
// Test variants are loaded on purpose: a reference from a _test.go file is
// exactly what an orphan still has, so seeing those files is what lets the
// rule tell "nothing calls this" from "nothing at all mentions it".
func orphanActionSpecPackages(root string) ([]string, error) {
	aggregated, err := actionSpecsAggregation(root)
	if err != nil {
		return nil, err
	}

	var orphans []string
	for pkg, called := range aggregated {
		if !called {
			orphans = append(orphans, pkg)
		}
	}
	sort.Strings(orphans)
	return orphans, nil
}

// actionSpecsAggregation maps every package under internal/tools that declares
// an exported ActionSpecs, named as the repository names it, to whether a
// production file references it.
//
// It reads ./internal/... and nothing else, because every aggregator is there:
// internal/tools/action_specs.go, internal/tools/runners and
// internal/tools/surfaces. The two references from cmd/ are both in test files
// and so would not count as aggregation even if they were loaded.
func actionSpecsAggregation(root string) (map[string]bool, error) {
	loaded, err := goprogram.LoadWith(root, []string{"./internal/..."}, goprogram.Options{Tests: true})
	if err != nil {
		return nil, fmt.Errorf("load internal packages: %w", err)
	}

	declared := declaredActionSpecs(loaded)
	for _, pkg := range loaded {
		recordProductionUses(pkg, declared)
	}

	aggregated := make(map[string]bool, len(declared))
	for path, called := range declared {
		aggregated[strings.TrimPrefix(path, toolsPathPrefix)] = called
	}
	return aggregated, nil
}

// declaredActionSpecs maps the import path of every package under
// internal/tools that declares an exported ActionSpecs to whether a production
// file has been seen referencing it, which starts false for all of them.
func declaredActionSpecs(loaded []*packages.Package) map[string]bool {
	declared := map[string]bool{}
	for _, pkg := range loaded {
		if pkg.Types == nil || !strings.HasPrefix(pkg.Types.Path(), toolsPathPrefix) {
			continue
		}
		object := pkg.Types.Scope().Lookup(actionSpecsFuncName)
		if function, ok := object.(*types.Func); ok && function.Exported() {
			declared[pkg.Types.Path()] = false
		}
	}
	return declared
}

// recordProductionUses marks as called every declared ActionSpecs that a
// non-test file of pkg names, whether it calls it, takes it as a value or
// passes it along.
func recordProductionUses(pkg *packages.Package, declared map[string]bool) {
	if pkg.TypesInfo == nil {
		return
	}
	for ident, object := range pkg.TypesInfo.Uses {
		function, ok := object.(*types.Func)
		if !ok || function.Name() != actionSpecsFuncName || function.Pkg() == nil {
			continue
		}
		if _, tracked := declared[function.Pkg().Path()]; !tracked {
			continue
		}
		if strings.HasSuffix(pkg.Fset.Position(ident.Pos()).Filename, testGoSuffix) {
			continue
		}
		declared[function.Pkg().Path()] = true
	}
}
