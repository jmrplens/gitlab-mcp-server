// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package sdk tests the two SDK-sourced audit rules: that every client-go
// service is either called or declared, and that every raw-GraphQL operation
// whose wrapper exists carries a decision. The end-to-end cases run against a
// throwaway module carrying a stand-in client-go, so a case can state the whole
// universe it asserts about instead of depending on the repository's shape.
package sdk

import (
	"encoding/json"
	"errors"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/enums"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
)

// TestRun_SeamFailures_AreReported verifies the three failures the real tree
// and the fixture module cannot produce surface through Run as errors rather
// than as a clean report: a failing JSON encoder, a service enumeration that
// fails after the root package resolved, and an enum rule that fails after
// the same packages loaded.
//
// The root is the fixture module rather than the repository. What each case
// asserts is that the seam's error reaches Run's caller, so the tree the
// report was built from is scenery, and every other end-to-end case here
// states its universe with that fixture for the same reason. Against the
// repository each case first paid the typed load of ./internal/tools/... —
// about a second warm and several times that cold — to reach a
// json.MarshalIndent rigged to fail. Nothing about the real tree goes
// unasserted: TestRun_Repository_GateIsGreenAndShapeIsStable drives the same
// entry point over it.
func TestRun_SeamFailures_AreReported(t *testing.T) {
	root := fixtureModule(t, nil)
	boom := errors.New("boom")
	cases := []struct {
		name    string
		arrange func(t *testing.T)
		wantErr string
	}{
		{
			name: "marshal",
			arrange: func(t *testing.T) {
				t.Helper()
				original := marshalIndent
				t.Cleanup(func() { marshalIndent = original })
				marshalIndent = func(any, string, string) ([]byte, error) { return nil, boom }
			},
			wantErr: "marshal report: boom",
		},
		{
			name: "service_enumeration",
			arrange: func(t *testing.T) {
				t.Helper()
				original := sdkServices
				t.Cleanup(func() { sdkServices = original })
				sdkServices = func(*types.Package) ([]sdkService, error) { return nil, boom }
			},
			wantErr: "boom",
		},
		{
			name: "enum_rule",
			arrange: func(t *testing.T) {
				t.Helper()
				original := analyzeEnums
				t.Cleanup(func() { analyzeEnums = original })
				analyzeEnums = func(string, bool) (enums.Report, error) { return enums.Report{}, boom }
			},
			wantErr: "boom",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.arrange(t)
			content, clean, runErr := Run(root, true)
			if runErr == nil || !strings.Contains(runErr.Error(), tc.wantErr) {
				t.Fatalf("Run = %v, want %q", runErr, tc.wantErr)
			}
			if clean || content != nil {
				t.Errorf("failed run returned clean=%v content=%d bytes, want false and nil", clean, len(content))
			}
		})
	}
}

// TestRun_AnE2EFixtureThatDoesNotParse_FailsTheRun verifies the one failure
// the upstream waits section can raise reaches Run's caller as an error
// rather than as a report that read none of the fixture's raw requests.
func TestRun_AnE2EFixtureThatDoesNotParse_FailsTheRun(t *testing.T) {
	root := fixtureModule(t, map[string]string{fixtureDir + "/broken.go": "package fixture\n\nfunc {\n"})
	content, clean, err := Run(root, true)
	if err == nil || !strings.Contains(err.Error(), "parse the e2e fixture") {
		t.Fatalf("Run = %v, want the fixture's parse error", err)
	}
	if clean || content != nil {
		t.Errorf("failed run returned clean=%v content=%d bytes, want false and nil", clean, len(content))
	}
}

// TestReportSummaryClean_Counters_EachFindingFailsTheGateAlone verifies the
// verdict the command's exit status is taken from: every counter the summary
// carries a finding in fails the gate on its own, and the counters that merely
// describe the tree do not. The five are asserted one at a time because they
// are read as one conjunction, where a term dropped by an edit is invisible
// against a summary that has findings in several places at once.
func TestReportSummaryClean_Counters_EachFindingFailsTheGateAlone(t *testing.T) {
	cases := []struct {
		name    string
		summary reportSummary
		want    bool
	}{
		{
			name: "no finding",
			summary: reportSummary{
				SDKServices: 120, ServicesCovered: 118, ServicesDeclared: 2, GraphQLOperations: 9, GraphQLAdjudicated: 9, EnumFields: 40,
				OptionStructs: 9, OptionPaired: 5, OptionConverted: 1, OptionCopied: 1, OptionDeclared: 2,
				UpstreamWaits: 47, WaitsHolding: 41, Workarounds: 55,
				CapturedTypes: 86, CapturedKeysShared: 77, CapturesKept: 77, CapturesAnswerNoStruct: 2,
				IdentifierArguments: 10, IdentifiersIDOnly: 6, IdentifiersDeclared: 3, IdentifiersNotFromInput: 1,
				SDKNarrowed: 18, SDKNarrowedResolved: 15, SDKNarrowedNotFromInput: 1, SDKNarrowedUncalled: 2,
			},
			want: true,
		},
		{name: "a client-go parameter that narrows a name, called, and no declaration", summary: reportSummary{SDKNarrowedUndeclared: 1}},
		{name: "a declared client-go parameter handed an integer from an input that offers no name", summary: reportSummary{SDKNarrowedUnresolved: 1}},
		{name: "an integer handed where the record takes a name or a path", summary: reportSummary{IdentifiersNarrowed: 1}},
		{name: "an integer handed where the record describes nothing", summary: reportSummary{IdentifiersUndescribed: 1}},
		{name: "an integer handed where the record holds no route", summary: reportSummary{IdentifiersUnplaced: 1}},
		{name: "an integer whose trace stopped where the record takes a name", summary: reportSummary{IdentifiersUntraced: 1}},
		{name: "a client-go method value the walk does not follow", summary: reportSummary{IdentifiersUnfollowed: 1}},
		{name: "a captured key the client-go struct beside it models", summary: reportSummary{CapturesRedundant: 1}},
		{name: "a captured struct the walk pairs with nothing", summary: reportSummary{CapturesUnpaired: 1}},
		{name: "a capture declaration with a predicate to edit", summary: reportSummary{CapturesStalePredicate: 1}},
		{name: "a capture the pinned SDK made obsolete", summary: reportSummary{CapturesObsolete: 1}},
		{name: "an undeclared service", summary: reportSummary{ServicesUndeclared: 1}},
		{name: "an unadjudicated GraphQL operation", summary: reportSummary{GraphQLUnadjudicated: 1}},
		{name: "an enum value the SDK declares and we do not", summary: reportSummary{EnumMissingValues: 1}},
		{name: "an enum value we publish and the SDK does not", summary: reportSummary{EnumExtraValues: 1}},
		{name: "an option struct nothing holds", summary: reportSummary{OptionUnpaired: 1}},
		{name: "a field a copy leaves out", summary: reportSummary{OptionCopyMissing: 1}},
		{name: "a field of a nested struct its input lacks", summary: reportSummary{OptionNestedMissing: 1}},
		{name: "a workaround with a predicate to edit", summary: reportSummary{WaitsStalePredicate: 1}},
		{name: "a workaround the pinned SDK made obsolete", summary: reportSummary{WaitsObsolete: 1}},
		{name: "a workaround no entry answers", summary: reportSummary{WorkaroundsUndeclared: 1}},
		{name: "a body key no predicate explains", summary: reportSummary{BodyEditKeysUnanswered: 1}},
		{name: "a body edit whose keys the walk cannot read", summary: reportSummary{BodyEditsUnread: 1}},
		{name: "a key a superset carries that no predicate explains", summary: reportSummary{SupersetNeedsUnanswered: 1}},
		{name: "a method a request option is handed to that no predicate names", summary: reportSummary{OptionMethodsUnanswered: 1}},
		{name: "a declaration that no longer describes the code", summary: reportSummary{StaleDeclarations: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.summary.clean(); got != tc.want {
				t.Errorf("clean() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestGapsOnlyFilters_MixedStatuses_KeepOnlyTheFindings verifies the two
// gaps-only filters keep undeclared services and unadjudicated operations
// and drop everything else, and return an empty (never nil) slice.
func TestGapsOnlyFilters_MixedStatuses_KeepOnlyTheFindings(t *testing.T) {
	services := keepUndeclaredServices([]sdkService{
		{Service: "A", Status: statusCovered},
		{Service: "B", Status: statusUndeclared},
		{Service: "C", Status: statusDeclared},
	})
	if len(services) != 1 || services[0].Service != "B" {
		t.Errorf("keepUndeclaredServices = %+v, want only B", services)
	}
	operations := keepUnadjudicatedOperations([]graphqlOperation{
		{Operation: "x", Status: statusAdjudicated},
		{Operation: "y", Status: statusUnadjudicated},
	})
	if len(operations) != 1 || operations[0].Operation != "y" {
		t.Errorf("keepUnadjudicatedOperations = %+v, want only y", operations)
	}
	if got := keepUndeclaredServices(nil); got == nil || len(got) != 0 {
		t.Errorf("keepUndeclaredServices(nil) = %#v, want an empty slice", got)
	}
}

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
	const sdkDir = "gitlab.com/gitlab-org/api/client-go/v3/sdk.go"
	files := map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.27\n",
		sdkDir:   fixtureSDK,
		"internal/tools/branches/branches.go": `package branches

import gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"

// List calls the SDK wrapper, so Branches is covered.
func List(c *gl.Client) error { return c.Branches.ListBranches() }
`,
		"internal/tools/widgets/widgets.go": `package widgets

import gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"

// List reaches GitLab over raw GraphQL even though Widgets is a wrapper.
func List(c *gl.Client) error {
	return c.GraphQL.Do(gl.GraphQLQuery{Query: "{ widgets }"}, nil)
}

// Mutate hands the GraphQL interface to a helper rather than calling Do on it,
// the form a .Do-only scanner misses.
func Mutate(c *gl.Client) error { return exec(c.GraphQL) }

// exec touches the interface twice, so one function carries two sites.
func exec(g gl.GraphQLInterface) error {
	if g == nil {
		return nil
	}
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
	byName := serviceIndex(services)
	operations := buildGraphQLOperations(collectGraphQLSites(pkgs, graphQL), byName, aliases, decisions)
	stale := mergeStale(staleServiceDeclarations(services, declaredBy), staleGraphQLDecisions(operations, decisions),
		staleGraphQLReplaces(decisions, byName, aliases, shared.CollectServiceUsage(pkgs)))
	return report{Summary: summarize(services, operations, enums.Summary{}, stale), Services: services, GraphQLOperations: operations, StaleDeclarations: stale}
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
		map[string]decision{"widgets.List": {Decision: decisionKeep, Reason: "the wrapper returns a different shape"}})

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
		"gitlab.com/gitlab-org/api/client-go/v3/sdk.go": withNewService,
	})
	declaredBy := map[string]declaration{
		"Widgets": {coveredGraphQL, "reached over GraphQL"},
		"Gadgets": {unwrappedTracked, "not exposed"},
	}
	decisions := map[string]decision{
		"widgets.List":   {Decision: decisionKeep, Reason: "kept"},
		"widgets.Mutate": {Decision: decisionKeep, Reason: "kept"},
		"widgets.exec":   {Decision: decisionKeep, Reason: "kept"},
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
		map[string]decision{"widgets.Gone": {Decision: decisionKeep, Reason: "stale: no such operation"}})

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

// TestFixture_Replaces_AnswerAMethodOfTheirServiceNoHandlerCalls holds the
// Replaces of a decision on a module whose universe is known. A KEEP
// operation of the branches package naming Branches.CreateBranch, a method of
// the service its package resolves to that a handler calls and no handler
// calls the method of, answers it: the gate stays green and R-ACTION is
// handed the method with the operation and its reason. The same method named
// by a second KEEP decision of the package is answered once, by the first in
// key order, and the second is stale; one an operation of the widgets
// package names is stale, since that operation is adjudicated against
// Widgets and cannot read in place of a method of Branches; and one a
// MIGRATE decision names is stale. Neither of the last two is handed to
// R-ACTION.
func TestFixture_Replaces_AnswerAMethodOfTheirServiceNoHandlerCalls(t *testing.T) {
	root := fixtureModule(t, map[string]string{
		"internal/tools/branches/graphql.go": `package branches

import gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"

// Create writes a branch over raw GraphQL, where Branches has CreateBranch.
func Create(c *gl.Client) error {
	return c.GraphQL.Do(gl.GraphQLQuery{Query: "mutation { createBranch }"}, nil)
}

// Rename is a second operation of the package.
func Rename(c *gl.Client) error {
	return c.GraphQL.Do(gl.GraphQLQuery{Query: "mutation { renameBranch }"}, nil)
}
`,
	})
	declaredBy := map[string]declaration{
		"Widgets": {coveredGraphQL, "reached over GraphQL"},
		"Gadgets": {unwrappedTracked, "not exposed"},
	}
	answered := map[string]decision{
		"branches.Create": {Decision: decisionKeep, Reason: "pages", Replaces: []string{"Branches.CreateBranch"}},
		"branches.Rename": {Decision: decisionKeep, Reason: "kept"},
		"widgets.List":    {Decision: decisionKeep, Reason: "kept"},
		"widgets.Mutate":  {Decision: decisionKeep, Reason: "kept"},
		"widgets.exec":    {Decision: decisionKeep, Reason: "kept"},
	}
	if rep := fixtureReport(t, root, declaredBy, nil, answered); !rep.Summary.clean() {
		t.Errorf("a Replaces that answers a method failed the gate: %v %v", rep.StaleDeclarations, rep.Summary)
	}
	if got, want := keptReplacements(answered, nil), map[string]string{"Branches.CreateBranch": "branches.Create: pages"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keptReplacements = %v, want %v", got, want)
	}

	answered["branches.Rename"] = decision{Decision: decisionKeep, Reason: "kept", Replaces: []string{"Branches.CreateBranch"}}
	answered["widgets.List"] = decision{Decision: decisionKeep, Reason: "kept", Replaces: []string{"Branches.CreateBranch"}}
	answered["widgets.exec"] = decision{Decision: decisionMigrate, Reason: "moves", Replaces: []string{"Gadgets.ListGadgets"}}
	rep := fixtureReport(t, root, declaredBy, nil, answered)
	want := []string{
		"graphql branches.Rename (replaces Branches.CreateBranch: graphql branches.Create replaces it too)",
		"graphql widgets.List (replaces Branches.CreateBranch: the operation's package, widgets, does not resolve to this service)",
		"graphql widgets.exec (replaces Gadgets.ListGadgets: only a KEEP decision stands in for a method)",
	}
	if !reflect.DeepEqual(rep.StaleDeclarations, want) {
		t.Errorf("stale = %v, want %v", rep.StaleDeclarations, want)
	}
	if got, want := keptReplacements(answered, nil), map[string]string{"Branches.CreateBranch": "branches.Create: pages"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keptReplacements = %v, want %v, the first KEEP decision of the service answering and the others handing nothing", got, want)
	}
}

// TestKeptReplacements_AMethodOfAnotherService_IsHandedToNobody verifies that
// R-ACTION is handed a method only by an operation of the method's service,
// which the operation's package resolves to by its name in any case or, when
// it has one, by its alias alone: a decision earlier in key order whose
// operation is of another service leaves the method to the next decision of
// its own, and an aliased package does not resolve to the service its own
// name would.
func TestKeptReplacements_AMethodOfAnotherService_IsHandedToNobody(t *testing.T) {
	aliases := map[string]string{"vulns": "ProjectVulnerabilities"}
	decisions := map[string]decision{
		"alpha.List":    {Decision: decisionKeep, Reason: "another service", Replaces: []string{"Branches.CreateBranch", "ProjectVulnerabilities.ListProjectVulnerabilities"}},
		"branches.List": {Decision: decisionKeep, Reason: "its own", Replaces: []string{"Branches.CreateBranch"}},
		"vulns.List":    {Decision: decisionKeep, Reason: "aliased", Replaces: []string{"ProjectVulnerabilities.ListProjectVulnerabilities", "Vulns.ListVulns"}},
	}
	want := map[string]string{
		"Branches.CreateBranch":                             "branches.List: its own",
		"ProjectVulnerabilities.ListProjectVulnerabilities": "vulns.List: aliased",
	}
	if got := keptReplacements(decisions, aliases); !reflect.DeepEqual(got, want) {
		t.Errorf("keptReplacements = %v, want %v", got, want)
	}
}

// TestCoveredGraphQLMethods_TheCommittedDecisions_HandRActionWhatTheyReplace
// verifies the accessor R-ACTION derives COVERED_GRAPHQL from reads the
// committed table: every method a KEEP decision names, with that decision's
// key and reason.
func TestCoveredGraphQLMethods_TheCommittedDecisions_HandRActionWhatTheyReplace(t *testing.T) {
	got := CoveredGraphQLMethods()
	want := map[string]string{}
	for key, adjudged := range graphqlDecisions {
		for _, method := range adjudged.Replaces {
			want[method] = key + ": " + adjudged.Reason
		}
	}
	if len(want) == 0 || !reflect.DeepEqual(got, want) {
		t.Errorf("CoveredGraphQLMethods = %v, want %v", got, want)
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
			"gitlab.com/gitlab-org/api/client-go/v3/sdk.go": strings.NewReplacer(
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
	if options := rep.OptionStructs; len(options.Structs) != 0 || len(options.Copies) != 0 || len(options.Conversions) != 0 {
		t.Errorf("gaps-only report kept option structs %+v, want none", options)
	}
	checkGapsOnlyCaptures(t, rep)
	checkGapsOnlyIdentifiers(t, rep)
	if s := rep.Summary; s.OptionStructs != s.OptionPaired+s.OptionConverted+s.OptionCopied+s.OptionDeclared+s.OptionUnpaired || s.OptionStructs < 400 {
		t.Errorf("option struct statuses do not add up to the SDK's request surface: %+v", s)
	}
	if rep.Summary.SDKServices != rep.Summary.ServicesCovered+rep.Summary.ServicesDeclared+rep.Summary.ServicesUndeclared {
		t.Errorf("service statuses do not add up: %+v", rep.Summary)
	}
	if rep.Summary.SDKServices < 100 {
		t.Errorf("sdk_services = %d, want the SDK's full service surface", rep.Summary.SDKServices)
	}
}

// TestBuildReport_Repository_ReplacesThatAnswerNothing_FailTheGate plants, in
// a copy of the committed decisions, one Replaces of each kind that answers
// no method R-ACTION would list, and holds the gate to naming each: a method
// the service does not declare, a service client-go does not declare, a
// service no handler calls (whose own declaration answers it), a method a
// handler calls, a method a MIGRATE decision names (which moves to the
// wrapper rather than standing in for it), a method a second decision names,
// the first in key order answering it, and a method of a service other than
// the one the operation's package resolves to, by its own name (orbit) and
// through graphqlServiceAliases (vulnerabilities). It runs on the tree, so
// the services, the calls and the operations are the real ones, and the one
// Replaces the tree carries, TerraformStates.List, stays answered by
// terraformstates.listPage although orbit.Status, earlier in key order, names
// it too. R-ACTION is handed no method of another service, so the
// Groups.ListSubGroups the vulnerabilities plant names stays on its list;
// the plants that name a method R-ACTION does not list are handed on, and
// hide nothing, since the gate fails on each.
func TestBuildReport_Repository_ReplacesThatAnswerNothing_FailTheGate(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	original := graphqlDecisions
	t.Cleanup(func() { graphqlDecisions = original })
	planted := maps.Clone(original)
	const reason = "planted to answer nothing"
	planted["terraformstates.listPage"] = decision{Decision: decisionKeep, Reason: reason, Replaces: []string{"TerraformStates.List", "TerraformStates.Lst", "TerraformStates.Get"}}
	planted["epicissues.List"] = decision{Decision: decisionKeep, Reason: reason, Replaces: []string{"EpicIssues.ListEpicIssues"}}
	planted["vulnerabilities.Get"] = decision{Decision: decisionKeep, Reason: reason, Replaces: []string{"Sprockets.ListSprockets", "Groups.ListSubGroups"}}
	planted["vulnerabilities.List"] = decision{Decision: decisionMigrate, Reason: reason, Replaces: []string{"ProjectVulnerabilities.ListProjectVulnerabilities"}}
	planted["orbit.Status"] = decision{Decision: decisionKeep, Reason: reason, Replaces: []string{"Orbit.GetStatus", "TerraformStates.List"}}
	planted["orbit.schemaPage"] = decision{Decision: decisionKeep, Reason: reason, Replaces: []string{"Orbit.GetStatus"}}
	graphqlDecisions = planted

	rep, err := buildReport(root, true)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	want := []string{
		"graphql epicissues.List (replaces EpicIssues.ListEpicIssues: no handler calls the service, which its declaration answers)",
		"graphql orbit.Status (replaces TerraformStates.List: the operation's package, orbit, does not resolve to this service)",
		"graphql orbit.schemaPage (replaces Orbit.GetStatus: graphql orbit.Status replaces it too)",
		"graphql terraformstates.listPage (replaces TerraformStates.Get: a handler calls it)",
		"graphql terraformstates.listPage (replaces TerraformStates.Lst: the service declares no such method)",
		"graphql vulnerabilities.Get (replaces Groups.ListSubGroups: the operation's package, vulnerabilities, does not resolve to this service)",
		"graphql vulnerabilities.Get (replaces Sprockets.ListSprockets: client-go declares no such service)",
		"graphql vulnerabilities.List (replaces ProjectVulnerabilities.ListProjectVulnerabilities: only a KEEP decision stands in for a method)",
	}
	if !reflect.DeepEqual(rep.StaleDeclarations, want) {
		t.Errorf("stale declarations =\n%s\nwant\n%s", strings.Join(rep.StaleDeclarations, "\n"), strings.Join(want, "\n"))
	}
	if rep.Summary.clean() {
		t.Error("the gate passed with eight Replaces that answer nothing")
	}
	handed := map[string]string{
		"EpicIssues.ListEpicIssues": "epicissues.List: " + reason,
		"Orbit.GetStatus":           "orbit.Status: " + reason,
		"TerraformStates.Get":       "terraformstates.listPage: " + reason,
		"TerraformStates.List":      "terraformstates.listPage: " + reason,
		"TerraformStates.Lst":       "terraformstates.listPage: " + reason,
	}
	if got := CoveredGraphQLMethods(); !reflect.DeepEqual(got, handed) {
		t.Errorf("CoveredGraphQLMethods =\n%v\nwant\n%v, no method of a service other than its operation's", got, handed)
	}
}

// checkGapsOnlyCaptures holds the captures section of a gaps-only report on
// the real tree to keeping nothing, since there is no finding, and its
// counters to adding up over the captures the tree decodes.
func checkGapsOnlyCaptures(t *testing.T, rep report) {
	t.Helper()
	if captures := rep.Captures; len(captures.Types) != 0 || len(captures.Keys) != 0 || len(captures.Unpaired) != 0 {
		t.Errorf("gaps-only report kept captures %+v, want none", captures)
	}
	if s := rep.Summary; s.CapturedKeysShared != s.CapturesKept+s.CapturesRedundant+s.CapturesStalePredicate+s.CapturesObsolete || s.CapturedTypes < 50 {
		t.Errorf("capture statuses do not add up to the captures the tree decodes: %+v", s)
	}
}

// checkGapsOnlyIdentifiers holds the identifiers section of a gaps-only
// report on the real tree to keeping nothing, since there is no finding, no
// method value the walk does not follow, and the client-go narrowings are
// no finding of this gate, and its counters to adding up over the integers
// the handlers hand a parameter of type any.
func checkGapsOnlyIdentifiers(t *testing.T, rep report) {
	t.Helper()
	if identifiers := rep.Identifiers; len(identifiers.Arguments) != 0 || len(identifiers.Unfollowed) != 0 || len(identifiers.SDKNarrowed) != 0 {
		t.Errorf("gaps-only report kept identifiers %+v, want none", identifiers)
	}
	s := rep.Summary
	if s.IdentifierArguments != s.IdentifiersIDOnly+s.IdentifiersDeclared+s.IdentifiersNotFromInput+s.IdentifiersNarrowed+s.IdentifiersUndescribed+s.IdentifiersUnplaced+s.IdentifiersUntraced || s.IdentifierArguments == 0 {
		t.Errorf("identifier statuses do not add up to the integers the tree hands client-go: %+v", s)
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
