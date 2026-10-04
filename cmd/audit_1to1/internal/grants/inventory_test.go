package grants

import (
	"reflect"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// row is one REST inventory row of a package.
func row(pkg, method, path string) requestinventory.Row {
	return requestinventory.Row{Package: pkg, Kind: requestinventory.KindREST, Method: method, Path: path}
}

// TestInventoryCheck_JoinsEachRowToItsPackagesDerivation verifies the
// package-grain cross-check: a row outside every owning package counted
// apart; a REST row matched to the record route its package derives, through
// a recorded literal where the route has a placeholder; a row placed through
// the derivation's own spelling of a route a declaration placed; a GraphQL row
// joined on its kind and root fields; a row no route meets and one its
// package derives nothing for, each a lead with its reason; and the derived
// requests no row records, each a lead with its actions.
func TestInventoryCheck_JoinsEachRowToItsPackagesDerivation(t *testing.T) {
	rows := []requestinventory.Row{
		row("internal/completions", "GET", "/projects"),
		row("internal/tools/issues", "GET", "/projects/:project_id/issues"),
		row("internal/tools/issues", "GET", "/projects/:project_id/integrations/slack"),
		row("internal/tools/issues", "GET", "/projects/:project_id/nowhere/at/all"),
		row("internal/tools/issues", "GET", "/namespaces"),
		{Package: "internal/tools/branchrules", Kind: requestinventory.KindGraphQL, Method: "POST", Path: "/graphql", Operation: "query BranchRules project"},
		{Package: "internal/tools/securityattributes", Kind: requestinventory.KindGraphQL, Method: "POST", Path: "/graphql", Operation: "mutation other"},
	}
	check := inventoryCheck(rows, fixtureRecord(), fixtureLive(), fixtureOwners())

	want := InventoryCheck{
		Grain: inventoryGrain, RowsOutsideOwners: 1,
		RESTRows: 4, RESTRowsDerived: 2, GraphQLRows: 2, GraphQLRowsDerived: 1,
		DerivedREST: 8, DerivedRESTRecorded: 2, DerivedGraphQL: 3, DerivedGraphQLRecorded: 1,
		NotDerived: []RowLead{
			{Package: "internal/tools/issues", Request: "GET /namespaces", Why: whyNotDerived},
			{Package: "internal/tools/issues", Request: "GET /projects/:project_id/nowhere/at/all", Why: whyNoRoute},
			{Package: "internal/tools/securityattributes", Request: "mutation other", Why: whyNotDerived},
		},
		NotRecorded: []RequestLead{
			{Package: "internal/tools/branches", Request: "GET /projects/:id/protected_branches", Actions: []string{"branch.protected_list"}},
			{Package: "internal/tools/issues", Request: "PATCH /projects/:id/issues", Actions: []string{"issue.bulk"}},
			{Package: "internal/tools/issues", Request: "POST /projects/:id/things", Actions: []string{"issue.thing"}},
			{Package: "internal/tools/issues", Request: "PUT /projects/:id/issues/:issue_iid", Actions: []string{"issue.update"}},
			{Package: "internal/tools/issues", Request: "query project", Actions: []string{"issue.list"}},
			{Package: "internal/tools/later", Request: "GET /later", Actions: []string{"later.get"}},
			{Package: "internal/tools/namespaces", Request: "GET /namespaces", Actions: []string{"namespace.list"}},
			{Package: "internal/tools/securityattributes", Request: "mutation bulkUpdate", Actions: []string{"security.bulk"}},
		},
	}
	if !reflect.DeepEqual(check, want) {
		t.Errorf("inventoryCheck =\n%+v\nwant\n%+v", check, want)
	}
}

// TestDerivedByPackage_KeysEachRequestUnderItsOwner verifies the derived
// requests of each owning package: actions of one package sharing a request
// are listed together and sorted, an unresolved request is left out, an
// action the catalog does not name is left out, and a route a declaration
// placed is also indexed by the derivation's own spelling.
func TestDerivedByPackage_KeysEachRequestUnderItsOwner(t *testing.T) {
	record := fixtureRecord()
	record.Actions = append(record.Actions, actionrequests.RecordAction{ID: "unowned", Requests: []actionrequests.RecordRequest{rest("GET /x", actionrequests.ClassMandatory)}})
	derived := derivedByPackage(record, fixtureOwners())

	if got := derived["internal/tools/issues"].keys["GET /projects/:id/issues"]; !slices.Equal(got, []string{"issue.get", "issue.update"}) {
		t.Errorf("GET issues is derived by %v, want both issue actions sorted", got)
	}
	if got := derived["internal/tools/namespaces"].keys; len(got) != 1 {
		t.Errorf("namespaces keys = %v, want only the route, the unresolved request left out", got)
	}
	if _, ok := derived[requestinventory.PackageName("")]; ok {
		t.Errorf("an action the catalog does not name was given a package")
	}
	placed, ok := derived["internal/tools/issues"].place(row("internal/tools/issues", "GET", "/projects/:project_id/integrations/harbor"))
	if !ok || placed != "GET /projects/:id/integrations/apple-app-store" {
		t.Errorf("place = %q, %t; want the declared route's key", placed, ok)
	}
	if _, other := derived["internal/tools/issues"].place(row("internal/tools/issues", "GET", "/projects/:project_id/other/x")); other {
		t.Errorf("a row no declared spelling meets was placed")
	}
	var none *packageRequests
	if _, placedByNone := none.place(row("internal/tools/x", "GET", "/x")); placedByNone || none.derives("GET /x") {
		t.Errorf("a package that derives nothing placed or derived a row")
	}
}

// TestRequestKey_KeysEachKindTheWayARowIsKeyed verifies a REST request is
// keyed by its record route, a GraphQL one by its kind and sorted root fields
// whatever its operation is named, and an unresolved one by nothing.
func TestRequestKey_KeysEachKindTheWayARowIsKeyed(t *testing.T) {
	cases := []struct {
		name    string
		request actionrequests.RecordRequest
		want    string
	}{
		{name: "rest", request: rest("GET /x", actionrequests.ClassMandatory), want: "GET /x"},
		{name: "graphql", request: graphQL("query b (doc)", actionrequests.ClassMandatory, "zeta", "alpha"), want: "query alpha,zeta"},
		{name: "unresolved", request: actionrequests.RecordRequest{Kind: actionrequests.KindUnresolved, Route: "GET /x", Reason: "raw"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := requestKey(&testCase.request); got != testCase.want {
				t.Errorf("requestKey = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestRowKey_PlacesAGraphQLRowByItsLabelAndARESTRowByTheRecord verifies a
// GraphQL row is keyed by its label's kind and root fields, and a REST row by
// the record route it meets, with the reason when it meets none.
func TestRowKey_PlacesAGraphQLRowByItsLabelAndARESTRowByTheRecord(t *testing.T) {
	index := apilive.NewRouteIndex(fixtureLive().Routes)
	cases := []struct {
		name              string
		row               requestinventory.Row
		key, request, why string
	}{
		{
			name: "graphql", row: requestinventory.Row{Kind: requestinventory.KindGraphQL, Operation: "query Named b,a"},
			key: "query a,b", request: "query Named b,a",
		},
		{name: "rest", row: row("p", "GET", "/projects/:project_id/issues"), key: "GET /projects/:id/issues", request: "GET /projects/:project_id/issues"},
		{name: "no_route", row: row("p", "DELETE", "/projects/:project_id/issues"), request: "DELETE /projects/:project_id/issues", why: whyNoRoute},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			key, request, why := rowKey(testCase.row, index)
			if key != testCase.key || request != testCase.request || why != testCase.why {
				t.Errorf("rowKey = %q, %q, %q; want %q, %q, %q", key, request, why, testCase.key, testCase.request, testCase.why)
			}
		})
	}
}

// TestParseOperationLabel_ReadsTheKindAndTheRootFields verifies the three
// shapes the test transport writes a label in: anonymous, named, and with
// several root fields; and a label of one word, which has no root fields to
// read.
func TestParseOperationLabel_ReadsTheKindAndTheRootFields(t *testing.T) {
	cases := []struct {
		label string
		kind  string
		roots []string
	}{
		{label: "query project", kind: "query", roots: []string{"project"}},
		{label: "mutation AwardAchievement achievementsAward", kind: "mutation", roots: []string{"achievementsAward"}},
		{label: "query Two group,project", kind: "query", roots: []string{"group", "project"}},
		{label: "query", kind: "query"},
	}
	for _, testCase := range cases {
		t.Run(testCase.label, func(t *testing.T) {
			kind, roots := parseOperationLabel(testCase.label)
			if kind != testCase.kind || !slices.Equal(roots, testCase.roots) {
				t.Errorf("parseOperationLabel = %q, %v; want %q, %v", kind, roots, testCase.kind, testCase.roots)
			}
		})
	}
}
