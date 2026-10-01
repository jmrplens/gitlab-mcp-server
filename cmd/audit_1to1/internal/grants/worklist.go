package grants

import (
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// WorklistEntry is one GraphQL type or mutation this server reaches that
// keeps a fine-grained token out, which is issue 1055's work: an upstream
// declaration for each retires the entry at the release that ships it.
type WorklistEntry struct {
	Element string `json:"element"`
	// Kind is mutation, union or interface, or type, as the record holds the
	// element.
	Kind   string              `json:"kind"`
	Causes []finegrained.Cause `json:"causes"`
	// Blocks are the actions it withholds whole, BlocksAWayOf those it
	// withholds an input of, and EmptiesPartOf those it leaves a part of the
	// answer empty in while the rest is served.
	Blocks        []string `json:"blocks,omitempty"`
	BlocksAWayOf  []string `json:"blocks_a_way_of,omitempty"`
	EmptiesPartOf []string `json:"empties_part_of,omitempty"`
	// Abilities are the abilities the record says the type authorizes with,
	// and Resource its own name where an assignable permission names that
	// resource: the two things a REST route is matched on.
	Abilities []string `json:"abilities,omitempty"`
	Resource  string   `json:"resource,omitempty"`
	// RESTLeads are routes this server already calls that declare one of the
	// type's abilities as a fine-grained permission, or a permission of its
	// resource. A lead for the upstream merge request and for issue 1054,
	// never a verdict: a permission shared is not an object served.
	RESTLeads []Lead `json:"rest_leads,omitempty"`
}

// Lead is one REST route that declares a permission for an element.
type Lead struct {
	Route string `json:"route"`
	// Packages are the owning packages of the actions that call it, and
	// SamePackage is set when one of them owns an action the element blocks
	// or empties. Where such a route exists only those are listed, which is
	// the narrowest reading of "the same package".
	Packages    []string `json:"packages"`
	SamePackage bool     `json:"same_package"`
	Permissions []string `json:"permissions"`
}

// The kinds an element is held as.
const (
	kindMutation = "mutation"
	kindAbstract = "union or interface"
	kindType     = "type"
)

// worklist gathers every GraphQL element the table withholds an action or
// empties a part of an answer on.
func worklist(table *finegrained.Table, record actionrequests.Record, live *apilive.Document, owners map[string]string) []WorklistEntry {
	entries := map[string]*WorklistEntry{}
	entry := func(element string, cause finegrained.Cause) *WorklistEntry {
		found := entries[element]
		if found == nil {
			found = &WorklistEntry{Element: element}
			entries[element] = found
		}
		if !slices.Contains(found.Causes, cause) {
			found.Causes = append(found.Causes, cause)
		}
		return found
	}
	for i := range table.Actions {
		row := &table.Actions[i]
		if row.Denied != nil && row.Denied.Cause.GraphQL() {
			e := entry(row.Denied.Element, row.Denied.Cause)
			e.Blocks = append(e.Blocks, row.ID)
		}
		for _, way := range row.DeniedWays {
			if way.Cause.GraphQL() {
				e := entry(way.Element, way.Cause)
				e.BlocksAWayOf = appendOnce(e.BlocksAWayOf, row.ID)
			}
		}
		for _, index := range row.Degraded {
			e := entry(table.Elements[index].Type, finegrained.CauseTypeUndeclared)
			e.EmptiesPartOf = appendOnce(e.EmptiesPartOf, row.ID)
		}
	}
	finder := newLeadFinder(record, live, owners)
	out := make([]WorklistEntry, 0, len(entries))
	for _, e := range entries {
		e.Kind = elementKind(live, e.Element)
		slices.Sort(e.Causes)
		e.Abilities, e.Resource = finder.describe(e.Element)
		e.RESTLeads = finder.leads(e)
		out = append(out, *e)
	}
	slices.SortFunc(out, func(a, b WorklistEntry) int { return strings.Compare(a.Element, b.Element) })
	return out
}

// appendOnce appends id unless the list already holds it.
func appendOnce(list []string, id string) []string {
	if slices.Contains(list, id) {
		return list
	}
	return append(list, id)
}

// elementKind reads how the record holds an element: a mutation field, a
// union or interface, or an object type.
func elementKind(live *apilive.Document, element string) string {
	authz := live.GraphQLAuthz
	if authz == nil {
		return kindType
	}
	if _, ok := authz.Mutations[element]; ok {
		return kindMutation
	}
	if _, ok := authz.Abstract[element]; ok {
		return kindAbstract
	}
	return kindType
}

// snakeCase spells a GraphQL name the way GitLab spells a resource:
// BranchRule is branch_rule and bulkUpdateSecurityAttributes is
// bulk_update_security_attributes.
func snakeCase(name string) string {
	var b strings.Builder
	for i, r := range name {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// leadFinder answers, for a worklist entry, which routes this server already
// calls declare a permission for the entry's element.
type leadFinder struct {
	live *apilive.Document
	// resourceOf maps a raw permission onto the resource of the assignable
	// GitLab names for it, the first a token can be granted or else the
	// first at all; resources is every resource an assignable names.
	resourceOf map[string]string
	resources  map[string]bool
	// calls maps every REST route the request record names onto the actions
	// that request it, any class: a lead is about what this server already
	// reaches, however it reaches it.
	calls map[string][]string
	// routes are the record's routes by the name the request record uses.
	routes map[string]*apilive.Route
	owners map[string]string
}

// newLeadFinder indexes the record's vocabulary and the routes this server
// calls.
func newLeadFinder(record actionrequests.Record, live *apilive.Document, owners map[string]string) *leadFinder {
	finder := &leadFinder{
		live:       live,
		resourceOf: map[string]string{},
		resources:  map[string]bool{},
		calls:      map[string][]string{},
		routes:     make(map[string]*apilive.Route, len(live.Routes)),
		owners:     owners,
	}
	if granular := live.Granular; granular != nil {
		byName := map[string]apilive.Assignable{}
		for _, assignable := range granular.Assignable {
			byName[assignable.Name] = assignable
			finder.resources[assignable.Resource] = true
		}
		for raw, match := range granular.RawToAssignable {
			name := match.FirstAvailable
			if name == "" {
				name = match.First
			}
			finder.resourceOf[raw] = byName[name].Resource
		}
	}
	for _, action := range record.Actions {
		if owners[action.ID] == "" {
			continue
		}
		for _, request := range action.Requests {
			if request.Kind == actionrequests.KindREST {
				finder.calls[request.Route] = appendOnce(finder.calls[request.Route], action.ID)
			}
		}
	}
	for i := range live.Routes {
		finder.routes[apilive.RouteName(&live.Routes[i])] = &live.Routes[i]
	}
	return finder
}

// describe reads what an element is matched on: the abilities its type
// authorizes with, and its own name in snake case when an assignable names
// that resource. A mutation has neither unless a type of its name does.
//
// The abilities are matched as the raw permission names they are rather than
// through the assignable each one rolls up to, because an assignable is wide:
// Work Item: Read expands to the note, board and milestone permissions too,
// and matching through it named the boards' delete routes as leads for a
// note's quick actions.
func (finder *leadFinder) describe(element string) (abilities []string, resource string) {
	if authz := finder.live.GraphQLAuthz; authz != nil {
		abilities = slices.Clone(authz.Types[element].Abilities)
	}
	if own := snakeCase(element); finder.resources[own] {
		resource = own
	}
	return abilities, resource
}

// matches reports whether a raw permission a route declares is one an
// element is matched on.
func (finder *leadFinder) matches(e *WorklistEntry, permission string) bool {
	return slices.Contains(e.Abilities, permission) || e.Resource != "" && finder.resourceOf[permission] == e.Resource
}

// leads lists the routes this server calls that declare a permission an
// element is matched on, the ones its own packages call when there are any.
func (finder *leadFinder) leads(e *WorklistEntry) []Lead {
	affected := map[string]bool{}
	for _, list := range [][]string{e.Blocks, e.BlocksAWayOf, e.EmptiesPartOf} {
		for _, id := range list {
			affected[finder.owners[id]] = true
		}
	}
	var all, same []Lead
	for name, callers := range finder.calls {
		route := finder.routes[name]
		if route == nil || route.Authorization == nil {
			continue
		}
		var matched []string
		for _, perm := range declaredPermissions(route.Authorization) {
			if finder.matches(e, perm) {
				matched = appendOnce(matched, perm)
			}
		}
		if len(matched) == 0 {
			continue
		}
		lead := Lead{Route: name, Permissions: matched}
		for _, id := range callers {
			owner := finder.owners[id]
			lead.Packages = appendOnce(lead.Packages, owner)
			lead.SamePackage = lead.SamePackage || affected[owner]
		}
		sort.Strings(lead.Packages)
		sort.Strings(lead.Permissions)
		all = append(all, lead)
		if lead.SamePackage {
			same = append(same, lead)
		}
	}
	if len(same) > 0 {
		all = same
	}
	slices.SortFunc(all, func(a, b Lead) int { return strings.Compare(a.Route, b.Route) })
	return all
}

// declaredPermissions lists every raw permission a route declares, its
// primary requirement's and each additional scope's.
func declaredPermissions(auth *apilive.RouteAuthorization) []string {
	perms := slices.Clone(auth.Permissions)
	for _, scope := range auth.AdditionalScopes {
		perms = append(perms, scope.Permissions...)
	}
	return perms
}
