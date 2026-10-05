package grants

import (
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// ActionGrant is one action's requirement in GitLab's words, and what shaped
// the requests it was derived from. It is the 1:1 view: one row per action,
// the requirement worded by [finegrained.Table.Describe], which is what the
// site's fine-grained permissions page and the fine_grained block of
// gitlab://tools/{id} word it with, so the three never disagree.
type ActionGrant struct {
	ID    string `json:"id"`
	Owner string `json:"owner,omitempty"`
	// AnyOf, Denied, DeniedWays, AlwaysEmpty and EmptyWithout are the
	// description's, without the release it repeats on every row; the
	// report names that once.
	AnyOf        []finegrained.Way      `json:"any_of,omitempty"`
	Denied       *finegrained.Denial    `json:"denied,omitempty"`
	DeniedWays   []finegrained.Denial   `json:"denied_ways,omitempty"`
	AlwaysEmpty  []string               `json:"always_empty,omitempty"`
	EmptyWithout []finegrained.Position `json:"empty_without,omitempty"`
	// Declaration is the category of the action-level declaration that
	// answers what the walk could not, when one does.
	Declaration string `json:"declaration,omitempty"`
	// ShapedBy are the requests a //gitlab:request directive or a declaration
	// qualified, with what qualified each: the author's part of the
	// requirement, which the syntax alone would have read otherwise.
	ShapedBy []Shaping `json:"shaped_by,omitempty"`
}

// Shaping is one request a person qualified.
type Shaping struct {
	Request     string   `json:"request"`
	Class       string   `json:"class"`
	Directives  []string `json:"directives,omitempty"`
	Declaration string   `json:"declaration,omitempty"`
}

// PhaseA is what no fine-grained token reaches at the recorded release,
// whatever it was granted: the set a fine-grained session is withheld before
// its grant is read.
type PhaseA struct {
	GitLabVersion string       `json:"gitlab_version"`
	ByCause       []CauseGroup `json:"by_cause"`
	// PartlyWithheld are actions that run, except with an input that sends a
	// request no fine-grained token passes.
	PartlyWithheld []PartlyWithheld `json:"partly_withheld,omitempty"`
}

// CauseGroup is the withheld actions of one cause.
type CauseGroup struct {
	Cause finegrained.Cause `json:"cause"`
	// GraphQL is set for a cause only GraphQL answers with, which is the set
	// issue 1054 would serve over REST instead.
	GraphQL bool       `json:"graphql"`
	Count   int        `json:"count"`
	Actions []Withheld `json:"actions"`
}

// Withheld is one action no fine-grained token reaches, with the element that
// decides it and what GitLab does there.
type Withheld struct {
	ID      string             `json:"id"`
	Element string             `json:"element"`
	Effect  finegrained.Effect `json:"effect"`
}

// PartlyWithheld is an action one of whose ways no fine-grained token runs.
type PartlyWithheld struct {
	ID         string               `json:"id"`
	DeniedWays []finegrained.Denial `json:"denied_ways"`
}

// DegradedAction is an action served to a fine-grained token with part of
// its answer empty: always, or unless the grant holds more than the action
// itself needs.
type DegradedAction struct {
	ID           string                 `json:"id"`
	AlwaysEmpty  []string               `json:"always_empty,omitempty"`
	EmptyWithout []finegrained.Position `json:"empty_without,omitempty"`
}

// PublicOperation is a REST operation GitLab serves on a public project or
// group to a token whatever its grant holds, because every permission it
// checks is one GitLab's anonymous policy grants there.
type PublicOperation struct {
	Operation string `json:"operation"`
	// At are the boundaries its primary requirement is public at.
	At      []string `json:"at"`
	Actions []string `json:"actions"`
}

// actionView is the per-action half of the report, read from the table and
// the request record together.
type actionView struct {
	grants   []ActionGrant
	phaseA   PhaseA
	degraded []DegradedAction
	public   []PublicOperation
	// notJudged, byDirective and byDeclaration count the actions with a way
	// GitLab does not judge by the grant, with a request a directive
	// qualified, and with one a declaration supplied.
	notJudged, byDirective, byDeclaration int
}

// viewActions words every row of the table, with what shaped it.
func viewActions(table *finegrained.Table, record actionrequests.Record, owners map[string]string) actionView {
	byID := make(map[string]*actionrequests.RecordAction, len(record.Actions))
	for i := range record.Actions {
		byID[record.Actions[i].ID] = &record.Actions[i]
	}
	view := actionView{phaseA: PhaseA{GitLabVersion: table.DisplayVersion()}}
	causes := map[finegrained.Cause][]Withheld{}
	for i := range table.Actions {
		row := &table.Actions[i]
		description := table.Describe(row)
		grant := ActionGrant{
			ID: row.ID, Owner: owners[row.ID],
			AnyOf: description.AnyOf, Denied: description.Denied, DeniedWays: description.DeniedWays,
			AlwaysEmpty: description.AlwaysEmpty, EmptyWithout: description.EmptyWithout,
		}
		if recorded := byID[row.ID]; recorded != nil {
			grant.Declaration = recorded.Declaration
			grant.ShapedBy = shapings(recorded)
		}
		view.count(&grant)
		view.grants = append(view.grants, grant)
		if row.Denied != nil {
			causes[row.Denied.Cause] = append(causes[row.Denied.Cause], Withheld{ID: row.ID, Element: row.Denied.Element, Effect: row.Denied.Effect})
			continue
		}
		if len(row.DeniedWays) > 0 {
			view.phaseA.PartlyWithheld = append(view.phaseA.PartlyWithheld, PartlyWithheld{ID: row.ID, DeniedWays: row.DeniedWays})
		}
		if len(grant.AlwaysEmpty) > 0 || len(grant.EmptyWithout) > 0 {
			view.degraded = append(view.degraded, DegradedAction{ID: row.ID, AlwaysEmpty: grant.AlwaysEmpty, EmptyWithout: grant.EmptyWithout})
		}
	}
	for cause, withheld := range causes {
		view.phaseA.ByCause = append(view.phaseA.ByCause, CauseGroup{Cause: cause, GraphQL: cause.GraphQL(), Count: len(withheld), Actions: withheld})
	}
	slices.SortFunc(view.phaseA.ByCause, func(a, b CauseGroup) int { return strings.Compare(string(a.Cause), string(b.Cause)) })
	view.public = publicOperations(table)
	return view
}

// count adds one action to the counters of what shaped the table.
func (view *actionView) count(grant *ActionGrant) {
	if slices.ContainsFunc(grant.AnyOf, func(way finegrained.Way) bool { return way.NotJudged }) {
		view.notJudged++
	}
	if slices.ContainsFunc(grant.ShapedBy, func(s Shaping) bool { return len(s.Directives) > 0 }) {
		view.byDirective++
	}
	if grant.Declaration != "" || slices.ContainsFunc(grant.ShapedBy, func(s Shaping) bool { return s.Declaration != "" }) {
		view.byDeclaration++
	}
}

// shapings lists the requests of one action a directive or a declaration
// qualified.
func shapings(action *actionrequests.RecordAction) []Shaping {
	var out []Shaping
	for i := range action.Requests {
		request := &action.Requests[i]
		if len(request.Directives) == 0 && request.Declaration == "" {
			continue
		}
		out = append(out, Shaping{
			Request: requestName(request), Class: request.Class,
			Directives: request.Directives, Declaration: request.Declaration,
		})
	}
	return out
}

// requestName names a recorded request the way a reader looks it up: a REST
// request by its route, a GraphQL one by its operation, an unresolved one by
// what the walk could not read.
func requestName(request *actionrequests.RecordRequest) string {
	switch request.Kind {
	case actionrequests.KindREST:
		return request.Route
	case actionrequests.KindGraphQL:
		return request.Operation
	default:
		return request.Reason
	}
}

// publicBoundaries are the two boundaries GitLab's anonymous bypass is
// evaluated at, with the index of their set in the table.
var publicBoundaries = []struct {
	boundary finegrained.Boundary
	index    int
}{
	{boundary: finegrained.BoundaryProject, index: finegrained.PublicProject},
	{boundary: finegrained.BoundaryGroup, index: finegrained.PublicGroup},
}

// publicOperations lists the REST operations a public project or group serves
// with nothing granted: every requirement GitLab checks on the route is held
// by its anonymous policy at a boundary the requirement may be held at. A
// table that carries no evaluated set has none to name.
//
// It reads the table the way the call guard of a later phase will, permission
// by permission against the public set; what it answers here is only which
// operations need no grant at all, which is the one reading the listing never
// makes.
func publicOperations(table *finegrained.Table) []PublicOperation {
	if !table.PublicKnown {
		return nil
	}
	actions := map[uint32][]string{}
	for i := range table.Actions {
		row := &table.Actions[i]
		seen := map[uint32]bool{}
		for _, path := range row.Paths {
			for _, op := range path {
				if !seen[op] {
					seen[op] = true
					actions[op] = append(actions[op], row.ID)
				}
			}
		}
	}
	var out []PublicOperation
	for index := range table.Operations {
		op := &table.Operations[index]
		if op.Skip || len(op.Groups) == 0 || graphQLOperation(op.Name) {
			continue
		}
		at := publicAt(table, op.Groups[0])
		if at == 0 || !allPublic(table, op.Groups[1:]) {
			continue
		}
		ids := actions[uint32(index)] //#nosec G115 -- an index into the table's operations, far below the conversion's range
		sort.Strings(ids)
		out = append(out, PublicOperation{Operation: op.Name, At: at.Names(), Actions: ids})
	}
	return out
}

// allPublic reports whether every group is public at some boundary.
func allPublic(table *finegrained.Table, groups []uint32) bool {
	for _, group := range groups {
		if publicAt(table, group) == 0 {
			return false
		}
	}
	return true
}

// publicAt is the set of boundaries a group is public at: those it may be
// held at where the anonymous policy grants every one of its permissions.
func publicAt(table *finegrained.Table, index uint32) finegrained.Boundary {
	group := &table.Groups[index]
	var at finegrained.Boundary
	for _, public := range publicBoundaries {
		if group.Any&public.boundary == 0 {
			continue
		}
		every := true
		for _, perm := range group.Perms {
			every = every && bitSet(table.PublicAnonymous[public.index], int(perm))
		}
		if every {
			at |= public.boundary
		}
	}
	return at
}

// graphQLOperation reports whether a table operation is a GraphQL one, which
// the table names by its operation kind where a REST one carries its verb.
func graphQLOperation(name string) bool {
	kind, _, _ := strings.Cut(name, " ")
	return kind == "query" || kind == "mutation" || kind == "subscription"
}
