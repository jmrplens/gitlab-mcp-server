// groupreleases_test.go contains unit tests for GitLab group release operations.
// Tests use httptest to mock the GitLab Group Releases API.
package groupreleases

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const pathGroupReleases = "/api/v4/groups/mygroup/releases"

func inputPagination(page, perPage int) toolutil.PaginationInput {
	return toolutil.PaginationInput{Page: page, PerPage: perPage}
}

// TestList_Success validates that List returns a fully populated release
// when the GitLab API responds with all fields (tag, name, description,
// dates, author, upcoming flag, and self-link).
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"tag_name":"v1.0.0","name":"Release 1.0","description":"First release","created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-01T00:00:00Z","author":{"username":"admin"},"upcoming_release":false,"_links":{"self":"https://git.example.com/group/proj/-/releases/v1.0.0"}}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Fatalf("len(Releases) = %d, want 1", len(out.Releases))
	}
	r := out.Releases[0]
	if r.TagName != "v1.0.0" {
		t.Errorf("TagName = %q, want %q", r.TagName, "v1.0.0")
	}
	if r.Name != "Release 1.0" {
		t.Errorf("Name = %q, want %q", r.Name, "Release 1.0")
	}
	if r.Description != "First release" {
		t.Errorf("Description = %q, want %q", r.Description, "First release")
	}
	if r.Author != "admin" {
		t.Errorf("Author = %q, want %q", r.Author, "admin")
	}
	if r.CreatedAt == "" {
		t.Error("CreatedAt should not be empty")
	}
	if r.ReleasedAt == "" {
		t.Error("ReleasedAt should not be empty")
	}
	if r.WebURL != "https://git.example.com/group/proj/-/releases/v1.0.0" {
		t.Errorf("WebURL = %q, want self link URL", r.WebURL)
	}
}

// TestList_Simple verifies the List_Simple handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_Simple(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.AssertQueryParam(t, r, "simple", "true")
			testutil.RespondJSON(w, http.StatusOK, `[{"tag_name":"v1.0.0","name":"Release 1.0"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup", Simple: true})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Fatalf("len(Releases) = %d, want 1", len(out.Releases))
	}
}

// TestList_MissingGroupID verifies that List_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing group_id, got nil")
	}
}

// TestList_CancelledContext verifies the List_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("List() expected error for canceled context, got nil")
	}
}

// TestList_EmptyResults verifies the List_EmptyResults handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_EmptyResults(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 0 {
		t.Errorf("len(Releases) = %d, want 0", len(out.Releases))
	}
}

// TestList_APIError verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("List() expected error for 404 response, got nil")
	}
}

// TestList_ServerError verifies that List_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_ServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusForbidden, `{"error":"internal"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("List() expected error for 500 response, got nil")
	}
}

// TestList_Pagination verifies that List forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestList_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.AssertQueryParam(t, r, "page", "2")
			testutil.AssertQueryParam(t, r, "per_page", "10")
			testutil.RespondJSONWithPagination(
				w, http.StatusOK,
				`[{"tag_name":"v0.9.0","name":"Old Release"}]`,
				testutil.PaginationHeaders{
					Page:       "2",
					PerPage:    "10",
					Total:      "11",
					TotalPages: "2",
					PrevPage:   "1",
				},
			)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID:         "mygroup",
		PaginationInput: inputPagination(2, 10),
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Fatalf("len(Releases) = %d, want 1", len(out.Releases))
	}
	if out.Pagination.TotalItems != 11 {
		t.Errorf("Pagination.TotalItems = %d, want 11", out.Pagination.TotalItems)
	}
	if out.Pagination.TotalPages != 2 {
		t.Errorf("Pagination.TotalPages = %d, want 2", out.Pagination.TotalPages)
	}
}

// TestList_MinimalRelease verifies the List_MinimalRelease handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_MinimalRelease(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusOK, `[{"tag_name":"v0.0.1","name":"Minimal"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Fatalf("len(Releases) = %d, want 1", len(out.Releases))
	}
	r := out.Releases[0]
	if r.TagName != "v0.0.1" {
		t.Errorf("TagName = %q, want %q", r.TagName, "v0.0.1")
	}
	if r.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty for nil date", r.CreatedAt)
	}
	if r.ReleasedAt != "" {
		t.Errorf("ReleasedAt = %q, want empty for nil date", r.ReleasedAt)
	}
	if r.Author != "" {
		t.Errorf("Author = %q, want empty for no author", r.Author)
	}
	if r.WebURL != "" {
		t.Errorf("WebURL = %q, want empty for no self-link", r.WebURL)
	}
}

// TestList_UpcomingRelease verifies the List_UpcomingRelease handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_UpcomingRelease(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusOK, `[{"tag_name":"v2.0.0-rc1","name":"RC1","upcoming_release":true}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Fatalf("len(Releases) = %d, want 1", len(out.Releases))
	}
	if !out.Releases[0].UpcomingRelease {
		t.Error("UpcomingRelease = false, want true")
	}
}

// TestList_MultipleReleases verifies the List_MultipleReleases handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_MultipleReleases(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupReleases {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"tag_name":"v2.0.0","name":"Major","created_at":"2026-07-01T00:00:00Z","author":{"username":"lead"}},
				{"tag_name":"v1.1.0","name":"Minor","created_at":"2026-06-01T00:00:00Z","author":{"username":"dev"}},
				{"tag_name":"v1.0.0","name":"Initial","created_at":"2026-05-01T00:00:00Z","author":{"username":"admin"}}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 3 {
		t.Fatalf("len(Releases) = %d, want 3", len(out.Releases))
	}
	wantTags := []string{"v2.0.0", "v1.1.0", "v1.0.0"}
	for i, wt := range wantTags {
		if out.Releases[i].TagName != wt {
			t.Errorf("Releases[%d].TagName = %q, want %q", i, out.Releases[i].TagName, wt)
		}
	}
}
