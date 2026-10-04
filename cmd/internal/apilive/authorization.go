package apilive

import (
	"fmt"
	"strings"
)

// This file is the fine-grained half of the record, added in schema version
// 4: what each REST route and each GraphQL element demands of a fine-grained
// personal access token, the permission vocabulary a token is granted in, and
// what an anonymous caller may do on a public project and group.
//
// All of it is read from the booted application, as the rest of the record
// is. GitLab's own permission tasks walk the same objects
// (lib/tasks/gitlab/permissions/routes/validate_task.rb and
// lib/tasks/gitlab/permissions/graphql/schema_directives.rb), and they are not
// loaded in a production image, so cmd/gen_api_live repeats their walk inside
// one rather than reading their output.

// RouteAuthorization is what a route's `route_setting :authorization`
// declares for a fine-grained token, key for key as lib/api/helpers.rb reads
// it. A route with none declares nothing a fine-grained token can satisfy.
type RouteAuthorization struct {
	// Permissions are raw permission names, the vocabulary GitLab checks a
	// token's expanded grant against, never the assignable names a user
	// grants.
	Permissions []string `json:"permissions,omitempty"`
	// BoundaryType is project, group, user or instance, and BoundaryParam the
	// request parameter naming the project or group when it is not the
	// default (project_id or group_id, then id).
	BoundaryType  string `json:"boundary_type,omitempty"`
	BoundaryParam string `json:"boundary_param,omitempty"`
	// Boundaries are alternatives: the token passes when it holds every
	// permission on any one of them.
	Boundaries []Boundary `json:"boundaries,omitempty"`
	// Boundary is a callable GitLab evaluates per request, which wins over
	// Boundaries and BoundaryType whenever the route declares one.
	Boundary *Callable `json:"boundary,omitempty"`
	// AdditionalScopes must each pass on their own, on top of the primary
	// requirement.
	AdditionalScopes []AdditionalScope `json:"additional_scopes,omitempty"`
	// Skip is the reason the route opts out of the fine-grained check. Any
	// reason disables it, so the grant does not decide such a route.
	Skip string `json:"skip,omitempty"`
	// Todo defers the decision; a fine-grained token is denied meanwhile.
	Todo string `json:"todo,omitempty"`
	// AssignableWhen names the conditions under which GitLab offers the
	// permission in its token form. It describes and never enforces.
	AssignableWhen []string `json:"assignable_when,omitempty"`
	// UnknownKeys are keys of the hash no reader here understands, recorded
	// so a new option is seen rather than dropped.
	UnknownKeys []string `json:"unknown_keys,omitempty"`
}

// Boundary is one boundary alternative a route accepts.
type Boundary struct {
	BoundaryType  string    `json:"boundary_type,omitempty"`
	BoundaryParam string    `json:"boundary_param,omitempty"`
	Boundary      *Callable `json:"boundary,omitempty"`
	UnknownKeys   []string  `json:"unknown_keys,omitempty"`
}

// AdditionalScope is one further requirement a route declares, judged on its
// own boundary: a boundary alternative's keys and permissions of its own.
type AdditionalScope struct {
	Permissions   []string  `json:"permissions,omitempty"`
	BoundaryType  string    `json:"boundary_type,omitempty"`
	BoundaryParam string    `json:"boundary_param,omitempty"`
	Boundary      *Callable `json:"boundary,omitempty"`
	UnknownKeys   []string  `json:"unknown_keys,omitempty"`
}

// Callable is a boundary given as code GitLab runs per request. What it
// returns is known only then, so the record locates and quotes it, the way it
// does a block condition.
type Callable struct {
	Callable bool   `json:"callable"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Text     string `json:"text,omitempty"`
}

// The classes a route falls into for a fine-grained token, in the order
// GitLab decides them: a skip reason disables the check whatever else the
// route declares, declared permissions are checked, a todo defers the
// decision, and a route declaring none of these is refused.
const (
	RouteSkipped    = "skipped"
	RouteAuthorized = "authorized"
	RouteTodo       = "todo"
	RouteUndeclared = "undeclared"
)

// FineGrained is the class the route falls into for a fine-grained token.
//
// Written as a chain of returns rather than a tagless switch so that every
// test is a statement of its own, which is what lets the mutation gate see
// that each one is reached.
func (r Route) FineGrained() string {
	auth := r.Authorization
	if auth == nil {
		return RouteUndeclared
	}
	if auth.Skip != "" {
		return RouteSkipped
	}
	if len(auth.Permissions) > 0 {
		return RouteAuthorized
	}
	if auth.Todo != "" {
		return RouteTodo
	}
	return RouteUndeclared
}

// The boundary types GitLab knows, lower case as its enum values and its
// boundary extractor compare them.
var boundaryTypes = map[string]bool{"project": true, "group": true, "user": true, "instance": true}

// KnownBoundaryType reports whether a boundary type is one GitLab resolves.
func KnownBoundaryType(boundaryType string) bool { return boundaryTypes[boundaryType] }

// Granular is the permission vocabulary a fine-grained token is granted in.
type Granular struct {
	// Assignable are the permissions a user grants, deprecated ones kept so a
	// grant written under an older name still reads, in the file-path order
	// GitLab searches them.
	Assignable []Assignable `json:"assignable"`
	// RawPermissions are every raw permission GitLab defines.
	RawPermissions []string `json:"raw_permissions"`
	// RawToAssignable maps each raw permission some assignable expands to
	// onto the assignable GitLab names for it.
	RawToAssignable map[string]AssignableMatch `json:"raw_to_assignable"`
	// PublicAnonymous is what an anonymous caller may do on a public project
	// and group, which GitLab serves to a token whatever its grant holds.
	// Absent when the evaluation could not run, which PublicAnonymousError
	// then explains.
	PublicAnonymous      *PublicAnonymous `json:"public_anonymous,omitempty"`
	PublicAnonymousError string           `json:"public_anonymous_error,omitempty"`
	// FlagDefaultEnabled is whether fine-grained tokens are on unless an
	// administrator turns them off.
	FlagDefaultEnabled *bool `json:"flag_default_enabled,omitempty"`
}

// Assignable is one permission a user can grant a token.
type Assignable struct {
	Name         string `json:"name"`
	Category     string `json:"category"`
	CategoryName string `json:"category_name"`
	Resource     string `json:"resource"`
	ResourceName string `json:"resource_name"`
	Action       string `json:"action"`
	// Display is "Resource: Action" exactly as GitLab writes it in a refusal.
	Display string `json:"display"`
	// Boundaries are the boundary types the permission can be granted at.
	Boundaries []string `json:"boundaries"`
	// Permissions are the raw permissions it expands to.
	Permissions []string `json:"permissions"`
	// AvailableFor names who may hold it: granular_access_token, role, or
	// both.
	AvailableFor   []string              `json:"available_for"`
	Deprecated     bool                  `json:"deprecated,omitempty"`
	AssignableWhen []AssignableCondition `json:"assignable_when,omitempty"`
}

// AssignableCondition is one condition under which GitLab offers an
// assignable permission in its token form.
type AssignableCondition struct {
	Condition  string   `json:"condition"`
	Boundaries []string `json:"boundaries,omitempty"`
}

// AssignableMatch is the assignable GitLab names for one raw permission, read
// two ways.
type AssignableMatch struct {
	// First is the first definition that expands to the raw permission,
	// deprecated ones included, which is the name GitLab's refusal prints.
	First string `json:"first"`
	// FirstAvailable is the first one a token can be granted: not deprecated
	// and available to a granular access token. Empty when there is none.
	FirstAvailable string `json:"first_available,omitempty"`
}

// The ways the public subjects can be built, from cheapest to dearest: records
// the evaluation never writes, a project and group created inside the
// throwaway container, or the end-to-end fixture's.
const (
	PublicSourceUnsaved   = "unsaved"
	PublicSourcePersisted = "persisted"
	PublicSourceFixture   = "fixture"
)

// KnownPublicSource reports whether a public set names one of the three ways
// it may be produced.
func KnownPublicSource(source string) bool {
	switch source {
	case PublicSourceUnsaved, PublicSourcePersisted, PublicSourceFixture:
		return true
	default:
		return false
	}
}

// PublicAnonymous is GitLab's evaluated anonymous policy on a public project
// with every feature enabled and on a public group, over the raw permissions
// some assignable expands to.
type PublicAnonymous struct {
	// Source says how the subjects were built.
	Source string `json:"source"`
	// Licensed is set when every licensed feature was made available for the
	// evaluation, so the set is the widest a licensed instance serves.
	Licensed bool     `json:"licensed,omitempty"`
	Project  []string `json:"project"`
	Group    []string `json:"group"`
}

// GraphQLAuthz is what the GraphQL schema demands of a fine-grained token.
type GraphQLAuthz struct {
	// Types are every object type, keyed by its GraphQL name.
	Types map[string]GraphQLType `json:"types"`
	// Abstract are every union and interface with the object types GitLab may
	// resolve there.
	Abstract map[string]AbstractType `json:"abstract"`
	// Mutations are keyed by the field name a document uses.
	Mutations map[string]Mutation `json:"mutations"`
	// Fields are the field-level declarations, keyed Type.field.
	Fields map[string][]Directive `json:"fields"`
	// UndeclaredByTodoRule is the set GitLab's own rule for its todo list
	// computes, so the two can be held to each other.
	UndeclaredByTodoRule TodoSet `json:"undeclared_by_todo_rule"`
	// Todo is GitLab's list of what it has not declared yet, when the image
	// ships it.
	Todo *Todo `json:"todo,omitempty"`
}

// GraphQLType is one object type.
type GraphQLType struct {
	Class string `json:"class,omitempty"`
	// Enforced is whether GitLab's granular check runs on an object of the
	// type. A type outside Types::BaseObject, or one overriding authorized?
	// without calling super, is never checked whatever it declares. Query and
	// Mutation are enforced and declare nothing, and pass anyway: the check
	// passes when there is neither an object nor arguments to find a
	// boundary in, which is what the root objects are.
	Enforced bool `json:"enforced,omitempty"`
	// AuthorizedBy names the override of authorized?, when there is one.
	AuthorizedBy string `json:"authorized_by,omitempty"`
	// Abilities are the type's ability authorization, one of the two
	// conditions under which a list of it is redacted rather than nulled.
	Abilities []string    `json:"abilities,omitempty"`
	Granular  []Directive `json:"granular,omitempty"`
	// ObjectFields are the fields whose type is an object, an interface or a
	// union, with their full signature.
	ObjectFields map[string]ObjectField `json:"object_fields,omitempty"`
}

// Declared reports whether the type carries a GranularScope directive, a skip
// reason included, which is what GitLab's todo rule counts as declared.
func (t GraphQLType) Declared() bool { return len(t.Granular) > 0 }

// ObjectField is one object-typed field.
type ObjectField struct {
	// Type is the signature as GraphQL writes it: [T!]!, [T], T!.
	Type       string `json:"type"`
	Connection bool   `json:"connection,omitempty"`
}

// NamedType is the type a signature names, without its list and non-null
// wrappers.
func (f ObjectField) NamedType() string { return strings.Trim(f.Type, "[]!") }

// AbstractType is a union or an interface.
type AbstractType struct {
	Kind          string   `json:"kind"`
	PossibleTypes []string `json:"possible_types"`
}

// Mutation is one mutation field.
type Mutation struct {
	// Name is the mutation's GraphQL name, the key GitLab's todo list uses.
	Name    string `json:"name"`
	Class   string `json:"class"`
	Payload string `json:"payload"`
	// Granular is what GitLab's runtime check reads, the mutation class's own
	// directives, and is empty for a mutation that declares nothing, which a
	// fine-grained token is refused.
	Granular []Directive `json:"granular,omitempty"`
	// FieldGranular is what GitLab's permission task reads for the mutation,
	// the Mutation field's directives (which graphql-ruby answers with any the
	// field declares itself followed by the class's), recorded only where it
	// differs from Granular. The task's todo list is generated from this
	// reading and the runtime check never makes it, so the gate refuses a
	// record carrying one: the todo list and what a token is refused would
	// then describe two requirements. Empty for every mutation at 19.4.1.
	FieldGranular []Directive `json:"field_granular,omitempty"`
}

// Directive is one GranularScope directive, argument for argument.
type Directive struct {
	Permissions      []string `json:"permissions,omitempty"`
	BoundaryType     string   `json:"boundary_type,omitempty"`
	Boundary         string   `json:"boundary,omitempty"`
	BoundaryArgument string   `json:"boundary_argument,omitempty"`
	RequirementGroup string   `json:"requirement_group,omitempty"`
	AssignableWhen   []string `json:"assignable_when,omitempty"`
	SkipReason       string   `json:"skip_reason,omitempty"`
	UnknownKeys      []string `json:"unknown_keys,omitempty"`
}

// TodoSet is a set of type and mutation names.
type TodoSet struct {
	Types     []string `json:"types"`
	Mutations []string `json:"mutations"`
}

// Todo is GitLab's config/authz/graphql/authorization_todo.txt as the image
// ships it.
type Todo struct {
	Digest string `json:"digest"`
	TodoSet
	// UnknownEntries are lines of a kind other than type: and mutation:.
	UnknownEntries []string `json:"unknown_entries,omitempty"`
}

// AuthorizationCounts are the fine-grained figures of a record, beside the
// entity, field, route and feature counts its provenance already carries.
type AuthorizationCounts struct {
	AuthorizedRoutes                int `json:"authorized_routes"`
	SkippedRoutes                   int `json:"skipped_routes"`
	TodoRoutes                      int `json:"todo_routes"`
	UndeclaredRoutes                int `json:"undeclared_routes"`
	AssignablePermissions           int `json:"assignable_permissions"`
	DeprecatedAssignablePermissions int `json:"deprecated_assignable_permissions"`
	GraphQLDeclaredTypes            int `json:"graphql_declared_types"`
	// GraphQLUndeclaredTypes are the types GitLab checks and that declare
	// nothing. A fine-grained token is denied an object of every one of them
	// but two: Query and Mutation are counted here and pass, because the
	// check passes when there is neither an object nor arguments to find a
	// boundary in, which is what a root is. A reader taking this figure, or
	// Enforced without Declared, for "denied" has to set those two apart.
	GraphQLUndeclaredTypes int `json:"graphql_undeclared_types"`
	// GraphQLUncheckedUndeclaredTypes are the ones GitLab's todo rule calls
	// undeclared and its check never runs on, such as PageInfo.
	GraphQLUncheckedUndeclaredTypes int `json:"graphql_unchecked_undeclared_types"`
	GraphQLDeclaredMutations        int `json:"graphql_declared_mutations"`
	GraphQLUndeclaredMutations      int `json:"graphql_undeclared_mutations"`
}

// CountAuthorization totals the fine-grained figures the provenance carries.
func (d Document) CountAuthorization() AuthorizationCounts {
	var counts AuthorizationCounts
	for _, route := range d.Routes {
		switch route.FineGrained() {
		case RouteAuthorized:
			counts.AuthorizedRoutes++
		case RouteSkipped:
			counts.SkippedRoutes++
		case RouteTodo:
			counts.TodoRoutes++
		default:
			counts.UndeclaredRoutes++
		}
	}
	if d.Granular != nil {
		counts.AssignablePermissions = len(d.Granular.Assignable)
		for _, assignable := range d.Granular.Assignable {
			if assignable.Deprecated {
				counts.DeprecatedAssignablePermissions++
			}
		}
	}
	if d.GraphQLAuthz == nil {
		return counts
	}
	for _, graphqlType := range d.GraphQLAuthz.Types {
		switch {
		case graphqlType.Declared():
			counts.GraphQLDeclaredTypes++
		case graphqlType.Enforced:
			counts.GraphQLUndeclaredTypes++
		}
	}
	for _, name := range d.GraphQLAuthz.UndeclaredByTodoRule.Types {
		if !d.GraphQLAuthz.Types[name].Enforced {
			counts.GraphQLUncheckedUndeclaredTypes++
		}
	}
	for _, mutation := range d.GraphQLAuthz.Mutations {
		if len(mutation.Granular) > 0 {
			counts.GraphQLDeclaredMutations++
		} else {
			counts.GraphQLUndeclaredMutations++
		}
	}
	return counts
}

// Expandable lists every raw permission some assignable expands to, which is
// the only set a route or a directive may name and the only one GitLab's
// public bypass is defined over.
func (g *Granular) Expandable() map[string]bool {
	expandable := map[string]bool{}
	if g == nil {
		return expandable
	}
	for _, assignable := range g.Assignable {
		for _, permission := range assignable.Permissions {
			expandable[permission] = true
		}
	}
	return expandable
}

// String renders the fine-grained figures as the one line a gate reports
// beside the provenance.
func (c AuthorizationCounts) String() string {
	return fmt.Sprintf(
		"fine-grained authorization of %d routes (%d skipped, %d todo, %d undeclared), "+
			"%d assignable permissions (%d deprecated), GraphQL %d declared and %d undeclared types "+
			"(%d more the check never runs on), %d declared and %d undeclared mutations",
		c.AuthorizedRoutes, c.SkippedRoutes, c.TodoRoutes, c.UndeclaredRoutes,
		c.AssignablePermissions, c.DeprecatedAssignablePermissions,
		c.GraphQLDeclaredTypes, c.GraphQLUndeclaredTypes, c.GraphQLUncheckedUndeclaredTypes,
		c.GraphQLDeclaredMutations, c.GraphQLUndeclaredMutations,
	)
}
