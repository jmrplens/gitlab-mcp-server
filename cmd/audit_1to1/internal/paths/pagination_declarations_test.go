// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package paths

import (
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
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

// withKeysetDeclarations replaces the keyset route declarations for the length
// of a test, for the reasons withPaginationDeclarations gives.
func withKeysetDeclarations(t *testing.T, table []keysetRouteDeclaration) {
	t.Helper()
	original := declaredKeysetRoutes
	declaredKeysetRoutes = table
	t.Cleanup(func() { declaredKeysetRoutes = original })
}

// TestKeysetPagination_ADeclaredRoute_IsOneThisServerCannotPageByKeyset
// verifies what a keyset route declaration does to the judgment of the routes
// it names. The record marks the route as reaching a keyset paginator and the
// declaration says GitLab serves this server no page there it can follow, so:
// an action offering page_token on it is on the offset list, with the route
// named as the reason it is there; a list sending it without page_token is
// declared rather than reported as a list that cannot ask for keyset; and an
// action sending it beside a route it can follow is held to that one alone.
func TestKeysetPagination_ADeclaredRoute_IsOneThisServerCannotPageByKeyset(t *testing.T) {
	withPaginationDeclarations(t, nil)
	withKeysetDeclarations(t, []keysetRouteDeclaration{{
		Route: "GET /projects/:id/anonymous", Category: categoryKeysetNotFollowable, Reason: "only a signed-out caller reaches it",
	}})
	half := keysetFixture{
		operations: map[string]response{
			"GET /projects/:id/anonymous": cursorRoute,
			"GET /projects/:id/cursors":   cursorRoute,
		},
		requests: []actionrequests.RecordAction{
			sending("thing.list", "things.List", "GET /projects/:id/anonymous"),
			sending("thing.search", "things.Search", "GET /projects/:id/anonymous"),
			sending("thing.both", "things.Both", "GET /projects/:id/anonymous", "GET /projects/:id/cursors"),
		},
		rows: []requestinventory.Row{followed("internal/tools/things", "cursor")},
		actions: []requestinventory.Action{
			listAction("thing.list", "things", reflect.TypeFor[fixturePaginatedListOutput](), withPageToken),
			listAction("thing.search", "things", reflect.TypeFor[fixturePaginatedListOutput](), withoutPageToken),
			listAction("thing.both", "things", reflect.TypeFor[fixturePaginatedListOutput](), withPageToken),
		},
	}.run(t)

	want := []keysetFindingJSON{
		{
			Action: "thing.list", Package: "internal/tools/things", Problem: keysetOnOffsetRoute,
			Routes: []string{"GET /projects/:id/anonymous"}, NotFollowable: []string{"GET /projects/:id/anonymous"},
		},
		{
			Action: "thing.search", Package: "internal/tools/things", Problem: keysetNotOffered,
			Routes: []string{"GET /projects/:id/anonymous"}, Cursors: []string{"cursor"},
			Category: categoryKeysetNotFollowable, Reason: "only a signed-out caller reaches it",
		},
	}
	if !reflect.DeepEqual(half.Findings, want) {
		t.Errorf("findings = %+v, want %+v", half.Findings, want)
	}
	if half.OnKeyset != 1 {
		t.Errorf("on keyset routes = %d, want the action sending a route it can follow beside the declared one", half.OnKeyset)
	}
	if len(half.Unused) != 0 {
		t.Errorf("unused declarations = %v, want the declaration used", half.Unused)
	}
}

// TestUnusedKeysetDeclarations_ADeclarationNoFindingUses_IsStale verifies the
// half of the table that gates: a route declaration is used when a finding
// names its route as one this server cannot follow or is a list it answers,
// and one neither does is reported, after the used ones are read past, since
// it would otherwise outlive the route it was written for.
func TestUnusedKeysetDeclarations_ADeclarationNoFindingUses_IsStale(t *testing.T) {
	withKeysetDeclarations(t, []keysetRouteDeclaration{
		{Route: "GET /named", Category: categoryKeysetNotFollowable, Reason: "named"},
		{Route: "GET /answered", Category: categoryKeysetNotFollowable, Reason: "answered"},
		{Route: "GET /retired", Category: categoryKeysetNotFollowable, Reason: "retired"},
	})

	unused := unusedKeysetDeclarations([]KeysetFinding{
		{Action: "a.list", Problem: keysetOnOffsetRoute, Routes: []string{"GET /named", "GET /x"}, NotFollowable: []string{"GET /named"}},
		{Action: "b.list", Problem: keysetNotOffered, Routes: []string{"GET /answered"}, Category: categoryKeysetNotFollowable},
		// An undeclared finding on a route keeps no declaration of it alive.
		{Action: "c.list", Problem: keysetNotOffered, Routes: []string{"GET /retired"}},
	})

	want := []string{"GET /retired is declared a route this server cannot page by keyset (keyset-not-followable) and no " +
		"finding uses it: the record no longer marks it as reaching a keyset paginator, or no action sends it"}
	if !reflect.DeepEqual(unused, want) {
		t.Errorf("unusedKeysetDeclarations() = %q, want %q", unused, want)
	}
}

// TestPaginationStaleDeclarations_TheKeysetHalfsUnusedOnes_JoinThem verifies
// that an unused route declaration reaches the gate the way every stale
// declaration does, through the pagination check's stale list.
func TestPaginationStaleDeclarations_TheKeysetHalfsUnusedOnes_JoinThem(t *testing.T) {
	withPaginationDeclarations(t, nil)
	check := PaginationCheck{Ran: true, Keyset: KeysetPagination{Ran: true, UnusedDeclarations: []string{"GET /retired is declared"}}}

	if stale := check.staleDeclarations(); !reflect.DeepEqual(stale, []string{"GET /retired is declared"}) {
		t.Errorf("staleDeclarations() = %v, want the keyset half's unused declaration", stale)
	}
}

// TestKeepUndeclaredKeyset_DropsWhatADeclarationAnswers verifies what
// -gaps-only asks of the keyset half, on the terms of the collection half: a
// declared finding is context rather than work.
func TestKeepUndeclaredKeyset_DropsWhatADeclarationAnswers(t *testing.T) {
	kept := keepUndeclaredKeyset([]KeysetFinding{
		{Action: "a.list", Category: categoryKeysetNotFollowable, Reason: "GitLab refuses keyset there"},
		{Action: "b.list"},
	})

	if len(kept) != 1 || kept[0].Action != "b.list" {
		t.Errorf("keepUndeclaredKeyset() = %+v, want only the undeclared finding", kept)
	}
}

// TestKeysetPagination_Counts_DivideTheFindingsByProblem verifies the figures
// the summary quotes: how many findings name each problem, and how many of
// them no declaration accounts for.
func TestKeysetPagination_Counts_DivideTheFindingsByProblem(t *testing.T) {
	check := KeysetPagination{Findings: []KeysetFinding{
		{Problem: keysetOnOffsetRoute},
		{Problem: keysetOnOffsetRoute},
		{Problem: keysetNotOffered, Category: categoryKeysetNotFollowable},
		{Problem: keysetNeverFollowed},
	}}

	for problem, want := range map[string]int{
		keysetOnOffsetRoute: 2, keysetNotOffered: 1, keysetNoNextPageToken: 0, keysetNeverFollowed: 1,
	} {
		t.Run(problem, func(t *testing.T) {
			if got := check.count(problem); got != want {
				t.Errorf("count(%q) = %d, want %d", problem, got, want)
			}
		})
	}
	if got := check.undeclared(); got != 3 {
		t.Errorf("undeclared() = %d, want the three findings no declaration answers", got)
	}
}

// keysetReasonSite is how a reason cites what it read in GitLab's source: a
// repository-relative Ruby file and a line in it.
var keysetReasonSite = regexp.MustCompile(`(?:ee/)?lib/[a-z0-9_/]+\.rb:[0-9]+`)

// TestDeclaredKeysetRoutes_EveryEntry_ExplainsItself verifies the committed
// table on the bar it sets: each entry is a route spelled the way the request
// record spells one, declared once, in the one category, with a reason that
// argues rather than labels and cites the handler it read, by file and line,
// since a reason that only asserts GitLab cannot page the route is the kind
// of entry that excused real findings in the collection half's table.
func TestDeclaredKeysetRoutes_EveryEntry_ExplainsItself(t *testing.T) {
	seen := map[string]bool{}
	for _, declaration := range declaredKeysetRoutes {
		t.Run(declaration.Route, func(t *testing.T) {
			if !strings.HasPrefix(declaration.Route, "GET /") {
				t.Errorf("route = %q, want a GET route spelled as the request record spells one", declaration.Route)
			}
			if declaration.Category != categoryKeysetNotFollowable {
				t.Errorf("category = %q, want %q", declaration.Category, categoryKeysetNotFollowable)
			}
			if len(declaration.Reason) < 80 {
				t.Errorf("reason = %q, want an argument rather than a label", declaration.Reason)
			}
			if !keysetReasonSite.MatchString(declaration.Reason) {
				t.Errorf("reason = %q, want it to cite the handler it read by file and line", declaration.Reason)
			}
			if seen[declaration.Route] {
				t.Errorf("%s is declared twice", declaration.Route)
			}
			seen[declaration.Route] = true
		})
	}
	if !slices.IsSortedFunc(declaredKeysetRoutes, func(a, b keysetRouteDeclaration) int { return strings.Compare(a.Route, b.Route) }) {
		t.Error("the declarations are not in route order, which is the order a reader looks one up in")
	}
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
		// A finding no declaration answers keeps no declaration alive.
		{Package: toolsDir + "/widgets", Action: "widget.list"},
	}}

	stale := check.staleDeclarations()

	if len(stale) != 1 {
		t.Fatalf("staleDeclarations() = %v, want the one claim that no longer holds", stale)
	}
	if !strings.Contains(stale[0], "retired.list") {
		t.Errorf("staleDeclarations() = %q, want it to name the retired declaration", stale[0])
	}
}

// TestPaginationStaleDeclarations_AnUndeclaredFinding_UsesNoDeclaration
// verifies that a finding no declaration answers keeps no declaration alive: a
// table whose only entry names another action is stale beside it.
func TestPaginationStaleDeclarations_AnUndeclaredFinding_UsesNoDeclaration(t *testing.T) {
	withPaginationDeclarations(t, []paginationDeclaration{
		{Package: toolsDir + "/issues", Action: "issue.participants", Category: categoryEndpointNotPaginated, Reason: "sent whole"},
	})
	check := PaginationCheck{Ran: true, Unpaginated: []UnpaginatedCollection{
		{Package: toolsDir + "/issues", Action: "issue.list_links"},
	}}

	stale := check.staleDeclarations()

	if len(stale) != 1 || !strings.Contains(stale[0], "issue.participants") {
		t.Errorf("staleDeclarations() = %v, want the one declaration the finding does not use", stale)
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
