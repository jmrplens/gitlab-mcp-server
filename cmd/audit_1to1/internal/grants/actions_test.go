package grants

import (
	"reflect"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestViewActions_TheFixture_WordsEveryRowAndSortsTheWithheld verifies the
// per-action half of the report: every row worded with its owner and what
// shaped it, the withheld actions grouped by cause in the cause's order with
// their element and effect, the one partly withheld action, the one served
// with parts empty, and the counters.
func TestViewActions_TheFixture_WordsEveryRowAndSortsTheWithheld(t *testing.T) {
	table := fixtureTable()
	view := viewActions(table, fixtureRecord(), fixtureOwners())

	if len(view.grants) != len(table.Actions) {
		t.Fatalf("%d grants for %d rows", len(view.grants), len(table.Actions))
	}
	update := view.grants[slices.IndexFunc(view.grants, func(g ActionGrant) bool { return g.ID == "issue.update" })]
	wantUpdate := ActionGrant{
		ID: "issue.update", Owner: "issues",
		AnyOf: []finegrained.Way{{Needs: []finegrained.Need{
			{Permissions: []string{"Issue: Read"}, At: []string{"project"}},
			{Permissions: []string{"Issue: Update"}, At: []string{"project", "group"}},
		}}},
		ShapedBy: []Shaping{{
			Request: "PUT /projects/:id/issues/:issue_iid", Class: actionrequests.ClassMandatory,
			Directives: []string{"mandatory: the update runs after the read"},
		}},
	}
	if !reflect.DeepEqual(update, wantUpdate) {
		t.Errorf("issue.update = %+v\nwant %+v", update, wantUpdate)
	}

	wantCauses := []CauseGroup{
		{Cause: finegrained.CauseMutationUndeclared, GraphQL: true, Count: 1, Actions: []Withheld{{ID: "security.bulk", Element: "bulkUpdate", Effect: finegrained.EffectRefused}}},
		{Cause: finegrained.CauseTypeUndeclared, GraphQL: true, Count: 1, Actions: []Withheld{{ID: "branch.rule_list", Element: "BranchRule", Effect: finegrained.EffectRemoved}}},
		{Cause: finegrained.CauseRESTTodo, Count: 1, Actions: []Withheld{{ID: "later.get", Element: "GET /later", Effect: finegrained.EffectRefused}}},
	}
	if !reflect.DeepEqual(view.phaseA.ByCause, wantCauses) || view.phaseA.GitLabVersion != "19.4.1" {
		t.Errorf("phase A = %q %+v\nwant 19.4.1 %+v", view.phaseA.GitLabVersion, view.phaseA.ByCause, wantCauses)
	}
	if len(view.phaseA.PartlyWithheld) != 1 || view.phaseA.PartlyWithheld[0].ID != "namespace.list" || len(view.phaseA.PartlyWithheld[0].DeniedWays) != 2 {
		t.Errorf("partly withheld = %+v, want namespace.list with its two ways", view.phaseA.PartlyWithheld)
	}
	wantDegraded := []DegradedAction{{
		ID: "issue.list", AlwaysEmpty: []string{"project { issues { nodes { links } } }"},
		EmptyWithout: []finegrained.Position{{
			Selection: "project { issues { nodes { author } } }",
			Needs:     []finegrained.Need{{Permissions: []string{"Namespace: Read"}, At: []string{"group"}}},
		}},
	}}
	if !reflect.DeepEqual(view.degraded, wantDegraded) {
		t.Errorf("degraded = %+v\nwant %+v", view.degraded, wantDegraded)
	}
	if view.notJudged != 1 || view.byDirective != 1 || view.byDeclaration != 3 {
		t.Errorf("counters = %d not judged, %d by directive, %d by declaration; want 1, 1, 3", view.notJudged, view.byDirective, view.byDeclaration)
	}
}

// TestViewActions_EitherEmptyPart_DegradesTheAction verifies an action is
// listed as served with a part empty when only a part always empty applies
// and when only a part empty without more of the grant does, not only when
// both do.
func TestViewActions_EitherEmptyPart_DegradesTheAction(t *testing.T) {
	table := fixtureTable()
	table.Actions = []finegrained.Requirement{
		{ID: "only.always", Paths: [][]uint32{{0}}, Degraded: []uint32{2}},
		{ID: "only.without", Paths: [][]uint32{{3}}},
		{ID: "neither", Paths: [][]uint32{{0}}},
	}
	view := viewActions(table, actionrequests.Record{}, nil)
	ids := make([]string, 0, len(view.degraded))
	for _, degraded := range view.degraded {
		ids = append(ids, degraded.ID)
	}
	if !slices.Equal(ids, []string{"only.always", "only.without"}) {
		t.Errorf("degraded = %v, want only.always and only.without", ids)
	}
}

// TestViewActions_ARowTheRecordLacks_IsWordedWithoutWhatShapedIt verifies a
// row with no entry in the request record still gets its requirement, with
// no shaping and no declaration, which the gate reports separately.
func TestViewActions_ARowTheRecordLacks_IsWordedWithoutWhatShapedIt(t *testing.T) {
	view := viewActions(fixtureTable(), actionrequests.Record{}, nil)
	for _, grant := range view.grants {
		if grant.ShapedBy != nil || grant.Declaration != "" || grant.Owner != "" {
			t.Errorf("%s = %+v, want no shaping, declaration or owner", grant.ID, grant)
		}
	}
	if view.byDirective != 0 || view.byDeclaration != 0 {
		t.Errorf("counters = %d, %d; want none without a record", view.byDirective, view.byDeclaration)
	}
}

// TestShapings_KeepsOnlyWhatAPersonQualified verifies a request with neither
// a directive nor a declaration is left out, and one with either is named the
// way its kind is looked up.
func TestShapings_KeepsOnlyWhatAPersonQualified(t *testing.T) {
	declared := graphQL("query project (x)", actionrequests.ClassOptional, "project")
	declared.Declaration = "sdk-graphql-template"
	action := &actionrequests.RecordAction{Requests: []actionrequests.RecordRequest{
		rest("GET /plain", actionrequests.ClassMandatory),
		declared,
		{Kind: actionrequests.KindUnresolved, Reason: "raw-path pkg.F", Class: actionrequests.ClassMandatory, Directives: []string{"mandatory: why"}},
	}}
	want := []Shaping{
		{Request: "query project (x)", Class: actionrequests.ClassOptional, Declaration: "sdk-graphql-template"},
		{Request: "raw-path pkg.F", Class: actionrequests.ClassMandatory, Directives: []string{"mandatory: why"}},
	}
	if got := shapings(action); !reflect.DeepEqual(got, want) {
		t.Errorf("shapings = %+v\nwant %+v", got, want)
	}
}

// TestRequestName_NamesEachKindByWhatALookupUses verifies a REST request is
// named by its route, a GraphQL one by its operation and an unresolved one by
// its reason.
func TestRequestName_NamesEachKindByWhatALookupUses(t *testing.T) {
	cases := []struct {
		name    string
		request actionrequests.RecordRequest
		want    string
	}{
		{name: "rest", request: actionrequests.RecordRequest{Kind: actionrequests.KindREST, Route: "GET /x", Operation: "no", Reason: "no"}, want: "GET /x"},
		{name: "graphql", request: actionrequests.RecordRequest{Kind: actionrequests.KindGraphQL, Route: "no", Operation: "query x (y)", Reason: "no"}, want: "query x (y)"},
		{name: "unresolved", request: actionrequests.RecordRequest{Kind: actionrequests.KindUnresolved, Route: "no", Operation: "no", Reason: "raw-path p.F"}, want: "raw-path p.F"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := requestName(&testCase.request); got != testCase.want {
				t.Errorf("requestName = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestPublicOperations_ListsWhatAPublicResourceServesWithNoGrant verifies the
// REST operations every requirement of which the anonymous policy grants at a
// boundary it may be held at: one at the project, one at the group, and one
// whose additional scope is public at another boundary than its primary
// requirement. A skipped route, its groups public or not, a GraphQL
// operation, a route whose primary requirement is not public, one whose
// additional scope is not, and one holding a public permission beside a
// private one are left out; a table carrying no evaluated set lists nothing.
func TestPublicOperations_ListsWhatAPublicResourceServesWithNoGrant(t *testing.T) {
	table := fixtureTable()
	table.Operations = append(table.Operations,
		finegrained.Operation{Name: "POST /projects/:id/other", Groups: []uint32{0, 1}},
		finegrained.Operation{Name: "GET /projects/:id/skipped", Groups: []uint32{0}, Skip: true},
	)
	// z.other reaches the public listing twice, through two of its ways, and
	// is named once.
	table.Actions = append(table.Actions, finegrained.Requirement{ID: "z.other", Paths: [][]uint32{{0}, {0, 8}}})
	want := []PublicOperation{
		{Operation: "GET /projects/:id/issues", At: []string{"project"}, Actions: []string{"issue.get", "issue.update", "z.other"}},
		{Operation: "GET /namespaces", At: []string{"group"}, Actions: []string{"namespace.list"}},
		{Operation: "POST /projects/:id/things", At: []string{"project"}, Actions: []string{"issue.thing"}},
	}
	if got := publicOperations(table); !reflect.DeepEqual(got, want) {
		t.Errorf("publicOperations =\n%+v\nwant\n%+v", got, want)
	}
	table.PublicKnown = false
	if got := publicOperations(table); got != nil {
		t.Errorf("publicOperations with no evaluated set = %+v, want none", got)
	}
}

// TestPublicAt_ReadsEachBoundaryAGroupMayBeHeldAt verifies a group public at
// both boundaries it may be held at, at the one of them its permissions are
// public on, and at neither when the boundary it is held at has no public set
// of it.
func TestPublicAt_ReadsEachBoundaryAGroupMayBeHeldAt(t *testing.T) {
	table := fixtureTable()
	table.PublicAnonymous[finegrained.PublicGroup] = []uint64{1<<permReadIssue | 1<<permReadNamespace}
	table.Groups = append(table.Groups,
		finegrained.Group{Perms: []uint16{permReadIssue}, Any: finegrained.BoundaryProject | finegrained.BoundaryGroup},
		finegrained.Group{Perms: []uint16{permReadNamespace}, Any: finegrained.BoundaryProject | finegrained.BoundaryGroup},
		finegrained.Group{Perms: []uint16{permReadIssue}, Any: finegrained.BoundaryUser},
	)
	cases := []struct {
		name  string
		group uint32
		want  finegrained.Boundary
	}{
		{name: "both", group: 5, want: finegrained.BoundaryProject | finegrained.BoundaryGroup},
		{name: "group_only", group: 6, want: finegrained.BoundaryGroup},
		{name: "a_boundary_with_no_public_set", group: 7},
		{name: "not_every_permission", group: 3},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := publicAt(table, testCase.group); got != testCase.want {
				t.Errorf("publicAt(%d) = %v, want %v", testCase.group, got, testCase.want)
			}
		})
	}
}

// TestGraphQLOperation_TellsAnOperationFromARoute verifies each operation
// kind GraphQL has, and a route whose verb is not one.
func TestGraphQLOperation_TellsAnOperationFromARoute(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{name: "query project (q)", want: true},
		{name: "mutation createNote (m)", want: true},
		{name: "subscription updated (s)", want: true},
		{name: "GET /projects/:id"},
		{name: "queryless"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := graphQLOperation(testCase.name); got != testCase.want {
				t.Errorf("graphQLOperation(%q) = %t, want %t", testCase.name, got, testCase.want)
			}
		})
	}
}
