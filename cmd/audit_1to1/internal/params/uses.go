// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package params

import (
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdk"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdkoptions"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
)

const (
	// clientGoPackage is the name client-go's root package declares, which
	// is how the raw request walk spells one of its types.
	clientGoPackage = "gitlab"
	// newRequestToURL is the third request constructor, which encodes its
	// options as NewRequest does.
	newRequestToURL = "NewRequestToURL"
	// mapPrefix opens the spelling of a map type.
	mapPrefix = "map["
	// declarationFollowsRedirect is the category the request record gives a
	// request that is GitLab's redirect to another of its routes, which the
	// HTTP client follows (cmd/gen_action_grants, request_declarations.go).
	declarationFollowsRedirect = "follows-redirect"
)

// The reasons a request's options cannot be read, which make an unoffered
// param of its route unjudged.
const (
	reasonUnknownMethod = "client-go declares no such method"
	reasonNoRawRequest  = "the site builds no raw request the walk found"
	reasonNoSite        = "the request names neither a client-go method nor a site"
	reasonNoCaller      = "no call of the function that forwards its options matches the action"
	reasonFreeMap       = "the options are a free map"
	reasonUnknownStruct = "client-go declares no such struct"
	reasonOwnStruct     = "a struct of the tool package's own that embeds no client-go struct"
)

// routeUse is everything that reaches one route: the actions whose requests
// send it, the params an option struct those requests carry can write, and
// the structs walked.
type routeUse struct {
	route   *apilive.Route
	actions map[string]bool
	sends   map[string]bool
	// always are the params a request of the route carries on every call
	// whatever its options hold: the form field an upload that always
	// uploads sends its file under, which the inventory does not record.
	always map[string]bool
	types  map[string]bool
	// unread is set when one of the requests carries options the walk could
	// not read.
	unread bool
}

// sources is what a request's options are read from: client-go's option
// structs and methods, and the raw requests of the tool packages by site.
type sources struct {
	sdk sdkoptions.SDK
	raw map[string][]sdk.RawRequest
}

// newSources indexes the raw requests by the site that builds them, a site
// that builds several keeping each.
func newSources(options sdkoptions.SDK, raw []sdk.RawRequest) sources {
	bySite := map[string][]sdk.RawRequest{}
	for _, request := range raw {
		bySite[request.Site] = append(bySite[request.Site], request)
	}
	return sources{sdk: options, raw: bySite}
}

// collectUses folds every REST request of the record onto the routes it
// sends, with what its options can carry, and lists the requests whose
// options it could not read.
func collectUses(record actionrequests.Record, routes map[string]*apilive.Route, src sources) (map[string]*routeUse, []UnreadRequest) {
	uses := map[string]*routeUse{}
	var unread []UnreadRequest
	for _, action := range record.Actions {
		for _, request := range action.Requests {
			if request.Kind != actionrequests.KindREST {
				continue
			}
			for _, name := range reachedRoutes(request, routes) {
				use := uses[name]
				if use == nil {
					use = &routeUse{
						route: routes[name], actions: map[string]bool{}, sends: map[string]bool{},
						always: map[string]bool{}, types: map[string]bool{},
					}
					uses[name] = use
				}
				use.actions[action.ID] = true
				for _, problem := range src.read(use, action, request) {
					problem.Route, problem.Action = name, action.ID
					unread = append(unread, problem)
					use.unread = true
				}
			}
		}
	}
	return uses, unread
}

// reachedRoutes names the record's routes one request reaches: the route the
// request record places it on, and, for a request whose derived spelling
// leaves a segment to the input (`PUT /projects/:/integrations/:`, one route
// per integration slug the caller names), every route of the record that
// spelling meets. The request record places such a request on the first of
// them alone.
func reachedRoutes(request actionrequests.RecordRequest, routes map[string]*apilive.Route) []string {
	var names []string
	if _, held := routes[request.Route]; held {
		names = append(names, request.Route)
	}
	verb, derived, spelled := strings.Cut(request.Derived, " ")
	if !spelled {
		return names
	}
	want := apilive.PathSegments(derived)
	for name, route := range routes {
		if route.Method != verb || name == request.Route {
			continue
		}
		for _, variant := range apilive.ExpandOptional(apilive.NormalizePath(route.Path)) {
			if derivedMeets(want, apilive.PathSegments(variant)) {
				names = append(names, name)
				break
			}
		}
	}
	slices.Sort(names)
	return names
}

// derivedMeets reports whether a derived spelling meets a record spelling:
// segment by segment the same, a placeholder of the derived one standing for
// any segment.
func derivedMeets(derived, record []string) bool {
	if len(derived) != len(record) {
		return false
	}
	for i := range derived {
		if derived[i] != ":" && derived[i] != record[i] {
			return false
		}
	}
	return true
}

// read adds to use what one request's options can carry and returns what it
// could not read: through each client-go method it names, or, for a request
// that names none, through the raw request each of its sites builds. A
// request that is the redirect GitLab answers another route with carries
// nothing of the action's: the HTTP client sends it to the location GitLab
// names, with no options of its own.
func (s sources) read(use *routeUse, action actionrequests.RecordAction, request actionrequests.RecordRequest) []UnreadRequest {
	verb := use.route.Method
	if len(request.SDKMethods) > 0 {
		var problems []UnreadRequest
		for _, key := range request.SDKMethods {
			method, known := s.sdk.Methods[key]
			if !known {
				problems = append(problems, UnreadRequest{Site: key, Reason: reasonUnknownMethod})
				continue
			}
			for _, name := range method.Options {
				s.walk(use, name, sdkoptions.Encoders(method.Constructors, verb))
			}
			// An upload's file goes under its own form field, which no
			// field of the options carries, and a method that uploads on
			// every call sends it on every call.
			always := slices.Equal(method.Constructors, []string{sdkroutes.ConstructorUpload})
			for _, field := range method.UploadFields {
				use.sends[field] = true
				use.always[field] = use.always[field] || always
			}
		}
		return problems
	}
	if request.Declaration == declarationFollowsRedirect {
		return nil
	}
	if len(request.Sites) == 0 {
		return []UnreadRequest{{Reason: reasonNoSite}}
	}
	var problems []UnreadRequest
	for _, site := range request.Sites {
		built := s.raw[site]
		if len(built) == 0 {
			problems = append(problems, UnreadRequest{Site: site, Reason: reasonNoRawRequest})
		}
		for _, raw := range built {
			problems = append(problems, s.readRaw(use, action, raw, verb)...)
		}
	}
	return problems
}

// readRaw adds what a raw request's options can carry: its own, or, for a
// function that forwards its options, what the calls of it the action makes
// hand it.
func (s sources) readRaw(use *routeUse, action actionrequests.RecordAction, raw sdk.RawRequest, verb string) []UnreadRequest {
	if raw.UploadField != "" {
		use.sends[raw.UploadField] = true
		use.always[raw.UploadField] = true
	}
	encoders := sdkoptions.Encoders([]string{constructorOf(raw.Constructor)}, verb)
	if len(raw.Callers) == 0 {
		return s.readOptions(use, raw, raw.Options, encoders)
	}
	callers := callersFor(raw.Callers, action.Handlers, verb)
	if len(callers) == 0 {
		return []UnreadRequest{{Site: raw.Site, Reason: reasonNoCaller}}
	}
	var problems []UnreadRequest
	for _, caller := range callers {
		problems = append(problems, s.readOptions(use, raw, caller.Options, encoders)...)
	}
	return problems
}

// readOptions adds what options of the spelled type carry: a client-go
// struct walked, or a struct of the package's own that embeds one, walked
// with the keys it carries beside it. No options carry nothing.
func (s sources) readOptions(use *routeUse, raw sdk.RawRequest, spelled string, encoders []string) []UnreadRequest {
	if spelled == "" {
		return nil
	}
	problem := UnreadRequest{Site: raw.Site, Options: spelled}
	if strings.HasPrefix(spelled, mapPrefix) {
		problem.Reason = reasonFreeMap
		return []UnreadRequest{problem}
	}
	named := strings.TrimPrefix(spelled, "*")
	if name, isClientGo := strings.CutPrefix(named, clientGoPackage+"."); isClientGo {
		if _, declared := s.sdk.Types[name]; !declared {
			problem.Reason = reasonUnknownStruct
			return []UnreadRequest{problem}
		}
		s.walk(use, name, encoders)
		return nil
	}
	for _, superset := range raw.Sends {
		if superset.Type != named {
			continue
		}
		s.walk(use, superset.Root, encoders)
		for _, key := range superset.Keys {
			use.sends[dottedParam(key)] = true
		}
		return nil
	}
	problem.Reason = reasonOwnStruct
	return []UnreadRequest{problem}
}

// walk adds every param the struct name carries under each encoder.
func (s sources) walk(use *routeUse, name string, encoders []string) {
	use.types[name] = true
	for _, encoder := range encoders {
		sdkoptions.Walk(s.sdk.Types, name, encoder, func(found sdkoptions.Param) {
			use.sends[sdkoptions.Canonical(found.Param)] = true
		})
	}
}

// callersFor picks the calls of a forwarding function an action makes: the
// ones its handlers are, and, where none is, the ones whose verb is the
// route's or not a constant.
func callersFor(callers []sdk.RawCaller, handlers []string, verb string) []sdk.RawCaller {
	var byHandler, byVerb []sdk.RawCaller
	for _, caller := range callers {
		if slices.Contains(handlers, caller.Site) {
			byHandler = append(byHandler, caller)
		}
		if caller.Verb == "" || strings.EqualFold(caller.Verb, verb) {
			byVerb = append(byVerb, caller)
		}
	}
	if len(byHandler) > 0 {
		return byHandler
	}
	return byVerb
}

// constructorOf maps a raw request's constructor onto the two
// [sdkoptions.Encoders] reads, NewRequestToURL encoding as NewRequest does.
func constructorOf(name string) string {
	if name == newRequestToURL {
		return sdkroutes.ConstructorNewRequest
	}
	return name
}

// dottedParam spells a key a superset carries, dotted below a field that
// holds a struct ("variables.raw"), the way the record declares a nested
// param ("variables[raw]").
func dottedParam(key string) string {
	head, rest, nested := strings.Cut(key, ".")
	if !nested {
		return key
	}
	return head + "[" + strings.ReplaceAll(rest, ".", "][") + "]"
}
