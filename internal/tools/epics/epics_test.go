package epics

import (
	"context"
	"net/http"
	"strings"
	"testing"

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

func TestList_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing full_path, got nil")
	}
}

func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := List(ctx, client, ListInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("List() expected context error, got nil")
	}
}

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

func TestGet_MissingIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := Get(context.Background(), client, GetInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("Get() expected error for missing iid, got nil")
	}
	if !strings.Contains(err.Error(), "iid") {
		t.Errorf("expected error to mention 'iid', got: %v", err)
	}
}

func TestGet_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := Get(context.Background(), client, GetInput{IID: 1})
	if err == nil {
		t.Fatal("Get() expected error for missing full_path, got nil")
	}
}

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

func TestGetLinks_MissingIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := GetLinks(context.Background(), client, GetLinksInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("GetLinks() expected error for missing iid, got nil")
	}
}

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

func TestCreate_MissingTitle(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := Create(context.Background(), client, CreateInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("Create() expected error for missing title, got nil")
	}
}

func TestCreate_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := Create(context.Background(), client, CreateInput{Title: "Some Title"})
	if err == nil {
		t.Fatal("Create() expected error for missing full_path, got nil")
	}
}

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

func TestUpdate_MissingIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := Update(context.Background(), client, UpdateInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("Update() expected error for missing iid, got nil")
	}
}

func TestUpdate_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := Update(context.Background(), client, UpdateInput{IID: 1})
	if err == nil {
		t.Fatal("Update() expected error for missing full_path, got nil")
	}
}

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

func TestDelete_MissingIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	err := Delete(context.Background(), client, DeleteInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal("Delete() expected error for missing iid, got nil")
	}
}

func TestDelete_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	err := Delete(context.Background(), client, DeleteInput{IID: 1})
	if err == nil {
		t.Fatal("Delete() expected error for missing full_path, got nil")
	}
}

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

func TestFormatOutputMarkdown(t *testing.T) {
	out := Output{
		IID: 1, Title: "Epic", Type: "Epic", State: "OPEN", Author: "alice",
	}
	result := FormatOutputMarkdown(out)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

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
