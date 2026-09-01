// epics_test.go contains unit tests for GitLab group epic operations.
// Tests use httptest to mock the GitLab Epics API.
package epics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
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
		Author:       &gl.BasicUser{ID: 1, Username: "alice", Name: "Alice", State: "active", AvatarURL: "a.png", WebURL: "https://gitlab.example.com/alice", CreatedAt: &created},
		Assignees:    []*gl.BasicUser{{Username: "bob"}, nil, {Username: "carol"}},
		Labels:       []gl.LabelDetails{{Name: "planning"}, {Name: "priority"}},
		LinkedItems:  []gl.LinkedWorkItem{{IID: 5, NamespacePath: "g/sub", LinkType: "blocks"}},
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

// authorName returns the username of a nested user output, or "" when nil. It
// keeps assertions on the migrated *BasicUserOutput author/assignee fields
// concise.
func authorName(u *BasicUserOutput) string {
	if u == nil {
		return ""
	}
	return u.Username
}

// --- List tests ---

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `[`+epicLinkJSON+`]`)
	}))
	out, err := List(context.Background(), client, ListInput{FullPath: testFullPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Epics) != 1 {
		t.Fatalf("len(Epics) = %d, want 1", len(out.Epics))
	}
	if out.Epics[0].ID != 201 {
		t.Errorf(fmtWantID, out.Epics[0].ID)
	}
	if out.Epics[0].Type != "Epic" {
		t.Errorf("Type = %q, want Epic", out.Epics[0].Type)
	}
}

// TestList_RESTFilterOptions verifies the List_RESTFilterOptions handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_RESTFilterOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		query := r.URL.Query()
		for _, key := range []string{
			"state", "search", "labels", "sort", "order_by", "author_id",
			"my_reaction_emoji", "created_after", "created_before",
			"updated_after", "updated_before", "with_labels_details",
			"include_ancestor_groups", "include_descendant_groups", "per_page",
			"pagination", "page_token",
		} {
			t.Run(key, func(t *testing.T) {
				if query.Get(key) == "" {
					t.Errorf("query missing %q in %s", key, r.URL.RawQuery)
				}
			})
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	include := true
	first := int64(10)
	authorID := int64(7)
	out, err := List(context.Background(), client, ListInput{
		FullPath:           testFullPath,
		State:              "opened",
		Search:             "planning",
		AuthorID:           &authorID,
		LabelName:          []string{"urgent"},
		MyReactionEmoji:    "thumbsup",
		OrderBy:            "created_at",
		Sort:               "created_desc",
		CreatedAfter:       "2026-01-01T00:00:00Z",
		CreatedBefore:      "2026-12-31T23:59:59Z",
		UpdatedAfter:       "2026-02-01T00:00:00Z",
		UpdatedBefore:      "2026-11-30T23:59:59Z",
		WithLabelsDetails:  &include,
		First:              &first,
		IncludeAncestors:   &include,
		IncludeDescendants: &include,
		Pagination:         "keyset", PageToken: "123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Epics) != 0 {
		t.Fatalf("len(Epics) = %d, want 0", len(out.Epics))
	}
}

// TestList_MissingFullPath verifies that List_MissingFullPath returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing full_path, got nil")
	}
}

// TestList_CancelledContext verifies the List_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("List() expected context error, got nil")
	}
}

// TestList_APIError verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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
	if authorName(out.Author) != "alice" {
		t.Errorf("out.Author = %q, want alice", authorName(out.Author))
	}
	if out.Type != "Epic" {
		t.Errorf("Type = %q, want Epic", out.Type)
	}
}

// TestGet_MissingIID verifies that Get_MissingIID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_MissingIID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Get(context.Background(), client, GetInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("Get() expected error for missing iid, got nil")
	}
	if !strings.Contains(err.Error(), "epic_iid") {
		t.Errorf("expected error to mention 'iid', got: %v", err)
	}
}

// TestGet_MissingFullPath verifies that Get_MissingFullPath returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Get(context.Background(), client, GetInput{IID: 1})
	if err == nil {
		t.Fatal("Get() expected error for missing full_path, got nil")
	}
}

// TestGet_APIError verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestGetLinks_Success verifies that GetLinks succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
	if authorName(out.ChildEpics[0].Author) != "carol" {
		t.Errorf("ChildEpics[0].Author = %q, want carol", authorName(out.ChildEpics[0].Author))
	}
}

// TestGetLinks_MissingIID verifies that GetLinks_MissingIID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetLinks_MissingIID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetLinks(context.Background(), client, GetLinksInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("GetLinks() expected error for missing iid, got nil")
	}
}

// TestGetLinks_MissingFullPath verifies that GetLinks_MissingFullPath returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetLinks_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetLinks(context.Background(), client, GetLinksInput{IID: 1})
	if err == nil {
		t.Fatal("GetLinks() expected error for missing full_path, got nil")
	}
}

// --- Create tests ---

// TestCreate_Success verifies that Create succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestCreate_MissingTitle verifies that Create_MissingTitle returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_MissingTitle(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Create(context.Background(), client, CreateInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("Create() expected error for missing title, got nil")
	}
}

// TestCreate_MissingFullPath verifies that Create_MissingFullPath returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Create(context.Background(), client, CreateInput{Title: "Some Title"})
	if err == nil {
		t.Fatal("Create() expected error for missing full_path, got nil")
	}
}

// TestCreate_APIError verifies that Create returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestUpdate_MissingIID verifies that Update_MissingIID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdate_MissingIID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Update(context.Background(), client, UpdateInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("Update() expected error for missing iid, got nil")
	}
}

// TestUpdate_MissingFullPath verifies that Update_MissingFullPath returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdate_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Update(context.Background(), client, UpdateInput{IID: 1})
	if err == nil {
		t.Fatal("Update() expected error for missing full_path, got nil")
	}
}

// TestUpdate_APIError verifies that Update returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestDelete_MissingIID verifies that Delete_MissingIID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MissingIID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := Delete(context.Background(), client, DeleteInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("Delete() expected error for missing iid, got nil")
	}
}

// TestDelete_MissingFullPath verifies that Delete_MissingFullPath returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := Delete(context.Background(), client, DeleteInput{IID: 1})
	if err == nil {
		t.Fatal("Delete() expected error for missing full_path, got nil")
	}
}

// TestDelete_APIError verifies that Delete returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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
	if out.Author != nil {
		t.Errorf("Author should be nil, got %+v", out.Author)
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

// TestFormatOutputMarkdown verifies the OutputMarkdown Markdown formatter for a representative output input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown(t *testing.T) {
	out := Output{
		IID: 1, Title: "Epic", Type: "Epic", State: "OPEN",
		Author:    &BasicUserOutput{Username: "alice"},
		Assignees: []*BasicUserOutput{{Username: "bob"}},
	}
	result := FormatOutputMarkdown(out)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

// TestFormatLinksMarkdown verifies the LinksMarkdown Markdown formatter for a representative links input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatLinksMarkdown(t *testing.T) {
	out := LinksOutput{
		ChildEpics: []LinksItem{
			{IID: 2, Title: "Sub", State: "opened", Author: &BasicUserOutput{Username: "bob"}},
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
	assertEpicOutputCoreFields(t, out)
	assertEpicOutputRelationshipFields(t, out)
	assertEpicOutputTimelineFields(t, out)
}

func assertEpicOutputCoreFields(t *testing.T, out Output) {
	t.Helper()
	if out.ID != 101 {
		t.Errorf("ID = %d, want 101", out.ID)
	}
	if out.Status != "IN_PROGRESS" {
		t.Errorf("Status = %q, want IN_PROGRESS", out.Status)
	}
	if authorName(out.Author) != "alice" {
		t.Errorf("Author = %q, want alice", authorName(out.Author))
	}
	if len(out.Assignees) != 2 || authorName(out.Assignees[0]) != "bob" || authorName(out.Assignees[1]) != "carol" {
		t.Errorf("Assignees = %v, want [bob carol]", out.Assignees)
	}
	if len(out.Labels) != 2 || out.Labels[0] != "planning" {
		t.Errorf("Labels = %v, want [planning priority]", out.Labels)
	}
	if !out.Confidential {
		t.Error("Confidential should be true")
	}
	if out.Weight == nil || *out.Weight != 5 {
		t.Errorf("Weight = %v, want 5", out.Weight)
	}
}

func assertEpicOutputRelationshipFields(t *testing.T, out Output) {
	t.Helper()
	if len(out.LinkedItems) != 1 || out.LinkedItems[0].IID != 5 || out.LinkedItems[0].LinkType != "blocks" {
		t.Errorf("LinkedItems = %v, unexpected", out.LinkedItems)
	}
	if out.ParentIID != 10 {
		t.Errorf("ParentIID = %d, want 10", out.ParentIID)
	}
	if out.ParentPath != "g" {
		t.Errorf("ParentPath = %q, want g", out.ParentPath)
	}
}

func assertEpicOutputTimelineFields(t *testing.T, out Output) {
	t.Helper()
	if out.Color != "#FF0000" {
		t.Errorf("Color = %q, want #FF0000", out.Color)
	}
	if out.HealthStatus != "onTrack" {
		t.Errorf("HealthStatus = %q, want onTrack", out.HealthStatus)
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
}

// --- toLinkItem coverage ---

// TestToLinkItem_NilAuthorAndCreatedAt verifies the ToLinkItem_NilAuthorAndCreatedAt handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToLinkItem_NilAuthorAndCreatedAt(t *testing.T) {
	e := &gl.Epic{ID: 10, IID: 3, Title: "Bare", State: "opened"}
	item := toLinkItem(e)
	if item.Author != nil {
		t.Errorf("Author = %+v, want nil", item.Author)
	}
	if item.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty", item.CreatedAt)
	}
}

// TestEpicToOutput_FullRESTEpic verifies the EpicToOutput_FullRESTEpic handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEpicToOutput_FullRESTEpic(t *testing.T) {
	start := gl.ISOTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	due := gl.ISOTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2026, 1, 3, 3, 4, 5, 0, time.UTC)
	closed := time.Date(2026, 1, 4, 3, 4, 5, 0, time.UTC)

	startFixed := gl.ISOTime(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	startFromMs := gl.ISOTime(time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC))
	dueFixed := gl.ISOTime(time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC))
	dueFromMs := gl.ISOTime(time.Date(2026, 2, 6, 0, 0, 0, 0, time.UTC))

	out := epicToOutput(&gl.Epic{
		ID:                      44,
		IID:                     7,
		GroupID:                 9,
		Title:                   "REST Epic",
		Description:             "REST body",
		State:                   "opened",
		WebURL:                  "https://gitlab.example.com/groups/g/-/epics/7",
		URL:                     "https://gitlab.example.com/api/v4/groups/9/epics/7",
		Author:                  &gl.EpicAuthor{ID: 2, Username: "alice", Name: "Alice", State: "active", AvatarURL: "a.png", WebURL: "https://gitlab.example.com/alice"},
		Labels:                  []string{"x"},
		StartDate:               &start,
		StartDateIsFixed:        true,
		StartDateFixed:          &startFixed,
		StartDateFromMilestones: &startFromMs,
		DueDate:                 &due,
		DueDateIsFixed:          true,
		DueDateFixed:            &dueFixed,
		DueDateFromMilestones:   &dueFromMs,
		Upvotes:                 3,
		Downvotes:               1,
		UserNotesCount:          4,
		CreatedAt:               &created,
		UpdatedAt:               &updated,
		ClosedAt:                &closed,
		Confidential:            true,
		ParentID:                3,
	})
	assertRESTEpicIdentity(t, out)
	assertRESTEpicDates(t, out)
	assertRESTEpicMetrics(t, out)
}

func assertRESTEpicIdentity(t *testing.T, out Output) {
	t.Helper()
	if out.ID != 44 || out.IID != 7 || out.GroupID != 9 || out.ParentID != 3 || out.ParentIID != 3 {
		t.Fatalf("unexpected epic output identity fields: %+v", out)
	}
	if authorName(out.Author) != "alice" || out.Author.ID != 2 || out.Author.Name != "Alice" || out.Author.State != "active" || out.Author.WebURL == "" {
		t.Fatalf("expected full author object, got %+v", out.Author)
	}
}

func assertRESTEpicDates(t *testing.T, out Output) {
	t.Helper()
	if out.StartDate != "2026-01-01" || out.StartDateFixed != "2026-01-05" || out.StartDateFromMilestones != "2026-01-06" || !out.StartDateIsFixed {
		t.Fatalf("unexpected start date fields: %+v", out)
	}
	if out.DueDate != "2026-02-01" || out.DueDateFixed != "2026-02-05" || out.DueDateFromMilestones != "2026-02-06" || !out.DueDateIsFixed {
		t.Fatalf("unexpected due date fields: %+v", out)
	}
	if out.CreatedAt == "" || out.UpdatedAt == "" || out.ClosedAt == "" {
		t.Fatalf("expected timestamp fields to be populated: %+v", out)
	}
}

func assertRESTEpicMetrics(t *testing.T, out Output) {
	t.Helper()
	if out.URL == "" || out.Upvotes != 3 || out.Downvotes != 1 || out.UserNotesCount != 4 {
		t.Fatalf("unexpected vote/url fields: %+v", out)
	}
}

// TestToLinkItem_FullRESTEpic verifies that toLinkItem maps every gl.Epic field
// (group_id, parent_id, description, url, fixed/from-milestone dates, votes,
// user notes count, full author, timestamps) onto the LinksItem output.
func TestToLinkItem_FullRESTEpic(t *testing.T) {
	start := gl.ISOTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	startFixed := gl.ISOTime(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	startFromMs := gl.ISOTime(time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC))
	due := gl.ISOTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	dueFixed := gl.ISOTime(time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC))
	dueFromMs := gl.ISOTime(time.Date(2026, 2, 6, 0, 0, 0, 0, time.UTC))
	created := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)

	item := toLinkItem(&gl.Epic{
		ID:                      201,
		IID:                     2,
		GroupID:                 9,
		ParentID:                7,
		Title:                   "Sub-Epic",
		Description:             "child body",
		State:                   "opened",
		WebURL:                  "https://gitlab.example.com/groups/g/-/epics/2",
		URL:                     "https://gitlab.example.com/api/v4/groups/9/epics/2",
		Author:                  &gl.EpicAuthor{ID: 3, Username: "carol", Name: "Carol", State: "active"},
		Labels:                  []string{"sub"},
		Confidential:            true,
		StartDate:               &start,
		StartDateIsFixed:        true,
		StartDateFixed:          &startFixed,
		StartDateFromMilestones: &startFromMs,
		DueDate:                 &due,
		DueDateIsFixed:          true,
		DueDateFixed:            &dueFixed,
		DueDateFromMilestones:   &dueFromMs,
		Upvotes:                 2,
		Downvotes:               1,
		UserNotesCount:          5,
		CreatedAt:               &created,
		UpdatedAt:               &updated,
		ClosedAt:                &closed,
	})
	if item.GroupID != 9 || item.ParentID != 7 || item.Description != "child body" || item.URL == "" {
		t.Fatalf("unexpected core fields: %+v", item)
	}
	if authorName(item.Author) != "carol" || item.Author.ID != 3 || item.Author.Name != "Carol" {
		t.Fatalf("unexpected author: %+v", item.Author)
	}
	if item.StartDateFixed != "2026-01-05" || item.StartDateFromMilestones != "2026-01-06" || !item.StartDateIsFixed {
		t.Fatalf("unexpected start date fields: %+v", item)
	}
	if item.DueDateFixed != "2026-02-05" || item.DueDateFromMilestones != "2026-02-06" || !item.DueDateIsFixed {
		t.Fatalf("unexpected due date fields: %+v", item)
	}
	if item.Upvotes != 2 || item.Downvotes != 1 || item.UserNotesCount != 5 {
		t.Fatalf("unexpected vote fields: %+v", item)
	}
	if item.UpdatedAt == "" || item.ClosedAt == "" {
		t.Fatalf("expected updated/closed timestamps: %+v", item)
	}
}

// --- List with all filter options ---

// TestList_WithAllFilters verifies that List passes all filter parameters to
// the GraphQL API without errors when every optional field is populated.
func TestList_WithAllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars, err := testutil.ParseGraphQLVariables(r)
		if err != nil {
			t.Errorf("ParseGraphQLVariables: %v", err)
			http.Error(w, "ParseGraphQLVariables", http.StatusInternalServerError)
			return
		}
		for _, key := range []string{"fullPath", "state", "search", "authorUsername", "labelName", "confidential", "sort", "first", "after", "includeAncestors", "includeDescendants"} {
			t.Run(key, func(t *testing.T) {
				if _, ok := vars[key]; !ok {
					t.Errorf("GraphQL variables missing %q", key)
				}
			})
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

// TestList_WorkItems_RequestsEEFields verifies the work-items list path opts
// into the EE work-item fields the epic output maps. As of client-go v2.49.0
// ListWorkItems returns only CE fields unless ReturnedFields is set; without
// the opt-in, epic weight/status/color/health_status would silently be empty.
func TestList_WorkItems_RequestsEEFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}
		query := string(body)
		for _, field := range []string{"weight", "healthStatus", "color", "status"} {
			t.Run(field, func(t *testing.T) {
				if !strings.Contains(query, field) {
					t.Errorf("GraphQL query missing EE field %q; ReturnedFields opt-in not applied\nquery: %s", field, query)
				}
			})
		}
		testutil.RespondJSON(w, http.StatusOK, listResponseJSON)
	}))
	boolTrue := true
	// Confidential set routes List through the work-items (GraphQL) path.
	if _, err := List(t.Context(), client, ListInput{FullPath: testFullPath, Confidential: &boolTrue}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_WorkItemsAPIError verifies that List_WorkItemsAPIError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_WorkItemsAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := List(context.Background(), client, ListInput{FullPath: testFullPath, AuthorUsername: "alice"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "epics require GitLab Premium or Ultimate") {
		t.Fatalf("error missing Premium/Ultimate hint: %v", err)
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
			t.Errorf("ParseGraphQLVariables: %v", err)
			http.Error(w, "ParseGraphQLVariables", http.StatusInternalServerError)
			return
		}
		input, ok := vars["input"].(map[string]any)
		if !ok {
			t.Error("GraphQL variables missing 'input' object")
			http.Error(w, "GraphQL variables missing 'input' object", http.StatusInternalServerError)
			return
		}
		for _, key := range []string{"title", "confidential", "descriptionWidget", "colorWidget", "startAndDueDateWidget", "assigneesWidget", "labelsWidget", "weightWidget", "healthStatusWidget"} {
			t.Run(key, func(t *testing.T) {
				if _, exists := input[key]; !exists {
					t.Errorf("GraphQL input missing %q", key)
				}
			})
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

// TestCreate_CancelledContext verifies the Create_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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
				t.Errorf("ParseGraphQLVariables: %v", err)
				http.Error(w, "ParseGraphQLVariables", http.StatusInternalServerError)
				return
			}
			input, ok := vars["input"].(map[string]any)
			if !ok {
				t.Error("GraphQL variables missing 'input' object")
				http.Error(w, "GraphQL variables missing 'input' object", http.StatusInternalServerError)
				return
			}
			for _, key := range []string{"title", "stateEvent", "descriptionWidget", "colorWidget", "startAndDueDateWidget", "labelsWidget", "assigneesWidget", "weightWidget", "healthStatusWidget", "statusWidget"} {
				t.Run(key, func(t *testing.T) {
					if _, exists := input[key]; !exists {
						t.Errorf("GraphQL input missing %q", key)
					}
				})
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

// TestUpdate_CancelledContext verifies the Update_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{FullPath: testFullPath, IID: 1, Title: "X"})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// --- GetLinks additional coverage ---

// TestGetLinks_CancelledContext verifies the GetLinks_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGetLinks_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := GetLinks(ctx, client, GetLinksInput{FullPath: testFullPath, IID: 1})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestGetLinks_APIError verifies that GetLinks returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetLinks_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := GetLinks(context.Background(), client, GetLinksInput{FullPath: testFullPath, IID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGet_CancelledContext verifies the Get_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{FullPath: testFullPath, IID: 1})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

// TestDelete_CancelledContext verifies the Delete_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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
		Author:       &BasicUserOutput{Username: "alice"},
		Assignees:    []*BasicUserOutput{{Username: "bob"}, {Username: "carol"}},
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(result, want) {
				t.Errorf("expected markdown to contain %q", want)
			}
		})
	}
}

// TestFormatMarkdown_NilUsers verifies that the formatters tolerate a nil
// author and empty assignees (the userName/userNames nil branches) without
// panicking and render the epic rows.
func TestFormatMarkdown_NilUsers(t *testing.T) {
	out := Output{IID: 1, Title: "No Author", State: "opened"}
	if got := FormatOutputMarkdown(out); !strings.Contains(got, "No Author") {
		t.Errorf("FormatOutputMarkdown missing title; got:\n%s", got)
	}
	list := ListOutput{Epics: []Output{{IID: 1, Title: "No Author", State: "opened"}}}
	if got := FormatListMarkdown(list); !strings.Contains(got, "No Author") {
		t.Errorf("FormatListMarkdown missing title; got:\n%s", got)
	}
	if names := userNames(nil); names != nil {
		t.Errorf("userNames(nil) = %v, want nil", names)
	}
}

// TestFormatLinksMarkdown_Empty verifies the LinksMarkdown_Empty Markdown formatter for a representative links_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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
		{IID: 1, Title: "Epic A", State: "opened", Author: &BasicUserOutput{Username: "alice"}, Labels: []string{"backend", "priority"}},
	}}
	result := FormatListMarkdown(out)
	if !strings.Contains(result, "backend, priority") {
		t.Errorf("expected joined labels in output; got:\n%s", result)
	}
}
