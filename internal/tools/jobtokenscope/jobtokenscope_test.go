// jobtokenscope_test.go contains unit tests for the job token scope MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package jobtokenscope

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestGetAccessSettings_Success verifies the behavior of get access settings success.
func TestGetAccessSettings_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/job_token_scope" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"inbound_enabled": true}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := GetAccessSettings(t.Context(), client, GetAccessSettingsInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.InboundEnabled {
		t.Error("expected inbound_enabled=true")
	}
}

// TestGetAccessSettings_Error verifies the behavior of get access settings error.
func TestGetAccessSettings_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"error"}`)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := GetAccessSettings(t.Context(), client, GetAccessSettingsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestPatchAccessSettings_Success verifies the behavior of patch access settings success.
func TestPatchAccessSettings_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := PatchAccessSettings(t.Context(), client, PatchAccessSettingsInput{ProjectID: "42", Enabled: true})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "updated" {
		t.Errorf("expected status 'updated', got %q", out.Status)
	}
}

// TestListInboundAllowlist_Success verifies the behavior of list inbound allowlist success.
func TestListInboundAllowlist_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id": 10, "name": "project-a", "path_with_namespace": "group/project-a", "web_url": "https://gitlab.example.com/group/project-a"}
		]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := ListInboundAllowlist(t.Context(), client, ListInboundAllowlistInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(out.Projects))
	}
	if out.Projects[0].ID != 10 {
		t.Errorf("expected ID 10, got %d", out.Projects[0].ID)
	}
	if out.Projects[0].Name != "project-a" {
		t.Errorf("expected name 'project-a', got %q", out.Projects[0].Name)
	}
}

// TestAddProjectAllowlist_Success verifies the behavior of add project allowlist success.
func TestAddProjectAllowlist_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"source_project_id": 42, "target_project_id": 99}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := AddProjectAllowlist(t.Context(), client, AddProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.SourceProjectID != 42 {
		t.Errorf("expected source 42, got %d", out.SourceProjectID)
	}
	if out.TargetProjectID != 99 {
		t.Errorf("expected target 99, got %d", out.TargetProjectID)
	}
}

// TestRemoveProjectAllowlist_Success verifies the behavior of remove project allowlist success.
func TestRemoveProjectAllowlist_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)
	err := RemoveProjectAllowlist(t.Context(), client, RemoveProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestRemoveProjectAllowlist_Error verifies the behavior of remove project allowlist error.
func TestRemoveProjectAllowlist_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"error"}`)
	})
	client := testutil.NewTestClient(t, handler)
	err := RemoveProjectAllowlist(t.Context(), client, RemoveProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestListGroupAllowlist_Success verifies the behavior of list group allowlist success.
func TestListGroupAllowlist_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id": 5, "name": "my-group", "full_path": "my-group", "web_url": "https://gitlab.example.com/groups/my-group"}
		]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := ListGroupAllowlist(t.Context(), client, ListGroupAllowlistInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(out.Groups))
	}
	if out.Groups[0].ID != 5 {
		t.Errorf("expected ID 5, got %d", out.Groups[0].ID)
	}
}

// TestAddGroupAllowlist_Success verifies the behavior of add group allowlist success.
func TestAddGroupAllowlist_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"source_project_id": 42, "target_group_id": 5}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := AddGroupAllowlist(t.Context(), client, AddGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.TargetGroupID != 5 {
		t.Errorf("expected target_group_id 5, got %d", out.TargetGroupID)
	}
}

// TestRemoveGroupAllowlist_Success verifies the behavior of remove group allowlist success.
func TestRemoveGroupAllowlist_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)
	err := RemoveGroupAllowlist(t.Context(), client, RemoveGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestAddProjectAllowlist_ZeroTargetProjectID verifies the behavior of add project allowlist zero target project i d.
func TestAddProjectAllowlist_ZeroTargetProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when TargetProjectID is 0")
	}))
	_, err := AddProjectAllowlist(t.Context(), client, AddProjectAllowlistInput{ProjectID: "42", TargetProjectID: 0})
	if err == nil {
		t.Fatal("expected error for zero TargetProjectID, got nil")
	}
}

// TestRemoveProjectAllowlist_ZeroTargetProjectID verifies the behavior of remove project allowlist zero target project i d.
func TestRemoveProjectAllowlist_ZeroTargetProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when TargetProjectID is 0")
	}))
	err := RemoveProjectAllowlist(t.Context(), client, RemoveProjectAllowlistInput{ProjectID: "42", TargetProjectID: 0})
	if err == nil {
		t.Fatal("expected error for zero TargetProjectID, got nil")
	}
}

// TestAddGroupAllowlist_ZeroTargetGroupID verifies the behavior of add group allowlist zero target group i d.
func TestAddGroupAllowlist_ZeroTargetGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when TargetGroupID is 0")
	}))
	_, err := AddGroupAllowlist(t.Context(), client, AddGroupAllowlistInput{ProjectID: "42", TargetGroupID: 0})
	if err == nil {
		t.Fatal("expected error for zero TargetGroupID, got nil")
	}
}

// TestRemoveGroupAllowlist_ZeroTargetGroupID verifies the behavior of remove group allowlist zero target group i d.
func TestRemoveGroupAllowlist_ZeroTargetGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when TargetGroupID is 0")
	}))
	err := RemoveGroupAllowlist(t.Context(), client, RemoveGroupAllowlistInput{ProjectID: "42", TargetGroupID: 0})
	if err == nil {
		t.Fatal("expected error for zero TargetGroupID, got nil")
	}
}

// TestFormatAccessSettingsMarkdown verifies the behavior of format access settings markdown.
func TestFormatAccessSettingsMarkdown(t *testing.T) {
	r := FormatAccessSettingsMarkdown(AccessSettingsOutput{InboundEnabled: true})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatListInboundAllowlistMarkdown_Empty verifies the behavior of format list inbound allowlist markdown empty.
func TestFormatListInboundAllowlistMarkdown_Empty(t *testing.T) {
	r := FormatListInboundAllowlistMarkdown(ListInboundAllowlistOutput{})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatListGroupAllowlistMarkdown_Empty verifies the behavior of format list group allowlist markdown empty.
func TestFormatListGroupAllowlistMarkdown_Empty(t *testing.T) {
	r := FormatListGroupAllowlistMarkdown(ListGroupAllowlistOutput{})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpNonNilResult = "expected non-nil result"

const errExpCancelledCtx = "expected error for canceled context"

const errExpectedAPI = "expected API error, got nil"

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// GetAccessSettings — canceled context
// ---------------------------------------------------------------------------.

// TestGetAccessSettings_CancelledContext verifies the behavior of get access settings cancelled context.
func TestGetAccessSettings_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetAccessSettings(ctx, client, GetAccessSettingsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// PatchAccessSettings — API error, canceled context
// ---------------------------------------------------------------------------.

// TestPatchAccessSettings_APIError verifies the behavior of patch access settings a p i error.
func TestPatchAccessSettings_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := PatchAccessSettings(context.Background(), client, PatchAccessSettingsInput{ProjectID: "42", Enabled: true})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPatchAccessSettings_CancelledContext verifies the behavior of patch access settings cancelled context.
func TestPatchAccessSettings_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := PatchAccessSettings(ctx, client, PatchAccessSettingsInput{ProjectID: "42", Enabled: false})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ListInboundAllowlist — API error, canceled context, pagination
// ---------------------------------------------------------------------------.

// TestListInboundAllowlist_APIError verifies the behavior of list inbound allowlist a p i error.
func TestListInboundAllowlist_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListInboundAllowlist(context.Background(), client, ListInboundAllowlistInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListInboundAllowlist_CancelledContext verifies the behavior of list inbound allowlist cancelled context.
func TestListInboundAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ListInboundAllowlist(ctx, client, ListInboundAllowlistInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListInboundAllowlist_WithPagination verifies the behavior of list inbound allowlist with pagination.
func TestListInboundAllowlist_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id": 10, "name": "proj-a", "path_with_namespace": "grp/proj-a", "web_url": "https://gitlab.example.com/grp/proj-a"},
			{"id": 11, "name": "proj-b", "path_with_namespace": "grp/proj-b", "web_url": "https://gitlab.example.com/grp/proj-b"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "2"})
	}))
	out, err := ListInboundAllowlist(context.Background(), client, ListInboundAllowlistInput{ProjectID: "42", Page: 1, PerPage: 2})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(out.Projects))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("NextPage = %d, want 2", out.Pagination.NextPage)
	}
}

// TestListInboundAllowlist_Empty verifies the behavior of list inbound allowlist empty.
func TestListInboundAllowlist_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := ListInboundAllowlist(context.Background(), client, ListInboundAllowlistInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(out.Projects))
	}
}

// ---------------------------------------------------------------------------
// AddProjectAllowlist — API error, canceled context
// ---------------------------------------------------------------------------.

// TestAddProjectAllowlist_APIError verifies the behavior of add project allowlist a p i error.
func TestAddProjectAllowlist_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := AddProjectAllowlist(context.Background(), client, AddProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAddProjectAllowlist_CancelledContext verifies the behavior of add project allowlist cancelled context.
func TestAddProjectAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := AddProjectAllowlist(ctx, client, AddProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// RemoveProjectAllowlist — canceled context
// ---------------------------------------------------------------------------.

// TestRemoveProjectAllowlist_CancelledContext verifies the behavior of remove project allowlist cancelled context.
func TestRemoveProjectAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RemoveProjectAllowlist(ctx, client, RemoveProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ListGroupAllowlist — API error, canceled context, pagination, empty
// ---------------------------------------------------------------------------.

// TestListGroupAllowlist_APIError verifies the behavior of list group allowlist a p i error.
func TestListGroupAllowlist_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListGroupAllowlist(context.Background(), client, ListGroupAllowlistInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListGroupAllowlist_CancelledContext verifies the behavior of list group allowlist cancelled context.
func TestListGroupAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ListGroupAllowlist(ctx, client, ListGroupAllowlistInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListGroupAllowlist_WithPagination verifies the behavior of list group allowlist with pagination.
func TestListGroupAllowlist_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id": 5, "name": "group-a", "full_path": "group-a", "web_url": "https://gitlab.example.com/groups/group-a"},
			{"id": 6, "name": "group-b", "full_path": "group-b", "web_url": "https://gitlab.example.com/groups/group-b"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "2", Total: "4", TotalPages: "2", NextPage: "2"})
	}))
	out, err := ListGroupAllowlist(context.Background(), client, ListGroupAllowlistInput{ProjectID: "42", Page: 1, PerPage: 2})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(out.Groups))
	}
	if out.Pagination.TotalPages != 2 {
		t.Errorf("TotalPages = %d, want 2", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("NextPage = %d, want 2", out.Pagination.NextPage)
	}
}

// TestListGroupAllowlist_Empty verifies the behavior of list group allowlist empty.
func TestListGroupAllowlist_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := ListGroupAllowlist(context.Background(), client, ListGroupAllowlistInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(out.Groups))
	}
}

// ---------------------------------------------------------------------------
// AddGroupAllowlist — API error, canceled context
// ---------------------------------------------------------------------------.

// TestAddGroupAllowlist_APIError verifies the behavior of add group allowlist a p i error.
func TestAddGroupAllowlist_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := AddGroupAllowlist(context.Background(), client, AddGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAddGroupAllowlist_CancelledContext verifies the behavior of add group allowlist cancelled context.
func TestAddGroupAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := AddGroupAllowlist(ctx, client, AddGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// RemoveGroupAllowlist — API error, canceled context
// ---------------------------------------------------------------------------.

// TestRemoveGroupAllowlist_APIError verifies the behavior of remove group allowlist a p i error.
func TestRemoveGroupAllowlist_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := RemoveGroupAllowlist(context.Background(), client, RemoveGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestRemoveGroupAllowlist_CancelledContext verifies the behavior of remove group allowlist cancelled context.
func TestRemoveGroupAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RemoveGroupAllowlist(ctx, client, RemoveGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// FormatAccessSettingsMarkdown — disabled state
// ---------------------------------------------------------------------------.

// TestFormatAccessSettingsMarkdown_Disabled verifies the behavior of format access settings markdown disabled.
func TestFormatAccessSettingsMarkdown_Disabled(t *testing.T) {
	r := FormatAccessSettingsMarkdown(AccessSettingsOutput{InboundEnabled: false})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := r.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "disabled") {
		t.Errorf("expected 'disabled' in markdown, got: %s", text)
	}
}

// TestFormatAccessSettingsMarkdown_Enabled verifies the behavior of format access settings markdown enabled.
func TestFormatAccessSettingsMarkdown_Enabled(t *testing.T) {
	r := FormatAccessSettingsMarkdown(AccessSettingsOutput{InboundEnabled: true})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := r.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "enabled") {
		t.Errorf("expected 'enabled' in markdown, got: %s", text)
	}
	if strings.Contains(text, "disabled") {
		t.Errorf("should not contain 'disabled' when enabled, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// FormatPatchResultMarkdown
// ---------------------------------------------------------------------------.

// TestFormatPatchResultMarkdown verifies the behavior of format patch result markdown.
func TestFormatPatchResultMarkdown(t *testing.T) {
	r := FormatPatchResultMarkdown(toolutil.DeleteOutput{Status: "updated"})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := r.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "updated") {
		t.Errorf("expected 'updated' in markdown, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// FormatListInboundAllowlistMarkdown — with data
// ---------------------------------------------------------------------------.

// TestFormatListInboundAllowlistMarkdown_WithData verifies the behavior of format list inbound allowlist markdown with data.
func TestFormatListInboundAllowlistMarkdown_WithData(t *testing.T) {
	r := FormatListInboundAllowlistMarkdown(ListInboundAllowlistOutput{
		Projects: []AllowlistProjectItem{
			{ID: 10, Name: "proj-a", PathWithNamespace: "grp/proj-a", WebURL: "https://gitlab.example.com/grp/proj-a"},
			{ID: 11, Name: "proj-b", PathWithNamespace: "grp/proj-b", WebURL: "https://gitlab.example.com/grp/proj-b"},
		},
	})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := r.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{
		"Job Token Inbound Allowlist (2 projects)",
		"| ID |",
		"| 10 |",
		"| 11 |",
		"proj-a",
		"proj-b",
		"grp/proj-a",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("markdown missing %q:\n%s", want, text)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatAddProjectAllowlistMarkdown
// ---------------------------------------------------------------------------.

// TestFormatAddProjectAllowlistMarkdown verifies the behavior of format add project allowlist markdown.
func TestFormatAddProjectAllowlistMarkdown(t *testing.T) {
	r := FormatAddProjectAllowlistMarkdown(InboundAllowItemOutput{SourceProjectID: 42, TargetProjectID: 99})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := r.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "99") {
		t.Errorf("expected target project ID in markdown, got: %s", text)
	}
	if !strings.Contains(text, "42") {
		t.Errorf("expected source project ID in markdown, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// FormatListGroupAllowlistMarkdown — with data
// ---------------------------------------------------------------------------.

// TestFormatListGroupAllowlistMarkdown_WithData verifies the behavior of format list group allowlist markdown with data.
func TestFormatListGroupAllowlistMarkdown_WithData(t *testing.T) {
	r := FormatListGroupAllowlistMarkdown(ListGroupAllowlistOutput{
		Groups: []AllowlistGroupItem{
			{ID: 5, Name: "group-a", FullPath: "group-a", WebURL: "https://gitlab.example.com/groups/group-a"},
			{ID: 6, Name: "group-b", FullPath: "org/group-b", WebURL: "https://gitlab.example.com/groups/org/group-b"},
		},
	})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := r.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{
		"Job Token Group Allowlist (2 groups)",
		"| ID |",
		"| 5 |",
		"| 6 |",
		"group-a",
		"group-b",
		"org/group-b",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("markdown missing %q:\n%s", want, text)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatAddGroupAllowlistMarkdown
// ---------------------------------------------------------------------------.

// TestFormatAddGroupAllowlistMarkdown verifies the behavior of format add group allowlist markdown.
func TestFormatAddGroupAllowlistMarkdown(t *testing.T) {
	r := FormatAddGroupAllowlistMarkdown(GroupAllowlistItemOutput{SourceProjectID: 42, TargetGroupID: 5})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := r.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "5") {
		t.Errorf("expected target group ID in markdown, got: %s", text)
	}
	if !strings.Contains(text, "42") {
		t.Errorf("expected source project ID in markdown, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// FormatListInboundAllowlistMarkdown — with pipe character in name
// ---------------------------------------------------------------------------.

// TestFormatListInboundAllowlistMarkdown_EscapesPipes verifies the behavior of format list inbound allowlist markdown escapes pipes.
func TestFormatListInboundAllowlistMarkdown_EscapesPipes(t *testing.T) {
	r := FormatListInboundAllowlistMarkdown(ListInboundAllowlistOutput{
		Projects: []AllowlistProjectItem{
			{ID: 10, Name: "proj|special", PathWithNamespace: "grp/proj-special", WebURL: "https://gitlab.example.com/grp/proj-special"},
		},
	})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := r.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "| proj|special |") {
		t.Errorf("pipe character in name should be escaped:\n%s", text)
	}
}

// ---------------------------------------------------------------------------
// FormatListGroupAllowlistMarkdown — with pipe character in name
// ---------------------------------------------------------------------------.

// TestFormatListGroupAllowlistMarkdown_EscapesPipes verifies the behavior of format list group allowlist markdown escapes pipes.
func TestFormatListGroupAllowlistMarkdown_EscapesPipes(t *testing.T) {
	r := FormatListGroupAllowlistMarkdown(ListGroupAllowlistOutput{
		Groups: []AllowlistGroupItem{
			{ID: 5, Name: "group|special", FullPath: "group-special", WebURL: "https://gitlab.example.com/groups/group-special"},
		},
	})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := r.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "| group|special |") {
		t.Errorf("pipe character in name should be escaped:\n%s", text)
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 8 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newJobTokenScopeMCPSession(t)
	ctx := context.Background()

	tools := []struct {
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
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newJobTokenScopeMCPSession is an internal helper for the jobtokenscope package.
func newJobTokenScopeMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	settingsJSON := `{"inbound_enabled": true}`
	projectJSON := `[{"id": 10, "name": "proj-a", "path_with_namespace": "grp/proj-a", "web_url": "https://gitlab.example.com/grp/proj-a"}]`
	addProjectJSON := `{"source_project_id": 42, "target_project_id": 99}`
	groupJSON := `[{"id": 5, "name": "group-a", "full_path": "group-a", "web_url": "https://gitlab.example.com/groups/group-a"}]`
	addGroupJSON := `{"source_project_id": 42, "target_group_id": 5}`

	handler := http.NewServeMux()

	// Get job token access settings
	handler.HandleFunc("GET /api/v4/projects/42/job_token_scope", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, settingsJSON)
	})

	// Patch job token access settings
	handler.HandleFunc("PATCH /api/v4/projects/42/job_token_scope", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// List inbound project allowlist
	handler.HandleFunc("GET /api/v4/projects/42/job_token_scope/allowlist", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, projectJSON)
	})

	// Add project to allowlist
	handler.HandleFunc("POST /api/v4/projects/42/job_token_scope/allowlist", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, addProjectJSON)
	})

	// Remove project from allowlist
	handler.HandleFunc("DELETE /api/v4/projects/42/job_token_scope/allowlist/99", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// List group allowlist
	handler.HandleFunc("GET /api/v4/projects/42/job_token_scope/groups_allowlist", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupJSON)
	})

	// Add group to allowlist
	handler.HandleFunc("POST /api/v4/projects/42/job_token_scope/groups_allowlist", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, addGroupJSON)
	})

	// Remove group from allowlist
	handler.HandleFunc("DELETE /api/v4/projects/42/job_token_scope/groups_allowlist/5", func(w http.ResponseWriter, _ *http.Request) {
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
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
