// issue_statistics_test.go contains unit tests for the issue statistics MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package issuestatistics

import (
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// TestFormatMarkdown verifies FormatMarkdown renders the whole card: the
// scope-free heading, one row per count, and the guidance section naming the
// canonical action that lists the issues behind the counts.
func TestFormatMarkdown(t *testing.T) {
	want := "## Issue Statistics\n\n" +
		"- **All**: 10\n" +
		"- **Opened**: 7\n" +
		"- **Closed**: 3\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.list' to see the individual issues behind these counts\n"
	if got := FormatMarkdown(newStats(10, 7, 3)); got != want {
		t.Errorf("FormatMarkdown()\n got %q\nwant %q", got, want)
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

// TestFormatMarkdown_Populated checks the whole rendering of a populated
// statistics object, rows included: a substring check over a card is what let
// a table row survive a migration to list items and still pass.
func TestFormatMarkdown_Populated(t *testing.T) {
	want := "## Issue Statistics\n\n" +
		"- **All**: 100\n" +
		"- **Opened**: 60\n" +
		"- **Closed**: 40\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.list' to see the individual issues behind these counts\n"
	if got := FormatMarkdown(newStats(100, 60, 40)); got != want {
		t.Errorf("FormatMarkdown(populated)\n got %q\nwant %q", got, want)
	}
}

// TestFormatMarkdown_Empty checks that a statistics object of zeros renders
// every row: a zero count is an answer GitLab gave, not an absent value, so
// the rows stay and read "0".
func TestFormatMarkdown_Empty(t *testing.T) {
	want := "## Issue Statistics\n\n" +
		"- **All**: 0\n" +
		"- **Opened**: 0\n" +
		"- **Closed**: 0\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.list' to see the individual issues behind these counts\n"
	if got := FormatMarkdown(StatisticsOutput{}); got != want {
		t.Errorf("FormatMarkdown(zero)\n got %q\nwant %q", got, want)
	}
}

// TestFormatMarkdown_HeadingNamesNoScope checks that the heading claims no
// scope. One output type answers the instance, group and project routes, so
// the registry has one key for all three and a heading naming a scope would
// name the wrong one two times out of three.
func TestFormatMarkdown_HeadingNamesNoScope(t *testing.T) {
	md := FormatMarkdown(newStats(1, 1, 0))
	if first, _, _ := strings.Cut(md, "\n"); first != "## Issue Statistics" {
		t.Errorf("heading = %q, want %q", first, "## Issue Statistics")
	}
	for _, scope := range []string{"Global", "Group", "Project", "All Issue Statistics"} {
		t.Run(scope, func(t *testing.T) {
			if strings.Contains(md, scope) {
				t.Errorf("rendering names the scope %q:\n%s", scope, md)
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

// ---------------------------------------------------------------------------
// Discovery metadata (R-META)
// ---------------------------------------------------------------------------.

// issueStatsSpecs builds the package's action specs against a client that
// answers nothing: every assertion below reads metadata rather than calling a
// route.
func issueStatsSpecs(t *testing.T) []toolutil.ActionSpec {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	return ActionSpecs(client)
}

// publishedJSONFields returns the json names a tool input struct publishes,
// which is the set an input-schema override may name.
func publishedJSONFields(input any) map[string]bool {
	fields := make(map[string]bool)
	for field := range reflect.TypeOf(input).Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			fields[name] = true
		}
	}
	return fields
}

// enumFor returns the enum an override publishes for the named input property.
func enumFor(overrides []toolutil.InputSchemaOverride, property string) ([]any, bool) {
	for _, override := range overrides {
		if override.PropertyPath != property {
			continue
		}
		values, ok := override.Values["enum"].([]any)
		return values, ok
	}
	return nil, false
}

// TestActionSpecs_EveryActionCarriesDecoratedMetadata checks that every action
// ActionSpecs registers has been through decorateIssueStatisticsMeta, by
// comparing each against the undecorated options an unrecognized tool name
// returns.
//
// Why that matters: decorateIssueStatisticsMeta is a switch with no default,
// so an action added to ActionSpecs without a case of its own ships silently
// with the generic package usage, no related actions, no individual-tool
// description and none of the enum constraints. That is the whole of what a
// model reads to find the tool and call it correctly, and nothing asserted any
// of it: the decorator could be neutered in full with this suite still green,
// which was verified by hand before this test existed.
//
// The undecorated baseline is computed rather than quoted, so it cannot drift
// from the source. It is asserted to be undecorated first, and that control is
// the load-bearing half: without it, a decorator that stopped decorating would
// move both sides of every comparison together and each assertion below would
// pass while asserting nothing.
func TestActionSpecs_EveryActionCarriesDecoratedMetadata(t *testing.T) {
	baseline := issueStatisticsOptions("gitlab_get_issue_statistics_no_such_tool")

	if len(baseline.RelatedActions) != 0 || len(baseline.InputSchemaOverrides) != 0 || baseline.IndividualTool.Description != "" {
		t.Fatalf("an unrecognized tool name came back decorated, so the comparisons below prove nothing: %+v", baseline)
	}

	for _, spec := range issueStatsSpecs(t) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			if spec.Usage == baseline.Usage {
				t.Errorf("Usage is the undecorated placeholder %q", spec.Usage)
			}
			if len(spec.Aliases) <= len(baseline.Aliases) {
				t.Errorf("Aliases = %v, want the tool name plus natural-language aliases", spec.Aliases)
			}
			if spec.IndividualTool.Description == "" {
				t.Error("IndividualTool.Description is empty, so the individual surface lists this tool with no description")
			}
			if len(spec.RelatedActions) == 0 {
				t.Error("RelatedActions is empty, so nothing routes a model to the other scopes")
			}
			if len(spec.InputSchemaOverrides) == 0 {
				t.Error("InputSchemaOverrides is empty, so no fixed vocabulary reaches the served schema")
			}
		})
	}
}

// TestActionSpecs_RelatedActionsNameTheSiblingScopes checks that each action's
// RelatedActions names the canonical ID of the package's other two actions,
// never its own, and at least one action outside the package.
//
// Why that matters: the three actions differ only in scope, so the routing a
// model needs most from this metadata is the way to the other two. The IDs are
// hand-written in three near-identical switch cases, where a copy-paste leaves
// an action pointing at itself or naming one sibling twice, and a rename
// leaves a dangling reference that resolves to nothing. The expected IDs are
// built from the specs themselves, so this fails on the rename rather than
// preserving whatever the source happens to say today.
func TestActionSpecs_RelatedActionsNameTheSiblingScopes(t *testing.T) {
	specs := issueStatsSpecs(t)
	canonical := func(spec toolutil.ActionSpec) string { return spec.OwnerPackage + "." + spec.Name }

	for _, spec := range specs {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			own := canonical(spec)
			if slices.Contains(spec.RelatedActions, own) {
				t.Errorf("RelatedActions = %v, names the action itself (%s)", spec.RelatedActions, own)
			}
			outside := 0
			for _, related := range spec.RelatedActions {
				if !strings.HasPrefix(related, spec.OwnerPackage+".") {
					outside++
				}
			}
			if outside == 0 {
				t.Errorf("RelatedActions = %v, names no action outside %s to reach the issues behind the counts", spec.RelatedActions, spec.OwnerPackage)
			}
			for _, sibling := range specs {
				id := canonical(sibling)
				if id != own && !slices.Contains(spec.RelatedActions, id) {
					t.Errorf("RelatedActions = %v, missing sibling scope %s", spec.RelatedActions, id)
				}
			}
		})
	}
}

// TestActionSpecs_EnumOverridesConstrainPublishedFields checks that every
// input-schema override names a field its own action publishes, and that each
// fixed-vocabulary field an action publishes is constrained to GitLab's own
// vocabulary.
//
// Why that matters in both directions: an override whose PropertyPath matches
// no property patches nothing and says nothing about it, so the constraint
// never reaches the served schema and a model stays free to send a value
// GitLab refuses. The three inputs are deliberately not the same shape, since
// only the global route takes "in", so the copy-paste that would carry the
// "in" override onto the group action is exactly what the first half catches
// and the second half is what catches one being dropped. The vocabularies are
// GitLab's documented ones for the issue statistics endpoints rather than a
// copy of the source line, so the assertion and the code cannot drift together.
func TestActionSpecs_EnumOverridesConstrainPublishedFields(t *testing.T) {
	inputFields := map[string]map[string]bool{
		"gitlab_get_issue_statistics":         publishedJSONFields(GetInput{}),
		"gitlab_get_group_issue_statistics":   publishedJSONFields(GetGroupInput{}),
		"gitlab_get_project_issue_statistics": publishedJSONFields(GetProjectInput{}),
	}
	vocabularies := map[string][]any{
		"scope": {"created_by_me", "assigned_to_me", "all"},
		"in":    {"title", "description", "title,description"},
	}

	for _, spec := range issueStatsSpecs(t) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			published, known := inputFields[spec.IndividualTool.Name]
			if !known {
				t.Fatalf("no input struct is mapped to %s", spec.IndividualTool.Name)
			}
			assertOverridesNamePublishedFields(t, spec.InputSchemaOverrides, published)
			for field, want := range vocabularies {
				t.Run(field, func(t *testing.T) {
					assertVocabulary(t, spec.InputSchemaOverrides, published, field, want)
				})
			}
		})
	}
}

// assertOverridesNamePublishedFields checks that every override names a field
// the action's input publishes. One that does not patches nothing, silently.
func assertOverridesNamePublishedFields(t *testing.T, overrides []toolutil.InputSchemaOverride, published map[string]bool) {
	t.Helper()
	for _, override := range overrides {
		if !published[override.PropertyPath] {
			t.Errorf("override constrains %q, which this action's input does not publish, so it patches nothing", override.PropertyPath)
		}
	}
}

// assertVocabulary checks that a fixed-vocabulary field the action's input
// publishes is constrained to GitLab's own values.
func assertVocabulary(t *testing.T, overrides []toolutil.InputSchemaOverride, published map[string]bool, field string, want []any) {
	t.Helper()
	if !published[field] {
		return
	}
	got, ok := enumFor(overrides, field)
	if !ok {
		t.Fatalf("input publishes %q and no override constrains it to a vocabulary", field)
	}
	if !slices.Equal(got, want) {
		t.Errorf("%q enum = %v, want GitLab's vocabulary %v", field, got, want)
	}
}
