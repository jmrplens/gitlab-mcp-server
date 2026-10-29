// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdk"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/freshness"
	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
)

// feedMethod is where the stand-in client-go grows a method for the feed
// route, the state in which a client-protocol row over it is refused.
const feedMethod = "// FEED\n"

// standInSDK is a stand-in for client-go's root package: a list an action
// calls, a delete decided COVERED_GRAPHQL, and a list decided COVERED_GENERIC.
const standInSDK = `package gitlab

import "net/http"

var (
	routeWidgets    = route("projects/%s/widgets")
	routeWidget     = route("projects/%s/widgets/%s")
	routeWidgetFeed = route("projects/%s/widgets/%s/feed")
	routeGizmos     = route("gizmos")
)

func (s *WidgetsService) ListWidgets(pid any, options ...RequestOptionFunc) ([]*Widget, *Response, error) {
	return do[[]*Widget](s.client, withPath(routeWidgets, ProjectID{pid}))
}

func (s *WidgetsService) DeleteWidget(pid any, widget int64, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client, withMethod(http.MethodDelete), withPath(routeWidget, ProjectID{pid}, widget))
	return resp, err
}

func (s *GizmosService) ListGizmos(options ...RequestOptionFunc) ([]*Gizmo, *Response, error) {
	return do[[]*Gizmo](s.client, withPath(routeGizmos))
}
` + feedMethod

// feedSource is the method the stand-in grows for the feed route.
const feedSource = `
func (s *WidgetsService) GetWidgetFeed(pid any, widget int64, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withPath(routeWidgetFeed, ProjectID{pid}, widget))
}
`

// standInMethods writes the stand-in client-go, with the feed method or
// without it, and reads it the way the scope does.
func standInMethods(t *testing.T, withFeed bool) []sdkroutes.Method {
	t.Helper()
	feed := ""
	if withFeed {
		feed = feedSource
	}
	dir := t.TempDir()
	source := strings.NewReplacer(feedMethod, feed).Replace(standInSDK)
	if err := os.WriteFile(filepath.Join(dir, "sdk.go"), []byte(source), 0o600); err != nil {
		t.Fatalf("write the stand-in: %v", err)
	}
	return sdkroutes.Read(dir).Methods()
}

// liveRoute builds a route of the record, skipping the fine-grained check for
// skip when it is not empty.
func liveRoute(method, path, skip string) apilive.Route {
	route := apilive.Route{Method: method, Path: path, Summary: "summary of " + path}
	if skip != "" {
		route.Authorization = &apilive.RouteAuthorization{Skip: skip}
	}
	return route
}

// The stand-in record's routes.
const (
	widgetsRoute     = "GET /projects/:id/widgets"
	feedRoute        = "GET /projects/:id/widgets/:widget_id/feed"
	feedLatestRoute  = "GET /projects/:id/widgets/:widget_id/feed/latest"
	deleteRoute      = "DELETE /projects/:id/widgets/:widget_id"
	authorizeRoute   = "POST /projects/:id/widgets/authorize"
	scimRoute        = "GET /api/scim/:version/Users"
	slackRoute       = "PUT /projects/:id/integrations/slack"
	jiraRoute        = "PUT /projects/:id/integrations/jira"
	jiraAliasRoute   = "PUT /projects/:id/services/jira"
	shellRoute       = "GET /internal/widgets"
	internalRoute    = "GET /internal/gadgets"
	gizmosRoute      = "GET /gizmos"
	verifyRoute      = "POST /runners/verify"
	managersRoute    = "DELETE /runners/managers"
	publicRoute      = "GET /widgets/public"
	unknownSkipRoute = "GET /widgets/brand_new"
	internalPublic   = "GET /internal/public"
	flowRoute        = "GET /widgets/flow"
	mineRoute        = "GET /widgets/mine"
	integrationsList = "GET /projects/:id/integrations"
	servicesList     = "GET /projects/:id/services"
)

// standInLive is the stand-in live record: every class of route, and one
// route mounted twice.
func standInLive() apilive.Document {
	prefix := apilive.EndpointPrefix
	return apilive.Document{Routes: []apilive.Route{
		liveRoute(http.MethodGet, prefix+"/projects/:id/widgets", ""),
		liveRoute(http.MethodGet, prefix+"/projects/:id/widgets", ""),
		liveRoute(http.MethodGet, prefix+"/projects/:id/widgets/:widget_id/feed", ""),
		liveRoute(http.MethodDelete, prefix+"/projects/:id/widgets/:widget_id", ""),
		liveRoute(http.MethodPost, prefix+"/projects/:id/widgets/authorize", "workhorse_pre_authorization"),
		liveRoute(http.MethodGet, "/api/scim/:version/Users", "scim_token_auth"),
		liveRoute(http.MethodPut, prefix+"/projects/:id/integrations/slack", ""),
		liveRoute(http.MethodPut, prefix+"/projects/:id/integrations/jira", ""),
		liveRoute(http.MethodPut, prefix+"/projects/:id/services/jira", ""),
		liveRoute(http.MethodGet, prefix+"/internal/widgets", "gitlab_shell_token_auth"),
		liveRoute(http.MethodGet, prefix+"/internal/gadgets", ""),
		liveRoute(http.MethodGet, prefix+"/gizmos", ""),
		liveRoute(http.MethodPost, prefix+"/runners/verify", "runner_token_auth"),
		liveRoute(http.MethodDelete, prefix+"/runners/managers", "runner_token_auth"),
		liveRoute(http.MethodGet, prefix+"/widgets/public", "public_endpoint"),
		liveRoute(http.MethodGet, prefix+"/internal/public", "public_endpoint"),
		liveRoute(http.MethodGet, prefix+"/widgets/flow", "widget_user_auth"),
		liveRoute(http.MethodGet, prefix+"/widgets/mine", "widget_user_auth"),
		liveRoute(http.MethodGet, prefix+"/projects/:id/integrations", ""),
		liveRoute(http.MethodGet, prefix+"/projects/:id/services", ""),
	}}
}

// rest builds a REST request of the record.
func rest(route string, sdkMethods ...string) actionrequests.RecordRequest {
	return actionrequests.RecordRequest{Kind: actionrequests.KindREST, Route: route, SDKMethods: sdkMethods}
}

// standInRecord is the stand-in request record: a list through client-go, an
// integration whose slug the caller names, the integrations listed through
// their deprecated spelling, a runner verification, and a GraphQL action.
func standInRecord() actionrequests.Record {
	set := rest(slackRoute)
	set.Derived = "PUT /projects/:/integrations/:"
	return actionrequests.Record{Actions: []actionrequests.RecordAction{
		{ID: "widget.list", Requests: []actionrequests.RecordRequest{rest(widgetsRoute, "Widgets.ListWidgets")}},
		{ID: "widget.integration_set", Requests: []actionrequests.RecordRequest{set}},
		{ID: "widget.integration_list", Requests: []actionrequests.RecordRequest{rest(servicesList)}},
		{ID: "runner.verify", Requests: []actionrequests.RecordRequest{rest(verifyRoute)}},
		{ID: "widget.graphql_feed", Requests: []actionrequests.RecordRequest{{Kind: actionrequests.KindGraphQL, Operation: "query feed"}}},
	}}
}

// standInSkips is the stand-in skip table.
func standInSkips() map[string]skipReason {
	return map[string]skipReason{
		"workhorse_pre_authorization": {Category: skipWorkhorse, Reason: "Workhorse asks first"},
		"scim_token_auth":             otherCredential("an identity provider's token"),
		"gitlab_shell_token_auth":     otherCredential("GitLab Shell's token"),
		"runner_token_auth":           otherCredential("a runner's token"),
		"public_endpoint":             {Category: skipPublic, Reason: "with or without a token"},
		"widget_user_auth":            {Category: skipUserToken, Reason: "a user's token calls it"},
	}
}

// trackedFeed is a tracked row over the feed route and anything under it.
func trackedFeed() routeFamily {
	return routeFamily{
		Methods: []string{http.MethodGet}, Pattern: "/projects/*/widgets/*/feed/**", Category: categoryTracked,
		Reason: "the feed no action reads", Issue: 7, Routes: 1,
	}
}

// standInInput is the stand-in tree as the scope reads it.
func standInInput(t *testing.T) judgeInput {
	t.Helper()
	return judgeInput{
		record:   standInRecord(),
		live:     standInLive(),
		methods:  standInMethods(t, false),
		accepted: map[string]string{"Widgets.DeleteWidget": "COVERED_GRAPHQL. widget.graphql_feed", "Gizmos.ListGizmos": "COVERED_GENERIC. another route"},
		services: map[string]sdk.ServiceDeclaration{},
		catalog: map[string]bool{
			"widget.list": true, "widget.integration_set": true, "widget.integration_list": true, "runner.verify": true, "widget.graphql_feed": true,
		},
		skips:      standInSkips(),
		exceptions: map[string]skipReason{flowRoute: otherCredential("only a flow's own identity")},
		graphql:    map[string][]string{"Widgets.DeleteWidget": {"widget.graphql_feed"}},
		slugs: []slugRule{{
			Action: "widget.integration_set", Method: http.MethodPut, Pattern: "/projects/*/integrations/*", Reason: "the caller names the slug",
		}},
	}
}

// classes maps every route a report lists to its class.
func classes(report Report) map[string]string {
	out := map[string]string{}
	for _, name := range report.Reached {
		out[name] = ClassReached
	}
	for _, route := range slices.Concat(report.Findings, report.Answered) {
		out[route.Route] = route.Class
	}
	return out
}

// TestJudgeAll_TheStandInTree_ClassifiesEveryRoute holds every rule to its
// place in the order on a tree that has one route of each: the route an
// action sends, SCIM outside the API, a Workhorse authorization the skip
// table excludes, an integration slug the caller names and its deprecated
// alias, an internal route a skip excludes, one that carries no skip and one
// whose reason excludes nothing, a delete whose method is decided
// COVERED_GRAPHQL with the GraphQL action that stands for it, a list decided
// COVERED_GENERIC, which does not carry, a runner route beside one an action
// sends, a public route, two routes a user's token calls of which an
// exception takes one out, the integrations list an action reaches only
// through its deprecated spelling, and the feed nothing answers. The route
// mounted twice counts once.
func TestJudgeAll_TheStandInTree_ClassifiesEveryRoute(t *testing.T) {
	report := judgeAll(standInInput(t))
	want := map[string]string{
		widgetsRoute:     ClassReached,
		slackRoute:       ClassReached,
		verifyRoute:      ClassReached,
		servicesList:     ClassReached,
		scimRoute:        ClassOutsideAPI,
		authorizeRoute:   ClassSkip,
		shellRoute:       ClassSkip,
		flowRoute:        ClassSkip,
		jiraRoute:        ClassSlugFromInput,
		jiraAliasRoute:   ClassDeprecatedAlias,
		internalRoute:    ClassInternalUnskipped,
		internalPublic:   ClassInternalNotExcluded,
		deleteRoute:      ClassInherited,
		gizmosRoute:      ClassUnanswered,
		managersRoute:    ClassUnanswered,
		publicRoute:      ClassUnanswered,
		mineRoute:        ClassUnanswered,
		integrationsList: ClassUnanswered,
		feedRoute:        ClassUnanswered,
	}
	got := classes(report)
	for route, class := range want {
		t.Run(route, func(t *testing.T) {
			if got[route] != class {
				t.Errorf("%s is %q, want %q", route, got[route], class)
			}
		})
	}
	if len(got) != len(want) {
		t.Errorf("the report lists %d routes, want %d: %v", len(got), len(want), got)
	}
	wantSummary := Summary{
		Mounted: 20, Routes: 19, Reached: 4, OutsideAPI: 1, Skipped: 3, SlugFromInput: 1, DeprecatedAlias: 1,
		Inherited: 1, Unanswered: 6, InternalUnskipped: 1, InternalNotExcluded: 1,
		SkipReasonsReached: []string{"runner_token_auth"},
	}
	if !summariesEqual(report.Summary, wantSummary) {
		t.Errorf("summary = %+v, want %+v", report.Summary, wantSummary)
	}
	if len(report.Stale) != 0 {
		t.Errorf("stale = %v, want none: a finding is a report and no declaration is stale", report.Stale)
	}
	checkStandInDetails(t, report)
}

// summariesEqual compares two summaries field by field.
func summariesEqual(got, want Summary) bool {
	reasons := slices.Equal(got.SkipReasonsReached, want.SkipReasonsReached)
	got.SkipReasonsReached, want.SkipReasonsReached = nil, nil
	return reasons && reflect.DeepEqual(got, want)
}

// standInVerdicts indexes a report's findings and answered routes by name.
func standInVerdicts(report Report) map[string]Route {
	byRoute := map[string]Route{}
	for _, route := range slices.Concat(report.Findings, report.Answered) {
		byRoute[route.Route] = route
	}
	return byRoute
}

// checkStandInExceptions holds what the stand-in report says of the routes
// the review of the scope added: the exception's category and reason, the
// reached twin of the integrations list and none on a route that is no
// integration, and the GraphQL action a carried decision names.
func checkStandInExceptions(t *testing.T, report Report) {
	t.Helper()
	byRoute := standInVerdicts(report)
	if flow := byRoute[flowRoute]; flow.SkipReason != "widget_user_auth" || flow.Category != skipOtherCaller || flow.Reason != "only a flow's own identity" {
		t.Errorf("%s = %+v, want the exception's category and reason", flowRoute, flow)
	}
	if integrations := byRoute[integrationsList]; integrations.Twin != servicesList {
		t.Errorf("%s twin = %q, want the deprecated spelling an action reaches, %s", integrationsList, integrations.Twin, servicesList)
	}
	if feedTwin := byRoute[feedRoute].Twin; feedTwin != "" {
		t.Errorf("%s twin = %q, want none on a route that is no integration", feedRoute, feedTwin)
	}
	wantDelete := []Adjudication{{Method: "Widgets.DeleteWidget", Category: adjudicationGraphQL, Reason: "widget.graphql_feed", Actions: []string{"widget.graphql_feed"}, Carries: true}}
	if got := byRoute[deleteRoute].Adjudications; !reflect.DeepEqual(got, wantDelete) {
		t.Errorf("%s decisions = %+v, want %+v", deleteRoute, got, wantDelete)
	}
}

// checkStandInDetails holds what the stand-in report says of the routes
// beside their class: the order of the lists, the actions, the skip, the
// slug rule, the alias's twin and the decisions kept on a finding.
func checkStandInDetails(t *testing.T, report Report) {
	t.Helper()
	checkStandInExceptions(t, report)
	byRoute := standInVerdicts(report)
	if first := report.Findings[0]; first.Group != "gizmos" {
		t.Errorf("first finding = %+v, want the findings ordered by group", first)
	}
	if got := report.Reached; !slices.Equal(got, []string{servicesList, widgetsRoute, verifyRoute, slackRoute}) {
		t.Errorf("reached = %v, want the four routes an action sends, sorted", got)
	}
	authorize := byRoute[authorizeRoute]
	if authorize.SkipReason != "workhorse_pre_authorization" || authorize.Category != skipWorkhorse || authorize.Reason == "" {
		t.Errorf("%s = %+v, want the skip row's category and reason", authorizeRoute, authorize)
	}
	if managers := byRoute[managersRoute]; managers.SkipReason != "runner_token_auth" || managers.Category != "" {
		t.Errorf("%s = %+v, want its skip reason named and no skip category", managersRoute, managers)
	}
	jira := byRoute[jiraRoute]
	if !slices.Equal(jira.Actions, []string{"widget.integration_set"}) || jira.Family != "PUT /projects/*/integrations/* (widget.integration_set)" {
		t.Errorf("%s = %+v, want the slug rule and its action", jiraRoute, jira)
	}
	if alias := byRoute[jiraAliasRoute]; alias.Twin != jiraRoute {
		t.Errorf("%s twin = %q, want %s", jiraAliasRoute, alias.Twin, jiraRoute)
	}
	gizmos := byRoute[gizmosRoute]
	wantDecision := []Adjudication{{Method: "Gizmos.ListGizmos", Category: "COVERED_GENERIC", Reason: "another route"}}
	if !slices.Equal(gizmos.SDKMethods, []string{"Gizmos.ListGizmos"}) || !reflect.DeepEqual(gizmos.Adjudications, wantDecision) {
		t.Errorf("%s = %+v, want the method and its decision that does not carry", gizmosRoute, gizmos)
	}
	if feed := byRoute[feedRoute]; feed.SDKMethods == nil || len(feed.SDKMethods) != 0 || feed.Summary == "" {
		t.Errorf("%s = %+v, want an empty sdk_methods list and the record's summary", feedRoute, feed)
	}
}

// TestJudgeAll_AFamilyRow_AnswersItsRoutesUntilTheyChange walks a tracked row
// through what the gate is for: the feed nothing answers is a finding; a
// tracked row answers it; an action that starts sending it leaves the row
// answering nothing; a second route under the family moves the count the row
// pins; a client-protocol row over a route client-go sends is refused; a
// client-go method decided in a way that carries takes the route from the
// row, which answers nothing then, the way the bump's declaration of
// GroupPlaceholderReassignments retires the placeholder row. The other
// tables walk the same way: a skip reason the table does not hold fails the
// gate whatever class its routes take, and on a route no action reaches is a
// finding too; a COVERED_GRAPHQL decision carries only with a GraphQL action
// that stands for it; and a GraphQL row or a skip exception that answers no
// route is stale.
func TestJudgeAll_AFamilyRow_AnswersItsRoutesUntilTheyChange(t *testing.T) {
	cases := []struct {
		name      string
		arrange   func(t *testing.T, in *judgeInput)
		wantClass map[string]string
		wantStale []string
	}{
		{
			name:      "no row",
			arrange:   func(*testing.T, *judgeInput) {},
			wantClass: map[string]string{feedRoute: ClassUnanswered},
		},
		{
			name:      "a tracked row",
			arrange:   func(_ *testing.T, in *judgeInput) { in.families = []routeFamily{trackedFeed()} },
			wantClass: map[string]string{feedRoute: ClassDeclared},
		},
		{
			name: "an action that reaches the route",
			arrange: func(_ *testing.T, in *judgeInput) {
				in.families = []routeFamily{trackedFeed()}
				in.record.Actions = append(in.record.Actions, actionrequests.RecordAction{ID: "widget.feed", Requests: []actionrequests.RecordRequest{rest(feedRoute)}})
			},
			wantClass: map[string]string{feedRoute: ClassReached},
			wantStale: []string{"family GET /projects/*/widgets/*/feed/** answers no route"},
		},
		{
			name: "a second route under the family",
			arrange: func(_ *testing.T, in *judgeInput) {
				in.families = []routeFamily{trackedFeed()}
				in.live.Routes = append(in.live.Routes, liveRoute(http.MethodGet, apilive.EndpointPrefix+"/projects/:id/widgets/:widget_id/feed/latest", ""))
			},
			wantClass: map[string]string{feedRoute: ClassDeclared, feedLatestRoute: ClassDeclared},
			wantStale: []string{"family GET /projects/*/widgets/*/feed/** answers 2 routes and pins 1: " + feedRoute + ", " + feedLatestRoute},
		},
		{
			name: "a client-protocol row over a route client-go sends",
			arrange: func(t *testing.T, in *judgeInput) {
				t.Helper()
				in.methods = standInMethods(t, true)
				in.families = []routeFamily{{Pattern: "/projects/*/widgets/*/feed", Category: categoryClientProtocol, Reason: "a client's protocol", Routes: 1}}
			},
			wantClass: map[string]string{feedRoute: ClassDeclared},
			wantStale: []string{"family * /projects/*/widgets/*/feed is client-protocol, and client-go sends " + feedRoute + " through Widgets.GetWidgetFeed: that route is R-ACTION's"},
		},
		{
			name: "a decision that carries, which the row loses to",
			arrange: func(t *testing.T, in *judgeInput) {
				t.Helper()
				in.methods = standInMethods(t, true)
				in.accepted["Widgets.GetWidgetFeed"] = "COVERED_GRAPHQL. widget.graphql_feed"
				in.graphql["Widgets.GetWidgetFeed"] = []string{"widget.graphql_feed"}
				in.families = []routeFamily{trackedFeed()}
			},
			wantClass: map[string]string{feedRoute: ClassInherited},
			wantStale: []string{"family GET /projects/*/widgets/*/feed/** answers no route"},
		},
		{
			name: "a skip reason the table does not hold",
			arrange: func(_ *testing.T, in *judgeInput) {
				in.live.Routes = append(in.live.Routes, liveRoute(http.MethodGet, apilive.EndpointPrefix+"/widgets/brand_new", "brand_new_auth"))
			},
			wantClass: map[string]string{unknownSkipRoute: ClassUnknownSkipReason},
			wantStale: []string{"skip reason brand_new_auth is carried by a route of the record and the table does not hold it"},
		},
		{
			name: "a skip reason only a reached route and a SCIM route carry",
			arrange: func(_ *testing.T, in *judgeInput) {
				in.live.Routes = append(in.live.Routes,
					liveRoute(http.MethodGet, apilive.EndpointPrefix+"/widgets/reached_new", "reached_new_auth"),
					liveRoute(http.MethodGet, "/api/scim/:version/Groups", "scim_new_auth"),
				)
				in.record.Actions = append(in.record.Actions, actionrequests.RecordAction{ID: "widget.reached_new", Requests: []actionrequests.RecordRequest{rest("GET /widgets/reached_new")}})
			},
			wantClass: map[string]string{"GET /widgets/reached_new": ClassReached, "GET /api/scim/:version/Groups": ClassOutsideAPI},
			wantStale: []string{
				"skip reason reached_new_auth is carried by a route of the record and the table does not hold it",
				"skip reason scim_new_auth is carried by a route of the record and the table does not hold it",
			},
		},
		{
			name:      "the row of a reason only SCIM routes carry",
			arrange:   func(_ *testing.T, in *judgeInput) { delete(in.skips, "scim_token_auth") },
			wantClass: map[string]string{scimRoute: ClassOutsideAPI},
			wantStale: []string{"skip reason scim_token_auth is carried by a route of the record and the table does not hold it"},
		},
		{
			name:      "a COVERED_GRAPHQL decision no GraphQL action stands for",
			arrange:   func(_ *testing.T, in *judgeInput) { delete(in.graphql, "Widgets.DeleteWidget") },
			wantClass: map[string]string{deleteRoute: ClassUnanswered},
		},
		{
			name:      "a GraphQL action the catalog does not hold",
			arrange:   func(_ *testing.T, in *judgeInput) { in.graphql["Widgets.DeleteWidget"] = []string{"widget.gone"} },
			wantClass: map[string]string{deleteRoute: ClassUnanswered},
			wantStale: []string{
				"graphql actions of Widgets.DeleteWidget: it names widget.gone, which sends no GraphQL request",
				"graphql actions of Widgets.DeleteWidget: it names widget.gone, which the catalog does not hold",
			},
		},
		{
			name: "GraphQL actions of a method no decision is read with",
			arrange: func(_ *testing.T, in *judgeInput) {
				in.graphql["Widgets.ListWidgets"] = []string{"widget.graphql_feed"}
			},
			wantClass: map[string]string{widgetsRoute: ClassReached, deleteRoute: ClassInherited},
			wantStale: []string{"graphql actions of Widgets.ListWidgets answer no route: no route no action reaches is sent by it under a COVERED_GRAPHQL decision"},
		},
		{
			name:      "a skip exception over a route its reason excludes already",
			arrange:   func(_ *testing.T, in *judgeInput) { in.exceptions[authorizeRoute] = otherCredential("again") },
			wantClass: map[string]string{authorizeRoute: ClassSkip},
			wantStale: []string{"skip exception " + authorizeRoute + " answers no route: the record does not mount it, an action reaches it, or its skip reason excludes it on its own"},
		},
		{
			name: "a skip exception over a route an action starts sending",
			arrange: func(_ *testing.T, in *judgeInput) {
				in.record.Actions = append(in.record.Actions, actionrequests.RecordAction{ID: "widget.flow", Requests: []actionrequests.RecordRequest{rest(flowRoute)}})
			},
			wantClass: map[string]string{flowRoute: ClassReached, mineRoute: ClassUnanswered},
			wantStale: []string{"skip exception " + flowRoute + " answers no route: the record does not mount it, an action reaches it, or its skip reason excludes it on its own"},
		},
		{
			name: "a skip exception over a route a reached reason leaves in",
			arrange: func(_ *testing.T, in *judgeInput) {
				in.exceptions[managersRoute] = otherCredential("a runner's own token")
			},
			wantClass: map[string]string{managersRoute: ClassSkip, verifyRoute: ClassReached},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := standInInput(t)
			tc.arrange(t, &in)
			report := judgeAll(in)
			got := classes(report)
			for route, class := range tc.wantClass {
				if got[route] != class {
					t.Errorf("%s is %q, want %q", route, got[route], class)
				}
			}
			if !slices.Equal(report.Stale, append([]string{}, tc.wantStale...)) {
				t.Errorf("stale = %q, want %q", report.Stale, tc.wantStale)
			}
			if report.Summary.Stale != len(tc.wantStale) || report.Summary.clean() != (len(tc.wantStale) == 0) {
				t.Errorf("summary = %+v, want %d stale and the gate passing only without them", report.Summary, len(tc.wantStale))
			}
		})
	}
}

// TestBuildReport_EachInputThatCannotBeRead_StopsTheRunNamingIt verifies a
// failure of each input is returned with what failed, before anything is
// judged, and that the marshal failure a report never produces is one too.
func TestBuildReport_EachInputThatCannotBeRead_StopsTheRunNamingIt(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name    string
		arrange func()
		wantErr string
	}{
		{name: "request record", arrange: func() {
			readRecord = func(string) (actionrequests.Record, error) { return actionrequests.Record{}, boom }
		}, wantErr: "boom"},
		{name: "live record", arrange: func() {
			readLive = func(string) (apilive.Document, error) { return apilive.Document{}, boom }
		}, wantErr: "read the live record: boom"},
		{name: "client-go", arrange: func() { clientGoDir = func(string) (string, error) { return "", boom } }, wantErr: "find client-go: boom"},
		{name: "catalog", arrange: func() {
			catalogActions = func() ([]requestinventory.Action, error) { return nil, boom }
		}, wantErr: "build the action catalog: boom"},
		{name: "marshal", arrange: func() {
			marshalIndent = func(any, string, string) ([]byte, error) { return nil, boom }
		}, wantErr: "marshal report: boom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			standInSeams(t)
			tc.arrange()
			content, clean, err := Run(t.TempDir(), Options{})
			if err == nil || err.Error() != tc.wantErr || content != nil || clean {
				t.Errorf("Run = %q, %t, %v; want no report and the error %q", content, clean, err, tc.wantErr)
			}
		})
	}
}

// standInSeams points every input of a run at the stand-in tree, and restores
// the seams when the test ends.
func standInSeams(t *testing.T) {
	t.Helper()
	originals := struct {
		record     func(string) (actionrequests.Record, error)
		live       func(string) (apilive.Document, error)
		dir        func(string) (string, error)
		read       func(string) []sdkroutes.Method
		catalog    func() ([]requestinventory.Action, error)
		accepted   func() map[string]string
		services   func() map[string]sdk.ServiceDeclaration
		waits      func() []waits.Entry
		marshal    func(any, string, string) ([]byte, error)
		skips      map[string]skipReason
		exceptions map[string]skipReason
		graphql    map[string][]string
		slugs      []slugRule
		families   []routeFamily
	}{
		readRecord, readLive, clientGoDir, readSDK, catalogActions, acceptedMethods, declaredServices, waitEntries, marshalIndent,
		skipReasons, skipExceptions, graphqlActions, slugRules, routeFamilies,
	}
	t.Cleanup(func() {
		readRecord, readLive, clientGoDir, readSDK = originals.record, originals.live, originals.dir, originals.read
		catalogActions, acceptedMethods, declaredServices, waitEntries = originals.catalog, originals.accepted, originals.services, originals.waits
		marshalIndent, skipReasons, skipExceptions, graphqlActions = originals.marshal, originals.skips, originals.exceptions, originals.graphql
		slugRules, routeFamilies = originals.slugs, originals.families
	})
	in := standInInput(t)
	readRecord = func(string) (actionrequests.Record, error) { return in.record, nil }
	readLive = func(string) (apilive.Document, error) { return in.live, nil }
	clientGoDir = func(string) (string, error) { return "stand-in", nil }
	readSDK = func(dir string) []sdkroutes.Method {
		if dir != "stand-in" {
			t.Errorf("readSDK(%q), want the directory clientGoDir answered", dir)
		}
		return in.methods
	}
	catalogActions = func() ([]requestinventory.Action, error) {
		var out []requestinventory.Action
		for id := range in.catalog {
			out = append(out, requestinventory.Action{ID: id})
		}
		return out, nil
	}
	acceptedMethods = func() map[string]string { return in.accepted }
	declaredServices = func() map[string]sdk.ServiceDeclaration { return in.services }
	waitEntries = func() []waits.Entry { return nil }
	skipReasons, skipExceptions, graphqlActions = in.skips, in.exceptions, in.graphql
	slugRules, routeFamilies = in.slugs, []routeFamily{trackedFeed()}
}

// TestRun_TheStandInTree_WritesTheReportAndGapsOnlyKeepsTheFindings runs the
// scope end to end on the stand-in tree through its seams: the report names
// the two files it reads, the gate passes, and -gaps-only drops the answered
// routes and the reached ones and keeps the findings.
func TestRun_TheStandInTree_WritesTheReportAndGapsOnlyKeepsTheFindings(t *testing.T) {
	for _, gapsOnly := range []bool{false, true} {
		t.Run("gaps only "+map[bool]string{false: "off", true: "on"}[gapsOnly], func(t *testing.T) {
			standInSeams(t)
			content, clean, err := Run(t.TempDir(), Options{GapsOnly: gapsOnly})
			if err != nil || !clean || !strings.HasSuffix(string(content), "}\n") {
				t.Fatalf("Run = %t, %v; want a clean report ending in a newline", clean, err)
			}
			report := decode(t, content)
			if report.Requests != actionrequests.RecordPath || report.Record != "docs/development/gitlab-api-live.json" {
				t.Errorf("report names %q and %q, want the request record and the live record", report.Requests, report.Record)
			}
			if len(report.Findings) == 0 || (len(report.Answered) == 0) == !gapsOnly || (len(report.Reached) == 0) == !gapsOnly {
				t.Errorf("gaps only %t: %d findings, %d answered, %d reached", gapsOnly, len(report.Findings), len(report.Answered), len(report.Reached))
			}
			if got := classes(report)[feedRoute]; got != ClassDeclared && !gapsOnly {
				t.Errorf("%s is %q, want the family row the run reads to answer it", feedRoute, got)
			}
		})
	}
}

// decode reads a report back.
func decode(t *testing.T, content []byte) Report {
	t.Helper()
	var report Report
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatalf("decode the report: %v", err)
	}
	return report
}

// TestDefaultReaders_ReadWhereTheRepositoryKeepsThem verifies the readers the
// scope runs with: the live record under the root, an error naming the file
// on a root that has none, client-go's directory as the go command resolves
// it, and the methods of that directory.
func TestDefaultReaders_ReadWhereTheRepositoryKeepsThem(t *testing.T) {
	if _, err := readLive(t.TempDir()); err == nil || !strings.Contains(err.Error(), apilive.FileName) {
		t.Errorf("readLive of an empty root = %v, want an error naming %s", err, apilive.FileName)
	}
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("find the repository root: %v", err)
	}
	dir, err := clientGoDir(root)
	if err != nil {
		t.Fatalf("clientGoDir: %v", err)
	}
	methods := readSDK(dir)
	if !slices.ContainsFunc(methods, func(method sdkroutes.Method) bool { return method.Key() == "Pipelines.ListProjectPipelines" }) {
		t.Errorf("readSDK(%s) holds %d methods, none Pipelines.ListProjectPipelines", dir, len(methods))
	}
}

// TestIndexMethods_AMethodSendingOneRouteTwice_NamesItOnce verifies a method
// whose two spellings of a route (an optional group taken and left out) find
// the same route of the record is named on it once.
func TestIndexMethods_AMethodSendingOneRouteTwice_NamesItOnce(t *testing.T) {
	j := newJudge(judgeInput{
		live: apilive.Document{Routes: []apilive.Route{{Method: http.MethodGet, Path: apilive.EndpointPrefix + "/groups/:id/(-/)epics"}}},
		methods: []sdkroutes.Method{{Service: "Epics", Name: "ListGroupEpics", Routes: []sdkroutes.Route{
			{Method: http.MethodGet, Path: "/groups/:/epics"}, {Method: http.MethodGet, Path: "/groups/:/-/epics"}, {Method: http.MethodGet, Path: "/nowhere"},
		}}},
	})
	if got := j.methodsOf["GET /groups/:id/(-/)epics"]; !slices.Equal(got, []string{"Epics.ListGroupEpics"}) {
		t.Errorf("methodsOf = %v, want the method named once", got)
	}
}

// TestIsInternal_OnlyGitLabsInternalAPI_IsInternal verifies the internal API
// is its root and what is under it, and not a segment that merely begins
// with its name or a route that holds it further down.
func TestIsInternal_OnlyGitLabsInternalAPI_IsInternal(t *testing.T) {
	cases := map[string]bool{"/internal": true, "/internal/pages": true, "/internals": false, "/projects/:id/internal": false}
	for path, want := range cases {
		t.Run(path, func(t *testing.T) {
			if got := isInternal(path); got != want {
				t.Errorf("isInternal(%q) = %t, want %t", path, got, want)
			}
		})
	}
}

// TestGroupOf_ARoute_ReadsInTheFamilyOfItsFirstLiteralSegments verifies the
// group a route is ordered by: two literal segments under a scope, one
// elsewhere, placeholders, splats and the `-` separator passed over, an
// optional group left out, and the root for a route of no literal segment.
func TestGroupOf_ARoute_ReadsInTheFamilyOfItsFirstLiteralSegments(t *testing.T) {
	cases := map[string]string{
		"/api/:version/projects/:id/(-/)packages/npm/*package_name": "projects/packages",
		"/api/:version/groups/:id/-/epics":                          "groups/epics",
		"/api/:version/projects/:id":                                "projects",
		"/api/:version/pipelines":                                   "pipelines",
		"/api/:version/geo_nodes/:id/repair":                        "geo_nodes",
		"/api/:version/*path":                                       "/",
		"/api/:version":                                             "/",
		"/api/scim/:version/groups/:group/Users":                    "api/scim",
	}
	for path, want := range cases {
		t.Run(path, func(t *testing.T) {
			if got := groupOf(path); got != want {
				t.Errorf("groupOf(%q) = %q, want %q", path, got, want)
			}
		})
	}
}

// repositoryReport runs the scope on the repository's own tree.
func repositoryReport(t *testing.T) Report {
	t.Helper()
	freshness.SkipIfDeferred(t)
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("find the repository root: %v", err)
	}
	content, clean, err := Run(root, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	report := decode(t, content)
	if !clean {
		t.Fatalf("stale route declarations: %v", report.Stale)
	}
	return report
}

// TestRun_Repository_GateIsGreen runs the gate on the repository's own
// request record, live record, client-go and catalog, which is what make
// audit-1to1-routes does: no declaration that no longer describes the tree.
// It reads the committed request record, so a stacked layer that defers that
// record's refresh skips it.
func TestRun_Repository_GateIsGreen(t *testing.T) {
	report := repositoryReport(t)
	summary := report.Summary
	if summary.Routes == 0 || summary.Reached == 0 || summary.Skipped == 0 || summary.Inherited == 0 || summary.Declared == 0 {
		t.Errorf("summary = %+v, want the tree's routes classified and its declarations used", summary)
	}
	total := summary.Reached + summary.OutsideAPI + summary.Skipped + summary.SlugFromInput + summary.DeprecatedAlias +
		summary.Inherited + summary.Declared + summary.Unanswered + summary.UnknownSkipReason + summary.InternalUnskipped +
		summary.InternalNotExcluded + summary.Contradicted
	if total != summary.Routes {
		t.Errorf("the classes count %d routes of %d", total, summary.Routes)
	}
	for _, route := range report.Answered {
		for _, decision := range route.Adjudications {
			if route.Class == ClassInherited && decision.Category == adjudicationGraphQL && len(decision.Actions) == 0 {
				t.Errorf("%s is inherited from %s, a COVERED_GRAPHQL decision that names no GraphQL action", route.Route, decision.Method)
			}
		}
	}
}

// TestRun_Repository_HoldsTheRoutesClientGoLaterModels fixes the routes
// R-ROUTE was built to see on the pinned client-go. It is sensitive to the
// version on purpose: the stack that bumps client-go changes it, and that
// change is the evidence the scope was run again.
//
// On v3.30.0, GET /pipelines and the two placeholder reassignment routes of
// a group are answered by tracked family rows, and no client-go method sends
// them: their fix started upstream, which v3.38.0 delivers
// (Pipelines.ListUserPipelines, GroupPlaceholderReassignments). The route
// Workhorse authorizes the reassignment's CSV upload at skips the
// fine-grained check for workhorse_pre_authorization and is no route a
// caller sends. When the bump declares GroupPlaceholderReassignments
// UNWRAPPED_TRACKED, the two placeholder routes are inherited and their row
// answers nothing, so the bump removes it; GET /pipelines stays tracked until
// an action lists the user's pipelines, ListUserPipelines being decided by
// nothing.
func TestRun_Repository_HoldsTheRoutesClientGoLaterModels(t *testing.T) {
	report := repositoryReport(t)
	byRoute := map[string]Route{}
	for _, route := range slices.Concat(report.Findings, report.Answered) {
		byRoute[route.Route] = route
	}
	for _, name := range []string{"GET /pipelines", "GET /groups/:id/placeholder_reassignments", "POST /groups/:id/placeholder_reassignments"} {
		t.Run(name, func(t *testing.T) {
			route := byRoute[name]
			if route.Class != ClassDeclared || route.Category != categoryTracked || route.Issue == 0 || len(route.SDKMethods) != 0 {
				t.Errorf("%s = %+v, want it tracked by a family row and sent by no client-go method", name, route)
			}
		})
	}
	authorize := byRoute["POST /groups/:id/placeholder_reassignments/authorize"]
	if authorize.Class != ClassSkip || authorize.SkipReason != "workhorse_pre_authorization" {
		t.Errorf("the reassignment upload's authorization = %+v, want skip:workhorse_pre_authorization", authorize)
	}
}

// TestRun_Repository_ReportsTheRoutesADecisionDoesNotCarryOnto fixes the
// routes no action reaches whose client-go method is decided in a way that
// does not hold for them, each a finding on v3.30.0 until the triage answers
// it. Six are decided COVERED_GENERIC, which says another route covers the
// capability, the question this scope asks: a group's and a project's direct
// members, a group's subgroups, the delete of an LDAP link by its cn, a merge
// request's changes and the PUT of the error tracking settings. The creation
// of a vulnerability from a finding is decided COVERED_GRAPHQL with its
// service, and no GraphQL action creates one, while the list beside it is
// inherited from vulnerability.list. The five Orbit routes GitLab.com
// removed are inherited SUPERSEDED_UPSTREAM, since GraphQL and the Orbit
// MCP endpoint replaced them. A runner's managers and a job request skip
// the fine-grained check for the reason the runner routes an action sends
// carry, which therefore excludes neither. The integrations list no action
// sends is offered through its deprecated /services/ spelling, which the
// finding names.
func TestRun_Repository_ReportsTheRoutesADecisionDoesNotCarryOnto(t *testing.T) {
	report := repositoryReport(t)
	findings := map[string]Route{}
	for _, route := range report.Findings {
		findings[route.Route] = route
	}
	cases := map[string]struct{ decisions, twin string }{
		"GET /groups/:id/members":                                     {decisions: "Groups.ListGroupMembers COVERED_GENERIC"},
		"GET /projects/:id/members":                                   {decisions: "ProjectMembers.ListProjectMembers COVERED_GENERIC"},
		"GET /groups/:id/subgroups":                                   {decisions: "Groups.ListSubGroups COVERED_GENERIC"},
		"DELETE /groups/:id/ldap_group_links/:cn":                     {decisions: "Groups.DeleteGroupLDAPLink COVERED_GENERIC"},
		"GET /projects/:id/merge_requests/:merge_request_iid/changes": {decisions: "MergeRequests.GetMergeRequestChanges COVERED_GENERIC"},
		"PUT /projects/:id/error_tracking/settings":                   {decisions: "ErrorTracking.CreateErrorTrackingSettings COVERED_GENERIC"},
		"POST /projects/:id/vulnerabilities":                          {decisions: "ProjectVulnerabilities.CreateVulnerability COVERED_GRAPHQL"},
		"DELETE /runners/managers":                                    {},
		"POST /jobs/request":                                          {},
		"GET /projects/:id/integrations":                              {twin: "GET /projects/:id/services"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			route, found := findings[name]
			var got []string
			for _, decision := range route.Adjudications {
				got = append(got, decision.Method+" "+decision.Category)
			}
			if !found || route.Class != ClassUnanswered || strings.Join(got, ", ") != want.decisions || route.Twin != want.twin {
				t.Errorf("%s = %+v (a finding: %t), want unanswered with the decisions %q and the twin %q", name, route, found, want.decisions, want.twin)
			}
		})
	}
	answered := map[string]Route{}
	for _, route := range report.Answered {
		answered[route.Route] = route
	}
	inherited := map[string]struct {
		category string
		actions  []string
	}{
		"GET /projects/:id/vulnerabilities": {category: adjudicationGraphQL, actions: []string{"vulnerability.list"}},
		"GET /orbit/status":                 {category: adjudicationSuperseded},
		"GET /orbit/schema":                 {category: adjudicationSuperseded},
		"GET /orbit/graph_status":           {category: adjudicationSuperseded},
		"GET /orbit/tools":                  {category: adjudicationSuperseded},
		"GET /orbit/schema/dsl":             {category: adjudicationSuperseded},
	}
	for name, want := range inherited {
		t.Run(name, func(t *testing.T) {
			route := answered[name]
			if route.Class != ClassInherited || len(route.Adjudications) != 1 || route.Adjudications[0].Category != want.category ||
				!slices.Equal(route.Adjudications[0].Actions, want.actions) {
				t.Errorf("%s = %+v, want it inherited from %s with the actions %q", name, route, want.category, want.actions)
			}
		})
	}
}

// TestRun_Repository_HoldsTheDuoWorkflowRoutesToTheTokenThatCallsThem fixes
// the routes that skip the fine-grained check for ai_workflows_oauth_auth or
// ai_workflows_token_auth. Their classes add the ai_workflows scope to the
// api scope every API class allows and only authenticate the user, so a
// user's token calls them and they are findings on v3.30.0 until the triage
// answers them; the three whose handler checks the Duo flow's composite
// identity are taken out by an exception that cites it.
func TestRun_Repository_HoldsTheDuoWorkflowRoutesToTheTokenThatCallsThem(t *testing.T) {
	byRoute := map[string]Route{}
	report := repositoryReport(t)
	for _, route := range slices.Concat(report.Findings, report.Answered) {
		byRoute[route.Route] = route
	}
	var composite, findings, other []string
	for name, route := range byRoute {
		switch {
		case !strings.HasPrefix(route.SkipReason, "ai_workflows_"):
		case route.Class == ClassSkip && route.Category == skipOtherCaller && strings.Contains(route.Reason, "verify_flow_composite_identity!"):
			composite = append(composite, name)
		case route.Class == ClassUnanswered:
			findings = append(findings, name)
		default:
			other = append(other, name+" "+route.Class)
		}
	}
	slices.Sort(composite)
	wantComposite := []string{
		"POST /ai/duo_workflows/code_review/add_comments",
		"POST /ai/duo_workflows/tools/risk_classification/results",
		"POST /projects/:id/merge_requests/:merge_request_iid/suggested_reviewers",
	}
	if !slices.Equal(composite, wantComposite) || len(findings) != 14 || len(other) != 0 {
		t.Errorf("composite identity = %v, %d findings a user's token calls and %v otherwise, want %v, 14 and none", composite, len(findings), other, wantComposite)
	}
}
