// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package paths

import (
	"cmp"
	"path"
	"reflect"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// KeysetPagination is the keyset half of R-PAGE (issue 1165): whether an
// action that offers page_token sends it to a route GitLab can page by
// keyset, and whether a list GitLab can page by keyset offers it, publishes
// the way to its next page and was seen following one.
//
// The collection half ([PaginationCheck]) asks whether a list says where it
// ends, and a keyset page says so in the Link header, which is why a keyset
// route used to raise no finding at all:
// the record's params could not tell which routes GitLab pages by keyset,
// since GitLab reads pagination=keyset without declaring it. Schema version 5
// of the live record says where each route's code reaches a keyset paginator
// ([apilive.Route.Keyset]), and that is the oracle here.
//
// # The join, and its two grains
//
// The routes an action sends are read from the request record the handlers
// were derived into (docs/development/action-requests.json), so the route
// half of this check is at the action's own grain, finer than the collection
// half's: two actions of one package are never held to each other's routes. Whether a page was followed is read from the request inventory,
// which records the package whose test sent a request and every query name
// it was seen sending, so that half is at the package grain: a route counts
// as followed when a package serving the action was recorded sending it one
// of the parameters its next page is asked by. [KeysetPagination.Grain] says
// so beside the findings.
//
// # Where the record is not the answer
//
// A route reaching a keyset paginator is where GitLab can serve a keyset
// page, not a promise that it serves this server one: paginate_with_strategies
// answers 405 to a model it has no keyset ordering for, a helper can take its
// keyset branch only for a caller this server never is, and a route can write
// a next link it never reads back. Those routes are declared, one entry per
// route, in [declaredKeysetRoutes], with what GitLab's handler does read from
// its source, and are held to as routes this server cannot page by keyset.
// A declaration that no finding uses is reported stale, and that, as for
// every declaration table here, is the half of this check that gates.
//
// A site behind a feature flag GitLab ships off counts as one this server can
// page by keyset: an instance that turns the flag on serves the route a
// keyset page, as GitLab.com does the commit list behind
// commits_keyset_pagination, and the action's page_token is what reaches it
// there. An instance that leaves the flag off ignores the parameter, which
// costs a caller nothing.
//
// # It gates on what no declaration answers
//
// Against the 19.5.0 record it once reported 182 actions offering page_token
// on a route GitLab pages by offset alone, which an action published whenever
// its input embedded [toolutil.KeysetPaginationInput], whatever its route did.
// With the keyset input taken off those inputs (issue 1165), every undeclared
// finding is a change the surface owes, so [Summary.clean] counts them: an
// action that offers page_token where no route pages by keyset, or a keyset
// list that does not offer it, fails the run until its input changes or a
// route declaration says why it cannot.
type KeysetPagination struct {
	// Ran is false when the request record could not be read, which is the
	// one input this half reads that the collection half does not.
	Ran bool `json:"ran"`
	// Requests names the request record the routes were read from.
	Requests string `json:"requests,omitempty"`
	// Grain is the two-grain caveat, spelled on the check like the
	// collection half's.
	Grain string `json:"grain,omitempty"`
	// Offering counts the actions whose input offers page_token, and
	// OnKeyset how many of them send a route this server can page by keyset.
	Offering int `json:"actions_offering_page_token"`
	OnKeyset int `json:"actions_offering_it_on_keyset_routes"`
	// Unrecorded names the actions the catalog holds and the request record
	// does not, which nothing here can judge: a request record older than
	// the catalog, and `make gen-action-grants` refreshes it.
	Unrecorded []string `json:"unrecorded_actions,omitempty"`
	// Findings are the actions this half has something to say about, by
	// package, action, problem and route.
	Findings []KeysetFinding `json:"findings,omitempty"`
	// UnusedDeclarations names the route declarations that no finding used.
	UnusedDeclarations []string `json:"unused_declarations,omitempty"`
}

// The problems a keyset finding names.
const (
	// keysetOnOffsetRoute is an action offering page_token where no route it
	// sends can be paged by keyset, so GitLab ignores the parameter, or
	// refuses it, and a caller is offered a way to page that does not exist.
	keysetOnOffsetRoute = "page-token-on-offset-route"
	// keysetNotOffered is a list sending a route GitLab can page by keyset
	// that does not offer page_token, so a caller can never ask for the
	// cheaper of GitLab's two ways through it.
	keysetNotOffered = "keyset-route-without-page-token"
	// keysetNoNextPageToken is an action offering page_token on a keyset
	// route whose output carries no next_page_token, so a caller who asks for
	// a keyset page is never told how to ask for the next one, which is what
	// issue 1165 was opened about.
	keysetNoNextPageToken = "keyset-route-without-next-page-token" //#nosec G101 -- the name of a problem, not a credential
	// keysetNeverFollowed is an action offering page_token on a keyset route
	// that no test was recorded following: the package that serves it never
	// sent the route the parameter its next page is asked by.
	keysetNeverFollowed = "keyset-route-never-followed"
)

// KeysetFinding is one problem of one action.
type KeysetFinding struct {
	// Action is the canonical catalog ID, and Package its owner as the
	// inventory spells a package.
	Action  string `json:"action"`
	Package string `json:"package"`
	// Problem is one of the four this half names.
	Problem string `json:"problem"`
	// Routes are the routes the problem is about, as the request record
	// spells them: every REST route the action sends for an offset finding,
	// and the one keyset route for each of the others.
	Routes []string `json:"routes,omitempty"`
	// Cursors are the parameters the route's next page is asked by, as the
	// record names them.
	Cursors []string `json:"cursors,omitempty"`
	// NotFollowable names the routes among Routes that the record marks as
	// reaching a keyset paginator and a declaration says this server cannot
	// page by keyset, which is why an action sending one is on the offset
	// list rather than held to its next_page_token.
	NotFollowable []string `json:"not_followable,omitempty"`
	// Category and Reason are the declaration of the route accounting for a
	// list that does not offer page_token, empty for a finding nothing
	// accounts for.
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// declared reports whether a declaration accounts for this finding.
func (f KeysetFinding) declared() bool { return f.Category != "" }

// keysetGrain is what [KeysetPagination.Grain] says, spelled once and as one
// literal for the reason [paginationGrain] gives.
const keysetGrain = "action for the routes, read from the request record the handlers were derived into; package for whether a page was followed: a route counts as followed when the inventory shows a package serving the action sending it one of the parameters its next page is asked by."

// pageTokenParameter is the input property this half is about, the one
// [toolutil.KeysetPaginationInput] publishes.
const pageTokenParameter = "page_token"

// nextPageTokenBlock is the pagination block that carries next_page_token,
// named as a type for the reason [toolutil.CollectionShapeOf] names its blocks
// as types: a rename breaks the build rather than emptying the check.
var nextPageTokenBlock = reflect.TypeFor[toolutil.PaginationOutput]().Name()

// offersPageToken reports whether an action's input schema offers page_token.
func offersPageToken(schema map[string]any) bool {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	_, offered := properties[pageTokenParameter]
	return offered
}

// publishesNextPageToken reports whether an action's output carries the
// block a keyset page's next_page_token is published in.
func publishesNextPageToken(outputType reflect.Type) bool {
	return toolutil.CollectionShapeOf(outputType).Pagination == nextPageTokenBlock
}

// keysetCursors maps each route the record marks as reaching a keyset
// paginator, as the request record spells a route, to the parameters its next
// page is asked by. A route mounted twice is one route here, with the
// parameters of both.
func keysetCursors(record apilive.Document) map[string][]string {
	cursors := map[string][]string{}
	for index := range record.Routes {
		route := &record.Routes[index]
		if !route.PagesByKeyset() {
			continue
		}
		name := apilive.RouteName(route)
		merged := slices.Concat(cursors[name], route.Cursors())
		slices.Sort(merged)
		cursors[name] = slices.Compact(merged)
	}
	return cursors
}

// sentRoute is what the request record says one action sends.
type sentRoute struct {
	// routes are its REST routes, sorted and once each, whatever their class:
	// a route the action sends on some calls only is still one a page_token
	// can reach. A GraphQL document and a request the derivation could not
	// read carry no route.
	routes []string
	// packages are the Go packages of its handlers and of every site that
	// builds one of its requests, by name, which is how the inventory's
	// package is matched to it.
	packages []string
}

// sentRoutes reads, per action, the REST routes it sends and the packages
// that send them.
func sentRoutes(record actionrequests.Record) map[string]sentRoute {
	sent := make(map[string]sentRoute, len(record.Actions))
	for _, action := range record.Actions {
		var routes []string
		packages := symbolPackages(action.Handlers)
		for _, request := range action.Requests {
			if request.Kind != actionrequests.KindREST {
				continue
			}
			routes = append(routes, request.Route)
			packages = append(packages, symbolPackages(request.Sites)...)
		}
		slices.Sort(routes)
		slices.Sort(packages)
		sent[action.ID] = sentRoute{routes: slices.Compact(routes), packages: slices.Compact(packages)}
	}
	return sent
}

// symbolPackages names the Go package of each symbol the request record
// qualifies by its package ("branches.List" is in branches).
func symbolPackages(symbols []string) []string {
	packages := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		name, _, _ := strings.Cut(symbol, ".")
		packages = append(packages, name)
	}
	return packages
}

// followedRoutes indexes the inventory by package name and route: which query
// names each package was seen sending each route of the record. A row the
// record holds no route for says nothing.
func followedRoutes(record apilive.Document, rows []requestinventory.Row) map[string]map[string]map[string]bool {
	index := apilive.NewRouteIndex(record.Routes)
	followed := map[string]map[string]map[string]bool{}
	for _, row := range rows {
		if !strings.EqualFold(row.Kind, requestinventory.KindREST) {
			continue
		}
		route, found := index.Match(row.Method, row.Path)
		if !found {
			continue
		}
		pkg := path.Base(row.Package)
		if followed[pkg] == nil {
			followed[pkg] = map[string]map[string]bool{}
		}
		name := apilive.RouteName(route)
		if followed[pkg][name] == nil {
			followed[pkg][name] = map[string]bool{}
		}
		for _, query := range row.Query {
			followed[pkg][name][query] = true
		}
	}
	return followed
}

// keysetInputs is everything one judgment reads, gathered once.
type keysetInputs struct {
	cursors  map[string][]string
	sent     map[string]sentRoute
	followed map[string]map[string]map[string]bool
}

// wasFollowed reports whether any package serving an action was seen sending
// a route one of the parameters its next page is asked by.
func (in keysetInputs) wasFollowed(packages []string, route string) bool {
	for _, pkg := range packages {
		for _, cursor := range in.cursors[route] {
			if in.followed[pkg][route][cursor] {
				return true
			}
		}
	}
	return false
}

// keysetPagination asks, of every catalog action, the keyset questions its
// input and its routes raise.
func keysetPagination(root string, record apilive.Document, rows []requestinventory.Row, actions []requestinventory.Action) KeysetPagination {
	derived, err := actionrequests.ReadRecord(root)
	if err != nil {
		return KeysetPagination{Ran: false}
	}
	inputs := keysetInputs{
		cursors:  keysetCursors(record),
		sent:     sentRoutes(derived),
		followed: followedRoutes(record, rows),
	}
	check := KeysetPagination{Ran: true, Requests: actionrequests.RecordPath, Grain: keysetGrain, Findings: []KeysetFinding{}}
	for _, action := range actions {
		check.judge(action, inputs)
	}
	slices.SortFunc(check.Findings, compareKeysetFindings)
	check.UnusedDeclarations = unusedKeysetDeclarations(check.Findings)
	return check
}

// judge holds one action to the routes it sends.
func (c *KeysetPagination) judge(action requestinventory.Action, in keysetInputs) {
	offered := offersPageToken(action.Route.InputSchema)
	if offered {
		c.Offering++
	}
	sent, recorded := in.sent[action.ID]
	if !recorded {
		if offered {
			c.Unrecorded = append(c.Unrecorded, action.ID)
		}
		return
	}
	pkg := requestinventory.PackageName(action.Owner)
	var followable, notFollowable []string
	for _, route := range sent.routes {
		if _, keyset := in.cursors[route]; !keyset {
			continue
		}
		if _, declared := declaredKeysetRoute(route); declared {
			notFollowable = append(notFollowable, route)
			continue
		}
		followable = append(followable, route)
	}

	if !offered {
		c.judgeNotOffered(action, pkg, slices.Concat(followable, notFollowable), in)
		return
	}
	if len(followable) == 0 {
		c.Findings = append(c.Findings, KeysetFinding{
			Action: action.ID, Package: pkg, Problem: keysetOnOffsetRoute, Routes: sent.routes, NotFollowable: notFollowable,
		})
		return
	}
	c.OnKeyset++
	packages := append(slices.Clone(sent.packages), path.Base(pkg))
	for _, route := range followable {
		if !publishesNextPageToken(action.Route.OutputType) {
			c.Findings = append(c.Findings, KeysetFinding{
				Action: action.ID, Package: pkg, Problem: keysetNoNextPageToken, Routes: []string{route}, Cursors: in.cursors[route],
			})
		}
		if !in.wasFollowed(packages, route) {
			c.Findings = append(c.Findings, KeysetFinding{
				Action: action.ID, Package: pkg, Problem: keysetNeverFollowed, Routes: []string{route}, Cursors: in.cursors[route],
			})
		}
	}
}

// judgeNotOffered reports a list that sends a keyset route without offering
// page_token, once per route, with the route's declaration when one says this
// server cannot page it by keyset.
//
// Only a read whose output is a collection envelope is asked: an action
// sending a keyset route to find one object by name, or to check that it is
// there, reads no list a caller could page through.
func (c *KeysetPagination) judgeNotOffered(action requestinventory.Action, pkg string, routes []string, in keysetInputs) {
	if !action.ReadOnly || !toolutil.CollectionShapeOf(action.Route.OutputType).IsCollection() {
		return
	}
	slices.Sort(routes)
	for _, route := range routes {
		finding := KeysetFinding{
			Action: action.ID, Package: pkg, Problem: keysetNotOffered, Routes: []string{route}, Cursors: in.cursors[route],
		}
		if declaration, declared := declaredKeysetRoute(route); declared {
			finding.Category = declaration.Category
			finding.Reason = declaration.Reason
		}
		c.Findings = append(c.Findings, finding)
	}
}

// compareKeysetFindings orders the findings the way a reader works through
// them: a package at a time, then an action, then its problems and routes.
func compareKeysetFindings(a, b KeysetFinding) int {
	return cmp.Or(
		strings.Compare(a.Package, b.Package),
		strings.Compare(a.Action, b.Action),
		strings.Compare(a.Problem, b.Problem),
		slices.Compare(a.Routes, b.Routes),
	)
}

// undeclared counts the findings no declaration accounts for.
func (c *KeysetPagination) undeclared() int {
	count := 0
	for _, finding := range c.Findings {
		if !finding.declared() {
			count++
		}
	}
	return count
}

// count is how many findings name one problem.
func (c *KeysetPagination) count(problem string) int {
	count := 0
	for _, finding := range c.Findings {
		if finding.Problem == problem {
			count++
		}
	}
	return count
}

// keepUndeclaredKeyset drops the findings a declaration accounts for, which
// is what -gaps-only asks for.
func keepUndeclaredKeyset(found []KeysetFinding) []KeysetFinding {
	kept := make([]KeysetFinding, 0, len(found))
	for _, finding := range found {
		if !finding.declared() {
			kept = append(kept, finding)
		}
	}
	return kept
}
