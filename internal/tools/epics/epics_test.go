package epics

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const (
	pathEpics    = "/api/v4/groups/mygroup/epics"
	pathEpicByID = "/api/v4/groups/mygroup/epics/1"

	epicJSON = `{
		"id": 101,
		"iid": 1,
		"group_id": 5,
		"parent_id": 0,
		"title": "Q1 Planning",
		"description": "Quarterly planning epic",
		"state": "opened",
		"confidential": false,
		"web_url": "https://gitlab.example.com/groups/mygroup/-/epics/1",
		"author": {"username": "alice"},
		"labels": ["planning", "q1"],
		"start_date": "2026-01-01",
		"due_date": "2026-03-31",
		"created_at": "2026-01-01T00:00:00Z",
		"updated_at": "2026-01-02T00:00:00Z",
		"upvotes": 3,
		"downvotes": 0,
		"user_notes_count": 5
	}`

	testGroupID  = "mygroup"
	fmtWantID    = "out.ID = %d, want 101"
	fmtWantTitle = "out.Title = %q, want %q"
)

func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathEpics {
			testutil.RespondJSON(w, http.StatusOK, "["+epicJSON+"]")
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: testGroupID})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Epics) != 1 {
		t.Fatalf("len(Epics) = %d, want 1", len(out.Epics))
	}
	if out.Epics[0].ID != 101 {
		t.Errorf(fmtWantID, out.Epics[0].ID)
	}
}

func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing group_id, got nil")
	}
}

func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := List(ctx, client, ListInput{GroupID: testGroupID})
	if err == nil {
		t.Fatal("List() expected context error, got nil")
	}
}

func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathEpicByID {
			testutil.RespondJSON(w, http.StatusOK, epicJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: testGroupID, EpicIID: 1})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.ID != 101 {
		t.Errorf(fmtWantID, out.ID)
	}
	if out.Title != "Q1 Planning" {
		t.Errorf(fmtWantTitle, out.Title, "Q1 Planning")
	}
	if out.Author != "alice" {
		t.Errorf("out.Author = %q, want %q", out.Author, "alice")
	}
	if out.StartDate != "2026-01-01" {
		t.Errorf("out.StartDate = %q, want %q", out.StartDate, "2026-01-01")
	}
}

func TestGet_MissingEpicIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{GroupID: testGroupID})
	if err == nil {
		t.Fatal("Get() expected error for missing epic_iid, got nil")
	}
}

func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{GroupID: testGroupID, EpicIID: 999})
	if err == nil {
		t.Fatal("Get() expected error for 404, got nil")
	}
}

func TestGetLinks_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/mygroup/epics/1/epics" {
			testutil.RespondJSON(w, http.StatusOK, "["+epicJSON+"]")
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetLinks(context.Background(), client, GetLinksInput{GroupID: testGroupID, EpicIID: 1})
	if err != nil {
		t.Fatalf("GetLinks() error: %v", err)
	}
	if len(out.ChildEpics) != 1 {
		t.Fatalf("len(ChildEpics) = %d, want 1", len(out.ChildEpics))
	}
}

func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathEpics {
			testutil.RespondJSON(w, http.StatusCreated, epicJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		GroupID: testGroupID,
		Title:   "Q1 Planning",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if out.ID != 101 {
		t.Errorf(fmtWantID, out.ID)
	}
}

func TestCreate_MissingTitle(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{GroupID: testGroupID})
	if err == nil {
		t.Fatal("Create() expected error for missing title, got nil")
	}
}

func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathEpicByID {
			testutil.RespondJSON(w, http.StatusOK, epicJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		GroupID:    testGroupID,
		EpicIID:    1,
		Title:      "Updated Title",
		StateEvent: "close",
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if out.ID != 101 {
		t.Errorf(fmtWantID, out.ID)
	}
}

func TestUpdate_MissingEpicIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{GroupID: testGroupID})
	if err == nil {
		t.Fatal("Update() expected error for missing epic_iid, got nil")
	}
}

func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathEpicByID {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: testGroupID, EpicIID: 1})
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestDelete_MissingEpicIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := Delete(context.Background(), client, DeleteInput{GroupID: testGroupID})
	if err == nil {
		t.Fatal("Delete() expected error for missing epic_iid, got nil")
	}
}

func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{GroupID: testGroupID, EpicIID: 1})
	if err == nil {
		t.Fatal("Delete() expected error for 403, got nil")
	}
}

func TestSplitLabels(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"single", "bug", 1},
		{"multiple", "bug, feature, test", 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := splitLabels(tc.input)
			if tc.want == 0 {
				if result != nil {
					t.Errorf("splitLabels(%q) = non-nil, want nil", tc.input)
				}
				return
			}
			if result == nil {
				t.Fatalf("splitLabels(%q) = nil, want %d labels", tc.input, tc.want)
			}
			if len(*result) != tc.want {
				t.Errorf("splitLabels(%q) len = %d, want %d", tc.input, len(*result), tc.want)
			}
		})
	}
}
