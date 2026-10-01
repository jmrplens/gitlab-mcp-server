package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// documentOf is the record a dump becomes, without the provenance runGenerate
// adds, for the rules that judge the record's content alone.
func documentOf(payload dumped) apilive.Document {
	return apilive.Document{
		SchemaVersion: apilive.SchemaVersion,
		Entities:      payload.Entities, Routes: payload.Routes, Features: payload.Features,
		Granular: payload.Granular, GraphQLAuthz: payload.GraphQLAuthz,
	}
}

// assertProblems compares a rule's answer with the sentences wanted, in order.
func assertProblems(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("problems =\n %q\nwant\n %q", got, want)
	}
}

// vocabulary is a small permission vocabulary shaped like GitLab's at the
// places the rules read: an assignable a token can be granted, a deprecated
// one sharing a raw permission with it, a role-only one, and a raw permission
// no assignable expands to.
//
// Three of its raw permissions are mapped the way GitLab 19.4.1 maps the
// awkward ones: read_code's first match is deprecated while a grantable one
// exists (the sixteen of the design), read_legacy has only a deprecated
// match, and read_email_enterprise_user only a role-only one, so neither has
// anything grantable to name.
func vocabulary() *apilive.Granular {
	return &apilive.Granular{
		Assignable: []apilive.Assignable{
			{Name: "read_old", Deprecated: true, Permissions: []string{"read_code", "read_legacy"}, AvailableFor: []string{grantableTo}},
			{Name: "read_project", Permissions: []string{"read_project", "read_code"}, AvailableFor: []string{grantableTo}},
			{Name: "read_email", Permissions: []string{"read_email_enterprise_user"}, AvailableFor: []string{"role"}},
		},
		RawPermissions: []string{"orphan_raw", "read_code", "read_email_enterprise_user", "read_legacy", "read_project"},
		RawToAssignable: map[string]apilive.AssignableMatch{
			"read_project":               {First: "read_project", FirstAvailable: "read_project"},
			"read_code":                  {First: "read_old", FirstAvailable: "read_project"},
			"read_legacy":                {First: "read_old"},
			"read_email_enterprise_user": {First: "read_email"},
		},
	}
}

// TestNamed_ListsInOrderAndCountsWhatItLeavesOut verifies how a problem names
// its sites: sorted, all of them up to the limit, and a count beyond it.
func TestNamed_ListsInOrderAndCountsWhatItLeavesOut(t *testing.T) {
	t.Parallel()
	many := func(count int) []string {
		var names []string
		// Added in reverse, so a list read in the order it was built differs.
		for i := count; i > 0; i-- {
			names = append(names, fmt.Sprintf("n%02d", i))
		}
		return names
	}
	for _, testCase := range []struct {
		name  string
		names []string
		want  string
	}{
		{name: "one", names: []string{"only"}, want: "only"},
		{name: "a few, sorted", names: []string{"b", "c", "a"}, want: "a, b, c"},
		{name: "exactly the limit", names: many(namedLimit), want: "n01, n02, n03, n04, n05, n06, n07, n08, n09, n10"},
		{name: "one past the limit", names: many(namedLimit + 1), want: "n01, n02, n03, n04, n05, n06, n07, n08, n09, n10 and 1 more"},
		{name: "far past the limit", names: many(namedLimit + 25), want: "n01, n02, n03, n04, n05, n06, n07, n08, n09, n10 and 25 more"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			original := slices.Clone(testCase.names)
			if got := named(testCase.names); got != testCase.want {
				t.Errorf("named() = %q, want %q", got, testCase.want)
			}
			// Sorting a caller's slice in place would reorder whatever it
			// goes on to report.
			if !slices.Equal(testCase.names, original) {
				t.Errorf("named() reordered its argument to %q", testCase.names)
			}
		})
	}
}

// TestQuoted_SpellsTheEmptyValue verifies that an empty value reads as one.
func TestQuoted_SpellsTheEmptyValue(t *testing.T) {
	t.Parallel()
	if got := quoted(""); got != `""` {
		t.Errorf(`quoted("") = %q, want ""`, got)
	}
	if got := quoted("project"); got != "project" {
		t.Errorf("quoted(project) = %q, want project", got)
	}
}

// TestAuthorizationProblems_AWholeRecord_HasNone verifies that a record every
// rule passes is passed whole: the record the generator writes from a real
// boot is one of these, and a rule refusing it would refuse every regeneration.
func TestAuthorizationProblems_AWholeRecord_HasNone(t *testing.T) {
	t.Parallel()
	if got := authorizationProblems(documentOf(wholeEnough())); got != nil {
		t.Errorf("authorizationProblems() = %q, want none", got)
	}
}

// TestAuthorizationProblems_AMissingBlock_IsReportedOnceAsMissing verifies the
// first refusal of a version 4 record: a block that is not there. Each is
// named once, as missing, and not again as a block holding too little, which
// would send a reader to the walk when it is the block that is gone.
func TestAuthorizationProblems_AMissingBlock_IsReportedOnceAsMissing(t *testing.T) {
	t.Parallel()
	noGranular := "it carries no granular block: nothing says which permissions a fine-grained token can be granted " +
		"or what they expand to, and every route would read as unreachable"
	noGraphQL := "it carries no graphql_authz block: nothing says what a GraphQL type or mutation demands of a fine-grained token"
	for _, testCase := range []struct {
		name   string
		mutate func(*dumped)
		want   []string
	}{
		{name: "no vocabulary", mutate: func(d *dumped) { d.Granular = nil }, want: []string{noGranular}},
		{name: "no GraphQL half", mutate: func(d *dumped) { d.GraphQLAuthz = nil }, want: []string{noGraphQL}},
		{name: "neither", mutate: func(d *dumped) { d.Granular, d.GraphQLAuthz = nil, nil }, want: []string{noGranular, noGraphQL}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			payload := wholeEnough()
			testCase.mutate(&payload)
			assertProblems(t, authorizationProblems(documentOf(payload)), testCase.want)
		})
	}
}

// authorizationFloor is the sentence a fine-grained floor reports.
func authorizationFloor(got int, what string, least int) string {
	return fmt.Sprintf("it holds %d %s and a GitLab has at least %d: the fine-grained walk did not finish", got, what, least)
}

// TestAuthorizationFloorProblems_AtEachFloor_PassesAndOneShortNamesItsOwnFigures
// holds the three fine-grained floors at their edge, each with its own count
// and minimum.
func TestAuthorizationFloorProblems_AtEachFloor_PassesAndOneShortNamesItsOwnFigures(t *testing.T) {
	t.Parallel()
	atEveryFloor := func() apilive.Document {
		doc := apilive.Document{
			Granular:     wholeVocabulary(minAssignablePermissions),
			GraphQLAuthz: wholeGraphQL(minGraphQLDeclaredTypes),
		}
		for i := range minAuthorizedRoutes {
			doc.Routes = append(doc.Routes, apilive.Route{
				Method: "GET", Path: routePath(i),
				Authorization: &apilive.RouteAuthorization{Permissions: []string{rawName(0)}, BoundaryType: "project"},
			})
		}
		return doc
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*apilive.Document)
		want   []string
	}{
		{name: "a record at every floor", mutate: func(*apilive.Document) {}},
		{
			name: "one authorized route short",
			mutate: func(d *apilive.Document) {
				d.Routes[0].Authorization = &apilive.RouteAuthorization{Skip: "public_endpoint"}
			},
			want: []string{authorizationFloor(minAuthorizedRoutes-1, "routes declaring fine-grained permissions", minAuthorizedRoutes)},
		},
		{
			name:   "one assignable permission short",
			mutate: func(d *apilive.Document) { d.Granular.Assignable = d.Granular.Assignable[1:] },
			want:   []string{authorizationFloor(minAssignablePermissions-1, "assignable permissions", minAssignablePermissions)},
		},
		{
			name:   "one declared GraphQL type short",
			mutate: func(d *apilive.Document) { delete(d.GraphQLAuthz.Types, typeName(0)) },
			want:   []string{authorizationFloor(minGraphQLDeclaredTypes-1, "GraphQL types declaring fine-grained permissions", minGraphQLDeclaredTypes)},
		},
		{
			// Their absence is the missing-block refusal's to report.
			name:   "a missing block is not also a block below its floor",
			mutate: func(d *apilive.Document) { d.Granular, d.GraphQLAuthz = nil, nil },
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			doc := atEveryFloor()
			testCase.mutate(&doc)
			assertProblems(t, authorizationFloorProblems(doc), testCase.want)
		})
	}
}

// TestVocabularyProblems_HoldsEachMatchToTheAssignablesItNames verifies the
// map a refusal is recognized by and a person is told what to grant from.
//
// The passing case is the one the design asked to be held: a raw permission
// whose only match is role-only names no grantable assignable, and that is a
// record telling the truth rather than a hole.
func TestVocabularyProblems_HoldsEachMatchToTheAssignablesItNames(t *testing.T) {
	t.Parallel()
	foreign := func(sites string, count int) string {
		return fmt.Sprintf("%d raw permissions are mapped to an assignable permission that does not expand to them (%s): "+
			"a refusal naming one would be read as the wrong permission", count, sites)
	}
	ungrantable := func(sites string, count int) string {
		return fmt.Sprintf("%d raw permissions offer as grantable an assignable permission no token can hold (%s): "+
			"a person told to grant it could not", count, sites)
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*apilive.Granular)
		want   []string
	}{
		{name: "a deprecated first match and a role-only one with nothing grantable pass", mutate: func(*apilive.Granular) {}},
		{
			name: "a deprecated assignable offered as grantable",
			mutate: func(g *apilive.Granular) {
				g.RawToAssignable["read_legacy"] = apilive.AssignableMatch{First: "read_old", FirstAvailable: "read_old"}
			},
			want: []string{ungrantable("read_legacy read_old", 1)},
		},
		{
			name: "a role-only assignable offered as grantable",
			mutate: func(g *apilive.Granular) {
				g.RawToAssignable["read_email_enterprise_user"] = apilive.AssignableMatch{First: "read_email", FirstAvailable: "read_email"}
			},
			want: []string{ungrantable("read_email_enterprise_user read_email", 1)},
		},
		{
			name: "a first match that does not expand to the permission",
			mutate: func(g *apilive.Granular) {
				g.RawToAssignable["read_project"] = apilive.AssignableMatch{First: "read_email", FirstAvailable: "read_project"}
			},
			want: []string{foreign("read_project read_email", 1)},
		},
		{
			name: "a first match the vocabulary does not hold",
			mutate: func(g *apilive.Granular) {
				g.RawToAssignable["read_project"] = apilive.AssignableMatch{FirstAvailable: "read_project"}
			},
			want: []string{foreign(`read_project ""`, 1)},
		},
		{
			name: "a grantable match that does not expand to the permission",
			mutate: func(g *apilive.Granular) {
				g.RawToAssignable["read_code"] = apilive.AssignableMatch{First: "read_old", FirstAvailable: "read_email"}
			},
			want: []string{foreign("read_code read_email", 1)},
		},
		{
			name: "both readings of one permission wrong count as two",
			mutate: func(g *apilive.Granular) {
				g.RawToAssignable["read_code"] = apilive.AssignableMatch{First: "read_email", FirstAvailable: "read_email"}
			},
			want: []string{foreign("read_code read_email, read_code read_email", 2)},
		},
		{
			name:   "a raw permission an assignable expands to with no match at all",
			mutate: func(g *apilive.Granular) { delete(g.RawToAssignable, "read_legacy") },
			want: []string{"1 raw permissions an assignable permission expands to have no assignable named for them (read_legacy): " +
				"nothing can tell a person what to grant"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			granular := vocabulary()
			testCase.mutate(granular)
			assertProblems(t, vocabularyProblems(granular), testCase.want)
		})
	}
	t.Run("no vocabulary has no vocabulary problems", func(t *testing.T) {
		t.Parallel()
		assertProblems(t, vocabularyProblems(nil), nil)
	})
}

// The sentences the route rules write, each with its count and its sites.
func notDefined(count int, sites string) string {
	return fmt.Sprintf("%d declarations name a permission GitLab does not define (%s): a reader can match no grant against it", count, sites)
}

func unassignable(count int, sites string) string {
	return fmt.Sprintf("%d declarations name a permission no assignable permission expands to (%s): no token can ever be granted it", count, sites)
}

func unbounded(count int, sites string) string {
	return fmt.Sprintf("%d routes declare permissions with no boundary type, callable boundary or boundary alternatives (%s): "+
		"GitLab resolves no boundary there and answers 404, which a reader would take for a grant decision", count, sites)
}

func routeUnknownKeys(count int, sites string) string {
	return fmt.Sprintf("%d route authorizations carry keys nothing here reads (%s): introspect.rb met an option it does not know, "+
		"and a reader would judge the route without it", count, sites)
}

func routeBadTypes(count int, sites string) string {
	return fmt.Sprintf("%d route boundaries name a type outside project, group, user and instance (%s): GitLab resolves no "+
		"boundary for it", count, sites)
}

// routeAt is the one route a route-rule case judges, at a path the expected
// sentences name.
func routeAt(auth *apilive.RouteAuthorization) apilive.Document {
	return apilive.Document{
		Granular: vocabulary(),
		Routes:   []apilive.Route{{Method: "POST", Path: "/api/:version/things", Authorization: auth}},
	}
}

const site = "POST /api/:version/things"

// TestRouteAuthorizationProblems_HoldsEachRouteToTheShapesGitLabReads verifies
// refusals 3, 4, 5 and 7 of the design for routes, and the shapes that pass
// although a narrower rule would refuse them.
func TestRouteAuthorizationProblems_HoldsEachRouteToTheShapesGitLabReads(t *testing.T) {
	t.Parallel()
	callable := &apilive.Callable{Callable: true, File: "lib/api/x.rb", Line: 3, Text: "lambda { project }"}
	for _, testCase := range []struct {
		name string
		auth *apilive.RouteAuthorization
		want []string
	}{
		{name: "a route declaring nothing is not judged here", auth: nil},
		{name: "a skipped route declaring nothing passes", auth: &apilive.RouteAuthorization{Skip: "public_endpoint"}},
		{name: "a boundary type passes", auth: &apilive.RouteAuthorization{Permissions: []string{"read_project"}, BoundaryType: "project"}},
		{
			// lib/api/integrations/integratable_operations.rb and
			// lib/api/project_import.rb declare boundaries: and nothing else.
			name: "boundary alternatives alone pass",
			auth: &apilive.RouteAuthorization{Permissions: []string{"read_project"}, Boundaries: []apilive.Boundary{
				{BoundaryType: "group", BoundaryParam: "namespace_id"}, {BoundaryType: "user"},
			}},
		},
		{name: "a callable boundary alone passes", auth: &apilive.RouteAuthorization{Permissions: []string{"read_project"}, Boundary: callable}},
		{
			name: "permissions with no boundary at all",
			auth: &apilive.RouteAuthorization{Permissions: []string{"read_project"}},
			want: []string{unbounded(1, site)},
		},
		{
			// GitLab evaluates the key only when it responds to call.
			name: "a boundary that is not callable is no boundary",
			auth: &apilive.RouteAuthorization{Permissions: []string{"read_project"}, Boundary: &apilive.Callable{Text: ":project"}},
			want: []string{unbounded(1, site)},
		},
		{
			name: "a permission GitLab does not define",
			auth: &apilive.RouteAuthorization{Permissions: []string{"read_project", "read_nothing"}, BoundaryType: "project"},
			want: []string{notDefined(1, site+" read_nothing")},
		},
		{
			name: "a permission no assignable expands to",
			auth: &apilive.RouteAuthorization{Permissions: []string{"orphan_raw"}, BoundaryType: "project"},
			want: []string{unassignable(1, site+" orphan_raw")},
		},
		{
			name: "an option nothing reads",
			auth: &apilive.RouteAuthorization{Skip: "public_endpoint", UnknownKeys: []string{"weight", "zone"}},
			want: []string{routeUnknownKeys(1, site+" weight+zone")},
		},
		{
			name: "a boundary type GitLab does not resolve",
			auth: &apilive.RouteAuthorization{Permissions: []string{"read_project"}, BoundaryType: "Project"},
			want: []string{routeBadTypes(1, site+" Project")},
		},
		{
			name: "boundary alternatives with no type, a bad type and an option nothing reads",
			auth: &apilive.RouteAuthorization{Permissions: []string{"read_project"}, Boundaries: []apilive.Boundary{
				{BoundaryParam: "id"}, {BoundaryType: "namespace"}, {BoundaryType: "user", UnknownKeys: []string{"weight"}},
			}},
			want: []string{
				routeUnknownKeys(1, site+" boundaries weight"),
				routeBadTypes(2, site+` boundaries "", `+site+" boundaries namespace"),
			},
		},
		{
			name: "additional scopes are held to the route's rules on their own boundary",
			auth: &apilive.RouteAuthorization{
				Permissions: []string{"read_project"}, BoundaryType: "project",
				AdditionalScopes: []apilive.AdditionalScope{
					{Permissions: []string{"read_code"}, BoundaryType: "group", BoundaryParam: "target"},
					{Permissions: []string{"read_code"}, Boundary: callable},
					{Permissions: []string{"read_code"}, BoundaryParam: "target"},
					{Permissions: []string{"read_nothing"}, BoundaryType: "realm", UnknownKeys: []string{"weight"}},
				},
			},
			want: []string{
				notDefined(1, site+" additional scope read_nothing"),
				unbounded(1, site+" additional scope"),
				routeUnknownKeys(1, site+" additional scope weight"),
				routeBadTypes(1, site+" additional scope realm"),
			},
		},
		{
			name: "an additional scope with no permissions",
			auth: &apilive.RouteAuthorization{
				Permissions: []string{"read_project"}, BoundaryType: "project",
				AdditionalScopes: []apilive.AdditionalScope{{BoundaryType: "group"}},
			},
			want: []string{"1 routes declare an additional scope with no permissions (" + site + "): GitLab refuses every " +
				"fine-grained token there, and a reader would take the scope for satisfied"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertProblems(t, routeAuthorizationProblems(routeAt(testCase.auth)), testCase.want)
		})
	}
	t.Run("without a vocabulary the permissions are not judged", func(t *testing.T) {
		t.Parallel()
		doc := routeAt(&apilive.RouteAuthorization{Permissions: []string{"read_nothing"}, BoundaryType: "project"})
		doc.Granular = nil
		assertProblems(t, routeAuthorizationProblems(doc), nil)
	})
	t.Run("every site of every route is counted", func(t *testing.T) {
		t.Parallel()
		doc := apilive.Document{Granular: vocabulary()}
		for i := range 3 {
			doc.Routes = append(doc.Routes, apilive.Route{
				Method: "GET", Path: fmt.Sprintf("/api/:version/r%d", i),
				Authorization: &apilive.RouteAuthorization{Permissions: []string{"read_project"}},
			})
		}
		want := []string{unbounded(3, "GET /api/:version/r0, GET /api/:version/r1, GET /api/:version/r2")}
		assertProblems(t, routeAuthorizationProblems(doc), want)
	})
}

// graphQLAt is a GraphQL half holding one directive list at each of the three
// places a directive can be declared.
func graphQLAt(directives []apilive.Directive) apilive.Document {
	return apilive.Document{
		Granular: vocabulary(),
		GraphQLAuthz: &apilive.GraphQLAuthz{
			Types:     map[string]apilive.GraphQLType{"WorkItem": {Enforced: true, Granular: directives}},
			Mutations: map[string]apilive.Mutation{"workItemCreate": {Name: "WorkItemCreate", Granular: directives}},
			Fields:    map[string][]apilive.Directive{"Issue.createNoteEmail": directives},
		},
	}
}

// TestDirectiveProblems_HoldsEveryDirectiveToTheShapesGitLabReads verifies
// refusals 4, 5, 6 and 7 of the design for GraphQL, at every place a directive
// is declared.
func TestDirectiveProblems_HoldsEveryDirectiveToTheShapesGitLabReads(t *testing.T) {
	t.Parallel()
	sites := func(suffix string) string {
		return "field Issue.createNoteEmail" + suffix + ", mutation workItemCreate" + suffix + ", type WorkItem" + suffix
	}
	for _, testCase := range []struct {
		name       string
		directives []apilive.Directive
		want       []string
	}{
		{name: "a skip reason alone passes", directives: []apilive.Directive{{SkipReason: "parent_authorizes"}}},
		{name: "a permission at a known boundary passes", directives: []apilive.Directive{
			{Permissions: []string{"read_project"}, BoundaryType: "project", Boundary: "project"},
			{Permissions: []string{"read_code"}, BoundaryType: "group", BoundaryArgument: "full_path", RequirementGroup: "target"},
		}},
		{
			name:       "both permissions and a skip reason",
			directives: []apilive.Directive{{Permissions: []string{"read_project"}, SkipReason: "parent_authorizes"}},
			want: []string{"3 GraphQL directives carry both permissions and a skip reason, or neither (" + sites("") +
				"): a reader cannot tell whether they require anything"},
		},
		{
			name:       "neither permissions nor a skip reason",
			directives: []apilive.Directive{{BoundaryType: "project"}},
			want: []string{"3 GraphQL directives carry both permissions and a skip reason, or neither (" + sites("") +
				"): a reader cannot tell whether they require anything"},
		},
		{
			name:       "a permission GitLab does not define",
			directives: []apilive.Directive{{Permissions: []string{"read_nothing"}}},
			want:       []string{notDefined(3, sites(" read_nothing"))},
		},
		{
			name:       "a permission no assignable expands to",
			directives: []apilive.Directive{{Permissions: []string{"orphan_raw"}}},
			want:       []string{unassignable(3, sites(" orphan_raw"))},
		},
		{
			name:       "a boundary type GitLab does not resolve",
			directives: []apilive.Directive{{Permissions: []string{"read_project"}, BoundaryType: "PROJECT"}},
			want: []string{"3 GraphQL directives name a boundary type outside project, group, user and instance (" +
				sites(" PROJECT") + "): GitLab resolves no boundary for it"},
		},
		{
			name:       "an argument nothing reads",
			directives: []apilive.Directive{{SkipReason: "parent_authorizes", UnknownKeys: []string{"weight", "zone"}}},
			want: []string{"3 GraphQL directives carry arguments nothing here reads (" + sites(" weight+zone") +
				"): introspect.rb met an argument it does not know, and a reader would judge the element without it"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertProblems(t, directiveProblems(graphQLAt(testCase.directives)), testCase.want)
		})
	}
	t.Run("no GraphQL half has no directive problems", func(t *testing.T) {
		t.Parallel()
		doc := graphQLAt(nil)
		doc.GraphQLAuthz = nil
		assertProblems(t, directiveProblems(doc), nil)
	})
}

// TestTodoProblems_HoldsTheComputedSetToTheListGitLabShips verifies refusal 8
// of the design: GitLab keeps its todo list exact, so the set its own rule
// computes in the image and the list it ships are equal, and a difference
// means the walk saw another schema.
//
// PageInfo is the case the design names: the rule calls it undeclared and the
// check never runs on it. That is not a difference between the two sets, and
// it passes.
func TestTodoProblems_HoldsTheComputedSetToTheListGitLabShips(t *testing.T) {
	t.Parallel()
	agreeing := func() *apilive.GraphQLAuthz {
		return &apilive.GraphQLAuthz{
			Types: map[string]apilive.GraphQLType{"PageInfo": {}, "Namespace": {Enforced: true}},
			UndeclaredByTodoRule: apilive.TodoSet{
				Types: []string{"Namespace", "PageInfo"}, Mutations: []string{"AiAction"},
			},
			Todo: &apilive.Todo{
				Digest:    "sha256:feedface",
				Types:     []string{"Namespace", "PageInfo"},
				Mutations: []string{"AiAction"},
			},
		}
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*apilive.GraphQLAuthz)
		want   []string
	}{
		{name: "equal sets, PageInfo among them, pass", mutate: func(*apilive.GraphQLAuthz) {}},
		{name: "no todo list shipped passes", mutate: func(a *apilive.GraphQLAuthz) { a.Todo = nil }},
		{
			name: "types only the computed set holds",
			mutate: func(a *apilive.GraphQLAuthz) {
				a.UndeclaredByTodoRule.Types = append(a.UndeclaredByTodoRule.Types, "BranchRule")
			},
			want: []string{"the undeclared GraphQL types computed with GitLab's todo rule differ from its authorization_todo.txt " +
				"(1 only computed: BranchRule; none only listed): the walk did not see the schema GitLab's task sees"},
		},
		{
			name: "types each side holds that the other does not",
			mutate: func(a *apilive.GraphQLAuthz) {
				a.UndeclaredByTodoRule.Types = append(a.UndeclaredByTodoRule.Types, "BranchRule", "CustomEmoji")
				a.Todo.Types = append(a.Todo.Types, "Epic")
			},
			want: []string{"the undeclared GraphQL types computed with GitLab's todo rule differ from its authorization_todo.txt " +
				"(2 only computed: BranchRule, CustomEmoji; 1 only listed: Epic): the walk did not see the schema GitLab's task sees"},
		},
		{
			// A type GitLab renamed between the list and the schema leaves
			// one name on each side, so the two differences are the same size
			// and only their sum, never their difference, tells them apart
			// from agreement.
			name: "a renamed type, one name on each side",
			mutate: func(a *apilive.GraphQLAuthz) {
				a.UndeclaredByTodoRule.Types = append(a.UndeclaredByTodoRule.Types, "WorkItemWidgetNew")
				a.Todo.Types = append(a.Todo.Types, "WorkItemWidgetOld")
			},
			want: []string{"the undeclared GraphQL types computed with GitLab's todo rule differ from its authorization_todo.txt " +
				"(1 only computed: WorkItemWidgetNew; 1 only listed: WorkItemWidgetOld): the walk did not see the schema GitLab's task sees"},
		},
		{
			name:   "mutations only the list holds",
			mutate: func(a *apilive.GraphQLAuthz) { a.Todo.Mutations = append(a.Todo.Mutations, "BoardEpicCreate") },
			want: []string{"the undeclared GraphQL mutations computed with GitLab's todo rule differ from its authorization_todo.txt " +
				"(none only computed; 1 only listed: BoardEpicCreate): the walk did not see the schema GitLab's task sees"},
		},
		{
			name:   "an entry of a kind nothing compares",
			mutate: func(a *apilive.GraphQLAuthz) { a.Todo.UnknownEntries = []string{"widget:Thing"} },
			want: []string{"authorization_todo.txt holds 1 entries of a kind other than type and mutation (widget:Thing): " +
				"the list now names something this record does not compare"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			authz := agreeing()
			testCase.mutate(authz)
			assertProblems(t, todoProblems(apilive.Document{GraphQLAuthz: authz}), testCase.want)
		})
	}
	t.Run("no GraphQL half has no todo problems", func(t *testing.T) {
		t.Parallel()
		assertProblems(t, todoProblems(apilive.Document{}), nil)
	})
}

// TestShapeProblems_HoldsTheGraphQLShapesAndThePublicSetToWhatAReaderNeeds
// verifies refusal 9 of the design.
func TestShapeProblems_HoldsTheGraphQLShapesAndThePublicSetToWhatAReaderNeeds(t *testing.T) {
	t.Parallel()
	whole := func() apilive.Document {
		return apilive.Document{
			Granular: func() *apilive.Granular {
				granular := vocabulary()
				granular.PublicAnonymous = &apilive.PublicAnonymous{
					Source: apilive.PublicSourceUnsaved, Project: []string{"read_code", "read_project"}, Group: []string{"read_project"},
				}
				return granular
			}(),
			GraphQLAuthz: &apilive.GraphQLAuthz{
				Types: map[string]apilive.GraphQLType{
					"Issue": {ObjectFields: map[string]apilive.ObjectField{
						"author":   {Type: "UserCore"},
						"notes":    {Type: "NoteConnection!", Connection: true},
						"assignee": {Type: "[Assignee!]"},
					}},
					"UserCore":       {},
					"NoteConnection": {},
				},
				Abstract: map[string]apilive.AbstractType{"Assignee": {Kind: "union", PossibleTypes: []string{"UserCore"}}},
			},
		}
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*apilive.Document)
		want   []string
	}{
		{name: "fields leading to objects and to abstract types, and a public set from the vocabulary, pass", mutate: func(*apilive.Document) {}},
		{
			name: "every public source passes",
			mutate: func(d *apilive.Document) {
				d.Granular.PublicAnonymous.Source = apilive.PublicSourceFixture
			},
		},
		{name: "no public set passes", mutate: func(d *apilive.Document) { d.Granular.PublicAnonymous = nil }},
		{name: "no vocabulary is not judged here", mutate: func(d *apilive.Document) { d.Granular = nil }},
		{name: "no GraphQL half is not judged here", mutate: func(d *apilive.Document) { d.GraphQLAuthz = nil }},
		{
			name: "an abstract type with no possible types",
			mutate: func(d *apilive.Document) {
				d.GraphQLAuthz.Abstract["Node"] = apilive.AbstractType{Kind: "interface"}
			},
			want: []string{"1 unions or interfaces record no possible types (Node): a position of one cannot be judged by its members"},
		},
		{
			name: "a field leading to a type the record does not describe",
			mutate: func(d *apilive.Document) {
				issue := d.GraphQLAuthz.Types["Issue"]
				issue.ObjectFields["epic"] = apilive.ObjectField{Type: "[Epic!]!"}
				d.GraphQLAuthz.Types["Issue"] = issue
			},
			want: []string{"1 object-typed fields name a type the record describes neither as an object nor as a union or " +
				"interface (Issue.epic Epic): a walk reaching one cannot go on"},
		},
		{
			name:   "a public set naming no source",
			mutate: func(d *apilive.Document) { d.Granular.PublicAnonymous.Source = "" },
			want: []string{`the public anonymous set names its source as "", which is none of unsaved, persisted and fixture: ` +
				"a reader cannot tell how wide it is"},
		},
		{
			name:   "a public set naming the role file",
			mutate: func(d *apilive.Document) { d.Granular.PublicAnonymous.Source = "role-file" },
			want: []string{"the public anonymous set names its source as role-file, which is none of unsaved, persisted and fixture: " +
				"a reader cannot tell how wide it is"},
		},
		{
			name: "a public set holding what no assignable expands to",
			mutate: func(d *apilive.Document) {
				d.Granular.PublicAnonymous.Project = append(d.Granular.PublicAnonymous.Project, "orphan_raw")
				d.Granular.PublicAnonymous.Group = append(d.Granular.PublicAnonymous.Group, "read_nothing")
			},
			want: []string{"the public anonymous set holds 2 permissions no assignable permission expands to " +
				"(group read_nothing, project orphan_raw): GitLab's public bypass is defined over none of them"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			doc := whole()
			testCase.mutate(&doc)
			assertProblems(t, shapeProblems(doc), testCase.want)
		})
	}
}

// TestAuthorizationProblems_CollectsEveryRuleInTheOrderAMaintainerReadsThem
// verifies that the aggregate carries what each rule reports, in a fixed
// order, rather than the first rule's alone.
func TestAuthorizationProblems_CollectsEveryRuleInTheOrderAMaintainerReadsThem(t *testing.T) {
	t.Parallel()
	payload := wholeEnough()
	payload.Routes[0].Authorization.UnknownKeys = []string{"weight"}
	payload.GraphQLAuthz.Abstract["Node"] = apilive.AbstractType{Kind: "interface"}
	payload.GraphQLAuthz.Types[typeName(0)] = apilive.GraphQLType{Granular: []apilive.Directive{{BoundaryType: "project"}}}
	payload.GraphQLAuthz.Todo = &apilive.Todo{Types: []string{"PageInfo"}}
	delete(payload.Granular.RawToAssignable, rawName(5))

	got := authorizationProblems(documentOf(payload))

	wantStarts := []string{
		"1 raw permissions an assignable permission expands to have no assignable named for them",
		"1 route authorizations carry keys nothing here reads",
		"1 GraphQL directives carry both permissions and a skip reason, or neither",
		"the undeclared GraphQL types computed with GitLab's todo rule differ",
		"1 unions or interfaces record no possible types",
	}
	if len(got) != len(wantStarts) {
		t.Fatalf("problems = %q, want %d of them", got, len(wantStarts))
	}
	for i, want := range wantStarts {
		t.Run(want, func(t *testing.T) {
			t.Parallel()
			if !strings.HasPrefix(got[i], want) {
				t.Errorf("problem %d = %q, want it to start %q", i, got[i], want)
			}
		})
	}
}
