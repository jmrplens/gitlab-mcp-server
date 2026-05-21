// deploymentmergerequests_test.go contains unit tests for the deployment merge request MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package deploymentmergerequests

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

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/deployments/7/merge_requests" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{
				"iid": 10,
				"title": "Add feature X",
				"state": "merged",
				"author": {"username": "dev1"},
				"source_branch": "feature-x",
				"target_branch": "main",
				"web_url": "https://gitlab.example.com/project/-/merge_requests/10",
				"merged_at": "2026-01-15T10:30:00Z"
			},
			{
				"iid": 11,
				"title": "Fix bug Y",
				"state": "merged",
				"author": {"username": "dev2"},
				"source_branch": "fix-y",
				"target_branch": "main",
				"web_url": "https://gitlab.example.com/project/-/merge_requests/11"
			}
		]`)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{
		ProjectID:    "42",
		DeploymentID: 7,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 2 {
		t.Fatalf("expected 2 merge requests, got %d", len(out.MergeRequests))
	}
	mr := out.MergeRequests[0]
	if mr.IID != 10 {
		t.Errorf("expected IID 10, got %d", mr.IID)
	}
	if mr.Title != "Add feature X" {
		t.Errorf("expected title 'Add feature X', got %q", mr.Title)
	}
	if mr.State != "merged" {
		t.Errorf("expected state 'merged', got %q", mr.State)
	}
	if mr.Author != "dev1" {
		t.Errorf("expected author 'dev1', got %q", mr.Author)
	}
	if mr.SourceBranch != "feature-x" {
		t.Errorf("expected source_branch 'feature-x', got %q", mr.SourceBranch)
	}
	if mr.MergedAt == "" {
		t.Error("expected merged_at to be set")
	}
	// Second MR has no merged_at
	if out.MergeRequests[1].MergedAt != "" {
		t.Errorf("expected empty merged_at for second MR, got %q", out.MergeRequests[1].MergedAt)
	}
}

// TestList_Empty verifies List when empty.
func TestList_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{
		ProjectID:    "42",
		DeploymentID: 7,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 0 {
		t.Fatalf("expected 0 merge requests, got %d", len(out.MergeRequests))
	}
}

// TestList_WithFilters verifies List when with filters.
func TestList_WithFilters(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != "merged" {
			t.Errorf("expected state=merged, got %q", q.Get("state"))
		}
		if q.Get("order_by") != "created_at" {
			t.Errorf("expected order_by=created_at, got %q", q.Get("order_by"))
		}
		if q.Get("sort") != "desc" {
			t.Errorf("expected sort=desc, got %q", q.Get("sort"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{
		ProjectID:    "42",
		DeploymentID: 7,
		State:        "merged",
		OrderBy:      "created_at",
		Sort:         "desc",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_InvalidDeploymentID verifies List when invalid deployment ID.
func TestList_InvalidDeploymentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := List(t.Context(), client, ListInput{ProjectID: "42", DeploymentID: 0})
	if err == nil {
		t.Fatal("expected error for zero deployment_id")
	}
	if !strings.Contains(err.Error(), "deployment_id") {
		t.Errorf("expected error to mention deployment_id, got %q", err)
	}
	_, err = List(t.Context(), client, ListInput{ProjectID: "42", DeploymentID: -1})
	if err == nil {
		t.Fatal("expected error for negative deployment_id")
	}
}

// TestList_Error verifies List when error.
func TestList_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message": "server error"}`)
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{
		ProjectID:    "42",
		DeploymentID: 7,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatListMarkdown_WithData verifies FormatListMarkdown when with data.
func TestFormatListMarkdown_WithData(t *testing.T) {
	out := ListOutput{
		MergeRequests: []MergeRequestItem{
			{IID: 1, Title: "MR One", State: "merged", Author: "dev", SourceBranch: "feat", TargetBranch: "main"},
		},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpNonNilResult identifies the err exp non nil result constant used by this package.
const errExpNonNilResult = "expected non-nil result"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// List — API error (404), canceled context, pagination, nil author
// ---------------------------------------------------------------------------.

// TestList_APIError404 verifies List when API error 404.
func TestList_APIError404(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "99", DeploymentID: 1})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if !strings.Contains(err.Error(), "list_deployment_merge_requests") {
		t.Errorf("error should contain tool name, got: %v", err)
	}
}

// TestList_CancelledContext verifies List when cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "42", DeploymentID: 7})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestList_WithPagination verifies List when with pagination.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/deployments/5/merge_requests" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"iid":20,"title":"MR Alpha","state":"merged","author":{"username":"alice"},"source_branch":"feat-a","target_branch":"main","web_url":"https://gl.example.com/mr/20"}
			]`, testutil.PaginationHeaders{
				Page: "2", PerPage: "1", Total: "3", TotalPages: "3", NextPage: "3", PrevPage: "1",
			})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "1", DeploymentID: 5, Page: 2, PerPage: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("expected 1 MR, got %d", len(out.MergeRequests))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 3 {
		t.Errorf("NextPage = %d, want 3", out.Pagination.NextPage)
	}
	if out.Pagination.PrevPage != 1 {
		t.Errorf("PrevPage = %d, want 1", out.Pagination.PrevPage)
	}
}

// TestList_NilAuthor verifies List when nil author.
func TestList_NilAuthor(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"iid":30,"title":"No Author MR","state":"opened","source_branch":"fix","target_branch":"main","web_url":"https://gl.example.com/mr/30"}
		]`)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "1", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.MergeRequests) != 1 {
		t.Fatalf("expected 1 MR, got %d", len(out.MergeRequests))
	}
	if out.MergeRequests[0].Author != "" {
		t.Errorf("expected empty author for nil author, got %q", out.MergeRequests[0].Author)
	}
}

// TestList_AllOptionalFilters verifies List when all optional filters.
func TestList_AllOptionalFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != "opened" {
			t.Errorf("expected state=opened, got %q", q.Get("state"))
		}
		if q.Get("order_by") != "updated_at" {
			t.Errorf("expected order_by=updated_at, got %q", q.Get("order_by"))
		}
		if q.Get("sort") != "asc" {
			t.Errorf("expected sort=asc, got %q", q.Get("sort"))
		}
		if q.Get("page") != "3" {
			t.Errorf("expected page=3, got %q", q.Get("page"))
		}
		if q.Get("per_page") != "50" {
			t.Errorf("expected per_page=50, got %q", q.Get("per_page"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID:    "1",
		DeploymentID: 2,
		State:        "opened",
		OrderBy:      "updated_at",
		Sort:         "asc",
		Page:         3,
		PerPage:      50,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — multiple items, special characters, pagination info
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_MultipleItems verifies FormatListMarkdown when multiple items.
func TestFormatListMarkdown_MultipleItems(t *testing.T) {
	out := ListOutput{
		MergeRequests: []MergeRequestItem{
			{IID: 10, Title: "Feature A", State: "merged", Author: "dev1", SourceBranch: "feat-a", TargetBranch: "main"},
			{IID: 11, Title: "Fix B", State: "opened", Author: "dev2", SourceBranch: "fix-b", TargetBranch: "develop"},
			{IID: 12, Title: "Hotfix C", State: "closed", Author: "dev3", SourceBranch: "hotfix-c", TargetBranch: "main"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 3, Page: 1, PerPage: 20, TotalPages: 1},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}

	text := result.Content[0].(*mcp.TextContent).Text

	for _, want := range []string{
		"## Deployment Merge Requests (3)",
		"| IID |",
		"|-----|",
		"!10",
		"!11",
		"!12",
		"Feature A",
		"Fix B",
		"Hotfix C",
		"dev1",
		"dev2",
		"dev3",
		"feat-a → main",
		"fix-b → develop",
		"hotfix-c → main",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("markdown missing %q:\n%s", want, text)
		}
	}
}

// TestFormatListMarkdown_SpecialCharacters verifies FormatListMarkdown when special characters.
func TestFormatListMarkdown_SpecialCharacters(t *testing.T) {
	out := ListOutput{
		MergeRequests: []MergeRequestItem{
			{IID: 1, Title: "Title with | pipe", State: "merged", Author: "user", SourceBranch: "src", TargetBranch: "tgt"},
		},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "| pipe |") {
		t.Error("pipe character in title should be escaped")
	}
}

// TestFormatListMarkdown_EmptyOutput verifies FormatListMarkdown when empty output.
func TestFormatListMarkdown_EmptyOutput(t *testing.T) {
	result := FormatListMarkdown(ListOutput{MergeRequests: []MergeRequestItem{}})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "No merge requests found") {
		t.Errorf("expected empty message, got:\n%s", text)
	}
	if strings.Contains(text, "| IID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListMarkdown_NilSlice verifies FormatListMarkdown when nil slice.
func TestFormatListMarkdown_NilSlice(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "No merge requests found") {
		t.Errorf("expected empty message for nil slice, got:\n%s", text)
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for deployment merge request actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	byTool := deploymentMRSpecsByTool(t, ActionSpecs(client))

	if len(byTool) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(byTool))
	}
	spec := byTool["gitlab_list_deployment_merge_requests"]
	if spec.OwnerPackage != "deploymentmergerequests" {
		t.Errorf("OwnerPackage = %q, want deploymentmergerequests", spec.OwnerPackage)
	}
	if !spec.ReadOnly || !spec.Idempotent {
		t.Error("deployment merge request list action should be read-only and idempotent")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates deployment merge request route coverage.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newDeploymentMRSpecsByTool(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_deployment_merge_requests", "gitlab_list_deployment_merge_requests", map[string]any{
			"project_id": "42", "deployment_id": 7,
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

// TestActionSpecs_CallRouteWithFilters verifies the list route accepts filters.
func TestActionSpecs_CallRouteWithFilters(t *testing.T) {
	byTool := newDeploymentMRSpecsByTool(t)

	result, err := byTool["gitlab_list_deployment_merge_requests"].Route.Handler(t.Context(), map[string]any{
		"project_id":    "42",
		"deployment_id": 7,
		"state":         "merged",
		"order_by":      "created_at",
		"sort":          "desc",
		"page":          1,
		"per_page":      10,
	})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("Route.Handler returned nil")
	}
}

// TestActionSpecs_CallRouteEmptyResult verifies the list route handles empty results.
func TestActionSpecs_CallRouteEmptyResult(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/42/deployments/7/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	client := testutil.NewTestClient(t, handler)
	byTool := deploymentMRSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_list_deployment_merge_requests"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "deployment_id": 7})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(ListOutput)
	if !ok {
		t.Fatalf("result type = %T, want ListOutput", result)
	}
	if len(out.MergeRequests) != 0 {
		t.Fatalf("len(MergeRequests) = %d, want 0", len(out.MergeRequests))
	}
}

// ---------------------------------------------------------------------------
// Helper: route factory
// ---------------------------------------------------------------------------.

// newDeploymentMRSpecsByTool constructs deployment MR specs by tool test fixtures.
func newDeploymentMRSpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/42/deployments/7/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{
				"iid": 10,
				"title": "Add feature X",
				"state": "merged",
				"author": {"username": "dev1"},
				"source_branch": "feature-x",
				"target_branch": "main",
				"web_url": "https://gitlab.example.com/project/-/merge_requests/10",
				"merged_at": "2026-01-15T10:30:00Z"
			},
			{
				"iid": 11,
				"title": "Fix bug Y",
				"state": "merged",
				"author": {"username": "dev2"},
				"source_branch": "fix-y",
				"target_branch": "main",
				"web_url": "https://gitlab.example.com/project/-/merge_requests/11"
			}
		]`)
	})

	client := testutil.NewTestClient(t, handler)
	return deploymentMRSpecsByTool(t, ActionSpecs(client))
}

// deploymentMRSpecsByTool supports deployment MR specs by tool assertions in deploymentmergerequests tests.
func deploymentMRSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
