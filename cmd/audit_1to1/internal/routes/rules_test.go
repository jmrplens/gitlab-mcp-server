// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package routes

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// TestSkipReasons_EveryRow_NamesACategoryTheTableDefinesAndAReason holds the
// closed table to its shape whatever the record says: a category the table
// defines and a reason for each row, and only public_endpoint and the four
// reasons GitLab's code lets a user's token call excluding nothing.
func TestSkipReasons_EveryRow_NamesACategoryTheTableDefinesAndAReason(t *testing.T) {
	carried := map[string]bool{}
	for reason := range skipReasons {
		carried[reason] = true
	}
	if problems := skipProblems(skipReasons, carried); len(problems) != 0 {
		t.Errorf("skipProblems = %q, want none", problems)
	}
	var excludeNothing []string
	for reason, row := range skipReasons {
		if !row.excludes() {
			excludeNothing = append(excludeNothing, reason)
		}
	}
	slices.Sort(excludeNothing)
	want := []string{"ai_workflows_oauth_auth", "ai_workflows_token_auth", "modular_service_token_exchange", "public_endpoint", "usage_data_auth"}
	if !slices.Equal(excludeNothing, want) {
		t.Errorf("the reasons that exclude nothing are %v, want %v", excludeNothing, want)
	}
}

// TestSkipProblems_EachWayARowFails_IsNamed verifies every problem of the
// skip table is reported: a category the table does not define, no reason, a
// reason no route carries, and a reason a route carries that no row holds.
func TestSkipProblems_EachWayARowFails_IsNamed(t *testing.T) {
	skips := map[string]skipReason{
		"odd_auth":    {Category: "odd", Reason: "a reason"},
		"silent_auth": {Category: skipWorkhorse},
		"gone_auth":   {Category: skipWorkhorse, Reason: "a reason"},
	}
	got := skipProblems(skips, map[string]bool{"odd_auth": true, "silent_auth": true, "new_auth": true})
	slices.Sort(got)
	want := []string{
		"skip reason gone_auth is carried by no route of the record",
		"skip reason new_auth is carried by a route of the record and the table does not hold it",
		`skip reason odd_auth: category "odd" is not one the table defines`,
		"skip reason silent_auth gives no reason",
	}
	if !slices.Equal(got, want) {
		t.Errorf("skipProblems = %q, want %q", got, want)
	}
}

// TestSkipExceptions_EveryRow_ExcludesWithTheCompositeIdentityGitLabChecks
// holds the exceptions as written to what their table needs, whatever the
// record says, and each to the check of GitLab's that it rests on.
func TestSkipExceptions_EveryRow_ExcludesWithTheCompositeIdentityGitLabChecks(t *testing.T) {
	excepted := map[string]bool{}
	for name := range skipExceptions {
		excepted[name] = true
	}
	if problems := exceptionProblems(skipExceptions, excepted); len(problems) != 0 {
		t.Errorf("exceptionProblems = %q, want none", problems)
	}
	for name, exception := range skipExceptions {
		t.Run(name, func(t *testing.T) {
			if exception.Category != skipOtherCaller || !strings.Contains(exception.Reason, "verify_flow_composite_identity!") {
				t.Errorf("%s = %+v, want other-credential by the composite identity GitLab checks", name, exception)
			}
		})
	}
}

// TestExceptionProblems_EachWayAnExceptionFails_IsNamed verifies every
// problem of a skip exception is reported: a category that excludes nothing,
// one the table does not define, no reason, and no route answered.
func TestExceptionProblems_EachWayAnExceptionFails_IsNamed(t *testing.T) {
	exceptions := map[string]skipReason{
		"GET /a": {Category: skipUserToken, Reason: "a reason"},
		"GET /b": {Category: "odd", Reason: "a reason"},
		"GET /c": {Category: skipOtherCaller},
		"GET /d": otherCredential("a reason"),
	}
	got := exceptionProblems(exceptions, map[string]bool{"GET /a": true, "GET /b": true, "GET /c": true})
	slices.Sort(got)
	want := []string{
		`skip exception GET /a: category "user-token" excludes nothing or is not one the table defines`,
		`skip exception GET /b: category "odd" excludes nothing or is not one the table defines`,
		"skip exception GET /c gives no reason",
		"skip exception GET /d answers no route: the record does not mount it, an action reaches it, or its skip reason excludes it on its own",
	}
	if !slices.Equal(got, want) {
		t.Errorf("exceptionProblems = %q, want %q", got, want)
	}
}

// slugInput is a tree of integration routes, one of them reached by an
// action whose requests derive a spelling of every PUT and every GET of the
// pattern, and a group integration the same action derives nothing of.
func slugInput() judgeInput {
	prefix := apilive.EndpointPrefix
	set := actionrequests.RecordRequest{Kind: actionrequests.KindREST, Route: "PUT /projects/:id/integrations/slack", Derived: "PUT /projects/:/integrations/:"}
	get := actionrequests.RecordRequest{Kind: actionrequests.KindREST, Route: "GET /projects/:id/integrations/slack", Derived: "GET /projects/:/integrations/:"}
	return judgeInput{
		live: apilive.Document{Routes: []apilive.Route{
			{Method: http.MethodPut, Path: prefix + "/projects/:id/integrations/slack"},
			{Method: http.MethodPut, Path: prefix + "/projects/:id/integrations/jira"},
			{Method: http.MethodGet, Path: prefix + "/projects/:id/integrations/jira"},
			{Method: http.MethodPut, Path: prefix + "/projects/:id/integrations/jira/extra"},
			{Method: http.MethodPut, Path: prefix + "/groups/:id/integrations/jira"},
		}},
		record: actionrequests.Record{Actions: []actionrequests.RecordAction{{ID: "integration.set", Requests: []actionrequests.RecordRequest{get, set}}}},
		slugs: []slugRule{
			{Action: "integration.set", Method: http.MethodPut, Pattern: "/projects/*/integrations/*", Reason: "the slug"},
			{Action: "integration.set", Method: http.MethodPut, Pattern: "/groups/*/integrations/*", Reason: "the slug"},
		},
	}
}

// TestSlugAnswers_OnlyARouteTheActionDerives_IsAnswered verifies a slug rule
// answers a route of its verb and pattern that a spelling its action derives
// meets, and nothing else: not another verb, even one the action derives a
// spelling of, not another pattern, and not a route of the pattern the
// action derives no spelling of.
func TestSlugAnswers_OnlyARouteTheActionDerives_IsAnswered(t *testing.T) {
	j := newJudge(slugInput())
	cases := []struct {
		method, path string
		want         bool
	}{
		{method: http.MethodPut, path: "/projects/:id/integrations/jira", want: true},
		{method: http.MethodGet, path: "/projects/:id/integrations/jira"},
		{method: http.MethodPut, path: "/projects/:id/integrations/jira/extra"},
		{method: http.MethodPut, path: "/groups/:id/integrations/jira"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			verdict := &Route{Route: tc.method + " " + tc.path}
			if got := j.slugAnswers(tc.method, tc.path, verdict); got != tc.want || (verdict.Class == ClassSlugFromInput) != tc.want {
				t.Errorf("slugAnswers = %t (%+v), want %t", got, verdict, tc.want)
			}
		})
	}
}

// TestDerivedMeets_ADerivedSpelling_StandsForAnySegmentItLeavesOpen verifies
// the comparison a slug rule rests on: the same length, and every segment the
// same or a placeholder of the derived spelling.
func TestDerivedMeets_ADerivedSpelling_StandsForAnySegmentItLeavesOpen(t *testing.T) {
	cases := []struct {
		name            string
		derived, record []string
		want            bool
	}{
		{name: "placeholder", derived: []string{"projects", ":", "integrations", ":"}, record: []string{"projects", ":", "integrations", "jira"}, want: true},
		{name: "longer", derived: []string{"projects", ":"}, record: []string{"projects", ":", "integrations"}},
		{name: "another literal", derived: []string{"projects", ":", "hooks"}, record: []string{"projects", ":", "integrations"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := derivedMeets(tc.derived, tc.record); got != tc.want {
				t.Errorf("derivedMeets(%v, %v) = %t, want %t", tc.derived, tc.record, got, tc.want)
			}
		})
	}
}

// TestSlugProblems_EachWayARuleFails_IsNamed verifies every problem of a slug
// rule is reported: an action the catalog does not hold, no reason, and no
// route answered, which is what a rule whose action stopped deriving a
// spelling of its pattern comes to.
func TestSlugProblems_EachWayARuleFails_IsNamed(t *testing.T) {
	rules := []slugRule{
		{Action: "integration.set", Method: http.MethodPut, Pattern: "/projects/*/integrations/*", Reason: "the slug"},
		{Action: "integration.gone", Method: http.MethodPut, Pattern: "/groups/*/integrations/*"},
	}
	got := slugProblems(rules, [][]string{{"PUT /projects/:id/integrations/jira"}, nil}, map[string]bool{"integration.set": true})
	want := []string{
		"slug rule PUT /groups/*/integrations/* (integration.gone) names an action the catalog does not hold",
		"slug rule PUT /groups/*/integrations/* (integration.gone) gives no reason",
		"slug rule PUT /groups/*/integrations/* (integration.gone) answers no route: no request of the action derives a spelling that meets one",
	}
	if !slices.Equal(got, want) {
		t.Errorf("slugProblems = %q, want %q", got, want)
	}
}

// TestJudgeAll_TheSlugTree_LeavesARuleThatAnswersNothingStale verifies the
// group rule of the slug tree, whose action derives no group spelling, is
// stale while the project rule answers its route.
func TestJudgeAll_TheSlugTree_LeavesARuleThatAnswersNothingStale(t *testing.T) {
	in := slugInput()
	in.catalog = map[string]bool{"integration.set": true}
	report := judgeAll(in)
	want := []string{"slug rule PUT /groups/*/integrations/* (integration.set) answers no route: no request of the action derives a spelling that meets one"}
	if !slices.Equal(report.Stale, want) || report.Summary.SlugFromInput != 1 {
		t.Errorf("stale = %q and %d slug routes, want %q and one", report.Stale, report.Summary.SlugFromInput, want)
	}
}

// TestAliasAnswers_AServicesRoute_IsAnAliasOnlyOfAnAnsweredTwin verifies the
// deprecated alias rule: a /services/ route whose /integrations/ twin of the
// same verb is reached is an alias; one whose twin the record does not mount,
// or whose twin is a finding, is not; and a route with no services segment is
// not asked.
func TestAliasAnswers_AServicesRoute_IsAnAliasOnlyOfAnAnsweredTwin(t *testing.T) {
	prefix := apilive.EndpointPrefix
	in := judgeInput{
		live: apilive.Document{Routes: []apilive.Route{
			{Method: http.MethodGet, Path: prefix + "/projects/:id/integrations"},
			{Method: http.MethodGet, Path: prefix + "/projects/:id/services"},
			{Method: http.MethodDelete, Path: prefix + "/projects/:id/integrations/:slug"},
			{Method: http.MethodDelete, Path: prefix + "/projects/:id/services/:slug"},
			{Method: http.MethodPost, Path: prefix + "/projects/:id/services/:slug"},
		}},
		record: actionrequests.Record{Actions: []actionrequests.RecordAction{{
			ID: "integration.list", Requests: []actionrequests.RecordRequest{{Kind: actionrequests.KindREST, Route: "GET /projects/:id/integrations"}},
		}}},
	}
	j := newJudge(in)
	cases := []struct {
		method, path string
		want         bool
	}{
		{method: http.MethodGet, path: "/projects/:id/services", want: true},
		{method: http.MethodDelete, path: "/projects/:id/services/:slug"},
		{method: http.MethodPost, path: "/projects/:id/services/:slug"},
		{method: http.MethodGet, path: "/projects/:id/integrations"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			verdict := &Route{Route: tc.method + " " + tc.path}
			got := j.aliasAnswers(tc.method, tc.path, verdict)
			if got != tc.want || (verdict.Twin != "") != tc.want {
				t.Errorf("aliasAnswers = %t (%+v), want %t", got, verdict, tc.want)
			}
		})
	}
}

// TestTwinOf_ASegment_IsSwappedWhereverThePathHoldsIt verifies the spelling
// both alias rules rest on: the first segment named is swapped, the first of
// the path included, and a path that does not hold it has no twin.
func TestTwinOf_ASegment_IsSwappedWhereverThePathHoldsIt(t *testing.T) {
	cases := []struct {
		path, want string
		spelled    bool
	}{
		{path: "/projects/:id/services/:slug", want: "GET /projects/:id/integrations/:slug", spelled: true},
		{path: "services/services", want: "GET integrations/services", spelled: true},
		{path: "/projects/:id/hooks"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got, spelled := twinOf(http.MethodGet, tc.path, servicesSegment, integrationsSegment)
			if got != tc.want || spelled != tc.spelled {
				t.Errorf("twinOf(%q) = %q, %t; want %q, %t", tc.path, got, spelled, tc.want, tc.spelled)
			}
		})
	}
}

// TestReachedAlias_AnIntegrationsRoute_NamesOnlyAReachedServicesTwin
// verifies the other direction of the alias: an /integrations/ route whose
// deprecated /services/ twin of the same verb an action reaches names it,
// and one whose twin no action reaches, a route of another verb and a route
// with no integrations segment name nothing.
func TestReachedAlias_AnIntegrationsRoute_NamesOnlyAReachedServicesTwin(t *testing.T) {
	j := newJudge(judgeInput{
		live: apilive.Document{Routes: []apilive.Route{
			{Method: http.MethodGet, Path: apilive.EndpointPrefix + "/projects/:id/services"},
			{Method: http.MethodDelete, Path: apilive.EndpointPrefix + "/projects/:id/services/:slug"},
		}},
		record: actionrequests.Record{Actions: []actionrequests.RecordAction{{
			ID: "integration.list", Requests: []actionrequests.RecordRequest{{Kind: actionrequests.KindREST, Route: "GET /projects/:id/services"}},
		}}},
	})
	cases := []struct{ method, path, want string }{
		{method: http.MethodGet, path: "/projects/:id/integrations", want: "GET /projects/:id/services"},
		{method: http.MethodDelete, path: "/projects/:id/integrations/:slug"},
		{method: http.MethodPost, path: "/projects/:id/integrations"},
		{method: http.MethodGet, path: "/projects/:id/hooks"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			if got := j.reachedAlias(tc.method, tc.path); got != tc.want {
				t.Errorf("reachedAlias(%s %s) = %q, want %q", tc.method, tc.path, got, tc.want)
			}
		})
	}
}
