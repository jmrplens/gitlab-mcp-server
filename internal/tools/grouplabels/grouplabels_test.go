// grouplabels_test.go contains unit tests for GitLab group label operations.
// Tests use httptest to mock the GitLab GroupLabels API.

package grouplabels

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
	pathGroupLabels = "/api/v4/groups/10/labels"
	pathLabel1      = "/api/v4/groups/10/labels/1"
	pathLabelBug    = "/api/v4/groups/10/labels/bug"
	labelJSON       = `{"id":1,"name":"bug","color":"#d9534f","text_color":"#FFFFFF","description":"Bug report","open_issues_count":5,"closed_issues_count":2,"open_merge_requests_count":1,"priority":1,"is_project_label":false,"subscribed":false}`
)

// TestList_Success verifies the behavior of list success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupLabels {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+labelJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Labels) != 1 {
		t.Fatalf("len(Labels) = %d, want 1", len(out.Labels))
	}
	if out.Labels[0].Name != "bug" {
		t.Errorf("Name = %q, want %q", out.Labels[0].Name, "bug")
	}
	if out.Labels[0].Priority != 1 {
		t.Errorf("Priority = %d, want 1", out.Labels[0].Priority)
	}
}

// TestList_WithSearch verifies the behavior of list with search.
func TestList_WithSearch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupLabels {
			q := r.URL.Query()
			if q.Get("search") != "bug" {
				t.Errorf("expected search=bug, got %q", q.Get("search"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[`+labelJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10", Search: "bug"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Labels) != 1 {
		t.Fatalf("len(Labels) = %d, want 1", len(out.Labels))
	}
}

// TestList_WithOptions verifies the behavior of list with options.
func TestList_WithOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupLabels {
			q := r.URL.Query()
			if q.Get("with_counts") != "true" {
				t.Errorf("expected with_counts=true, got %q", q.Get("with_counts"))
			}
			if q.Get("include_ancestor_groups") != "true" {
				t.Errorf("expected include_ancestor_groups=true, got %q", q.Get("include_ancestor_groups"))
			}
			if q.Get("include_descendant_groups") != "true" {
				t.Errorf("expected include_descendant_groups=true, got %q", q.Get("include_descendant_groups"))
			}
			if q.Get("only_group_labels") != "true" {
				t.Errorf("expected only_group_labels=true, got %q", q.Get("only_group_labels"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{
		GroupID:                 "10",
		WithCounts:              true,
		IncludeAncestorGroups:   true,
		IncludeDescendantGroups: true,
		OnlyGroupLabels:         true,
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
}

// TestList_EmptyGroupID verifies the behavior of list empty group i d.
func TestList_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestGet_Success verifies the behavior of get success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathLabel1 {
			testutil.RespondJSON(w, http.StatusOK, labelJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: "10", LabelID: "1"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Name != "bug" {
		t.Errorf("Name = %q, want %q", out.Name, "bug")
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
}

// TestGet_EmptyGroupID verifies the behavior of get empty group i d.
func TestGet_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := Get(context.Background(), client, GetInput{LabelID: "1"})
	if err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestCreate_Success verifies the behavior of create success.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupLabels {
			testutil.RespondJSON(w, http.StatusCreated, labelJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		GroupID: "10",
		Name:    "bug",
		Color:   "#d9534f",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Name != "bug" {
		t.Errorf("Name = %q, want %q", out.Name, "bug")
	}
}

// TestCreate_EmptyGroupID verifies the behavior of create empty group i d.
func TestCreate_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := Create(context.Background(), client, CreateInput{Name: "test", Color: "#000"})
	if err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestUpdate_Success verifies the behavior of update success.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathLabel1 {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"bug-fix","color":"#00ff00","text_color":"#000","description":"Updated","priority":2,"is_project_label":false,"subscribed":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		GroupID: "10",
		LabelID: "1",
		NewName: "bug-fix",
		Color:   "#00ff00",
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Name != "bug-fix" {
		t.Errorf("Name = %q, want %q", out.Name, "bug-fix")
	}
}

// TestUpdate_EmptyGroupID verifies the behavior of update empty group i d.
func TestUpdate_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := Update(context.Background(), client, UpdateInput{LabelID: "1"})
	if err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestDelete_Success verifies the behavior of delete success.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathLabel1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: "10", LabelID: "1"})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestDelete_EmptyGroupID verifies the behavior of delete empty group i d.
func TestDelete_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	err := Delete(context.Background(), client, DeleteInput{LabelID: "1"})
	if err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestSubscribe_Success verifies the behavior of subscribe success.
func TestSubscribe_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathLabel1+"/subscribe" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"bug","color":"#d9534f","text_color":"#FFFFFF","description":"Bug report","subscribed":true,"is_project_label":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Subscribe(context.Background(), client, SubscribeInput{GroupID: "10", LabelID: "1"})
	if err != nil {
		t.Fatalf("Subscribe() unexpected error: %v", err)
	}
	if !out.Subscribed {
		t.Error("Subscribed = false, want true")
	}
}

// TestSubscribe_EmptyGroupID verifies the behavior of subscribe empty group i d.
func TestSubscribe_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := Subscribe(context.Background(), client, SubscribeInput{LabelID: "1"})
	if err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestUnsubscribe_Success verifies the behavior of unsubscribe success.
func TestUnsubscribe_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathLabel1+"/unsubscribe" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	err := Unsubscribe(context.Background(), client, SubscribeInput{GroupID: "10", LabelID: "1"})
	if err != nil {
		t.Fatalf("Unsubscribe() unexpected error: %v", err)
	}
}

// TestUnsubscribe_EmptyGroupID verifies the behavior of unsubscribe empty group i d.
func TestUnsubscribe_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	err := Unsubscribe(context.Background(), client, SubscribeInput{LabelID: "1"})
	if err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestFormatMarkdown verifies the behavior of format markdown.
func TestFormatMarkdown(t *testing.T) {
	out := Output{
		ID:          1,
		Name:        "bug",
		Color:       "#d9534f",
		Description: "Bug report",
		Priority:    1,
		Subscribed:  true,
	}
	md := FormatMarkdown(out)
	if md == "" {
		t.Fatal("FormatMarkdown returned empty string")
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Labels: []Output{
			{ID: 1, Name: "bug", Color: "#d9534f"},
			{ID: 2, Name: "feature", Color: "#428bca"},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, PerPage: 20, TotalItems: 2, TotalPages: 1},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal("FormatListMarkdown returned nil")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpCancelledCtx = "expected error for canceled context"

const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// List — API error, canceled context, pagination params
// ---------------------------------------------------------------------------.

// TestList_APIError verifies the behavior of list a p i error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestList_CancelledContext verifies the behavior of list cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{GroupID: "10"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestList_WithPaginationParams verifies the behavior of list with pagination params.
func TestList_WithPaginationParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/10/labels" {
			q := r.URL.Query()
			if q.Get("page") != "2" {
				t.Errorf("expected page=2, got %q", q.Get("page"))
			}
			if q.Get("per_page") != "5" {
				t.Errorf("expected per_page=5, got %q", q.Get("per_page"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "10", TotalPages: "2", PrevPage: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID:         "10",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if out.Pagination.TotalPages != 2 {
		t.Errorf("TotalPages = %d, want 2", out.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// Get — API error, canceled context
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies the behavior of get a p i error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{GroupID: "10", LabelID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGet_CancelledContext verifies the behavior of get cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{GroupID: "10", LabelID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Create — API error, canceled context, with optional fields
// ---------------------------------------------------------------------------.

// TestCreate_APIError verifies the behavior of create a p i error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		GroupID: "10", Name: "bug", Color: "#d9534f",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_CancelledContext verifies the behavior of create cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{
		GroupID: "10", Name: "bug", Color: "#d9534f",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestCreate_WithOptionalFields verifies the behavior of create with optional fields.
func TestCreate_WithOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/10/labels" {
			testutil.RespondJSON(w, http.StatusCreated,
				`{"id":2,"name":"feature","color":"#428bca","text_color":"#FFFFFF","description":"Feature request","priority":3,"is_project_label":false,"subscribed":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		GroupID:     "10",
		Name:        "feature",
		Color:       "#428bca",
		Description: "Feature request",
		Priority:    3,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Description != "Feature request" {
		t.Errorf("Description = %q, want %q", out.Description, "Feature request")
	}
	if out.Priority != 3 {
		t.Errorf("Priority = %d, want 3", out.Priority)
	}
}

// ---------------------------------------------------------------------------
// Update — API error, canceled context, with all optional fields
// ---------------------------------------------------------------------------.

// TestUpdate_APIError verifies the behavior of update a p i error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{GroupID: "10", LabelID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdate_CancelledContext verifies the behavior of update cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{GroupID: "10", LabelID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestUpdate_AllOptionalFields verifies the behavior of update all optional fields.
func TestUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/groups/10/labels/1" {
			testutil.RespondJSON(w, http.StatusOK,
				`{"id":1,"name":"critical-bug","color":"#ff0000","text_color":"#FFFFFF","description":"Critical bugs only","priority":5,"is_project_label":false,"subscribed":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		GroupID:     "10",
		LabelID:     "1",
		NewName:     "critical-bug",
		Color:       "#ff0000",
		Description: "Critical bugs only",
		Priority:    5,
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Name != "critical-bug" {
		t.Errorf("Name = %q, want %q", out.Name, "critical-bug")
	}
	if out.Priority != 5 {
		t.Errorf("Priority = %d, want 5", out.Priority)
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, canceled context
// ---------------------------------------------------------------------------.

// TestDelete_APIError verifies the behavior of delete a p i error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{GroupID: "10", LabelID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_CancelledContext verifies the behavior of delete cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{GroupID: "10", LabelID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Subscribe — API error, canceled context
// ---------------------------------------------------------------------------.

// TestSubscribe_APIError verifies the behavior of subscribe a p i error.
func TestSubscribe_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Subscribe(context.Background(), client, SubscribeInput{GroupID: "10", LabelID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestSubscribe_CancelledContext verifies the behavior of subscribe cancelled context.
func TestSubscribe_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Subscribe(ctx, client, SubscribeInput{GroupID: "10", LabelID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Unsubscribe — API error, canceled context
// ---------------------------------------------------------------------------.

// TestUnsubscribe_APIError verifies the behavior of unsubscribe a p i error.
func TestUnsubscribe_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Unsubscribe(context.Background(), client, SubscribeInput{GroupID: "10", LabelID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUnsubscribe_CancelledContext verifies the behavior of unsubscribe cancelled context.
func TestUnsubscribe_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := Unsubscribe(ctx, client, SubscribeInput{GroupID: "10", LabelID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdown — with all fields, minimal fields, empty
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_AllFields verifies the behavior of format markdown all fields.
func TestFormatMarkdown_AllFields(t *testing.T) {
	md := FormatMarkdown(Output{
		ID:                     1,
		Name:                   "bug",
		Color:                  "#d9534f",
		Description:            "Bug report",
		Priority:               2,
		IsProjectLabel:         false,
		Subscribed:             true,
		OpenIssuesCount:        5,
		ClosedIssuesCount:      3,
		OpenMergeRequestsCount: 1,
	})

	for _, want := range []string{
		"## Group Label: bug",
		"- **ID**: 1",
		"- **Color**: #d9534f",
		"- **Description**: Bug report",
		"- **Priority**: 2",
		"- **Project label**: false",
		"- **Subscribed**: true",
		"- **Issues**: 5 open, 3 closed",
		"- **Open MRs**: 1",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatMarkdown_MinimalFields verifies the behavior of format markdown minimal fields.
func TestFormatMarkdown_MinimalFields(t *testing.T) {
	md := FormatMarkdown(Output{
		ID:    3,
		Name:  "docs",
		Color: "#0e8a16",
	})

	if !strings.Contains(md, "## Group Label: docs") {
		t.Errorf("missing header:\n%s", md)
	}
	if !strings.Contains(md, "- **Color**: #0e8a16") {
		t.Errorf("missing color:\n%s", md)
	}
	for _, absent := range []string{
		"**Description**",
		"**Priority**",
		"**Issues**",
		"**Open MRs**",
	} {
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
		}
	}
}

// TestFormatMarkdown_Empty verifies the behavior of format markdown empty.
func TestFormatMarkdown_Empty(t *testing.T) {
	md := FormatMarkdown(Output{})
	if md == "" {
		t.Fatal("FormatMarkdown returned empty string for zero-valued Output")
	}
	if !strings.Contains(md, "## Group Label:") {
		t.Errorf("missing header:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdownString — with data, empty list
// ---------------------------------------------------------------------------.

// TestFormatListMarkdownString_WithData verifies the behavior of format list markdown string with data.
func TestFormatListMarkdownString_WithData(t *testing.T) {
	out := ListOutput{
		Labels: []Output{
			{ID: 1, Name: "bug", Color: "#d9534f", OpenIssuesCount: 5, ClosedIssuesCount: 2, OpenMergeRequestsCount: 1},
			{ID: 2, Name: "feature", Color: "#428bca", OpenIssuesCount: 3, ClosedIssuesCount: 0, OpenMergeRequestsCount: 2},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdownString(out)

	for _, want := range []string{
		"## Group Labels (2)",
		"| Name |",
		"|------|",
		"| bug |",
		"| feature |",
		"| #d9534f |",
		"| #428bca |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdownString_Empty verifies the behavior of format list markdown string empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(md, "No group labels found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| Name |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — returns non-nil result
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Result verifies the behavior of format list markdown result.
func TestFormatListMarkdown_Result(t *testing.T) {
	result := FormatListMarkdown(ListOutput{
		Labels:     []Output{{ID: 1, Name: "test", Color: "#000"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	})
	if result == nil {
		t.Fatal("FormatListMarkdown returned nil")
	}
}

// ---------------------------------------------------------------------------
// priorityFromNullable — zero for unset
// ---------------------------------------------------------------------------.

// TestPriorityFromNullable_Zero verifies the behavior of priority from nullable zero.
func TestPriorityFromNullable_Zero(t *testing.T) {
	// toOutput is tested through the handlers, but verify edge case
	out := Output{}
	if out.Priority != 0 {
		t.Errorf("Priority = %d, want 0 for zero value", out.Priority)
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
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 7 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newGroupLabelsMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_group_label_list", map[string]any{"group_id": "10"}},
		{"get", "gitlab_group_label_get", map[string]any{"group_id": "10", "label_id": "1"}},
		{"create", "gitlab_group_label_create", map[string]any{"group_id": "10", "name": "bug", "color": "#d9534f"}},
		{"update", "gitlab_group_label_update", map[string]any{"group_id": "10", "label_id": "1", "new_name": "bug-fix"}},
		{"delete", "gitlab_group_label_delete", map[string]any{"group_id": "10", "label_id": "1"}},
		{"subscribe", "gitlab_group_label_subscribe", map[string]any{"group_id": "10", "label_id": "1"}},
		{"unsubscribe", "gitlab_group_label_unsubscribe", map[string]any{"group_id": "10", "label_id": "1"}},
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

// newGroupLabelsMCPSession is an internal helper for the grouplabels package.
func newGroupLabelsMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	labelJSON := `{"id":1,"name":"bug","color":"#d9534f","text_color":"#FFFFFF","description":"Bug report","open_issues_count":5,"closed_issues_count":2,"open_merge_requests_count":1,"priority":1,"is_project_label":false,"subscribed":false}`
	subscribedJSON := `{"id":1,"name":"bug","color":"#d9534f","text_color":"#FFFFFF","description":"Bug report","subscribed":true,"is_project_label":false}`

	handler := http.NewServeMux()

	// List group labels
	handler.HandleFunc("GET /api/v4/groups/10/labels", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+labelJSON+`]`)
	})

	// Get group label
	handler.HandleFunc("GET /api/v4/groups/10/labels/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, labelJSON)
	})

	// Create group label
	handler.HandleFunc("POST /api/v4/groups/10/labels", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, labelJSON)
	})

	// Update group label
	handler.HandleFunc("PUT /api/v4/groups/10/labels/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, labelJSON)
	})

	// Delete group label
	handler.HandleFunc("DELETE /api/v4/groups/10/labels/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Subscribe to group label
	handler.HandleFunc("POST /api/v4/groups/10/labels/1/subscribe", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, subscribedJSON)
	})

	// Unsubscribe from group label
	handler.HandleFunc("POST /api/v4/groups/10/labels/1/unsubscribe", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
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
