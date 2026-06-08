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

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestRegisterIndividualCatalogTools_GoldenSnapshotParity verifies RegisterIndividualCatalogTools when golden snapshot parity.
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

// TestRegisterAll_CatalogBackedMatchesCatalogProjectionToolNames covers RegisterAll with table-driven subtests for catalog backed matches catalog projection tool names.
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

// TestRegisterIndividualCatalogTools_ExecutesCatalogHandler verifies RegisterIndividualCatalogTools when executes catalog handler.
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

// TestRegisterIndividualCatalogTools_ReadOnlyAndSafeModePolicies verifies RegisterIndividualCatalogTools when read only and safe mode policies.
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

// TestRegisterIndividualCatalogTools_EditionFilters covers RegisterIndividualCatalogTools with table-driven subtests for edition filters.
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

	catalog := testIndividualCatalog(
		t,
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

// TestRegisterIndividualCatalogTools_AllowExcludeAndDuplicateTools verifies
// allow lists, exclude lists, and duplicate individual tool names are resolved
// deterministically.
//
// The catalog contains keep, skip, excluded, and duplicate actions. Registration
// should include allowed tools, remove explicitly excluded tools, and register a
// duplicate tool name only once.
func TestRegisterIndividualCatalogTools_AllowExcludeAndDuplicateTools(t *testing.T) {
	newSpec := func(actionName, toolName string) toolutil.ActionSpec {
		return toolutil.NewActionSpec(actionName, toolutil.RouteAction(nil,
			func(_ context.Context, _ *gitlabclient.Client, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			}), toolutil.ActionSpecOptions{
			ReadOnly:       true,
			Idempotent:     true,
			OpenWorld:      true,
			OwnerPackage:   "tools",
			IndividualTool: toolutil.IndividualToolSpec{Name: toolName, Title: toolutil.TitleFromName(toolName), Description: "Test tool."},
		})
	}

	catalog := testIndividualCatalog(
		t,
		newSpec("keep", "gitlab_test_keep"),
		newSpec("skip", "gitlab_test_skip"),
		newSpec("excluded", "gitlab_test_excluded"),
		newSpec("duplicate_a", "gitlab_test_duplicate"),
		newSpec("duplicate_b", "gitlab_test_duplicate"),
	)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterIndividualCatalogTools(server, catalog, IndividualCatalogRegisterOptions{
		AllowedToolNames: []string{"gitlab_test_keep", "gitlab_test_excluded", "gitlab_test_duplicate"},
		ExcludeToolNames: []string{"gitlab_test_excluded"},
	})

	names := toolNamesFromServer(t, server)
	if strings.Join(names, ",") != "gitlab_test_duplicate,gitlab_test_keep" {
		t.Fatalf("registered tools = %v, want duplicate once and keep", names)
	}
}

// TestRegisterIndividualCatalogTools_SkipsIneligibleGroup verifies individual
// projection ignores catalog groups that are not eligible for the selected
// surface.
//
// The test builds a runtime utility group without standalone utility opt-in and
// expects no tools to be registered. This prevents internal maintenance surfaces
// from leaking into individual mode by default.
func TestRegisterIndividualCatalogTools_SkipsIneligibleGroup(t *testing.T) {
	spec := toolutil.NewActionSpec("runtime", toolutil.RouteAction(nil,
		func(_ context.Context, _ *gitlabclient.Client, _ struct{}) (struct{}, error) {
			return struct{}{}, nil
		}), toolutil.ActionSpecOptions{
		ReadOnly:       true,
		Idempotent:     true,
		OpenWorld:      true,
		OwnerPackage:   "tools",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_test_runtime", Title: "Runtime", Description: "Runtime utility."},
	})
	group, err := actioncatalog.GroupFromSpecs(actioncatalog.GroupOptions{
		ToolName:     "gitlab_test_runtime",
		OwnerPackage: "tools",
		SurfaceKind:  actioncatalog.SurfaceKindRuntimeUtility,
	}, []toolutil.ActionSpec{spec})
	if err != nil {
		t.Fatalf("GroupFromSpecs() error = %v", err)
	}
	catalog := actioncatalog.NewCatalog()
	if err = catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterIndividualCatalogTools(server, catalog, IndividualCatalogRegisterOptions{})
	if names := toolNamesFromServer(t, server); len(names) != 0 {
		t.Fatalf("registered tools = %v, want none", names)
	}
}

// TestRegisterIndividualCatalogTools_DestructiveConfirmationDeclined verifies a
// declined elicitation prevents destructive catalog handlers from executing.
//
// The in-memory MCP client always declines the confirmation request. The test
// expects cancellation text and asserts the destructive handler was never called,
// preserving the safety guard for individual tool projection.
func TestRegisterIndividualCatalogTools_DestructiveConfirmationDeclined(t *testing.T) {
	called := false
	spec := toolutil.NewActionSpec("delete", toolutil.RouteAction(nil,
		func(_ context.Context, _ *gitlabclient.Client, _ struct{}) (struct{}, error) {
			called = true
			return struct{}{}, nil
		}), toolutil.ActionSpecOptions{
		Destructive:    true,
		OwnerPackage:   "tools",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_test_delete", Title: "Delete", Description: "Delete test."},
	})
	catalog := testIndividualCatalog(t, spec)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterIndividualCatalogTools(server, catalog, IndividualCatalogRegisterOptions{})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		_ = serverSession.Close()
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Close()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "gitlab_test_delete", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if called {
		t.Fatal("destructive handler executed after declined confirmation")
	}
	if !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "Operation canceled") {
		t.Fatalf("result = %#v, want cancellation", result.Content)
	}
}

// TestMustIndividualToolFromCatalogAction_DescriptionFallbacks verifies
// individual tool descriptions use callbacks, usage text, then title fallback.
//
// Each table case constructs a catalog action with different description inputs
// and expects the projected MCP tool description to follow the fallback order
// used by generated individual tools.
func TestMustIndividualToolFromCatalogAction_DescriptionFallbacks(t *testing.T) {
	newAction := func(name, usage string) actioncatalog.Action {
		return actioncatalog.Action{
			Name: name,
			Route: toolutil.ActionRoute{
				Handler:      func(context.Context, map[string]any) (any, error) { return nil, nil },
				InputSchema:  map[string]any{"type": "object"},
				OutputSchema: map[string]any{"type": "object"},
			},
			Usage:        usage,
			OwnerPackage: "tools",
			IndividualTool: toolutil.IndividualToolSpec{
				Name:  "gitlab_test_" + name,
				Title: toolutil.TitleFromName("gitlab_test_" + name),
			},
		}
	}

	tests := []struct {
		name string
		actioncatalog.Action
		opts IndividualCatalogRegisterOptions
		want string
	}{
		{
			name:   "custom description callback",
			Action: newAction("custom", ""),
			opts: IndividualCatalogRegisterOptions{DescriptionForTool: func(actioncatalog.Action) string {
				return "Generated description."
			}},
			want: "Generated description.",
		},
		{
			name:   "usage fallback",
			Action: newAction("usage", "Use this catalog action."),
			want:   "Use this catalog action.",
		},
		{
			name:   "title fallback",
			Action: newAction("title", ""),
			want:   "Test Title.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := mustIndividualToolFromCatalogAction(tt.Action, nil, tt.opts)
			if tool.Description != tt.want {
				t.Fatalf("Description = %q, want %q", tool.Description, tt.want)
			}
		})
	}
}

// TestMustIndividualToolFromCatalogAction_InvalidActionPanics verifies invalid
// catalog actions fail during individual tool projection.
//
// The action lacks an output schema, so projection should panic instead of
// registering a malformed MCP tool that would fail later at runtime.
func TestMustIndividualToolFromCatalogAction_InvalidActionPanics(t *testing.T) {
	action := actioncatalog.Action{
		Name: "broken",
		Route: toolutil.ActionRoute{
			Handler:     func(context.Context, map[string]any) (any, error) { return struct{}{}, nil },
			InputSchema: map[string]any{"type": "object"},
		},
		OwnerPackage:   "tools",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_test_broken", Title: "Broken"},
	}
	assertPanics(t, func() { _ = mustIndividualToolFromCatalogAction(action, nil, IndividualCatalogRegisterOptions{}) })
}

// TestIndividualCatalogActionReadOnly_AnnotationOverride verifies individual
// annotation overrides can mark an otherwise mutating action as read-only.
//
// The action metadata is mutating, but the individual tool override sets
// ReadOnly to true. The helper should honor that override because it controls
// MCP annotations exposed by individual tools.
func TestIndividualCatalogActionReadOnly_AnnotationOverride(t *testing.T) {
	readOnly := true
	action := actioncatalog.Action{
		ReadOnly: false,
		IndividualTool: toolutil.IndividualToolSpec{AnnotationOverrides: toolutil.IndividualToolAnnotationOverrides{
			ReadOnly: &readOnly,
		}},
	}
	if !individualCatalogActionReadOnly(action) {
		t.Fatal("individualCatalogActionReadOnly() = false, want override true")
	}
}

// TestRegisterIndividualCatalogTools_NilInputs verifies nil server or catalog
// inputs are ignored without panicking.
func TestRegisterIndividualCatalogTools_NilInputs(t *testing.T) {
	RegisterIndividualCatalogTools(nil, nil, IndividualCatalogRegisterOptions{})
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterIndividualCatalogTools(server, nil, IndividualCatalogRegisterOptions{})
}

// TestIndividualCatalogGroupEligible_SurfaceAndEditionGates verifies group
// surface and edition gates used before individual tool projection.
func TestIndividualCatalogGroupEligible_SurfaceAndEditionGates(t *testing.T) {
	if individualCatalogGroupEligible(actioncatalog.Group{SurfaceKind: actioncatalog.SurfaceKindRuntimeUtility}, IndividualCatalogRegisterOptions{}) {
		t.Fatal("runtime utility should require standalone utilities opt-in")
	}
	if !individualCatalogGroupEligible(actioncatalog.Group{SurfaceKind: actioncatalog.SurfaceKindRuntimeUtility}, IndividualCatalogRegisterOptions{IncludeStandaloneUtilities: true}) {
		t.Fatal("runtime utility should be eligible when standalone utilities are included")
	}
	if individualCatalogGroupEligible(actioncatalog.Group{SurfaceKind: "unknown"}, IndividualCatalogRegisterOptions{IncludeStandaloneUtilities: true}) {
		t.Fatal("unknown surface kind should be rejected")
	}
	if individualCatalogGroupEligible(actioncatalog.Group{SurfaceKind: actioncatalog.SurfaceKindMetaGroup, EnterpriseOnly: true}, IndividualCatalogRegisterOptions{ApplyEditionFilters: true}) {
		t.Fatal("enterprise-only group should be rejected without enterprise mode")
	}
	if individualCatalogGroupEligible(actioncatalog.Group{SurfaceKind: actioncatalog.SurfaceKindMetaGroup, GitLabDotComOnly: true}, IndividualCatalogRegisterOptions{ApplyEditionFilters: true, Enterprise: true}) {
		t.Fatal("GitLab.com-only group should be rejected without GitLab.com mode")
	}
}

// TestStringSet_TrimsAndSkipsEmpty verifies stringSet normalizes configured
// allow and deny lists.
func TestStringSet_TrimsAndSkipsEmpty(t *testing.T) {
	if got := stringSet(nil); got != nil {
		t.Fatalf("stringSet(nil) = %+v, want nil", got)
	}
	set := stringSet([]string{" gitlab_get_project ", "", "\t", "gitlab_list_projects"})
	if len(set) != 2 {
		t.Fatalf("stringSet size = %d, want 2", len(set))
	}
	if _, ok := set["gitlab_get_project"]; !ok {
		t.Fatal("trimmed tool name missing from set")
	}
}

// testIndividualCatalog supports test individual catalog assertions in tools tests.
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

// listToolsFromServer supports list tools from server assertions in tools tests.
func listToolsFromServer(t *testing.T, server *mcp.Server) []*mcp.Tool {
	t.Helper()
	session := connectServerForTools(t, server)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf(fmtListToolsErr, err)
	}
	return result.Tools
}

// connectServerForTools supports connect server for tools assertions in tools tests.
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

// diffStringSlices supports diff string slices assertions in tools tests.
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
