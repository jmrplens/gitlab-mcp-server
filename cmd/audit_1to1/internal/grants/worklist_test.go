package grants

import (
	"reflect"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestWorklist_TheFixture_ListsEachUndeclaredElementWithItsLeads verifies
// the upstream register's row 87 worklist over the fixture: a type that
// withholds an action, a type that withholds one way of another, an
// undeclared position served
// empty, and a mutation, each with what it is matched on and the routes this
// server already calls that declare it. A REST denial is not GraphQL's and is
// left out. Where a route of the affected package declares the permission
// only those are listed (Namespace keeps its own package's route and drops
// the issues' additional scope); where none does every caller is (BranchRule's
// lead is the branches package's).
func TestWorklist_TheFixture_ListsEachUndeclaredElementWithItsLeads(t *testing.T) {
	want := []WorklistEntry{
		{
			Element: "BranchRule", Kind: kindType, Causes: []finegrained.Cause{finegrained.CauseTypeUndeclared},
			Blocks: []string{"branch.rule_list"}, Abilities: []string{"read_protected_branch"}, Resource: "branch_rule",
			RESTLeads: []Lead{{Route: "GET /projects/:id/protected_branches", Packages: []string{"branches"}, Permissions: []string{"read_protected_branch"}}},
		},
		{
			Element: "Namespace", Kind: kindType, Causes: []finegrained.Cause{finegrained.CauseTypeUndeclared},
			BlocksAWayOf: []string{"namespace.list"}, Abilities: []string{"read_namespace"}, Resource: "namespace",
			RESTLeads: []Lead{{Route: "GET /namespaces", Packages: []string{"namespaces"}, SamePackage: true, Permissions: []string{"read_namespace"}}},
		},
		{
			Element: "VulnerabilityIssueLink", Kind: kindType, Causes: []finegrained.Cause{finegrained.CauseTypeUndeclared},
			EmptiesPartOf: []string{"issue.list"}, Abilities: []string{"read_issue"},
			RESTLeads: []Lead{
				{Route: "GET /projects/:id/integrations/apple-app-store", Packages: []string{"issues"}, SamePackage: true, Permissions: []string{"read_issue"}},
				{Route: "GET /projects/:id/issues", Packages: []string{"issues"}, SamePackage: true, Permissions: []string{"read_issue"}},
				{Route: "PATCH /projects/:id/issues", Packages: []string{"issues"}, SamePackage: true, Permissions: []string{"read_issue"}},
				{Route: "POST /projects/:id/things", Packages: []string{"issues"}, SamePackage: true, Permissions: []string{"read_issue"}},
			},
		},
		{
			Element: "bulkUpdate", Kind: kindMutation, Causes: []finegrained.Cause{finegrained.CauseMutationUndeclared},
			Blocks: []string{"security.bulk"},
		},
	}
	got := worklist(fixtureTable(), fixtureRecord(), fixtureLive(), fixtureOwners())
	if !reflect.DeepEqual(got, want) {
		t.Errorf("worklist =\n%+v\nwant\n%+v", got, want)
	}
}

// TestWorklist_OneElementSeveralWays_IsOneEntry verifies an element met
// several ways is one entry: each cause once and sorted, each action once in
// each list, an action two of whose ways it denies named once.
func TestWorklist_OneElementSeveralWays_IsOneEntry(t *testing.T) {
	way := finegrained.Denial{Cause: finegrained.CauseTypeUndeclared, Element: "Thing"}
	table := &finegrained.Table{
		Elements: []finegrained.Element{{Path: "thing", Type: "Thing", Undeclared: true}},
		Actions: []finegrained.Requirement{
			{ID: "a.one", Denied: &finegrained.Denial{Cause: finegrained.CausePayloadUndeclared, Element: "Thing"}},
			{ID: "a.two", Denied: &finegrained.Denial{Cause: finegrained.CauseTypeUndeclared, Element: "Thing"}},
			{ID: "a.three", DeniedWays: []finegrained.Denial{way, way}, Degraded: []uint32{0, 0}},
		},
	}
	got := worklist(table, actionrequests.Record{}, &apilive.Document{}, nil)
	want := []WorklistEntry{{
		Element: "Thing", Kind: kindType,
		Causes:       []finegrained.Cause{finegrained.CausePayloadUndeclared, finegrained.CauseTypeUndeclared},
		Blocks:       []string{"a.one", "a.two"},
		BlocksAWayOf: []string{"a.three"}, EmptiesPartOf: []string{"a.three"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("worklist =\n%+v\nwant\n%+v", got, want)
	}
}

// TestElementKind_ReadsHowTheRecordHoldsAnElement verifies a mutation, a
// union or interface, an object type, and anything at all when the record
// carries no GraphQL authorization.
func TestElementKind_ReadsHowTheRecordHoldsAnElement(t *testing.T) {
	live := fixtureLive()
	cases := []struct {
		name    string
		element string
		live    *apilive.Document
		want    string
	}{
		{name: "mutation", element: "bulkUpdate", live: live, want: kindMutation},
		{name: "abstract", element: "Noteable", live: live, want: kindAbstract},
		{name: "type", element: "BranchRule", live: live, want: kindType},
		{name: "no_authorization", element: "bulkUpdate", live: &apilive.Document{}, want: kindType},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := elementKind(testCase.live, testCase.element); got != testCase.want {
				t.Errorf("elementKind(%q) = %q, want %q", testCase.element, got, testCase.want)
			}
		})
	}
}

// TestSnakeCase_SpellsAGraphQLNameAsAResource verifies a type name, a
// mutation name starting lower case, and a name of one word.
func TestSnakeCase_SpellsAGraphQLNameAsAResource(t *testing.T) {
	cases := map[string]string{
		"BranchRule":                   "branch_rule",
		"bulkUpdateSecurityAttributes": "bulk_update_security_attributes",
		"Namespace":                    "namespace",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := snakeCase(name); got != want {
				t.Errorf("snakeCase(%q) = %q, want %q", name, got, want)
			}
		})
	}
}

// TestLeadFinder_MatchesOnAbilitiesOrTheElementsOwnResource verifies the two
// ways a route is a lead, and the ones that are not: an ability declared
// verbatim; the element's own resource, reached through an assignable a
// token cannot be granted when no available one names the permission; a
// called route the record does not hold, one declaring nothing, and one
// whose permission belongs to no assignable, which an element with no
// resource of its own must not match on an empty resource; a caller the
// catalog does not name, whose routes are no lead.
func TestLeadFinder_MatchesOnAbilitiesOrTheElementsOwnResource(t *testing.T) {
	live := fixtureLive()
	live.Routes = append(live.Routes, route("GET", "/odd", held("project", "read_odd")))
	record := actionrequests.Record{Actions: []actionrequests.RecordAction{
		{ID: "n.list", Requests: []actionrequests.RecordRequest{
			rest("GET /namespaces", actionrequests.ClassMandatory),
			rest("GET /gone", actionrequests.ClassMandatory),
			rest("GET /nothing", actionrequests.ClassMandatory),
			rest("GET /odd", actionrequests.ClassMandatory),
			graphQL("query namespace (q)", actionrequests.ClassMandatory, "namespace"),
		}},
		{ID: "unowned", Requests: []actionrequests.RecordRequest{rest("GET /projects/:id/issues", actionrequests.ClassMandatory)}},
	}}
	finder := newLeadFinder(record, live, map[string]string{"n.list": "namespaces"})

	t.Run("by_resource_alone", func(t *testing.T) {
		e := &WorklistEntry{Element: "Namespace", Resource: "namespace"}
		want := []Lead{{Route: "GET /namespaces", Packages: []string{"namespaces"}, Permissions: []string{"read_namespace"}}}
		if got := finder.leads(e); !reflect.DeepEqual(got, want) {
			t.Errorf("leads = %+v, want %+v", got, want)
		}
	})
	t.Run("no_resource_no_ability", func(t *testing.T) {
		if got := finder.leads(&WorklistEntry{Element: "Thing"}); got != nil {
			t.Errorf("leads = %+v, want none for an element matched on nothing", got)
		}
	})
	t.Run("an_unowned_caller", func(t *testing.T) {
		if got := finder.leads(&WorklistEntry{Element: "Issue", Abilities: []string{"read_issue"}}); got != nil {
			t.Errorf("leads = %+v, want none from a caller the catalog does not name", got)
		}
	})
}

// TestNewLeadFinder_NoVocabulary_MatchesOnAbilitiesAlone verifies a record
// with no permission vocabulary knows no resource, so an element is matched
// on its abilities and its own name never reads as one.
func TestNewLeadFinder_NoVocabulary_MatchesOnAbilitiesAlone(t *testing.T) {
	live := fixtureLive()
	live.Granular = nil
	finder := newLeadFinder(fixtureRecord(), live, fixtureOwners())
	abilities, resource := finder.describe("Namespace")
	if !slices.Equal(abilities, []string{"read_namespace"}) || resource != "" {
		t.Errorf("describe(Namespace) = %v, %q; want its ability and no resource", abilities, resource)
	}
	live.GraphQLAuthz = nil
	if bare, _ := newLeadFinder(fixtureRecord(), live, fixtureOwners()).describe("Namespace"); bare != nil {
		t.Errorf("describe with no GraphQL authorization = %v, want no abilities", bare)
	}
}
