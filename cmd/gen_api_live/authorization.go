package main

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// Floors below which the fine-grained half of a record is not describing a
// GitLab, for the same failure the other floors exist for: an introspection
// that half ran returns less rather than failing, and less here reads
// downstream as routes and types GitLab stopped declaring, which a filter
// would then withhold from every fine-grained token.
//
// Each is about seventy percent of what 19.4.1-ee gives (1892 authorized
// routes, 857 assignable permissions, 260 declared GraphQL types), the margin
// the other floors keep, so an ordinary release-to-release change never trips
// one while a walk that lost a third of the schema does.
const (
	minAuthorizedRoutes      = 1300
	minAssignablePermissions = 600
	minGraphQLDeclaredTypes  = 180
)

// namedLimit is how many names one problem lists before it counts the rest. A
// record broken at its root fails the same rule thousands of times, and a line
// listing them all buries the other problems under it.
const namedLimit = 10

// named lists names in order, the first namedLimit of them, counting the rest.
func named(names []string) string {
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	if len(sorted) <= namedLimit {
		return strings.Join(sorted, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(sorted[:namedLimit], ", "), len(sorted)-namedLimit)
}

// authorizationProblems reports the ways the fine-grained half of a version 4
// record cannot be rested on: each is a shape a reader would misread, and
// every rule below names the reading it protects.
func authorizationProblems(doc apilive.Document) []string {
	var problems []string
	if doc.Granular == nil {
		problems = append(problems, "it carries no granular block: nothing says which permissions "+
			"a fine-grained token can be granted or what they expand to, and every route would read as unreachable")
	}
	if doc.GraphQLAuthz == nil {
		problems = append(problems, "it carries no graphql_authz block: nothing says what a GraphQL type or "+
			"mutation demands of a fine-grained token")
	}
	problems = append(problems, authorizationFloorProblems(doc)...)
	problems = append(problems, vocabularyProblems(doc.Granular)...)
	problems = append(problems, routeAuthorizationProblems(doc)...)
	problems = append(problems, directiveProblems(doc)...)
	problems = append(problems, mutationFieldProblems(doc.GraphQLAuthz)...)
	problems = append(problems, todoProblems(doc)...)
	problems = append(problems, shapeProblems(doc)...)
	return problems
}

// authorizationFloorProblems reports a fine-grained half too small to be a
// GitLab's.
func authorizationFloorProblems(doc apilive.Document) []string {
	counts := doc.CountAuthorization()
	var problems []string
	for _, floor := range []struct {
		what  string
		got   int
		least int
		has   bool
	}{
		{"routes declaring fine-grained permissions", counts.AuthorizedRoutes, minAuthorizedRoutes, true},
		{"assignable permissions", counts.AssignablePermissions, minAssignablePermissions, doc.Granular != nil},
		{"GraphQL types declaring fine-grained permissions", counts.GraphQLDeclaredTypes, minGraphQLDeclaredTypes, doc.GraphQLAuthz != nil},
	} {
		// A missing block is reported once, as missing, rather than a second
		// time as a block holding nothing.
		if floor.has && floor.got < floor.least {
			problems = append(problems, fmt.Sprintf(
				"it holds %d %s and a GitLab has at least %d: the fine-grained walk did not finish",
				floor.got, floor.what, floor.least,
			))
		}
	}
	return problems
}

// grantableTo is the consumer a fine-grained token is, in an assignable's
// available_for.
const grantableTo = "granular_access_token"

// vocabularyProblems holds the map from a raw permission to the assignable
// GitLab names for it to what the assignables themselves say.
//
// Two readings rest on it. GitLab's refusal names the first assignable that
// expands to a missing permission, deprecated or not, and that is how a
// refusal is recognized; a person is told to grant the first one a token can
// actually hold. A match naming an assignable that does not expand to the
// permission, or offering as grantable one that is deprecated or reserved for
// roles, would put the wrong words in both.
func vocabularyProblems(granular *apilive.Granular) []string {
	if granular == nil {
		return nil
	}
	byName := make(map[string]apilive.Assignable, len(granular.Assignable))
	for _, assignable := range granular.Assignable {
		byName[assignable.Name] = assignable
	}
	expandsTo := func(name, raw string) bool {
		assignable, ok := byName[name]
		return ok && slices.Contains(assignable.Permissions, raw)
	}
	var foreign, ungrantable, unnamed []string
	for raw, match := range granular.RawToAssignable {
		if !expandsTo(match.First, raw) {
			foreign = append(foreign, raw+" "+quoted(match.First))
		}
		if match.FirstAvailable == "" {
			continue
		}
		available := byName[match.FirstAvailable]
		if !expandsTo(match.FirstAvailable, raw) {
			foreign = append(foreign, raw+" "+match.FirstAvailable)
		} else if available.Deprecated || !slices.Contains(available.AvailableFor, grantableTo) {
			ungrantable = append(ungrantable, raw+" "+match.FirstAvailable)
		}
	}
	for raw := range granular.Expandable() {
		if _, ok := granular.RawToAssignable[raw]; !ok {
			unnamed = append(unnamed, raw)
		}
	}

	problems := reportSites(nil, foreign,
		"%d raw permissions are mapped to an assignable permission that does not expand to them (%s): "+
			"a refusal naming one would be read as the wrong permission")
	problems = reportSites(problems, ungrantable,
		"%d raw permissions offer as grantable an assignable permission no token can hold (%s): "+
			"a person told to grant it could not")
	return reportSites(problems, unnamed,
		"%d raw permissions an assignable permission expands to have no assignable named for them (%s): "+
			"nothing can tell a person what to grant")
}

// permissionChecker sorts the permission names a declaration uses into the
// two ways one can be wrong, and remembers every site of each.
type permissionChecker struct {
	raw        map[string]bool
	expandable map[string]bool
	notRaw     []string
	unassigned []string
}

func newPermissionChecker(granular *apilive.Granular) *permissionChecker {
	checker := &permissionChecker{raw: map[string]bool{}, expandable: granular.Expandable()}
	if granular != nil {
		for _, permission := range granular.RawPermissions {
			checker.raw[permission] = true
		}
	}
	return checker
}

// check files each permission a site names that GitLab does not define, or
// that no assignable expands to and so no grant can ever hold.
func (c *permissionChecker) check(site string, permissions []string) {
	for _, permission := range permissions {
		switch {
		case !c.raw[permission]:
			c.notRaw = append(c.notRaw, site+" "+permission)
		case !c.expandable[permission]:
			c.unassigned = append(c.unassigned, site+" "+permission)
		}
	}
}

func (c *permissionChecker) problems(granular *apilive.Granular) []string {
	// Without the vocabulary nothing can be judged, and its absence is
	// already reported.
	if granular == nil {
		return nil
	}
	problems := reportSites(nil, c.notRaw,
		"%d declarations name a permission GitLab does not define (%s): a reader can match no grant against it")
	return reportSites(problems, c.unassigned,
		"%d declarations name a permission no assignable permission expands to (%s): no token can ever be granted it")
}

// reportSites appends one problem naming every site a rule found, when it
// found any. format takes the count and then the named sites.
func reportSites(problems, sites []string, format string) []string {
	if len(sites) == 0 {
		return problems
	}
	return append(problems, fmt.Sprintf(format, len(sites), named(sites)))
}

// routeFindings collects what the route rules find, site by site.
type routeFindings struct {
	permissions *permissionChecker
	unbounded   []string
	emptyScopes []string
	unknownKeys []string
	badTypes    []string
}

// unknown files the keys a site carries that nothing here reads.
func (f *routeFindings) unknown(site string, keys []string) {
	if len(keys) > 0 {
		f.unknownKeys = append(f.unknownKeys, site+" "+strings.Join(keys, "+"))
	}
}

// boundaryType files a boundary type GitLab does not resolve. An empty one is
// a missing value only where the shape demands one, which a boundary
// alternative does and a route or an additional scope with another boundary
// does not.
func (f *routeFindings) boundaryType(site, boundaryType string, required bool) {
	if (required || boundaryType != "") && !apilive.KnownBoundaryType(boundaryType) {
		f.badTypes = append(f.badTypes, site+" "+quoted(boundaryType))
	}
}

// route judges one route's authorization, key for key as lib/api/helpers.rb
// reads it.
func (f *routeFindings) route(site string, auth *apilive.RouteAuthorization) {
	f.permissions.check(site, auth.Permissions)
	if len(auth.Permissions) > 0 && auth.BoundaryType == "" && !callable(auth.Boundary) && len(auth.Boundaries) == 0 {
		f.unbounded = append(f.unbounded, site)
	}
	f.unknown(site, auth.UnknownKeys)
	f.boundaryType(site, auth.BoundaryType, false)
	for _, boundary := range auth.Boundaries {
		f.boundaryType(site+" boundaries", boundary.BoundaryType, true)
		f.unknown(site+" boundaries", boundary.UnknownKeys)
	}
	for _, scope := range auth.AdditionalScopes {
		f.scope(site, scope)
	}
}

// scope judges one additional scope, which GitLab checks on its own boundary
// and which needs permissions of its own.
func (f *routeFindings) scope(routeSite string, scope apilive.AdditionalScope) {
	site := routeSite + " additional scope"
	f.permissions.check(site, scope.Permissions)
	if len(scope.Permissions) == 0 {
		f.emptyScopes = append(f.emptyScopes, routeSite)
	} else if scope.BoundaryType == "" && !callable(scope.Boundary) {
		f.unbounded = append(f.unbounded, site)
	}
	f.boundaryType(site, scope.BoundaryType, false)
	f.unknown(site, scope.UnknownKeys)
}

// routeAuthorizationProblems holds every route's authorization to the shapes
// lib/api/helpers.rb reads.
func routeAuthorizationProblems(doc apilive.Document) []string {
	findings := routeFindings{permissions: newPermissionChecker(doc.Granular)}
	for _, route := range doc.Routes {
		if route.Authorization != nil {
			findings.route(route.Method+" "+route.Path, route.Authorization)
		}
	}

	problems := findings.permissions.problems(doc.Granular)
	problems = reportSites(problems, findings.unbounded,
		"%d routes declare permissions with no boundary type, callable boundary or boundary alternatives (%s): "+
			"GitLab resolves no boundary there and answers 404, which a reader would take for a grant decision")
	problems = reportSites(problems, findings.emptyScopes,
		"%d routes declare an additional scope with no permissions (%s): GitLab refuses every fine-grained token "+
			"there, and a reader would take the scope for satisfied")
	problems = reportSites(problems, findings.unknownKeys,
		"%d route authorizations carry keys nothing here reads (%s): introspect.rb met an option it does not know, "+
			"and a reader would judge the route without it")
	return reportSites(problems, findings.badTypes,
		"%d route boundaries name a type outside project, group, user and instance (%s): GitLab resolves no "+
			"boundary for it")
}

// callable reports whether a boundary is code GitLab runs per request, which
// is the only form of the key GitLab evaluates.
func callable(boundary *apilive.Callable) bool { return boundary != nil && boundary.Callable }

// quoted spells an empty value so it reads as one rather than as nothing.
func quoted(value string) string {
	if value == "" {
		return `""`
	}
	return value
}

// directiveProblems holds every GraphQL directive to the shapes
// Gitlab::Graphql::Authz::GranularScopeAuthorization reads.
func directiveProblems(doc apilive.Document) []string {
	authz := doc.GraphQLAuthz
	if authz == nil {
		return nil
	}
	permissions := newPermissionChecker(doc.Granular)
	var ambiguous, unknownKeys, badTypes []string
	judge := func(site string, directives []apilive.Directive) {
		for _, directive := range directives {
			permissions.check(site, directive.Permissions)
			// A skip and a requirement on one directive cannot both be meant,
			// and a directive carrying neither requires nothing anybody can
			// name; GitLab's own validator refuses both.
			if (len(directive.Permissions) > 0) == (directive.SkipReason != "") {
				ambiguous = append(ambiguous, site)
			}
			if directive.BoundaryType != "" && !apilive.KnownBoundaryType(directive.BoundaryType) {
				badTypes = append(badTypes, site+" "+directive.BoundaryType)
			}
			if len(directive.UnknownKeys) > 0 {
				unknownKeys = append(unknownKeys, site+" "+strings.Join(directive.UnknownKeys, "+"))
			}
		}
	}
	for name, graphqlType := range authz.Types {
		judge("type "+name, graphqlType.Granular)
	}
	for name, mutation := range authz.Mutations {
		judge("mutation "+name, mutation.Granular)
		judge("mutation field "+name, mutation.FieldGranular)
	}
	for name, directives := range authz.Fields {
		judge("field "+name, directives)
	}

	problems := permissions.problems(doc.Granular)
	problems = reportSites(problems, ambiguous,
		"%d GraphQL directives carry both permissions and a skip reason, or neither (%s): "+
			"a reader cannot tell whether they require anything")
	problems = reportSites(problems, badTypes,
		"%d GraphQL directives name a boundary type outside project, group, user and instance (%s): "+
			"GitLab resolves no boundary for it")
	return reportSites(problems, unknownKeys,
		"%d GraphQL directives carry arguments nothing here reads (%s): introspect.rb met an argument it "+
			"does not know, and a reader would judge the element without it")
}

// mutationFieldProblems reports a mutation whose Mutation field declares
// directives other than its class's.
//
// GitLab reads a mutation's directives in two places, and not the same way:
// its runtime check reads the class alone
// (Mutations::BaseMutation#granular_scope_authorization), while the permission
// task its todo list is generated with reads the field first and the class
// only when the field has none. The record holds the runtime's reading as
// Granular and the task's as FieldGranular, which introspect.rb writes only
// where the two differ; a FieldGranular equal to Granular passes all the same.
// Where they differ, the todo list and what a fine-grained token is refused
// describe two requirements, and no reader of one of them can tell which
// GitLab means.
func mutationFieldProblems(authz *apilive.GraphQLAuthz) []string {
	if authz == nil {
		return nil
	}
	var differing []string
	for name, mutation := range authz.Mutations {
		if len(mutation.FieldGranular) > 0 && !reflect.DeepEqual(mutation.FieldGranular, mutation.Granular) {
			differing = append(differing, name)
		}
	}
	return reportSites(nil, differing,
		"%d mutations declare directives on their Mutation field that differ from their class's (%s): "+
			"GitLab's permission task reads the field and its runtime check reads the class, so its todo list "+
			"and what a fine-grained token is refused no longer describe one requirement")
}

// todoProblems holds the undeclared set computed with GitLab's own rule to the
// todo list GitLab ships, when it ships one.
//
// GitLab keeps the list exact both ways in its own CI, so at a released
// revision the two are equal, and a difference means the walk here did not
// see the schema GitLab's task sees. The types the rule calls undeclared and
// the check never runs on are not a difference: they are counted apart, in
// [apilive.AuthorizationCounts.GraphQLUncheckedUndeclaredTypes].
func todoProblems(doc apilive.Document) []string {
	if doc.GraphQLAuthz == nil || doc.GraphQLAuthz.Todo == nil {
		return nil
	}
	rule := doc.GraphQLAuthz.UndeclaredByTodoRule
	todo := doc.GraphQLAuthz.Todo
	var problems []string
	for _, kind := range []struct {
		what       string
		rule, todo []string
	}{
		{"types", rule.Types, todo.Types},
		{"mutations", rule.Mutations, todo.Mutations},
	} {
		onlyRule, onlyTodo := setDifference(kind.rule, kind.todo), setDifference(kind.todo, kind.rule)
		if len(onlyRule)+len(onlyTodo) == 0 {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"the undeclared GraphQL %s computed with GitLab's todo rule differ from its authorization_todo.txt "+
				"(%s; %s): the walk did not see the schema GitLab's task sees",
			kind.what, onlyOneSide(onlyRule, "computed"), onlyOneSide(onlyTodo, "listed"),
		))
	}
	return reportSites(problems, todo.UnknownEntries,
		"authorization_todo.txt holds %d entries of a kind other than type and mutation (%s): "+
			"the list now names something this record does not compare")
}

// onlyOneSide spells the names one side of a comparison holds alone.
func onlyOneSide(names []string, side string) string {
	if len(names) == 0 {
		return "none only " + side
	}
	return fmt.Sprintf("%d only %s: %s", len(names), side, named(names))
}

// setDifference lists what a holds and b does not.
func setDifference(a, b []string) []string {
	in := make(map[string]bool, len(b))
	for _, name := range b {
		in[name] = true
	}
	var only []string
	for _, name := range a {
		if !in[name] {
			only = append(only, name)
		}
	}
	return only
}

// shapeProblems reports the GraphQL shapes and the public set a reader would
// misread: an abstract position it cannot resolve to its members, a field
// whose type the record does not describe, and a public set that does not say
// where it came from or holds what no grant could name.
func shapeProblems(doc apilive.Document) []string {
	return append(graphQLShapeProblems(doc.GraphQLAuthz), publicSetProblems(doc.Granular)...)
}

// graphQLShapeProblems reports an abstract type a walk cannot resolve to its
// members, and an object-typed field leading to a type the record does not
// describe.
func graphQLShapeProblems(authz *apilive.GraphQLAuthz) []string {
	if authz == nil {
		return nil
	}
	var memberless, unresolved []string
	for name, abstract := range authz.Abstract {
		if len(abstract.PossibleTypes) == 0 {
			memberless = append(memberless, name)
		}
	}
	for typeName, graphqlType := range authz.Types {
		for fieldName, field := range graphqlType.ObjectFields {
			if !describes(authz, field.NamedType()) {
				unresolved = append(unresolved, typeName+"."+fieldName+" "+field.NamedType())
			}
		}
	}
	problems := reportSites(nil, memberless,
		"%d unions or interfaces record no possible types (%s): a position of one cannot be judged by its members")
	return reportSites(problems, unresolved,
		"%d object-typed fields name a type the record describes neither as an object nor as a union or "+
			"interface (%s): a walk reaching one cannot go on")
}

// describes reports whether the record holds a type as an object or as a
// union or interface.
func describes(authz *apilive.GraphQLAuthz, name string) bool {
	_, isObject := authz.Types[name]
	_, isAbstract := authz.Abstract[name]
	return isObject || isAbstract
}

// publicSetProblems reports a public set that does not say how it was
// produced, or holds a permission GitLab's public bypass is not defined over.
func publicSetProblems(granular *apilive.Granular) []string {
	if granular == nil || granular.PublicAnonymous == nil {
		return nil
	}
	public := granular.PublicAnonymous
	var problems []string
	if !apilive.KnownPublicSource(public.Source) {
		problems = append(problems, fmt.Sprintf(
			"the public anonymous set names its source as %s, which is none of unsaved, persisted and fixture: "+
				"a reader cannot tell how wide it is",
			quoted(public.Source),
		))
	}
	expandable := granular.Expandable()
	var stray []string
	for _, set := range []struct {
		boundary    string
		permissions []string
	}{{"project", public.Project}, {"group", public.Group}} {
		for _, permission := range set.permissions {
			if !expandable[permission] {
				stray = append(stray, set.boundary+" "+permission)
			}
		}
	}
	return reportSites(problems, stray,
		"the public anonymous set holds %d permissions no assignable permission expands to (%s): "+
			"GitLab's public bypass is defined over none of them")
}
