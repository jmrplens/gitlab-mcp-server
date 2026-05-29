// milestones_test.go contains unit tests for GitLab milestone listing operations.
// Tests use httptest to mock the GitLab Milestones API.
package milestones

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// errExpectedNil identifies the err expected nil constant used by this package.
	errExpectedNil = "expected error, got nil"
	// pathProjectMilestones identifies the path project milestones constant used by this package.
	pathProjectMilestones = "/api/v4/projects/42/milestones"
	// fmtMilestoneListErr identifies the fmt milestone list err constant used by this package.
	fmtMilestoneListErr = "milestoneList() unexpected error: %v"
)

// TestMilestoneList_Success verifies MilestoneList when success.
func TestMilestoneList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{
					"id":1,
					"iid":1,
					"project_id":42,
					"title":"v1.0",
					"description":"First release",
					"state":"active",
					"start_date":"2026-01-01",
					"due_date":"2026-03-31",
					"web_url":"https://gitlab.example.com/mygroup/api/-/milestones/1",
					"created_at":"2026-01-01T00:00:00Z",
					"updated_at":"2026-01-15T10:00:00Z",
					"expired":false
				},
				{
					"id":2,
					"iid":2,
					"project_id":42,
					"title":"v2.0",
					"description":"Second release",
					"state":"closed",
					"web_url":"https://gitlab.example.com/mygroup/api/-/milestones/2",
					"created_at":"2026-02-01T00:00:00Z",
					"updated_at":"2026-02-28T10:00:00Z",
					"expired":true
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
		t.Fatalf(fmtMilestoneListErr, err)
	}
	if len(out.Milestones) != 2 {
		t.Fatalf("len(Milestones) = %d, want 2", len(out.Milestones))
	}
	if out.Milestones[0].Title != "v1.0" {
		t.Errorf("Milestones[0].Title = %q, want %q", out.Milestones[0].Title, "v1.0")
	}
	if out.Milestones[0].State != "active" {
		t.Errorf("Milestones[0].State = %q, want %q", out.Milestones[0].State, "active")
	}
	if out.Milestones[1].Expired != true {
		t.Errorf("Milestones[1].Expired = %v, want true", out.Milestones[1].Expired)
	}
	if out.Milestones[0].WebURL == "" {
		t.Error("Milestones[0].WebURL is empty")
	}
}

// TestMilestoneList_WithStateFilter verifies MilestoneList when with state filter.
func TestMilestoneList_WithStateFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones {
			q := r.URL.Query()
			if q.Get("state") != "active" {
				t.Errorf("expected state=active, got %q", q.Get("state"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0","state":"active","web_url":"https://gitlab.example.com/-/milestones/1"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		State:     "active",
	})
	if err != nil {
		t.Fatalf(fmtMilestoneListErr, err)
	}
	if len(out.Milestones) != 1 {
		t.Fatalf("len(Milestones) = %d, want 1", len(out.Milestones))
	}
}

// TestMilestoneList_WithSearch verifies MilestoneList when with search.
func TestMilestoneList_WithSearch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones {
			q := r.URL.Query()
			if q.Get("search") != "v1" {
				t.Errorf("expected search=v1, got %q", q.Get("search"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0","state":"active","web_url":"https://gitlab.example.com/-/milestones/1"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		Search:    "v1",
	})
	if err != nil {
		t.Fatalf(fmtMilestoneListErr, err)
	}
	if len(out.Milestones) != 1 {
		t.Fatalf("len(Milestones) = %d, want 1", len(out.Milestones))
	}
}

// TestMilestoneList_EmptyProjectID verifies MilestoneList when empty project ID.
func TestMilestoneList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("milestoneList() expected error for empty project_id, got nil")
	}
}

// TestMilestoneListServer_Error verifies MilestoneListServer when error.
func TestMilestoneListServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Internal Server Error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
	})
	if err == nil {
		t.Fatal("milestoneList() expected error, got nil")
	}
}

// ---------- Get ----------.

// TestMilestoneGet_Success verifies MilestoneGet when success.
func TestMilestoneGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones+"/1" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"iid":1,"project_id":42,"title":"v1.0",
				"description":"First release","state":"active",
				"start_date":"2026-01-01","due_date":"2026-03-31",
				"web_url":"https://gitlab.example.com/-/milestones/1",
				"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-15T10:00:00Z",
				"expired":false
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", MilestoneIID: 1})
	if err != nil {
		t.Fatalf("milestoneGet() unexpected error: %v", err)
	}
	if out.Title != "v1.0" {
		t.Errorf("Title = %q, want %q", out.Title, "v1.0")
	}
	if out.State != "active" {
		t.Errorf("State = %q, want %q", out.State, "active")
	}
}

// TestMilestoneGet_MissingParams verifies MilestoneGet when missing params.
func TestMilestoneGet_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	if _, err := Get(context.Background(), client, GetInput{}); err == nil {
		t.Fatal("expected error for empty project_id")
	}
	if _, err := Get(context.Background(), client, GetInput{ProjectID: "42"}); err == nil {
		t.Fatal("expected error for zero milestone_id")
	}
}

// TestMilestoneGetServer_Error verifies MilestoneGetServer when error.
func TestMilestoneGetServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------- Create ----------.

// TestMilestoneCreate_Success verifies MilestoneCreate when success.
func TestMilestoneCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjectMilestones {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":3,"iid":3,"project_id":42,"title":"v3.0",
				"description":"Third release","state":"active",
				"start_date":"2026-06-01","due_date":"2026-09-30",
				"web_url":"https://gitlab.example.com/-/milestones/3",
				"created_at":"2026-06-01T00:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   "42",
		Title:       "v3.0",
		Description: "Third release",
		StartDate:   "2026-06-01",
		DueDate:     "2026-09-30",
	})
	if err != nil {
		t.Fatalf("milestoneCreate() unexpected error: %v", err)
	}
	if out.Title != "v3.0" {
		t.Errorf("Title = %q, want %q", out.Title, "v3.0")
	}
	if out.StartDate != "2026-06-01" {
		t.Errorf("StartDate = %q, want %q", out.StartDate, "2026-06-01")
	}
}

// TestMilestoneCreate_MissingParams verifies MilestoneCreate when missing params.
func TestMilestoneCreate_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	if _, err := Create(context.Background(), client, CreateInput{}); err == nil {
		t.Fatal("expected error for empty project_id")
	}
	if _, err := Create(context.Background(), client, CreateInput{ProjectID: "42"}); err == nil {
		t.Fatal("expected error for empty title")
	}
}

// TestMilestoneCreate_InvalidDate verifies MilestoneCreate when invalid date.
func TestMilestoneCreate_InvalidDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Title: "v1.0", StartDate: "bad-date"})
	if err == nil {
		t.Fatal("expected error for invalid start_date")
	}
	_, err = Create(context.Background(), client, CreateInput{ProjectID: "42", Title: "v1.0", DueDate: "bad-date"})
	if err == nil {
		t.Fatal("expected error for invalid due_date")
	}
}

// ---------- Update ----------.

// TestMilestoneUpdate_Success verifies MilestoneUpdate when success.
func TestMilestoneUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.Method == http.MethodPut && r.URL.Path == pathProjectMilestones+"/1" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"iid":1,"project_id":42,"title":"v1.1",
				"description":"Updated","state":"active",
				"web_url":"https://gitlab.example.com/-/milestones/1",
				"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-05-01T10:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:    "42",
		MilestoneIID: 1,
		Title:        "v1.1",
		Description:  "Updated",
	})
	if err != nil {
		t.Fatalf("milestoneUpdate() unexpected error: %v", err)
	}
	if out.Title != "v1.1" {
		t.Errorf("Title = %q, want %q", out.Title, "v1.1")
	}
}

// TestMilestoneUpdate_MissingParams verifies MilestoneUpdate when missing params.
func TestMilestoneUpdate_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	if _, err := Update(context.Background(), client, UpdateInput{}); err == nil {
		t.Fatal("expected error for empty project_id")
	}
	if _, err := Update(context.Background(), client, UpdateInput{ProjectID: "42"}); err == nil {
		t.Fatal("expected error for zero milestone_id")
	}
}

// TestMilestoneUpdate_InvalidDate verifies MilestoneUpdate when invalid date.
func TestMilestoneUpdate_InvalidDate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathProjectMilestones, func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
	})
	client := testutil.NewTestClient(t, mux)

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", MilestoneIID: 1, StartDate: "bad"})
	if err == nil {
		t.Fatal("expected error for invalid start_date")
	}
	_, err = Update(context.Background(), client, UpdateInput{ProjectID: "42", MilestoneIID: 1, DueDate: "bad"})
	if err == nil {
		t.Fatal("expected error for invalid due_date")
	}
}

// TestMilestoneUpdate_APIError verifies that Update wraps API errors.
func TestMilestoneUpdate_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathProjectMilestones, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1}]`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc(pathProjectMilestones+"/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, mux)

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", MilestoneIID: 1, Title: "new"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestMilestoneUpdate_WithDates verifies valid date options are sent on update.
func TestMilestoneUpdate_WithDates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathProjectMilestones, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1}]`)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc(pathProjectMilestones+"/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"project_id":42,"title":"v1.0"}`)
	})
	client := testutil.NewTestClient(t, mux)

	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42", MilestoneIID: 1, StartDate: "2026-01-01", DueDate: "2026-02-01",
	})
	if err != nil {
		t.Fatalf("milestoneUpdate() unexpected error: %v", err)
	}
}

// TestMilestoneUpdate_ResolveError verifies that Update handles resolveIID failure.
func TestMilestoneUpdate_ResolveError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", MilestoneIID: 99})
	if err == nil {
		t.Fatal("expected error when resolveIID fails")
	}
}

// TestMilestoneCreate_BadRequest verifies that Create returns a hint on 400 errors.
func TestMilestoneCreate_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Title has already been taken"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Title: "v1.0"})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "check that the title is unique") {
		t.Errorf("expected hint in error, got: %v", err)
	}
}

// TestMilestoneGet_ResolveNotFound verifies Get returns an error when the milestone IID doesn't exist.
func TestMilestoneGet_ResolveNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", MilestoneIID: 999})
	if err == nil {
		t.Fatal("expected error for not-found IID")
	}
}

// TestMilestoneDelete_ResolveError verifies Delete returns error when resolve fails.
func TestMilestoneDelete_ResolveError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", MilestoneIID: 999})
	if err == nil {
		t.Fatal("expected error for not-found IID")
	}
}

// TestMilestoneGetIssues_ResolveError verifies GetIssues returns error when resolve fails.
func TestMilestoneGetIssues_ResolveError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := GetIssues(context.Background(), client, GetIssuesInput{ProjectID: "42", MilestoneIID: 999})
	if err == nil {
		t.Fatal("expected error for not-found IID")
	}
}

// TestMilestoneGetMergeRequests_ResolveError verifies GetMergeRequests returns error when resolve fails.
func TestMilestoneGetMergeRequests_ResolveError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{ProjectID: "42", MilestoneIID: 999})
	if err == nil {
		t.Fatal("expected error for not-found IID")
	}
}

// TestMilestoneResolvedAPIErrors covers API errors after resolving milestone IID to global ID.
func TestMilestoneResolvedAPIErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathProjectMilestones, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1}]`)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc(pathProjectMilestones+"/1", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
		case http.MethodDelete:
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc(pathProjectMilestones+"/1/issues", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	})
	mux.HandleFunc(pathProjectMilestones+"/1/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	})
	client := testutil.NewTestClient(t, mux)

	tests := []struct {
		name string
		call func(context.Context) error
	}{
		{"Get", func(ctx context.Context) error {
			_, err := Get(ctx, client, GetInput{ProjectID: "42", MilestoneIID: 1})
			return err
		}},
		{"Delete", func(ctx context.Context) error {
			return Delete(ctx, client, DeleteInput{ProjectID: "42", MilestoneIID: 1})
		}},
		{"GetIssues", func(ctx context.Context) error {
			_, err := GetIssues(ctx, client, GetIssuesInput{ProjectID: "42", MilestoneIID: 1})
			return err
		}},
		{"GetMergeRequests", func(ctx context.Context) error {
			_, err := GetMergeRequests(ctx, client, GetMergeRequestsInput{ProjectID: "42", MilestoneIID: 1})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(t.Context()); err == nil {
				t.Fatal("expected API error")
			}
		})
	}
}

// ---------- Delete ----------.

// TestMilestoneDelete_Success verifies MilestoneDelete when success.
func TestMilestoneDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.Method == http.MethodDelete && r.URL.Path == pathProjectMilestones+"/1" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", MilestoneIID: 1})
	if err != nil {
		t.Fatalf("milestoneDelete() unexpected error: %v", err)
	}
}

// TestMilestoneDelete_MissingParams verifies MilestoneDelete when missing params.
func TestMilestoneDelete_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	if err := Delete(context.Background(), client, DeleteInput{}); err == nil {
		t.Fatal("expected error for empty project_id")
	}
	if err := Delete(context.Background(), client, DeleteInput{ProjectID: "42"}); err == nil {
		t.Fatal("expected error for zero milestone_id")
	}
}

// TestMilestoneDeleteServer_Error verifies MilestoneDeleteServer when error.
func TestMilestoneDeleteServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------- GetIssues ----------.

// TestMilestoneGetIssues_Success verifies MilestoneGetIssues when success.
func TestMilestoneGetIssues_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones+"/1/issues" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":10,"iid":1,"title":"Bug fix","state":"opened","web_url":"https://gitlab.example.com/-/issues/1","created_at":"2026-01-05T00:00:00Z"},
				{"id":11,"iid":2,"title":"Feature","state":"closed","web_url":"https://gitlab.example.com/-/issues/2","created_at":"2026-01-06T00:00:00Z"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetIssues(context.Background(), client, GetIssuesInput{ProjectID: "42", MilestoneIID: 1})
	if err != nil {
		t.Fatalf("milestoneGetIssues() unexpected error: %v", err)
	}
	if len(out.Issues) != 2 {
		t.Fatalf("len(Issues) = %d, want 2", len(out.Issues))
	}
	if out.Issues[0].Title != "Bug fix" {
		t.Errorf("Issues[0].Title = %q, want %q", out.Issues[0].Title, "Bug fix")
	}
	if out.Issues[1].State != "closed" {
		t.Errorf("Issues[1].State = %q, want %q", out.Issues[1].State, "closed")
	}
}

// TestMilestoneGetIssues_MissingParams verifies MilestoneGetIssues when missing params.
func TestMilestoneGetIssues_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	if _, err := GetIssues(context.Background(), client, GetIssuesInput{}); err == nil {
		t.Fatal("expected error for empty project_id")
	}
	if _, err := GetIssues(context.Background(), client, GetIssuesInput{ProjectID: "42"}); err == nil {
		t.Fatal("expected error for zero milestone_id")
	}
}

// TestMilestoneGetIssuesServer_Error verifies MilestoneGetIssuesServer when error.
func TestMilestoneGetIssuesServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))

	_, err := GetIssues(context.Background(), client, GetIssuesInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------- GetMergeRequests ----------.

// TestMilestoneGetMergeRequests_Success verifies MilestoneGetMergeRequests when success.
func TestMilestoneGetMergeRequests_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones+"/1/merge_requests" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":20,"iid":1,"title":"Add feature X","state":"merged","source_branch":"feature-x","target_branch":"main","web_url":"https://gitlab.example.com/-/merge_requests/1","created_at":"2026-02-01T00:00:00Z"},
				{"id":21,"iid":2,"title":"Fix bug Y","state":"opened","source_branch":"fix-y","target_branch":"main","web_url":"https://gitlab.example.com/-/merge_requests/2","created_at":"2026-02-02T00:00:00Z"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{ProjectID: "42", MilestoneIID: 1})
	if err != nil {
		t.Fatalf("milestoneGetMergeRequests() unexpected error: %v", err)
	}
	if len(out.MergeRequests) != 2 {
		t.Fatalf("len(MergeRequests) = %d, want 2", len(out.MergeRequests))
	}
	if out.MergeRequests[0].Title != "Add feature X" {
		t.Errorf("MergeRequests[0].Title = %q, want %q", out.MergeRequests[0].Title, "Add feature X")
	}
	if out.MergeRequests[0].SourceBranch != "feature-x" {
		t.Errorf("MergeRequests[0].SourceBranch = %q, want %q", out.MergeRequests[0].SourceBranch, "feature-x")
	}
	if out.MergeRequests[1].State != "opened" {
		t.Errorf("MergeRequests[1].State = %q, want %q", out.MergeRequests[1].State, "opened")
	}
}

// TestMilestoneGetMergeRequests_MissingParams verifies MilestoneGetMergeRequests when missing params.
func TestMilestoneGetMergeRequests_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	if _, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{}); err == nil {
		t.Fatal("expected error for empty project_id")
	}
	if _, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{ProjectID: "42"}); err == nil {
		t.Fatal("expected error for zero milestone_id")
	}
}

// TestMilestoneGetMergeRequestsServer_Error verifies MilestoneGetMergeRequestsServer when error.
func TestMilestoneGetMergeRequestsServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))

	_, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestMilestoneList_CancelledContext verifies MilestoneList when cancelled context.
func TestMilestoneList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("milestoneList() expected error for canceled context, got nil")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// JSON fixtures
// ---------------------------------------------------------------------------.

const (
	// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
	errExpCancelledCtx = "expected error for canceled context"
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// covMilestoneJSON identifies the cov milestone JSON constant used by this package.
	covMilestoneJSON = `{"id":1,"iid":1,"project_id":42,"title":"v1.0","description":"First release","state":"active","start_date":"2026-01-01","due_date":"2026-03-31","web_url":"https://gitlab.example.com/-/milestones/1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-15T10:00:00Z","expired":false}`
	// covMilestoneListJSON identifies the cov milestone list JSON constant used by this package.
	covMilestoneListJSON = `[` + covMilestoneJSON + `]`
	// covIssueJSON identifies the cov issue JSON constant used by this package.
	covIssueJSON = `[{"id":10,"iid":1,"title":"Bug","state":"opened","web_url":"https://example.com/issues/1","created_at":"2026-01-05T00:00:00Z"}]`
	// covIssueNoDateJSON identifies the cov issue no date JSON constant used by this package.
	covIssueNoDateJSON = `[{"id":11,"iid":2,"title":"Feature","state":"closed"}]`
	// covMRJSON identifies the cov mrjson constant used by this package.
	covMRJSON = `[{"id":20,"iid":1,"title":"Feature X","state":"merged","source_branch":"feat-x","target_branch":"main","web_url":"https://example.com/mr/1","created_at":"2026-02-01T00:00:00Z"}]`
	// covMRNoDateJSON identifies the cov MR no date JSON constant used by this package.
	covMRNoDateJSON = `[{"id":21,"iid":2,"title":"Fix Y","state":"opened","source_branch":"fix-y","target_branch":"main"}]`
)

// ---------------------------------------------------------------------------
// List — additional coverage
// ---------------------------------------------------------------------------.

// TestList_IncludeAncestors verifies List when include ancestors.
func TestList_IncludeAncestors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("include_ancestors") != "true" {
			t.Errorf("expected include_ancestors=true, got %q", r.URL.Query().Get("include_ancestors"))
		}
		testutil.RespondJSON(w, http.StatusOK, covMilestoneListJSON)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", IncludeAncestors: true})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_WithIIDs verifies List when with ii ds.
func TestList_WithIIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		q := r.URL.Query()
		iids := q["iids[]"]
		if len(iids) != 2 || iids[0] != "1" || iids[1] != "2" {
			t.Errorf("expected iids[]=[1,2], got %v", iids)
		}
		testutil.RespondJSON(w, http.StatusOK, covMilestoneListJSON)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", IIDs: []int64{1, 2}})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_WithTitleFilter verifies List when with title filter.
func TestList_WithTitleFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("title") != "v1.0" {
			t.Errorf("expected title=v1.0, got %q", r.URL.Query().Get("title"))
		}
		testutil.RespondJSON(w, http.StatusOK, covMilestoneListJSON)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", Title: "v1.0"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_WithPagination verifies List when with pagination.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("expected page=2, got %q", r.URL.Query().Get("page"))
		}
		if r.URL.Query().Get("per_page") != "5" {
			t.Errorf("expected per_page=5, got %q", r.URL.Query().Get("per_page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, covMilestoneListJSON,
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
// Get — canceled context
// ---------------------------------------------------------------------------.

// TestGet_CancelledContext verifies Get when cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Create — server error, canceled context
// ---------------------------------------------------------------------------.

// TestCreate_ServerError verifies Create when server error.
func TestCreate_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Title: "v1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestCreate_CancelledContext verifies Create when cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", Title: "v1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Update — state_event, server error, canceled context
// ---------------------------------------------------------------------------.

// TestUpdate_WithStateEvent verifies Update when with state event.
func TestUpdate_WithStateEvent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, covMilestoneJSON)
	}))
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42", MilestoneIID: 1, StateEvent: "close",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestUpdate_ServerError verifies Update when server error.
func TestUpdate_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Server Error"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", MilestoneIID: 1, Title: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestUpdate_CancelledContext verifies Update when cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", MilestoneIID: 1, Title: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Delete — canceled context
// ---------------------------------------------------------------------------.

// TestDelete_CancelledContext verifies Delete when cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// GetIssues — canceled context, pagination, no created_at
// ---------------------------------------------------------------------------.

// TestGetIssues_CancelledContext verifies GetIssues when cancelled context.
func TestGetIssues_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetIssues(ctx, client, GetIssuesInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestGetIssues_WithPagination verifies GetIssues when with pagination.
func TestGetIssues_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("expected page=2, got %q", r.URL.Query().Get("page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, covIssueJSON,
			testutil.PaginationHeaders{Page: "2", PerPage: "10", Total: "1", TotalPages: "1"})
	}))
	out, err := GetIssues(context.Background(), client, GetIssuesInput{
		ProjectID:       "42",
		MilestoneIID:    1,
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("expected page 2, got %d", out.Pagination.Page)
	}
}

// TestGetIssues_NoCreatedAt verifies GetIssues when no created at.
func TestGetIssues_NoCreatedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, covIssueNoDateJSON)
	}))
	out, err := GetIssues(context.Background(), client, GetIssuesInput{ProjectID: "42", MilestoneIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Issues[0].CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", out.Issues[0].CreatedAt)
	}
}

// ---------------------------------------------------------------------------
// GetMergeRequests — canceled context, pagination, no created_at
// ---------------------------------------------------------------------------.

// TestGetMergeRequests_CancelledContext verifies GetMergeRequests when cancelled context.
func TestGetMergeRequests_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetMergeRequests(ctx, client, GetMergeRequestsInput{ProjectID: "42", MilestoneIID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestGetMergeRequests_WithPagination verifies GetMergeRequests when with pagination.
func TestGetMergeRequests_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		if r.URL.Query().Get("page") != "3" {
			t.Errorf("expected page=3, got %q", r.URL.Query().Get("page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, covMRJSON,
			testutil.PaginationHeaders{Page: "3", PerPage: "5", Total: "1", TotalPages: "1"})
	}))
	out, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{
		ProjectID:       "42",
		MilestoneIID:    1,
		PaginationInput: toolutil.PaginationInput{Page: 3, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 3 {
		t.Errorf("expected page 3, got %d", out.Pagination.Page)
	}
}

// TestGetMergeRequests_NoCreatedAt verifies GetMergeRequests when no created at.
func TestGetMergeRequests_NoCreatedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectMilestones && r.URL.Query().Get("iids[]") != "" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"title":"v1.0"}]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, covMRNoDateJSON)
	}))
	out, err := GetMergeRequests(context.Background(), client, GetMergeRequestsInput{ProjectID: "42", MilestoneIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.MergeRequests[0].CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", out.MergeRequests[0].CreatedAt)
	}
}

// ---------------------------------------------------------------------------
// Formatters — additional coverage
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_AllFields verifies FormatMarkdown when all fields.
func TestFormatMarkdown_AllFields(t *testing.T) {
	o := Output{
		ID: 1, IID: 1, ProjectID: 42, Title: "v1.0", Description: "First release",
		State: "active", StartDate: "2026-01-01", DueDate: "2026-03-31",
		WebURL: "https://example.com/-/milestones/1", CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-15T10:00:00Z", Expired: false,
	}
	md := FormatMarkdown(o)
	for _, want := range []string{"v1.0", "active", "First release", "Start Date", "Due Date", "Created", "Updated", "URL"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatMarkdown missing %q in:\n%s", want, md)
		}
	}
}

// TestFormatMarkdown_Minimal verifies FormatMarkdown when minimal.
func TestFormatMarkdown_Minimal(t *testing.T) {
	o := Output{ID: 2, IID: 2, Title: "v2.0", State: "closed"}
	md := FormatMarkdown(o)
	if strings.Contains(md, "Start Date") {
		t.Error("minimal milestone should not show Start Date")
	}
	if strings.Contains(md, "Due Date") {
		t.Error("minimal milestone should not show Due Date")
	}
	if strings.Contains(md, "Description") {
		t.Error("minimal milestone should not show Description")
	}
	if !strings.Contains(md, "v2.0") {
		t.Error("missing milestone title")
	}
}

// TestFormatListMarkdownString_Empty verifies FormatListMarkdownString when empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(md, "No milestones found") {
		t.Errorf("expected 'No milestones found', got:\n%s", md)
	}
}

// TestFormatListMarkdownString_WithExpired verifies FormatListMarkdownString when with expired.
func TestFormatListMarkdownString_WithExpired(t *testing.T) {
	out := ListOutput{
		Milestones: []Output{
			{IID: 1, Title: "v1.0", State: "active", DueDate: "2026-03-31", Expired: true},
			{IID: 2, Title: "v2.0", State: "active"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	}
	md := FormatListMarkdownString(out)
	if !strings.Contains(md, "Yes") {
		t.Errorf("expected 'Yes' for expired, got:\n%s", md)
	}
	if !strings.Contains(md, "No") {
		t.Errorf("expected 'No' for not expired, got:\n%s", md)
	}
	if !strings.Contains(md, "| IID |") {
		t.Errorf("missing table header:\n%s", md)
	}
}

// TestFormatListMarkdown verifies FormatListMarkdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Milestones: []Output{{IID: 1, Title: "v1.0", State: "active"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal("result is nil")
	}
}

// TestFormatListMarkdownString_ClickableLinks verifies that milestone IIDs
// in the list are rendered as clickable Markdown links [IID](weburl).
func TestFormatListMarkdownString_ClickableLinks(t *testing.T) {
	out := ListOutput{
		Milestones: []Output{
			{
				IID: 3, Title: "v3.0", State: "active",
				WebURL: "https://gitlab.example.com/-/milestones/3",
			},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	md := FormatListMarkdownString(out)
	if !strings.Contains(md, "[3](https://gitlab.example.com/-/milestones/3)") {
		t.Errorf("expected clickable milestone link, got:\n%s", md)
	}
}

// TestFormatMarkdown_ClickableURL verifies that milestone detail renders
// the URL as a clickable Markdown link [url](url).
func TestFormatMarkdown_ClickableURL(t *testing.T) {
	md := FormatMarkdown(Output{
		ID: 1, IID: 1, Title: "v1.0", State: "active",
		WebURL: "https://gitlab.example.com/-/milestones/1",
	})
	if !strings.Contains(md, "[https://gitlab.example.com/-/milestones/1](https://gitlab.example.com/-/milestones/1)") {
		t.Errorf("expected clickable URL in detail, got:\n%s", md)
	}
}

// TestFormatMarkdown_NoURLWhenEmpty verifies that no URL line appears
// when WebURL is empty.
func TestFormatMarkdown_NoURLWhenEmpty(t *testing.T) {
	md := FormatMarkdown(Output{ID: 1, IID: 1, Title: "v1.0", State: "active"})
	if strings.Contains(md, "**URL**") {
		t.Errorf("expected no URL line when WebURL is empty, got:\n%s", md)
	}
}

// TestFormatIssuesMarkdownString_Empty verifies FormatIssuesMarkdownString when empty.
func TestFormatIssuesMarkdownString_Empty(t *testing.T) {
	md := FormatIssuesMarkdownString(MilestoneIssuesOutput{})
	if !strings.Contains(md, "No issues found") {
		t.Errorf("expected 'No issues found', got:\n%s", md)
	}
}

// TestFormatIssuesMarkdownString_WithIssues verifies FormatIssuesMarkdownString when with issues.
func TestFormatIssuesMarkdownString_WithIssues(t *testing.T) {
	out := MilestoneIssuesOutput{
		Issues: []IssueItem{
			{IID: 1, Title: "Bug fix", State: "opened", CreatedAt: "2026-01-05T00:00:00Z"},
			{IID: 2, Title: "Feature", State: "closed"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	}
	md := FormatIssuesMarkdownString(out)
	if !strings.Contains(md, "Bug fix") {
		t.Errorf("missing issue title:\n%s", md)
	}
	if !strings.Contains(md, "| IID |") {
		t.Errorf("missing table header:\n%s", md)
	}
}

// TestFormatIssuesMarkdown verifies FormatIssuesMarkdown.
func TestFormatIssuesMarkdown(t *testing.T) {
	out := MilestoneIssuesOutput{
		Issues:     []IssueItem{{IID: 1, Title: "x", State: "opened"}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	result := FormatIssuesMarkdown(out)
	if result == nil {
		t.Fatal("result is nil")
	}
}

// TestFormatMergeRequestsMarkdownString_Empty verifies FormatMergeRequestsMarkdownString when empty.
func TestFormatMergeRequestsMarkdownString_Empty(t *testing.T) {
	md := FormatMergeRequestsMarkdownString(MilestoneMergeRequestsOutput{})
	if !strings.Contains(md, "No merge requests found") {
		t.Errorf("expected 'No merge requests found', got:\n%s", md)
	}
}

// TestFormatMergeRequestsMarkdownString_WithMRs verifies FormatMergeRequestsMarkdownString when with MRs.
func TestFormatMergeRequestsMarkdownString_WithMRs(t *testing.T) {
	out := MilestoneMergeRequestsOutput{
		MergeRequests: []MergeRequestItem{
			{IID: 1, Title: "Feature X", State: "merged", SourceBranch: "feat-x", TargetBranch: "main", CreatedAt: "2026-02-01T00:00:00Z"},
			{IID: 2, Title: "Fix Y", State: "opened", SourceBranch: "fix-y", TargetBranch: "main"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	}
	md := FormatMergeRequestsMarkdownString(out)
	if !strings.Contains(md, "Feature X") {
		t.Errorf("missing MR title:\n%s", md)
	}
	if !strings.Contains(md, "feat-x") {
		t.Errorf("missing source branch:\n%s", md)
	}
	if !strings.Contains(md, "| IID |") {
		t.Errorf("missing table header:\n%s", md)
	}
}

// TestFormatMergeRequestsMarkdown verifies FormatMergeRequestsMarkdown.
func TestFormatMergeRequestsMarkdown(t *testing.T) {
	out := MilestoneMergeRequestsOutput{
		MergeRequests: []MergeRequestItem{{IID: 1, Title: "x", State: "merged", SourceBranch: "a", TargetBranch: "main"}},
		Pagination:    toolutil.PaginationOutput{TotalItems: 1},
	}
	result := FormatMergeRequestsMarkdown(out)
	if result == nil {
		t.Fatal("result is nil")
	}
}

// TestFormatMilestoneNotFound verifies not-found result formatting for milestones.
func TestFormatMilestoneNotFound(t *testing.T) {
	result := formatMilestoneNotFound(milestoneNotFoundOutput{Identifier: "IID 9 in project 42"})
	if result == nil || !result.IsError {
		t.Fatalf("formatMilestoneNotFound() = %+v, want error result", result)
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for milestone actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	specs := ActionSpecs(client)
	byTool := milestoneSpecsByTool(t, specs)

	if len(specs) != 7 {
		t.Fatalf("len(ActionSpecs) = %d, want 7", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "milestones" {
			t.Fatalf("OwnerPackage for %s = %q, want milestones", spec.Name, spec.OwnerPackage)
		}
	}

	list := byTool["gitlab_milestone_list"]
	if list.Usage == "" || len(list.Aliases) == 0 {
		t.Fatalf("gitlab_milestone_list metadata incomplete: usage=%q aliases=%d", list.Usage, len(list.Aliases))
	}

	get := byTool["gitlab_milestone_get"]
	if get.Usage == "" || len(get.Aliases) == 0 || get.ParameterGuidance["milestone_iid"].SemanticRole == "" {
		t.Fatalf("gitlab_milestone_get metadata incomplete: usage=%q aliases=%d guidance(milestone_iid)=%q", get.Usage, len(get.Aliases), get.ParameterGuidance["milestone_iid"].SemanticRole)
	}

	create := byTool["gitlab_milestone_create"]
	if create.Usage == "" || len(create.Aliases) == 0 {
		t.Fatalf("gitlab_milestone_create metadata incomplete: usage=%q aliases=%d", create.Usage, len(create.Aliases))
	}
}

// TestActionSpecs_CallAllRoutes validates milestone routes across multiple scenarios.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	const milestonePath = "/api/v4/projects/42/milestones"

	mux := http.NewServeMux()
	mux.HandleFunc(milestonePath, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSONWithPagination(w, http.StatusOK, covMilestoneListJSON,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, covMilestoneJSON)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc(milestonePath+"/1", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, covMilestoneJSON)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, covMilestoneJSON)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc(milestonePath+"/1/issues", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, covIssueJSON,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})
	mux.HandleFunc(milestonePath+"/1/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, covMRJSON,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})

	client := testutil.NewTestClient(t, mux)
	byTool := milestoneSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_milestone_list", map[string]any{"project_id": "42"}},
		{"gitlab_milestone_get", map[string]any{"project_id": "42", "milestone_iid": float64(1)}},
		{"gitlab_milestone_create", map[string]any{"project_id": "42", "title": "v1.0"}},
		{"gitlab_milestone_update", map[string]any{"project_id": "42", "milestone_iid": float64(1), "title": "v1.1"}},
		{"gitlab_milestone_delete", map[string]any{"project_id": "42", "milestone_iid": float64(1)}},
		{"gitlab_milestone_issues", map[string]any{"project_id": "42", "milestone_iid": float64(1)}},
		{"gitlab_milestone_merge_requests", map[string]any{"project_id": "42", "milestone_iid": float64(1)}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			result, err := byTool[tc.name].Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s): %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s): nil result", tc.name)
			}
		})
	}
}

// TestActionSpecs_MilestoneGetRoute verifies the canonical milestone get route output.
func TestActionSpecs_MilestoneGetRoute(t *testing.T) {
	const listJSON = `[{"id":99,"iid":3,"project_id":42,"title":"M3","description":"","state":"active"}]`
	const getJSON = `{"id":99,"iid":3,"project_id":42,"title":"M3","description":"","state":"active"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/milestones/99"):
			testutil.RespondJSON(w, http.StatusOK, getJSON)
		case strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/milestones"):
			testutil.RespondJSON(w, http.StatusOK, listJSON)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, handler)
	byTool := milestoneSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_milestone_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "milestone_iid": 3})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.IID != 3 || out.ID != 99 {
		t.Fatalf("milestone output = %#v, want IID 3 ID 99", out)
	}
}

// TestActionSpecs_MilestoneGetRouteNotFound verifies get route converts 404 into a not-found result.
func TestActionSpecs_MilestoneGetRouteNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	byTool := milestoneSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_milestone_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "milestone_iid": 9})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if _, ok := result.(milestoneNotFoundOutput); !ok {
		t.Fatalf("result type = %T, want milestoneNotFoundOutput", result)
	}
}

// milestoneSpecsByTool supports milestone specs by tool assertions in milestones tests.
func milestoneSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
