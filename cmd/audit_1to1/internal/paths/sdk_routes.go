// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package paths

import (
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
)

// sdkRoute is one endpoint a client-go service method reaches.
type sdkRoute struct {
	// Method is the HTTP verb, uppercase.
	Method string
	// Path is the endpoint with every identifier segment collapsed to the
	// placeholder, which is the spelling [pathShape] produces and the only one
	// in which our routes and GitLab's document meet.
	Path string
	// Many is true for a method answering with a slice of the type. It changes
	// no comparison, because GitLab's document describes a collection by the
	// element it returns rather than by the array around it, and it is
	// reported so a reader of a finding can see that the names searched are an
	// element's.
	Many bool
	// Entity is the entity the method's answer is presented with on a route
	// GitLab annotates with none, which [scopedEntities] declares for a search
	// method from the scope it sends. Empty for every other method, whose
	// route the record speaks for.
	Entity string
}

// operation spells the route the way a finding names it, with the entity a
// declaration carries for it, since the record names none.
func (r sdkRoute) operation() string {
	spelled := r.Method + " " + r.Path
	if r.Many {
		spelled += " (collection)"
	}
	if r.Entity != "" {
		spelled += " as " + r.Entity
	}
	return spelled
}

// readSDKRoutes returns, for every struct a client-go service method answers
// with, the endpoints those methods reach.
//
// A directory that cannot be read, or a source with no route in it, yields
// nothing rather than failing the scope: this reads a module cache it does not
// own the state of, and the join it feeds reports rather than gates.
//
// A method declared misdecoded is left out: client-go decodes its answer into
// a struct GitLab does not send, so crediting its routes to that struct would
// hold every type modeling the struct to an answer it never decodes.
func readSDKRoutes(dir string) map[string][]sdkRoute {
	return routesBy(sdkroutes.Read(dir), func(method sdkroutes.Method) []string {
		if method.Answers == "" || misdecoded(method.Key()) {
			return nil
		}
		return []string{method.Answers}
	})
}

// readSDKMethodRoutes returns, for every client-go service method, the
// endpoints it reaches, keyed "Service.Method" with the service spelled the way
// shared.ServiceName spells the interface a handler calls it through: the
// concrete MilestonesService behind MilestonesServiceInterface is
// "Milestones". Every service interface client-go declares has its concrete
// struct under that name, which is the convention the join rests on.
//
// It is the per-method view of what [readSDKRoutes] unions per struct, for
// the projections a handler builds out of one method's answer: a milestone's
// issue list is filled from GET /projects/:id/milestones/:milestone_id/issues
// and from nothing else client-go's Issue is answered by, and judging it
// against all of them would hold six fields of a row to twenty endpoints it is
// never read from. A method answering with nothing but the pagination wrapper
// is listed too, since its routes are a fact about the method whatever it
// returns.
func readSDKMethodRoutes(dir string) map[string][]sdkRoute {
	return routesBy(sdkroutes.Read(dir), func(method sdkroutes.Method) []string {
		return []string{method.Key()}
	})
}

// routesBy indexes every method's routes under the keys keysOf names for it,
// each with the method's Many flag and the entity a scope declaration names
// for the method.
//
// The reading is [sdkroutes]', the one the action request derivation reads
// too: it follows a method into the helper, the field and the generic function
// it delegates to, carries the constants it hands them into the route
// template, and folds a path the legacy request form builds with fmt.Sprintf.
// R-PATH used to read the source itself, method body by method body, and that
// reading lost every route a method reaches through a helper and merged every
// collection a helper is handed into one placeholder; two readers of one fact
// are how a check ends up answering a narrower question than the one it
// prints.
func routesBy(sdk *sdkroutes.SDK, keysOf func(sdkroutes.Method) []string) map[string][]sdkRoute {
	found := map[string]map[sdkRoute]bool{}
	for _, method := range sdk.Methods() {
		entity := scopedEntityOf(method.Key())
		for _, key := range keysOf(method) {
			for _, route := range method.Routes {
				addRoute(found, key, sdkRoute{Method: route.Method, Path: route.Path, Many: method.Many, Entity: entity})
			}
		}
	}
	if len(found) == 0 {
		return nil
	}
	return sortedRoutes(found)
}

// addRoute records one route under one key.
func addRoute(into map[string]map[sdkRoute]bool, key string, route sdkRoute) {
	known := into[key]
	if known == nil {
		known = map[sdkRoute]bool{}
		into[key] = known
	}
	known[route] = true
}

// sortedRoutes flattens the collected set into a stable slice per key.
func sortedRoutes(found map[string]map[sdkRoute]bool) map[string][]sdkRoute {
	out := make(map[string][]sdkRoute, len(found))
	for name, routes := range found {
		flat := make([]sdkRoute, 0, len(routes))
		for route := range routes {
			flat = append(flat, route)
		}
		slices.SortFunc(flat, func(a, b sdkRoute) int { return strings.Compare(a.operation(), b.operation()) })
		out[name] = flat
	}
	return out
}
