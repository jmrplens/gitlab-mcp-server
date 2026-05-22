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

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestGetAccessSettings_Success verifies GetAccessSettings when success.
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

// TestGetAccessSettings_Error verifies GetAccessSettings when error.
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

// TestPatchAccessSettings_Success verifies PatchAccessSettings when success.
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

// TestListInboundAllowlist_Success verifies ListInboundAllowlist when success.
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

// TestAddProjectAllowlist_Success verifies AddProjectAllowlist when success.
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

// TestRemoveProjectAllowlist_Success verifies RemoveProjectAllowlist when success.
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

// TestRemoveProjectAllowlist_Error verifies RemoveProjectAllowlist when error.
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

// TestListGroupAllowlist_Success verifies ListGroupAllowlist when success.
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

// TestAddGroupAllowlist_Success verifies AddGroupAllowlist when success.
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

// TestRemoveGroupAllowlist_Success verifies RemoveGroupAllowlist when success.
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

// TestAddProjectAllowlist_ZeroTargetProjectID verifies AddProjectAllowlist when zero target project ID.
func TestAddProjectAllowlist_ZeroTargetProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when TargetProjectID is 0")
	}))
	_, err := AddProjectAllowlist(t.Context(), client, AddProjectAllowlistInput{ProjectID: "42", TargetProjectID: 0})
	if err == nil {
		t.Fatal("expected error for zero TargetProjectID, got nil")
	}
}

// TestRemoveProjectAllowlist_ZeroTargetProjectID verifies RemoveProjectAllowlist when zero target project ID.
func TestRemoveProjectAllowlist_ZeroTargetProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when TargetProjectID is 0")
	}))
	err := RemoveProjectAllowlist(t.Context(), client, RemoveProjectAllowlistInput{ProjectID: "42", TargetProjectID: 0})
	if err == nil {
		t.Fatal("expected error for zero TargetProjectID, got nil")
	}
}

// TestAddGroupAllowlist_ZeroTargetGroupID verifies AddGroupAllowlist when zero target group ID.
func TestAddGroupAllowlist_ZeroTargetGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when TargetGroupID is 0")
	}))
	_, err := AddGroupAllowlist(t.Context(), client, AddGroupAllowlistInput{ProjectID: "42", TargetGroupID: 0})
	if err == nil {
		t.Fatal("expected error for zero TargetGroupID, got nil")
	}
}

// TestRemoveGroupAllowlist_ZeroTargetGroupID verifies RemoveGroupAllowlist when zero target group ID.
func TestRemoveGroupAllowlist_ZeroTargetGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when TargetGroupID is 0")
	}))
	err := RemoveGroupAllowlist(t.Context(), client, RemoveGroupAllowlistInput{ProjectID: "42", TargetGroupID: 0})
	if err == nil {
		t.Fatal("expected error for zero TargetGroupID, got nil")
	}
}

// TestFormatAccessSettingsMarkdown verifies FormatAccessSettingsMarkdown.
func TestFormatAccessSettingsMarkdown(t *testing.T) {
	r := FormatAccessSettingsMarkdown(AccessSettingsOutput{InboundEnabled: true})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatListInboundAllowlistMarkdown_Empty verifies FormatListInboundAllowlistMarkdown when empty.
func TestFormatListInboundAllowlistMarkdown_Empty(t *testing.T) {
	r := FormatListInboundAllowlistMarkdown(ListInboundAllowlistOutput{})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatListGroupAllowlistMarkdown_Empty verifies FormatListGroupAllowlistMarkdown when empty.
func TestFormatListGroupAllowlistMarkdown_Empty(t *testing.T) {
	r := FormatListGroupAllowlistMarkdown(ListGroupAllowlistOutput{})
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpNonNilResult identifies the err exp non nil result constant used by this package.
const errExpNonNilResult = "expected non-nil result"

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// GetAccessSettings — canceled context
// ---------------------------------------------------------------------------.

// TestGetAccessSettings_CancelledContext verifies GetAccessSettings when cancelled context.
func TestGetAccessSettings_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetAccessSettings(ctx, client, GetAccessSettingsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// PatchAccessSettings — API error, canceled context
// ---------------------------------------------------------------------------.

// TestPatchAccessSettings_APIError verifies PatchAccessSettings when API error.
func TestPatchAccessSettings_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := PatchAccessSettings(context.Background(), client, PatchAccessSettingsInput{ProjectID: "42", Enabled: true})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPatchAccessSettings_CancelledContext verifies PatchAccessSettings when cancelled context.
func TestPatchAccessSettings_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := PatchAccessSettings(ctx, client, PatchAccessSettingsInput{ProjectID: "42", Enabled: false})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ListInboundAllowlist — API error, canceled context, pagination
// ---------------------------------------------------------------------------.

// TestListInboundAllowlist_APIError verifies ListInboundAllowlist when API error.
func TestListInboundAllowlist_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListInboundAllowlist(context.Background(), client, ListInboundAllowlistInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListInboundAllowlist_CancelledContext verifies ListInboundAllowlist when cancelled context.
func TestListInboundAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListInboundAllowlist(ctx, client, ListInboundAllowlistInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListInboundAllowlist_WithPagination verifies ListInboundAllowlist when with pagination.
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

// TestListInboundAllowlist_Empty verifies ListInboundAllowlist when empty.
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

// TestAddProjectAllowlist_APIError verifies AddProjectAllowlist when API error.
func TestAddProjectAllowlist_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := AddProjectAllowlist(context.Background(), client, AddProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAddProjectAllowlist_CancelledContext verifies AddProjectAllowlist when cancelled context.
func TestAddProjectAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := AddProjectAllowlist(ctx, client, AddProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// RemoveProjectAllowlist — canceled context
// ---------------------------------------------------------------------------.

// TestRemoveProjectAllowlist_CancelledContext verifies RemoveProjectAllowlist when cancelled context.
func TestRemoveProjectAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := RemoveProjectAllowlist(ctx, client, RemoveProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ListGroupAllowlist — API error, canceled context, pagination, empty
// ---------------------------------------------------------------------------.

// TestListGroupAllowlist_APIError verifies ListGroupAllowlist when API error.
func TestListGroupAllowlist_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListGroupAllowlist(context.Background(), client, ListGroupAllowlistInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListGroupAllowlist_CancelledContext verifies ListGroupAllowlist when cancelled context.
func TestListGroupAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListGroupAllowlist(ctx, client, ListGroupAllowlistInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListGroupAllowlist_WithPagination verifies ListGroupAllowlist when with pagination.
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

// TestListGroupAllowlist_Empty verifies ListGroupAllowlist when empty.
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

// TestAddGroupAllowlist_APIError verifies AddGroupAllowlist when API error.
func TestAddGroupAllowlist_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := AddGroupAllowlist(context.Background(), client, AddGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAddGroupAllowlist_CancelledContext verifies AddGroupAllowlist when cancelled context.
func TestAddGroupAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := AddGroupAllowlist(ctx, client, AddGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// RemoveGroupAllowlist — API error, canceled context
// ---------------------------------------------------------------------------.

// TestRemoveGroupAllowlist_APIError verifies RemoveGroupAllowlist when API error.
func TestRemoveGroupAllowlist_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := RemoveGroupAllowlist(context.Background(), client, RemoveGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestRemoveGroupAllowlist_CancelledContext verifies RemoveGroupAllowlist when cancelled context.
func TestRemoveGroupAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := RemoveGroupAllowlist(ctx, client, RemoveGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// FormatAccessSettingsMarkdown — disabled state
// ---------------------------------------------------------------------------.

// TestFormatAccessSettingsMarkdown_Disabled verifies FormatAccessSettingsMarkdown when disabled.
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

// TestFormatAccessSettingsMarkdown_Enabled verifies FormatAccessSettingsMarkdown when enabled.
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

// TestFormatPatchResultMarkdown verifies FormatPatchResultMarkdown.
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

// TestFormatListInboundAllowlistMarkdown_WithData verifies FormatListInboundAllowlistMarkdown when with data.
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

// TestFormatAddProjectAllowlistMarkdown verifies FormatAddProjectAllowlistMarkdown.
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

// TestFormatListGroupAllowlistMarkdown_WithData verifies FormatListGroupAllowlistMarkdown when with data.
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

// TestFormatAddGroupAllowlistMarkdown verifies FormatAddGroupAllowlistMarkdown.
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

// TestFormatListInboundAllowlistMarkdown_EscapesPipes verifies FormatListInboundAllowlistMarkdown when escapes pipes.
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

// TestFormatListGroupAllowlistMarkdown_EscapesPipes verifies FormatListGroupAllowlistMarkdown when escapes pipes.
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
