// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package routes

import (
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// TestPatternMatches_EachWildcard_StandsForWhatItSays verifies a pattern
// against a path: a literal segment for itself, "*" for one segment, a last
// "**" for any tail, none included, and nothing for a path longer or shorter
// than the pattern.
func TestPatternMatches_EachWildcard_StandsForWhatItSays(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{pattern: "/pipelines", path: "/pipelines", want: true},
		{pattern: "/groups/*/placeholder_reassignments", path: "/groups/:id/placeholder_reassignments", want: true},
		{pattern: "/groups/*/placeholder_reassignments", path: "/groups/:id/placeholder_reassignments/authorize"},
		{pattern: "/groups/*/placeholder_reassignments", path: "/groups/:id"},
		{pattern: "/groups/*/epics", path: "/groups/:id/issues"},
		{pattern: "/projects/*/packages/npm/**", path: "/projects/:id/packages/npm", want: true},
		{pattern: "/projects/*/packages/npm/**", path: "/projects/:id/packages/npm/-/package/*package_name/dist-tags", want: true},
		{pattern: "/projects/*/packages/npm/**", path: "/projects/:id/packages/maven/*path"},
	}
	for _, tc := range cases {
		t.Run(tc.pattern+" "+tc.path, func(t *testing.T) {
			if got := patternMatches(tc.pattern, tc.path); got != tc.want {
				t.Errorf("patternMatches(%q, %q) = %t, want %t", tc.pattern, tc.path, got, tc.want)
			}
		})
	}
}

// TestRouteFamily_Matches_EverySpellingOfAnOptionalGroupAndTheVerbs verifies
// a row answers a route of one of its verbs, or of any verb when it names
// none, in either spelling of an optional group.
func TestRouteFamily_Matches_EverySpellingOfAnOptionalGroupAndTheVerbs(t *testing.T) {
	epics := routeFamily{Methods: []string{http.MethodGet, http.MethodPost}, Pattern: "/groups/*/-/epics"}
	anyVerb := routeFamily{Pattern: "/groups/*/epics"}
	cases := []struct {
		name         string
		family       routeFamily
		method, path string
		want         bool
	}{
		{name: "taken group", family: epics, method: http.MethodGet, path: "/groups/:id/(-/)epics", want: true},
		{name: "another verb", family: epics, method: http.MethodDelete, path: "/groups/:id/(-/)epics"},
		{name: "any verb, group left out", family: anyVerb, method: http.MethodDelete, path: "/groups/:id/(-/)epics", want: true},
		{name: "another path", family: anyVerb, method: http.MethodGet, path: "/groups/:id/issues"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.family.matches(tc.method, tc.path); got != tc.want {
				t.Errorf("%s matches %s %s = %t, want %t", tc.family.label(), tc.method, tc.path, got, tc.want)
			}
		})
	}
}

// TestFamilyNeeds_EachWayARowFails_IsNamed verifies every lack a family row
// can have is reported, and that a well-formed row of each category lacks
// nothing: a category the table defines, a reason, a pattern under the API
// prefix with "**" last, an issue on a tracked row alone, and GraphQL
// actions on a covered-by-graphql row alone, each in the catalog and sending
// GraphQL.
func TestFamilyNeeds_EachWayARowFails_IsNamed(t *testing.T) {
	j := newJudge(judgeInput{
		record: actionrequests.Record{Actions: []actionrequests.RecordAction{
			{ID: "epic.list", Requests: []actionrequests.RecordRequest{{Kind: actionrequests.KindGraphQL}}},
			{ID: "epic.rest", Requests: []actionrequests.RecordRequest{{Kind: actionrequests.KindREST, Route: "GET /groups/:id/epics"}}},
		}},
		catalog: map[string]bool{"epic.list": true, "epic.rest": true},
	})
	cases := []struct {
		name   string
		family routeFamily
		want   []string
	}{
		{name: "tracked", family: routeFamily{Pattern: "/pipelines", Category: categoryTracked, Reason: "r", Issue: 1}},
		{name: "covered by graphql", family: routeFamily{Pattern: "/groups/*/epics/**", Category: categoryCoveredByGraphQL, Reason: "r", Actions: []string{"epic.list"}}},
		{name: "client protocol", family: routeFamily{Pattern: "/packages/npm/**", Category: categoryClientProtocol, Reason: "r"}},
		{name: "unknown category", family: routeFamily{Pattern: "/a", Category: "odd", Reason: "r"}, want: []string{`category "odd" is not one the table defines`}},
		{name: "no reason", family: routeFamily{Pattern: "/a", Category: categoryClientProtocol}, want: []string{"it gives no reason"}},
		{name: "relative pattern", family: routeFamily{Pattern: "a", Category: categoryClientProtocol, Reason: "r"}, want: []string{"its pattern is not a path under /api/:version"}},
		{name: "prefixed pattern", family: routeFamily{Pattern: "/api/:version/a", Category: categoryClientProtocol, Reason: "r"}, want: []string{"its pattern is not a path under /api/:version"}},
		{name: "inner tail", family: routeFamily{Pattern: "/a/**/b", Category: categoryClientProtocol, Reason: "r"}, want: []string{"its pattern puts ** before its last segment"}},
		{
			name: "leading tail", family: routeFamily{Pattern: "**/a", Category: categoryClientProtocol, Reason: "r"},
			want: []string{"its pattern is not a path under /api/:version", "its pattern puts ** before its last segment"},
		},
		{name: "tracked without issue", family: routeFamily{Pattern: "/a", Category: categoryTracked, Reason: "r"}, want: []string{"an issue is what a tracked row names, and only a tracked row"}},
		{name: "issue not tracked", family: routeFamily{Pattern: "/a", Category: categoryClientProtocol, Reason: "r", Issue: 1}, want: []string{"an issue is what a tracked row names, and only a tracked row"}},
		{name: "graphql without actions", family: routeFamily{Pattern: "/a", Category: categoryCoveredByGraphQL, Reason: "r"}, want: []string{"actions are what a covered-by-graphql row names, and only a covered-by-graphql row"}},
		{
			name:   "actions not graphql",
			family: routeFamily{Pattern: "/a", Category: categoryTracked, Reason: "r", Issue: 1, Actions: []string{"epic.list"}},
			want:   []string{"actions are what a covered-by-graphql row names, and only a covered-by-graphql row"},
		},
		{
			name:   "an action the catalog lacks and one sending no graphql",
			family: routeFamily{Pattern: "/a", Category: categoryCoveredByGraphQL, Reason: "r", Actions: []string{"epic.gone", "epic.rest"}},
			want: []string{
				"it names epic.gone, which the catalog does not hold", "it names epic.gone, which sends no GraphQL request",
				"it names epic.rest, which sends no GraphQL request",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := j.familyNeeds(tc.family); !slices.Equal(got, tc.want) {
				t.Errorf("familyNeeds = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDeclares_TwoRowsOverOneRoute_IsAProblemAndTheFirstAnswers verifies a
// route two rows match is answered by the first and reported against the
// second, and that a family's problems name what it lacks.
func TestDeclares_TwoRowsOverOneRoute_IsAProblemAndTheFirstAnswers(t *testing.T) {
	first := routeFamily{Pattern: "/pipelines", Category: categoryTracked, Reason: "r", Issue: 1, Routes: 1}
	second := routeFamily{Methods: []string{http.MethodGet}, Pattern: "/pipelines", Category: categoryClientProtocol, Routes: 1}
	report := judgeAll(judgeInput{
		live:     apilive.Document{Routes: []apilive.Route{{Method: http.MethodGet, Path: apilive.EndpointPrefix + "/pipelines"}}},
		families: []routeFamily{first, second},
	})
	want := []string{
		"family GET /pipelines answers GET /pipelines, which family * /pipelines answers too",
		"family GET /pipelines answers no route",
		"family GET /pipelines: it gives no reason",
	}
	if !slices.Equal(report.Stale, sortedUnique(want)) {
		t.Errorf("stale = %q, want %q", report.Stale, want)
	}
	if got := report.Answered; len(got) != 1 || got[0].Family != "* /pipelines" || got[0].Issue != 1 {
		t.Errorf("answered = %+v, want the pipelines route answered by the first row", got)
	}
}

// TestDeclares_ATrackedRowOverARouteADecisionSettles_IsAProblem verifies a
// tracked row that answers a route whose client-go method is decided
// COVERED_GENERIC or INTENTIONAL_SKIP_*, which say the route needs no action
// of its own, is reported against that decision, and that neither a decision
// that settles nothing under a tracked row nor a settling one under a row of
// another category is. The row answers the route all the same.
func TestDeclares_ATrackedRowOverARouteADecisionSettles_IsAProblem(t *testing.T) {
	settled := func(category string) []string {
		return []string{"family GET /a tracks GET /a, and A.Get is decided " + category + ", which says the route needs no action of its own: an action still to implement is no method acceptedMissingMethods settles"}
	}
	cases := []struct {
		name, category, decision string
		want                     []string
	}{
		{name: "tracked over COVERED_GENERIC", category: categoryTracked, decision: "COVERED_GENERIC", want: settled("COVERED_GENERIC")},
		{name: "tracked over INTENTIONAL_SKIP_BINARY", category: categoryTracked, decision: "INTENTIONAL_SKIP_BINARY", want: settled("INTENTIONAL_SKIP_BINARY")},
		{name: "tracked over INTENTIONAL_SKIP_OTHER", category: categoryTracked, decision: "INTENTIONAL_SKIP_OTHER", want: settled("INTENTIONAL_SKIP_OTHER")},
		{name: "tracked over a method nothing decides", category: categoryTracked, decision: adjudicationNone},
		{name: "covered by graphql over COVERED_GENERIC", category: categoryCoveredByGraphQL, decision: "COVERED_GENERIC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := newJudge(judgeInput{families: []routeFamily{{Methods: []string{http.MethodGet}, Pattern: "/a", Category: tc.category, Reason: "r", Routes: 1}}})
			verdict := &Route{Route: "GET /a", Adjudications: []Adjudication{{Method: "A.Get", Category: tc.decision}}}
			if !j.declares(http.MethodGet, "/a", verdict) || verdict.Class != ClassDeclared || !slices.Equal(j.problems, tc.want) {
				t.Errorf("declares: %+v, problems %q; want the route declared and the problems %q", verdict, j.problems, tc.want)
			}
		})
	}
}

// TestFamilyProblems_AClientProtocolRow_IsRefusedOnlyOverARouteClientGoSends
// verifies a client-protocol row is refused for each route it answers that
// client-go sends, and for no other, and that a row of another category is
// not asked.
func TestFamilyProblems_AClientProtocolRow_IsRefusedOnlyOverARouteClientGoSends(t *testing.T) {
	j := newJudge(judgeInput{})
	j.methodsOf["GET /packages/npm/:name"] = []string{"Packages.GetNpmPackage"}
	answered := []string{"GET /packages/npm/:name", "GET /packages/npm/-/ping"}
	protocol := routeFamily{Pattern: "/packages/npm/**", Category: categoryClientProtocol, Reason: "the npm client", Routes: 2}
	want := []string{"family * /packages/npm/** is client-protocol, and client-go sends GET /packages/npm/:name through Packages.GetNpmPackage: that route is R-ACTION's"}
	if got := j.familyProblems(protocol, answered); !slices.Equal(got, want) {
		t.Errorf("familyProblems = %q, want %q", got, want)
	}
	tracked := routeFamily{Pattern: "/packages/npm/**", Category: categoryTracked, Reason: "pending", Issue: 1, Routes: 2}
	if got := j.familyProblems(tracked, answered); len(got) != 0 {
		t.Errorf("familyProblems of a tracked row = %q, want none", got)
	}
}

// TestRowBuilders_EachCategory_BuildsTheRowItNames verifies the constructors
// the table is written with: a tracked row with its issue, a download over
// GET alone with the tail that says why it is tracked, a public_endpoint row
// with its own tail, a client-protocol row whose reason names the protocol,
// and a covered-by-graphql row with its actions. on lists the verbs, none
// standing for every verb.
func TestRowBuilders_EachCategory_BuildsTheRowItNames(t *testing.T) {
	cases := map[string]struct{ got, want routeFamily }{
		"tracked": {
			got:  tracked(7, 2, on(http.MethodGet, http.MethodPost), "/a/**", "why"),
			want: routeFamily{Methods: []string{http.MethodGet, http.MethodPost}, Pattern: "/a/**", Category: categoryTracked, Reason: "why", Issue: 7, Routes: 2},
		},
		"download": {
			got:  download(7, 1, "/a/*/file", "downloads a file"),
			want: routeFamily{Methods: []string{http.MethodGet}, Pattern: "/a/*/file", Category: categoryTracked, Reason: "downloads a file" + downloadTail, Issue: 7, Routes: 1},
		},
		"public endpoint": {
			got:  publicEndpoint(7, 1, on(), "/a", "reads an index"),
			want: routeFamily{Pattern: "/a", Category: categoryTracked, Reason: "reads an index" + publicTail, Issue: 7, Routes: 1},
		},
		"client protocol": {
			got:  protocol("npm", 3, on(http.MethodPut), "/b/**", "the dist-tags"),
			want: routeFamily{Methods: []string{http.MethodPut}, Pattern: "/b/**", Category: categoryClientProtocol, Reason: "the npm protocol: the dist-tags", Routes: 3},
		},
		"covered by graphql": {
			got:  overGraphQL([]string{"epic.list"}, 1, on(http.MethodGet), "/c", "lists them"),
			want: routeFamily{Methods: []string{http.MethodGet}, Pattern: "/c", Category: categoryCoveredByGraphQL, Reason: "lists them", Actions: []string{"epic.list"}, Routes: 1},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if !reflect.DeepEqual(tc.got, tc.want) {
				t.Errorf("built %+v, want %+v", tc.got, tc.want)
			}
		})
	}
}

// TestRouteFamilies_EveryRow_LacksNothing holds the table as written to what
// its categories need, whatever the record says. The catalog and the request
// record are read for the actions a covered-by-graphql row names.
func TestRouteFamilies_EveryRow_LacksNothing(t *testing.T) {
	catalog := map[string]bool{}
	record := actionrequests.Record{}
	for _, family := range routeFamilies {
		for _, action := range family.Actions {
			catalog[action] = true
			record.Actions = append(record.Actions, actionrequests.RecordAction{ID: action, Requests: []actionrequests.RecordRequest{{Kind: actionrequests.KindGraphQL}}})
		}
	}
	j := newJudge(judgeInput{record: record, catalog: catalog})
	for _, family := range routeFamilies {
		t.Run(family.label(), func(t *testing.T) {
			if needs := j.familyNeeds(family); len(needs) != 0 || family.Routes <= 0 {
				t.Errorf("family %s lacks %q and pins %d routes", family.label(), needs, family.Routes)
			}
		})
	}
}
