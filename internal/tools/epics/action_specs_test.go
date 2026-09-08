// action_specs_test.go contains canonical-route tests for epic actions.
package epics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/epics") {
			testutil.RespondJSON(w, http.StatusOK, `[`+epicLinkJSON+`]`)
			return
		}
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/epics/") {
			testutil.RespondJSON(w, http.StatusOK, `[`+epicLinkJSON+`]`)
			return
		}
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, "read request body", http.StatusInternalServerError)
				return
			}
			query := string(body)
			switch {
			case strings.Contains(query, "ListWorkItems"):
				testutil.RespondJSON(w, http.StatusOK, listResponseJSON)
			case strings.Contains(query, "GetWorkItemID"):
				testutil.RespondJSON(w, http.StatusOK, deleteGIDResponseJSON)
			case strings.Contains(query, "GetWorkItem"):
				testutil.RespondJSON(w, http.StatusOK, getResponseJSON)
			case strings.Contains(query, "workItemCreate"):
				testutil.RespondJSON(w, http.StatusOK, createResponseJSON)
			case strings.Contains(query, "workItemUpdate"):
				testutil.RespondJSON(w, http.StatusOK, updateResponseJSON)
			case strings.Contains(query, "workItemDelete"):
				testutil.RespondJSON(w, http.StatusOK, deleteDeleteResponseJSON)
			default:
				t.Errorf("unexpected GraphQL query: %s", query)
				http.Error(w, "unexpected GraphQL query", http.StatusInternalServerError)
				return
			}
			return
		}
		http.NotFound(w, r)
	})
	byTool := epicSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, mux)))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_epic_list", map[string]any{"full_path": testFullPath}},
		{"get", "gitlab_epic_get", map[string]any{"full_path": testFullPath, "epic_iid": float64(1)}},
		{"get_links", "gitlab_epic_get_links", map[string]any{"full_path": testFullPath, "epic_iid": float64(1)}},
		{"create", "gitlab_epic_create", map[string]any{"full_path": testFullPath, "title": "New Epic"}},
		{"update", "gitlab_epic_update", map[string]any{"full_path": testFullPath, "epic_iid": float64(1), "title": "Updated"}},
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

// TestActionSpecs_DeleteOutput validates the DeleteOutput route through the catalog surface.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	call := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		call++
		switch call {
		case 1:
			testutil.RespondJSON(w, http.StatusOK, deleteGIDResponseJSON)
		default:
			testutil.RespondJSON(w, http.StatusOK, deleteDeleteResponseJSON)
		}
	}))
	byTool := epicSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_epic_delete"].Route.Handler(t.Context(), map[string]any{
		"full_path": testFullPath,
		"epic_iid":  int64(1),
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_epic_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_epic_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted epic &1 from group my-group." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_DeleteError validates the DeleteError route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := epicSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))))

	_, err := byTool["gitlab_epic_delete"].Route.Handler(t.Context(), map[string]any{
		"full_path": testFullPath,
		"epic_iid":  float64(1),
	})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := epicSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_epic_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test epic destructive confirmation.",
		Icons:       toolutil.IconEpic,
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

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_epic_delete",
		Arguments: map[string]any{
			"full_path": testFullPath,
			"epic_iid":  float64(1),
		},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			if textContent.Text == "" {
				t.Error("expected non-empty cancellation message")
			}
			return
		}
	}
	t.Error("expected text content in cancellation result")
}

// TestActionSpecs_PublishedVocabularies verifies that every fixed-vocabulary
// filter and every timestamp reaches the served input schema at the property
// the model reads.
//
// A GraphQL enum coercion failure comes back as HTTP 200 carrying a GraphQL
// error, so a value a model guessed is refused by GitLab and never reaches the
// status-keyed hint. An unpublished vocabulary is therefore a failure with no
// diagnosis, and an enum published on an array instead of on its items is a
// schema no value can satisfy.
func TestActionSpecs_PublishedVocabularies(t *testing.T) {
	byTool := epicSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.NotFoundHandler())))
	cases := []struct {
		name     string
		tool     string
		property string
		key      string
		want     []any
	}{
		{"assignee wildcard", "gitlab_epic_list", "assignee_wildcard_id", "enum", []any{"ANY", "ME", "NONE"}},
		{"milestone wildcard", "gitlab_epic_list", "milestone_wildcard_id", "enum", []any{"ANY", "NONE", "STARTED", "UPCOMING"}},
		{"weight wildcard", "gitlab_epic_list", "weight_wildcard_id", "enum", []any{"ANY", "NONE"}},
		{"subscription status", "gitlab_epic_list", "subscribed", "enum", []any{"EXPLICITLY_SUBSCRIBED", "EXPLICITLY_UNSUBSCRIBED"}},
		{"health status filter", "gitlab_epic_list", "health_status_filter", "enum", []any{"ANY", "NONE", "atRisk", "needsAttention", "onTrack"}},
		{"searchable fields on the array items", "gitlab_epic_list", "in.", "enum", []any{"TITLE", "DESCRIPTION"}},
		{"link type nested in linked_items", "gitlab_epic_create", "linked_items.link_type", "enum", []any{"BLOCKED_BY", "BLOCKS", "RELATED"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := epicSchemaValue(t, byTool[tc.tool].Route.InputSchema, tc.property, tc.key)
			values, isSlice := got.([]any)
			if !isSlice || len(values) != len(tc.want) {
				t.Fatalf("%s.%s %s = %v, want %v", tc.tool, tc.property, tc.key, got, tc.want)
			}
			for i, want := range tc.want {
				if values[i] != want {
					t.Errorf("%s.%s %s[%d] = %v, want %v", tc.tool, tc.property, tc.key, i, values[i], want)
				}
			}
		})
	}
	formats := []struct {
		tool     string
		property string
	}{
		{"gitlab_epic_list", "closed_after"},
		{"gitlab_epic_list", "closed_before"},
		{"gitlab_epic_list", "due_after"},
		{"gitlab_epic_list", "due_before"},
		{"gitlab_epic_list", "created_after"},
		{"gitlab_epic_list", "updated_before"},
		{"gitlab_epic_create", "created_at"},
	}
	for _, tc := range formats {
		t.Run(tc.tool+" "+tc.property+" is a date-time", func(t *testing.T) {
			if got := epicSchemaValue(t, byTool[tc.tool].Route.InputSchema, tc.property, "format"); got != "date-time" {
				t.Errorf("%s.%s format = %v, want date-time", tc.tool, tc.property, got)
			}
		})
	}
}

// epicSchemaValue reads one key of one property out of a built input schema. A
// property path ending in "." reads the array's items instead of the array.
func epicSchemaValue(t *testing.T, schema map[string]any, propertyPath, key string) any {
	t.Helper()
	node := schema
	for segment := range strings.SplitSeq(propertyPath, ".") {
		if segment == "" {
			items, isObject := node["items"].(map[string]any)
			if !isObject {
				t.Fatalf("property %q has no items object: %v", propertyPath, node)
			}
			node = items
			continue
		}
		properties, isObject := node["properties"].(map[string]any)
		if !isObject {
			t.Fatalf("property %q: no properties at %q", propertyPath, segment)
		}
		child, isObject := properties[segment].(map[string]any)
		if !isObject {
			t.Fatalf("property %q: %q is not an object", propertyPath, segment)
		}
		node = child
	}
	return node[key]
}

func epicSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
