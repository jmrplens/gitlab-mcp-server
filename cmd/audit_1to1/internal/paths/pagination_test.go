// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package paths

import (
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The fixture output types below stand for the shapes this repository really
// publishes, spelled small enough that a reader of a case can see the whole of
// what is being judged.

// fixtureItem is one object of a list, since a list of objects is what a page of
// GitLab's answer holds.
type fixtureItem struct {
	ID int64 `json:"id"`
}

// fixtureListOutput is a collection envelope with no pagination: exactly the
// shape internal/tools/impersonationtokens publishes, which six dimensions of
// this audit were green on.
type fixtureListOutput struct {
	toolutil.HintableOutput
	Tokens []fixtureItem `json:"tokens"`
}

// fixturePaginatedListOutput is the same envelope saying where its page sits.
type fixturePaginatedListOutput struct {
	toolutil.HintableOutput
	Tokens     []fixtureItem             `json:"tokens"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// fixtureObjectOutput is one object that happens to carry a list under it. Its
// caller asked for the object, so it is not a collection read and admitting it
// is what turned this rule's finding list from 24 into 79. Every other shape
// the reading tells apart is pinned where the reading lives, in
// internal/toolutil's collection_shape_test.go.
type fixtureObjectOutput struct {
	toolutil.HintableOutput
	Name   string        `json:"name"`
	Labels []fixtureItem `json:"labels"`
}

// TestTypeName_FollowsPointers_ToTheTypeAFindingNames verifies that a finding
// names the type a reader edits, whether the route declared it by value or
// through a pointer to it.
func TestTypeName_FollowsPointers_ToTheTypeAFindingNames(t *testing.T) {
	cases := []struct {
		name       string
		outputType reflect.Type
	}{
		{name: "a value type", outputType: reflect.TypeFor[fixtureListOutput]()},
		{name: "a pointer to it", outputType: reflect.TypeFor[*fixtureListOutput]()},
		{name: "a pointer to a pointer to it", outputType: reflect.TypeFor[**fixtureListOutput]()},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := typeName(testCase.outputType); got != "fixtureListOutput" {
				t.Errorf("typeName(%v) = %q, want %q", testCase.outputType, got, "fixtureListOutput")
			}
		})
	}
}

// TestRoutePagination_ParamsSayOffsetAndTheRecordSaysKeyset verifies the
// oracle's own reading. Offset is what the route declares, page beside
// per_page. Keyset is the record's keyset field and never the params: GitLab
// reads pagination=keyset without declaring it, so most of the routes it can
// page by keyset declare no cursor, and a route declaring one with no keyset
// site is one the walk would have refused to write. The two are independent,
// since a keyset route still pages by offset unless a caller asks otherwise.
func TestRoutePagination_ParamsSayOffsetAndTheRecordSaysKeyset(t *testing.T) {
	sites := []apilive.KeysetSite{fixtureKeysetSite("cursor")}
	cases := []struct {
		name       string
		params     []string
		keyset     []apilive.KeysetSite
		wantOffset bool
		wantKeyset bool
	}{
		{name: "page and per_page is offset pagination", params: []string{"id", "page", "per_page"}, wantOffset: true},
		{name: "page without per_page is not", params: []string{"id", "page"}},
		{name: "per_page without page is not", params: []string{"id", "per_page"}},
		{name: "a declared cursor alone is not keyset pagination", params: []string{"id", "per_page", "cursor"}},
		{name: "a keyset site is keyset pagination", params: []string{"id"}, keyset: sites, wantKeyset: true},
		{name: "a keyset site beside page and per_page is both", params: []string{"page", "per_page"}, keyset: sites, wantOffset: true, wantKeyset: true},
		{name: "no per_page is an endpoint that sends everything", params: []string{"id", "sort"}},
		{name: "a route declaring nothing at all", params: nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			route := apilive.Route{Method: "GET", Path: "/api/:version/things", Keyset: testCase.keyset}
			for _, name := range testCase.params {
				if route.Params == nil {
					route.Params = map[string]apilive.Param{}
				}
				route.Params[name] = apilive.Param{}
			}

			offset, keyset := routePagination(route)

			if offset != testCase.wantOffset || keyset != testCase.wantKeyset {
				t.Errorf("routePagination() = offset %t, keyset %t; want %t and %t",
					offset, keyset, testCase.wantOffset, testCase.wantKeyset)
			}
		})
	}
}

// TestPaginatedEndpoints_JoinsRecordedRequestsToTheRoute verifies the half of
// the join that reads the inventory: which endpoints a package was recorded
// calling that GitLab pages, once each, with the rows that say nothing left out.
func TestPaginatedEndpoints_JoinsRecordedRequestsToTheRoute(t *testing.T) {
	index, _ := fixtureRecord(map[string]response{
		"GET /groups/:id/ssh_certificates": {Params: []string{"id", "page", "per_page"}},
		"GET /groups/:id/saml_group_links": {},
		"GET /projects/:id/-/work_items/:w": {
			Params: []string{"per_page", "cursor"}, Keyset: []apilive.KeysetSite{fixtureKeysetSite("cursor")},
		},
	})
	rows := []requestinventory.Row{
		{Package: "internal/tools/groupsshcerts", Kind: requestinventory.KindREST, Method: "GET", Path: "/groups/:group_id/ssh_certificates"},
		{Package: "internal/tools/groupsshcerts", Kind: requestinventory.KindREST, Method: "GET", Path: "/groups/:group_id/ssh_certificates"},
		{Package: "internal/tools/groupsaml", Kind: requestinventory.KindREST, Method: "GET", Path: "/groups/:group_id/saml_group_links"},
		{Package: "internal/tools/workitems", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:project_id/-/work_items/:work_item_id"},
		{Package: "internal/tools/epics", Kind: requestinventory.KindGraphQL, Operation: "epicNotes"},
		{Package: "internal/tools/ghost", Kind: requestinventory.KindREST, Method: "GET", Path: "/nothing/gitlab/has"},
	}

	offset, keyset := paginatedEndpoints(index, rows)

	t.Run("a paginated endpoint is recorded once for its package", func(t *testing.T) {
		got := offset["internal/tools/groupsshcerts"]
		if len(got) != 1 || got[0] != "GET /groups/:group_id/ssh_certificates" {
			t.Errorf("offset endpoints = %v, want the one endpoint listed once", got)
		}
	})
	t.Run("an endpoint GitLab does not page is not recorded", func(t *testing.T) {
		if got, ok := offset["internal/tools/groupsaml"]; ok {
			t.Errorf("offset endpoints = %v, want the package absent", got)
		}
	})
	t.Run("a cursor endpoint is kept apart from the offset ones", func(t *testing.T) {
		if got := keyset["internal/tools/workitems"]; len(got) != 1 {
			t.Errorf("keyset endpoints = %v, want the one cursor endpoint", got)
		}
		if got, ok := offset["internal/tools/workitems"]; ok {
			t.Errorf("offset endpoints = %v, want a cursor endpoint counted as no offset one", got)
		}
	})
	t.Run("a GraphQL row names no REST endpoint", func(t *testing.T) {
		if _, ok := offset["internal/tools/epics"]; ok {
			t.Error("a GraphQL row was read as a REST endpoint")
		}
	})
	t.Run("a row the record cannot resolve says nothing", func(t *testing.T) {
		if _, ok := offset["internal/tools/ghost"]; ok {
			t.Error("an endpoint the record does not hold was read as paginated")
		}
	})
}

// paginationFixture is the three inputs of one end-to-end case, named so a case
// reads as the situation it describes rather than as three literals.
type paginationFixture struct {
	operations map[string]response
	rows       []requestinventory.Row
	actions    []requestinventory.Action
}

// listAction builds one catalog action reading a collection, since every case
// below differs only in its output type and its owner.
func listAction(id, owner string, outputType reflect.Type, input map[string]any) requestinventory.Action {
	return requestinventory.Action{
		ID:       id,
		Owner:    owner,
		ReadOnly: true,
		Route:    toolutil.ActionRoute{OutputType: outputType, InputSchema: input},
	}
}

// TestPaginationCheck_AListWithNoPaginationAndAPaginatedEndpoint_IsAFinding
// verifies the whole rule on the shape it was built for: an action publishing a
// bare array while GitLab serves that endpoint one page at a time, which is the
// state internal/tools/impersonationtokens shipped in with six dimensions green.
func TestPaginationCheck_AListWithNoPaginationAndAPaginatedEndpoint_IsAFinding(t *testing.T) {
	withPaginationDeclarations(t, nil)
	fixture := paginationFixture{
		operations: map[string]response{
			"GET /users/:id/impersonation_tokens": {Params: []string{"id", "page", "per_page"}},
		},
		rows: []requestinventory.Row{{
			Package: "internal/tools/impersonationtokens", Kind: requestinventory.KindREST,
			Method: "GET", Path: "/users/:user_id/impersonation_tokens",
		}},
		actions: []requestinventory.Action{
			listAction("user.list_impersonation_tokens", "impersonationtokens",
				reflect.TypeFor[fixtureListOutput](),
				map[string]any{"properties": map[string]any{"page": map[string]any{}, "per_page": map[string]any{}}}),
		},
	}

	check := paginationCheck(recordIn(t, fixture.operations), fixture.rows, fixture.actions)

	if !check.Ran {
		t.Fatal("paginationCheck() did not run against a record it should have read")
	}
	if len(check.Unpaginated) != 1 {
		t.Fatalf("findings = %+v, want the one action publishing no pagination", check.Unpaginated)
	}
	finding := check.Unpaginated[0]
	t.Run("the finding names the action, its type and the list", func(t *testing.T) {
		if finding.Action != "user.list_impersonation_tokens" || finding.Type != "fixtureListOutput" || finding.Collection != "tokens" {
			t.Errorf("finding = %+v, want it to name the action, the output type and the list", finding)
		}
	})
	t.Run("the finding carries the endpoint as its evidence", func(t *testing.T) {
		if len(finding.Endpoints) != 1 || finding.Endpoints[0] != "GET /users/:user_id/impersonation_tokens" {
			t.Errorf("endpoints = %v, want the recorded paginated endpoint", finding.Endpoints)
		}
	})
	t.Run("the finding says whether the request already pages", func(t *testing.T) {
		if !finding.RequestPaginates {
			t.Error("request_paginates = false, want an action offering page and per_page to say so")
		}
	})
	t.Run("the counts divide the same way the findings do", func(t *testing.T) {
		want := CollectionCounts{Actions: 1, Unpaginated: 1, Undeclared: 1}
		if check.Collections != want {
			t.Errorf("collections = %+v, want %+v", check.Collections, want)
		}
	})
	t.Run("the grain is stated on the check itself", func(t *testing.T) {
		if check.Grain != paginationGrain {
			t.Errorf("grain = %q, want the package-grain caveat", check.Grain)
		}
	})
}

// TestPaginationCheck_ShapesThatAreNotFindings_AreCountedApart verifies the
// three ways an action reading a collection is not a finding, since a rule that
// could not tell them apart would report 244 of the 268 collection actions and
// be read by nobody.
func TestPaginationCheck_ShapesThatAreNotFindings_AreCountedApart(t *testing.T) {
	withPaginationDeclarations(t, nil)
	operations := map[string]response{
		"GET /projects/:id/things": {Params: []string{"id", "page", "per_page"}},
		"GET /projects/:id/whole":  {Params: []string{"id"}},
		"GET /projects/:id/cursors": {
			Params: []string{"id", "per_page", "cursor"}, Keyset: []apilive.KeysetSite{fixtureKeysetSite("cursor")},
		},
	}
	rows := []requestinventory.Row{
		{Package: "internal/tools/paged", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:project_id/things"},
		{Package: "internal/tools/whole", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:project_id/whole"},
		{Package: "internal/tools/cursor", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:project_id/cursors"},
	}
	actions := []requestinventory.Action{
		listAction("paged.publishing", "paged", reflect.TypeFor[fixturePaginatedListOutput](), nil),
		listAction("whole.list", "whole", reflect.TypeFor[fixtureListOutput](), nil),
		listAction("cursor.list", "cursor", reflect.TypeFor[fixtureListOutput](), nil),
		listAction("paged.detail", "paged", reflect.TypeFor[fixtureObjectOutput](), nil),
		{
			ID: "paged.create", Owner: "paged", ReadOnly: false,
			Route: toolutil.ActionRoute{OutputType: reflect.TypeFor[fixtureListOutput]()},
		},
	}

	check := paginationCheck(recordIn(t, operations), rows, actions)

	t.Run("an action that publishes pagination is not a finding", func(t *testing.T) {
		if check.Collections.Paginated != 1 {
			t.Errorf("publishing_pagination = %d, want 1", check.Collections.Paginated)
		}
	})
	t.Run("a package calling no paginated endpoint is not asked about", func(t *testing.T) {
		if check.Collections.Unasked != 2 {
			t.Errorf("not_asked_about = %d, want the whole-response and the cursor-only package", check.Collections.Unasked)
		}
	})
	t.Run("a cursor endpoint raises no offset finding", func(t *testing.T) {
		for _, finding := range check.Unpaginated {
			if finding.Package == "internal/tools/cursor" {
				t.Errorf("finding = %+v, want a keyset endpoint to raise none", finding)
			}
		}
		if check.Recorded.KeysetPackages != 1 || check.Recorded.KeysetEndpoints != 1 {
			t.Errorf("recorded = %+v, want the one cursor endpoint counted apart", check.Recorded)
		}
	})
	t.Run("an object carrying a list is no collection and a mutation is not asked", func(t *testing.T) {
		if check.Collections.Actions != 3 {
			t.Errorf("collection actions = %d, want the three read-only envelopes", check.Collections.Actions)
		}
	})
	t.Run("nothing is reported", func(t *testing.T) {
		if len(check.Unpaginated) != 0 {
			t.Errorf("findings = %+v, want none", check.Unpaginated)
		}
	})
}

// TestRequestPaginates_EitherParameterAlone_SaysTheRequestPages verifies what
// tells a finding that needs one output field from one that needs an input
// parameter first.
//
// Either name alone is enough, because they are asked about separately: an
// action offering per_page and no page can already ask for a longer answer,
// and one offering page and no per_page can already ask for the next of
// GitLab's own. Demanding both would report an action that pages as one that
// cannot, which points the fix at the wrong file.
func TestRequestPaginates_EitherParameterAlone_SaysTheRequestPages(t *testing.T) {
	schemaWith := func(names ...string) map[string]any {
		properties := map[string]any{}
		for _, name := range names {
			properties[name] = map[string]any{}
		}
		return map[string]any{"properties": properties}
	}
	cases := []struct {
		name   string
		schema map[string]any
		want   bool
	}{
		{name: "page and per_page", schema: schemaWith("id", "page", "per_page"), want: true},
		{name: "per_page alone", schema: schemaWith("id", "per_page"), want: true},
		{name: "page alone", schema: schemaWith("id", "page"), want: true},
		{name: "neither", schema: schemaWith("id", "sort")},
		{name: "no properties block at all", schema: map[string]any{"type": "object"}},
		{name: "no schema at all", schema: nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := requestPaginates(testCase.schema); got != testCase.want {
				t.Errorf("requestPaginates() = %t, want %t", got, testCase.want)
			}
		})
	}
}

// TestPaginationCheck_Findings_AreOrderedByPackageThenAction verifies the order
// a reader meets the findings in.
//
// The list is read as work rather than as data, so the two actions of one
// package have to arrive together and in a fixed order, and the package is what
// decides first: a reader opening one file wants everything this rule has to
// say about it before moving on. The fixture names its packages and its actions
// so the two orders disagree, since a list ordered by either alone would
// otherwise read as correct.
func TestPaginationCheck_Findings_AreOrderedByPackageThenAction(t *testing.T) {
	withPaginationDeclarations(t, nil)
	operations := map[string]response{
		"GET /projects/:id/alpha": {Params: []string{"id", "page", "per_page"}},
		"GET /projects/:id/bravo": {Params: []string{"id", "page", "per_page"}},
	}
	rows := []requestinventory.Row{
		{Package: "internal/tools/alpha", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:project_id/alpha"},
		{Package: "internal/tools/bravo", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:project_id/bravo"},
	}
	actions := []requestinventory.Action{
		listAction("zebra.list", "alpha", reflect.TypeFor[fixtureListOutput](), nil),
		listAction("apple.list", "bravo", reflect.TypeFor[fixtureListOutput](), nil),
		listAction("alpha.list", "alpha", reflect.TypeFor[fixtureListOutput](), nil),
	}

	check := paginationCheck(recordIn(t, operations), rows, actions)

	got := make([]string, 0, len(check.Unpaginated))
	for _, finding := range check.Unpaginated {
		got = append(got, finding.Package+" "+finding.Action)
	}
	want := []string{
		"internal/tools/alpha alpha.list",
		"internal/tools/alpha zebra.list",
		"internal/tools/bravo apple.list",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
}

// TestPaginationCheck_TheRecordCountsItsOwnRoutes verifies the oracle's size is
// published, because a finding that cites an endpoint means nothing to a reader
// who cannot see how much of GitLab the record held.
//
// The keyset count is the record's keyset field: a route whose code reaches a
// keyset paginator counts whatever it declares, the same route counts as
// offset as well when it declares page and per_page, and a route declaring a
// cursor and reaching no paginator is not keyset at all, which is how the
// count used to be read off the params.
func TestPaginationCheck_TheRecordCountsItsOwnRoutes(t *testing.T) {
	withPaginationDeclarations(t, nil)
	sites := []apilive.KeysetSite{fixtureKeysetSite("cursor")}
	operations := map[string]response{
		"GET /a": {Params: []string{"page", "per_page"}},
		"GET /b": {Params: []string{"per_page", "cursor"}},
		"GET /c": {},
		"GET /d": {Params: []string{"page", "per_page"}, Keyset: sites},
		"GET /e": {Keyset: sites},
	}

	check := paginationCheck(recordIn(t, operations), nil, nil)

	want := RoutePagination{Routes: 5, Offset: 2, Keyset: 2}
	if check.Routes != want {
		t.Errorf("routes = %+v, want %+v", check.Routes, want)
	}
}

// TestPaginationCheck_NoRecord_DoesNotRun verifies the one way this check is
// skipped. It says so itself rather than inheriting the shape check's verdict,
// so a reader of an empty finding list can tell "nothing found" from "nothing
// asked".
func TestPaginationCheck_NoRecord_DoesNotRun(t *testing.T) {
	check := paginationCheck(t.TempDir(), nil, nil)

	if check.Ran {
		t.Error("paginationCheck() ran without a record to read")
	}
	if check.Grain != "" || len(check.Unpaginated) != 0 {
		t.Errorf("check = %+v, want the zero value when it did not run", check)
	}
}

// TestKeepUndeclaredCollections_DropsWhatADeclarationAnswers verifies what
// -gaps-only asks for, on the same terms the silent owners and the undocumented
// endpoints are filtered: a declared finding is context rather than work.
func TestKeepUndeclaredCollections_DropsWhatADeclarationAnswers(t *testing.T) {
	kept := keepUndeclaredCollections([]UnpaginatedCollection{
		{Action: "a.list", Category: categoryEndpointNotPaginated, Reason: "GitLab sends it whole"},
		{Action: "b.list"},
	})

	if len(kept) != 1 || kept[0].Action != "b.list" {
		t.Errorf("keepUndeclaredCollections() = %+v, want only the undeclared finding", kept)
	}
}
