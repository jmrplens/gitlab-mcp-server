package join

import (
	"cmp"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"sort"
	"strings"

	gqlast "github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
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
// fatal, where a reviewer finds the spine rule wrong. It moves a declared
// position between the spine and the rest of the answer, and an undeclared
// or unresolvable one between withholding the action and emptying a field;
// an action it departs for gets an operation of its own, since every other
// action sending the same document keeps the rule's answer.
type EffectDeclaration struct {
	Action   string
	Path     string
	Category string
	Reason   string
	Fatal    bool
}

// BoundaryDeclaration says a GraphQL position is declared at a boundary the
// object one action reaches there never resolves to, so no fine-grained token
// passes it whatever it was granted. Path is where the position sits in the
// answer (`namespace.workItem` for a group's work item, which GitLab declares
// at the project boundary only), which GitLab then answers as null or removes
// from a list; or a mutation field (`workItemUpdate`), whose own check GitLab
// then refuses with `404 Not Found` before anything runs. It is keyed by path
// and not by type because one document reaches objects of one type that
// resolve and objects that do not: an epic's child issues are project work
// items, the epic is not.
type BoundaryDeclaration struct {
	Action   string
	Path     string
	Category string
	Reason   string
}

// ClassicDeclaration says which classic scope a REST route needs where GitLab
// departs from its general rule, read_api for a GET or a HEAD and api for
// anything else: a route granting read_api, or every scope, for every method,
// and one GitLab authenticates by another credential the caller passes and
// never by the token. The record carries no route's scopes, so this is the
// one place the join reads them from a person; it corroborates what it can,
// holding a declared other credential to the route's skip reason.
type ClassicDeclaration struct {
	// Route is the record route, as the table names it ("POST /markdown").
	Route    string
	Category string
	Reason   string
	Scope    finegrained.ClassicScope
}

// Declarations are the answers the join takes from a person.
type Declarations struct {
	Routes       []RouteDeclaration
	Effects      []EffectDeclaration
	Unresolvable []BoundaryDeclaration
	Classic      []ClassicDeclaration
}

// otherCredentialSkips are the skip reasons of the record that mean GitLab
// authenticates a route by a credential the caller passes as a parameter,
// a runner's token or a pipeline trigger's, and never reads the token this
// server holds.
var otherCredentialSkips = map[string]bool{"runner_token_auth": true, "trigger_token_auth": true}

// apiOnlyQueryFields are the GraphQL fields GitLab answers only to a token
// carrying api, a query selecting one needing api although it reads: each
// is declared with scopes: [:api] (app/graphql/types/issue_type.rb:170-171
// and app/graphql/types/work_item_type.rb:80-81 at v19.4.1-ee), and the
// field's authorization answers any other token null there, with no error
// (app/graphql/types/base_field.rb and GitlabSchema.unauthorized_field). Keyed
// by the type the field is selected on and its name.
var apiOnlyQueryFields = map[string]bool{"Issue.createNoteEmail": true, "WorkItem.createNoteEmail": true}

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
	// Classic is the classic scope the request needs, and ClassicDeclaration
	// the category of what departed from GitLab's general rule to decide it.
	Classic            finegrained.ClassicScope
	ClassicDeclaration string
}

// WayClassic is the classic scope one way of running an action needs: the
// strongest any of its requests needs, and [finegrained.ClassicNoRequest]
// for a way that sends nothing.
func WayClassic(requests []Request, path []int) finegrained.ClassicScope {
	way := finegrained.ClassicNoRequest
	for _, index := range path {
		way = max(way, requests[index].Classic)
	}
	return way
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
	routes   *apilive.RouteIndex
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
	// opDegraded are the positions an operation always answers empty, by
	// index.
	opDegraded map[int][]uint32
	// opClassicDecl is the category of what decided an operation's classic
	// scope where GitLab departs from its general rule, by index.
	opClassicDecl map[int]string
	// judged keeps each GraphQL document's judged operation, by key.
	judged     map[string]*Operation
	elements   []finegrained.Element
	elemIndex  map[string]uint32
	opGraphQL  map[int]bool
	collection map[int]bool
	findings   []string
	fallbacks  int
}

// Join joins every derived action to the record.
func Join(record *apilive.Document, schema *gqlast.Schema, actions []derive.Action, decl Declarations) Result {
	j := &joiner{
		record:        record,
		routes:        apilive.NewRouteIndex(record.Routes),
		analyzer:      &analyzer{authz: record.GraphQLAuthz, schema: schema},
		decl:          decl,
		usedDecl:      map[string]bool{},
		groupIndex:    map[string]uint32{},
		opIndex:       map[string]int{},
		opDenial:      map[int]*finegrained.Denial{},
		opDegraded:    map[int][]uint32{},
		judged:        map[string]*Operation{},
		opClassicDecl: map[int]string{},
		elemIndex:     map[string]uint32{},
		opGraphQL:     map[int]bool{},
		collection:    map[int]bool{},
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
	slices.SortFunc(table.Actions, func(a, b finegrained.Requirement) int { return strings.Compare(a.ID, b.ID) })
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
	displays := make([]string, len(permissions))
	for i, name := range permissions {
		displays[i] = byName[granular.RawToAssignable[name].FirstAvailable].Display
	}
	table := finegrained.Table{
		Version:     j.record.Source.Version,
		Bucket:      finegrained.Bucket(j.record.Source.Version),
		Permissions: permissions,
	}
	table.Displays, table.Display = indexDisplays(displays)
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

// indexDisplays holds each display once, sorted with the empty one first, and
// says per permission where its display sits in that list.
func indexDisplays(displays []string) (distinct []string, index []uint16) {
	distinct = append([]string{""}, displays...)
	slices.Sort(distinct)
	distinct = slices.Compact(distinct)
	index = make([]uint16, len(displays))
	for i, display := range displays {
		at, _ := slices.BinarySearch(distinct, display)
		index[i] = uint16(at) //#nosec G115 -- a position among the displays, which are no more than the permissions a uint16 already indexes
	}
	return distinct, index
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
	index := uint32(len(j.groups)) //#nosec G115 -- a count of the groups one run builds, far below the conversion's range
	j.groups = append(j.groups, group)
	j.groupIndex[key] = index
	return index
}

// groupsOf indexes a list of requirements. Every list it is handed holds each
// requirement once already, which is what makes one index per entry right: a
// route's comes from apilive.Route.Requirements, which returns a requirement
// the route names twice once, and a GraphQL operation's or position's from
// dedupeRequirements. Two requirements that differ are two groups, since a
// group is indexed by the requirement's key.
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
	j.findings = append(j.findings, act.Findings...)
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
		} else {
			request.Classic = j.ops[request.Operation].Classic
			request.ClassicDeclaration = j.opClassicDecl[request.Operation]
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
	route, ok := j.routes.Match(request.Method, request.Path)
	if !ok {
		route, ok = j.declaredRoute(derived)
		if !ok {
			j.findings = append(j.findings, fmt.Sprintf("%s: %s is no route of the live record; declare the route whose authorization it carries", actionID, derived))
			return
		}
		request.Route = derived
	}
	request.Name = apilive.RouteName(route)
	key := "rest " + request.Name
	if index, seen := j.opIndex[key]; seen {
		request.Operation = index
		return
	}
	groups, skip, denied := restRequirements(route)
	classic, decided := j.restClassic(route, request.Name)
	op := finegrained.Operation{Name: request.Name, Classic: classic, Groups: j.groupsOf(groups, request.Name), Skip: skip}
	index := j.addOp(key, op)
	if denied != "" {
		j.opDenial[index] = &finegrained.Denial{Cause: denied, Element: request.Name, Effect: finegrained.EffectRefused}
	}
	if decided != "" {
		j.opClassicDecl[index] = decided
	}
	request.Operation = index
}

// restClassic reads which classic scope a route needs, and the category of
// the declaration that decided it where GitLab departs from its general rule:
// read_api for a GET or a HEAD (lib/api/api.rb:60-61 at v19.4.1-ee), api for
// any other method. The record holds what it can of a declaration to account:
// a route whose skip reason says GitLab authenticates it by another credential
// is declared so, and a declaration says so of no other route. A declaration
// that agrees with the rule answers nothing.
func (j *joiner) restClassic(route *apilive.Route, name string) (scope finegrained.ClassicScope, declaration string) {
	rule := finegrained.ClassicAPI
	if readMethod(route.Method) {
		rule = finegrained.ClassicReadAPI
	}
	skip := ""
	if route.Authorization != nil {
		skip = route.Authorization.Skip
	}
	declared := j.classicDeclared(name)
	if declared == nil {
		if rule == finegrained.ClassicAPI && otherCredentialSkips[skip] {
			j.findings = append(j.findings, fmt.Sprintf("%s skips the fine-grained check as %s, so GitLab authenticates it by another credential; "+
				"declare the classic scope it needs", name, skip))
		}
		return rule, ""
	}
	if declared.Scope == rule {
		j.findings = append(j.findings, fmt.Sprintf("the %s classic declaration of %s answers nothing: GitLab's rule already gives a %s %s",
			declared.Category, name, route.Method, rule))
		return rule, ""
	}
	if (declared.Scope == finegrained.ClassicOtherCredential) != otherCredentialSkips[skip] {
		j.findings = append(j.findings, fmt.Sprintf("the %s classic declaration of %s says %s, and the live record's skip reason for it is %q",
			declared.Category, name, declared.Scope, skip))
	}
	return declared.Scope, declared.Category
}

// classicDeclared finds the classic declaration of a record route, noting
// that it answered something.
func (j *joiner) classicDeclared(name string) *ClassicDeclaration {
	for i := range j.decl.Classic {
		if j.decl.Classic[i].Route == name {
			j.usedDecl["classic "+name] = true
			return &j.decl.Classic[i]
		}
	}
	return nil
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
			if apilive.RouteName(&j.record.Routes[i]) == declaration.Use {
				j.sameAuthorization(derived, &j.record.Routes[i])
				j.sameClassicRule(derived, declaration, &j.record.Routes[i])
				return &j.record.Routes[i], true
			}
		}
		j.findings = append(j.findings, fmt.Sprintf("the %s declaration of %s names %s, which is no route of the live record", declaration.Category, derived, declaration.Use))
	}
	return nil, false
}

// readMethod reports whether GitLab's general rule grants read_api to a
// request sent with method: a GET or a HEAD (lib/api/api.rb:60-61 at
// v19.4.1-ee), whose rule keys on the request's own method.
func readMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// sameClassicRule holds a declared route to the side of GitLab's scope rule
// its own method falls on. The classic scope is read from the record route a
// request is joined to and shared by every request joined there, and R-GRANT
// reads it back from that same route, so a declaration naming a route on the
// other side of the read_api line would hand the request the other method's
// scope with nothing left to notice.
func (j *joiner) sameClassicRule(derived string, declaration RouteDeclaration, named *apilive.Route) {
	method, _, _ := strings.Cut(derived, " ")
	if readMethod(method) != readMethod(named.Method) {
		j.findings = append(j.findings, fmt.Sprintf("the %s declaration of %s names %s, which GitLab's scope rule reads on the other side of read_api from a %s",
			declaration.Category, derived, declaration.Use, method))
	}
}

// sameAuthorization holds a declared route to what it stands for: every
// record route its placeholders could be must declare what the named route
// declares, or naming one of them would hide the others' requirement.
func (j *joiner) sameAuthorization(derived string, named *apilive.Route) {
	method, path, _ := strings.Cut(derived, " ")
	segments := apilive.PathSegments(path)
	for i := range j.record.Routes {
		candidate := &j.record.Routes[i]
		recordPath, mounted := strings.CutPrefix(candidate.Path, apilive.EndpointPrefix)
		if candidate.Method != method || !mounted || !looselyAgree(apilive.PathSegments(recordPath), segments) {
			continue
		}
		if !reflect.DeepEqual(candidate.Authorization, named.Authorization) {
			j.findings = append(j.findings, fmt.Sprintf("%s stands for %s, which declares another authorization than %s",
				derived, apilive.RouteName(candidate), apilive.RouteName(named)))
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
	judged, ok := j.judged[key]
	if !ok {
		operations, err := j.analyzer.operations(label, request.Document)
		if err != nil {
			j.findings = append(j.findings, fmt.Sprintf("%s: %v", actionID, err))
			return
		}
		if len(operations) != 1 {
			j.findings = append(j.findings, fmt.Sprintf("%s: document %s holds %d operations; one is what a request sends", actionID, label, len(operations)))
			return
		}
		judged = &operations[0]
		j.judged[key] = judged
		j.fallbacks += judged.Fallbacks
	}
	rules := j.rulesFor(actionID, judged)
	index, seen := j.opIndex[key+rules.variant]
	if !seen {
		index = j.graphQLOperation(key+rules.variant, judged, rules)
	}
	request.Operation = index
	request.Name = judged.Name
	request.RootFields = judged.RootFields
	request.Positions = judged.Paths
}

// positionVerdict is how one action meets one judged position: whether a
// denial there takes the answer with it, and whether the position's boundary
// never resolves for the object the action reaches.
type positionVerdict struct {
	fatal        bool
	unresolvable bool
}

// actionRules are one action's verdicts on every position of an operation,
// read through its declarations.
type actionRules struct {
	// elements holds a verdict per element of the operation, in its order.
	elements []positionVerdict
	// mutationUnresolvable is set when a declaration says the mutation's own
	// boundary never resolves for the action: the first root field it says so
	// of, which is the one GitLab refuses first.
	mutationUnresolvable string
	// variant spells every verdict, so the actions holding the same verdicts
	// on a document share one operation and an action a declaration departs
	// for is given its own.
	variant string
}

// rulesFor reads one action's verdicts on an operation, noting each
// declaration that answers something.
func (j *joiner) rulesFor(actionID string, op *Operation) actionRules {
	rules := actionRules{elements: make([]positionVerdict, len(op.Elements))}
	if op.Mutation {
		for _, field := range op.RootFields {
			if j.unresolvable(actionID, field) {
				rules.mutationUnresolvable = field
				break
			}
		}
	}
	for i, element := range op.Elements {
		rules.elements[i] = positionVerdict{fatal: element.Fatal}
		if !element.Skip {
			rules.elements[i] = positionVerdict{fatal: j.fatalFor(actionID, element), unresolvable: j.unresolvable(actionID, element.Path)}
		}
	}
	rules.variant = fmt.Sprintf(" %q %v", rules.mutationUnresolvable, rules.elements)
	return rules
}

// graphQLOperation builds the table's operation for a judged document under
// one set of verdicts: what a grant must hold on its spine and elsewhere, the
// positions it always answers empty, and the first denial, in the order the
// document's positions are answered, that no grant passes. The mutation's own
// refusal comes before anything its payload holds, since nothing is written
// when it is refused.
func (j *joiner) graphQLOperation(key string, judged *Operation, rules actionRules) int {
	op := finegrained.Operation{Name: judged.Name, Classic: judged.classic(), Groups: j.groupsOf(judged.Groups, judged.Name), Skip: judged.Skip}
	index := j.addOp(key, op)
	if !judged.Mutation && len(judged.APIOnly) > 0 {
		j.opClassicDecl[index] = actionrequests.ClassicAPIOnlyField + " " + strings.Join(judged.APIOnly, ", ")
	}
	j.opGraphQL[index] = true
	j.collection[index] = judged.Collection
	// The cases here and below are sequences of ifs rather than untagged
	// switches so the mutation tool can measure each condition; it cannot see
	// a case expression.
	if judged.Undeclared != "" {
		j.opDenial[index] = &finegrained.Denial{Cause: finegrained.CauseMutationUndeclared, Element: judged.Undeclared, Effect: finegrained.EffectRefused}
	} else if rules.mutationUnresolvable != "" {
		j.opDenial[index] = &finegrained.Denial{Cause: finegrained.CauseBoundaryUnresolvable, Element: rules.mutationUnresolvable, Effect: finegrained.EffectRefused}
	}
	for i, element := range judged.Elements {
		if !element.Skip {
			j.placeElement(index, judged.Mutation, element, rules.elements[i])
		}
	}
	return index
}

// placeElement files one judged position of an operation: a denial no grant
// passes when it is fatal (the first of them, in the order the document is
// answered), a part served empty when it is denied and not fatal, and
// otherwise a requirement on the spine or off it.
func (j *joiner) placeElement(index int, mutation bool, element Element, met positionVerdict) {
	denied := element.Undeclared || met.unresolvable
	if denied && met.fatal {
		if j.opDenial[index] == nil {
			j.opDenial[index] = positionDenial(mutation, element, met)
		}
		return
	}
	if denied {
		j.opDegraded[index] = append(j.opDegraded[index], j.element(element))
		return
	}
	if met.fatal {
		j.ops[index].Spine = append(j.ops[index].Spine, j.element(element))
		return
	}
	j.ops[index].OffSpine = append(j.ops[index].OffSpine, j.element(element))
}

// positionDenial is why no grant passes an operation whose position, on the
// spine, no fine-grained token passes. In a mutation the write has committed
// by the time its payload is answered; in a read the position is answered
// null, or emptied of the items no token passes.
func positionDenial(mutation bool, element Element, met positionVerdict) *finegrained.Denial {
	denial := &finegrained.Denial{Cause: finegrained.CauseTypeUndeclared, Element: element.Type, Effect: element.Effect}
	if met.unresolvable {
		denial.Cause, denial.Effect = finegrained.CauseBoundaryUnresolvable, finegrained.EffectNullOrEmpty
	}
	if mutation {
		denial.Effect = finegrained.EffectCommittedThenNull
		if !met.unresolvable {
			denial.Cause = finegrained.CausePayloadUndeclared
		}
	}
	return denial
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
	index := uint32(len(j.elements)) //#nosec G115 -- a count of the elements one run judges, far below the conversion's range
	j.elements = append(j.elements, finegrained.Element{
		Path: element.Path, Type: element.Type, Members: element.Members, Groups: groups,
		Undeclared: element.Undeclared, Effect: element.Effect,
	})
	j.elemIndex[key] = index
	return index
}

// requirement builds an action's row: its paths over operations, with the
// paths a denied operation sits on kept apart as denied ways, and the denial
// when every path holds one. A path's denial is its first in the order the
// action makes its requests, which is the derivation's order, since that is
// the refusal a caller meets: a lookup no token passes stops the write after
// it from being sent at all. Whether the action reads GraphQL or a collection
// is asked of every path, a denied one included, since an input that selects
// a denied way is answered the way that way answers.
func (j *joiner) requirement(id string, requests []Request, paths [][]int) *finegrained.Requirement {
	row := &finegrained.Requirement{ID: id}
	degraded := map[uint32]bool{}
	var deniedWays []finegrained.Denial
	// A classic token is judged on every way the action can run, the ways no
	// fine-grained token passes included: GitLab asks a classic token for its
	// scopes and never for a grant.
	var ways []finegrained.ClassicScope
	for _, path := range paths {
		ways = append(ways, WayClassic(requests, path))
	}
	if len(ways) > 0 {
		row.Classic = slices.Min(ways)
	}
	for _, path := range paths {
		var ops []uint32
		var denial *finegrained.Denial
		for _, request := range path {
			op := requests[request].Operation
			ops = append(ops, uint32(op)) //#nosec G115 -- an operation index, set and non-negative on every request of a complete action
			row.GraphQL = row.GraphQL || j.opGraphQL[op]
			row.Collection = row.Collection || j.collection[op]
			if denial == nil {
				denial = j.opDenial[op]
			}
		}
		if denial != nil {
			if !slices.Contains(deniedWays, *denial) {
				deniedWays = append(deniedWays, *denial)
			}
			continue
		}
		for _, op := range ops {
			for _, index := range j.opDegraded[int(op)] {
				degraded[index] = true
			}
		}
		slices.Sort(ops)
		ops = slices.Compact(ops)
		row.Paths = append(row.Paths, ops)
	}
	if len(row.Paths) == 0 && len(deniedWays) > 0 {
		row.Denied = &deniedWays[0]
		return row
	}
	row.DeniedWays = deniedWays
	row.Paths = minimizeOps(row.Paths)
	for index := range degraded {
		row.Degraded = append(row.Degraded, index)
	}
	slices.Sort(row.Degraded)
	return row
}

// fatalFor reads whether a position is fatal for one action, a declaration
// overriding the spine rule. A declaration answers something only where it
// departs from the rule; one that agrees with it is stale.
func (j *joiner) fatalFor(id string, element Element) bool {
	for _, declaration := range j.decl.Effects {
		if declaration.Action == id && declaration.Path == element.Path {
			if declaration.Fatal != element.Fatal {
				j.usedDecl["effect "+id+" "+element.Path] = true
			}
			return declaration.Fatal
		}
	}
	return element.Fatal
}

// unresolvable reports whether a declaration says a position, or a
// mutation's own check, never resolves its boundary for one action.
func (j *joiner) unresolvable(id, path string) bool {
	for _, declaration := range j.decl.Unresolvable {
		if declaration.Action == id && declaration.Path == path {
			j.usedDecl["boundary "+id+" "+path] = true
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
		if !j.usedDecl["boundary "+declaration.Action+" "+declaration.Path] {
			j.findings = append(j.findings, fmt.Sprintf("the %s declaration of %s at %s answers nothing: no position or mutation of the action is there", declaration.Category, declaration.Action, declaration.Path))
		}
	}
	for _, declaration := range j.decl.Classic {
		if !j.usedDecl["classic "+declaration.Route] {
			j.findings = append(j.findings, fmt.Sprintf("the %s classic declaration of %s answers nothing: no action sends the route", declaration.Category, declaration.Route))
		}
	}
}

// minimizeOps drops repeated paths and every path holding another.
func minimizeOps(paths [][]uint32) [][]uint32 {
	slices.SortFunc(paths, func(a, b []uint32) int {
		return cmp.Or(cmp.Compare(len(a), len(b)), slices.Compare(a, b))
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
