package join

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	gqlast "github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// RouteDeclaration answers a derived route the record lacks: the route whose
// authorization it carries.
type RouteDeclaration struct {
	// Route is the derived route, as the derivation spells it.
	Route    string
	Category string
	Reason   string
	// Use is the record route, as the table names it ("GET /projects/:id/...").
	Use string
}

// EffectDeclaration overrides, for one action, whether a GraphQL position is
// fatal, where a reviewer finds the spine rule wrong.
type EffectDeclaration struct {
	Action   string
	Path     string
	Category string
	Reason   string
	Fatal    bool
}

// BoundaryDeclaration says a GraphQL type is declared at a boundary the
// object one action reaches never resolves to, so no fine-grained token
// passes it there whatever it was granted.
type BoundaryDeclaration struct {
	Action   string
	Type     string
	Category string
	Reason   string
}

// Declarations are the answers the join takes from a person.
type Declarations struct {
	Routes       []RouteDeclaration
	Effects      []EffectDeclaration
	Unresolvable []BoundaryDeclaration
}

// Request is one derived request, joined.
type Request struct {
	derive.Use
	// Name is the route as the record spells it, or the GraphQL operation.
	Name string
	// RootFields and Positions describe a GraphQL request's operation.
	RootFields []string
	Positions  []string
	// Operation is the request's index in the table's operations, -1 for one
	// the join could not place.
	Operation int
	// Route is the declaration that placed a route the record lacks.
	Route string
}

// Action is one action, joined.
type Action struct {
	ID       string
	Handlers []string
	Requests []Request
	// Paths are the derivation's, as indices into Requests.
	Paths       [][]int
	Declaration string
	// Row is the action's row in the table, nil for an action the join could
	// not place whole.
	Row *finegrained.Requirement
}

// Result is the join.
type Result struct {
	Table   finegrained.Table
	Actions []Action
	// Findings are what keeps the table from being read as complete.
	Findings []string
	// Fallbacks counts GraphQL positions whose signature came from the pinned
	// schema because the record does not describe their parent type.
	Fallbacks int
}

// joiner builds a table.
type joiner struct {
	record   *apilive.Document
	routes   *routeIndex
	analyzer *analyzer
	decl     Declarations
	usedDecl map[string]bool

	permIndex  map[string]uint16
	groups     []finegrained.Group
	groupIndex map[string]uint32
	ops        []finegrained.Operation
	opIndex    map[string]int
	// opDenial is why no fine-grained token passes an operation, by index.
	opDenial map[int]*finegrained.Denial
	// opJudged keeps every judged position of a GraphQL operation, for the
	// per-action rules.
	opJudged   map[int][]judgedElement
	elements   []finegrained.Element
	elemIndex  map[string]uint32
	opGraphQL  map[int]bool
	collection map[int]bool
	opDetail   map[int]Operation
	findings   []string
	fallbacks  int
}

// judgedElement is an element of an operation with its index in the table.
type judgedElement struct {
	Element
	index uint32
}

// Join joins every derived action to the record.
func Join(record *apilive.Document, schema *gqlast.Schema, actions []derive.Action, decl Declarations) Result {
	j := &joiner{
		record:     record,
		routes:     newRouteIndex(record.Routes),
		analyzer:   &analyzer{authz: record.GraphQLAuthz, schema: schema},
		decl:       decl,
		usedDecl:   map[string]bool{},
		groupIndex: map[string]uint32{},
		opIndex:    map[string]int{},
		opDenial:   map[int]*finegrained.Denial{},
		opJudged:   map[int][]judgedElement{},
		elemIndex:  map[string]uint32{},
		opGraphQL:  map[int]bool{},
		collection: map[int]bool{},
		opDetail:   map[int]Operation{},
	}
	table := j.vocabulary()
	var out []Action
	for _, act := range actions {
		out = append(out, j.action(act))
	}
	j.staleDeclarations()
	table.Groups, table.Operations, table.Elements = j.groups, j.ops, j.elements
	for i := range out {
		if out[i].Row != nil {
			table.Actions = append(table.Actions, *out[i].Row)
		}
	}
	sort.Slice(table.Actions, func(a, b int) bool { return table.Actions[a].ID < table.Actions[b].ID })
	sort.Strings(j.findings)
	return Result{Table: table, Actions: out, Findings: j.findings, Fallbacks: j.fallbacks}
}

// vocabulary reads the permission vocabulary into a table.
func (j *joiner) vocabulary() finegrained.Table {
	granular := j.record.Granular
	expandable := granular.Expandable()
	permissions := make([]string, 0, len(expandable))
	for name := range expandable {
		permissions = append(permissions, name)
	}
	sort.Strings(permissions)
	j.permIndex = make(map[string]uint16, len(permissions))
	for i, name := range permissions {
		j.permIndex[name] = uint16(i)
	}
	byName := map[string]apilive.Assignable{}
	for _, assignable := range granular.Assignable {
		byName[assignable.Name] = assignable
	}
	table := finegrained.Table{
		Version:        j.record.Source.Version,
		Bucket:         finegrained.Bucket(j.record.Source.Version),
		Permissions:    permissions,
		Display:        make([]string, len(permissions)),
		RefusalDisplay: make([]string, len(permissions)),
	}
	for i, name := range permissions {
		match := granular.RawToAssignable[name]
		table.Display[i] = byName[match.FirstAvailable].Display
		table.RefusalDisplay[i] = byName[match.First].Display
	}
	for _, assignable := range granular.Assignable {
		entry := finegrained.Assignable{
			Name:       assignable.Name,
			Deprecated: assignable.Deprecated,
			Grantable:  slices.Contains(assignable.AvailableFor, "granular_access_token"),
		}
		for _, boundary := range assignable.Boundaries {
			entry.Boundaries |= boundaryOf(boundary)
		}
		for _, raw := range assignable.Permissions {
			entry.Permissions = append(entry.Permissions, j.permIndex[raw])
		}
		table.Assignables = append(table.Assignables, entry)
	}
	if public := granular.PublicAnonymous; public != nil {
		table.PublicKnown = true
		table.PublicAnonymous[finegrained.PublicProject] = j.bitset(public.Project, len(permissions))
		table.PublicAnonymous[finegrained.PublicGroup] = j.bitset(public.Group, len(permissions))
	}
	return table
}

// bitset sets one bit per named permission.
func (j *joiner) bitset(names []string, size int) []uint64 {
	bits := make([]uint64, (size+63)/64)
	for _, name := range names {
		if index, ok := j.permIndex[name]; ok {
			bits[index/64] |= 1 << (index % 64)
		}
	}
	return bits
}

// group indexes a requirement, reporting a permission the vocabulary lacks.
func (j *joiner) group(req requirement, where string) uint32 {
	key := req.key()
	if index, ok := j.groupIndex[key]; ok {
		return index
	}
	group := finegrained.Group{Any: req.any}
	for _, name := range req.perms {
		index, ok := j.permIndex[name]
		if !ok {
			j.findings = append(j.findings, fmt.Sprintf("%s demands %s, which no assignable permission expands to", where, name))
			continue
		}
		group.Perms = append(group.Perms, index)
	}
	index := uint32(len(j.groups))
	j.groups = append(j.groups, group)
	j.groupIndex[key] = index
	return index
}

// groupsOf indexes a list of requirements.
func (j *joiner) groupsOf(reqs []requirement, where string) []uint32 {
	var out []uint32
	for _, req := range reqs {
		out = append(out, j.group(req, where))
	}
	return out
}

// action joins one action.
func (j *joiner) action(act derive.Action) Action {
	out := Action{ID: act.ID, Handlers: act.Handlers, Paths: act.Paths, Declaration: act.Declaration}
	complete := len(act.Findings) == 0
	for _, finding := range act.Findings {
		j.findings = append(j.findings, finding)
	}
	for _, use := range act.Uses {
		request := Request{Use: use, Operation: -1}
		switch use.Kind {
		case derive.KindREST:
			j.restRequest(&request, act.ID)
		case derive.KindGraphQL:
			j.graphQLRequest(&request, act.ID)
		default:
			j.findings = append(j.findings, fmt.Sprintf("%s: unresolved request %s; declare what it sends", act.ID, use.Reason))
		}
		if request.Operation < 0 {
			complete = false
		}
		out.Requests = append(out.Requests, request)
	}
	if complete {
		out.Row = j.requirement(act.ID, out.Requests, act.Paths)
	}
	return out
}

// restRequest places a derived route.
func (j *joiner) restRequest(request *Request, actionID string) {
	derived := request.Method + " " + request.Path
	route, ok := j.routes.match(request.Method, request.Path)
	if !ok {
		route, ok = j.declaredRoute(derived)
		if !ok {
			j.findings = append(j.findings, fmt.Sprintf("%s: %s is no route of the live record; declare the route whose authorization it carries", actionID, derived))
			return
		}
		request.Route = derived
	}
	request.Name = liveName(route)
	key := "rest " + request.Name
	if index, seen := j.opIndex[key]; seen {
		request.Operation = index
		return
	}
	groups, skip, denied := restRequirements(route)
	op := finegrained.Operation{Name: request.Name, Groups: j.groupsOf(groups, request.Name), Skip: skip}
	index := j.addOp(key, op)
	if denied != "" {
		j.opDenial[index] = &finegrained.Denial{Cause: denied, Element: request.Name, Effect: finegrained.EffectRefused}
	}
	request.Operation = index
}

// declaredRoute finds the record route a declaration says a derived route
// carries the authorization of.
func (j *joiner) declaredRoute(derived string) (*apilive.Route, bool) {
	for _, declaration := range j.decl.Routes {
		if declaration.Route != derived {
			continue
		}
		j.usedDecl["route "+derived] = true
		for i := range j.record.Routes {
			if liveName(&j.record.Routes[i]) == declaration.Use {
				j.sameAuthorization(derived, &j.record.Routes[i])
				return &j.record.Routes[i], true
			}
		}
		j.findings = append(j.findings, fmt.Sprintf("the %s declaration of %s names %s, which is no route of the live record", declaration.Category, derived, declaration.Use))
	}
	return nil, false
}

// sameAuthorization holds a declared route to what it stands for: every
// record route its placeholders could be must declare what the named route
// declares, or naming one of them would hide the others' requirement.
func (j *joiner) sameAuthorization(derived string, named *apilive.Route) {
	method, path, _ := strings.Cut(derived, " ")
	segments := segmentsOf(path)
	for i := range j.record.Routes {
		candidate := &j.record.Routes[i]
		recordPath, mounted := strings.CutPrefix(candidate.Path, apiPrefix)
		if candidate.Method != method || !mounted || !looselyAgree(segmentsOf(recordPath), segments) {
			continue
		}
		if !reflect.DeepEqual(candidate.Authorization, named.Authorization) {
			j.findings = append(j.findings, fmt.Sprintf("%s stands for %s, which declares another authorization than %s",
				derived, liveName(candidate), liveName(named)))
		}
	}
}

// looselyAgree reports whether two spellings meet when a placeholder on
// either side may stand for a literal on the other.
func looselyAgree(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] && a[i] != ":" && b[i] != ":" {
			return false
		}
	}
	return true
}

// graphQLRequest places a GraphQL document.
func (j *joiner) graphQLRequest(request *Request, actionID string) {
	label := request.Use.Name
	if label == "" && len(request.SDKMethods) > 0 {
		label = request.SDKMethods[0]
	}
	if label == "" && len(request.Sites) > 0 {
		label = request.Sites[0]
	}
	key := "graphql " + derive.Digest(request.Document)
	if index, seen := j.opIndex[key]; seen {
		request.Operation = index
		request.Name = j.opDetail[index].Name
		request.RootFields = j.opDetail[index].RootFields
		request.Positions = j.opDetail[index].Paths
		return
	}
	operations, err := j.analyzer.operations(label, request.Document)
	if err != nil {
		j.findings = append(j.findings, fmt.Sprintf("%s: %v", actionID, err))
		return
	}
	if len(operations) != 1 {
		j.findings = append(j.findings, fmt.Sprintf("%s: document %s holds %d operations; one is what a request sends", actionID, label, len(operations)))
		return
	}
	judged := operations[0]
	j.fallbacks += judged.Fallbacks
	op := finegrained.Operation{Name: judged.Name, Groups: j.groupsOf(judged.Groups, judged.Name), Skip: judged.Skip}
	index := j.addOp(key, op)
	j.opGraphQL[index] = true
	j.collection[index] = judged.Collection
	j.opDetail[index] = judged
	if judged.Undeclared != "" {
		j.opDenial[index] = &finegrained.Denial{Cause: finegrained.CauseMutationUndeclared, Element: judged.Undeclared, Effect: finegrained.EffectRefused}
	}
	for _, element := range judged.Elements {
		if element.Skip {
			continue
		}
		elementIndex := j.element(element)
		j.opJudged[index] = append(j.opJudged[index], judgedElement{Element: element, index: elementIndex})
		switch {
		case element.Undeclared && element.Fatal:
			if j.opDenial[index] == nil {
				cause := finegrained.CauseTypeUndeclared
				if judged.Mutation {
					cause = finegrained.CausePayloadUndeclared
				}
				j.opDenial[index] = &finegrained.Denial{Cause: cause, Element: element.Type, Effect: element.Effect}
			}
		case element.Undeclared:
		case element.Fatal:
			j.ops[index].Spine = append(j.ops[index].Spine, elementIndex)
		default:
			j.ops[index].OffSpine = append(j.ops[index].OffSpine, elementIndex)
		}
	}
	request.Operation = index
	request.Name = judged.Name
	request.RootFields = judged.RootFields
	request.Positions = judged.Paths
}

// addOp records a new operation.
func (j *joiner) addOp(key string, op finegrained.Operation) int {
	index := len(j.ops)
	j.ops = append(j.ops, op)
	j.opIndex[key] = index
	return index
}

// element indexes one judged position.
func (j *joiner) element(element Element) uint32 {
	groups := j.groupsOf(element.Groups, element.Type)
	key := fmt.Sprintf("%s|%s|%v|%v|%v|%s", element.Path, element.Type, element.Members, groups, element.Undeclared, element.Effect)
	if index, ok := j.elemIndex[key]; ok {
		return index
	}
	index := uint32(len(j.elements))
	j.elements = append(j.elements, finegrained.Element{
		Path: element.Path, Type: element.Type, Members: element.Members, Groups: groups,
		Undeclared: element.Undeclared, Effect: element.Effect,
	})
	j.elemIndex[key] = index
	return index
}

// requirement builds an action's row: its paths over operations, with the
// paths a denied operation sits on left out, and the denial when every path
// holds one.
func (j *joiner) requirement(id string, requests []Request, paths [][]int) *finegrained.Requirement {
	row := &finegrained.Requirement{ID: id}
	degraded := map[uint32]bool{}
	var firstDenial *finegrained.Denial
	for _, path := range paths {
		var ops []uint32
		var denial *finegrained.Denial
		empty := map[uint32]bool{}
		for _, request := range path {
			op := requests[request].Operation
			ops = append(ops, uint32(op))
			if denied := j.denialFor(id, op, empty); denied != nil && denial == nil {
				denial = denied
			}
		}
		if denial != nil {
			if firstDenial == nil {
				firstDenial = denial
			}
			continue
		}
		for index := range empty {
			degraded[index] = true
		}
		slices.Sort(ops)
		ops = slices.Compact(ops)
		row.Paths = append(row.Paths, ops)
		for _, op := range ops {
			row.GraphQL = row.GraphQL || j.opGraphQL[int(op)]
			row.Collection = row.Collection || j.collection[int(op)]
		}
	}
	if len(row.Paths) == 0 && firstDenial != nil {
		row.Denied = firstDenial
		row.Paths = nil
		return row
	}
	row.Paths = minimizeOps(row.Paths)
	for index := range degraded {
		row.Degraded = append(row.Degraded, index)
	}
	slices.Sort(row.Degraded)
	return row
}

// denialFor says why no fine-grained token passes one operation of one
// action, and collects the positions it leaves empty.
func (j *joiner) denialFor(id string, op int, degraded map[uint32]bool) *finegrained.Denial {
	denial := j.opDenial[op]
	for _, element := range j.opJudged[op] {
		fatal := j.fatalFor(id, element)
		unresolvable := j.unresolvable(id, element.Type)
		switch {
		case unresolvable && fatal && denial == nil:
			effect := finegrained.EffectNullOrEmpty
			if j.opDetail[op].Mutation {
				effect = finegrained.EffectRefused
			}
			denial = &finegrained.Denial{Cause: finegrained.CauseBoundaryUnresolvable, Element: element.Type, Effect: effect}
		case (element.Undeclared || unresolvable) && !fatal:
			degraded[element.index] = true
		}
	}
	return denial
}

// fatalFor reads whether a position is fatal for one action, a declaration
// overriding the spine rule.
func (j *joiner) fatalFor(id string, element judgedElement) bool {
	for _, declaration := range j.decl.Effects {
		if declaration.Action == id && declaration.Path == element.Path {
			key := "effect " + id + " " + element.Path
			j.usedDecl[key] = j.usedDecl[key] || declaration.Fatal != element.Fatal
			return declaration.Fatal
		}
	}
	return element.Fatal
}

// unresolvable reports whether a declaration says a type never resolves its
// boundary for one action.
func (j *joiner) unresolvable(id, typeName string) bool {
	for _, declaration := range j.decl.Unresolvable {
		if declaration.Action == id && declaration.Type == typeName {
			j.usedDecl["boundary "+id+" "+typeName] = true
			return true
		}
	}
	return false
}

// staleDeclarations reports every declaration that answered nothing: a route
// the record now carries, an effect the rule already agrees with, a type no
// position of the action holds.
func (j *joiner) staleDeclarations() {
	for _, declaration := range j.decl.Routes {
		if !j.usedDecl["route "+declaration.Route] {
			j.findings = append(j.findings, fmt.Sprintf("the %s declaration of %s answers nothing: no action sends it, or the record carries it", declaration.Category, declaration.Route))
		}
	}
	for _, declaration := range j.decl.Effects {
		if !j.usedDecl["effect "+declaration.Action+" "+declaration.Path] {
			j.findings = append(j.findings, fmt.Sprintf("the %s declaration of %s at %s answers nothing: the spine rule already agrees, or no position is there", declaration.Category, declaration.Action, declaration.Path))
		}
	}
	for _, declaration := range j.decl.Unresolvable {
		if !j.usedDecl["boundary "+declaration.Action+" "+declaration.Type] {
			j.findings = append(j.findings, fmt.Sprintf("the %s declaration of %s on %s answers nothing: no position of the action is that type", declaration.Category, declaration.Action, declaration.Type))
		}
	}
}

// minimizeOps drops repeated paths and every path holding another.
func minimizeOps(paths [][]uint32) [][]uint32 {
	sort.Slice(paths, func(a, b int) bool {
		if len(paths[a]) != len(paths[b]) {
			return len(paths[a]) < len(paths[b])
		}
		return slices.Compare(paths[a], paths[b]) < 0
	})
	var out [][]uint32
	for _, path := range paths {
		kept := true
		for _, shorter := range out {
			if holds(path, shorter) {
				kept = false
				break
			}
		}
		if kept {
			out = append(out, path)
		}
	}
	return out
}

// holds reports whether the sorted set a holds every member of b.
func holds(a, b []uint32) bool {
	for _, want := range b {
		if _, found := slices.BinarySearch(a, want); !found {
			return false
		}
	}
	return true
}
