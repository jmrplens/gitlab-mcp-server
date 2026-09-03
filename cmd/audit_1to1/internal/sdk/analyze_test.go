// Package sdk tests the two SDK-sourced audit rules: that every client-go
// service is either called or declared, and that every raw-GraphQL operation
// whose wrapper exists carries a decision. The end-to-end cases run against a
// throwaway module carrying a stand-in client-go, so a case can state the whole
// universe it asserts about instead of depending on the repository's shape.
package sdk

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// fixtureSDK is a stand-in for the client-go root package. Its import path
// carries the client-go path as an infix, which is what the resolver matches
// on, so the analyzers treat it exactly as they treat the real SDK.
const fixtureSDK = `package gitlab

// RequestOptionFunc is the variadic tail that marks a REST endpoint.
type RequestOptionFunc func()

type BranchesServiceInterface interface {
	ListBranches(options ...RequestOptionFunc) error
	CreateBranch(options ...RequestOptionFunc) error
	helper() error
}

type WidgetsServiceInterface interface {
	ListWidgets(options ...RequestOptionFunc) error
}

type GadgetsServiceInterface interface {
	ListGadgets(options ...RequestOptionFunc) error
}

type GraphQLQuery struct{ Query string }

type GraphQLInterface interface {
	Do(query GraphQLQuery, target any, options ...RequestOptionFunc) error
}

type Client struct {
	UserAgent string
	notAField int
	GraphQL   GraphQLInterface
	Branches  BranchesServiceInterface
	Widgets   WidgetsServiceInterface
	Gadgets   GadgetsServiceInterface
}
`

// fixtureModule writes a module whose internal/tools tree calls the stand-in
// SDK, and returns its root. Extra files are merged in, overriding the
// defaults, so a case can add or replace a tool package.
func fixtureModule(t *testing.T, extra map[string]string) string {
	t.Helper()
	const sdkDir = "gitlab.com/gitlab-org/api/client-go/v2/sdk.go"
	files := map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.27\n",
		sdkDir:   fixtureSDK,
		"internal/tools/branches/branches.go": `package branches

import gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v2"

// List calls the SDK wrapper, so Branches is covered.
func List(c *gl.Client) error { return c.Branches.ListBranches() }
`,
		"internal/tools/widgets/widgets.go": `package widgets

import gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v2"

// List reaches GitLab over raw GraphQL even though Widgets is a wrapper.
func List(c *gl.Client) error {
	return c.GraphQL.Do(gl.GraphQLQuery{Query: "{ widgets }"}, nil)
}

// Mutate hands the GraphQL interface to a helper rather than calling Do on it,
// the form a .Do-only scanner misses.
func Mutate(c *gl.Client) error { return exec(c.GraphQL) }

func exec(g gl.GraphQLInterface) error {
	return g.Do(gl.GraphQLQuery{Query: "mutation {}"}, nil)
}
`,
	}
	maps.Copy(files, extra)
	root := t.TempDir()
	writeFiles(t, root, files)
	return root
}

// writeFiles writes each slash-separated relative path under root, creating
// parents as needed.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

// fixtureReport builds the report for a fixture module with the given tables.
func fixtureReport(t *testing.T, root string, declaredBy map[string]declaration, aliases map[string]string, decisions map[string]decision) report {
	t.Helper()
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		t.Fatalf("load fixture packages: %v", err)
	}
	clientGo, err := shared.ClientGoTypes(pkgs)
	if err != nil {
		t.Fatalf("client-go types: %v", err)
	}
	declared, err := collectSDKServices(clientGo)
	if err != nil {
		t.Fatalf("collectSDKServices: %v", err)
	}
	services := adjudicateServices(declared, shared.CollectServiceUsage(pkgs), declaredBy)
	graphQL, err := shared.GraphQLInterface(clientGo)
	if err != nil {
		t.Fatalf("GraphQLInterface: %v", err)
	}
	operations := buildGraphQLOperations(collectGraphQLSites(pkgs, graphQL), serviceIndex(services), aliases, decisions)
	stale := mergeStale(staleServiceDeclarations(services, declaredBy), staleGraphQLDecisions(operations, decisions))
	return report{Summary: summarize(services, operations, stale), Services: services, GraphQLOperations: operations, StaleDeclarations: stale}
}

// statusOf indexes a report's services by name for assertions.
func statusOf(services []sdkService) map[string]string {
	out := map[string]string{}
	for _, service := range services {
		out[service.Service] = service.Status
	}
	return out
}

// TestFixture_EveryServiceGetsADecision verifies the whole rule on a module
// whose universe is known: a called service is covered, an uncalled one that
// the table names is declared, and an uncalled one nothing names is the
// finding this scope exists to raise. The GraphQL interface itself counts as a
// service and is covered by the handler that uses it.
func TestFixture_EveryServiceGetsADecision(t *testing.T) {
	root := fixtureModule(t, nil)
	rep := fixtureReport(t, root,
		map[string]declaration{"Widgets": {coveredGraphQL, "reached over GraphQL"}},
		nil, nil)

	want := map[string]string{
		"Branches":         statusCovered,
		"GraphQLInterface": statusCovered,
		"Widgets":          statusDeclared,
		"Gadgets":          statusUndeclared,
	}
	if got := statusOf(rep.Services); !reflect.DeepEqual(got, want) {
		t.Errorf("service statuses = %v, want %v", got, want)
	}
	if rep.Summary.SDKServices != 4 || rep.Summary.ServicesCovered != 2 ||
		rep.Summary.ServicesDeclared != 1 || rep.Summary.ServicesUndeclared != 1 {
		t.Errorf("summary = %+v, want 4 services split 2 covered / 1 declared / 1 undeclared", rep.Summary)
	}
	if rep.Summary.clean() {
		t.Error("a report with an undeclared service reported a clean gate")
	}
	for _, service := range rep.Services {
		if service.Service != "Branches" {
			continue
		}
		if service.Field != "Branches" || service.Interface != "BranchesServiceInterface" {
			t.Errorf("Branches field/interface = %q/%q", service.Field, service.Interface)
		}
		if service.APIMethods != 2 {
			t.Errorf("Branches api_methods = %d, want the 2 exported endpoint methods", service.APIMethods)
		}
		if !reflect.DeepEqual(service.Packages, []string{"branches"}) {
			t.Errorf("Branches packages = %v, want [branches]", service.Packages)
		}
	}
}

// TestFixture_GraphQLOperationsAreReportedPerFunction verifies the second rule:
// both raw-GraphQL forms are found (a direct Do and an interface handed to a
// helper), each is reported under its enclosing function rather than as one
// package verdict, and the table's decision rides along.
func TestFixture_GraphQLOperationsAreReportedPerFunction(t *testing.T) {
	root := fixtureModule(t, nil)
	rep := fixtureReport(t, root,
		map[string]declaration{"Widgets": {coveredGraphQL, "reached over GraphQL"}, "Gadgets": {unwrappedTracked, "not exposed"}},
		nil,
		map[string]decision{"widgets.List": {decisionKeep, "the wrapper returns a different shape"}})

	if len(rep.GraphQLOperations) != 3 {
		t.Fatalf("operations = %+v, want List, Mutate and the helper", rep.GraphQLOperations)
	}
	byName := map[string]graphqlOperation{}
	for _, op := range rep.GraphQLOperations {
		byName[op.operationKey()] = op
	}
	for _, key := range []string{"widgets.List", "widgets.Mutate", "widgets.exec"} {
		t.Run(key, func(t *testing.T) {
			op, ok := byName[key]
			if !ok {
				t.Fatalf("%s not reported (got %v)", key, rep.GraphQLOperations)
			}
			if op.Service != "Widgets" || op.ServiceMethods != 1 {
				t.Errorf("%s resolved %s/%d, want Widgets/1", key, op.Service, op.ServiceMethods)
			}
			if len(op.Sites) == 0 || !strings.HasPrefix(op.Sites[0], "internal/tools/widgets/widgets.go:") {
				t.Errorf("%s sites = %v, want a repository-relative file:line", key, op.Sites)
			}
		})
	}
	if op := byName["widgets.List"]; op.Status != statusAdjudicated || op.Decision != decisionKeep {
		t.Errorf("widgets.List = %s/%s, want adjudicated/KEEP", op.Status, op.Decision)
	}
	if op := byName["widgets.Mutate"]; op.Status != statusUnadjudicated || op.Decision != "" {
		t.Errorf("widgets.Mutate = %s/%s, want unadjudicated with no decision", op.Status, op.Decision)
	}
	if rep.Summary.GraphQLAdjudicated != 1 || rep.Summary.GraphQLUnadjudicated != 2 {
		t.Errorf("graphql summary = %+v, want 1 adjudicated / 2 unadjudicated", rep.Summary)
	}
}

// TestFixture_NewUpstreamServiceIsAFinding is the regression this scope was
// built for: adding a service to the Client struct that nothing calls turns the
// gate red, where the call-site-derived action scope would have gone on
// reporting zero gaps.
func TestFixture_NewUpstreamServiceIsAFinding(t *testing.T) {
	withNewService := strings.Replace(fixtureSDK,
		"	Gadgets   GadgetsServiceInterface\n",
		"	Gadgets   GadgetsServiceInterface\n	Sprockets SprocketsServiceInterface\n", 1)
	withNewService += `
type SprocketsServiceInterface interface {
	ListSprockets(options ...RequestOptionFunc) error
}
`
	root := fixtureModule(t, map[string]string{
		"gitlab.com/gitlab-org/api/client-go/v2/sdk.go": withNewService,
	})
	declaredBy := map[string]declaration{
		"Widgets": {coveredGraphQL, "reached over GraphQL"},
		"Gadgets": {unwrappedTracked, "not exposed"},
	}
	decisions := map[string]decision{
		"widgets.List":   {decisionKeep, "kept"},
		"widgets.Mutate": {decisionKeep, "kept"},
		"widgets.exec":   {decisionKeep, "kept"},
	}
	rep := fixtureReport(t, root, declaredBy, nil, decisions)
	if statusOf(rep.Services)["Sprockets"] != statusUndeclared {
		t.Fatalf("new service status = %q, want undeclared", statusOf(rep.Services)["Sprockets"])
	}
	if rep.Summary.clean() {
		t.Error("gate passed with an undeclared upstream service")
	}

	declaredBy["Sprockets"] = declaration{unwrappedTracked, "tracked elsewhere"}
	if !fixtureReport(t, root, declaredBy, nil, decisions).Summary.clean() {
		t.Error("gate still failed after the new service was declared")
	}
}

// TestFixture_StaleDeclarationsFailTheGate verifies both stale forms: a
// declaration for a service the tree now calls, and a decision for an operation
// that no longer exists. Either one is a claim about code that has moved on.
func TestFixture_StaleDeclarationsFailTheGate(t *testing.T) {
	root := fixtureModule(t, nil)
	rep := fixtureReport(t, root,
		map[string]declaration{
			"Branches": {coveredRaw, "stale: branches is called directly"},
			"Removed":  {coveredRaw, "stale: no such service upstream"},
			"Widgets":  {coveredGraphQL, "reached over GraphQL"},
			"Gadgets":  {unwrappedTracked, "not exposed"},
		},
		nil,
		map[string]decision{"widgets.Gone": {decisionKeep, "stale: no such operation"}})

	want := []string{
		"graphql widgets.Gone",
		"service Branches (now called directly)",
		"service Removed (no longer declared by client-go)",
	}
	if !reflect.DeepEqual(rep.StaleDeclarations, want) {
		t.Errorf("stale = %v, want %v", rep.StaleDeclarations, want)
	}
	if rep.Summary.StaleDeclarations != 3 || rep.Summary.clean() {
		t.Errorf("summary = %+v, want 3 stale declarations and a failing gate", rep.Summary)
	}
}

// TestFixture_MissingClientGoPieces_AbortTheRun verifies the two structural
// preconditions fail loudly rather than reporting an empty, passing audit: a
// module with no client-go in its import graph, and one whose SDK declares no
// GraphQL interface.
func TestFixture_MissingClientGoPieces_AbortTheRun(t *testing.T) {
	t.Run("no_client_go_in_the_import_graph", func(t *testing.T) {
		root := t.TempDir()
		writeFiles(t, root, map[string]string{
			"go.mod":                        "module example.com/plain\n\ngo 1.27\n",
			"internal/tools/alpha/alpha.go": "package alpha\n\nfunc A() int { return 1 }\n",
		})
		if _, _, err := Run(root, true); err == nil || !strings.Contains(err.Error(), "client-go root package not found") {
			t.Fatalf("Run = %v, want a missing client-go error", err)
		}
	})

	t.Run("sdk_without_a_graphql_interface", func(t *testing.T) {
		root := fixtureModule(t, map[string]string{
			"gitlab.com/gitlab-org/api/client-go/v2/sdk.go": strings.NewReplacer(
				"GraphQLInterface", "QueryRunner",
				"GraphQL   QueryRunner", "Runner    QueryRunner",
			).Replace(fixtureSDK),
			"internal/tools/widgets/widgets.go": "package widgets\n\nfunc Nothing() {}\n",
		})
		if _, _, err := Run(root, true); err == nil || !strings.Contains(err.Error(), "GraphQLInterface not found") {
			t.Fatalf("Run = %v, want a missing GraphQLInterface error", err)
		}
	})
}

// TestRun_Repository_GateIsGreenAndShapeIsStable runs the real repository
// through the command-facing entry point: the gate passes with the committed
// tables, the JSON carries the shared header and a trailing newline, and
// -gaps-only leaves the finding lists empty because there are none.
func TestRun_Repository_GateIsGreenAndShapeIsStable(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	content, clean, err := Run(root, true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !clean {
		t.Errorf("SDK parity gate is red; run `make audit-1to1-sdk`:\n%s", content)
	}
	if !strings.HasSuffix(string(content), "}\n") {
		t.Error("report lacks the trailing newline")
	}
	var rep report
	if unmarshalErr := json.Unmarshal(content, &rep); unmarshalErr != nil {
		t.Fatalf("report is not JSON: %v", unmarshalErr)
	}
	if rep.SchemaVersion != shared.SchemaVersion || rep.ClientGoPath != shared.ClientGoPkgPath {
		t.Errorf("report header = %d/%q, want %d/%q", rep.SchemaVersion, rep.ClientGoPath, shared.SchemaVersion, shared.ClientGoPkgPath)
	}
	if len(rep.Services) != 0 || len(rep.GraphQLOperations) != 0 {
		t.Errorf("gaps-only report kept %d services and %d operations, want none", len(rep.Services), len(rep.GraphQLOperations))
	}
	if rep.Summary.SDKServices != rep.Summary.ServicesCovered+rep.Summary.ServicesDeclared+rep.Summary.ServicesUndeclared {
		t.Errorf("service statuses do not add up: %+v", rep.Summary)
	}
	if rep.Summary.SDKServices < 100 {
		t.Errorf("sdk_services = %d, want the SDK's full service surface", rep.Summary.SDKServices)
	}
}

// TestBuildReport_Repository_ListsEveryServiceAndIsDeterministic verifies the
// full (non gaps-only) report against the repository: every service carries a
// status, a declared one carries its category and reason, and two runs agree.
func TestBuildReport_Repository_ListsEveryServiceAndIsDeterministic(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	first, err := buildReport(root, false)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	if len(first.Services) != first.Summary.SDKServices {
		t.Fatalf("listed %d services, summary says %d", len(first.Services), first.Summary.SDKServices)
	}
	var prev string
	for _, service := range first.Services {
		if service.Service < prev {
			t.Errorf("services not sorted: %q before %q", prev, service.Service)
		}
		prev = service.Service
		if service.Status == statusDeclared && (service.Category == "" || service.Reason == "") {
			t.Errorf("declared service %s has no category or reason", service.Service)
		}
		if service.Status == statusCovered && len(service.Packages) == 0 {
			t.Errorf("covered service %s names no referencing package", service.Service)
		}
	}
	for _, op := range first.GraphQLOperations {
		if op.Status != statusAdjudicated {
			t.Errorf("operation %s is not adjudicated", op.operationKey())
		}
		if op.Decision == "" || op.Reason == "" {
			t.Errorf("operation %s carries no decision or reason", op.operationKey())
		}
	}

	second, err := buildReport(root, false)
	if err != nil {
		t.Fatalf("second buildReport: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Error("buildReport is not deterministic across runs")
	}
}

// TestRun_MissingRoot_Fails verifies a root the loader cannot enter aborts the
// run and reports the gate as failed rather than clean.
func TestRun_MissingRoot_Fails(t *testing.T) {
	content, clean, err := Run(filepath.Join(t.TempDir(), "absent"), true)
	if err == nil || !strings.Contains(err.Error(), "load packages") {
		t.Fatalf("Run on a missing root = %v, want a load error", err)
	}
	if clean || content != nil {
		t.Errorf("failed run returned clean=%v content=%d bytes, want false and nil", clean, len(content))
	}
}
