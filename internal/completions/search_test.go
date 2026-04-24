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

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects" {
			respondJSON(w, http.StatusOK, `[
				{"id":1,"path_with_namespace":"group/alpha"},
				{"id":2,"path_with_namespace":"group/beta"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, err := searchProjects(context.Background(), client, "group")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
	if values[0] != "1: group/alpha" {
		t.Errorf(fmtUnexpectedValue0, values[0])
	}
}

// TestSearchProjects_EmptyQuery verifies that [searchProjects] omits the search
// parameter when the query is empty.
func TestSearchProjects_EmptyQuery(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects" {
			if r.URL.Query().Get("search") != "" {
				t.Errorf("expected no search param for empty query, got %q", r.URL.Query().Get("search"))
			}
			respondJSON(w, http.StatusOK, `[{"id":1,"path_with_namespace":"team/repo"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, err := searchProjects(context.Background(), client, "")
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := searchProjects(context.Background(), client, "test")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchGroups verifies that [searchGroups] returns formatted group entries
// matching the given query.
func TestSearchGroups(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups" {
			respondJSON(w, http.StatusOK, `[{"id":10,"full_path":"engineering/platform"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, err := searchGroups(context.Background(), client, "eng")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 1 {
		t.Fatalf(fmtExpected1Value, len(values))
	}
	if values[0] != "10: engineering/platform" {
		t.Errorf("unexpected value: %s", values[0])
	}
}

// TestSearchGroups_APIError verifies that [searchGroups] returns an error when
// the GitLab API responds with a failure status.
func TestSearchGroups_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := searchGroups(context.Background(), client, "test")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchUsers verifies that [searchUsers] returns matching usernames.
func TestSearchUsers(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users" {
			respondJSON(w, http.StatusOK, `[{"id":1,"username":"alice"},{"id":2,"username":"alicia"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, err := searchUsers(context.Background(), client, "ali")
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := searchUsers(context.Background(), client, "test")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchMRs verifies that [searchMRs] returns merge request entries
// filtered by IID prefix, using subtests for prefix match and unfiltered queries.
func TestSearchMRs(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests" {
			respondJSON(w, http.StatusOK, `[
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/issues" {
			respondJSON(w, http.StatusOK, `[
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/branches" {
			respondJSON(w, http.StatusOK, `[
				{"name":"main","default":true},
				{"name":"feature/auth","default":false}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, err := searchBranches(context.Background(), client, "42", "main")
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/branches" {
			if r.URL.Query().Get("search") != "" {
				t.Errorf("expected no search param for empty query")
			}
			respondJSON(w, http.StatusOK, `[{"name":"main","default":true}]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, err := searchBranches(context.Background(), client, "42", "")
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := searchBranches(context.Background(), client, "42", "main")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchTags verifies that [searchTags] returns tag names matching the
// given query.
func TestSearchTags(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/tags" {
			respondJSON(w, http.StatusOK, `[
				{"name":"v1.0.0"},
				{"name":"v1.1.0"},
				{"name":"v2.0.0"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, err := searchTags(context.Background(), client, "42", "v1")
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := searchTags(context.Background(), client, "42", "v1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearch_ContextCancelled uses table-driven subtests to verify that all
// search functions return a context cancellation error when given a canceled
// context.
func TestSearch_ContextCancelled(t *testing.T) {
	client := newTestClient(t, http.NotFoundHandler())
	ctx := testutil.CancelledCtx(t)

	tests := []struct {
		name string
		fn   func() error
	}{
		{"projects", func() error { _, err := searchProjects(ctx, client, "x"); return err }},
		{"groups", func() error { _, err := searchGroups(ctx, client, "x"); return err }},
		{"users", func() error { _, err := searchUsers(ctx, client, "x"); return err }},
		{"mrs", func() error { _, err := searchMRs(ctx, client, "42", "x"); return err }},
		{"issues", func() error { _, err := searchIssues(ctx, client, "42", "x"); return err }},
		{"branches", func() error { _, err := searchBranches(ctx, client, "42", "x"); return err }},
		{"tags", func() error { _, err := searchTags(ctx, client, "42", "x"); return err }},
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/pipelines" {
			respondJSON(w, http.StatusOK, `[
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
		if values[0] != "100: main (success)" {
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/commits" {
			respondJSON(w, http.StatusOK, `[
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
		if values[0] != "abc123d: Fix login bug" {
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/labels" {
			respondJSON(w, http.StatusOK, `[
				{"id":1,"name":"bug","color":"#d9534f"},
				{"id":2,"name":"enhancement","color":"#5cb85c"},
				{"id":3,"name":"documentation","color":"#0275d8"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	t.Run("search with query", func(t *testing.T) {
		values, err := searchLabels(context.Background(), client, "42", "bug")
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
		values, err := searchLabels(context.Background(), client, "42", "")
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := searchLabels(context.Background(), client, "42", "bug")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchMilestones verifies that [searchMilestones] returns milestone
// entries matching the query.
func TestSearchMilestones(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/milestones" {
			respondJSON(w, http.StatusOK, `[
				{"id":1,"title":"v1.0","state":"active"},
				{"id":2,"title":"v2.0","state":"active"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	values, err := searchMilestones(context.Background(), client, "42", "v1")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values (server-side search), got %d: %v", len(values), values)
	}
	if values[0] != "1: v1.0" {
		t.Errorf(fmtUnexpectedValue0, values[0])
	}
}

// TestSearchMilestones_APIError verifies that [searchMilestones] returns an
// error when the GitLab API responds with a failure status.
func TestSearchMilestones_APIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := searchMilestones(context.Background(), client, "42", "v1")
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSearchJobs verifies that [searchJobs] returns job entries for a pipeline,
// filtered by ID prefix.
func TestSearchJobs(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/pipelines/10/jobs" {
			respondJSON(w, http.StatusOK, `[
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
		if values[0] != "501: build (success)" {
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
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	client := newTestClient(t, http.NotFoundHandler())
	ctx := testutil.CancelledCtx(t)

	tests := []struct {
		name string
		fn   func() error
	}{
		{"pipelines", func() error { _, err := searchPipelines(ctx, client, "42", "x"); return err }},
		{"commits", func() error { _, err := searchCommits(ctx, client, "42", "x"); return err }},
		{"labels", func() error { _, err := searchLabels(ctx, client, "42", "x"); return err }},
		{"milestones", func() error { _, err := searchMilestones(ctx, client, "42", "x"); return err }},
		{"jobs", func() error { _, err := searchJobs(ctx, client, "42", 10, "x"); return err }},
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
