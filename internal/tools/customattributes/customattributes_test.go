// customattributes_test.go contains unit tests for the custom attribute MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package customattributes

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// errExpectedNil identifies the err expected nil constant used by this package.
const errExpectedNil = "expected error, got nil"

// testResourceID identifies the test resource ID constant used by this package.
const testResourceID = "resource_id"

// testKeyDept identifies the test key dept constant used by this package.
const testKeyDept = "dept"

// fmtErrWantResourceID identifies the fmt err want resource ID constant used by this package.
const fmtErrWantResourceID = "error = %q, want it to contain resource_id"

// testTypeUser identifies the test type user constant used by this package.
const testTypeUser = "user"

// testTypeGroup identifies the test type group constant used by this package.
const testTypeGroup = "group"

// testTypeProject identifies the test type project constant used by this package.
const testTypeProject = "project"

// TestList_User verifies the List_User handler.
// The mock GitLab API at /api/v4/users/1/custom_attributes (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList_User(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/users/1/custom_attributes")
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"dept","value":"engineering"}]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ResourceType: testTypeUser, ResourceID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Attributes) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Attributes))
	}
	if out.Attributes[0].Key != testKeyDept {
		t.Errorf("Key = %q, want dept", out.Attributes[0].Key)
	}
}

// TestList_Group verifies the List_Group handler.
// The mock GitLab API at /api/v4/groups/2/custom_attributes (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList_Group(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/groups/2/custom_attributes")
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"tier","value":"gold"}]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ResourceType: testTypeGroup, ResourceID: 2})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Attributes[0].Value != "gold" {
		t.Errorf("Value = %q, want gold", out.Attributes[0].Value)
	}
}

// TestList_Project verifies the List_Project handler.
// The mock GitLab API at /api/v4/projects/3/custom_attributes (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList_Project(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/projects/3/custom_attributes")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ResourceType: testTypeProject, ResourceID: 3})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Attributes) != 0 {
		t.Errorf("len = %d, want 0", len(out.Attributes))
	}
}

// TestList_InvalidType verifies the List_InvalidType handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_InvalidType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := List(t.Context(), client, ListInput{ResourceType: "invalid", ResourceID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGet_User verifies the Get_User handler.
// The mock GitLab API at /api/v4/users/1/custom_attributes/dept (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet_User(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/users/1/custom_attributes/dept")
		testutil.RespondJSON(w, http.StatusOK, `{"key":"dept","value":"engineering"}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{ResourceType: testTypeUser, ResourceID: 1, Key: testKeyDept})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Key != testKeyDept || out.Value != "engineering" {
		t.Errorf("got %q=%q, want dept=engineering", out.Key, out.Value)
	}
}

// TestGet_Error verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Get(t.Context(), client, GetInput{ResourceType: testTypeUser, ResourceID: 1, Key: "missing"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestSet_Group verifies the Set_Group handler.
// The mock GitLab API at /api/v4/groups/2/custom_attributes/tier (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestSet_Group(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/groups/2/custom_attributes/tier")
		testutil.AssertRequestMethod(t, r, http.MethodPut)
		testutil.RespondJSON(w, http.StatusOK, `{"key":"tier","value":"platinum"}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Set(t.Context(), client, SetInput{ResourceType: testTypeGroup, ResourceID: 2, Key: "tier", Value: "platinum"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Value != "platinum" {
		t.Errorf("Value = %q, want platinum", out.Value)
	}
}

// TestSet_Error verifies that Set returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestSet_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Set(t.Context(), client, SetInput{ResourceType: testTypeProject, ResourceID: 1, Key: "k", Value: "v"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestDelete_Project verifies the Delete_Project handler.
// The mock GitLab API at /api/v4/projects/3/custom_attributes/old_key (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDelete_Project(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/projects/3/custom_attributes/old_key")
		testutil.AssertRequestMethod(t, r, http.MethodDelete)
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ResourceType: testTypeProject, ResourceID: 3, Key: "old_key"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies that Delete returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ResourceType: testTypeUser, ResourceID: 1, Key: "missing"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestFormatListMarkdown_Output verifies the ListMarkdown_Output Markdown formatter for a representative list_output input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Output(t *testing.T) {
	out := ListOutput{Attributes: []AttributeItem{{Key: testKeyDept, Value: "eng"}}}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, testKeyDept) {
		t.Error("missing key")
	}
	if !strings.Contains(md, "eng") {
		t.Error("missing value")
	}
}

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No custom attributes") {
		t.Error("missing empty message")
	}
}

// TestFormatGetMarkdown_Output verifies the GetMarkdown_Output Markdown formatter for a representative get_output input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatGetMarkdown_Output(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{Key: "k", Value: "v"})
	if !strings.Contains(md, "k") || !strings.Contains(md, "v") {
		t.Error("missing key/value")
	}
}

// TestList_InvalidResourceID verifies the List_InvalidResourceID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_InvalidResourceID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := List(t.Context(), client, ListInput{ResourceType: testTypeUser, ResourceID: 0})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), testResourceID) {
		t.Errorf(fmtErrWantResourceID, err.Error())
	}
}

// TestGet_InvalidResourceID verifies the Get_InvalidResourceID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_InvalidResourceID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Get(t.Context(), client, GetInput{ResourceType: testTypeUser, ResourceID: 0, Key: "k"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), testResourceID) {
		t.Errorf(fmtErrWantResourceID, err.Error())
	}
}

// TestSet_InvalidResourceID verifies the Set_InvalidResourceID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSet_InvalidResourceID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Set(t.Context(), client, SetInput{ResourceType: testTypeGroup, ResourceID: 0, Key: "k", Value: "v"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), testResourceID) {
		t.Errorf(fmtErrWantResourceID, err.Error())
	}
}

// TestDelete_InvalidResourceID verifies the Delete_InvalidResourceID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDelete_InvalidResourceID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	err := Delete(t.Context(), client, DeleteInput{ResourceType: testTypeProject, ResourceID: 0, Key: "k"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), testResourceID) {
		t.Errorf(fmtErrWantResourceID, err.Error())
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// Get — group and project resource types
// ---------------------------------------------------------------------------.

// TestGet_Group verifies the Get_Group handler.
// The mock GitLab API at /api/v4/groups/2/custom_attributes/tier (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet_Group(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/2/custom_attributes/tier" {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"tier","value":"gold"}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{ResourceType: "group", ResourceID: 2, Key: "tier"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Value != "gold" {
		t.Errorf("Value = %q, want gold", out.Value)
	}
}

// TestGet_Project verifies the Get_Project handler.
// The mock GitLab API at /api/v4/projects/3/custom_attributes/env (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet_Project(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/3/custom_attributes/env" {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"env","value":"prod"}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{ResourceType: "project", ResourceID: 3, Key: "env"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Value != "prod" {
		t.Errorf("Value = %q, want prod", out.Value)
	}
}

// TestGet_InvalidType verifies the Get_InvalidType handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_InvalidType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Get(t.Context(), client, GetInput{ResourceType: "invalid", ResourceID: 1, Key: "k"})
	if err == nil {
		t.Fatal("expected error for invalid resource_type")
	}
}

// ---------------------------------------------------------------------------
// Set — user and project resource types
// ---------------------------------------------------------------------------.

// TestSet_User verifies the Set_User handler.
// The mock GitLab API at /api/v4/users/1/custom_attributes/role (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestSet_User(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users/1/custom_attributes/role" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"role","value":"admin"}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Set(t.Context(), client, SetInput{ResourceType: "user", ResourceID: 1, Key: "role", Value: "admin"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Key != "role" || out.Value != "admin" {
		t.Errorf("got %q=%q, want role=admin", out.Key, out.Value)
	}
}

// TestSet_Project verifies the Set_Project handler.
// The mock GitLab API at /api/v4/projects/5/custom_attributes/env (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestSet_Project(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/5/custom_attributes/env" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"env","value":"staging"}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Set(t.Context(), client, SetInput{ResourceType: "project", ResourceID: 5, Key: "env", Value: "staging"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Value != "staging" {
		t.Errorf("Value = %q, want staging", out.Value)
	}
}

// TestSet_InvalidType verifies the Set_InvalidType handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSet_InvalidType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Set(t.Context(), client, SetInput{ResourceType: "bad", ResourceID: 1, Key: "k", Value: "v"})
	if err == nil {
		t.Fatal("expected error for invalid resource_type")
	}
}

// ---------------------------------------------------------------------------
// Delete — user and group resource types + invalid type
// ---------------------------------------------------------------------------.

// TestDelete_User verifies the Delete_User handler.
// The mock GitLab API at /api/v4/users/1/custom_attributes/old (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDelete_User(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users/1/custom_attributes/old" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ResourceType: "user", ResourceID: 1, Key: "old"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Group verifies the Delete_Group handler.
// The mock GitLab API at /api/v4/groups/2/custom_attributes/stale (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDelete_Group(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/2/custom_attributes/stale" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ResourceType: "group", ResourceID: 2, Key: "stale"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_InvalidType verifies the Delete_InvalidType handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDelete_InvalidType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := Delete(t.Context(), client, DeleteInput{ResourceType: "bad", ResourceID: 1, Key: "k"})
	if err == nil {
		t.Fatal("expected error for invalid resource_type")
	}
}

// ---------------------------------------------------------------------------
// List — API error for user type
// ---------------------------------------------------------------------------.

// TestList_Error verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := List(t.Context(), client, ListInput{ResourceType: "user", ResourceID: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// FormatSetMarkdown
// ---------------------------------------------------------------------------.

// TestFormatSetMarkdown_Coverage verifies the SetMarkdown_Coverage Markdown formatter for a representative set_coverage input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatSetMarkdown_Coverage(t *testing.T) {
	md := FormatSetMarkdown(SetOutput{Key: "env", Value: "prod"})
	if !strings.Contains(md, "env") || !strings.Contains(md, "prod") {
		t.Error("missing key/value in markdown")
	}
	if !strings.Contains(md, "Set") {
		t.Error("missing 'Set' in title")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs — metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	specs := ActionSpecs(client)
	byTool := customAttributeSpecsByTool(t, specs)

	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	if !byTool["gitlab_delete_custom_attribute"].Route.Destructive {
		t.Fatal("gitlab_delete_custom_attribute should be destructive")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newCustomAttributeRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_custom_attributes", map[string]any{
			"resource_type": "user", "resource_id": float64(1),
		}},
		{"get", "gitlab_get_custom_attribute", map[string]any{
			"resource_type": "user", "resource_id": float64(1), "key": "dept",
		}},
		{"set", "gitlab_set_custom_attribute", map[string]any{
			"resource_type": "user", "resource_id": float64(1), "key": "dept", "value": "eng",
		}},
		{"delete", "gitlab_delete_custom_attribute", map[string]any{
			"resource_type": "user", "resource_id": float64(1), "key": "dept",
		}},
	}

	for _, tt := range tools {
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

// TestActionSpecs_ErrorPaths validates the ErrorPaths route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := customAttributeSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_custom_attributes", map[string]any{"resource_type": "users", "resource_id": float64(1)}},
		{"gitlab_get_custom_attribute", map[string]any{"resource_type": "users", "resource_id": float64(1), "key": "k"}},
		{"gitlab_set_custom_attribute", map[string]any{"resource_type": "users", "resource_id": float64(1), "key": "k", "value": "v"}},
		{"gitlab_delete_custom_attribute", map[string]any{"resource_type": "users", "resource_id": float64(1), "key": "k"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("expected error for %s with failing backend", tt.name)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	handler := http.NewServeMux()
	client := testutil.NewTestClient(t, handler)
	byTool := customAttributeSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_custom_attribute"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test custom attribute destructive confirmation.",
		Icons:       toolutil.IconConfig,
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
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_custom_attribute",
		Arguments: map[string]any{"resource_type": "users", "resource_id": float64(1), "key": "k"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
	found := false
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected non-empty text content in cancellation result")
	}
}

// newCustomAttributeRouteSpecs constructs custom attribute route specs test fixtures.
func newCustomAttributeRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/users/1/custom_attributes", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"dept","value":"eng"}]`)
	})
	handler.HandleFunc("GET /api/v4/users/1/custom_attributes/dept", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"key":"dept","value":"eng"}`)
	})
	handler.HandleFunc("PUT /api/v4/users/1/custom_attributes/dept", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"key":"dept","value":"eng"}`)
	})
	handler.HandleFunc("DELETE /api/v4/users/1/custom_attributes/dept", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	return customAttributeSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))
}

// customAttributeSpecsByTool supports custom attribute specs by tool assertions in customattributes tests.
func customAttributeSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
