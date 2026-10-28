// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package params

import (
	"maps"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdk"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdkoptions"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
)

// newUse returns an empty use of a route sent with verb.
func newUse(verb string) *routeUse {
	return &routeUse{
		route: &apilive.Route{Method: verb}, actions: map[string]bool{}, sends: map[string]bool{},
		always: map[string]bool{}, types: map[string]bool{},
	}
}

// TestReadRaw_EachShapeOfOptions_IsReadOrNamedUnread verifies what a raw
// request's options contribute, one shape at a time: a client-go struct is
// walked with the encoder its constructor uses, a struct of the package's
// own walks the client-go struct it embeds and adds the keys beside it, no
// options carry nothing, and a free map, a client-go struct the SDK does not
// declare, a struct of the package's own that embeds none and a forwarder
// no call of which matches the action are each named unread with why.
func TestReadRaw_EachShapeOfOptions_IsReadOrNamedUnread(t *testing.T) {
	src := sources{sdk: sdkoptions.SDK{Types: map[string]sdkoptions.Type{
		"ListOptions": {Fields: []sdkoptions.Field{{GoName: "Page", JSON: sdkoptions.Tag{Name: "page_json"}, Query: sdkoptions.Tag{Name: "page"}}}},
	}}}
	cases := []struct {
		name      string
		raw       sdk.RawRequest
		verb      string
		wantSends []string
		reason    string
	}{
		{name: "a client-go struct read by its query", raw: sdk.RawRequest{Constructor: "NewRequest", Options: "*gitlab.ListOptions"}, verb: "GET", wantSends: []string{"page"}},
		{name: "a client-go struct read by its body", raw: sdk.RawRequest{Constructor: "NewRequestToURL", Options: "gitlab.ListOptions"}, verb: "POST", wantSends: []string{"page_json"}},
		{name: "no options", raw: sdk.RawRequest{Constructor: "NewRequest"}, verb: "GET"},
		{
			name: "a struct of the package's own embedding client-go's",
			raw: sdk.RawRequest{Constructor: "NewRequest", Options: "*widgets.body", Sends: []waits.Superset{
				{Type: "widgets.other", Root: "GoneOptions", Keys: []string{"never"}},
				{Type: "widgets.body", Root: "ListOptions", Keys: []string{"extra.key"}},
			}},
			verb: "GET", wantSends: []string{"extra[key]", "page"},
		},
		{name: "a free map", raw: sdk.RawRequest{Constructor: "NewRequest", Options: "map[string]any"}, verb: "PUT", reason: reasonFreeMap},
		{name: "a client-go struct the SDK does not declare", raw: sdk.RawRequest{Constructor: "NewRequest", Options: "*gitlab.GoneOptions"}, verb: "GET", reason: reasonUnknownStruct},
		{name: "a struct of the package's own embedding none", raw: sdk.RawRequest{Constructor: "NewRequest", Options: "orbit.mcpRequest"}, verb: "POST", reason: reasonOwnStruct},
		{
			name: "a forwarder no call of which matches",
			raw:  sdk.RawRequest{Constructor: "NewRequest", Options: "any", Callers: []sdk.RawCaller{{Site: "widgets.Other", Verb: "DELETE"}}},
			verb: "GET", reason: reasonNoCaller,
		},
		{
			name: "a forwarder called with a verb that is no constant",
			raw:  sdk.RawRequest{Constructor: "UploadRequest", Options: "any", Callers: []sdk.RawCaller{{Site: "widgets.Other", Options: "*gitlab.ListOptions"}}},
			verb: "POST", wantSends: []string{"page"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			use := newUse(tc.verb)
			tc.raw.Site = "widgets.site"
			problems := src.readRaw(use, actionrequests.RecordAction{Handlers: []string{"widgets.Handler"}}, tc.raw, tc.verb)
			if got := slices.Sorted(maps.Keys(use.sends)); !slices.Equal(got, tc.wantSends) {
				t.Errorf("sends = %v, want %v", got, tc.wantSends)
			}
			var reasons []string
			for _, problem := range problems {
				reasons = append(reasons, problem.Reason)
			}
			if want := slices.DeleteFunc([]string{tc.reason}, func(r string) bool { return r == "" }); !slices.Equal(reasons, want) {
				t.Errorf("unread reasons = %v, want %v", reasons, want)
			}
		})
	}
}

// TestCallersFor_TheActionsOwnCall_IsPreferredToTheVerb verifies which calls
// of a forwarding function an action is held to: the ones its handlers are,
// and only where none is, the ones whose verb is the route's or is no
// constant.
func TestCallersFor_TheActionsOwnCall_IsPreferredToTheVerb(t *testing.T) {
	callers := []sdk.RawCaller{
		{Site: "projects.AddHook", Verb: "POST"},
		{Site: "projects.CreateForkRelation", Verb: "POST"},
		{Site: "projects.ListHooks", Verb: "GET"},
		{Site: "projects.Forward"},
	}
	cases := []struct {
		name     string
		handlers []string
		verb     string
		want     []string
	}{
		{name: "the handler's own call", handlers: []string{"projects.AddHook"}, verb: "POST", want: []string{"projects.AddHook"}},
		{name: "by the verb", handlers: []string{"projects.Elsewhere"}, verb: "post", want: []string{"projects.CreateForkRelation", "projects.AddHook", "projects.Forward"}},
		{name: "by a verb no call names", verb: "DELETE", want: []string{"projects.Forward"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, caller := range callersFor(callers, tc.handlers, tc.verb) {
				got = append(got, caller.Site)
			}
			slices.Sort(got)
			slices.Sort(tc.want)
			if !slices.Equal(got, tc.want) {
				t.Errorf("callersFor = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestReachedRoutes_ADerivedSpelling_MeetsEveryRouteItCanSend verifies the
// routes one request reaches: the one the record places it on, and for a
// derived spelling every route of its verb that spelling meets, a
// placeholder of the derived spelling standing for any one segment and a
// literal of it for itself alone, through each spelling of a route's
// optional groups.
func TestReachedRoutes_ADerivedSpelling_MeetsEveryRouteItCanSend(t *testing.T) {
	routes := routesByName([]apilive.Route{
		{Method: "PUT", Path: apilive.EndpointPrefix + "/projects/:id/integrations/slack"},
		{Method: "PUT", Path: apilive.EndpointPrefix + "/projects/:id/integrations/jira"},
		{Method: "PUT", Path: apilive.EndpointPrefix + "/projects/:id/(-/)integrations/teams"},
		{Method: "GET", Path: apilive.EndpointPrefix + "/projects/:id/integrations/slack"},
		{Method: "PUT", Path: apilive.EndpointPrefix + "/projects/:id/services/slack"},
		{Method: "PUT", Path: apilive.EndpointPrefix + "/projects/:id/integrations/slack/extra"},
	})
	request := actionrequests.RecordRequest{Kind: actionrequests.KindREST, Route: "PUT /projects/:id/integrations/slack", Derived: "PUT /projects/:/integrations/:"}
	want := []string{
		"PUT /projects/:id/(-/)integrations/teams", "PUT /projects/:id/integrations/jira", "PUT /projects/:id/integrations/slack",
	}
	if got := reachedRoutes(request, routes); !slices.Equal(got, want) {
		t.Errorf("reachedRoutes = %v, want %v", got, want)
	}
	if got := reachedRoutes(actionrequests.RecordRequest{Route: "GET /absent"}, routes); got != nil {
		t.Errorf("reachedRoutes(a route the record does not hold) = %v, want none", got)
	}
}

// TestConstructorOf_NewRequestToURL_EncodesAsNewRequest verifies the one
// constructor rename: NewRequestToURL builds its request the way NewRequest
// does, and every other name passes through.
func TestConstructorOf_NewRequestToURL_EncodesAsNewRequest(t *testing.T) {
	if got := constructorOf(newRequestToURL); got != sdkroutes.ConstructorNewRequest {
		t.Errorf("constructorOf(NewRequestToURL) = %q, want NewRequest", got)
	}
	if got := constructorOf(sdkroutes.ConstructorUpload); got != sdkroutes.ConstructorUpload {
		t.Errorf("constructorOf(UploadRequest) = %q, want it unchanged", got)
	}
}

// TestDottedParam_AKeyBesideClientGo_IsSpelledAsTheRecordDeclaresIt verifies
// the spelling a superset's dotted key is matched under.
func TestDottedParam_AKeyBesideClientGo_IsSpelledAsTheRecordDeclaresIt(t *testing.T) {
	cases := map[string]string{"variables.raw": "variables[raw]", "invite_source": "invite_source", "a.b.c": "a[b][c]"}
	for key, want := range cases {
		t.Run(key, func(t *testing.T) {
			if got := dottedParam(key); got != want {
				t.Errorf("dottedParam(%q) = %q, want %q", key, got, want)
			}
		})
	}
}

// TestRead_ASiteThatBuildsNoRawRequest_IsUnreadUnlessItFollowsARedirect
// verifies a request through a site the walk found no raw request in: named
// unread, so its route's unoffered params are unjudged, except when the
// request is the redirect GitLab answers another route with, which the HTTP
// client follows with no options of the action's.
func TestRead_ASiteThatBuildsNoRawRequest_IsUnreadUnlessItFollowsARedirect(t *testing.T) {
	cases := []struct {
		name        string
		declaration string
		want        []string
	}{
		{name: "a site of its own", want: []string{reasonNoRawRequest}},
		{name: "a redirect followed", declaration: declarationFollowsRedirect},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := actionrequests.RecordRequest{Kind: actionrequests.KindREST, Sites: []string{"releases.GetLatest"}, Declaration: tc.declaration}
			var reasons []string
			for _, problem := range (sources{}).read(newUse("GET"), actionrequests.RecordAction{}, request) {
				reasons = append(reasons, problem.Reason)
			}
			if !slices.Equal(reasons, tc.want) {
				t.Errorf("unread reasons = %v, want %v", reasons, tc.want)
			}
		})
	}
}

// TestRead_AnUploadOnEveryCall_SendsItsFileAlways verifies the file field
// of a method that uploads on every call is carried always, and that of a
// method that uploads only when a file is given only when one is, without
// taking back the field another method of the request always sends.
func TestRead_AnUploadOnEveryCall_SendsItsFileAlways(t *testing.T) {
	src := sources{sdk: sdkoptions.SDK{Methods: map[string]sdkoptions.Method{
		"Svc.Always":    {Constructors: []string{sdkroutes.ConstructorUpload}, UploadFields: []string{"file"}},
		"Svc.Sometimes": {Constructors: []string{sdkroutes.ConstructorNewRequest, sdkroutes.ConstructorUpload}, UploadFields: []string{"avatar", "file"}},
	}}}
	use := newUse("POST")
	request := actionrequests.RecordRequest{Kind: actionrequests.KindREST, SDKMethods: []string{"Svc.Always", "Svc.Sometimes"}}
	if problems := src.read(use, actionrequests.RecordAction{}, request); problems != nil {
		t.Fatalf("read = %v, want no unread request", problems)
	}
	if !use.sends["file"] || !use.sends["avatar"] || !use.always["file"] || use.always["avatar"] {
		t.Errorf("sends = %v, always = %v; want both sent and the file always", use.sends, use.always)
	}
}
