// action_specs_test.go contains canonical-route tests for CI/CD job token scope actions.
package jobtokenscope

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	actionSpecSettingsJSON   = `{"inbound_enabled": true}`
	actionSpecProjectJSON    = `[{"id": 10, "name": "proj-a", "path_with_namespace": "grp/proj-a", "web_url": "https://gitlab.example.com/grp/proj-a"}]`
	actionSpecAddProjectJSON = `{"source_project_id": 42, "target_project_id": 99}`
	actionSpecGroupJSON      = `[{"id": 5, "name": "group-a", "full_path": "group-a", "web_url": "https://gitlab.example.com/groups/group-a"}]`
	actionSpecAddGroupJSON   = `{"source_project_id": 42, "target_group_id": 5}`

	// genericUsage is the placeholder [jobTokenScopeOptions] hands every
	// action before decorateJobTokenScopeMeta replaces it.
	genericUsage = "Use to execute jobtokenscope domain action."
)

// TestActionSpecs_CallAllRoutes exercises every job token scope tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := jobTokenScopeSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, jobTokenScopeActionHandler())))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get_access_settings", "gitlab_get_job_token_access_settings", map[string]any{"project_id": "42"}},
		{"patch_access_settings", "gitlab_patch_job_token_access_settings", map[string]any{"project_id": "42", "enabled": true}},
		{"list_inbound_allowlist", "gitlab_list_job_token_inbound_allowlist", map[string]any{"project_id": "42"}},
		{"add_project_allowlist", "gitlab_add_project_job_token_allowlist", map[string]any{"project_id": "42", "target_project_id": 99}},
		{"remove_project_allowlist", "gitlab_remove_project_job_token_allowlist", map[string]any{"project_id": "42", "target_project_id": 99}},
		{"list_group_allowlist", "gitlab_list_job_token_group_allowlist", map[string]any{"project_id": "42"}},
		{"add_group_allowlist", "gitlab_add_group_job_token_allowlist", map[string]any{"project_id": "42", "target_group_id": 5}},
		{"remove_group_allowlist", "gitlab_remove_group_job_token_allowlist", map[string]any{"project_id": "42", "target_group_id": 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// TestActionSpecs_DeleteErrors verifies remove routes propagate backend errors.
func TestActionSpecs_DeleteErrors(t *testing.T) {
	byTool := jobTokenScopeSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_remove_project_job_token_allowlist", map[string]any{"project_id": "42", "target_project_id": 99}},
		{"gitlab_remove_group_job_token_allowlist", map[string]any{"project_id": "42", "target_group_id": 99}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatal("expected route error")
			}
		})
	}
}

// TestActionSpecs_DeleteOutput verifies remove routes preserve their success messages.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := jobTokenScopeSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, jobTokenScopeActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"gitlab_remove_project_job_token_allowlist", map[string]any{"project_id": "42", "target_project_id": 99}, "Successfully deleted project from job token allowlist."},
		{"gitlab_remove_group_job_token_allowlist", map[string]any{"project_id": "42", "target_group_id": 5}, "Successfully deleted group from job token allowlist."},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			out, ok := result.(toolutil.DeleteOutput)
			if !ok {
				t.Fatalf("Route.Handler(%s) returned %T, want toolutil.DeleteOutput", tt.tool, result)
			}
			if out.Message != tt.want {
				t.Fatalf("delete message = %q, want %q", out.Message, tt.want)
			}
		})
	}
}

// TestCatalogSurface_RemoveConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_RemoveConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := jobTokenScopeSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, toolName := range []string{"gitlab_remove_project_job_token_allowlist", "gitlab_remove_group_job_token_allowlist"} {
		toolutil.RegisterSurfaceToolFromSpec(server, byTool[toolName], toolutil.SurfaceToolRegisterOptions{
			Description: "Test job token scope destructive confirmation.",
			Icons:       toolutil.IconToken,
		})
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_remove_project_job_token_allowlist", map[string]any{"project_id": "42", "target_project_id": 99}},
		{"gitlab_remove_group_job_token_allowlist", map[string]any{"project_id": "42", "target_group_id": 99}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if callErr != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, callErr)
			}
			if result == nil {
				t.Fatalf("expected non-nil result for declined confirmation on %s", tt.name)
			}
		})
	}
}

// TestDecorateJobTokenScopeMeta_UnknownToolNoOp verifies the decorator leaves
// options untouched for a tool name absent from jobTokenScopeActionMeta.
func TestDecorateJobTokenScopeMeta_UnknownToolNoOp(t *testing.T) {
	options := jobTokenScopeOptions("gitlab_unknown_job_token_tool")
	before := options
	decorateJobTokenScopeMeta(&options, "gitlab_unknown_job_token_tool")
	if options.Usage != before.Usage {
		t.Errorf("Usage changed for unknown tool: %q", options.Usage)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("Description set for unknown tool: %q", options.IndividualTool.Description)
	}
	if len(options.RelatedActions) != 0 {
		t.Errorf("RelatedActions set for unknown tool: %v", options.RelatedActions)
	}
}

// TestDecorateJobTokenScopeMeta_EmptyEntry_LeavesEveryOptionAlone verifies each
// of the four metadata fields is copied only when the entry supplies it. Every
// entry in the real table fills all four, so nothing in the package reaches the
// other side of those guards, and the decorator would read as correct with any
// of them gone. What they protect is the shape
// [jobTokenScopeRemoveProjectSpec] already has: options built from
// [jobTokenScopeOptions] and then filled by hand, which is why that tool is the
// one deliberately absent from the table. An entry added for it later carrying
// only a usage would, without these guards, blank its hand-written aliases,
// related actions and description on the way past.
func TestDecorateJobTokenScopeMeta_EmptyEntry_LeavesEveryOptionAlone(t *testing.T) {
	const probe = "gitlab_job_token_scope_meta_probe"
	jobTokenScopeActionMeta[probe] = jobTokenScopeActionMetaEntry{}
	t.Cleanup(func() { delete(jobTokenScopeActionMeta, probe) })

	options := jobTokenScopeOptions(probe)
	options.RelatedActions = []string{actionJobTokenScopeListInbound}
	options.IndividualTool.Description = "hand-written description"
	decorateJobTokenScopeMeta(&options, probe)

	if options.Usage != genericUsage {
		t.Errorf("Usage = %q, want the base usage left as it was", options.Usage)
	}
	if len(options.Aliases) != 1 || options.Aliases[0] != probe {
		t.Errorf("Aliases = %v, want only the tool name the base options set", options.Aliases)
	}
	if len(options.RelatedActions) != 1 || options.RelatedActions[0] != actionJobTokenScopeListInbound {
		t.Errorf("RelatedActions = %v, want the hand-written entry left as it was", options.RelatedActions)
	}
	if options.IndividualTool.Description != "hand-written description" {
		t.Errorf("Description = %q, want the hand-written one left as it was", options.IndividualTool.Description)
	}
}

// TestActionSpecs_DiscoveryMetadataPopulated verifies every projected job token
// scope tool carries non-generic usage, natural-language aliases, related
// actions, and a Returns/See also individual-tool description.
func TestActionSpecs_DiscoveryMetadataPopulated(t *testing.T) {
	byTool := jobTokenScopeSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, jobTokenScopeActionHandler())))
	for tool, spec := range byTool {
		if spec.Usage == genericUsage || spec.Usage == "" {
			t.Errorf("%s: generic or empty usage %q", tool, spec.Usage)
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: empty related actions", tool)
		}
		hasNaturalAlias := false
		for _, a := range spec.Aliases {
			if a != tool {
				hasNaturalAlias = true
				break
			}
		}
		if !hasNaturalAlias {
			t.Errorf("%s: aliases only contain the tool name: %v", tool, spec.Aliases)
		}
		if spec.IndividualTool.Description == "" {
			t.Errorf("%s: empty individual-tool description", tool)
		}
	}
}

// jobTokenScopeDiscoveryExpectation is the aliases and the related actions one
// projected tool must publish.
type jobTokenScopeDiscoveryExpectation struct {
	aliases []string
	related []string
}

// TestActionSpecs_EachToolPublishesItsOwnAliasesAndRelatedActions pins the
// discovery metadata of every projected tool to the tool it belongs to.
//
// The check above it is shape-only, and every entry of
// [jobTokenScopeActionMeta] satisfies it under whichever tool name it sits, so
// two entries can trade aliases and related actions with the suite green. No
// committed artifact catches it either: the individual-tool descriptions are in
// the surface snapshots and the usage lines in llms-full.txt, while aliases and
// related actions are in neither. What a crossing costs is discovery itself,
// since aliases are what gitlab_find_action matches a phrase against: "add
// group to job token allowlist" would answer with the project tool, whose
// schema has no target_group_id.
//
// The expectations are spelled here rather than read from the table under test,
// which would let that table supply its own answer, and the count is held too
// so a new tool cannot be projected without a row of its own.
func TestActionSpecs_EachToolPublishesItsOwnAliasesAndRelatedActions(t *testing.T) {
	want := map[string]jobTokenScopeDiscoveryExpectation{
		"gitlab_get_job_token_access_settings": {
			aliases: []string{"get job token access settings", "show job token scope", "is job token scope enabled"},
			related: []string{"job.token_scope_patch", "job.token_scope_list_inbound", "job.token_scope_list_groups"},
		},
		"gitlab_patch_job_token_access_settings": {
			aliases: []string{"enable job token scope", "disable job token scope", "toggle job token access settings"},
			related: []string{"job.token_scope_get", "job.token_scope_add_project", "job.token_scope_add_group"},
		},
		"gitlab_list_job_token_inbound_allowlist": {
			aliases: []string{"list job token allowlist projects", "show inbound job token allowlist", "audit job token project allowlist"},
			related: []string{"job.token_scope_add_project", "job.token_scope_remove_project", "job.token_scope_get"},
		},
		"gitlab_add_project_job_token_allowlist": {
			aliases: []string{"add project to job token allowlist", "allow project job token access", "grant inbound job token access"},
			related: []string{"job.token_scope_list_inbound", "job.token_scope_remove_project", "job.token_scope_add_group"},
		},
		"gitlab_remove_project_job_token_allowlist": {
			aliases: []string{"remove project from job token allowlist", "revoke project job token access", "delete project job token allowlist entry"},
			related: []string{"job.token_scope_list_inbound", "job.token_scope_add_project", "job.token_scope_remove_group"},
		},
		"gitlab_list_job_token_group_allowlist": {
			aliases: []string{"list job token allowlist groups", "show group job token allowlist", "audit job token group allowlist"},
			related: []string{"job.token_scope_add_group", "job.token_scope_remove_group", "job.token_scope_get"},
		},
		"gitlab_add_group_job_token_allowlist": {
			aliases: []string{"add group to job token allowlist", "allow group job token access", "grant group inbound job token access"},
			related: []string{"job.token_scope_list_groups", "job.token_scope_remove_group", "job.token_scope_add_project"},
		},
		"gitlab_remove_group_job_token_allowlist": {
			aliases: []string{"remove group from job token allowlist", "revoke group job token access", "delete group job token allowlist entry"},
			related: []string{"job.token_scope_list_groups", "job.token_scope_add_group", "job.token_scope_remove_project"},
		},
	}

	byTool := jobTokenScopeSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, jobTokenScopeActionHandler())))
	if len(byTool) != len(want) {
		t.Fatalf("projected %d tools, want %d: a new tool needs its own row here", len(byTool), len(want))
	}
	for tool, spec := range byTool {
		t.Run(tool, func(t *testing.T) {
			expected, ok := want[tool]
			if !ok {
				t.Fatalf("%s: no expectation; add its aliases and related actions", tool)
			}
			if !slices.Equal(spec.Aliases, expected.aliases) {
				t.Errorf("Aliases = %v, want %v", spec.Aliases, expected.aliases)
			}
			if !slices.Equal(spec.RelatedActions, expected.related) {
				t.Errorf("RelatedActions = %v, want %v", spec.RelatedActions, expected.related)
			}
		})
	}
}

func jobTokenScopeActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/42/job_token_scope", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecSettingsJSON)
	})
	handler.HandleFunc("PATCH /api/v4/projects/42/job_token_scope", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("GET /api/v4/projects/42/job_token_scope/allowlist", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecProjectJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/job_token_scope/allowlist", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecAddProjectJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/job_token_scope/allowlist/99", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("GET /api/v4/projects/42/job_token_scope/groups_allowlist", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecGroupJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/job_token_scope/groups_allowlist", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecAddGroupJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/job_token_scope/groups_allowlist/5", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return handler
}

func jobTokenScopeSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
