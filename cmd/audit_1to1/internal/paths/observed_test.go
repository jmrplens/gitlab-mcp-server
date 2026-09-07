package paths

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/requestinventory"
)

// withDeclarations replaces the declaration table for the length of a test, so
// a case can describe a tree the repository does not have without waiting for
// the repository to grow one.
func withDeclarations(t *testing.T, table map[string]silentOwnerDeclaration) {
	t.Helper()
	original := declaredSilentOwners
	declaredSilentOwners = table
	t.Cleanup(func() { declaredSilentOwners = original })
}

// makeToolsPackage creates internal/tools/<name> under root, which is how a
// package that recorded nothing is told from a name that is no package.
func makeToolsPackage(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(requestinventory.ToolsDir), name), 0o750); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
}

// TestObserved_SilentOwners_AreHeldToADeclaration verifies the three statuses
// an owner can have, since the gate is exactly the undeclared one and reading
// the other two as findings would make it unfailable in practice.
func TestObserved_SilentOwners_AreHeldToADeclaration(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "issues")
	makeToolsPackage(t, root, "adminspecs")
	makeToolsPackage(t, root, "forgotten")
	withDeclarations(t, map[string]silentOwnerDeclaration{
		"adminspecs": {Category: categoryRecordedElsewhere, Reason: "its handlers live elsewhere"},
	})
	rows := []requestinventory.Row{{Package: "internal/tools/issues", Kind: requestinventory.KindREST, Path: "/projects/:id/issues"}}
	actions := []requestinventory.Action{
		{ID: "issue.list", Owner: "issues"},
		{ID: "topic.list", Owner: "adminspecs"},
		{ID: "widget.list", Owner: "forgotten"},
		{ID: "ghost.list", Owner: "nowhere"},
	}

	coverage, owners := observed(root, rows, actions)

	if coverage.Covered != 1 || coverage.Silent != 2 || coverage.Unmapped != 1 {
		t.Fatalf("coverage = %+v, want one covered, two silent and one unmapped", coverage)
	}
	byPackage := map[string]SilentOwner{}
	for _, owner := range owners {
		byPackage[owner.Package] = owner
	}
	cases := []struct {
		pkg  string
		want string
	}{
		{pkg: "adminspecs", want: statusDeclared},
		{pkg: "forgotten", want: statusUndeclared},
		{pkg: "nowhere", want: statusUnmapped},
	}
	for _, testCase := range cases {
		t.Run(testCase.pkg, func(t *testing.T) {
			if got := byPackage[testCase.pkg].Status; got != testCase.want {
				t.Errorf("%s status = %q, want %q", testCase.pkg, got, testCase.want)
			}
		})
	}
	t.Run("a declaration carries its reason into the report", func(t *testing.T) {
		if byPackage["adminspecs"].Reason == "" || byPackage["adminspecs"].Category != categoryRecordedElsewhere {
			t.Errorf("declared owner = %+v, want the category and reason from the table", byPackage["adminspecs"])
		}
	})
	t.Run("owners are sorted", func(t *testing.T) {
		names := make([]string, 0, len(owners))
		for _, owner := range owners {
			names = append(names, owner.Package)
		}
		if !slices.IsSorted(names) {
			t.Errorf("owners = %v, want them sorted", names)
		}
	})
}

// TestUndeclaredSilent_CountsPackagesAndActions verifies what the gate fails
// on: a package with no recording and no declaration, and every action it owns.
func TestUndeclaredSilent_CountsPackagesAndActions(t *testing.T) {
	owners := []SilentOwner{
		{Package: "declared", Status: statusDeclared, Actions: []string{"a.one", "a.two"}},
		{Package: "one", Status: statusUndeclared, Actions: []string{"b.one"}},
		{Package: "two", Status: statusUndeclared, Actions: []string{"c.one", "c.two"}},
		{Package: "elsewhere", Status: statusUnmapped, Actions: []string{"d.one"}},
	}

	packages, actions := undeclaredSilent(owners)

	if packages != 2 || actions != 3 {
		t.Errorf("undeclaredSilent() = %d package(s), %d action(s); want 2 and 3", packages, actions)
	}
}

// TestStaleDeclarations_AClaimThatNoLongerHolds_IsAFinding verifies the other
// half of the declaration table. The claim is the only thing standing between a
// package and the gate, so a claim that has stopped being true has to be
// retired rather than left for a later reader to trust.
func TestStaleDeclarations_AClaimThatNoLongerHolds_IsAFinding(t *testing.T) {
	withDeclarations(t, map[string]silentOwnerDeclaration{
		"adminspecs": {Category: categoryRecordedElsewhere, Reason: "its handlers live elsewhere"},
		"retired":    {Category: categoryRecordedElsewhere, Reason: "no longer owns an action"},
	})

	stale := staleDeclarations([]SilentOwner{{Package: "adminspecs", Status: statusDeclared}})

	if len(stale) != 1 {
		t.Fatalf("staleDeclarations() = %v, want the one claim that no longer holds", stale)
	}
	if !strings.Contains(stale[0], "retired") {
		t.Errorf("staleDeclarations() = %q, want it to name the retired declaration", stale[0])
	}
}

// TestStaleDeclarations_EveryClaimStillHolds_IsEmpty verifies the clean case
// returns an empty list rather than nil, so the report renders it the same way
// every run.
func TestStaleDeclarations_EveryClaimStillHolds_IsEmpty(t *testing.T) {
	withDeclarations(t, map[string]silentOwnerDeclaration{
		"adminspecs": {Category: categoryRecordedElsewhere, Reason: "its handlers live elsewhere"},
	})

	stale := staleDeclarations([]SilentOwner{{Package: "adminspecs", Status: statusDeclared}})

	if stale == nil || len(stale) != 0 {
		t.Errorf("staleDeclarations() = %v, want an empty list", stale)
	}
}

// TestDeclaredSilentOwners_EveryEntry_ExplainsItself verifies the committed
// table, since an entry with no reason is a package excused from the gate for
// nothing anybody can review.
func TestDeclaredSilentOwners_EveryEntry_ExplainsItself(t *testing.T) {
	for pkg, declaration := range declaredSilentOwners {
		t.Run(pkg, func(t *testing.T) {
			if declaration.Category != categoryRecordedElsewhere {
				t.Errorf("category = %q, want one of the declared categories", declaration.Category)
			}
			if len(declaration.Reason) < 40 {
				t.Errorf("reason = %q, want an argument rather than a label", declaration.Reason)
			}
		})
	}
}
