// epics_test.go contains unit tests for GitLab group epic operations.
// Tests use httptest to mock the GitLab Epics API.

package epics

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// testMinimalWorkItem is a bare-minimum WorkItem for toOutput converter tests.
var testMinimalWorkItem = gl.WorkItem{
	ID:    1,
	IID:   1,
	Type:  "Epic",
	State: "OPEN",
	Title: "Minimal Epic",
}

// testFullWorkItem exercises every optional field path in toOutput.
var testFullWorkItem = func() gl.WorkItem {
	status := "IN_PROGRESS"
	color := "#FF0000"
	health := "onTrack"
	weight := int64(5)
	start := gl.ISOTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	due := gl.ISOTime(time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC))
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	return gl.WorkItem{
		ID:           101,
		IID:          1,
		Type:         "Epic",
		State:        "CLOSED",
		Status:       &status,
		Title:        "Q1 Planning",
		Description:  "Full description",
		WebURL:       "https://gitlab.example.com/groups/g/-/epics/1",
		Confidential: true,
		Author:       &gl.BasicUser{Username: "alice"},
		Assignees:    []*gl.BasicUser{{Username: "bob"}, {Username: "carol"}},
		Labels:       []gl.LabelDetails{{Name: "planning"}, {Name: "priority"}},
		LinkedItems:  []gl.LinkedWorkItem{{WorkItemIID: gl.WorkItemIID{IID: 5, NamespacePath: "g/sub"}, LinkType: "blocks"}},
		Color:        &color,
		HealthStatus: &health,
		Weight:       &weight,
		Parent:       &gl.WorkItemIID{IID: 10, NamespacePath: "g"},
		StartDate:    &start,
		DueDate:      &due,
		CreatedAt:    &created,
		UpdatedAt:    &updated,
		ClosedAt:     &closed,
	}
}()

const (
	testFullPath   = "my-group"
	fmtWantID      = "out.ID = %d, want 101"
	fmtWantTitle   = "out.Title = %q, want %q"
	fmtUnexpErr    = "unexpected error: %v"
	fmtUnexpMethod = "unexpected method: %s"
	errExpectedNil = "expected error, got nil"

	// GraphQL JSON for a single work item of type Epic.
	workItemEpicJSON = `{
		"id":"gid://gitlab/WorkItem/101",
		"iid":"1",
		"workItemType":{"name":"Epic"},
		"state":"OPEN",
		"title":"Q1 Planning",
		"description":"Quarterly planning epic",
		"webUrl":"https://gitlab.example.com/groups/my-group/-/epics/1",
		"confidential":false,
		"author":{"username":"alice"},
		"widgets":[
			{"type":"ASSIGNEES","assignees":{"nodes":[{"username":"bob"}]}},
			{"type":"LABELS","labels":{"nodes":[{"name":"planning","id":"gid://gitlab/Label/1","color":"#428BCA","description":""}]}},
			{"type":"START_AND_DUE_DATE","startDate":"2026-01-01","dueDate":"2026-03-31"},
			{"type":"COLOR","color":"#FF0000"},
			{"type":"HEALTH_STATUS","healthStatus":"onTrack"},
			{"type":"WEIGHT","weight":5},
			{"type":"STATUS","status":"IN_PROGRESS"}
		],
		"createdAt":"2026-01-01T00:00:00Z",
		"updatedAt":"2026-01-02T00:00:00Z"
	}`

	// GraphQL response envelope for Get.
	getResponseJSON = `{"data":{"namespace":{"workItem":` + workItemEpicJSON + `}}}`

	// GraphQL response envelope for List.
	listResponseJSON = `{"data":{"namespace":{"workItems":{"nodes":[` + workItemEpicJSON + `],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`

	// GraphQL response envelope for Create.
	createResponseJSON = `{"data":{"workItemCreate":{"workItem":` + workItemEpicJSON + `}}}`

	// GraphQL response envelope for Update.
	updateResponseJSON = `{"data":{"workItemUpdate":{"workItem":` + workItemEpicJSON + `}}}`

	// GraphQL response envelope for Delete (two-step: resolve GID + delete).
	deleteGIDResponseJSON    = `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/101"}}}}`
	deleteDeleteResponseJSON = `{"data":{"workItemDelete":{"errors":[]}}}`

	// REST JSON for GetLinks (child epics).
	epicLinkJSON = `{
		"id": 201,
		"iid": 2,
		"title": "Sub-Epic",
		"state": "opened",
		"web_url": "https://gitlab.example.com/groups/my-group/-/epics/2",
		"author": {"username": "carol"},
		"labels": ["sub"],
		"confidential": false,
		"created_at": "2026-02-01T00:00:00Z"
	}`
)

// --- List tests ---

// TestList_Success verifies List returns one epic with Type="Epic" when the
// GraphQL namespace.workItems query responds 200 with a single work item.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, listResponseJSON)
	}))
	out, err := List(context.Background(), client, ListInput{FullPath: testFullPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Epics) != 1 {
		t.Fatalf("len(Epics) = %d, want 1", len(out.Epics))
	}
	if out.Epics[0].ID != 101 {
		t.Errorf(fmtWantID, out.Epics[0].ID)
	}
	if out.Epics[0].Type != "Epic" {
		t.Errorf("Type = %q, want Epic", out.Epics[0].Type)
	}
}

// TestList_MissingFullPath verifies List returns a validation error when
// full_path is empty, without issuing any GraphQL request.
func TestList_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing full_path, got nil")
	}
}

// TestList_CancelledContext verifies List returns a context error when
// invoked with an already-cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("List() expected context error, got nil")
	}
}

// TestList_APIError verifies List propagates an error when the GraphQL
// endpoint responds 403 Forbidden.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := List(context.Background(), client, ListInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// --- Get tests ---

// TestGet_Success verifies Get returns an epic with the expected ID, title,
// author, and Type="Epic" when the GraphQL namespace.workItem query responds
// 200 with a full work item payload.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, getResponseJSON)
	}))
	out, err := Get(context.Background(), client, GetInput{FullPath: testFullPath, IID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 101 {
		t.Errorf(fmtWantID, out.ID)
	}
	if out.Title != "Q1 Planning" {
		t.Errorf(fmtWantTitle, out.Title, "Q1 Planning")
	}
	if out.Author != "alice" {
		t.Errorf("out.Author = %q, want alice", out.Author)
	}
	if out.Type != "Epic" {
		t.Errorf("Type = %q, want Epic", out.Type)
	}
}

// TestGet_MissingIID verifies Get returns a validation error mentioning "iid"
// when the iid field is zero, without issuing a GraphQL request.
func TestGet_MissingIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := Get(context.Background(), client, GetInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("Get() expected error for missing iid, got nil")
	}
	if !strings.Contains(err.Error(), "epic_iid") {
		t.Errorf("expected error to mention 'iid', got: %v", err)
	}
}

// TestGet_MissingFullPath verifies Get returns a validation error when
// full_path is empty.
func TestGet_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := Get(context.Background(), client, GetInput{IID: 1})
	if err == nil {
		t.Fatal("Get() expected error for missing full_path, got nil")
	}
}

// TestGet_APIError verifies Get propagates an error when the GraphQL endpoint
// responds 404 Not Found.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := Get(context.Background(), client, GetInput{FullPath: testFullPath, IID: 999})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// --- GetLinks tests (REST) ---

// TestGetLinks_Success verifies GetLinks returns the child epic list when
// GET /groups/:path/epics/:iid/epics responds 200 with one linked epic.
func TestGetLinks_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/epics/1/epics") {
			testutil.RespondJSON(w, http.StatusOK, "["+epicLinkJSON+"]")
			return
		}
		http.NotFound(w, r)
	}))
	out, err := GetLinks(context.Background(), client, GetLinksInput{FullPath: testFullPath, IID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.ChildEpics) != 1 {
		t.Fatalf("len(ChildEpics) = %d, want 1", len(out.ChildEpics))
	}
	if out.ChildEpics[0].ID != 201 {
		t.Errorf("ChildEpics[0].ID = %d, want 201", out.ChildEpics[0].ID)
	}
	if out.ChildEpics[0].Author != "carol" {
		t.Errorf("ChildEpics[0].Author = %q, want carol", out.ChildEpics[0].Author)
	}
}

// TestGetLinks_MissingIID verifies GetLinks returns a validation error when
// iid is zero.
func TestGetLinks_MissingIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := GetLinks(context.Background(), client, GetLinksInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("GetLinks() expected error for missing iid, got nil")
	}
}

// TestGetLinks_MissingFullPath verifies GetLinks returns a validation error
// when full_path is empty.
func TestGetLinks_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := GetLinks(context.Background(), client, GetLinksInput{IID: 1})
	if err == nil {
		t.Fatal("GetLinks() expected error for missing full_path, got nil")
	}
}

// --- Create tests ---

// TestCreate_Success verifies Create returns the new epic when the GraphQL
// workItemCreate mutation responds 200 with a work item payload.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, createResponseJSON)
	}))
	out, err := Create(context.Background(), client, CreateInput{
		FullPath: testFullPath,
		Title:    "Q1 Planning",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 101 {
		t.Errorf(fmtWantID, out.ID)
	}
	if out.Title != "Q1 Planning" {
		t.Errorf(fmtWantTitle, out.Title, "Q1 Planning")
	}
}

// TestCreate_MissingTitle verifies Create returns a validation error when
// the title field is empty.
func TestCreate_MissingTitle(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := Create(context.Background(), client, CreateInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("Create() expected error for missing title, got nil")
	}
}

// TestCreate_MissingFullPath verifies Create returns a validation error when
// full_path is empty.
func TestCreate_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := Create(context.Background(), client, CreateInput{Title: "Some Title"})
	if err == nil {
		t.Fatal("Create() expected error for missing full_path, got nil")
	}
}

// TestCreate_APIError verifies Create propagates an error when the GraphQL
// workItemCreate mutation responds 403 Forbidden.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := Create(context.Background(), client, CreateInput{FullPath: testFullPath, Title: "Epic"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// --- Update tests ---

// TestUpdate_Success verifies Update succeeds across the two-step GraphQL
// flow: first call resolves the IID to a work item global ID, second call
// performs the workItemUpdate mutation with the new title and state.
func TestUpdate_Success(t *testing.T) {
	call := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		call++
		switch call {
		case 1:
			// workItemGID query to resolve global ID
			testutil.RespondJSON(w, http.StatusOK, deleteGIDResponseJSON)
		default:
			// workItemUpdate mutation
			testutil.RespondJSON(w, http.StatusOK, updateResponseJSON)
		}
	}))
	out, err := Update(context.Background(), client, UpdateInput{
		FullPath:   testFullPath,
		IID:        1,
		Title:      "Updated Title",
		StateEvent: "CLOSE",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 101 {
		t.Errorf(fmtWantID, out.ID)
	}
}

// TestUpdate_MissingIID verifies Update returns a validation error when
// iid is zero.
func TestUpdate_MissingIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := Update(context.Background(), client, UpdateInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("Update() expected error for missing iid, got nil")
	}
}

// TestUpdate_MissingFullPath verifies Update returns a validation error when
// full_path is empty.
func TestUpdate_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := Update(context.Background(), client, UpdateInput{IID: 1})
	if err == nil {
		t.Fatal("Update() expected error for missing full_path, got nil")
	}
}

// TestUpdate_APIError verifies Update propagates an error when the first
// GraphQL call (GID resolution) responds 403 Forbidden.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := Update(context.Background(), client, UpdateInput{FullPath: testFullPath, IID: 1, Title: "X"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// --- Delete tests ---

// TestDelete_Success verifies Delete succeeds across the two-step GraphQL
// flow: first call resolves the IID to a work item global ID, second call
// performs the workItemDelete mutation with no errors.
func TestDelete_Success(t *testing.T) {
	call := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		call++
		switch call {
		case 1:
			// workItemGID query to resolve global ID
			testutil.RespondJSON(w, http.StatusOK, deleteGIDResponseJSON)
		default:
			// workItemDelete mutation
			testutil.RespondJSON(w, http.StatusOK, deleteDeleteResponseJSON)
		}
	}))
	err := Delete(context.Background(), client, DeleteInput{FullPath: testFullPath, IID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_MissingIID verifies Delete returns a validation error when
// iid is zero.
func TestDelete_MissingIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	err := Delete(context.Background(), client, DeleteInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("Delete() expected error for missing iid, got nil")
	}
}

// TestDelete_MissingFullPath verifies Delete returns a validation error when
// full_path is empty.
func TestDelete_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	err := Delete(context.Background(), client, DeleteInput{IID: 1})
	if err == nil {
		t.Fatal("Delete() expected error for missing full_path, got nil")
	}
}

// TestDelete_APIError verifies Delete propagates an error when the first
// GraphQL call (GID resolution) responds 403 Forbidden.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	err := Delete(context.Background(), client, DeleteInput{FullPath: testFullPath, IID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// --- toOutput converter tests ---

// TestToOutput_Minimal verifies toOutput correctly handles a minimally
// populated WorkItem, leaving optional fields (Author, Assignees) at their
// zero values without panicking.
func TestToOutput_Minimal(t *testing.T) {
	out := toOutput(&testMinimalWorkItem)
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
	if out.Author != "" {
		t.Errorf("Author should be empty, got %q", out.Author)
	}
	if len(out.Assignees) != 0 {
		t.Errorf("Assignees should be empty, got %v", out.Assignees)
	}
}

// TestMapStatusToID uses table-driven subtests to verify mapStatusToID returns
// a non-empty status ID for every supported status (TODO, IN_PROGRESS, DONE,
// WONT_DO, DUPLICATE) and falls back to CUSTOM for unknown values.
func TestMapStatusToID(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"TODO", "TODO"},
		{"IN_PROGRESS", "IN_PROGRESS"},
		{"DONE", "DONE"},
		{"WONT_DO", "WONT_DO"},
		{"DUPLICATE", "DUPLICATE"},
		{"unknown", "CUSTOM"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := mapStatusToID(tc.in)
			if result == "" {
				t.Errorf("mapStatusToID(%q) returned empty", tc.in)
			}
		})
	}
}

// --- Markdown tests ---

// TestFormatOutputMarkdown verifies FormatOutputMarkdown produces a non-empty
// string for a minimally populated Output.
func TestFormatOutputMarkdown(t *testing.T) {
	out := Output{
		IID: 1, Title: "Epic", Type: "Epic", State: "OPEN", Author: "alice",
	}
	result := FormatOutputMarkdown(out)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown produces a
// non-empty string for a zero-value ListOutput (empty epic list).
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

// TestFormatLinksMarkdown verifies FormatLinksMarkdown produces a non-empty
// string for a LinksOutput containing one child epic.
func TestFormatLinksMarkdown(t *testing.T) {
	out := LinksOutput{
		ChildEpics: []LinksItem{
			{IID: 2, Title: "Sub", State: "opened", Author: "bob"},
		},
	}
	result := FormatLinksMarkdown(out)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

// --- toOutput full coverage ---

// TestToOutput_FullFields verifies that toOutput correctly maps every optional
// field on a WorkItem (status, color, health, dates, parent, linked items,
// assignees, labels, closed) to the Output struct.
func TestToOutput_FullFields(t *testing.T) {
	out := toOutput(&testFullWorkItem)

	if out.ID != 101 {
		t.Errorf("ID = %d, want 101", out.ID)
	}
	if out.Status != "IN_PROGRESS" {
		t.Errorf("Status = %q, want IN_PROGRESS", out.Status)
	}
	if out.Author != "alice" {
		t.Errorf("Author = %q, want alice", out.Author)
	}
	if len(out.Assignees) != 2 || out.Assignees[0] != "bob" || out.Assignees[1] != "carol" {
		t.Errorf("Assignees = %v, want [bob carol]", out.Assignees)
	}
	if len(out.Labels) != 2 || out.Labels[0] != "planning" {
		t.Errorf("Labels = %v, want [planning priority]", out.Labels)
	}
	if len(out.LinkedItems) != 1 || out.LinkedItems[0].IID != 5 || out.LinkedItems[0].LinkType != "blocks" {
		t.Errorf("LinkedItems = %v, unexpected", out.LinkedItems)
	}
	if out.Color != "#FF0000" {
		t.Errorf("Color = %q, want #FF0000", out.Color)
	}
	if out.HealthStatus != "onTrack" {
		t.Errorf("HealthStatus = %q, want onTrack", out.HealthStatus)
	}
	if out.ParentIID != 10 {
		t.Errorf("ParentIID = %d, want 10", out.ParentIID)
	}
	if out.ParentPath != "g" {
		t.Errorf("ParentPath = %q, want g", out.ParentPath)
	}
	if out.StartDate != "2026-01-01" {
		t.Errorf("StartDate = %q, want 2026-01-01", out.StartDate)
	}
	if out.DueDate != "2026-03-31" {
		t.Errorf("DueDate = %q, want 2026-03-31", out.DueDate)
	}
	if out.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want 2026-01-01T00:00:00Z", out.CreatedAt)
	}
	if out.UpdatedAt != "2026-01-02T00:00:00Z" {
		t.Errorf("UpdatedAt = %q, want 2026-01-02T00:00:00Z", out.UpdatedAt)
	}
	if out.ClosedAt != "2026-03-01T00:00:00Z" {
		t.Errorf("ClosedAt = %q, want 2026-03-01T00:00:00Z", out.ClosedAt)
	}
	if !out.Confidential {
		t.Error("Confidential should be true")
	}
	if out.Weight == nil || *out.Weight != 5 {
		t.Errorf("Weight = %v, want 5", out.Weight)
	}
}

// --- toLinkItem coverage ---

// TestToLinkItem_NilAuthorAndCreatedAt verifies toLinkItem handles a minimal
// Epic with nil Author and nil CreatedAt without panicking.
func TestToLinkItem_NilAuthorAndCreatedAt(t *testing.T) {
	e := &gl.Epic{ID: 10, IID: 3, Title: "Bare", State: "opened"}
	item := toLinkItem(e)
	if item.Author != "" {
		t.Errorf("Author = %q, want empty", item.Author)
	}
	if item.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty", item.CreatedAt)
	}
}

// --- List with all filter options ---

// TestList_WithAllFilters verifies that List passes all filter parameters to
// the GraphQL API without errors when every optional field is populated.
func TestList_WithAllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars, err := testutil.ParseGraphQLVariables(r)
		if err != nil {
			t.Fatalf("ParseGraphQLVariables: %v", err)
		}
		for _, key := range []string{"fullPath", "state", "search", "authorUsername", "labelName", "confidential", "sort", "first", "after", "includeAncestors", "includeDescendants"} {
			if _, ok := vars[key]; !ok {
				t.Errorf("GraphQL variables missing %q", key)
			}
		}
		testutil.RespondJSON(w, http.StatusOK, listResponseJSON)
	}))
	boolTrue := true
	first := int64(10)
	out, err := List(context.Background(), client, ListInput{
		FullPath:           testFullPath,
		State:              "opened",
		Search:             "planning",
		AuthorUsername:     "alice",
		LabelName:          []string{"urgent"},
		Confidential:       &boolTrue,
		Sort:               "CREATED_DESC",
		First:              &first,
		After:              "abc123",
		IncludeAncestors:   &boolTrue,
		IncludeDescendants: &boolTrue,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Epics) != 1 {
		t.Errorf("len(Epics) = %d, want 1", len(out.Epics))
	}
}

// --- Create with all optional fields ---

// TestCreate_WithAllOptions verifies that Create handles all optional fields
// (description, confidential, color, dates, assignees, labels, weight, health)
// without errors.
func TestCreate_WithAllOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars, err := testutil.ParseGraphQLVariables(r)
		if err != nil {
			t.Fatalf("ParseGraphQLVariables: %v", err)
		}
		input, ok := vars["input"].(map[string]any)
		if !ok {
			t.Fatal("GraphQL variables missing 'input' object")
		}
		for _, key := range []string{"title", "confidential", "descriptionWidget", "colorWidget", "startAndDueDateWidget", "assigneesWidget", "labelsWidget", "weightWidget", "healthStatusWidget"} {
			if _, exists := input[key]; !exists {
				t.Errorf("GraphQL input missing %q", key)
			}
		}
		testutil.RespondJSON(w, http.StatusOK, createResponseJSON)
	}))
	boolTrue := true
	weight := int64(5)
	out, err := Create(context.Background(), client, CreateInput{
		FullPath:     testFullPath,
		Title:        "Full Epic",
		Description:  "Full description\nwith newlines",
		Confidential: &boolTrue,
		Color:        "#FF0000",
		StartDate:    "2026-01-01",
		DueDate:      "2026-03-31",
		AssigneeIDs:  []int64{1, 2},
		LabelIDs:     []int64{10, 20},
		Weight:       &weight,
		HealthStatus: "onTrack",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 101 {
		t.Errorf(fmtWantID, out.ID)
	}
}

// TestCreate_CancelledContext verifies that Create returns context error early.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{FullPath: testFullPath, Title: "X"})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// --- Update with all optional fields ---

// TestUpdate_WithAllOptions verifies that Update handles all optional fields
// (title, description, state event, parent, color, dates, labels, assignees,
// weight, health, status) without errors.
func TestUpdate_WithAllOptions(t *testing.T) {
	call := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		switch call {
		case 1:
			testutil.RespondJSON(w, http.StatusOK, deleteGIDResponseJSON)
		default:
			vars, err := testutil.ParseGraphQLVariables(r)
			if err != nil {
				t.Fatalf("ParseGraphQLVariables: %v", err)
			}
			input, ok := vars["input"].(map[string]any)
			if !ok {
				t.Fatal("GraphQL variables missing 'input' object")
			}
			for _, key := range []string{"title", "stateEvent", "descriptionWidget", "colorWidget", "startAndDueDateWidget", "labelsWidget", "assigneesWidget", "weightWidget", "healthStatusWidget", "statusWidget"} {
				if _, exists := input[key]; !exists {
					t.Errorf("GraphQL input missing %q", key)
				}
			}
			testutil.RespondJSON(w, http.StatusOK, updateResponseJSON)
		}
	}))
	parentID := int64(42)
	weight := int64(8)
	out, err := Update(context.Background(), client, UpdateInput{
		FullPath:       testFullPath,
		IID:            1,
		Title:          "Updated",
		Description:    "New description",
		StateEvent:     "CLOSE",
		ParentID:       &parentID,
		Color:          "#00FF00",
		StartDate:      "2026-02-01",
		DueDate:        "2026-04-30",
		AddLabelIDs:    []int64{100},
		RemoveLabelIDs: []int64{200},
		AssigneeIDs:    []int64{1, 2, 3},
		Weight:         &weight,
		HealthStatus:   "needsAttention",
		Status:         "IN_PROGRESS",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 101 {
		t.Errorf(fmtWantID, out.ID)
	}
}

// TestUpdate_CancelledContext verifies that Update returns context error early.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{FullPath: testFullPath, IID: 1, Title: "X"})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// --- GetLinks additional coverage ---

// TestGetLinks_CancelledContext verifies GetLinks returns context error early.
func TestGetLinks_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetLinks(ctx, client, GetLinksInput{FullPath: testFullPath, IID: 1})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestGetLinks_APIError verifies GetLinks wraps API errors.
func TestGetLinks_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := GetLinks(context.Background(), client, GetLinksInput{FullPath: testFullPath, IID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGet_CancelledContext verifies Get returns context error early.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{FullPath: testFullPath, IID: 1})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestDelete_CancelledContext verifies Delete returns context error early.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{FullPath: testFullPath, IID: 1})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// --- Markdown full coverage ---

// TestFormatOutputMarkdown_FullFields verifies that FormatOutputMarkdown
// renders all optional fields (status, assignees, confidential, labels,
// health, weight, dates, color, parent, closedAt, webURL, linked items,
// description) into the Markdown output.
func TestFormatOutputMarkdown_FullFields(t *testing.T) {
	w := int64(5)
	out := Output{
		IID:          1,
		Title:        "Full Epic",
		Type:         "Epic",
		State:        "CLOSED",
		Status:       "IN_PROGRESS",
		Author:       "alice",
		Assignees:    []string{"bob", "carol"},
		Confidential: true,
		Labels:       []string{"planning", "urgent"},
		HealthStatus: "onTrack",
		Weight:       &w,
		StartDate:    "2026-01-01",
		DueDate:      "2026-03-31",
		Color:        "#FF0000",
		ParentIID:    10,
		ParentPath:   "group",
		CreatedAt:    "2026-01-01T00:00:00Z",
		ClosedAt:     "2026-03-01T00:00:00Z",
		WebURL:       "https://gitlab.example.com/groups/g/-/epics/1",
		Description:  "Epic description body",
		LinkedItems: []LinkedItem{
			{IID: 5, LinkType: "blocks", Path: "g/sub"},
		},
	}
	result := FormatOutputMarkdown(out)
	for _, want := range []string{
		"IN_PROGRESS", "bob, carol", "Confidential", "planning", "onTrack",
		"Weight", "2026-01-01", "2026-03-31", "#FF0000", "Parent", "&10",
		"Closed", "gitlab.example.com", "Linked Items", "blocks", "g/sub",
		"Epic description body",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("expected markdown to contain %q", want)
		}
	}
}

// TestFormatLinksMarkdown_Empty verifies FormatLinksMarkdown renders the
// empty state message when no child epics are present.
func TestFormatLinksMarkdown_Empty(t *testing.T) {
	result := FormatLinksMarkdown(LinksOutput{})
	if !strings.Contains(result, "No child epics found") {
		t.Errorf("expected 'No child epics found', got %q", result)
	}
}

// TestFormatListMarkdown_WithLabels verifies that FormatListMarkdown joins
// non-empty Labels with ", " separator and includes the joined value in
// the output. This targets the labels-non-empty branch that builds the
// labels column from the slice.
func TestFormatListMarkdown_WithLabels(t *testing.T) {
	out := ListOutput{Epics: []Output{
		{IID: 1, Title: "Epic A", State: "opened", Author: "alice", Labels: []string{"backend", "priority"}},
	}}
	result := FormatListMarkdown(out)
	if !strings.Contains(result, "backend, priority") {
		t.Errorf("expected joined labels in output; got:\n%s", result)
	}
}
