// customattributes_test.go contains unit tests for the custom attribute MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package customattributes

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const fmtUnexpPath = "unexpected path: %s"

const errExpectedNil = "expected error, got nil"

const testResourceID = "resource_id"

const testKeyDept = "dept"

const fmtErrWantResourceID = "error = %q, want it to contain resource_id"

const testTypeUser = "user"

const testTypeGroup = "group"

const testTypeProject = "project"

// TestList_User verifies the behavior of list user.
func TestList_User(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/users/1/custom_attributes" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
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

// TestList_Group verifies the behavior of list group.
func TestList_Group(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/2/custom_attributes" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
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

// TestList_Project verifies the behavior of list project.
func TestList_Project(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/3/custom_attributes" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
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

// TestList_InvalidType verifies the behavior of list invalid type.
func TestList_InvalidType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := List(t.Context(), client, ListInput{ResourceType: "invalid", ResourceID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGet_User verifies the behavior of get user.
func TestGet_User(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/users/1/custom_attributes/dept" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
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

// TestGet_Error verifies the behavior of get error.
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

// TestSet_Group verifies the behavior of set group.
func TestSet_Group(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/2/custom_attributes/tier" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
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

// TestSet_Error verifies the behavior of set error.
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

// TestDelete_Project verifies the behavior of delete project.
func TestDelete_Project(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/3/custom_attributes/old_key" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ResourceType: testTypeProject, ResourceID: 3, Key: "old_key"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies the behavior of delete error.
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

// TestFormatListMarkdown_Output verifies the behavior of format list markdown output.
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

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No custom attributes") {
		t.Error("missing empty message")
	}
}

// TestFormatGetMarkdown_Output verifies the behavior of format get markdown output.
func TestFormatGetMarkdown_Output(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{AttributeItem: AttributeItem{Key: "k", Value: "v"}})
	if !strings.Contains(md, "k") || !strings.Contains(md, "v") {
		t.Error("missing key/value")
	}
}

// TestList_InvalidResourceID verifies the behavior of list invalid resource i d.
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

// TestGet_InvalidResourceID verifies the behavior of get invalid resource i d.
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

// TestSet_InvalidResourceID verifies the behavior of set invalid resource i d.
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

// TestDelete_InvalidResourceID verifies the behavior of delete invalid resource i d.
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

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// Get — group and project resource types
// ---------------------------------------------------------------------------.

// TestGet_Group verifies the behavior of get group.
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

// TestGet_Project verifies the behavior of get project.
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

// TestGet_InvalidType verifies the behavior of get invalid type.
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

// TestSet_User verifies the behavior of set user.
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

// TestSet_Project verifies the behavior of set project.
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

// TestSet_InvalidType verifies the behavior of set invalid type.
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

// TestDelete_User verifies the behavior of delete user.
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

// TestDelete_Group verifies the behavior of delete group.
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

// TestDelete_InvalidType verifies the behavior of delete invalid type.
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

// TestList_Error verifies the behavior of list error.
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

// TestFormatSetMarkdown_Coverage verifies the behavior of format set markdown coverage.
func TestFormatSetMarkdown_Coverage(t *testing.T) {
	md := FormatSetMarkdown(SetOutput{AttributeItem: AttributeItem{Key: "env", Value: "prod"}})
	if !strings.Contains(md, "env") || !strings.Contains(md, "prod") {
		t.Error("missing key/value in markdown")
	}
	if !strings.Contains(md, "Set") {
		t.Error("missing 'Set' in title")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip
// ---------------------------------------------------------------------------.

// TestMCPRound_Trip validates m c p round trip across multiple scenarios using table-driven subtests.
func TestMCPRound_Trip(t *testing.T) {
	session := newCustomAttrsMCPSession(t)
	ctx := context.Background()

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
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// newCustomAttrsMCPSession is an internal helper for the customattributes package.
// TestMCPRoundTrip_ErrorPaths covers the error return paths in register.go
// handlers when the GitLab API returns an error.
func TestMCPRoundTrip_ErrorPaths(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_custom_attributes", map[string]any{"resource_type": "users", "resource_id": float64(1)}},
		{"gitlab_get_custom_attribute", map[string]any{"resource_type": "users", "resource_id": float64(1), "key": "k"}},
		{"gitlab_set_custom_attribute", map[string]any{"resource_type": "users", "resource_id": float64(1), "key": "k", "value": "v"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("expected error result for %s with 500 backend", tt.name)
			}
		})
	}
}

// TestMCPRoundTrip_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in delete_custom_attribute when user declines.
func TestMCPRoundTrip_DeleteConfirmDeclined(t *testing.T) {
	handler := http.NewServeMux()
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
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
	t.Cleanup(func() { session.Close() })

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
}

func newCustomAttrsMCPSession(t *testing.T) *mcp.ClientSession {
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

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
