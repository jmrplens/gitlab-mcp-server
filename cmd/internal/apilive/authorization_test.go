package apilive

import (
	"fmt"
	"reflect"
	"testing"
)

// TestRoute_FineGrained_ClassifiesInGitLabsOrder verifies the class a route
// falls into for a fine-grained token, in the order lib/api/helpers.rb and the
// authorization service decide it.
//
// The order is the point: a skip disables the check whatever else the route
// declares, and a todo beside declared permissions does not stop GitLab
// checking them. A classifier testing the keys in another order puts the
// skipped route with permissions among the authorized ones, or the todo route
// with permissions among the denied ones.
func TestRoute_FineGrained_ClassifiesInGitLabsOrder(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name string
		auth *RouteAuthorization
		want string
	}{
		{name: "a route with no authorization declares nothing", auth: nil, want: RouteUndeclared},
		{name: "a skip wins over declared permissions", auth: &RouteAuthorization{
			Permissions: []string{"read_project"}, Skip: "public_endpoint",
		}, want: RouteSkipped},
		{name: "declared permissions are checked", auth: &RouteAuthorization{
			Permissions: []string{"read_project"},
		}, want: RouteAuthorized},
		{name: "a todo beside permissions does not stop the check", auth: &RouteAuthorization{
			Permissions: []string{"read_project"}, Todo: "later",
		}, want: RouteAuthorized},
		{name: "a todo alone defers the decision", auth: &RouteAuthorization{Todo: "later"}, want: RouteTodo},
		{name: "a hash carrying only what nothing reads declares nothing", auth: &RouteAuthorization{
			UnknownKeys: []string{"future_option"},
		}, want: RouteUndeclared},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := (Route{Authorization: testCase.auth}).FineGrained(); got != testCase.want {
				t.Errorf("FineGrained() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestRouteClasses_AreSpelledAsTheCountsNameThem holds the four class names to
// their spelling, which a reader of a report compares by text.
func TestRouteClasses_AreSpelledAsTheCountsNameThem(t *testing.T) {
	t.Parallel()
	got := []string{RouteSkipped, RouteAuthorized, RouteTodo, RouteUndeclared}
	want := []string{"skipped", "authorized", "todo", "undeclared"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("classes = %q, want %q", got, want)
	}
}

// TestKnownBoundaryType_IsGitLabsLowerCaseFour verifies the boundary types a
// record may name, spelled as GitLab's enum values and its boundary extractor
// compare them, and nothing near them.
func TestKnownBoundaryType_IsGitLabsLowerCaseFour(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		boundaryType string
		want         bool
	}{
		{"project", true},
		{"group", true},
		{"user", true},
		{"instance", true},
		{"Project", false},
		{"PROJECT", false},
		{"namespace", false},
		{"", false},
	} {
		t.Run(testCase.boundaryType, func(t *testing.T) {
			t.Parallel()
			if got := KnownBoundaryType(testCase.boundaryType); got != testCase.want {
				t.Errorf("KnownBoundaryType(%q) = %v, want %v", testCase.boundaryType, got, testCase.want)
			}
		})
	}
}

// TestKnownPublicSource_IsTheThreeWaysASetIsProduced verifies the sources a
// public anonymous set may name, spelled as the introspection writes them.
func TestKnownPublicSource_IsTheThreeWaysASetIsProduced(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		source string
		want   bool
	}{
		{"unsaved", true},
		{"persisted", true},
		{"fixture", true},
		{"role-file", false},
		{"Unsaved", false},
		{"", false},
	} {
		t.Run(testCase.source, func(t *testing.T) {
			t.Parallel()
			if got := KnownPublicSource(testCase.source); got != testCase.want {
				t.Errorf("KnownPublicSource(%q) = %v, want %v", testCase.source, got, testCase.want)
			}
		})
	}
	t.Run("the constants are the spellings accepted", func(t *testing.T) {
		t.Parallel()
		got := []string{PublicSourceUnsaved, PublicSourcePersisted, PublicSourceFixture}
		if want := []string{"unsaved", "persisted", "fixture"}; !reflect.DeepEqual(got, want) {
			t.Errorf("sources = %q, want %q", got, want)
		}
	})
}

// TestGraphQLType_Declared_CountsASkipAsDeclared verifies GitLab's todo rule's
// reading of a declaration: any GranularScope directive, a skip reason
// included, takes a type off the todo list.
func TestGraphQLType_Declared_CountsASkipAsDeclared(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name string
		typ  GraphQLType
		want bool
	}{
		{name: "no directive", typ: GraphQLType{Enforced: true}, want: false},
		{name: "a permission", typ: GraphQLType{Granular: []Directive{{Permissions: []string{"read_project"}}}}, want: true},
		{name: "a skip reason", typ: GraphQLType{Granular: []Directive{{SkipReason: "parent_authorizes"}}}, want: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := testCase.typ.Declared(); got != testCase.want {
				t.Errorf("Declared() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestObjectField_NamedType_StripsEveryWrapper verifies that a signature
// resolves to the type it names however it is wrapped, which is how a walk
// finds the type a field leads to.
func TestObjectField_NamedType_StripsEveryWrapper(t *testing.T) {
	t.Parallel()
	for _, signature := range []string{"UserCore", "UserCore!", "[UserCore]", "[UserCore!]", "[UserCore!]!", "[[UserCore!]!]"} {
		t.Run(signature, func(t *testing.T) {
			t.Parallel()
			if got := (ObjectField{Type: signature}).NamedType(); got != "UserCore" {
				t.Errorf("NamedType() of %q = %q, want UserCore", signature, got)
			}
		})
	}
}

// countedRecord holds a different number of each thing the counts total, one
// to eleven, so a count read from its neighbor reads differently from the one
// asserted.
func countedRecord() Document {
	doc := Document{
		Granular: &Granular{},
		GraphQLAuthz: &GraphQLAuthz{
			Types:     map[string]GraphQLType{},
			Mutations: map[string]Mutation{},
		},
	}
	add := func(count int, auth func(int) *RouteAuthorization) {
		for i := range count {
			doc.Routes = append(doc.Routes, Route{Method: "GET", Path: fmt.Sprintf("/r/%d", len(doc.Routes)), Authorization: auth(i)})
		}
	}
	add(4, func(int) *RouteAuthorization { return &RouteAuthorization{Permissions: []string{"read_project"}} })
	// A skip wins over permissions declared beside it.
	add(3, func(int) *RouteAuthorization {
		return &RouteAuthorization{Skip: "public_endpoint", Permissions: []string{"read_project"}}
	})
	add(2, func(int) *RouteAuthorization { return &RouteAuthorization{Todo: "later"} })
	add(1, func(int) *RouteAuthorization { return nil })

	for i := range 9 {
		// Five of the nine are deprecated.
		doc.Granular.Assignable = append(doc.Granular.Assignable, Assignable{Name: fmt.Sprintf("a%d", i), Deprecated: i < 5})
	}

	types := doc.GraphQLAuthz.Types
	for i := range 6 {
		// Declared whether or not the check runs, and a skip counts.
		types[fmt.Sprintf("Declared%d", i)] = GraphQLType{Enforced: i%2 == 0, Granular: []Directive{{SkipReason: "parent_authorizes"}}}
	}
	rule := &doc.GraphQLAuthz.UndeclaredByTodoRule
	for i := range 7 {
		name := fmt.Sprintf("Undeclared%d", i)
		types[name] = GraphQLType{Enforced: true}
		// The rule names every enforced undeclared type too, and none of them
		// is unchecked.
		rule.Types = append(rule.Types, name)
	}
	for i := range 8 {
		// Types the rule names and the check never runs on, such as PageInfo.
		name := fmt.Sprintf("Unchecked%d", i)
		types[name] = GraphQLType{AuthorizedBy: "Types::Override"}
		rule.Types = append(rule.Types, name)
	}
	// Never checked and never on the rule's list: a connection.
	types["FooConnection"] = GraphQLType{}

	for i := range 10 {
		doc.GraphQLAuthz.Mutations[fmt.Sprintf("declared%d", i)] = Mutation{Granular: []Directive{{Permissions: []string{"create_note"}}}}
	}
	for i := range 11 {
		doc.GraphQLAuthz.Mutations[fmt.Sprintf("undeclared%d", i)] = Mutation{}
	}
	return doc
}

// TestCountAuthorization_TotalsEachFigureFromTheRecord verifies every
// fine-grained figure the provenance carries, each against a distinct count.
func TestCountAuthorization_TotalsEachFigureFromTheRecord(t *testing.T) {
	t.Parallel()
	want := AuthorizationCounts{
		AuthorizedRoutes:                4,
		SkippedRoutes:                   3,
		TodoRoutes:                      2,
		UndeclaredRoutes:                1,
		AssignablePermissions:           9,
		DeprecatedAssignablePermissions: 5,
		GraphQLDeclaredTypes:            6,
		GraphQLUndeclaredTypes:          7,
		GraphQLUncheckedUndeclaredTypes: 8,
		GraphQLDeclaredMutations:        10,
		GraphQLUndeclaredMutations:      11,
	}
	if got := countedRecord().CountAuthorization(); got != want {
		t.Errorf("CountAuthorization() =\n %+v\nwant\n %+v", got, want)
	}

	t.Run("a record with neither block counts its routes alone", func(t *testing.T) {
		t.Parallel()
		doc := countedRecord()
		doc.Granular, doc.GraphQLAuthz = nil, nil
		routesOnly := AuthorizationCounts{AuthorizedRoutes: 4, SkippedRoutes: 3, TodoRoutes: 2, UndeclaredRoutes: 1}
		if got := doc.CountAuthorization(); got != routesOnly {
			t.Errorf("CountAuthorization() = %+v, want %+v", got, routesOnly)
		}
	})
}

// TestGranular_Expandable_IsEveryRawPermissionAnAssignableExpandsTo verifies
// the set a declaration may name, and that a missing vocabulary answers an
// empty set rather than failing, since the gate reports the absence on its own.
func TestGranular_Expandable_IsEveryRawPermissionAnAssignableExpandsTo(t *testing.T) {
	t.Parallel()
	granular := &Granular{Assignable: []Assignable{
		{Name: "read_project", Permissions: []string{"read_project", "read_code"}},
		{Name: "read_old", Deprecated: true, Permissions: []string{"read_code", "read_legacy"}},
	}}
	want := map[string]bool{"read_project": true, "read_code": true, "read_legacy": true}
	if got := granular.Expandable(); !reflect.DeepEqual(got, want) {
		t.Errorf("Expandable() = %v, want %v", got, want)
	}

	t.Run("no vocabulary expands to nothing", func(t *testing.T) {
		t.Parallel()
		var none *Granular
		got := none.Expandable()
		if got == nil || len(got) != 0 {
			t.Errorf("Expandable() of a nil vocabulary = %v, want an empty set", got)
		}
	})
}

// TestAuthorizationCounts_String_PutsEveryFigureInItsPlace verifies the line a
// gate prints, every figure distinct so a traded pair reads differently.
func TestAuthorizationCounts_String_PutsEveryFigureInItsPlace(t *testing.T) {
	t.Parallel()
	got := AuthorizationCounts{
		AuthorizedRoutes: 1892, SkippedRoutes: 255, TodoRoutes: 5, UndeclaredRoutes: 3,
		AssignablePermissions: 857, DeprecatedAssignablePermissions: 71,
		GraphQLDeclaredTypes: 260, GraphQLUndeclaredTypes: 866, GraphQLUncheckedUndeclaredTypes: 1,
		GraphQLDeclaredMutations: 645, GraphQLUndeclaredMutations: 61,
	}.String()
	want := "fine-grained authorization of 1892 routes (255 skipped, 5 todo, 3 undeclared), " +
		"857 assignable permissions (71 deprecated), GraphQL 260 declared and 866 undeclared types " +
		"(1 more the check never runs on), 645 declared and 61 undeclared mutations"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
