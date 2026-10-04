package grants

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/freshness"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// The fixture is one small GitLab and one small server, consistent with each
// other, which each test bends in the one place it is about.
//
// Permissions, in the table's sorted order: read_issue (0), read_namespace
// (1), read_protected_branch (2), update_issue (3).
const (
	permReadIssue = iota
	permReadNamespace
	permReadProtectedBranch
	permUpdateIssue
)

// fixtureTable is the table the fixture record joins to.
func fixtureTable() *finegrained.Table {
	return &finegrained.Table{
		Version:     "19.4.1-ee",
		Bucket:      "19.4",
		Permissions: []string{"read_issue", "read_namespace", "read_protected_branch", "update_issue"},
		Displays:    []string{"", "Branch Rule: Read", "Issue: Read", "Issue: Update", "Namespace: Read"},
		Display:     []uint16{2, 4, 1, 3},
		PublicAnonymous: [2][]uint64{
			finegrained.PublicProject: {1 << permReadIssue},
			finegrained.PublicGroup:   {1 << permReadNamespace},
		},
		PublicKnown: true,
		Groups: []finegrained.Group{
			{Perms: []uint16{permReadIssue}, Any: finegrained.BoundaryProject},
			{Perms: []uint16{permUpdateIssue}, Any: finegrained.BoundaryProject | finegrained.BoundaryGroup},
			{Perms: []uint16{permReadNamespace}, Any: finegrained.BoundaryGroup},
			{Perms: []uint16{permReadIssue, permUpdateIssue}, Any: finegrained.BoundaryProject},
			{Perms: []uint16{permReadProtectedBranch}, Any: finegrained.BoundaryProject},
		},
		Operations: []finegrained.Operation{
			{Name: "GET /projects/:id/issues", Groups: []uint32{0}},
			{Name: "PUT /projects/:id/issues/:issue_iid", Groups: []uint32{1}},
			{Name: "GET /namespaces", Groups: []uint32{2}},
			{Name: "query project (issuesQuery)", Spine: []uint32{0}, OffSpine: []uint32{1}},
			{Name: "GET /topics", Skip: true},
			{Name: "POST /projects/:id/things", Groups: []uint32{0, 2}},
			{Name: "PATCH /projects/:id/issues", Groups: []uint32{3}},
			{Name: "GET /projects/:id/protected_branches", Groups: []uint32{4}},
		},
		Elements: []finegrained.Element{
			{Path: "project", Type: "Project", Groups: []uint32{0}},
			{Path: "project.issues.nodes.author", Type: "UserCore", Groups: []uint32{2}},
			{Path: "project.issues.nodes.links", Type: "VulnerabilityIssueLink", Undeclared: true, Effect: finegrained.EffectRemoved},
		},
		Actions: []finegrained.Requirement{
			{ID: "branch.protected_list", Paths: [][]uint32{{7}}},
			{ID: "branch.rule_list", Denied: &finegrained.Denial{Cause: finegrained.CauseTypeUndeclared, Element: "BranchRule", Effect: finegrained.EffectRemoved}, GraphQL: true, Collection: true},
			{ID: "issue.bulk", Paths: [][]uint32{{6}}},
			{ID: "issue.get", Paths: [][]uint32{{0}}},
			{ID: "issue.list", Paths: [][]uint32{{3}}, Degraded: []uint32{2}, GraphQL: true, Collection: true},
			{ID: "issue.thing", Paths: [][]uint32{{5}}},
			{ID: "issue.update", Paths: [][]uint32{{0, 1}}},
			{ID: "later.get", Denied: &finegrained.Denial{Cause: finegrained.CauseRESTTodo, Element: "GET /later", Effect: finegrained.EffectRefused}},
			{
				ID: "namespace.list", Paths: [][]uint32{{2}},
				DeniedWays: []finegrained.Denial{
					{Cause: finegrained.CauseTypeUndeclared, Element: "Namespace", Effect: finegrained.EffectNull},
					{Cause: finegrained.CauseRESTUndeclared, Element: "GET /nothing", Effect: finegrained.EffectRefused},
				},
				GraphQL: true,
			},
			{ID: "security.bulk", Denied: &finegrained.Denial{Cause: finegrained.CauseMutationUndeclared, Element: "bulkUpdate", Effect: finegrained.EffectRefused}, GraphQL: true},
			{ID: "topic.list", Paths: [][]uint32{{4}}},
		},
	}
}

// route is a record route mounted under the API prefix.
func route(method, path string, auth *apilive.RouteAuthorization) apilive.Route {
	return apilive.Route{Method: method, Path: apilive.EndpointPrefix + path, Authorization: auth}
}

// held is a route authorization of permissions at one boundary type.
func held(boundary string, permissions ...string) *apilive.RouteAuthorization {
	return &apilive.RouteAuthorization{Permissions: permissions, BoundaryType: boundary}
}

// assignable is one assignable permission of the fixture vocabulary.
func assignable(name, resource string, permissions ...string) apilive.Assignable {
	return apilive.Assignable{Name: name, Resource: resource, Permissions: permissions, AvailableFor: []string{"granular_access_token"}}
}

// fixtureLive is the live record the fixture table was joined from.
func fixtureLive() *apilive.Document {
	return &apilive.Document{
		Source: apilive.Source{Version: "19.4.1-ee"},
		Routes: []apilive.Route{
			route("GET", "/projects/:id/issues", held("project", "read_issue")),
			route("PUT", "/projects/:id/issues/:issue_iid", &apilive.RouteAuthorization{
				Permissions: []string{"update_issue"},
				Boundaries:  []apilive.Boundary{{BoundaryType: "project"}, {BoundaryType: "group"}},
			}),
			route("GET", "/namespaces", held("group", "read_namespace")),
			route("GET", "/topics", &apilive.RouteAuthorization{Skip: "catch_all"}),
			route("GET", "/later", &apilive.RouteAuthorization{Todo: "pending"}),
			route("GET", "/nothing", nil),
			route("POST", "/projects/:id/things", &apilive.RouteAuthorization{
				Permissions: []string{"read_issue"}, BoundaryType: "project",
				AdditionalScopes: []apilive.AdditionalScope{{Permissions: []string{"read_namespace"}, BoundaryType: "group"}},
			}),
			route("PATCH", "/projects/:id/issues", held("project", "read_issue", "update_issue")),
			route("GET", "/projects/:id/protected_branches", held("project", "read_protected_branch")),
			route("GET", "/projects/:id/integrations/apple-app-store", held("project", "read_issue")),
		},
		Granular: &apilive.Granular{
			Assignable: []apilive.Assignable{
				assignable("read_issue", "issue", "read_issue"),
				assignable("update_issue", "issue", "update_issue"),
				assignable("read_namespace", "namespace", "read_namespace"),
				assignable("read_branch_rule", "branch_rule", "read_protected_branch"),
			},
			RawToAssignable: map[string]apilive.AssignableMatch{
				"read_issue":            {First: "read_issue", FirstAvailable: "read_issue"},
				"update_issue":          {First: "update_issue", FirstAvailable: "update_issue"},
				"read_namespace":        {First: "read_namespace"},
				"read_protected_branch": {First: "read_branch_rule", FirstAvailable: "read_branch_rule"},
			},
			PublicAnonymous: &apilive.PublicAnonymous{Source: apilive.PublicSourceUnsaved, Project: []string{"read_issue"}, Group: []string{"read_namespace"}},
		},
		GraphQLAuthz: &apilive.GraphQLAuthz{
			Types: map[string]apilive.GraphQLType{
				"BranchRule":             {Enforced: true, Abilities: []string{"read_protected_branch"}},
				"Namespace":              {Enforced: true, Abilities: []string{"read_namespace"}},
				"VulnerabilityIssueLink": {Enforced: true, Abilities: []string{"read_issue"}},
			},
			Abstract:  map[string]apilive.AbstractType{"Noteable": {Kind: "interface", PossibleTypes: []string{"Issue"}}},
			Mutations: map[string]apilive.Mutation{"bulkUpdate": {Name: "BulkUpdate"}},
		},
	}
}

// rest and graphQL are recorded requests of each kind.
func rest(route, class string) actionrequests.RecordRequest {
	return actionrequests.RecordRequest{Kind: actionrequests.KindREST, Route: route, Class: class}
}

func graphQL(operation, class string, roots ...string) actionrequests.RecordRequest {
	return actionrequests.RecordRequest{Kind: actionrequests.KindGraphQL, Operation: operation, RootFields: roots, Class: class}
}

// fixtureRecord is the request record the fixture table was joined from.
func fixtureRecord() actionrequests.Record {
	mandatory := actionrequests.ClassMandatory
	directed := rest("PUT /projects/:id/issues/:issue_iid", mandatory)
	directed.Directives = []string{"mandatory: the update runs after the read"}
	placed := rest("GET /projects/:id/integrations/apple-app-store", actionrequests.ClassAlternative)
	placed.Derived = "GET /projects/:/integrations/:"
	placed.Declaration = "slug-route"
	return actionrequests.Record{Note: actionrequests.RecordNote, Actions: []actionrequests.RecordAction{
		{ID: "branch.protected_list", Requests: []actionrequests.RecordRequest{rest("GET /projects/:id/protected_branches", mandatory)}, Paths: [][]int{{0}}},
		{ID: "branch.rule_list", Requests: []actionrequests.RecordRequest{graphQL("query project (branchRules)", mandatory, "project")}, Paths: [][]int{{0}}},
		{ID: "issue.bulk", Requests: []actionrequests.RecordRequest{rest("PATCH /projects/:id/issues", mandatory)}, Paths: [][]int{{0}}},
		{ID: "issue.get", Requests: []actionrequests.RecordRequest{rest("GET /projects/:id/issues", mandatory), placed}, Paths: [][]int{{0}, {0, 1}}},
		{ID: "issue.list", Requests: []actionrequests.RecordRequest{graphQL("query project (issuesQuery)", mandatory, "project")}, Paths: [][]int{{0}}},
		{ID: "issue.thing", Requests: []actionrequests.RecordRequest{rest("POST /projects/:id/things", mandatory)}, Paths: [][]int{{0}}},
		{ID: "issue.update", Requests: []actionrequests.RecordRequest{rest("GET /projects/:id/issues", mandatory), directed}, Paths: [][]int{{0, 1}}},
		{ID: "later.get", Requests: []actionrequests.RecordRequest{rest("GET /later", mandatory)}, Paths: [][]int{{0}}},
		{ID: "namespace.list", Requests: []actionrequests.RecordRequest{rest("GET /namespaces", actionrequests.ClassAlternative), {Kind: actionrequests.KindUnresolved, Reason: "raw-path namespaces.List", Class: actionrequests.ClassAlternative, Declaration: "sdk-path-sprintf"}}, Paths: [][]int{{0}, {1}}},
		{ID: "security.bulk", Requests: []actionrequests.RecordRequest{graphQL("mutation bulkUpdate (bulkUpdate)", mandatory, "bulkUpdate")}, Paths: [][]int{{0}}},
		{ID: "topic.list", Declaration: "sends-nothing", Paths: [][]int{{}}},
	}}
}

// fixtureOwners maps each fixture action onto the package that owns it.
func fixtureOwners() map[string]string {
	return map[string]string{
		"branch.protected_list": "branches",
		"branch.rule_list":      "branchrules",
		"issue.bulk":            "issues",
		"issue.get":             "issues",
		"issue.list":            "issues",
		"issue.thing":           "issues",
		"issue.update":          "issues",
		"later.get":             "later",
		"namespace.list":        "namespaces",
		"security.bulk":         "securityattributes",
		"topic.list":            "topics",
	}
}

// fixtureCatalog is the catalog the fixture table was joined for, one action
// per row, sorted by ID as actionrequests.Catalog returns it.
func fixtureCatalog() []actionrequests.Action {
	var actions []actionrequests.Action
	for id, owner := range fixtureOwners() {
		actions = append(actions, actionrequests.Action{ID: id, Owner: owner})
	}
	slices.SortFunc(actions, func(a, b actionrequests.Action) int { return strings.Compare(a.ID, b.ID) })
	return actions
}

// useFixtures points every input seam at the fixture, restoring the real ones
// when the test ends.
func useFixtures(t *testing.T) {
	t.Helper()
	originalRecord, originalLive, originalInventory := readRecord, readLive, readInventory
	originalCatalog, originalTable, originalE2E := catalog, grantTable, readE2ECalls
	t.Cleanup(func() {
		readRecord, readLive, readInventory = originalRecord, originalLive, originalInventory
		catalog, grantTable, readE2ECalls = originalCatalog, originalTable, originalE2E
	})
	readRecord = func(string) (actionrequests.Record, error) { return fixtureRecord(), nil }
	readLive = func(string) (apilive.Document, error) { return *fixtureLive(), nil }
	readInventory = func(string) (requestinventory.Inventory, error) {
		return requestinventory.Inventory{Requests: []requestinventory.Row{
			{Package: "internal/tools/issues", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:project_id/issues"},
		}}, nil
	}
	catalog = func() ([]actionrequests.Action, error) { return fixtureCatalog(), nil }
	grantTable = fixtureTable
	readE2ECalls = func(string) ([]e2ecalls.Record, error) { return nil, nil }
}

// decode reads a report back.
func decode(t *testing.T, content []byte) Report {
	t.Helper()
	var report Report
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatalf("the report is not JSON: %v", err)
	}
	return report
}

// TestRun_TheFixture_ReportsEveryPartAndPassesTheGate verifies one run over a
// consistent fixture: the gate passes, every part is present, the summary
// counts what the parts hold, and the report ends with a newline.
func TestRun_TheFixture_ReportsEveryPartAndPassesTheGate(t *testing.T) {
	useFixtures(t)
	content, clean, err := Run("root", Options{})
	if err != nil || !clean {
		t.Fatalf("Run = clean %t, error %v; want a clean run", clean, err)
	}
	if !strings.HasSuffix(string(content), "}\n") {
		t.Errorf("the report does not end with a newline")
	}
	report := decode(t, content)
	if report.Requests != actionrequests.RecordPath || report.Record != "docs/development/gitlab-api-live.json" ||
		report.Inventory != requestinventory.Path || report.GitLabVersion != "19.4.1" {
		t.Errorf("inputs = %q, %q, %q at %q; want the three committed paths at 19.4.1",
			report.Requests, report.Record, report.Inventory, report.GitLabVersion)
	}
	want := Summary{
		Actions: 11, Reachable: 8, Withheld: 3, WithheldByGraphQL: 2, PartlyWithheld: 1, NotJudged: 1,
		Degraded: 1, ShapedByDirective: 1, ShapedByDeclaration: 3, PublicOperations: 3, PublicKnown: true,
		WorklistElements: 4, WorklistLeads: 3,
		InventoryRESTRows: 1, InventoryRESTRowsDerived: 1, DerivedREST: 8, DerivedRESTRecorded: 1,
		DerivedGraphQL: 3, DerivedGraphQLRecorded: 0,
	}
	if report.Summary != want {
		t.Errorf("Summary = %+v\nwant %+v", report.Summary, want)
	}
	if len(report.Actions) != 11 || len(report.Degraded) != 1 || len(report.PublicOperations) != 3 || report.Inconsistencies == nil {
		t.Errorf("parts = %d actions, %d degraded, %d public, inconsistencies %v; want the context listed and an empty gate list",
			len(report.Actions), len(report.Degraded), len(report.PublicOperations), report.Inconsistencies)
	}
}

// TestRun_GapsOnly_DropsTheContextAndKeepsTheWork verifies -gaps-only leaves
// out the per-action requirements, the served-empty positions and the public
// operations, and keeps the phase A set, the worklist and the cross-checks.
func TestRun_GapsOnly_DropsTheContextAndKeepsTheWork(t *testing.T) {
	useFixtures(t)
	content, _, err := Run("root", Options{GapsOnly: true, E2ECallsDir: "shards"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	report := decode(t, content)
	if report.Actions != nil || report.Degraded != nil || report.PublicOperations != nil {
		t.Errorf("-gaps-only kept context: %d actions, %d degraded, %d public", len(report.Actions), len(report.Degraded), len(report.PublicOperations))
	}
	if len(report.PhaseA.ByCause) == 0 || len(report.Worklist) == 0 || !report.E2E.Ran || report.InventoryCheck.Grain == "" {
		t.Errorf("-gaps-only dropped work: %+v", report)
	}
}

// TestRun_AnInconsistentTable_FailsTheGate verifies a table joined at another
// release reports the finding and fails the gate, the report still written.
func TestRun_AnInconsistentTable_FailsTheGate(t *testing.T) {
	useFixtures(t)
	grantTable = func() *finegrained.Table {
		table := fixtureTable()
		table.Version = "19.3.0-ee"
		return table
	}
	content, clean, err := Run("root", Options{GapsOnly: true})
	if err != nil || clean {
		t.Fatalf("Run = clean %t, error %v; want the gate to fail with a report", clean, err)
	}
	if report := decode(t, content); report.Summary.Inconsistencies != 1 || len(report.Inconsistencies) != 1 {
		t.Errorf("inconsistencies = %v, want the one release finding", report.Inconsistencies)
	}
}

// TestRun_AnInputThatCannotBeRead_StopsTheRunNamingIt verifies each input's
// failure surfaces with what failed, and that a report that cannot be encoded
// is an error rather than an empty file.
func TestRun_AnInputThatCannotBeRead_StopsTheRunNamingIt(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name    string
		arrange func(t *testing.T)
		wantErr string
	}{
		{name: "record", arrange: func(*testing.T) {
			readRecord = func(string) (actionrequests.Record, error) { return actionrequests.Record{}, boom }
		}, wantErr: "boom"},
		{name: "live", arrange: func(*testing.T) {
			readLive = func(string) (apilive.Document, error) { return apilive.Document{}, boom }
		}, wantErr: "read the live record: boom"},
		{name: "inventory", arrange: func(*testing.T) {
			readInventory = func(string) (requestinventory.Inventory, error) { return requestinventory.Inventory{}, boom }
		}, wantErr: "boom"},
		{name: "catalog", arrange: func(*testing.T) {
			catalog = func() ([]actionrequests.Action, error) { return nil, boom }
		}, wantErr: "build the action catalog: boom"},
		{name: "marshal", arrange: func(t *testing.T) {
			t.Helper()
			original := marshalIndent
			t.Cleanup(func() { marshalIndent = original })
			marshalIndent = func(any, string, string) ([]byte, error) { return nil, boom }
		}, wantErr: "marshal report: boom"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			useFixtures(t)
			testCase.arrange(t)
			_, clean, err := Run("root", Options{})
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) || clean {
				t.Errorf("Run = clean %t, error %v; want an error containing %q", clean, err, testCase.wantErr)
			}
		})
	}
}

// TestReadLive_ReadsTheRecordUnderTheRoot verifies the default reader looks
// for the record where the repository keeps it, so a root without one is an
// error naming the file.
func TestReadLive_ReadsTheRecordUnderTheRoot(t *testing.T) {
	_, err := readLive(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), apilive.FileName) {
		t.Errorf("readLive of an empty root = %v, want an error naming %s", err, apilive.FileName)
	}
}

// TestRun_TheCommittedArtifacts_AgreeWithTheLiveRecord runs the gate on the
// repository's own committed table, request record and live record, which is
// what make audit-1to1-grants does. It compares committed artifacts, so a
// stacked layer that defers their refresh skips it.
func TestRun_TheCommittedArtifacts_AgreeWithTheLiveRecord(t *testing.T) {
	freshness.SkipIfDeferred(t)
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("find the repository root: %v", err)
	}
	content, clean, err := Run(root, Options{GapsOnly: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	report := decode(t, content)
	if !clean {
		t.Fatalf("the committed artifacts are inconsistent: %v", report.Inconsistencies)
	}
	if report.Summary.Actions == 0 || report.Summary.Withheld == 0 || report.Summary.InventoryRESTRows == 0 {
		t.Errorf("summary = %+v, want the tree's actions, its withheld set and its inventory read", report.Summary)
	}
	if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(actionrequests.RecordPath))); statErr != nil {
		t.Errorf("the request record is not where the report says: %v", statErr)
	}
	if !slices.ContainsFunc(report.PhaseA.ByCause, func(group CauseGroup) bool { return group.GraphQL }) {
		t.Errorf("phase A = %+v, want the GraphQL causes the tree is withheld by", report.PhaseA)
	}
}
