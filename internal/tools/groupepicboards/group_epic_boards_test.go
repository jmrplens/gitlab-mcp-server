// group_epic_boards_test.go validates the List and Get handlers for GitLab
// group epic board operations, covering success paths, input validation
// (missing group_id, missing/zero board_id), API error responses, context
// cancellation, pagination parameter forwarding, empty results, and edge
// cases in toOutput (nil labels, nil list entries, lists without labels).

package groupepicboards

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	pathBoards = "/api/v4/groups/mygroup/epic_boards"
	pathBoard1 = "/api/v4/groups/mygroup/epic_boards/1"

	boardJSON = `{
		"id": 1,
		"name": "Epic Board",
		"labels": [{"id": 10, "name": "Priority"}],
		"lists": [
			{"id": 100, "label": {"id": 10, "name": "Priority"}, "position": 0}
		]
	}`

	testGroupID = "mygroup"
)

// TestList validates the List handler for group epic boards across success,
// error, validation, pagination, and edge-case scenarios.
func TestList(t *testing.T) {
	tests := []struct {
		name      string
		input     ListInput
		handler   http.HandlerFunc
		wantErr   bool
		wantCount int
		validate  func(t *testing.T, out ListOutput)
	}{
		{
			name:  "returns boards on success",
			input: ListInput{GroupID: testGroupID},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathBoards)
				testutil.RespondJSON(w, http.StatusOK, "["+boardJSON+"]")
			}),
			wantCount: 1,
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if out.Boards[0].ID != 1 {
					t.Errorf("Boards[0].ID = %d, want 1", out.Boards[0].ID)
				}
				if out.Boards[0].Name != "Epic Board" {
					t.Errorf("Boards[0].Name = %q, want %q", out.Boards[0].Name, "Epic Board")
				}
				if len(out.Boards[0].Labels) != 1 {
					t.Fatalf("len(Labels) = %d, want 1", len(out.Boards[0].Labels))
				}
				if out.Boards[0].Labels[0] != "Priority" {
					t.Errorf("Labels[0] = %q, want %q", out.Boards[0].Labels[0], "Priority")
				}
				if len(out.Boards[0].Lists) != 1 {
					t.Fatalf("len(Lists) = %d, want 1", len(out.Boards[0].Lists))
				}
				if out.Boards[0].Lists[0].Label != "Priority" {
					t.Errorf("Lists[0].Label = %q, want %q", out.Boards[0].Lists[0].Label, "Priority")
				}
			},
		},
		{
			name:  "returns empty boards for empty array",
			input: ListInput{GroupID: testGroupID},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, "[]")
			}),
			wantCount: 0,
		},
		{
			name:  "forwards pagination parameters",
			input: ListInput{GroupID: testGroupID, PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5}},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertQueryParam(t, r, "page", "2")
				testutil.AssertQueryParam(t, r, "per_page", "5")
				testutil.RespondJSONWithPagination(w, http.StatusOK, "[]", testutil.PaginationHeaders{
					Page: "2", PerPage: "5", Total: "10", TotalPages: "2",
				})
			}),
			wantCount: 0,
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if out.Pagination.TotalItems != 10 {
					t.Errorf("TotalItems = %d, want 10", out.Pagination.TotalItems)
				}
			},
		},
		{
			name:  "returns error for missing group_id",
			input: ListInput{},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("handler should not be called for missing group_id")
			}),
			wantErr: true,
		},
		{
			name:  "returns error on API failure",
			input: ListInput{GroupID: testGroupID},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := List(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("List() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(out.Boards) != tt.wantCount {
					t.Fatalf("len(Boards) = %d, want %d", len(out.Boards), tt.wantCount)
				}
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestList_CancelledContext verifies the List handler returns an error
// immediately when the context is already cancelled.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called for cancelled context")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{GroupID: testGroupID})
	if err == nil {
		t.Fatal("List() expected context error, got nil")
	}
}

// TestGet validates the Get handler for group epic boards across success,
// error, validation, and edge-case scenarios.
func TestGet(t *testing.T) {
	tests := []struct {
		name     string
		input    GetInput
		handler  http.HandlerFunc
		wantErr  bool
		validate func(t *testing.T, out Output)
	}{
		{
			name:  "returns board on success",
			input: GetInput{GroupID: testGroupID, BoardID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathBoard1)
				testutil.RespondJSON(w, http.StatusOK, boardJSON)
			}),
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != 1 {
					t.Errorf("ID = %d, want 1", out.ID)
				}
				if out.Name != "Epic Board" {
					t.Errorf("Name = %q, want %q", out.Name, "Epic Board")
				}
				if len(out.Labels) != 1 || out.Labels[0] != "Priority" {
					t.Errorf("Labels = %v, want [Priority]", out.Labels)
				}
				if len(out.Lists) != 1 {
					t.Fatalf("len(Lists) = %d, want 1", len(out.Lists))
				}
				if out.Lists[0].LabelID != 10 {
					t.Errorf("Lists[0].LabelID = %d, want 10", out.Lists[0].LabelID)
				}
				if out.Lists[0].Position != 0 {
					t.Errorf("Lists[0].Position = %d, want 0", out.Lists[0].Position)
				}
			},
		},
		{
			name:  "returns board with no labels and no lists",
			input: GetInput{GroupID: testGroupID, BoardID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"Empty Board"}`)
			}),
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.Name != "Empty Board" {
					t.Errorf("Name = %q, want %q", out.Name, "Empty Board")
				}
				if len(out.Labels) != 0 {
					t.Errorf("len(Labels) = %d, want 0", len(out.Labels))
				}
				if len(out.Lists) != 0 {
					t.Errorf("len(Lists) = %d, want 0", len(out.Lists))
				}
			},
		},
		{
			name:  "handles list entry without label",
			input: GetInput{GroupID: testGroupID, BoardID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"B","lists":[{"id":50,"position":2}]}`)
			}),
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if len(out.Lists) != 1 {
					t.Fatalf("len(Lists) = %d, want 1", len(out.Lists))
				}
				if out.Lists[0].Label != "" {
					t.Errorf("Lists[0].Label = %q, want empty", out.Lists[0].Label)
				}
				if out.Lists[0].LabelID != 0 {
					t.Errorf("Lists[0].LabelID = %d, want 0", out.Lists[0].LabelID)
				}
				if out.Lists[0].Position != 2 {
					t.Errorf("Lists[0].Position = %d, want 2", out.Lists[0].Position)
				}
			},
		},
		{
			name:  "handles null label entry in labels array",
			input: GetInput{GroupID: testGroupID, BoardID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"B","labels":[null,{"id":1,"name":"Bug"}]}`)
			}),
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if len(out.Labels) != 1 {
					t.Fatalf("len(Labels) = %d, want 1", len(out.Labels))
				}
				if out.Labels[0] != "Bug" {
					t.Errorf("Labels[0] = %q, want %q", out.Labels[0], "Bug")
				}
			},
		},
		{
			name:  "returns error for missing group_id",
			input: GetInput{BoardID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("handler should not be called for missing group_id")
			}),
			wantErr: true,
		},
		{
			name:  "returns error for zero board_id",
			input: GetInput{GroupID: testGroupID, BoardID: 0},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("handler should not be called for zero board_id")
			}),
			wantErr: true,
		},
		{
			name:  "returns error for negative board_id",
			input: GetInput{GroupID: testGroupID, BoardID: -1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("handler should not be called for negative board_id")
			}),
			wantErr: true,
		},
		{
			name:  "returns error on 404 response",
			input: GetInput{GroupID: testGroupID, BoardID: 999},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			}),
			wantErr: true,
		},
		{
			name:  "returns error on 500 response",
			input: GetInput{GroupID: testGroupID, BoardID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Get(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestGet_CancelledContext verifies the Get handler returns an error
// immediately when the context is already cancelled.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called for cancelled context")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{GroupID: testGroupID, BoardID: 1})
	if err == nil {
		t.Fatal("Get() expected context error, got nil")
	}
}

// TestFormatOutputMarkdown validates the single-board Markdown formatter
// for boards with/without labels and lists.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    Output
		contains []string
		excludes []string
	}{
		{
			name: "renders board with labels and lists",
			input: Output{
				ID:     1,
				Name:   "Sprint Board",
				Labels: []string{"Priority", "Bug"},
				Lists: []BoardListEntry{
					{ID: 100, Label: "Priority", LabelID: 10, Position: 0},
					{ID: 101, Label: "Bug", LabelID: 11, Position: 1},
				},
			},
			contains: []string{
				"## Epic Board #1 — Sprint Board",
				"**Labels**: Priority, Bug",
				"### Board Lists",
				"| 100 | Priority | 0 |",
				"| 101 | Bug | 1 |",
			},
		},
		{
			name: "renders board without labels or lists",
			input: Output{
				ID:   2,
				Name: "Empty Board",
			},
			contains: []string{"## Epic Board #2 — Empty Board"},
			excludes: []string{"**Labels**", "### Board Lists"},
		},
		{
			name: "escapes pipe characters in name",
			input: Output{
				ID:   3,
				Name: "Foo | Bar",
			},
			contains: []string{"## Epic Board #3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q", s)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(got, s) {
					t.Errorf("output should not contain %q", s)
				}
			}
		})
	}
}

// TestFormatListMarkdown validates the list Markdown formatter for
// non-empty boards, empty boards, and pagination output.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		contains []string
	}{
		{
			name: "renders board list table",
			input: ListOutput{
				Boards: []Output{
					{ID: 1, Name: "Sprint", Labels: []string{"P1"}, Lists: []BoardListEntry{{ID: 10}}},
					{ID: 2, Name: "Backlog"},
				},
				Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
			},
			contains: []string{
				"## Group Epic Boards (2)",
				"| 1 | Sprint | P1 | 1 |",
				"| 2 | Backlog |  | 0 |",
			},
		},
		{
			name: "renders empty state",
			input: ListOutput{
				Pagination: toolutil.PaginationOutput{TotalItems: 0},
			},
			contains: []string{
				"No epic boards found.",
			},
		},
		{
			name: "shows pagination when multiple pages",
			input: ListOutput{
				Boards: []Output{{ID: 1, Name: "B"}},
				Pagination: toolutil.PaginationOutput{
					TotalItems: 50, Page: 1, PerPage: 20, TotalPages: 3, NextPage: 2,
				},
			},
			contains: []string{
				"## Group Epic Boards (50)",
				"Page 1 of 3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q\ngot:\n%s", s, got)
				}
			}
		})
	}
}
