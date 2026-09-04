// issue_statistics_test.go contains unit tests for the issue statistics MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package issuestatistics

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// newStats builds a [StatisticsOutput] with the nested statistics/counts
// structure populated from the given all/opened/closed values.
func newStats(all, opened, closed int64) StatisticsOutput {
	return StatisticsOutput{
		Statistics: StatisticsCountsOutput{
			Counts: CountsOutput{All: all, Opened: opened, Closed: closed},
		},
	}
}

// TestGet verifies Get.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/issues_statistics")
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":10,"closed":3,"opened":7}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 10 || out.Statistics.Counts.Opened != 7 || out.Statistics.Counts.Closed != 3 {
		t.Errorf("unexpected counts: %+v", out)
	}
}

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetGroup verifies GetGroup.
func TestGetGroup(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/groups/5/issues_statistics")
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":5,"closed":2,"opened":3}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "5"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 5 {
		t.Errorf("All = %d", out.Statistics.Counts.All)
	}
}

// TestGetGroup_Error verifies GetGroup when error.
func TestGetGroup_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetProject verifies GetProject.
func TestGetProject(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/projects/1/issues_statistics")
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":20,"closed":10,"opened":10}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.Opened != 10 {
		t.Errorf("Opened = %d", out.Statistics.Counts.Opened)
	}
}

// TestGetProject_Error verifies GetProject when error.
func TestGetProject_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestFormatMarkdown verifies FormatMarkdown.
func TestFormatMarkdown(t *testing.T) {
	md := FormatMarkdown("Test", newStats(10, 7, 3))
	if !strings.Contains(md, "10") || !strings.Contains(md, "Test") {
		t.Error("missing content")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpCancelledNil identifies the err exp cancelled nil constant used by this package.
const errExpCancelledNil = "expected error for canceled context, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// commonStatsJSON identifies the common stats JSON constant used by this package.
const commonStatsJSON = `{"statistics":{"counts":{"all":5,"closed":2,"opened":3}}}`

const (
	// errExpLabelsParam identifies the err exp labels param constant used by this package.
	errExpLabelsParam = "expected labels query param"
	// errExpMilestoneParam identifies the err exp milestone param constant used by this package.
	errExpMilestoneParam = "expected milestone query param"
	// errExpScopeParam identifies the err exp scope param constant used by this package.
	errExpScopeParam = "expected scope query param"
	// errExpSearchParam identifies the err exp search param constant used by this package.
	errExpSearchParam = "expected search query param"
	// errExpAPIErrResponse identifies the err exp API err response constant used by this package.
	errExpAPIErrResponse = "expected error for API error response"
)

// ---------------------------------------------------------------------------
// FormatMarkdown
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_Populated covers FormatMarkdown with table-driven subtests for populated.
func TestFormatMarkdown_Populated(t *testing.T) {
	md := FormatMarkdown("Global", newStats(100, 60, 40))

	checks := []struct {
		label, want string
	}{
		{"header", "## Global Issue Statistics"},
		{"all count", "| All | 100 |"},
		{"opened count", "| Opened | 60 |"},
		{"closed count", "| Closed | 40 |"},
		{"table header status", "| Status | Count |"},
	}
	for _, c := range checks {
		t.Run(c.label, func(t *testing.T) {
			if !strings.Contains(md, c.want) {
				t.Errorf("%s: missing %q in:\n%s", c.label, c.want, md)
			}
		})
	}
}

// TestFormatMarkdown_Empty covers FormatMarkdown with table-driven subtests for empty.
func TestFormatMarkdown_Empty(t *testing.T) {
	md := FormatMarkdown("Empty", StatisticsOutput{})

	checks := []struct {
		label, want string
	}{
		{"header", "## Empty Issue Statistics"},
		{"all zero", "| All | 0 |"},
		{"opened zero", "| Opened | 0 |"},
		{"closed zero", "| Closed | 0 |"},
	}
	for _, c := range checks {
		t.Run(c.label, func(t *testing.T) {
			if !strings.Contains(md, c.want) {
				t.Errorf("%s: missing %q in:\n%s", c.label, c.want, md)
			}
		})
	}
}

// TestFormatMarkdown_DifferentLabels verifies FormatMarkdown when different labels.
func TestFormatMarkdown_DifferentLabels(t *testing.T) {
	labels := []string{"Group", "Project", "Custom Label"}
	for _, label := range labels {
		t.Run(label, func(t *testing.T) {
			md := FormatMarkdown(label, newStats(1, 1, 0))
			want := "## " + label + " Issue Statistics"
			if !strings.Contains(md, want) {
				t.Errorf("missing %q in:\n%s", want, md)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// fromGL converter (tested indirectly via handlers)
// ---------------------------------------------------------------------------.

// TestFromGL_FullData verifies FromGL when full data.
func TestFromGL_FullData(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":250,"closed":100,"opened":150}}}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, resp)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 250 {
		t.Errorf("All = %d, want 250", out.Statistics.Counts.All)
	}
	if out.Statistics.Counts.Closed != 100 {
		t.Errorf("Closed = %d, want 100", out.Statistics.Counts.Closed)
	}
	if out.Statistics.Counts.Opened != 150 {
		t.Errorf("Opened = %d, want 150", out.Statistics.Counts.Opened)
	}
}

// TestFromGL_ZeroCounts verifies FromGL when zero counts.
func TestFromGL_ZeroCounts(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":0,"closed":0,"opened":0}}}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, resp)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 0 || out.Statistics.Counts.Closed != 0 || out.Statistics.Counts.Opened != 0 {
		t.Errorf("expected all zeros, got %+v", out)
	}
}

// ---------------------------------------------------------------------------
// Get (global) -- filter branches
// ---------------------------------------------------------------------------.

// TestGet_WithAllFilters verifies Get when with all filters.
func TestGet_WithAllFilters(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":10,"closed":3,"opened":7}}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/issues_statistics")
		q := r.URL.Query()
		if q.Get("labels") == "" {
			t.Error(errExpLabelsParam)
		}
		if q.Get("milestone") == "" {
			t.Error(errExpMilestoneParam)
		}
		if q.Get("scope") == "" {
			t.Error(errExpScopeParam)
		}
		if q.Get("search") == "" {
			t.Error(errExpSearchParam)
		}
		testutil.RespondJSON(w, http.StatusOK, resp)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Get(t.Context(), client, GetInput{
		Labels:    []string{"bug", "critical"},
		Milestone: "v1.0",
		Scope:     "all",
		Search:    "memory leak",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 10 {
		t.Errorf("All = %d, want 10", out.Statistics.Counts.All)
	}
}

// TestGet_WithLabelsOnly verifies Get when with labels only.
func TestGet_WithLabelsOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("labels") == "" {
			t.Error(errExpLabelsParam)
		}
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := Get(t.Context(), client, GetInput{Labels: []string{"bug"}})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGet_WithMilestoneOnly verifies Get when with milestone only.
func TestGet_WithMilestoneOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("milestone") == "" {
			t.Error(errExpMilestoneParam)
		}
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := Get(t.Context(), client, GetInput{Milestone: "v2.0"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGet_WithScopeOnly verifies Get when with scope only.
func TestGet_WithScopeOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("scope") == "" {
			t.Error(errExpScopeParam)
		}
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := Get(t.Context(), client, GetInput{Scope: "created_by_me"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGet_WithSearchOnly verifies Get when with search only.
func TestGet_WithSearchOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("search") == "" {
			t.Error(errExpSearchParam)
		}
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := Get(t.Context(), client, GetInput{Search: "timeout"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGet_ContextCancelled verifies Get when context cancelled.
func TestGet_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":0,"closed":0,"opened":0}}}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGet_APIError500 verifies Get when API error 500.
func TestGet_APIError500(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal(errExpAPIErrResponse)
	}
}

// TestGet_APIError403 verifies Get when API error 403.
func TestGet_APIError403(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

// ---------------------------------------------------------------------------
// GetGroup -- filter branches
// ---------------------------------------------------------------------------.

// TestGetGroup_WithAllFilters verifies GetGroup when with all filters.
func TestGetGroup_WithAllFilters(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":30,"closed":10,"opened":20}}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/groups/99/issues_statistics")
		q := r.URL.Query()
		if q.Get("labels") == "" {
			t.Error(errExpLabelsParam)
		}
		if q.Get("milestone") == "" {
			t.Error(errExpMilestoneParam)
		}
		if q.Get("scope") == "" {
			t.Error(errExpScopeParam)
		}
		if q.Get("search") == "" {
			t.Error(errExpSearchParam)
		}
		testutil.RespondJSON(w, http.StatusOK, resp)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := GetGroup(t.Context(), client, GetGroupInput{
		GroupID:   "99",
		Labels:    []string{"feature", "enhancement"},
		Milestone: "sprint-3",
		Scope:     "assigned_to_me",
		Search:    "refactor",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 30 {
		t.Errorf("All = %d, want 30", out.Statistics.Counts.All)
	}
	if out.Statistics.Counts.Opened != 20 {
		t.Errorf("Opened = %d, want 20", out.Statistics.Counts.Opened)
	}
}

// TestGetGroup_WithLabelsOnly verifies GetGroup when with labels only.
func TestGetGroup_WithLabelsOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99", Labels: []string{"bug"}})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_WithMilestoneOnly verifies GetGroup when with milestone only.
func TestGetGroup_WithMilestoneOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99", Milestone: "v1.0"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_WithScopeOnly verifies GetGroup when with scope only.
func TestGetGroup_WithScopeOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99", Scope: "all"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_WithSearchOnly verifies GetGroup when with search only.
func TestGetGroup_WithSearchOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99", Search: "deploy"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_ContextCancelled verifies GetGroup when context cancelled.
func TestGetGroup_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":0,"closed":0,"opened":0}}}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetGroup(ctx, client, GetGroupInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetGroup_APIError500 verifies GetGroup when API error 500.
func TestGetGroup_APIError500(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpAPIErrResponse)
	}
}

// TestGetGroup_APIError404 verifies GetGroup when API error 404.
func TestGetGroup_APIError404(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"group not found"}`)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

// ---------------------------------------------------------------------------
// GetProject -- filter branches
// ---------------------------------------------------------------------------.

// TestGetProject_WithAllFilters verifies GetProject when with all filters.
func TestGetProject_WithAllFilters(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":50,"closed":20,"opened":30}}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/projects/42/issues_statistics")
		q := r.URL.Query()
		if q.Get("labels") == "" {
			t.Error(errExpLabelsParam)
		}
		if q.Get("milestone") == "" {
			t.Error(errExpMilestoneParam)
		}
		if q.Get("scope") == "" {
			t.Error(errExpScopeParam)
		}
		if q.Get("search") == "" {
			t.Error(errExpSearchParam)
		}
		testutil.RespondJSON(w, http.StatusOK, resp)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := GetProject(t.Context(), client, GetProjectInput{
		ProjectID: "42",
		Labels:    []string{"bug", "security"},
		Milestone: "release-1",
		Scope:     "created_by_me",
		Search:    "crash",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statistics.Counts.All != 50 {
		t.Errorf("All = %d, want 50", out.Statistics.Counts.All)
	}
	if out.Statistics.Counts.Closed != 20 {
		t.Errorf("Closed = %d, want 20", out.Statistics.Counts.Closed)
	}
	if out.Statistics.Counts.Opened != 30 {
		t.Errorf("Opened = %d, want 30", out.Statistics.Counts.Opened)
	}
}

// TestGetProject_WithLabelsOnly verifies GetProject when with labels only.
func TestGetProject_WithLabelsOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42", Labels: []string{"bug"}})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_WithMilestoneOnly verifies GetProject when with milestone only.
func TestGetProject_WithMilestoneOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42", Milestone: "v3.0"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_WithScopeOnly verifies GetProject when with scope only.
func TestGetProject_WithScopeOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42", Scope: "all"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_WithSearchOnly verifies GetProject when with search only.
func TestGetProject_WithSearchOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42", Search: "nil pointer"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_ContextCancelled verifies GetProject when context cancelled.
func TestGetProject_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":0,"closed":0,"opened":0}}}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetProject(ctx, client, GetProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetProject_APIError500 verifies GetProject when API error 500.
func TestGetProject_APIError500(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIErrResponse)
	}
}

// TestGetProject_APIError401 verifies GetProject when API error 401.
func TestGetProject_APIError401(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"unauthorized"}`)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// covStatsJSON identifies the cov stats JSON constant used by this package.
const covStatsJSON = `{"statistics":{"counts":{"all":100,"closed":40,"opened":60}}}`

// newIssueStatsRouteSpecs constructs issue stats route specs test fixtures.
func newIssueStatsRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == "/api/v4/issues_statistics":
			testutil.RespondJSON(w, http.StatusOK, covStatsJSON)
		case r.Method == http.MethodGet && path == "/api/v4/groups/99/issues_statistics":
			testutil.RespondJSON(w, http.StatusOK, covStatsJSON)
		case r.Method == http.MethodGet && path == "/api/v4/projects/42/issues_statistics":
			testutil.RespondJSON(w, http.StatusOK, covStatsJSON)
		default:
			http.NotFound(w, r)
		}
	}))

	return issueStatsSpecsByTool(ActionSpecs(client))
}

// issueStatsSpecsByTool indexes action specs by individual tool name for route assertions.
func issueStatsSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}

// assertRouteCallSuccess checks route call success invariants for tests.
func assertRouteCallSuccess(t *testing.T, specByTool map[string]toolutil.ActionSpec, name string, args map[string]any) {
	t.Helper()
	spec, ok := specByTool[name]
	if !ok {
		t.Fatalf("missing ActionSpec for %s", name)
	}
	result, err := spec.Route.Handler(t.Context(), args)
	if err != nil {
		t.Fatalf("Route.Handler(%s) error: %v", name, err)
	}
	if result == nil {
		t.Fatalf("Route.Handler(%s) returned nil", name)
	}
}

// TestActionSpecs_CallRoutes validates issue statistics canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := newIssueStatsRouteSpecs(t)

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_issue_statistics", map[string]any{
			"labels": []string{}, "milestone": "", "scope": "", "search": "",
		}},
		{"gitlab_get_group_issue_statistics", map[string]any{
			"group_id": "99", "labels": []string{}, "milestone": "", "scope": "", "search": "",
		}},
		{"gitlab_get_project_issue_statistics", map[string]any{
			"project_id": "42", "labels": []string{}, "milestone": "", "scope": "", "search": "",
		}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			assertRouteCallSuccess(t, specByTool, tt.name, tt.args)
		})
	}
}

// TestActionSpecs_Metadata verifies issue statistics action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "issuestatistics" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// ---------------------------------------------------------------------------
// panics due to nil FormatResultFunc in production code -- tracked separately)
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRouteErrors validates issue statistics route error paths.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	client := testutil.NewTestClient(t, mux)
	specByTool := issueStatsSpecsByTool(ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_issue_statistics", map[string]any{}},
		{"gitlab_get_group_issue_statistics", map[string]any{"group_id": "42"}},
		{"gitlab_get_project_issue_statistics", map[string]any{"project_id": "42"}},
	}
	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			spec, ok := specByTool[tc.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tc.name)
			}
			if _, err := spec.Route.Handler(t.Context(), tc.args); err == nil {
				t.Fatal("expected route error")
			}
		})
	}
}

// TestMarkdownInit validates the init-registered markdown formatter is callable
// via the toolutil registry.
func TestMarkdownInit(t *testing.T) {
	out := newStats(10, 7, 3)
	res := toolutil.MarkdownForResult(out)
	if res == nil {
		t.Fatal("expected non-nil result from registered formatter")
	}
}

// ---------------------------------------------------------------------------
// Extended 1:1 filter coverage (assignee, author, confidential, dates, IIDs)
// ---------------------------------------------------------------------------.

// assertExtendedStatsParams verifies that the extended issue-statistics filter
// fields are serialized into the outgoing query string.
func assertExtendedStatsParams(t *testing.T, q map[string][]string, getParam func(string) string) {
	t.Helper()
	checks := map[string]string{
		"assignee_id":       "7",
		"author_id":         "9",
		"author_username":   "octocat",
		"confidential":      "true",
		"my_reaction_emoji": "thumbsup",
	}
	for key, want := range checks {
		if got := getParam(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if got := q["assignee_username"]; len(got) == 0 {
		t.Error("expected assignee_username query param")
	}
	if got := q["iids[]"]; len(got) == 0 {
		t.Error("expected iids[] query param")
	}
	if getParam("created_after") == "" {
		t.Error("expected created_after query param")
	}
	if getParam("created_before") == "" {
		t.Error("expected created_before query param")
	}
	if getParam("updated_after") == "" {
		t.Error("expected updated_after query param")
	}
	if getParam("updated_before") == "" {
		t.Error("expected updated_before query param")
	}
}

// TestGet_ExtendedFilters verifies that global statistics serializes every
// extended filter field, covering optStrings and optIIDs non-empty branches.
func TestGet_ExtendedFilters(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assertExtendedStatsParams(t, q, q.Get)
		if q.Get("in") != "title" {
			t.Errorf("in = %q, want title", q.Get("in"))
		}
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Get(t.Context(), client, GetInput{
		AssigneeID:       new(int64(7)),
		AssigneeUsername: []string{"alice"},
		AuthorID:         new(int64(9)),
		AuthorUsername:   "octocat",
		Confidential:     new(true),
		CreatedAfter:     "2025-01-01T00:00:00Z",
		CreatedBefore:    "2025-12-31T23:59:59Z",
		UpdatedAfter:     "2025-02-01T00:00:00Z",
		UpdatedBefore:    "2025-11-30T23:59:59Z",
		IIDs:             []int64{1, 2},
		In:               "title",
		MyReactionEmoji:  "thumbsup",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_ExtendedFilters verifies group statistics serializes every
// extended filter field.
func TestGetGroup_ExtendedFilters(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assertExtendedStatsParams(t, q, q.Get)
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := GetGroup(t.Context(), client, GetGroupInput{
		GroupID:          "99",
		AssigneeID:       new(int64(7)),
		AssigneeUsername: []string{"alice"},
		AuthorID:         new(int64(9)),
		AuthorUsername:   "octocat",
		Confidential:     new(true),
		CreatedAfter:     "2025-01-01T00:00:00Z",
		CreatedBefore:    "2025-12-31T23:59:59Z",
		UpdatedAfter:     "2025-02-01T00:00:00Z",
		UpdatedBefore:    "2025-11-30T23:59:59Z",
		IIDs:             []int64{1, 2},
		MyReactionEmoji:  "thumbsup",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_ExtendedFilters verifies project statistics serializes every
// extended filter field.
func TestGetProject_ExtendedFilters(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assertExtendedStatsParams(t, q, q.Get)
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := GetProject(t.Context(), client, GetProjectInput{
		ProjectID:        "42",
		AssigneeID:       new(int64(7)),
		AssigneeUsername: []string{"alice"},
		AuthorID:         new(int64(9)),
		AuthorUsername:   "octocat",
		Confidential:     new(true),
		CreatedAfter:     "2025-01-01T00:00:00Z",
		CreatedBefore:    "2025-12-31T23:59:59Z",
		UpdatedAfter:     "2025-02-01T00:00:00Z",
		UpdatedBefore:    "2025-11-30T23:59:59Z",
		IIDs:             []int64{1, 2},
		MyReactionEmoji:  "thumbsup",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}
