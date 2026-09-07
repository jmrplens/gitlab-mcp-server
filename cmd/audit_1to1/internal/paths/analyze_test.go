package paths

import (
	"context"
	"encoding/json"
	"errors"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/requestinventory"
)

// stubInputs replaces the four inputs with fixtures, so a case can describe a
// tree rather than wait for one: the committed inventory, the catalog compiled
// into the binary, the type-checked read of every GraphQL document, and the
// documentation comparison, which would otherwise download 250 pages from
// gitlab.com the moment a case handed Run a real fetcher.
func stubInputs(t *testing.T, rows []requestinventory.Row, actions []requestinventory.Action, documents graphqldocs.Result) {
	t.Helper()
	inventory, catalog, graphQL, endpoints := readInventory, catalogActions, auditGraphQL, auditEndpoints
	t.Cleanup(func() {
		readInventory, catalogActions, auditGraphQL, auditEndpoints = inventory, catalog, graphQL, endpoints
	})
	readInventory = func(string) (requestinventory.Inventory, error) {
		return requestinventory.Inventory{Note: "fixture", Requests: rows}, nil
	}
	catalogActions = func() ([]requestinventory.Action, error) { return actions, nil }
	auditGraphQL = func(graphqldocs.Options) (graphqldocs.Result, error) { return documents, nil }
	auditEndpoints = func(context.Context, *apidocs.Fetcher, []requestinventory.Row) (EndpointCheck, error) {
		return EndpointCheck{Ran: true}, nil
	}
}

// oneRow is an inventory with one recorded request in it, which is enough for
// every case that is not about the inventory.
var oneRow = []requestinventory.Row{
	{Package: "internal/tools/issues", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects/:id/issues"},
}

// repoRoot walks up from the test's working directory to the module root, so
// the one case that audits this repository can name it.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// decode reads a report back out of the JSON Run produced.
func decode(t *testing.T, content []byte) Report {
	t.Helper()
	var report Report
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatalf("the report is not JSON: %v\n%s", err, content)
	}
	return report
}

// TestRun_ACleanTree_PassesTheGateAndCountsWhatItSaw verifies the passing
// shape: no finding, and a summary that says how much of the catalog the
// recording could see rather than only that nothing failed.
func TestRun_ACleanTree_PassesTheGateAndCountsWhatItSaw(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "issues")
	withDeclarations(t, map[string]silentOwnerDeclaration{})
	stubInputs(t, oneRow,
		[]requestinventory.Action{{ID: "issue.list", Owner: "issues"}},
		graphqldocs.Result{Documents: []graphqldocs.Document{{Name: "query"}}})

	content, clean, err := Run(t.Context(), root, false, nil)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if !clean {
		t.Errorf("Run() reported findings on a clean tree:\n%s", content)
	}
	if !strings.HasSuffix(string(content), "\n") {
		t.Error("the report does not end in a newline")
	}
	report := decode(t, content)
	if report.Summary.CatalogActions != 1 || report.Summary.ActionsObserved != 1 || report.Summary.GraphQLDocuments != 1 {
		t.Errorf("summary = %+v, want the one action observed and the one document read", report.Summary)
	}
	if report.Inventory != requestinventory.Path {
		t.Errorf("report names the inventory %q, want %q", report.Inventory, requestinventory.Path)
	}
	if report.Endpoints.Ran {
		t.Error("the documentation comparison ran without a fetcher")
	}
}

// TestRun_TheThreeFindings_FailTheGate verifies what R-PATH gates on, one
// finding at a time, since a gate that fails for the wrong reason is as bad as
// one that does not fail.
func TestRun_TheThreeFindings_FailTheGate(t *testing.T) {
	cases := []struct {
		name     string
		arrange  func(t *testing.T, root string)
		wantJSON string
	}{
		{
			name: "a package that recorded nothing and declared nothing",
			arrange: func(t *testing.T, root string) {
				t.Helper()
				makeToolsPackage(t, root, "forgotten")
				withDeclarations(t, map[string]silentOwnerDeclaration{})
				stubInputs(t, oneRow, []requestinventory.Action{{ID: "widget.list", Owner: "forgotten"}}, graphqldocs.Result{})
			},
			wantJSON: `"undeclared_silent_packages": 1`,
		},
		{
			name: "a document the schema refuses",
			arrange: func(t *testing.T, root string) {
				t.Helper()
				makeToolsPackage(t, root, "issues")
				withDeclarations(t, map[string]silentOwnerDeclaration{})
				stubInputs(t, oneRow, []requestinventory.Action{{ID: "issue.list", Owner: "issues"}}, graphqldocs.Result{
					Documents: []graphqldocs.Document{{Name: "listVulnerabilities"}},
					Refusals: []graphqldocs.Refusal{{
						Document: graphqldocs.Document{Package: "internal/tools/vulnerabilities", Name: "listVulnerabilities", Position: token.Position{Filename: "v.go", Line: 12}},
						Reasons:  []string{`Cannot query field "hasSolutions"`},
					}},
				})
			},
			wantJSON: `"graphql_refused": 1`,
		},
		{
			name: "a declaration that no longer holds",
			arrange: func(t *testing.T, root string) {
				t.Helper()
				makeToolsPackage(t, root, "issues")
				withDeclarations(t, map[string]silentOwnerDeclaration{
					"retired": {Category: categoryRecordedElsewhere, Reason: "recorded a request since"},
				})
				stubInputs(t, oneRow, []requestinventory.Action{{ID: "issue.list", Owner: "issues"}}, graphqldocs.Result{})
			},
			wantJSON: `"stale_declarations": 1`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			testCase.arrange(t, root)

			content, clean, err := Run(t.Context(), root, false, nil)
			if err != nil {
				t.Fatalf("Run() error = %v, want the finding in the report", err)
			}
			if clean {
				t.Errorf("Run() reported clean:\n%s", content)
			}
			if !strings.Contains(string(content), testCase.wantJSON) {
				t.Errorf("the report does not carry %s:\n%s", testCase.wantJSON, content)
			}
		})
	}
}

// TestRun_TheRefusedDocument_IsNamedWhereAReaderCanOpenIt verifies that a
// refusal carries what a person needs to act on it, since a count of refused
// documents is not something anybody can fix.
func TestRun_TheRefusedDocument_IsNamedWhereAReaderCanOpenIt(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "issues")
	withDeclarations(t, map[string]silentOwnerDeclaration{})
	stubInputs(t, oneRow, []requestinventory.Action{{ID: "issue.list", Owner: "issues"}}, graphqldocs.Result{
		Documents: []graphqldocs.Document{{Name: "listVulnerabilities"}},
		Refusals: []graphqldocs.Refusal{{
			Document: graphqldocs.Document{Package: "internal/tools/vulnerabilities", Position: token.Position{Filename: "v.go", Line: 12}},
			Reasons:  []string{"first", "second"},
		}},
	})

	content, _, err := Run(t.Context(), root, false, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	report := decode(t, content)
	if len(report.GraphQLRefusals) != 1 {
		t.Fatalf("refusals = %+v, want one", report.GraphQLRefusals)
	}
	refusal := report.GraphQLRefusals[0]
	if refusal.Package != "internal/tools/vulnerabilities" || refusal.Document != "an inline document" || !strings.Contains(refusal.Position, "v.go:12") {
		t.Errorf("refusal = %+v, want the package, the label and the position", refusal)
	}
	if len(refusal.Reasons) != 2 {
		t.Errorf("reasons = %v, want every objection under the document", refusal.Reasons)
	}
}

// TestRun_GapsOnly_KeepsTheWorkAndDropsTheContext verifies the flag every scope
// carries: a declared silence is context, and a work list that carries it is a
// work list nobody reads.
//
// An unmapped owner stays, because it is work: the catalog names a package
// that is not one, so the recording could never have covered those actions.
// It used to be filtered out here while the gate ignored it too, which made
// forgetting to name an owner the one way to leave the gate with nothing to
// say.
func TestRun_GapsOnly_KeepsTheWorkAndDropsTheContext(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "declared")
	makeToolsPackage(t, root, "forgotten")
	withDeclarations(t, map[string]silentOwnerDeclaration{
		"declared": {Category: categoryRecordedElsewhere, Reason: "these handlers live in another package altogether"},
	})
	stubInputs(t, oneRow, []requestinventory.Action{
		{ID: "a.list", Owner: "declared"},
		{ID: "b.list", Owner: "forgotten"},
		{ID: "c.list", Owner: "nowhere"},
	}, graphqldocs.Result{})

	full, _, err := Run(t.Context(), root, false, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	gaps, _, err := Run(t.Context(), root, true, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(decode(t, full).SilentOwners) != 3 {
		t.Errorf("the full report lists %+v, want all three owners", decode(t, full).SilentOwners)
	}
	owners := decode(t, gaps).SilentOwners
	if len(owners) != 2 || owners[0].Package != "forgotten" || owners[1].Package != "nowhere" {
		t.Errorf("the gaps-only report lists %+v, want the undeclared and the unmapped owner", owners)
	}
	t.Run("the summary still counts everything", func(t *testing.T) {
		if decode(t, gaps).Summary.ActionsSilent != 2 {
			t.Errorf("summary = %+v, want the counts unchanged by the filter", decode(t, gaps).Summary)
		}
	})
}

// TestRun_TheDocumentationComparison_RunsOnlyWithAFetcher verifies the switch,
// since the comparison needs the network and two minutes of it: a run that
// only wants the other two checks must not pay for it.
//
// When it does run it gates, on the endpoints no declaration accounts for.
// The undocumented endpoint stubbed here is declared by nothing, so the gate
// fails and the run that never compared anything passes.
func TestRun_TheDocumentationComparison_RunsOnlyWithAFetcher(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "issues")
	withDeclarations(t, map[string]silentOwnerDeclaration{})
	stubInputs(t, oneRow, []requestinventory.Action{{ID: "issue.list", Owner: "issues"}}, graphqldocs.Result{})
	called := 0
	original := auditEndpoints
	t.Cleanup(func() { auditEndpoints = original })
	auditEndpoints = func(context.Context, *apidocs.Fetcher, []requestinventory.Row) (EndpointCheck, error) {
		called++
		return EndpointCheck{Ran: true, Undocumented: []Endpoint{{Method: "GET", Path: "/nowhere"}}}, nil
	}

	_, cleanWithout, err := Run(t.Context(), root, false, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if called != 0 {
		t.Errorf("the comparison ran %d time(s) without a fetcher, want none", called)
	}
	if !cleanWithout {
		t.Error("the gate failed on a comparison that never ran")
	}

	content, clean, err := Run(t.Context(), root, false, apidocs.New(root, apidocs.Options{}))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if called != 1 {
		t.Errorf("the comparison ran %d time(s) with a fetcher, want one", called)
	}
	if clean {
		t.Error("an endpoint no declaration accounts for passed the gate")
	}
	summary := decode(t, content).Summary
	if summary.UndocumentedEndpoints != 1 || summary.UndeclaredEndpoints != 1 {
		t.Errorf("summary = %+v, want the endpoint counted as undocumented and undeclared", summary)
	}
	t.Run("the work list keeps it", func(t *testing.T) {
		gaps, _, gapsErr := Run(t.Context(), root, true, apidocs.New(root, apidocs.Options{}))
		if gapsErr != nil {
			t.Fatalf("Run() error = %v", gapsErr)
		}
		if found := decode(t, gaps).Endpoints.Undocumented; len(found) != 1 || found[0].Path != "/nowhere" {
			t.Errorf("the gaps-only report lists %+v, want the undeclared endpoint", found)
		}
	})
}

// TestRun_ADeclaredEndpoint_PassesAndIsNotWork verifies the other half of the
// endpoint gate: a shape the declaration table accounts for is reported with
// its reason, kept out of the work list, and does not fail the run.
func TestRun_ADeclaredEndpoint_PassesAndIsNotWork(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "issues")
	withDeclarations(t, map[string]silentOwnerDeclaration{})
	stubInputs(t, oneRow, []requestinventory.Action{{ID: "issue.list", Owner: "issues"}}, graphqldocs.Result{})
	original := auditEndpoints
	t.Cleanup(func() { auditEndpoints = original })
	withEndpointDeclarations(t, []endpointDeclaration{
		{Shape: "/orbit/...", Category: categoryUndocumentedAPI, Reason: "experimental and documented nowhere"},
	})
	auditEndpoints = func(context.Context, *apidocs.Fetcher, []requestinventory.Row) (EndpointCheck, error) {
		found, unused := classifyUndocumented([]Endpoint{{Method: "GET", Path: "/orbit/status"}})
		return EndpointCheck{Ran: true, Undocumented: found, UnusedDeclarations: unused}, nil
	}

	content, clean, err := Run(t.Context(), root, false, apidocs.New(root, apidocs.Options{}))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	report := decode(t, content)

	if !clean {
		t.Errorf("a declared endpoint failed the gate: %+v", report.Summary)
	}
	if len(report.Endpoints.Undocumented) != 1 || report.Endpoints.Undocumented[0].Category != categoryUndocumentedAPI {
		t.Errorf("undocumented = %+v, want the declaration's category on it", report.Endpoints.Undocumented)
	}
	gaps, _, err := Run(t.Context(), root, true, apidocs.New(root, apidocs.Options{}))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if found := decode(t, gaps).Endpoints.Undocumented; len(found) != 0 {
		t.Errorf("the gaps-only report lists %+v, want a declared endpoint dropped", found)
	}
}

// TestRun_ADeclarationThatMatchesNothing_IsAFinding verifies that an endpoint
// declaration outlives nothing: once the documentation spells the endpoint out,
// or nothing issues it any more, the excuse left behind is itself reported.
func TestRun_ADeclarationThatMatchesNothing_IsAFinding(t *testing.T) {
	root := t.TempDir()
	makeToolsPackage(t, root, "issues")
	withDeclarations(t, map[string]silentOwnerDeclaration{})
	stubInputs(t, oneRow, []requestinventory.Action{{ID: "issue.list", Owner: "issues"}}, graphqldocs.Result{})
	original := auditEndpoints
	t.Cleanup(func() { auditEndpoints = original })
	auditEndpoints = func(context.Context, *apidocs.Fetcher, []requestinventory.Row) (EndpointCheck, error) {
		return EndpointCheck{Ran: true, UnusedDeclarations: []string{"/orbit/..."}}, nil
	}

	content, clean, err := Run(t.Context(), root, false, apidocs.New(root, apidocs.Options{}))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if clean {
		t.Error("a declaration that accounts for nothing passed the gate")
	}
	if stale := decode(t, content).StaleDeclarations; len(stale) != 1 || !strings.Contains(stale[0], "/orbit/...") {
		t.Errorf("stale = %v, want the unused declaration named", stale)
	}
}

// TestRun_AnInputItCannotRead_Fails verifies that every way the audit fails to
// run at all is reported as an error, never as a tree with nothing wrong with
// it.
func TestRun_AnInputItCannotRead_Fails(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(t *testing.T)
		want    string
	}{
		{
			name: "no committed inventory",
			arrange: func(t *testing.T) {
				t.Helper()
				original := readInventory
				t.Cleanup(func() { readInventory = original })
				readInventory = func(string) (requestinventory.Inventory, error) {
					return requestinventory.Inventory{}, errors.New("read the request inventory: no such file")
				}
			},
			want: "read the request inventory",
		},
		{
			name: "a catalog that will not build",
			arrange: func(t *testing.T) {
				t.Helper()
				original := catalogActions
				t.Cleanup(func() { catalogActions = original })
				catalogActions = func() ([]requestinventory.Action, error) { return nil, errors.New("catalog is broken") }
			},
			want: "build action catalog",
		},
		{
			name: "source the document audit cannot read",
			arrange: func(t *testing.T) {
				t.Helper()
				original := auditGraphQL
				t.Cleanup(func() { auditGraphQL = original })
				auditGraphQL = func(graphqldocs.Options) (graphqldocs.Result, error) {
					return graphqldocs.Result{}, errors.New("load packages")
				}
			},
			want: "read the GraphQL documents",
		},
		{
			name: "a documentation listing it cannot get",
			arrange: func(t *testing.T) {
				t.Helper()
				original := auditEndpoints
				t.Cleanup(func() { auditEndpoints = original })
				auditEndpoints = func(context.Context, *apidocs.Fetcher, []requestinventory.Row) (EndpointCheck, error) {
					return EndpointCheck{}, errors.New("HTTP 404")
				}
			},
			want: "compare the recorded endpoints",
		},
		{
			name: "a report that will not marshal",
			arrange: func(t *testing.T) {
				t.Helper()
				original := marshalIndent
				t.Cleanup(func() { marshalIndent = original })
				marshalIndent = func(any, string, string) ([]byte, error) { return nil, errors.New("no") }
			},
			want: "marshal report",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			makeToolsPackage(t, root, "issues")
			withDeclarations(t, map[string]silentOwnerDeclaration{})
			stubInputs(t, oneRow, []requestinventory.Action{{ID: "issue.list", Owner: "issues"}}, graphqldocs.Result{})
			testCase.arrange(t)

			content, clean, err := Run(t.Context(), root, false, apidocs.New(root, apidocs.Options{}))

			if err == nil {
				t.Fatalf("Run() error = nil, want one naming %q", testCase.want)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Run() error = %q, want it to name %q", err, testCase.want)
			}
			if content != nil || clean {
				t.Errorf("Run() = %q/%v, want nothing and a failed gate", content, clean)
			}
		})
	}
}

// recordingDirEnv is internal/testutil.InventoryDirEnv, spelled again rather
// than imported: that package embeds the pinned GraphQL schema and
// net/http/httptest, and this test needs one string from it. The same reason
// keeps cmd/gen_request_inventory from importing it.
const recordingDirEnv = "GITLAB_MCP_TEST_INVENTORY_DIR"

// TestRun_TheRealTree_PassesItsOwnGate verifies the dimension against this
// repository, which is the only case that can catch an input this scope reads
// wrongly: a committed inventory it cannot parse, a catalog owner it cannot
// map, or a declaration table that has drifted from the tree.
//
// It skips while the suite is being run to record a new inventory, because the
// committed one is then the output under construction rather than an input to
// judge. Without that, adding a domain package deadlocked its own fix: the
// suite failed here because the committed inventory did not know the package
// yet, and `make gen-request-inventory` stops at the failing suite before it
// ever merges the shards that would have taught it. The gate itself still runs
// in `make audit-1to1-paths`, in `make analyze` and in CI.
func TestRun_TheRealTree_PassesItsOwnGate(t *testing.T) {
	if dir := os.Getenv(recordingDirEnv); dir != "" {
		t.Skipf("%s=%s: the committed inventory is being regenerated by this very run", recordingDirEnv, dir)
	}

	content, clean, err := Run(t.Context(), repoRoot(t), false, nil)
	if err != nil {
		t.Fatalf("Run() error = %v, want the real tree audited", err)
	}
	report := decode(t, content)
	if !clean {
		t.Errorf("the repository fails its own R-PATH gate: %+v\nsilent owners: %+v\nrefusals: %+v",
			report.Summary, report.SilentOwners, report.GraphQLRefusals)
	}
	if report.Summary.CatalogActions == 0 || report.Summary.InventoryRows == 0 || report.Summary.GraphQLDocuments == 0 {
		t.Errorf("summary = %+v, want all three inputs to have been read", report.Summary)
	}
	// The floor counts an action as accounted for when its owner recorded
	// something or when its owner's silence is declared, rather than by
	// observation alone. Measured the other way it sat sixteen actions above
	// today's number, so declaring one more package the size of adminspecs
	// would have failed this test for doing exactly what the declaration table
	// is for.
	accounted := report.Summary.ActionsObserved + report.Summary.ActionsSilent - report.Summary.UndeclaredActions
	if accounted*10 < report.Summary.CatalogActions*9 {
		t.Errorf("only %d of %d catalog actions are owned by a package that recorded a request or declared why it did not; the recording has regressed",
			accounted, report.Summary.CatalogActions)
	}
}
