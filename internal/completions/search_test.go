// search_test.go contains unit tests for the GitLab API search functions
// (projects, groups, users, merge requests, issues, branches, tags).
// Tests verify successful searches, empty queries, API errors, and context
// cancellation using httptest mocks.
package completions

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// Shared test assertion messages and subtest names for search tests.
const (
	msgExpectedAPIErr    = "expected error on API failure"
	fmtUnexpectedValue0  = "unexpected value[0]: %s"
	fmtExpected3Values   = "expected 3 values, got %d"
	subtestEmptyQueryAll = "empty query returns all"
)

// TestSearchProjects verifies that [searchProjects] returns formatted project
// entries matching the given query.
func TestSearchProjects(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"path_with_namespace":"group/alpha"},
				{"id":2,"path_with_namespace":"group/beta"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchProjects(context.Background(), client, "group")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
	if values[0] != "group/alpha" {
		t.Errorf(fmtUnexpectedValue0, values[0])
	}
}

// TestSearchProjects_EmptyQuery verifies that [searchProjects] omits the search
// parameter when the query is empty.
func TestSearchProjects_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects" {
			if r.URL.Query().Get("search") != "" {
				t.Errorf("expected no search param for empty query, got %q", r.URL.Query().Get("search"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"path_with_namespace":"team/repo"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchProjects(context.Background(), client, "")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 1 {
		t.Fatalf(fmtExpected1Value, len(values))
	}
}

// TestSearchProjects_APIError verifies that [searchProjects] returns an error
// when the GitLab API responds with a failure status.
func TestSearchProjects_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchProjects(context.Background(), client, "test")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchGroups verifies that [searchGroups] returns formatted group entries
// matching the given query.
func TestSearchGroups(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"full_path":"engineering/platform"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchGroups(context.Background(), client, "eng")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 1 {
		t.Fatalf(fmtExpected1Value, len(values))
	}
	if values[0] != "engineering/platform" {
		t.Errorf("unexpected value: %s", values[0])
	}
}

// TestSearchGroups_APIError verifies that [searchGroups] returns an error when
// the GitLab API responds with a failure status.
func TestSearchGroups_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchGroups(context.Background(), client, "test")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchUsers verifies that [searchUsers] returns matching usernames.
func TestSearchUsers(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"username":"alice"},{"id":2,"username":"alicia"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchUsers(context.Background(), client, "ali")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
	if values[0] != "alice" {
		t.Errorf(fmtUnexpectedValue0, values[0])
	}
}

// TestSearchUsers_APIError verifies that [searchUsers] returns an error when
// the GitLab API responds with a failure status.
func TestSearchUsers_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchUsers(context.Background(), client, "test")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchMRs verifies that [searchMRs] returns merge request entries
// filtered by IID prefix, using subtests for prefix match and unfiltered queries.
func TestSearchMRs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"iid":1,"title":"Fix critical bug"},
				{"iid":12,"title":"Add documentation"},
				{"iid":23,"title":"Refactor auth"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	t.Run("filter by IID prefix", func(t *testing.T) {
		values, err := searchMRs(context.Background(), client, "42", "1")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 2 {
			t.Fatalf("expected 2 values matching prefix '1', got %d: %v", len(values), values)
		}
	})

	t.Run(subtestEmptyQueryAll, func(t *testing.T) {
		values, err := searchMRs(context.Background(), client, "42", "")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 3 {
			t.Fatalf(fmtExpected3Values, len(values))
		}
	})
}

// TestSearchMRs_APIError verifies that [searchMRs] returns an error when the
// GitLab API responds with a failure status.
func TestSearchMRs_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := searchMRs(context.Background(), client, "42", "1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchIssues verifies that [searchIssues] returns issue entries filtered
// by IID prefix, using subtests for matching and non-matching queries.
func TestSearchIssues(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/issues" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":100,"iid":5,"title":"Login broken"},
				{"id":101,"iid":50,"title":"Performance issue"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	t.Run("filter by IID prefix", func(t *testing.T) {
		values, err := searchIssues(context.Background(), client, "42", "5")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 2 {
			t.Fatalf("expected 2 values matching '5', got %d: %v", len(values), values)
		}
	})

	t.Run("no match", func(t *testing.T) {
		values, err := searchIssues(context.Background(), client, "42", "9")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 0 {
			t.Errorf("expected 0 values, got %d", len(values))
		}
	})
}

// TestSearchIssues_APIError verifies that [searchIssues] returns an error when
// the GitLab API responds with a failure status.
func TestSearchIssues_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := searchIssues(context.Background(), client, "42", "1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchBranches verifies that [searchBranches] returns branch names
// matching the given query.
func TestSearchBranches(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/branches" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"name":"main","default":true},
				{"name":"feature/auth","default":false}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchBranches(context.Background(), client, "42", "main")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values (search is server-side), got %d", len(values))
	}
	if values[0] != "main" {
		t.Errorf(fmtUnexpectedValue0, values[0])
	}
}

// TestSearchBranches_EmptyQuery verifies that [searchBranches] omits the search
// parameter when the query is empty.
func TestSearchBranches_EmptyQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/branches" {
			if r.URL.Query().Get("search") != "" {
				t.Errorf("expected no search param for empty query")
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"main","default":true}]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchBranches(context.Background(), client, "42", "")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 1 {
		t.Fatalf(fmtExpected1Value, len(values))
	}
}

// TestSearchBranches_APIError verifies that [searchBranches] returns an error
// when the GitLab API responds with a failure status.
func TestSearchBranches_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchBranches(context.Background(), client, "42", "main")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchTags verifies that [searchTags] returns tag names matching the
// given query.
func TestSearchTags(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/tags" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"name":"v1.0.0"},
				{"name":"v1.1.0"},
				{"name":"v2.0.0"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchTags(context.Background(), client, "42", "v1")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 3 {
		t.Fatalf("expected 3 values (search is server-side), got %d", len(values))
	}
}

// TestSearchTags_APIError verifies that [searchTags] returns an error when the
// GitLab API responds with a failure status.
func TestSearchTags_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchTags(context.Background(), client, "42", "v1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearch_ContextCancelled uses table-driven subtests to verify that all
// search functions return a context cancellation error when given a canceled
// context.
func TestSearch_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	ctx := testutil.CancelledCtx(t)

	tests := []struct {
		name string
		fn   func() error
	}{
		{"projects", func() error { _, _, err := searchProjects(ctx, client, "x"); return err }},
		{"groups", func() error { _, _, err := searchGroups(ctx, client, "x"); return err }},
		{"users", func() error { _, _, err := searchUsers(ctx, client, "x"); return err }},
		{"mrs", func() error { _, err := searchMRs(ctx, client, "42", "x"); return err }},
		{"issues", func() error { _, err := searchIssues(ctx, client, "42", "x"); return err }},
		{"branches", func() error { _, _, err := searchBranches(ctx, client, "42", "x"); return err }},
		{"tags", func() error { _, _, err := searchTags(ctx, client, "42", "x"); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Error("expected context cancellation error")
			} else if !strings.Contains(err.Error(), "context canceled") {
				t.Errorf("expected context canceled error, got: %v", err)
			}
		})
	}
}

// TestSearchPipelines verifies that [searchPipelines] returns pipeline entries
// filtered by ID prefix.
func TestSearchPipelines(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/pipelines" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":100,"ref":"main","status":"success"},
				{"id":101,"ref":"develop","status":"running"},
				{"id":23,"ref":"feature","status":"failed"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	t.Run("filter by ID prefix", func(t *testing.T) {
		values, err := searchPipelines(context.Background(), client, "42", "10")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 2 {
			t.Fatalf("expected 2 values matching prefix '10', got %d: %v", len(values), values)
		}
		if values[0] != "100" {
			t.Errorf(fmtUnexpectedValue0, values[0])
		}
	})

	t.Run(subtestEmptyQueryAll, func(t *testing.T) {
		values, err := searchPipelines(context.Background(), client, "42", "")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 3 {
			t.Fatalf(fmtExpected3Values, len(values))
		}
	})
}

// TestSearchPipelines_APIError verifies that [searchPipelines] returns an error
// when the GitLab API responds with a failure status.
func TestSearchPipelines_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := searchPipelines(context.Background(), client, "42", "1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchCommits verifies that [searchCommits] returns commit entries
// filtered by SHA prefix.
func TestSearchCommits(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/commits" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":"abc123def456","short_id":"abc123d","title":"Fix login bug"},
				{"id":"def789abc012","short_id":"def789a","title":"Add tests"},
				{"id":"abc999aaa111","short_id":"abc999a","title":"Update docs"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	t.Run("filter by short SHA prefix", func(t *testing.T) {
		values, err := searchCommits(context.Background(), client, "42", "abc")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 2 {
			t.Fatalf("expected 2 values matching prefix 'abc', got %d: %v", len(values), values)
		}
		if values[0] != "abc123d" {
			t.Errorf(fmtUnexpectedValue0, values[0])
		}
	})

	t.Run(subtestEmptyQueryAll, func(t *testing.T) {
		values, err := searchCommits(context.Background(), client, "42", "")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 3 {
			t.Fatalf(fmtExpected3Values, len(values))
		}
	})
}

// TestSearchCommits_APIError verifies that [searchCommits] returns an error
// when the GitLab API responds with a failure status.
func TestSearchCommits_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := searchCommits(context.Background(), client, "42", "abc")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchLabels verifies that [searchLabels] returns label names matching
// the query.
func TestSearchLabels(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/labels" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"name":"bug","color":"#d9534f"},
				{"id":2,"name":"enhancement","color":"#5cb85c"},
				{"id":3,"name":"documentation","color":"#0275d8"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	t.Run("search with query", func(t *testing.T) {
		values, _, err := searchLabels(context.Background(), client, "42", "bug")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		// Server-side search: mock returns all regardless, so we get 3
		if len(values) != 3 {
			t.Fatalf("expected 3 values (server-side search), got %d: %v", len(values), values)
		}
		if values[0] != "bug" {
			t.Errorf(fmtUnexpectedValue0, values[0])
		}
	})

	t.Run("empty query", func(t *testing.T) {
		values, _, err := searchLabels(context.Background(), client, "42", "")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 3 {
			t.Fatalf(fmtExpected3Values, len(values))
		}
	})
}

// TestSearchLabels_APIError verifies that [searchLabels] returns an error when
// the GitLab API responds with a failure status.
func TestSearchLabels_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchLabels(context.Background(), client, "42", "bug")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchMilestones verifies that [searchMilestones] returns milestone
// entries matching the query.
func TestSearchMilestones(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/milestones" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"title":"v1.0","state":"active"},
				{"id":2,"title":"v2.0","state":"active"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, _, err := searchMilestones(context.Background(), client, "42", "v1")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values (server-side search), got %d: %v", len(values), values)
	}
	if values[0] != "1" {
		t.Errorf(fmtUnexpectedValue0, values[0])
	}
}

// TestSearchMilestones_APIError verifies that [searchMilestones] returns an
// error when the GitLab API responds with a failure status.
func TestSearchMilestones_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, _, err := searchMilestones(context.Background(), client, "42", "v1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchJobs verifies that [searchJobs] returns job entries for a pipeline,
// filtered by ID prefix.
func TestSearchJobs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/pipelines/10/jobs" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":501,"name":"build","status":"success","pipeline":{"id":10}},
				{"id":502,"name":"test","status":"running","pipeline":{"id":10}},
				{"id":601,"name":"deploy","status":"pending","pipeline":{"id":10}}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	t.Run("filter by ID prefix", func(t *testing.T) {
		values, err := searchJobs(context.Background(), client, "42", 10, "50")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 2 {
			t.Fatalf("expected 2 values matching prefix '50', got %d: %v", len(values), values)
		}
		if values[0] != "501" {
			t.Errorf(fmtUnexpectedValue0, values[0])
		}
	})

	t.Run(subtestEmptyQueryAll, func(t *testing.T) {
		values, err := searchJobs(context.Background(), client, "42", 10, "")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if len(values) != 3 {
			t.Fatalf(fmtExpected3Values, len(values))
		}
	})
}

// TestSearchJobs_APIError verifies that [searchJobs] returns an error when the
// GitLab API responds with a failure status.
func TestSearchJobs_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := searchJobs(context.Background(), client, "42", 10, "1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchNew_ContextCancelled uses table-driven subtests to verify that
// the new search functions return a context cancellation error.
func TestSearchNew_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	ctx := testutil.CancelledCtx(t)

	tests := []struct {
		name string
		fn   func() error
	}{
		{"pipelines", func() error { _, err := searchPipelines(ctx, client, "42", "x"); return err }},
		{"commits", func() error { _, err := searchCommits(ctx, client, "42", "x"); return err }},
		{"labels", func() error { _, _, err := searchLabels(ctx, client, "42", "x"); return err }},
		{"milestones", func() error { _, _, err := searchMilestones(ctx, client, "42", "x"); return err }},
		{"jobs", func() error { _, err := searchJobs(ctx, client, "42", 10, "x"); return err }},
		{"milestone titles", func() error { _, _, err := searchMilestoneTitles(ctx, client, "42", "x"); return err }},
		{"group milestone titles", func() error { _, _, err := searchGroupMilestoneTitles(ctx, client, "99", "x"); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Error("expected context cancellation error")
			} else if !strings.Contains(err.Error(), "context canceled") {
				t.Errorf("expected context canceled error, got: %v", err)
			}
		})
	}
}

// TestSearchMilestoneTitles verifies that [searchMilestoneTitles] returns
// plain milestone titles (not "id: title") and that the query parameter is
// forwarded to GitLab as the Search filter. The mock asserts the request URL
// includes ?search=v1, then returns a single milestone whose title is read
// from the response body.
func TestSearchMilestoneTitles(t *testing.T) {
	var gotSearch string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/milestones" {
			http.NotFound(w, r)
			return
		}
		gotSearch = r.URL.Query().Get("search")
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":1,"title":"v1.0","state":"active"},
			{"id":2,"title":"v1.1","state":"active"}
		]`)
	}))

	values, _, err := searchMilestoneTitles(context.Background(), client, "42", "v1")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if gotSearch != "v1" {
		t.Errorf("query param 'search' = %q, want %q (query branch not exercised)", gotSearch, "v1")
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 titles, got %d: %v", len(values), values)
	}
	if values[0] != "v1.0" || values[1] != "v1.1" {
		t.Errorf("titles = %v, want [v1.0 v1.1]", values)
	}
}

// TestSearchMilestoneTitles_EmptyQuery verifies that when query is empty,
// the Search option is NOT set on the request (covers the false branch of
// `if query != ""`).
func TestSearchMilestoneTitles_EmptyQuery(t *testing.T) {
	var hadSearch bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/milestones" {
			http.NotFound(w, r)
			return
		}
		_, hadSearch = r.URL.Query()["search"]
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, _, err := searchMilestoneTitles(context.Background(), client, "42", "")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if hadSearch {
		t.Error("expected no 'search' query param when query is empty")
	}
}

// TestSearchMilestoneTitles_APIError verifies that an error from the GitLab
// API is wrapped and returned. Uses 403 (not 5xx) to avoid client-go retries.
func TestSearchMilestoneTitles_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, _, err := searchMilestoneTitles(context.Background(), client, "42", "v1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
	if !strings.Contains(err.Error(), "search milestone titles") {
		t.Errorf("expected error to be wrapped with 'search milestone titles', got: %v", err)
	}
}

// TestSearchGroupMilestoneTitles verifies group milestone title search with
// and without a query filter.
func TestSearchGroupMilestoneTitles(t *testing.T) {
	t.Run("search with query", func(t *testing.T) {
		var gotSearch string
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v4/groups/99/milestones" {
				http.NotFound(w, r)
				return
			}
			gotSearch = r.URL.Query().Get("search")
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"title":"v1.0","state":"active"},
				{"id":2,"title":"v1.1","state":"active"}
			]`)
		}))

		values, _, err := searchGroupMilestoneTitles(context.Background(), client, "99", "v1")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if gotSearch != "v1" {
			t.Errorf("query param 'search' = %q, want %q", gotSearch, "v1")
		}
		if len(values) != 2 {
			t.Fatalf("expected 2 titles, got %d: %v", len(values), values)
		}
		if values[0] != "v1.0" || values[1] != "v1.1" {
			t.Errorf("titles = %v, want [v1.0 v1.1]", values)
		}
	})

	t.Run("empty query", func(t *testing.T) {
		var hadSearch bool
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v4/groups/99/milestones" {
				http.NotFound(w, r)
				return
			}
			_, hadSearch = r.URL.Query()["search"]
			testutil.RespondJSON(w, http.StatusOK, `[]`)
		}))

		_, _, err := searchGroupMilestoneTitles(context.Background(), client, "99", "")
		if err != nil {
			t.Fatalf(fmtUnexpectedErr, err)
		}
		if hadSearch {
			t.Error("expected no 'search' query param when query is empty")
		}
	})
}

// TestSearchGroupMilestoneTitles_APIError verifies group milestone API errors
// are wrapped with search context.
func TestSearchGroupMilestoneTitles_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, _, err := searchGroupMilestoneTitles(context.Background(), client, "99", "v1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
	if !strings.Contains(err.Error(), "search group milestone titles") {
		t.Errorf("expected error to be wrapped with 'search group milestone titles', got: %v", err)
	}
}

// TestSearch_PropagatesXTotalHeader exercises every server-side-filtered
// search helper and confirms that the X-Total header is captured into the
// returned total. Mocked endpoints all return 1 row but advertise a much
// larger total so the assertion is unambiguous.
func TestSearch_PropagatesXTotalHeader(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		body    string
		invoke  func(client *testClientType) (int, error)
		wantTot int
	}{
		{
			name:    "projects",
			path:    "/api/v4/projects",
			body:    `[{"id":1,"path_with_namespace":"a/b"}]`,
			wantTot: 250,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchProjects(context.Background(), c, "x")
				return total, err
			},
		},
		{
			name:    "groups",
			path:    "/api/v4/groups",
			body:    `[{"id":1,"full_path":"a"}]`,
			wantTot: 99,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchGroups(context.Background(), c, "x")
				return total, err
			},
		},
		{
			name:    "users",
			path:    "/api/v4/users",
			body:    `[{"id":1,"username":"alice"}]`,
			wantTot: 47,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchUsers(context.Background(), c, "ali")
				return total, err
			},
		},
		{
			name:    "branches",
			path:    "/api/v4/projects/42/repository/branches",
			body:    `[{"name":"main"}]`,
			wantTot: 12,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchBranches(context.Background(), c, "42", "")
				return total, err
			},
		},
		{
			name:    "tags",
			path:    "/api/v4/projects/42/repository/tags",
			body:    `[{"name":"v1"}]`,
			wantTot: 8,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchTags(context.Background(), c, "42", "")
				return total, err
			},
		},
		{
			name:    "labels",
			path:    "/api/v4/projects/42/labels",
			body:    `[{"name":"bug"}]`,
			wantTot: 33,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchLabels(context.Background(), c, "42", "")
				return total, err
			},
		},
		{
			name:    "milestones",
			path:    "/api/v4/projects/42/milestones",
			body:    `[{"id":1,"title":"v1"}]`,
			wantTot: 5,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchMilestones(context.Background(), c, "42", "")
				return total, err
			},
		},
		{
			name:    "milestone titles",
			path:    "/api/v4/projects/42/milestones",
			body:    `[{"id":1,"title":"v1"}]`,
			wantTot: 5,
			invoke: func(c *testClientType) (int, error) {
				_, total, err := searchMilestoneTitles(context.Background(), c, "42", "")
				return total, err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("X-Total", itoa(tc.wantTot))
				testutil.RespondJSON(w, http.StatusOK, tc.body)
			})
			client := testutil.NewTestClient(t, handler)
			got, err := tc.invoke(client)
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if got != tc.wantTot {
				t.Errorf("total = %d, want %d (X-Total propagation broken)", got, tc.wantTot)
			}
		})
	}
}

// TestTotalFromResponse_NilSafe documents that totalFromResponse returns 0
// for a nil *gitlab.Response (defensive: the helper is called from search
// fast-paths that may not always have a response object).
func TestTotalFromResponse_NilSafe(t *testing.T) {
	if got := totalFromResponse(nil); got != 0 {
		t.Errorf("totalFromResponse(nil) = %d, want 0", got)
	}
}

// itoa is an inline minimal int→ascii helper to avoid pulling strconv into
// this test file's already-broad assertion surface.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// testClientType is a type alias used by the tests above so the table can
// declare invoke closures without naming the gitlabclient package directly.
type testClientType = gitlabclient.Client
