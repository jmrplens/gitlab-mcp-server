// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package paths

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The cases below read the keyset half the way a reader of the report does,
// as JSON under pagination.keyset, so each one says what a reader is told and
// under which name.

// keysetFindingJSON is one finding as the report spells it.
type keysetFindingJSON struct {
	Action        string   `json:"action"`
	Package       string   `json:"package"`
	Problem       string   `json:"problem"`
	Routes        []string `json:"routes"`
	Cursors       []string `json:"cursors"`
	NotFollowable []string `json:"not_followable"`
	Category      string   `json:"category"`
	Reason        string   `json:"reason"`
}

// keysetHalfJSON is the keyset half as the report spells it.
type keysetHalfJSON struct {
	Ran        bool                `json:"ran"`
	Requests   string              `json:"requests"`
	Grain      string              `json:"grain"`
	Offering   int                 `json:"actions_offering_page_token"`
	OnKeyset   int                 `json:"actions_offering_it_on_keyset_routes"`
	Unrecorded []string            `json:"unrecorded_actions"`
	Findings   []keysetFindingJSON `json:"findings"`
	Unused     []string            `json:"unused_declarations"`
}

// keysetFixture is the four inputs one case of the keyset half reads: the live
// record, the request record the handlers were derived into, the inventory and
// the catalog.
type keysetFixture struct {
	operations map[string]response
	requests   []actionrequests.RecordAction
	rows       []requestinventory.Row
	actions    []requestinventory.Action
}

// run writes both records into a root, runs the whole pagination check and
// returns its keyset half as the report renders it. The request record is
// written the way the always-sent cases write it.
func (f keysetFixture) run(t *testing.T) keysetHalfJSON {
	t.Helper()
	root := recordIn(t, f.operations)
	writeActionRequests(t, root, f.requests)
	return keysetHalfOf(t, paginationCheck(root, f.rows, f.actions))
}

// keysetHalfOf renders a check the way the report does and reads its keyset
// half back.
func keysetHalfOf(t *testing.T, check PaginationCheck) keysetHalfJSON {
	t.Helper()
	content, err := json.Marshal(check)
	if err != nil {
		t.Fatalf("marshal the check: %v", err)
	}
	var decoded struct {
		Keyset *keysetHalfJSON `json:"keyset"`
	}
	if decodeErr := json.Unmarshal(content, &decoded); decodeErr != nil {
		t.Fatalf("read the check back: %v", decodeErr)
	}
	if decoded.Keyset == nil {
		t.Fatalf("the pagination check carries no keyset half: %s", content)
	}
	return *decoded.Keyset
}

// sending is one action of the request record, sending the REST routes given
// from a handler in its owner's package.
func sending(id, handler string, routes ...string) actionrequests.RecordAction {
	action := actionrequests.RecordAction{ID: id, Handlers: []string{handler}, Classic: "read_api"}
	for _, route := range routes {
		action.Requests = append(action.Requests, actionrequests.RecordRequest{
			Kind: actionrequests.KindREST, Route: route, Class: actionrequests.ClassMandatory, Sites: []string{handler},
		})
	}
	return action
}

// withPageToken is an input schema offering page_token beside the offset
// parameters, which is what toolutil.KeysetPaginationInput publishes.
var withPageToken = map[string]any{"properties": map[string]any{
	"page": map[string]any{}, "per_page": map[string]any{}, "pagination": map[string]any{}, "page_token": map[string]any{},
}}

// withoutPageToken is an input schema offering the offset parameters alone.
var withoutPageToken = map[string]any{"properties": map[string]any{"page": map[string]any{}, "per_page": map[string]any{}}}

// fixtureGraphQLListOutput is a list paged by a GraphQL connection, whose
// block carries a cursor of GraphQL's and no next_page_token.
type fixtureGraphQLListOutput struct {
	toolutil.HintableOutput
	Tokens     []fixtureItem                    `json:"tokens"`
	Pagination toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// cursorRoute is a route whose code reaches a keyset paginator asked by
// cursor, and that pages by offset as well, which is how most of GitLab's
// keyset routes are recorded.
var cursorRoute = response{
	Params: []string{"id", "page", "per_page"}, Keyset: []apilive.KeysetSite{fixtureKeysetSite("cursor")},
}

// followed is a recorded request of a package to the cursor route that
// carried its cursor, which is what a test following a keyset page records.
func followed(pkg string, query ...string) requestinventory.Row {
	return requestinventory.Row{
		Package: pkg, Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:project_id/cursors", Query: query,
	}
}

// TestPaginationCheck_PageTokenWhereNoRouteIsKeyset_IsAFinding verifies the
// question issue 1165 left for this rule: page_token is offered only where
// GitLab can page by keyset. An action offering it on a route that pages by
// offset alone is offering a way through the list GitLab ignores, and so is
// one that sends no REST route at all; an action offering only page and
// per_page there is asked nothing.
func TestPaginationCheck_PageTokenWhereNoRouteIsKeyset_IsAFinding(t *testing.T) {
	withPaginationDeclarations(t, nil)
	half := keysetFixture{
		operations: map[string]response{"GET /projects/:id/things": {Params: []string{"id", "page", "per_page"}}},
		requests: []actionrequests.RecordAction{
			sending("thing.list", "things.List", "GET /projects/:id/things"),
			{ID: "thing.graphql", Handlers: []string{"things.Graph"}, Classic: "read_api", Requests: []actionrequests.RecordRequest{
				{Kind: actionrequests.KindGraphQL, Operation: "things", Class: actionrequests.ClassMandatory},
			}},
			// A GraphQL document before the REST route, which is read past
			// rather than taken as the end of what the action sends.
			{ID: "thing.mixed", Handlers: []string{"things.Mixed"}, Classic: "read_api", Requests: []actionrequests.RecordRequest{
				{Kind: actionrequests.KindGraphQL, Operation: "things", Class: actionrequests.ClassAlternative},
				{Kind: actionrequests.KindREST, Route: "GET /projects/:id/things", Class: actionrequests.ClassAlternative},
			}},
			sending("thing.offset_list", "things.OffsetList", "GET /projects/:id/things"),
		},
		actions: []requestinventory.Action{
			listAction("thing.list", "things", reflect.TypeFor[fixturePaginatedListOutput](), withPageToken),
			listAction("thing.graphql", "things", reflect.TypeFor[fixturePaginatedListOutput](), withPageToken),
			listAction("thing.mixed", "things", reflect.TypeFor[fixturePaginatedListOutput](), withPageToken),
			listAction("thing.offset_list", "things", reflect.TypeFor[fixturePaginatedListOutput](), withoutPageToken),
		},
	}.run(t)

	want := []keysetFindingJSON{
		{Action: "thing.graphql", Package: "internal/tools/things", Problem: "page-token-on-offset-route"},
		{
			Action: "thing.list", Package: "internal/tools/things", Problem: "page-token-on-offset-route",
			Routes: []string{"GET /projects/:id/things"},
		},
		{
			Action: "thing.mixed", Package: "internal/tools/things", Problem: "page-token-on-offset-route",
			Routes: []string{"GET /projects/:id/things"},
		},
	}
	if !reflect.DeepEqual(half.Findings, want) {
		t.Errorf("findings = %+v, want %+v", half.Findings, want)
	}
	t.Run("the half says what it read and at which grain", func(t *testing.T) {
		if !half.Ran || half.Requests != actionrequests.RecordPath || half.Grain == "" {
			t.Errorf("half = %+v, want it run, naming the request record and its grain", half)
		}
	})
	t.Run("the actions offering page_token are counted, none of them on a keyset route", func(t *testing.T) {
		if half.Offering != 3 || half.OnKeyset != 0 {
			t.Errorf("offering = %d, on keyset routes = %d; want 3 and 0", half.Offering, half.OnKeyset)
		}
	})
}

// TestPaginationCheck_AKeysetRouteFollowedAndPublished_RaisesNothing verifies
// the shape every keyset list should have, which is the one this rule passes:
// page_token offered on a route GitLab can page by keyset, the next_page_token
// block published, and a test of the package that serves it recorded sending
// the route its cursor. An offset route the same action sends beside it, and
// inventory rows that say nothing, change none of that.
func TestPaginationCheck_AKeysetRouteFollowedAndPublished_RaisesNothing(t *testing.T) {
	withPaginationDeclarations(t, nil)
	half := keysetFixture{
		operations: map[string]response{
			"GET /projects/:id":         {Params: []string{"id"}},
			"GET /projects/:id/cursors": cursorRoute,
		},
		requests: []actionrequests.RecordAction{
			sending("thing.list", "things.List", "GET /projects/:id", "GET /projects/:id/cursors"),
		},
		rows: []requestinventory.Row{
			{Package: "internal/tools/things", Kind: requestinventory.KindGraphQL, Operation: "things", Query: []string{"cursor"}},
			{Package: "internal/tools/things", Kind: requestinventory.KindREST, Method: "GET", Path: "/nothing/gitlab/has", Query: []string{"cursor"}},
			followed("internal/tools/things", "cursor", "pagination", "per_page"),
		},
		actions: []requestinventory.Action{
			listAction("thing.list", "things", reflect.TypeFor[fixturePaginatedListOutput](), withPageToken),
		},
	}.run(t)

	if len(half.Findings) != 0 {
		t.Errorf("findings = %+v, want none", half.Findings)
	}
	if half.Offering != 1 || half.OnKeyset != 1 {
		t.Errorf("offering = %d, on keyset routes = %d; want 1 and 1", half.Offering, half.OnKeyset)
	}
}

// TestPaginationCheck_AKeysetRouteWithoutItsNextPageToken_IsAFinding verifies
// the question issue 1165 was opened about: a caller asking for a keyset page
// is told how to ask for the next one only through next_page_token, so an
// action offering page_token on a keyset route whose output carries no block,
// or a GraphQL connection's block, which has no next_page_token, is a finding
// naming the route and what its next page is asked by.
func TestPaginationCheck_AKeysetRouteWithoutItsNextPageToken_IsAFinding(t *testing.T) {
	withPaginationDeclarations(t, nil)
	for _, testCase := range []struct {
		name   string
		output reflect.Type
	}{
		{name: "no pagination block at all", output: reflect.TypeFor[fixtureListOutput]()},
		{name: "a GraphQL connection's block", output: reflect.TypeFor[fixtureGraphQLListOutput]()},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			half := keysetFixture{
				operations: map[string]response{"GET /projects/:id/cursors": cursorRoute},
				requests:   []actionrequests.RecordAction{sending("thing.list", "things.List", "GET /projects/:id/cursors")},
				rows:       []requestinventory.Row{followed("internal/tools/things", "cursor")},
				actions:    []requestinventory.Action{listAction("thing.list", "things", testCase.output, withPageToken)},
			}.run(t)

			want := []keysetFindingJSON{{
				Action: "thing.list", Package: "internal/tools/things", Problem: "keyset-route-without-next-page-token",
				Routes: []string{"GET /projects/:id/cursors"}, Cursors: []string{"cursor"},
			}}
			if !reflect.DeepEqual(half.Findings, want) {
				t.Errorf("findings = %+v, want %+v", half.Findings, want)
			}
		})
	}
}

// TestPaginationCheck_AKeysetRouteNoTestFollowed_IsAFinding verifies the half
// of the question only the inventory can answer: a keyset page is followed by
// sending its route the parameter the next page is asked by, and a route no
// test of a package serving the action was recorded sending one of those has
// never been followed, whatever the action publishes.
//
// The inventory files a request under the package whose test sent it, so a
// handler's own package counts beside the owner, and another package's
// request does not; a route asked by a pair is followed by either half of it.
func TestPaginationCheck_AKeysetRouteNoTestFollowed_IsAFinding(t *testing.T) {
	withPaginationDeclarations(t, nil)
	never := []keysetFindingJSON{{
		Action: "thing.list", Package: "internal/tools/things", Problem: "keyset-route-never-followed",
		Routes: []string{"GET /projects/:id/cursors"}, Cursors: []string{"cursor"},
	}}
	pairRoute := response{Keyset: []apilive.KeysetSite{fixtureKeysetSite("id_after", "id_before")}}
	for _, testCase := range []struct {
		name  string
		route response
		rows  []requestinventory.Row
		want  []keysetFindingJSON
	}{
		{name: "no recorded request at all", route: cursorRoute, want: never},
		{
			name: "the owner's request without the cursor", route: cursorRoute,
			rows: []requestinventory.Row{followed("internal/tools/things", "pagination", "per_page")}, want: never,
		},
		{
			name: "another package's request with it", route: cursorRoute,
			rows: []requestinventory.Row{followed("internal/tools/widgets", "cursor")}, want: never,
		},
		{
			name: "a request of the package the handler lives in", route: cursorRoute,
			rows: []requestinventory.Row{followed("internal/tools/thinghandlers", "cursor")},
		},
		{
			name: "one half of a pair", route: pairRoute,
			rows: []requestinventory.Row{followed("internal/tools/things", "id_after")},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			half := keysetFixture{
				operations: map[string]response{"GET /projects/:id/cursors": testCase.route},
				requests: []actionrequests.RecordAction{
					sending("thing.list", "thinghandlers.List", "GET /projects/:id/cursors"),
				},
				rows: testCase.rows,
				actions: []requestinventory.Action{
					listAction("thing.list", "things", reflect.TypeFor[fixturePaginatedListOutput](), withPageToken),
				},
			}.run(t)

			if !reflect.DeepEqual(half.Findings, testCase.want) {
				t.Errorf("findings = %+v, want %+v", half.Findings, testCase.want)
			}
		})
	}
}

// TestPaginationCheck_AListOnAKeysetRouteWithoutPageToken_IsAFinding verifies
// the converse: a list sending a route GitLab can page by keyset that does
// not offer page_token cannot ask for the cheaper of GitLab's two ways
// through it, which is a gap in the 1:1 surface. Only a read whose output is
// a list is asked: an action reading one object off such a route, or writing,
// reads no list a caller could page through.
func TestPaginationCheck_AListOnAKeysetRouteWithoutPageToken_IsAFinding(t *testing.T) {
	withPaginationDeclarations(t, nil)
	half := keysetFixture{
		operations: map[string]response{
			"GET /projects/:id":         {Params: []string{"id"}},
			"GET /projects/:id/cursors": cursorRoute,
		},
		requests: []actionrequests.RecordAction{
			sending("thing.list", "things.List", "GET /projects/:id", "GET /projects/:id/cursors"),
			sending("thing.find", "things.Find", "GET /projects/:id/cursors"),
			sending("thing.prune", "things.Prune", "GET /projects/:id/cursors"),
			sending("thing.get", "things.Get", "GET /projects/:id"),
		},
		actions: []requestinventory.Action{
			listAction("thing.list", "things", reflect.TypeFor[fixturePaginatedListOutput](), withoutPageToken),
			listAction("thing.find", "things", reflect.TypeFor[fixtureObjectOutput](), withoutPageToken),
			{
				ID: "thing.prune", Owner: "things", ReadOnly: false,
				Route: toolutil.ActionRoute{OutputType: reflect.TypeFor[fixturePaginatedListOutput](), InputSchema: withoutPageToken},
			},
			// No input schema at all, as a route registered without one
			// carries: it offers nothing.
			listAction("thing.get", "things", reflect.TypeFor[fixturePaginatedListOutput](), nil),
		},
	}.run(t)

	want := []keysetFindingJSON{{
		Action: "thing.list", Package: "internal/tools/things", Problem: "keyset-route-without-page-token",
		Routes: []string{"GET /projects/:id/cursors"}, Cursors: []string{"cursor"},
	}}
	if !reflect.DeepEqual(half.Findings, want) {
		t.Errorf("findings = %+v, want %+v", half.Findings, want)
	}
	if half.Offering != 0 || half.OnKeyset != 0 {
		t.Errorf("offering = %d, on keyset routes = %d; want neither counted for actions that offer no page_token",
			half.Offering, half.OnKeyset)
	}
}

// TestPaginationCheck_ARouteMountedTwice_IsAskedByTheCursorsOfBoth verifies a
// route the record mounts twice is one route to the request record, which
// spells it once, and its next page is asked by what either mount names: the
// record mounts GET /pipelines and the project tag list twice at 19.5.0.
func TestPaginationCheck_ARouteMountedTwice_IsAskedByTheCursorsOfBoth(t *testing.T) {
	withPaginationDeclarations(t, nil)
	doc := fixtureDocument(map[string]response{
		"GET /projects/:id":         {Params: []string{"id"}},
		"GET /projects/:id/cursors": cursorRoute,
	})
	doc.Routes = append(doc.Routes, apilive.Route{
		Method: "GET", Path: apilive.EndpointPrefix + "/projects/:id/cursors",
		Keyset: []apilive.KeysetSite{fixtureKeysetSite("page_token")},
	})
	root := t.TempDir()
	if err := apilive.Write(filepath.Join(root, apilive.DefaultDir), doc); err != nil {
		t.Fatalf("prepare the record: %v", err)
	}
	writeActionRequests(t, root, []actionrequests.RecordAction{sending("thing.list", "things.List", "GET /projects/:id/cursors")})

	half := keysetHalfOf(t, paginationCheck(root, nil, []requestinventory.Action{
		listAction("thing.list", "things", reflect.TypeFor[fixturePaginatedListOutput](), withPageToken),
	}))

	want := []keysetFindingJSON{{
		Action: "thing.list", Package: "internal/tools/things", Problem: "keyset-route-never-followed",
		Routes: []string{"GET /projects/:id/cursors"}, Cursors: []string{"cursor", "page_token"},
	}}
	if !reflect.DeepEqual(half.Findings, want) {
		t.Errorf("findings = %+v, want %+v", half.Findings, want)
	}
}

// TestPaginationCheck_AnActionTheRequestRecordDoesNotHold_IsListedApart
// verifies an action nothing here can judge is said rather than passed: an
// action offering page_token that the request record does not hold, which a
// record older than the catalog produces, is listed by name, and one that
// offers nothing is not, since there is no question to leave unanswered.
func TestPaginationCheck_AnActionTheRequestRecordDoesNotHold_IsListedApart(t *testing.T) {
	withPaginationDeclarations(t, nil)
	half := keysetFixture{
		operations: map[string]response{"GET /projects/:id/things": {Params: []string{"id", "page", "per_page"}}},
		requests:   []actionrequests.RecordAction{sending("thing.list", "things.List", "GET /projects/:id/things")},
		actions: []requestinventory.Action{
			listAction("thing.new", "things", reflect.TypeFor[fixturePaginatedListOutput](), withPageToken),
			listAction("thing.newer", "things", reflect.TypeFor[fixturePaginatedListOutput](), withoutPageToken),
		},
	}.run(t)

	if !reflect.DeepEqual(half.Unrecorded, []string{"thing.new"}) {
		t.Errorf("unrecorded = %v, want the one action offering page_token", half.Unrecorded)
	}
	if half.Offering != 1 || len(half.Findings) != 0 {
		t.Errorf("offering = %d, findings = %+v; want it counted and judged on nothing", half.Offering, half.Findings)
	}
}

// TestPaginationCheck_NoRequestRecord_LeavesTheKeysetHalfUnrun verifies the
// one input this half reads that the collection half does not: without the
// request record nothing says which routes an action sends, so the half says
// it did not run, rather than report every action as offering page_token on
// no route, and the collection half runs as before.
func TestPaginationCheck_NoRequestRecord_LeavesTheKeysetHalfUnrun(t *testing.T) {
	withPaginationDeclarations(t, nil)
	root := recordIn(t, map[string]response{"GET /projects/:id/cursors": cursorRoute})

	check := paginationCheck(root, nil, []requestinventory.Action{
		listAction("thing.list", "things", reflect.TypeFor[fixturePaginatedListOutput](), withPageToken),
	})

	half := keysetHalfOf(t, check)
	if half.Ran || half.Offering != 0 || len(half.Findings) != 0 {
		t.Errorf("keyset half = %+v, want the zero value of a half that did not run", half)
	}
	if !check.Ran {
		t.Error("the collection half did not run, want it independent of the request record")
	}
}

// TestPaginationCheck_KeysetFindings_AreOrderedByPackageActionAndProblem
// verifies the order a reader meets the findings in, on the terms the
// collection half's order test sets: a package at a time, then an action,
// then its problems, then its routes. The fixture names its packages, actions
// and routes so that no order but that one reads as correct.
func TestPaginationCheck_KeysetFindings_AreOrderedByPackageActionAndProblem(t *testing.T) {
	withPaginationDeclarations(t, nil)
	half := keysetFixture{
		operations: map[string]response{
			"GET /projects/:id/cursors": cursorRoute,
			"GET /projects/:id/tokens":  {Keyset: []apilive.KeysetSite{fixtureKeysetSite("page_token")}},
			"GET /projects/:id/things":  {Params: []string{"id", "page", "per_page"}},
		},
		requests: []actionrequests.RecordAction{
			sending("zebra.list", "alpha.Zebra", "GET /projects/:id/things"),
			sending("apple.list", "bravo.Apple", "GET /projects/:id/tokens", "GET /projects/:id/cursors"),
			sending("alpha.list", "alpha.List", "GET /projects/:id/tokens"),
		},
		actions: []requestinventory.Action{
			listAction("zebra.list", "alpha", reflect.TypeFor[fixturePaginatedListOutput](), withPageToken),
			listAction("apple.list", "bravo", reflect.TypeFor[fixtureListOutput](), withPageToken),
			listAction("alpha.list", "alpha", reflect.TypeFor[fixturePaginatedListOutput](), withPageToken),
		},
	}.run(t)

	got := make([]string, 0, len(half.Findings))
	for _, finding := range half.Findings {
		got = append(got, finding.Package+" "+finding.Action+" "+finding.Problem+" "+finding.Routes[0])
	}
	want := []string{
		"internal/tools/alpha alpha.list keyset-route-never-followed GET /projects/:id/tokens",
		"internal/tools/alpha zebra.list page-token-on-offset-route GET /projects/:id/things",
		"internal/tools/bravo apple.list keyset-route-never-followed GET /projects/:id/cursors",
		"internal/tools/bravo apple.list keyset-route-never-followed GET /projects/:id/tokens",
		"internal/tools/bravo apple.list keyset-route-without-next-page-token GET /projects/:id/cursors",
		"internal/tools/bravo apple.list keyset-route-without-next-page-token GET /projects/:id/tokens",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
}
