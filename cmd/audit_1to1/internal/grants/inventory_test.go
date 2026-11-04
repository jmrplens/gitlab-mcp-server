// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package grants

import (
	"reflect"
	"slices"
	"strings"
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
	useSlashes(t)
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
	useSlashes(t)
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

// useSlashes declares placeholders that take a slash for one test.
func useSlashes(t *testing.T, declared ...apilive.SlashPlaceholder) {
	t.Helper()
	original := slashPlaceholders
	t.Cleanup(func() { slashPlaceholders = original })
	slashPlaceholders = declared
}

// artifactLive is a record holding the routes a recorded path needs a segment
// to take several of its own to meet, and Grape's catch-all, which any path
// would.
func artifactLive() *apilive.Document {
	return &apilive.Document{Routes: []apilive.Route{
		route("GET", "/projects/:id/jobs/:job_id/artifacts/*artifact_path", held("project", "read_build")),
		route("GET", "/projects/:id/jobs/artifacts/:ref_name/download", held("project", "read_build")),
		route("PUT", "/projects/:id/packages/generic/:package_name/*package_version/(*path/):file_name", held("project", "create_package")),
		route("GET", "/*path", nil),
	}}
}

// refDownload declares the placeholder of artifactLive's download route that
// takes a slash.
var refDownload = apilive.SlashPlaceholder{Route: "GET /projects/:id/jobs/artifacts/:ref_name/download", Param: "ref_name", Reason: "a requirement"}

// TestInventoryCheck_ASplatHoldsWhatARowRecords verifies the row a one-segment
// reading could not place: a splat holding several segments of a recorded path
// joins the route the package derives, an optional group taken, and a
// placeholder takes a slash only under a declaration. A path only the
// catch-all meets, and an extra segment where the route has no splat, stay
// leads.
func TestInventoryCheck_ASplatHoldsWhatARowRecords(t *testing.T) {
	const (
		jobs    = "internal/tools/jobs"
		packs   = "internal/tools/packages"
		artFile = "GET /projects/:id/jobs/:job_id/artifacts/*artifact_path"
		pkgFile = "PUT /projects/:id/packages/generic/:package_name/*package_version/(*path/):file_name"
	)
	record := actionrequests.Record{Actions: []actionrequests.RecordAction{
		{ID: "job.artifact", Requests: []actionrequests.RecordRequest{rest(artFile, actionrequests.ClassMandatory)}},
		{ID: "job.download", Requests: []actionrequests.RecordRequest{rest(refDownload.Route, actionrequests.ClassMandatory)}},
		{ID: "package.publish", Requests: []actionrequests.RecordRequest{rest(pkgFile, actionrequests.ClassMandatory)}},
	}}
	owners := map[string]string{"job.artifact": "jobs", "job.download": "jobs", "package.publish": "packages"}
	rows := []requestinventory.Row{
		row(jobs, "GET", "/projects/:project_id/jobs/:job_id/artifacts/build/output/report.txt"),
		row(jobs, "GET", "/projects/:project_id/jobs/artifacts/feature/my-branch/download"),
		row(packs, "PUT", "/projects/:project_id/packages/generic/pkg/1.0.0/dist/linux-amd64/tool.bin"),
		row(jobs, "GET", "/anything/at/all"),
		row(jobs, "GET", "/projects/:project_id/jobs/artifacts/feature/download/extra"),
	}
	cases := []struct {
		name     string
		declared []apilive.SlashPlaceholder
		derived  int
		leads    []RowLead
	}{
		{
			name: "undeclared", derived: 2,
			leads: []RowLead{
				{Package: jobs, Request: "GET /anything/at/all", Why: whyNoRoute},
				{Package: jobs, Request: "GET /projects/:project_id/jobs/artifacts/feature/download/extra", Why: whyNoRoute},
				{Package: jobs, Request: "GET /projects/:project_id/jobs/artifacts/feature/my-branch/download", Why: whyNoRoute},
			},
		},
		{
			name: "declared", declared: []apilive.SlashPlaceholder{refDownload}, derived: 3,
			leads: []RowLead{
				{Package: jobs, Request: "GET /anything/at/all", Why: whyNoRoute},
				{Package: jobs, Request: "GET /projects/:project_id/jobs/artifacts/feature/download/extra", Why: whyNoRoute},
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			useSlashes(t, testCase.declared...)
			check := inventoryCheck(rows, record, artifactLive(), owners)
			if check.RESTRowsDerived != testCase.derived || !reflect.DeepEqual(check.NotDerived, testCase.leads) {
				t.Errorf("derived %d, leads %+v; want %d, %+v", check.RESTRowsDerived, check.NotDerived, testCase.derived, testCase.leads)
			}
		})
	}
}

// TestStaleSlashes_NamesADeclarationNoRowGoesThrough verifies a declaration
// answers something only when a recorded row is placed through it: not a row a
// spelling of its own length meets, not a row a splat placed, not a declaration
// of a route the record does not mount, and not an empty inventory.
func TestStaleSlashes_NamesADeclarationNoRowGoesThrough(t *testing.T) {
	index := apilive.NewRouteIndex(artifactLive().Routes)
	unmounted := apilive.SlashPlaceholder{Route: "GET /nowhere/:ref_name/download", Param: "ref_name", Reason: "a requirement"}
	slashed := row("internal/tools/jobs", "GET", "/projects/:project_id/jobs/artifacts/feature/my-branch/download")
	plain := row("internal/tools/jobs", "GET", "/projects/:project_id/jobs/artifacts/main/download")
	splat := row("internal/tools/jobs", "GET", "/projects/:project_id/jobs/:job_id/artifacts/a/b")
	cases := []struct {
		name     string
		rows     []requestinventory.Row
		declared []apilive.SlashPlaceholder
		stale    []string
	}{
		{name: "answered", rows: []requestinventory.Row{plain, slashed}, declared: []apilive.SlashPlaceholder{refDownload}},
		{name: "own_length", rows: []requestinventory.Row{plain}, declared: []apilive.SlashPlaceholder{refDownload}, stale: []string{refDownload.Route}},
		{name: "a_splat", rows: []requestinventory.Row{splat}, declared: []apilive.SlashPlaceholder{refDownload}, stale: []string{refDownload.Route}},
		{name: "no_rows", declared: []apilive.SlashPlaceholder{refDownload}, stale: []string{refDownload.Route}},
		{name: "unmounted", rows: []requestinventory.Row{slashed}, declared: []apilive.SlashPlaceholder{refDownload, unmounted}, stale: []string{unmounted.Route}},
		{name: "none_declared", rows: []requestinventory.Row{slashed}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			useSlashes(t, testCase.declared...)
			got := staleSlashes(testCase.rows, index)
			if len(got) != len(testCase.stale) {
				t.Fatalf("staleSlashes = %q, want one each for %q", got, testCase.stale)
			}
			for i, route := range testCase.stale {
				if !strings.Contains(got[i], ":ref_name on "+route+" answers nothing") {
					t.Errorf("staleSlashes[%d] = %q, want it to name %s", i, got[i], route)
				}
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
