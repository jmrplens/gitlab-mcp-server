package paths

import (
	"strings"
	"testing"
)

// withPaginationDeclarations replaces the declaration table for the length of a
// test, so a case can describe a tree the repository does not have and so that
// the committed entries do not silence a fixture that happens to share a name.
func withPaginationDeclarations(t *testing.T, table []paginationDeclaration) {
	t.Helper()
	original := declaredUnpaginatedCollections
	declaredUnpaginatedCollections = table
	t.Cleanup(func() { declaredUnpaginatedCollections = original })
}

// TestClassifyUnpaginated_ADeclaration_CarriesItsReasonIntoTheFinding verifies
// the half of the table a reader meets: a finding a declaration accounts for
// still appears, with the reason beside it, rather than being deleted. Deleting
// it would leave the next reader to rediscover the same endpoint.
func TestClassifyUnpaginated_ADeclaration_CarriesItsReasonIntoTheFinding(t *testing.T) {
	withPaginationDeclarations(t, []paginationDeclaration{{
		Package:  toolsDir + "/issues",
		Action:   "issue.participants",
		Category: categoryEndpointNotPaginated,
		Reason:   "the participants endpoint declares neither param",
	}})

	classified := classifyUnpaginated([]UnpaginatedCollection{
		{Package: toolsDir + "/issues", Action: "issue.participants"},
		{Package: toolsDir + "/issues", Action: "issue.list"},
	})

	cases := []struct {
		action string
		want   string
	}{
		{action: "issue.participants", want: categoryEndpointNotPaginated},
		{action: "issue.list", want: ""},
	}
	byAction := map[string]UnpaginatedCollection{}
	for _, finding := range classified {
		byAction[finding.Action] = finding
	}
	for _, testCase := range cases {
		t.Run(testCase.action, func(t *testing.T) {
			if got := byAction[testCase.action].Category; got != testCase.want {
				t.Errorf("category = %q, want %q", got, testCase.want)
			}
			if (byAction[testCase.action].Reason != "") != (testCase.want != "") {
				t.Errorf("reason = %q, want it present only with a category", byAction[testCase.action].Reason)
			}
			if byAction[testCase.action].declared() != (testCase.want != "") {
				t.Errorf("declared() = %t, want %t", byAction[testCase.action].declared(), testCase.want != "")
			}
		})
	}
}

// TestPaginationDeclaration_Covers_MatchesOnPackageAndAction verifies that a
// declaration is keyed on both halves, since an action name alone repeats across
// packages and excusing the wrong one would hide a real finding.
func TestPaginationDeclaration_Covers_MatchesOnPackageAndAction(t *testing.T) {
	declaration := paginationDeclaration{Package: toolsDir + "/issues", Action: "issue.participants"}
	cases := []struct {
		name    string
		finding UnpaginatedCollection
		want    bool
	}{
		{name: "the same package and action", finding: UnpaginatedCollection{Package: toolsDir + "/issues", Action: "issue.participants"}, want: true},
		{name: "another package, same action", finding: UnpaginatedCollection{Package: toolsDir + "/epics", Action: "issue.participants"}},
		{name: "the same package, another action", finding: UnpaginatedCollection{Package: toolsDir + "/issues", Action: "issue.list"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := declaration.covers(testCase.finding); got != testCase.want {
				t.Errorf("covers() = %t, want %t", got, testCase.want)
			}
		})
	}
}

// TestPaginationStaleDeclarations_AClaimThatMatchesNothing_IsAFinding verifies
// the other half of the table. A declaration is the only thing standing between
// an action and the finding list, so one that has stopped describing the tree
// has to be retired rather than left for a later reader to trust. This is the
// part of the rule that gates.
func TestPaginationStaleDeclarations_AClaimThatMatchesNothing_IsAFinding(t *testing.T) {
	withPaginationDeclarations(t, []paginationDeclaration{
		{Package: toolsDir + "/issues", Action: "issue.participants", Category: categoryEndpointNotPaginated, Reason: "sent whole"},
		{Package: toolsDir + "/retired", Action: "retired.list", Category: categoryGraphQLBacked, Reason: "answered over GraphQL"},
	})
	check := PaginationCheck{Ran: true, Unpaginated: []UnpaginatedCollection{
		{Package: toolsDir + "/issues", Action: "issue.participants"},
	}}

	stale := check.staleDeclarations()

	if len(stale) != 1 {
		t.Fatalf("staleDeclarations() = %v, want the one claim that no longer holds", stale)
	}
	if !strings.Contains(stale[0], "retired.list") {
		t.Errorf("staleDeclarations() = %q, want it to name the retired declaration", stale[0])
	}
}

// TestPaginationStaleDeclarations_EveryClaimHolds_IsEmpty verifies the clean
// case returns an empty list rather than nil, so the report renders the same way
// every run.
func TestPaginationStaleDeclarations_EveryClaimHolds_IsEmpty(t *testing.T) {
	withPaginationDeclarations(t, []paginationDeclaration{
		{Package: toolsDir + "/issues", Action: "issue.participants", Category: categoryEndpointNotPaginated, Reason: "sent whole"},
	})
	check := PaginationCheck{Ran: true, Unpaginated: []UnpaginatedCollection{
		{Package: toolsDir + "/issues", Action: "issue.participants"},
	}}

	stale := check.staleDeclarations()

	if stale == nil || len(stale) != 0 {
		t.Errorf("staleDeclarations() = %v, want an empty list", stale)
	}
}

// TestPaginationStaleDeclarations_CheckDidNotRun_ReportsNothing verifies that a
// run with no record reports no stale declaration. Without the guard, a machine
// that could not read the record would fail the gate with every entry of the
// table, which is the loudest possible wrong answer to "the record is missing".
func TestPaginationStaleDeclarations_CheckDidNotRun_ReportsNothing(t *testing.T) {
	if stale := (PaginationCheck{Ran: false}).staleDeclarations(); stale != nil {
		t.Errorf("staleDeclarations() = %v, want nothing from a check that did not run", stale)
	}
}

// TestDeclaredUnpaginatedCollections_EveryEntry_ExplainsItself verifies the
// committed table, since an entry with no argument is an action excused from the
// finding list for nothing anybody can review.
func TestDeclaredUnpaginatedCollections_EveryEntry_ExplainsItself(t *testing.T) {
	categories := map[string]bool{categoryEndpointNotPaginated: true, categoryGraphQLBacked: true}
	seen := map[string]bool{}
	for _, declaration := range declaredUnpaginatedCollections {
		t.Run(declaration.key(), func(t *testing.T) {
			if !categories[declaration.Category] {
				t.Errorf("category = %q, want one of the declared categories", declaration.Category)
			}
			if !strings.HasPrefix(declaration.Package, toolsDir+"/") {
				t.Errorf("package = %q, want it spelled the way the inventory spells one", declaration.Package)
			}
			if !strings.Contains(declaration.Action, ".") {
				t.Errorf("action = %q, want a canonical domain.action ID", declaration.Action)
			}
			if len(declaration.Reason) < 80 {
				t.Errorf("reason = %q, want an argument rather than a label", declaration.Reason)
			}
			if seen[declaration.key()] {
				t.Errorf("%s is declared twice", declaration.key())
			}
			seen[declaration.key()] = true
		})
	}
}

// TestDeclaredUnpaginatedCollections_EveryReason_CarriesItsCategorysEvidence
// verifies the bar the table sets for itself, which is not the same bar for both
// categories. An entry saying GitLab does not page the endpoint is admitted on
// the route the record mounts, so it has to name that path; an entry saying the
// action is answered over GraphQL has no REST route to name and has to say so
// instead. A reason that only asserts the conclusion is what this keeps out.
func TestDeclaredUnpaginatedCollections_EveryReason_CarriesItsCategorysEvidence(t *testing.T) {
	for _, declaration := range declaredUnpaginatedCollections {
		t.Run(declaration.key(), func(t *testing.T) {
			switch declaration.Category {
			case categoryEndpointNotPaginated:
				if !strings.Contains(declaration.Reason, " /") {
					t.Errorf("reason = %q, want it to name the route the record holds", declaration.Reason)
				}
			case categoryGraphQLBacked:
				if !strings.Contains(declaration.Reason, "GraphQL") {
					t.Errorf("reason = %q, want it to say what answers the action instead of a REST route", declaration.Reason)
				}
			}
		})
	}
}
