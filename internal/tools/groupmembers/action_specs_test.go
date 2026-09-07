// action_specs_test.go contains canonical-route tests for group member actions.
package groupmembers

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	memberJSON := `{"id":10,"username":"dev","name":"Developer","state":"active","access_level":30}`
	groupJSON := `{"id":5,"name":"MyGroup","path":"mygroup","web_url":"https://gl/groups/mygroup"}`
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, memberJSON)
	})
	handler.HandleFunc("GET /api/v4/groups/5/members/all/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, memberJSON)
	})
	handler.HandleFunc("POST /api/v4/groups/5/members", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":20,"username":"newuser","name":"New User","state":"active","access_level":30}`)
	})
	handler.HandleFunc("PUT /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"username":"dev","name":"Developer","state":"active","access_level":40}`)
	})
	handler.HandleFunc("DELETE /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("POST /api/v4/groups/5/share", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, groupJSON)
	})
	handler.HandleFunc("DELETE /api/v4/groups/5/share/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("GET /api/v4/groups/5/billable_members", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"username":"dev","name":"Developer","state":"active"}]`)
	})
	handler.HandleFunc("GET /api/v4/groups/5/billable_members/10/memberships", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":99,"source_id":7,"source_full_name":"Org / Team"}]`)
	})
	handler.HandleFunc("DELETE /api/v4/groups/5/billable_members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := groupMemberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get", "gitlab_group_member_get", map[string]any{"group_id": "5", "user_id": 10}},
		{"get_inherited", "gitlab_group_member_get_inherited", map[string]any{"group_id": "5", "user_id": 10}},
		{"add", "gitlab_group_member_add", map[string]any{"group_id": "5", "user_id": 20, "access_level": 30}},
		{"edit", "gitlab_group_member_edit", map[string]any{"group_id": "5", "user_id": 10, "access_level": 40}},
		{"remove", "gitlab_group_member_remove", map[string]any{"group_id": "5", "user_id": 10}},
		{"share", "gitlab_group_share", map[string]any{"group_id": "5", "share_group_id": 10, "group_access": 30}},
		{"unshare", "gitlab_group_unshare", map[string]any{"group_id": "5", "share_group_id": 10}},
		{"billable_members", "gitlab_list_billable_group_members", map[string]any{"group_id": "5"}},
		{"billable_memberships", "gitlab_list_billable_member_memberships", map[string]any{"group_id": "5", "user_id": 10}},
		{"billable_remove", "gitlab_remove_billable_group_member", map[string]any{"group_id": "5", "user_id": 10}},
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

// TestActionSpecs_DeleteErrors validates the DeleteErrors route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_DeleteErrors(t *testing.T) {
	byTool := groupMemberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		{"gitlab_group_member_remove", map[string]any{"group_id": "42", "user_id": 1}},
		{"gitlab_group_unshare", map[string]any{"group_id": "42", "share_group_id": 99}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error", tt.tool)
			}
		})
	}
}

// TestActionSpecs_DeleteOutputs validates the DeleteOutputs route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_DeleteOutputs(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("DELETE /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("DELETE /api/v4/groups/5/share/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := groupMemberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		tool    string
		args    map[string]any
		message string
	}{
		{"gitlab_group_member_remove", map[string]any{"group_id": "5", "user_id": 10}, "Successfully deleted group member."},
		{"gitlab_group_unshare", map[string]any{"group_id": "5", "share_group_id": 10}, "Successfully deleted group share."},
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
			if out.Message != tt.message {
				t.Fatalf("delete message = %q", out.Message)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := groupMemberSpecsByTool(t, ActionSpecs(client))

	for _, tt := range []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_member_remove", map[string]any{"group_id": "42", "user_id": 1}},
		{"gitlab_group_unshare", map[string]any{"group_id": "42", "share_group_id": 99}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			toolutil.RegisterSurfaceToolFromSpec(server, byTool[tt.name], toolutil.SurfaceToolRegisterOptions{
				Description: "Test group member destructive confirmation.",
				Icons:       toolutil.IconUser,
			})

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

			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool returned transport error: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result when confirmation is declined")
			}
		})
	}
}

// TestActionSpecs_Metadata verifies that every group-member action carries
// non-generic discovery metadata (Usage, natural-language aliases, canonical
// related actions, parameter guidance, and an individual-tool description) per
// the 1:1 audit R-META requirement.
func TestActionSpecs_Metadata(t *testing.T) {
	byTool := groupMemberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.NewServeMux())))

	for tool := range groupMemberActionMeta {
		spec, ok := byTool[tool]
		if !ok {
			t.Fatalf("metadata declared for %q but no spec projects it", tool)
		}
		t.Run(tool, func(t *testing.T) {
			assertGroupMemberMeta(t, tool, spec)
		})
	}
}

// assertGroupMemberMeta checks that one projected spec carries the required
// R-META discovery metadata.
func assertGroupMemberMeta(t *testing.T, tool string, spec toolutil.ActionSpec) {
	t.Helper()
	if spec.Usage == "" || spec.Usage == "Use to execute groupmembers domain action." {
		t.Errorf("%s: generic or empty Usage: %q", tool, spec.Usage)
	}
	if spec.IndividualTool.Description == "" {
		t.Errorf("%s: missing individual-tool Description", tool)
	}
	if len(spec.RelatedActions) == 0 {
		t.Errorf("%s: missing RelatedActions", tool)
	}
	if _, ok := spec.ParameterGuidance["group_id"]; !ok {
		t.Errorf("%s: ParameterGuidance missing group_id", tool)
	}
	// The canonical individual-tool alias must remain present alongside the
	// natural-language aliases.
	if !slices.Contains(spec.Aliases, tool) {
		t.Errorf("%s: canonical alias %q dropped from aliases %v", tool, tool, spec.Aliases)
	}
}

// TestDecorateGroupMemberMeta_UnknownTool verifies the no-op path of
// decorateGroupMemberMeta for a tool name absent from the metadata map.
func TestDecorateGroupMemberMeta_UnknownTool(t *testing.T) {
	options := groupMemberOptions("gitlab_unknown_tool")
	before := options.Usage
	decorateGroupMemberMeta(&options, "gitlab_unknown_tool")
	if options.Usage != before {
		t.Errorf("expected Usage unchanged for unknown tool, got %q", options.Usage)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("expected empty Description for unknown tool, got %q", options.IndividualTool.Description)
	}
}

// TestActionSpecs_BillableMembersSortEnum verifies the schema
// gitlab_list_billable_group_members serves publishes the combined
// field_direction tokens GitLab documents for the billable members endpoint,
// and neither half of the asc/desc pair.
//
// The pair is what toolutil injects into any string sort carrying no enum of
// its own, and this endpoint accepts neither value, so without the override
// the server would advertise two values GitLab rejects while refusing the ten
// it accepts. The sibling memberships action is asserted alongside because its
// sort really is the plain direction pair: the override belongs to one action
// and a test that only looked at that one could not say so.
//
// GitLab API docs: https://docs.gitlab.com/api/members/#list-all-billable-members-of-a-group
func TestActionSpecs_BillableMembersSortEnum(t *testing.T) {
	byTool := groupMemberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.NewServeMux())))

	t.Run(toolBillableMembers, func(t *testing.T) {
		served := servedEnum(t, byTool[toolBillableMembers].Route.InputSchema, "sort")
		want := []string{
			"access_level_asc", "access_level_desc",
			"last_activity_on_asc", "last_activity_on_desc",
			"last_joined", "name_asc", "name_desc",
			"oldest_joined", "oldest_sign_in", "recent_sign_in",
		}
		slices.Sort(served)
		if !slices.Equal(served, want) {
			t.Errorf("served sort enum = %v, want the documented billable members values %v", served, want)
		}
	})

	t.Run(toolBillableMemberMemberships, func(t *testing.T) {
		served := servedEnum(t, byTool[toolBillableMemberMemberships].Route.InputSchema, "sort")
		slices.Sort(served)
		if !slices.Equal(served, []string{"asc", "desc"}) {
			t.Errorf("served sort enum = %v, want the plain direction pair this endpoint really takes", served)
		}
	})
}

// servedEnum returns the enum values a built input schema publishes for one
// top-level property, which is what a client is offered after the action's
// overrides and the canonical parameter enums have both been applied.
func servedEnum(t *testing.T, schema map[string]any, property string) []string {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("input schema has no properties: %v", schema)
	}
	field, ok := properties[property].(map[string]any)
	if !ok {
		t.Fatalf("input schema has no %q property", property)
	}
	enum, ok := field["enum"].([]any)
	if !ok {
		t.Fatalf("%q publishes no enum: %v", property, field)
	}
	values := make([]string, 0, len(enum))
	for _, value := range enum {
		text, isText := value.(string)
		if !isText {
			t.Fatalf("%q enum carries a non-string value %v", property, value)
		}
		values = append(values, text)
	}
	return values
}

func groupMemberSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
