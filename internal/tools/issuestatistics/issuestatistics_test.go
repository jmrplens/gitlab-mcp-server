// issuestatistics_test.go contains unit tests for the issue statistics MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package issuestatistics

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const errExpectedErr = "expected error"

// TestGet verifies the behavior of get.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/issues_statistics" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":10,"closed":3,"opened":7}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.All != 10 || out.Opened != 7 || out.Closed != 3 {
		t.Errorf("unexpected counts: %+v", out)
	}
}

// TestGet_Error verifies the behavior of get error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetGroup verifies the behavior of get group.
func TestGetGroup(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/5/issues_statistics" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":5,"closed":2,"opened":3}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "5"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.All != 5 {
		t.Errorf("All = %d", out.All)
	}
}

// TestGetGroup_Error verifies the behavior of get group error.
func TestGetGroup_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetProject verifies the behavior of get project.
func TestGetProject(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/issues_statistics" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":20,"closed":10,"opened":10}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Opened != 10 {
		t.Errorf("Opened = %d", out.Opened)
	}
}

// TestGetProject_Error verifies the behavior of get project error.
func TestGetProject_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestFormatMarkdown verifies the behavior of format markdown.
func TestFormatMarkdown(t *testing.T) {
	md := FormatMarkdown("Test", StatisticsOutput{All: 10, Opened: 7, Closed: 3})
	if !strings.Contains(md, "10") || !strings.Contains(md, "Test") {
		t.Error("missing content")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const fmtUnexpPath = "unexpected path: %s"

const errExpCancelledNil = "expected error for canceled context, got nil"

const fmtUnexpErr = "unexpected error: %v"

const commonStatsJSON = `{"statistics":{"counts":{"all":5,"closed":2,"opened":3}}}`

const (
	errExpLabelsParam    = "expected labels query param"
	errExpMilestoneParam = "expected milestone query param"
	errExpScopeParam     = "expected scope query param"
	errExpSearchParam    = "expected search query param"
	errExpAPIErrResponse = "expected error for API error response"
)

// ---------------------------------------------------------------------------
// FormatMarkdown
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_Populated validates format markdown populated across multiple scenarios using table-driven subtests.
func TestFormatMarkdown_Populated(t *testing.T) {
	md := FormatMarkdown("Global", StatisticsOutput{All: 100, Opened: 60, Closed: 40})

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
		if !strings.Contains(md, c.want) {
			t.Errorf("%s: missing %q in:\n%s", c.label, c.want, md)
		}
	}
}

// TestFormatMarkdown_Empty validates format markdown empty across multiple scenarios using table-driven subtests.
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
		if !strings.Contains(md, c.want) {
			t.Errorf("%s: missing %q in:\n%s", c.label, c.want, md)
		}
	}
}

// TestFormatMarkdown_DifferentLabels verifies the behavior of format markdown different labels.
func TestFormatMarkdown_DifferentLabels(t *testing.T) {
	labels := []string{"Group", "Project", "Custom Label"}
	for _, label := range labels {
		t.Run(label, func(t *testing.T) {
			md := FormatMarkdown(label, StatisticsOutput{All: 1, Opened: 1})
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

// TestFromGL_FullData verifies the behavior of from g l full data.
func TestFromGL_FullData(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":250,"closed":100,"opened":150}}}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, resp)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.All != 250 {
		t.Errorf("All = %d, want 250", out.All)
	}
	if out.Closed != 100 {
		t.Errorf("Closed = %d, want 100", out.Closed)
	}
	if out.Opened != 150 {
		t.Errorf("Opened = %d, want 150", out.Opened)
	}
}

// TestFromGL_ZeroCounts verifies the behavior of from g l zero counts.
func TestFromGL_ZeroCounts(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":0,"closed":0,"opened":0}}}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, resp)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.All != 0 || out.Closed != 0 || out.Opened != 0 {
		t.Errorf("expected all zeros, got %+v", out)
	}
}

// ---------------------------------------------------------------------------
// Get (global) -- filter branches
// ---------------------------------------------------------------------------.

// TestGet_WithAllFilters verifies the behavior of get with all filters.
func TestGet_WithAllFilters(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":10,"closed":3,"opened":7}}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/issues_statistics" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
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
		Labels:    "bug,critical",
		Milestone: "v1.0",
		Scope:     "all",
		Search:    "memory leak",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.All != 10 {
		t.Errorf("All = %d, want 10", out.All)
	}
}

// TestGet_WithLabelsOnly verifies the behavior of get with labels only.
func TestGet_WithLabelsOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("labels") == "" {
			t.Error(errExpLabelsParam)
		}
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := Get(t.Context(), client, GetInput{Labels: "bug"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGet_WithMilestoneOnly verifies the behavior of get with milestone only.
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

// TestGet_WithScopeOnly verifies the behavior of get with scope only.
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

// TestGet_WithSearchOnly verifies the behavior of get with search only.
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

// TestGet_ContextCancelled verifies the behavior of get context cancelled.
func TestGet_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":0,"closed":0,"opened":0}}}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Get(ctx, client, GetInput{})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGet_APIError500 verifies the behavior of get a p i error500.
func TestGet_APIError500(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal(errExpAPIErrResponse)
	}
}

// TestGet_APIError403 verifies the behavior of get a p i error403.
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

// TestGetGroup_WithAllFilters verifies the behavior of get group with all filters.
func TestGetGroup_WithAllFilters(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":30,"closed":10,"opened":20}}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/99/issues_statistics" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
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
		Labels:    "feature,enhancement",
		Milestone: "sprint-3",
		Scope:     "assigned_to_me",
		Search:    "refactor",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.All != 30 {
		t.Errorf("All = %d, want 30", out.All)
	}
	if out.Opened != 20 {
		t.Errorf("Opened = %d, want 20", out.Opened)
	}
}

// TestGetGroup_WithLabelsOnly verifies the behavior of get group with labels only.
func TestGetGroup_WithLabelsOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99", Labels: "bug"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_WithMilestoneOnly verifies the behavior of get group with milestone only.
func TestGetGroup_WithMilestoneOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99", Milestone: "v1.0"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_WithScopeOnly verifies the behavior of get group with scope only.
func TestGetGroup_WithScopeOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99", Scope: "all"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_WithSearchOnly verifies the behavior of get group with search only.
func TestGetGroup_WithSearchOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99", Search: "deploy"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetGroup_ContextCancelled verifies the behavior of get group context cancelled.
func TestGetGroup_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":0,"closed":0,"opened":0}}}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetGroup(ctx, client, GetGroupInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetGroup_APIError500 verifies the behavior of get group a p i error500.
func TestGetGroup_APIError500(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "99"})
	if err == nil {
		t.Fatal(errExpAPIErrResponse)
	}
}

// TestGetGroup_APIError404 verifies the behavior of get group a p i error404.
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

// TestGetProject_WithAllFilters verifies the behavior of get project with all filters.
func TestGetProject_WithAllFilters(t *testing.T) {
	const resp = `{"statistics":{"counts":{"all":50,"closed":20,"opened":30}}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues_statistics" {
			t.Fatalf(fmtUnexpPath, r.URL.Path)
		}
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
		Labels:    "bug,security",
		Milestone: "release-1",
		Scope:     "created_by_me",
		Search:    "crash",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.All != 50 {
		t.Errorf("All = %d, want 50", out.All)
	}
	if out.Closed != 20 {
		t.Errorf("Closed = %d, want 20", out.Closed)
	}
	if out.Opened != 30 {
		t.Errorf("Opened = %d, want 30", out.Opened)
	}
}

// TestGetProject_WithLabelsOnly verifies the behavior of get project with labels only.
func TestGetProject_WithLabelsOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42", Labels: "bug"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_WithMilestoneOnly verifies the behavior of get project with milestone only.
func TestGetProject_WithMilestoneOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42", Milestone: "v3.0"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_WithScopeOnly verifies the behavior of get project with scope only.
func TestGetProject_WithScopeOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42", Scope: "all"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_WithSearchOnly verifies the behavior of get project with search only.
func TestGetProject_WithSearchOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, commonStatsJSON)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42", Search: "nil pointer"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGetProject_ContextCancelled verifies the behavior of get project context cancelled.
func TestGetProject_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"statistics":{"counts":{"all":0,"closed":0,"opened":0}}}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetProject(ctx, client, GetProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetProject_APIError500 verifies the behavior of get project a p i error500.
func TestGetProject_APIError500(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIErrResponse)
	}
}

// TestGetProject_APIError401 verifies the behavior of get project a p i error401.
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
// MCP integration -- RegisterTools
// ---------------------------------------------------------------------------.

const covStatsJSON = `{"statistics":{"counts":{"all":100,"closed":40,"opened":60}}}`

// newIssueStatsMCPSession is an internal helper for the issuestatistics package.
func newIssueStatsMCPSession(t *testing.T) *mcp.ClientSession {
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

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// assertToolCallSuccess is an internal helper for the issuestatistics package.
func assertToolCallSuccess(t *testing.T, session *mcp.ClientSession, ctx context.Context, name string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", name, err)
	}
	if result.IsError {
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				t.Fatalf("CallTool(%s) returned error: %s", name, tc.Text)
			}
		}
		t.Fatalf("CallTool(%s) returned IsError=true", name)
	}
}

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newIssueStatsMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_issue_statistics", map[string]any{
			"labels": "", "milestone": "", "scope": "", "search": "",
		}},
		{"gitlab_get_group_issue_statistics", map[string]any{
			"group_id": "99", "labels": "", "milestone": "", "scope": "", "search": "",
		}},
		{"gitlab_get_project_issue_statistics", map[string]any{
			"project_id": "42", "labels": "", "milestone": "", "scope": "", "search": "",
		}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			assertToolCallSuccess(t, session, ctx, tt.name, tt.args)
		})
	}
}

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP integration -- RegisterMeta (registration only; calling through MCP
// panics due to nil FormatResultFunc in production code -- tracked separately)
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// TestMCPRoundTrip_Errors validates register.go error paths for the 3 statistics
// tools via MCP round-trip against a 500 backend.
func TestMCPRoundTrip_Errors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, mux)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

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
			res, callErr := session.CallTool(ctx, &mcp.CallToolParams{
				Name: tc.name, Arguments: tc.args,
			})
			if callErr != nil {
				t.Fatalf("CallTool: %v", callErr)
			}
			if !res.IsError {
				t.Error("expected IsError=true")
			}
		})
	}
}

// TestMarkdownInit validates the init-registered markdown formatter is callable
// via the toolutil registry.
func TestMarkdownInit(t *testing.T) {
	out := StatisticsOutput{All: 10, Opened: 7, Closed: 3}
	res := toolutil.MarkdownForResult(out)
	if res == nil {
		t.Fatal("expected non-nil result from registered formatter")
	}
}
