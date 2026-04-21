// labels_test.go contains unit tests for GitLab label operations.
// Tests use httptest to mock the GitLab Labels API.
package labels

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	pathProjectLabels = "/api/v4/projects/42/labels"
	pathLabelBug      = "/api/v4/projects/42/labels/bug"
	pathLabel1        = "/api/v4/projects/42/labels/1"
	labelJSON         = `{"id":1,"name":"bug","color":"#d9534f","text_color":"#FFFFFF","description":"Bug report","open_issues_count":5,"closed_issues_count":2,"open_merge_requests_count":1,"priority":1,"is_project_label":true,"subscribed":false}`
)

// TestLabelList_Success verifies the behavior of label list success.
func TestLabelList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectLabels {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{
					"id":1,
					"name":"bug",
					"color":"#d9534f",
					"text_color":"#FFFFFF",
					"description":"Bug report",
					"open_issues_count":5,
					"closed_issues_count":2,
					"open_merge_requests_count":1,
					"priority":1,
					"is_project_label":true
				},
				{
					"id":2,
					"name":"feature",
					"color":"#428bca",
					"text_color":"#FFFFFF",
					"description":"New feature request",
					"open_issues_count":3,
					"closed_issues_count":10,
					"open_merge_requests_count":2,
					"priority":2,
					"is_project_label":true
				}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Labels) != 2 {
		t.Fatalf("len(Labels) = %d, want 2", len(out.Labels))
	}
	if out.Labels[0].Name != "bug" {
		t.Errorf("Labels[0].Name = %q, want %q", out.Labels[0].Name, "bug")
	}
	if out.Labels[1].Name != "feature" {
		t.Errorf("Labels[1].Name = %q, want %q", out.Labels[1].Name, "feature")
	}
	if out.Labels[0].OpenIssuesCount != 5 {
		t.Errorf("Labels[0].OpenIssuesCount = %d, want 5", out.Labels[0].OpenIssuesCount)
	}
}

// TestLabelList_WithSearch verifies the behavior of label list with search.
func TestLabelList_WithSearch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectLabels {
			q := r.URL.Query()
			if q.Get("search") != "bug" {
				t.Errorf("expected search=bug, got %q", q.Get("search"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"bug","color":"#d9534f","is_project_label":true}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		Search:    "bug",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Labels) != 1 {
		t.Fatalf("len(Labels) = %d, want 1", len(out.Labels))
	}
}

// TestLabelList_EmptyProjectID verifies the behavior of label list empty project i d.
func TestLabelList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for empty project_id, got nil")
	}
}

// TestLabelListServer_Error verifies the behavior of label list server error.
func TestLabelListServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Internal Server Error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
	})
	if err == nil {
		t.Fatal("List() expected error, got nil")
	}
}

// TestLabelList_CancelledContext verifies the behavior of label list cancelled context.
func TestLabelList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("List() expected error for canceled context, got nil")
	}
}

// TestLabelGet_Success verifies the behavior of label get success.
func TestLabelGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathLabelBug {
			testutil.RespondJSON(w, http.StatusOK, labelJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", LabelID: "bug"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
	if out.Name != "bug" {
		t.Errorf("out.Name = %q, want %q", out.Name, "bug")
	}
}

// TestLabelGet_NotFound verifies the behavior of label get not found.
func TestLabelGet_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", LabelID: "nope"})
	if err == nil {
		t.Fatal("Get() expected error, got nil")
	}
}

// TestLabelCreate_Success verifies the behavior of label create success.
func TestLabelCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjectLabels {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"name":"enhancement","color":"#00FF00","description":"Enhancement","is_project_label":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   "42",
		Name:        "enhancement",
		Color:       "#00FF00",
		Description: "Enhancement",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Name != "enhancement" {
		t.Errorf("out.Name = %q, want %q", out.Name, "enhancement")
	}
	if out.Color != "#00FF00" {
		t.Errorf("out.Color = %q, want %q", out.Color, "#00FF00")
	}
}

// TestLabelCreate_MissingProject verifies the behavior of label create missing project.
func TestLabelCreate_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{Name: "test", Color: "#000"})
	if err == nil {
		t.Fatal("Create() expected error for empty project_id, got nil")
	}
}

// TestLabelUpdate_Success verifies the behavior of label update success.
func TestLabelUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathLabelBug {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"critical-bug","color":"#FF0000","description":"Critical","is_project_label":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42",
		LabelID:   "bug",
		NewName:   "critical-bug",
		Color:     "#FF0000",
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Name != "critical-bug" {
		t.Errorf("out.Name = %q, want %q", out.Name, "critical-bug")
	}
}

// TestLabelUpdate_NotFound verifies the behavior of label update not found.
func TestLabelUpdate_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", LabelID: "nope", NewName: "x"})
	if err == nil {
		t.Fatal("Update() expected error, got nil")
	}
}

// TestLabelDelete_Success verifies the behavior of label delete success.
func TestLabelDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathLabelBug {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", LabelID: "bug"})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestLabelDelete_NotFound verifies the behavior of label delete not found.
func TestLabelDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", LabelID: "nope"})
	if err == nil {
		t.Fatal("Delete() expected error, got nil")
	}
}

// TestLabelSubscribe_Success verifies the behavior of label subscribe success.
func TestLabelSubscribe_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/labels/1/subscribe" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"bug","color":"#d9534f","subscribed":true,"is_project_label":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Subscribe(context.Background(), client, SubscribeInput{ProjectID: "42", LabelID: "1"})
	if err != nil {
		t.Fatalf("Subscribe() unexpected error: %v", err)
	}
	if !out.Subscribed {
		t.Error("out.Subscribed = false, want true")
	}
}

// TestLabelSubscribe_Error verifies the behavior of label subscribe error.
func TestLabelSubscribe_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Subscribe(context.Background(), client, SubscribeInput{ProjectID: "42", LabelID: "999"})
	if err == nil {
		t.Fatal("Subscribe() expected error, got nil")
	}
}

// TestLabelUnsubscribe_Success verifies the behavior of label unsubscribe success.
func TestLabelUnsubscribe_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/labels/1/unsubscribe" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Unsubscribe(context.Background(), client, SubscribeInput{ProjectID: "42", LabelID: "1"})
	if err != nil {
		t.Fatalf("Unsubscribe() unexpected error: %v", err)
	}
}

// TestLabelUnsubscribe_Error verifies the behavior of label unsubscribe error.
func TestLabelUnsubscribe_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	err := Unsubscribe(context.Background(), client, SubscribeInput{ProjectID: "42", LabelID: "999"})
	if err == nil {
		t.Fatal("Unsubscribe() expected error, got nil")
	}
}

// TestLabelPromote_Success verifies the behavior of label promote success.
func TestLabelPromote_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/labels/1/promote" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	err := Promote(context.Background(), client, PromoteInput{ProjectID: "42", LabelID: "1"})
	if err != nil {
		t.Fatalf("Promote() unexpected error: %v", err)
	}
}

// TestLabelPromote_Error verifies the behavior of label promote error.
func TestLabelPromote_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	err := Promote(context.Background(), client, PromoteInput{ProjectID: "42", LabelID: "999"})
	if err == nil {
		t.Fatal("Promote() expected error, got nil")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// JSON fixtures
// ---------------------------------------------------------------------------.

const (
	errProjectIDRequired = "project_id is required"
	errExpectedErr       = "expected error"
	errExpCancelledCtx   = "expected error for canceled context"
	fmtUnexpErr          = "unexpected error: %v"
	covLabelJSON         = `{"id":1,"name":"bug","color":"#d9534f","text_color":"#FFFFFF","description":"Bug report","open_issues_count":5,"closed_issues_count":2,"open_merge_requests_count":1,"priority":1,"is_project_label":true,"subscribed":false}`
	covLabelMinimalJSON  = `{"id":2,"name":"wontfix","color":"#000000","text_color":"#FFFFFF","is_project_label":true}`
	covLabelListJSON     = `[` + covLabelJSON + `]`
	covLabelWithPriJSON  = `{"id":3,"name":"critical","color":"#FF0000","text_color":"#000","description":"Critical","priority":5,"is_project_label":true}`
)

// ---------------------------------------------------------------------------
// List — additional coverage
// ---------------------------------------------------------------------------.

// TestList_WithCounts verifies the behavior of cov list with counts.
func TestList_WithCounts(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("with_counts") != "true" {
			t.Errorf("expected with_counts=true, got %q", r.URL.Query().Get("with_counts"))
		}
		testutil.RespondJSON(w, http.StatusOK, covLabelListJSON)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", WithCounts: true})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_IncludeAncestorGroups verifies the behavior of cov list include ancestor groups.
func TestList_IncludeAncestorGroups(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("include_ancestor_groups") != "true" {
			t.Errorf("expected include_ancestor_groups=true, got %q", r.URL.Query().Get("include_ancestor_groups"))
		}
		testutil.RespondJSON(w, http.StatusOK, covLabelListJSON)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", IncludeAncestorGroups: true})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_WithPagination verifies the behavior of cov list with pagination.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("expected page=2, got %q", r.URL.Query().Get("page"))
		}
		if r.URL.Query().Get("per_page") != "5" {
			t.Errorf("expected per_page=5, got %q", r.URL.Query().Get("per_page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, covLabelListJSON,
			testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "10", TotalPages: "2"})
	}))
	out, err := List(context.Background(), client, ListInput{
		ProjectID:       "42",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("expected page 2, got %d", out.Pagination.Page)
	}
}

// ---------------------------------------------------------------------------
// Get — additional coverage
// ---------------------------------------------------------------------------.

// TestGet_CancelledContext verifies the behavior of cov get cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "42", LabelID: "bug"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestGet_MissingProjectID verifies the behavior of cov get missing project i d.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := Get(context.Background(), client, GetInput{LabelID: "bug"})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf("expected project_id required error, got %v", err)
	}
}

// TestGet_ServerError verifies the behavior of cov get server error.
func TestGet_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", LabelID: "bug"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// ---------------------------------------------------------------------------
// Create — additional coverage
// ---------------------------------------------------------------------------.

// TestCreate_WithPriority verifies the behavior of cov create with priority.
func TestCreate_WithPriority(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, covLabelWithPriJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42", Name: "critical", Color: "#FF0000", Description: "Critical", Priority: 5,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Priority != 5 {
		t.Errorf("expected priority 5, got %d", out.Priority)
	}
}

// TestCreate_ServerError verifies the behavior of cov create server error.
func TestCreate_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Name: "x", Color: "#000"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestCreate_CancelledContext verifies the behavior of cov create cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", Name: "x", Color: "#000"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Update — additional coverage
// ---------------------------------------------------------------------------.

// TestUpdate_WithDescAndPriority verifies the behavior of cov update with desc and priority.
func TestUpdate_WithDescAndPriority(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, covLabelWithPriJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42", LabelID: "bug", Description: "Critical", Priority: 5,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Priority != 5 {
		t.Errorf("expected priority 5, got %d", out.Priority)
	}
}

// TestUpdate_MissingProjectID verifies the behavior of cov update missing project i d.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := Update(context.Background(), client, UpdateInput{LabelID: "bug", NewName: "x"})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf("expected project_id required, got %v", err)
	}
}

// TestUpdate_ServerError verifies the behavior of cov update server error.
func TestUpdate_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", LabelID: "bug", NewName: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdate_CancelledContext verifies the behavior of cov update cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", LabelID: "bug", NewName: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Delete — additional coverage
// ---------------------------------------------------------------------------.

// TestDelete_MissingProjectID verifies the behavior of cov delete missing project i d.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	err := Delete(context.Background(), client, DeleteInput{LabelID: "bug"})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf("expected project_id required, got %v", err)
	}
}

// TestDelete_ServerError verifies the behavior of cov delete server error.
func TestDelete_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", LabelID: "bug"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDelete_CancelledContext verifies the behavior of cov delete cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: "42", LabelID: "bug"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Subscribe — additional coverage
// ---------------------------------------------------------------------------.

// TestSubscribe_MissingProjectID verifies the behavior of cov subscribe missing project i d.
func TestSubscribe_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := Subscribe(context.Background(), client, SubscribeInput{LabelID: "1"})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf("expected project_id required, got %v", err)
	}
}

// TestSubscribe_CancelledContext verifies the behavior of cov subscribe cancelled context.
func TestSubscribe_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Subscribe(ctx, client, SubscribeInput{ProjectID: "42", LabelID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Unsubscribe — additional coverage
// ---------------------------------------------------------------------------.

// TestUnsubscribe_MissingProjectID verifies the behavior of cov unsubscribe missing project i d.
func TestUnsubscribe_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	err := Unsubscribe(context.Background(), client, SubscribeInput{LabelID: "1"})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf("expected project_id required, got %v", err)
	}
}

// TestUnsubscribe_CancelledContext verifies the behavior of cov unsubscribe cancelled context.
func TestUnsubscribe_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Unsubscribe(ctx, client, SubscribeInput{ProjectID: "42", LabelID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Promote — additional coverage
// ---------------------------------------------------------------------------.

// TestPromote_MissingProjectID verifies the behavior of cov promote missing project i d.
func TestPromote_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	err := Promote(context.Background(), client, PromoteInput{LabelID: "1"})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf("expected project_id required, got %v", err)
	}
}

// TestPromote_CancelledContext verifies the behavior of cov promote cancelled context.
func TestPromote_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Promote(ctx, client, PromoteInput{ProjectID: "42", LabelID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Formatters — additional coverage
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_AllFields verifies the behavior of cov format markdown all fields.
func TestFormatMarkdown_AllFields(t *testing.T) {
	o := Output{
		ID: 1, Name: "bug", Color: "#d9534f", Description: "Bug report",
		Priority: 3, IsProjectLabel: true, Subscribed: true,
		OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1,
	}
	md := FormatMarkdown(o)
	for _, want := range []string{"bug", "#d9534f", "Bug report", "Priority", "3", "Issues", "5 open", "2 closed", "Open MRs", "1"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatMarkdown missing %q in:\n%s", want, md)
		}
	}
}

// TestFormatMarkdown_Minimal verifies the behavior of cov format markdown minimal.
func TestFormatMarkdown_Minimal(t *testing.T) {
	o := Output{ID: 2, Name: "wontfix", Color: "#000"}
	md := FormatMarkdown(o)
	if strings.Contains(md, "Priority") {
		t.Error("minimal label should not show Priority")
	}
	if strings.Contains(md, "Issues") {
		t.Error("minimal label should not show Issues section")
	}
	if !strings.Contains(md, "wontfix") {
		t.Error("missing label name")
	}
}

// TestFormatListMarkdownString_Empty verifies the behavior of cov format list markdown string empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(md, "No labels found") {
		t.Errorf("expected 'No labels found', got:\n%s", md)
	}
}

// TestFormatListMarkdownString_WithLabels verifies the behavior of cov format list markdown string with labels.
func TestFormatListMarkdownString_WithLabels(t *testing.T) {
	out := ListOutput{
		Labels: []Output{
			{ID: 1, Name: "bug", Color: "#d9534f", OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	md := FormatListMarkdownString(out)
	if !strings.Contains(md, "bug") {
		t.Errorf("missing label in table:\n%s", md)
	}
	if !strings.Contains(md, "| Name |") {
		t.Errorf("missing table header:\n%s", md)
	}
}

// TestFormatListMarkdown verifies the behavior of cov format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Labels:     []Output{{ID: 1, Name: "test", Color: "#000"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal("result is nil")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic + MCP round-trip
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallAllThroughMCP validates cov register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	const labelPath = "/api/v4/projects/42/labels"

	mux := http.NewServeMux()
	mux.HandleFunc(labelPath, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covLabelJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, covLabelJSON)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc(labelPath+"/bug", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, covLabelJSON)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, covLabelJSON)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc(labelPath+"/1/subscribe", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covLabelJSON)
	})
	mux.HandleFunc(labelPath+"/1/unsubscribe", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc(labelPath+"/1/promote", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_label_list", map[string]any{"project_id": "42"}},
		{"gitlab_label_get", map[string]any{"project_id": "42", "label_id": "bug"}},
		{"gitlab_label_create", map[string]any{"project_id": "42", "name": "test", "color": "#000"}},
		{"gitlab_label_update", map[string]any{"project_id": "42", "label_id": "bug", "new_name": "updated"}},
		{"gitlab_label_delete", map[string]any{"project_id": "42", "label_id": "bug"}},
		{"gitlab_label_subscribe", map[string]any{"project_id": "42", "label_id": "1"}},
		{"gitlab_label_unsubscribe", map[string]any{"project_id": "42", "label_id": "1"}},
		{"gitlab_label_promote", map[string]any{"project_id": "42", "label_id": "1"}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool(%s): %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s): nil result", tc.name)
			}
		})
	}
}

// TestMCPRoundTrip_GetNotFound covers the 404 NotFoundResult path in
// gitlab_label_get when the label does not exist.
func TestMCPRoundTrip_GetNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Label Not Found"}`)
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
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_label_get",
		Arguments: map[string]any{"project_id": "42", "label_id": "nonexist"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError result for 404")
	}
}

// TestMCPRoundTrip_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in gitlab_label_delete when user declines.
func TestMCPRoundTrip_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
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
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_label_delete",
		Arguments: map[string]any{"project_id": "42", "label_id": "bug"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestMCPRoundTrip_DeleteError covers the delete error path through register.go.
func TestMCPRoundTrip_DeleteError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
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
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_label_delete",
		Arguments: map[string]any{"project_id": "42", "label_id": "bug"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for 500 backend")
	}
}

// TestCreate_ConflictError covers the 409 Conflict branch in Create.
func TestCreate_ConflictError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"Label already exists"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Name: "bug", Color: "#f00"})
	if err == nil {
		t.Fatal("expected error for 409")
	}
}

// TestCreate_BadRequestError covers the 400 BadRequest branch in Create.
func TestCreate_BadRequestError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad Request"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Name: "bug", Color: "invalid"})
	if err == nil {
		t.Fatal("expected error for 400")
	}
}

// TestPromote_ForbiddenError covers the 403 Forbidden branch in Promote.
func TestPromote_ForbiddenError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	err := Promote(context.Background(), client, PromoteInput{ProjectID: "42", LabelID: "1"})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}
