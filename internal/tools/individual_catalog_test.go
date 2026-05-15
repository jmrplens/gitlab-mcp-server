package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func TestRegisterIndividualCatalogTools_GoldenSnapshotParity(t *testing.T) {
	goldenPath := filepath.Join("testdata", "tools_individual.json")
	goldenData, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v", goldenPath, err)
	}
	var golden []toolSnapshot
	if unmarshalErr := json.Unmarshal(goldenData, &golden); unmarshalErr != nil {
		t.Fatalf("parse golden file %s: %v", goldenPath, unmarshalErr)
	}
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{PageSize: 2000})
	RegisterIndividualCatalogTools(server, catalog, IndividualCatalogRegisterOptions{
		IncludeStandaloneUtilities: true,
	})

	tools := listToolsFromServer(t, server)
	gotSnapshots := buildSnapshots(t, tools)
	registered := make(map[string]struct{}, len(gotSnapshots))
	for _, snapshot := range gotSnapshots {
		registered[snapshot.Name] = struct{}{}
	}
	wantSnapshots := make([]toolSnapshot, 0, len(gotSnapshots))
	for _, snapshot := range golden {
		if _, ok := registered[snapshot.Name]; ok {
			wantSnapshots = append(wantSnapshots, snapshot)
		}
	}

	compareSnapshotSlices(t, goldenPath, wantSnapshots, gotSnapshots)
}

func TestRegisterAll_CatalogBackedMatchesCatalogProjectionToolNames(t *testing.T) {
	testCases := []struct {
		name       string
		client     *gitlabclient.Client
		enterprise bool
	}{
		{name: "ce", client: newTestClient(t, auditHandler())},
		{name: "self-managed enterprise", client: newTestClient(t, auditHandler()), enterprise: true},
		{name: "gitlab.com enterprise", client: newGitLabDotComClient(t), enterprise: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			catalog := mustBuildActionCatalog(t, tc.client, ActionCatalogOptions{Enterprise: tc.enterprise, IncludeMCP: true})
			expectedServer := mcp.NewServer(&mcp.Implementation{Name: "expected", Version: "0.0.1"}, &mcp.ServerOptions{PageSize: 2000})
			RegisterIndividualCatalogTools(expectedServer, catalog, IndividualCatalogRegisterOptions{IncludeStandaloneUtilities: true})
			RegisterMetaStandaloneTools(expectedServer, tc.client)
			expectedNames := toolNamesFromServer(t, expectedServer)

			catalogServer := mcp.NewServer(&mcp.Implementation{Name: "catalog", Version: "0.0.1"}, &mcp.ServerOptions{PageSize: 2000})
			RegisterAll(catalogServer, tc.client, tc.enterprise)
			catalogNames := toolNamesFromServer(t, catalogServer)

			missing, extra := diffStringSlices(expectedNames, catalogNames)
			if len(missing) > 0 || len(extra) > 0 {
				t.Fatalf("RegisterAll catalog projection name drift\nmissing: %v\nextra: %v", missing, extra)
			}
		})
	}
}

func TestRegisterIndividualCatalogTools_ExecutesCatalogHandler(t *testing.T) {
	type echoInput struct {
		Value string `json:"value" jsonschema:"Value to echo,required"`
	}
	type echoOutput struct {
		Message string `json:"message"`
	}

	called := false
	catalog := testIndividualCatalog(t, toolutil.NewActionSpec("echo", toolutil.RouteAction(nil,
		func(_ context.Context, _ *gitlabclient.Client, input echoInput) (echoOutput, error) {
			called = true
			return echoOutput{Message: input.Value}, nil
		}), toolutil.ActionSpecOptions{
		ReadOnly:       true,
		Idempotent:     true,
		OpenWorld:      true,
		OwnerPackage:   "tools",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_test_echo", Title: "Echo", Description: "Echo a value."},
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterIndividualCatalogTools(server, catalog, IndividualCatalogRegisterOptions{})
	session := connectServerForTools(t, server)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "gitlab_test_echo",
		Arguments: map[string]any{"value": "hello"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() IsError = true: %#v", result.Content)
	}
	if !called {
		t.Fatal("catalog handler was not called")
	}
	structured, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var output echoOutput
	if unmarshalErr := json.Unmarshal(structured, &output); unmarshalErr != nil {
		t.Fatalf("unmarshal structured content: %v", unmarshalErr)
	}
	if output.Message != "hello" {
		t.Fatalf("output message = %q, want hello", output.Message)
	}
}

func TestRegisterIndividualCatalogTools_ReadOnlyAndSafeModePolicies(t *testing.T) {
	type input struct {
		Value string `json:"value" jsonschema:"Value,required"`
	}
	type output struct {
		Value string `json:"value"`
	}

	mutatingCalled := false
	readSpec := toolutil.NewActionSpec("read", toolutil.RouteAction(nil,
		func(_ context.Context, _ *gitlabclient.Client, input input) (output, error) {
			return output(input), nil
		}), toolutil.ActionSpecOptions{
		ReadOnly:       true,
		Idempotent:     true,
		OpenWorld:      true,
		OwnerPackage:   "tools",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_test_read", Title: "Read", Description: "Read a value."},
	})
	writeSpec := toolutil.NewActionSpec("write", toolutil.RouteAction(nil,
		func(_ context.Context, _ *gitlabclient.Client, input input) (output, error) {
			mutatingCalled = true
			return output(input), nil
		}), toolutil.ActionSpecOptions{
		ReadOnly:       false,
		Idempotent:     false,
		OpenWorld:      true,
		OwnerPackage:   "tools",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_test_write", Title: "Write", Description: "Write a value."},
	})
	catalog := testIndividualCatalog(t, readSpec, writeSpec)

	readOnlyServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterIndividualCatalogTools(readOnlyServer, catalog, IndividualCatalogRegisterOptions{ReadOnlyOnly: true})
	readOnlyNames := toolNamesFromServer(t, readOnlyServer)
	if strings.Join(readOnlyNames, ",") != "gitlab_test_read" {
		t.Fatalf("read-only registered tools = %v, want only gitlab_test_read", readOnlyNames)
	}

	safeServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterIndividualCatalogTools(safeServer, catalog, IndividualCatalogRegisterOptions{SafeMode: true})
	session := connectServerForTools(t, safeServer)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "gitlab_test_write",
		Arguments: map[string]any{"value": "blocked"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if mutatingCalled {
		t.Fatal("mutating handler was called in safe mode")
	}
	if len(result.Content) == 0 {
		t.Fatalf("safe mode result content = %#v, want blocked preview", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, `"status":"blocked"`) {
		t.Fatalf("safe mode result content = %#v, want blocked preview", result.Content)
	}
}

func TestRegisterIndividualCatalogTools_EditionFilters(t *testing.T) {
	newSpec := func(name string, opts toolutil.ActionSpecOptions) toolutil.ActionSpec {
		opts.ReadOnly = true
		opts.Idempotent = true
		opts.OpenWorld = true
		opts.OwnerPackage = "tools"
		opts.IndividualTool = toolutil.IndividualToolSpec{Name: "gitlab_test_" + name, Title: toolutil.TitleFromName("gitlab_test_" + name), Description: "Test tool."}
		return toolutil.NewActionSpec(name, toolutil.RouteAction(nil,
			func(_ context.Context, _ *gitlabclient.Client, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			}), opts)
	}

	catalog := testIndividualCatalog(t,
		newSpec("base", toolutil.ActionSpecOptions{}),
		newSpec("enterprise", toolutil.ActionSpecOptions{Edition: "premium"}),
		newSpec("dotcom", toolutil.ActionSpecOptions{GitLabDotComOnly: true}),
	)

	testCases := []struct {
		name string
		opts IndividualCatalogRegisterOptions
		want []string
	}{
		{name: "base", opts: IndividualCatalogRegisterOptions{ApplyEditionFilters: true}, want: []string{"gitlab_test_base"}},
		{name: "enterprise", opts: IndividualCatalogRegisterOptions{ApplyEditionFilters: true, Enterprise: true}, want: []string{"gitlab_test_base", "gitlab_test_enterprise"}},
		{name: "gitlab.com enterprise", opts: IndividualCatalogRegisterOptions{ApplyEditionFilters: true, Enterprise: true, GitLabDotCom: true}, want: []string{"gitlab_test_base", "gitlab_test_dotcom", "gitlab_test_enterprise"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			RegisterIndividualCatalogTools(server, catalog, tc.opts)
			names := toolNamesFromServer(t, server)
			if strings.Join(names, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("registered tools = %v, want %v", names, tc.want)
			}
		})
	}
}

func testIndividualCatalog(t *testing.T, specs ...toolutil.ActionSpec) *actioncatalog.Catalog {
	t.Helper()
	group, err := actioncatalog.GroupFromSpecs(actioncatalog.GroupOptions{
		ToolName:     "gitlab_test",
		Title:        "Test",
		Description:  "Test catalog group.",
		OwnerPackage: "tools",
		SurfaceKind:  actioncatalog.SurfaceKindMetaGroup,
	}, specs)
	if err != nil {
		t.Fatalf("GroupFromSpecs() error = %v", err)
	}
	catalog := actioncatalog.NewCatalog()
	if addGroupErr := catalog.AddGroup(group); addGroupErr != nil {
		t.Fatalf("AddGroup() error = %v", addGroupErr)
	}
	return catalog
}

func listToolsFromServer(t *testing.T, server *mcp.Server) []*mcp.Tool {
	t.Helper()
	session := connectServerForTools(t, server)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}
	return result.Tools
}

func connectServerForTools(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func diffStringSlices(want, got []string) ([]string, []string) {
	wantSet := make(map[string]struct{}, len(want))
	for _, name := range want {
		wantSet[name] = struct{}{}
	}
	gotSet := make(map[string]struct{}, len(got))
	for _, name := range got {
		gotSet[name] = struct{}{}
	}
	missing := make([]string, 0)
	for name := range wantSet {
		if _, ok := gotSet[name]; !ok {
			missing = append(missing, name)
		}
	}
	extra := make([]string, 0)
	for name := range gotSet {
		if _, ok := wantSet[name]; !ok {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}
