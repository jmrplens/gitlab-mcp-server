// epics_extra_test.go extends test coverage for the epics package.
// Covers: List filters/pagination, Create/Update with all optional fields,
// GetLinks validation/error paths, Delete missing group_id, context cancellation
// for all functions, toOutput edge cases, and all three Markdown formatters.
package epics

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// epicClosedJSON represents an epic with ClosedAt set for toOutput edge coverage.
const epicClosedJSON = `{
	"id": 102,
	"iid": 2,
	"group_id": 5,
	"parent_id": 10,
	"title": "Closed Epic",
	"description": "",
	"state": "closed",
	"confidential": true,
	"web_url": "https://gitlab.example.com/groups/mygroup/-/epics/2",
	"author": null,
	"labels": [],
	"created_at": "2026-01-01T00:00:00Z",
	"updated_at": "2026-02-01T00:00:00Z",
	"closed_at": "2026-03-01T00:00:00Z",
	"upvotes": 0,
	"downvotes": 1,
	"user_notes_count": 0
}`

// epicMinimalJSON represents an epic with most optional fields absent.
const epicMinimalJSON = `{
	"id": 103,
	"iid": 3,
	"group_id": 5,
	"title": "Minimal Epic",
	"state": "opened",
	"confidential": false,
	"web_url": "",
	"labels": []
}`

// --- List: filter params, pagination, API error ---

// TestList_WithAllFilters verifies that List passes every filter parameter
// to the GitLab API and correctly returns paginated results.
func TestList_WithAllFilters(t *testing.T) {
	authorID := int64(42)
	boolTrue := true
	boolFalse := false

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, pathEpics)

		testutil.AssertQueryParam(t, r, "author_id", "42")
		testutil.AssertQueryParam(t, r, "labels", "bug,feature")
		testutil.AssertQueryParam(t, r, "order_by", "updated_at")
		testutil.AssertQueryParam(t, r, "sort", "desc")
		testutil.AssertQueryParam(t, r, "search", "planning")
		testutil.AssertQueryParam(t, r, "state", "opened")
		testutil.AssertQueryParam(t, r, "include_ancestor_groups", "true")
		testutil.AssertQueryParam(t, r, "include_descendant_groups", "false")
		testutil.AssertQueryParam(t, r, "page", "2")
		testutil.AssertQueryParam(t, r, "per_page", "10")

		testutil.RespondJSONWithPagination(w, http.StatusOK, "["+epicJSON+"]", testutil.PaginationHeaders{
			Page:       "2",
			PerPage:    "10",
			Total:      "25",
			TotalPages: "3",
			NextPage:   "3",
			PrevPage:   "1",
		})
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID:                 testGroupID,
		AuthorID:                &authorID,
		Labels:                  "bug,feature",
		OrderBy:                 "updated_at",
		Sort:                    "desc",
		Search:                  "planning",
		State:                   "opened",
		IncludeAncestorGroups:   &boolTrue,
		IncludeDescendantGroups: &boolFalse,
		PaginationInput:         toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Epics) != 1 {
		t.Fatalf("len(Epics) = %d, want 1", len(out.Epics))
	}
	if out.Pagination.TotalItems != 25 {
		t.Errorf("TotalItems = %d, want 25", out.Pagination.TotalItems)
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 3 {
		t.Errorf("NextPage = %d, want 3", out.Pagination.NextPage)
	}
}

// TestList_APIError verifies that List wraps and returns GitLab API errors.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{GroupID: testGroupID})
	if err == nil {
		t.Fatal("List() expected error for 500, got nil")
	}
}

// TestList_EmptyResult verifies that List returns an empty slice for no epics.
func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: testGroupID})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Epics) != 0 {
		t.Errorf("len(Epics) = %d, want 0", len(out.Epics))
	}
}

// --- Get: missing group_id, cancelled context ---

// TestGet_MissingGroupID verifies that Get returns an error when group_id is empty.
func TestGet_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{EpicIID: 1})
	if err == nil {
		t.Fatal("Get() expected error for missing group_id, got nil")
	}
}

// TestGet_CancelledContext verifies that Get returns an error when context is cancelled.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Get(ctx, client, GetInput{GroupID: testGroupID, EpicIID: 1})
	if err == nil {
		t.Fatal("Get() expected context error, got nil")
	}
}

// --- GetLinks: missing group_id, missing epic_iid, API error, cancelled context ---

// TestGetLinks_Validation verifies GetLinks input validation for missing fields.
func TestGetLinks_Validation(t *testing.T) {
	tests := []struct {
		name  string
		input GetLinksInput
	}{
		{name: "missing group_id", input: GetLinksInput{EpicIID: 1}},
		{name: "missing epic_iid", input: GetLinksInput{GroupID: testGroupID}},
	}
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetLinks(context.Background(), client, tt.input)
			if err == nil {
				t.Fatalf("GetLinks() expected error for %s, got nil", tt.name)
			}
		})
	}
}

// TestGetLinks_APIError verifies GetLinks returns wrapped errors on API failure.
func TestGetLinks_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := GetLinks(context.Background(), client, GetLinksInput{GroupID: testGroupID, EpicIID: 1})
	if err == nil {
		t.Fatal("GetLinks() expected error for 403, got nil")
	}
}

// TestGetLinks_CancelledContext verifies GetLinks returns an error on cancelled context.
func TestGetLinks_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetLinks(ctx, client, GetLinksInput{GroupID: testGroupID, EpicIID: 1})
	if err == nil {
		t.Fatal("GetLinks() expected context error, got nil")
	}
}

// TestGetLinks_EmptyResult verifies GetLinks returns an empty slice when no children.
func TestGetLinks_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/mygroup/epics/1/epics" {
			testutil.RespondJSON(w, http.StatusOK, "[]")
			return
		}
		http.NotFound(w, r)
	}))
	out, err := GetLinks(context.Background(), client, GetLinksInput{GroupID: testGroupID, EpicIID: 1})
	if err != nil {
		t.Fatalf("GetLinks() error: %v", err)
	}
	if len(out.ChildEpics) != 0 {
		t.Errorf("len(ChildEpics) = %d, want 0", len(out.ChildEpics))
	}
}

// --- Create: all optional fields, missing group_id, API error, cancelled context ---

// TestCreate_AllOptionalFields verifies Create passes all optional fields to the API.
func TestCreate_AllOptionalFields(t *testing.T) {
	confidential := true
	parentID := int64(99)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, pathEpics)
		testutil.RespondJSON(w, http.StatusCreated, epicJSON)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		GroupID:      testGroupID,
		Title:        "Full Epic",
		Description:  "A **detailed** description",
		Labels:       "bug, feature",
		Confidential: &confidential,
		ParentID:     &parentID,
		Color:        "#FF0000",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if out.ID != 101 {
		t.Errorf("out.ID = %d, want 101", out.ID)
	}
}

// TestCreate_MissingGroupID verifies Create returns an error when group_id is empty.
func TestCreate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{Title: "Test"})
	if err == nil {
		t.Fatal("Create() expected error for missing group_id, got nil")
	}
}

// TestCreate_APIError verifies Create wraps and returns GitLab API errors.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{GroupID: testGroupID, Title: "Test"})
	if err == nil {
		t.Fatal("Create() expected error for 422, got nil")
	}
}

// TestCreate_CancelledContext verifies Create returns an error on cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Create(ctx, client, CreateInput{GroupID: testGroupID, Title: "Test"})
	if err == nil {
		t.Fatal("Create() expected context error, got nil")
	}
}

// --- Update: all optional fields, missing group_id, API error, cancelled context ---

// TestUpdate_AllOptionalFields verifies Update passes all optional fields to the API.
func TestUpdate_AllOptionalFields(t *testing.T) {
	confidential := false
	parentID := int64(50)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPut)
		testutil.AssertRequestPath(t, r, pathEpicByID)
		testutil.RespondJSON(w, http.StatusOK, epicJSON)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		GroupID:      testGroupID,
		EpicIID:      1,
		Title:        "Updated",
		Description:  "New **desc**",
		Labels:       "label1",
		AddLabels:    "label2",
		RemoveLabels: "label3",
		StateEvent:   "close",
		Confidential: &confidential,
		ParentID:     &parentID,
		Color:        "#00FF00",
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if out.ID != 101 {
		t.Errorf("out.ID = %d, want 101", out.ID)
	}
}

// TestUpdate_MissingGroupID verifies Update returns an error when group_id is empty.
func TestUpdate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{EpicIID: 1})
	if err == nil {
		t.Fatal("Update() expected error for missing group_id, got nil")
	}
}

// TestUpdate_APIError verifies Update wraps and returns GitLab API errors.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{GroupID: testGroupID, EpicIID: 1, Title: "X"})
	if err == nil {
		t.Fatal("Update() expected error for 404, got nil")
	}
}

// TestUpdate_CancelledContext verifies Update returns an error on cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Update(ctx, client, UpdateInput{GroupID: testGroupID, EpicIID: 1})
	if err == nil {
		t.Fatal("Update() expected context error, got nil")
	}
}

// --- Delete: missing group_id, cancelled context ---

// TestDelete_MissingGroupID verifies Delete returns an error when group_id is empty.
func TestDelete_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{EpicIID: 1})
	if err == nil {
		t.Fatal("Delete() expected error for missing group_id, got nil")
	}
}

// TestDelete_CancelledContext verifies Delete returns an error on cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Delete(ctx, client, DeleteInput{GroupID: testGroupID, EpicIID: 1})
	if err == nil {
		t.Fatal("Delete() expected context error, got nil")
	}
}

// --- toOutput: edge cases (nil author, ClosedAt set, no dates) ---

// TestToOutput_ClosedEpic verifies toOutput correctly formats a closed epic
// with ClosedAt timestamp and nil author.
func TestToOutput_ClosedEpic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/mygroup/epics/2" {
			testutil.RespondJSON(w, http.StatusOK, epicClosedJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: testGroupID, EpicIID: 2})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.ID != 102 {
		t.Errorf("out.ID = %d, want 102", out.ID)
	}
	if out.State != "closed" {
		t.Errorf("out.State = %q, want %q", out.State, "closed")
	}
	if out.Confidential != true {
		t.Error("out.Confidential = false, want true")
	}
	if out.Author != "" {
		t.Errorf("out.Author = %q, want empty (nil author)", out.Author)
	}
	if out.ClosedAt == "" {
		t.Error("out.ClosedAt is empty, want non-empty timestamp")
	}
	if out.ParentID != 10 {
		t.Errorf("out.ParentID = %d, want 10", out.ParentID)
	}
}

// TestToOutput_MinimalEpic verifies toOutput handles an epic with no optional fields.
func TestToOutput_MinimalEpic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/mygroup/epics/3" {
			testutil.RespondJSON(w, http.StatusOK, epicMinimalJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: testGroupID, EpicIID: 3})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.ID != 103 {
		t.Errorf("out.ID = %d, want 103", out.ID)
	}
	if out.StartDate != "" {
		t.Errorf("out.StartDate = %q, want empty", out.StartDate)
	}
	if out.DueDate != "" {
		t.Errorf("out.DueDate = %q, want empty", out.DueDate)
	}
	if out.ClosedAt != "" {
		t.Errorf("out.ClosedAt = %q, want empty", out.ClosedAt)
	}
}

// --- Markdown formatters ---

// TestFormatOutputMarkdown validates the Markdown formatter for a single epic.
// Covers: all fields populated, minimal fields, confidential flag, and description block.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name      string
		input     Output
		wantParts []string
		dontWant  []string
	}{
		{
			name: "all fields populated",
			input: Output{
				IID:          1,
				Title:        "Q1 Planning",
				State:        "opened",
				Author:       "alice",
				Confidential: true,
				Labels:       []string{"planning", "q1"},
				StartDate:    "2026-01-01",
				DueDate:      "2026-03-31",
				CreatedAt:    "2026-01-01T00:00:00Z",
				ClosedAt:     "2026-03-31T00:00:00Z",
				WebURL:       "https://gitlab.example.com/groups/mygroup/-/epics/1",
				Description:  "Quarterly **planning** epic",
			},
			wantParts: []string{
				"## Epic &1",
				"Q1 Planning",
				"opened",
				"alice",
				"**Confidential**: yes",
				"planning, q1",
				"**Start date**: 2026-01-01",
				"**Due date**: 2026-03-31",
				"gitlab.example.com",
				"Quarterly **planning** epic",
			},
		},
		{
			name: "minimal fields omits optional parts",
			input: Output{
				IID:       2,
				Title:     "Minimal",
				State:     "opened",
				Author:    "bob",
				CreatedAt: "2026-06-01T00:00:00Z",
			},
			wantParts: []string{
				"## Epic &2",
				"Minimal",
				"bob",
			},
			dontWant: []string{
				"Confidential",
				"Start date",
				"Due date",
				"Closed",
				"Labels",
			},
		},
		{
			name: "closed epic shows closed timestamp",
			input: Output{
				IID:       3,
				Title:     "Done",
				State:     "closed",
				Author:    "carol",
				CreatedAt: "2026-01-01T00:00:00Z",
				ClosedAt:  "2026-02-01T00:00:00Z",
			},
			wantParts: []string{
				"closed",
				"**Closed**",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdown(tt.input)
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot:\n%s", part, got)
				}
			}
			for _, part := range tt.dontWant {
				if strings.Contains(got, part) {
					t.Errorf("output should not contain %q\ngot:\n%s", part, got)
				}
			}
		})
	}
}

// TestFormatListMarkdown validates the Markdown formatter for a list of epics.
// Covers: empty list, single epic, multiple epics with labels.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name      string
		input     ListOutput
		wantParts []string
		dontWant  []string
	}{
		{
			name:  "empty list shows no-results message",
			input: ListOutput{Epics: nil},
			wantParts: []string{
				"No epics found.",
			},
			dontWant: []string{
				"| IID |",
			},
		},
		{
			name: "single epic renders table",
			input: ListOutput{
				Epics: []Output{
					{
						IID:       1,
						Title:     "Epic One",
						State:     "opened",
						Author:    "alice",
						Labels:    []string{"bug"},
						CreatedAt: "2026-01-01T00:00:00Z",
						WebURL:    "https://gitlab.example.com/groups/g/-/epics/1",
					},
				},
				Pagination: toolutil.PaginationOutput{TotalItems: 1},
			},
			wantParts: []string{
				"## Group Epics (1)",
				"| IID | Title | State | Author | Labels | Created |",
				"&1",
				"Epic One",
				"opened",
				"alice",
				"bug",
			},
		},
		{
			name: "multiple epics with labels",
			input: ListOutput{
				Epics: []Output{
					{IID: 1, Title: "A", State: "opened", Author: "x", Labels: []string{"a", "b"}, CreatedAt: "2026-01-01T00:00:00Z"},
					{IID: 2, Title: "B", State: "closed", Author: "y", CreatedAt: "2026-02-01T00:00:00Z"},
				},
				Pagination: toolutil.PaginationOutput{TotalItems: 2},
			},
			wantParts: []string{
				"## Group Epics (2)",
				"&1",
				"&2",
				"a, b",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot:\n%s", part, got)
				}
			}
			for _, part := range tt.dontWant {
				if strings.Contains(got, part) {
					t.Errorf("output should not contain %q\ngot:\n%s", part, got)
				}
			}
		})
	}
}

// TestFormatLinksMarkdown validates the Markdown formatter for child epics.
// Covers: empty list, single child, multiple children.
func TestFormatLinksMarkdown(t *testing.T) {
	tests := []struct {
		name      string
		input     LinksOutput
		wantParts []string
		dontWant  []string
	}{
		{
			name:  "empty child list",
			input: LinksOutput{ChildEpics: nil},
			wantParts: []string{
				"Child Epics (0)",
				"No child epics found.",
			},
			dontWant: []string{
				"| IID |",
			},
		},
		{
			name: "single child epic",
			input: LinksOutput{
				ChildEpics: []Output{
					{
						IID:       10,
						Title:     "Sub Task",
						State:     "opened",
						Author:    "dev1",
						CreatedAt: "2026-05-01T00:00:00Z",
						WebURL:    "https://gitlab.example.com/groups/g/-/epics/10",
					},
				},
			},
			wantParts: []string{
				"Child Epics (1)",
				"| IID | Title | State | Author | Created |",
				"&10",
				"Sub Task",
				"opened",
				"dev1",
			},
		},
		{
			name: "multiple children",
			input: LinksOutput{
				ChildEpics: []Output{
					{IID: 10, Title: "A", State: "opened", Author: "x", CreatedAt: "2026-01-01T00:00:00Z"},
					{IID: 11, Title: "B", State: "closed", Author: "y", CreatedAt: "2026-02-01T00:00:00Z"},
				},
			},
			wantParts: []string{
				"Child Epics (2)",
				"&10",
				"&11",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatLinksMarkdown(tt.input)
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot:\n%s", part, got)
				}
			}
			for _, part := range tt.dontWant {
				if strings.Contains(got, part) {
					t.Errorf("output should not contain %q\ngot:\n%s", part, got)
				}
			}
		})
	}
}
