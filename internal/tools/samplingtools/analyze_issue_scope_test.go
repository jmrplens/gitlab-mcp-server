// analyze_issue_scope_test.go contains unit tests for the samplingtools MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package samplingtools

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestFormatIssueScopeForAnalysis verifies the Markdown output from issue scope data.
func TestFormatIssueScopeForAnalysis(t *testing.T) {
	issue := issues.Output{
		IID: 10, Title: "Improve login", State: "opened",
		Author: "alice", CreatedAt: "2026-01-15T10:00:00Z",
		DueDate: "2026-02-15", Labels: []string{"enhancement"},
		Assignees: []string{"alice", "bob"}, Weight: 3,
		Description: "Improve the login flow",
	}
	timeStats := issues.TimeStatsOutput{
		HumanTimeEstimate:   "2h",
		HumanTotalTimeSpent: "1h 30m",
	}
	participants := issues.ParticipantsOutput{
		Participants: []issues.ParticipantOutput{
			{Username: "alice"}, {Username: "bob"},
		},
	}
	closingMRs := issues.RelatedMRsOutput{
		MergeRequests: []issues.RelatedMROutput{
			{IID: 20, Title: "Fix login", State: "merged", Author: "alice"},
		},
	}
	relatedMRs := issues.RelatedMRsOutput{
		MergeRequests: []issues.RelatedMROutput{
			{IID: 21, Title: "Refactor auth", State: "opened", Author: "bob"},
		},
	}
	notes := issuenotes.ListOutput{
		Notes: []issuenotes.Output{
			{Author: "alice", Body: "Working on this", CreatedAt: "2026-01-16T10:00:00Z"},
		},
	}
	result := FormatIssueScopeForAnalysis(issue, timeStats, participants, closingMRs, relatedMRs, notes)
	checks := []struct {
		name, want string
	}{
		{"header", "# Issue #10: Improve login"},
		{"state", "**State**: opened"},
		{"due_date", "**Due Date**: 15 Feb 2026"},
		{"labels", "**Labels**: enhancement"},
		{"weight", "**Weight**: 3"},
		{"estimate", "**Estimate**: 2h"},
		{"time_spent", "**Time Spent**: 1h 30m"},
		{"participants", "## Participants (2)"},
		{"description_section", "## Description"},
		{"closing_mrs", "## Closing MRs (1)"},
		{"closing_mr_entry", "!20 — Fix login [merged] (@alice)"},
		{"related_mrs", "## Related MRs (1)"},
		{"related_mr_entry", "!21 — Refactor auth [opened] (@bob)"},
		{"discussion", "## Discussion (1 notes)"},
		{"note_content", "Working on this"},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.want) {
			t.Errorf("FormatIssueScopeForAnalysis missing %s: want %q", c.name, c.want)
		}
	}
}

// TestFormatAnalyzeIssueScopeMarkdown verifies issue scope analysis rendering.
func TestFormatAnalyzeIssueScopeMarkdown(t *testing.T) {
	a := AnalyzeIssueScopeOutput{
		IssueIID: 10, Title: "Improve login",
		Analysis: "Issue is well-scoped", Model: "gpt-4o",
	}
	md := FormatAnalyzeIssueScopeMarkdown(a)
	checks := []string{"## Issue Scope Analysis: #10", "Improve login", "Issue is well-scoped", "*Model: gpt-4o*"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("FormatAnalyzeIssueScopeMarkdown missing %q", c)
		}
	}
}

// TestFormatAnalyzeIssueScopeMarkdown_Truncated verifies truncation warning.
func TestFormatAnalyzeIssueScopeMarkdown_Truncated(t *testing.T) {
	a := AnalyzeIssueScopeOutput{Title: "x", Truncated: true}
	md := FormatAnalyzeIssueScopeMarkdown(a)
	if !strings.Contains(md, "truncated") {
		t.Error("missing truncation warning")
	}
}

// TestAnalyzeIssueScope_EmptyProjectID verifies project_id validation.
func TestAnalyzeIssueScope_EmptyProjectID(t *testing.T) {
	_, err := AnalyzeIssueScope(context.Background(), &mcp.CallToolRequest{}, nil, AnalyzeIssueScopeInput{
		ProjectID: "", IssueIID: 10,
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Errorf("error = %v, want project_id validation error", err)
	}
}

// TestAnalyzeIssueScope_InvalidIID verifies issue_iid validation.
func TestAnalyzeIssueScope_InvalidIID(t *testing.T) {
	_, err := AnalyzeIssueScope(context.Background(), &mcp.CallToolRequest{}, nil, AnalyzeIssueScopeInput{
		ProjectID: "42", IssueIID: 0,
	})
	if err == nil || !strings.Contains(err.Error(), "issue_iid") {
		t.Errorf("error = %v, want issue_iid validation error", err)
	}
}

// TestAnalyzeIssueScope_SamplingNotSupported verifies ErrSamplingNotSupported.
func TestAnalyzeIssueScope_SamplingNotSupported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	req := &mcp.CallToolRequest{}
	_, err := AnalyzeIssueScope(context.Background(), req, client, AnalyzeIssueScopeInput{
		ProjectID: "42", IssueIID: 10,
	})
	if !errors.Is(err, sampling.ErrSamplingNotSupported) {
		t.Errorf("error = %v, want %v", err, sampling.ErrSamplingNotSupported)
	}
}

// TestAnalyzeIssueScope_IssueNotFound verifies error wrapping on 404.
func TestAnalyzeIssueScope_IssueNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))
	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := AnalyzeIssueScope(ctx, req, client, AnalyzeIssueScopeInput{
		ProjectID: "42", IssueIID: 999,
	})
	if err == nil || !strings.Contains(err.Error(), "fetching issue") {
		t.Errorf("error = %v, want 'fetching issue' context", err)
	}
}

// TestAnalyzeIssueScope_FullFlow verifies the complete issue scope analysis flow.
func TestAnalyzeIssueScope_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/issues/10", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/time_stats") || strings.HasSuffix(r.URL.Path, "/participants") ||
			strings.Contains(r.URL.Path, "/closed_by") || strings.Contains(r.URL.Path, "/related_merge_requests") {
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": 200, "iid": 10, "title": "Improve login", "state": "opened",
			"author": {"username": "alice"}, "created_at": "2026-01-15T10:00:00Z"
		}`)
	})
	mux.HandleFunc("/api/v4/projects/42/issues/10/time_stats", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"human_time_estimate": "2h", "human_total_time_spent": "1h"}`)
	})
	mux.HandleFunc("/api/v4/projects/42/issues/10/participants", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id": 1, "username": "alice"}]`)
	})
	mux.HandleFunc("/api/v4/projects/42/issues/10/closed_by", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("/api/v4/projects/42/issues/10/related_merge_requests", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("/api/v4/projects/42/issues/10/notes", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id": 1, "body": "Working on it", "author": {"username": "alice"}, "system": false, "created_at": "2026-01-16T10:00:00Z"}]`)
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := AnalyzeIssueScope(ctx, req, client, AnalyzeIssueScopeInput{
		ProjectID: "42", IssueIID: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.IssueIID != 10 {
		t.Errorf("IssueIID = %d, want 10", out.IssueIID)
	}
	if out.Title != "Improve login" {
		t.Errorf("Title = %q, want %q", out.Title, "Improve login")
	}
	if out.Model != testModelName {
		t.Errorf("Model = %q, want %q", out.Model, testModelName)
	}
	if out.Analysis == "" {
		t.Error("Analysis is empty")
	}
}

// TestAnalyzeIssueScope_LLMError covers analyze_issue_scope.go:121-123.
func TestAnalyzeIssueScope_LLMError(t *testing.T) {
	mux := http.NewServeMux()
	// GraphQL returns issue context successfully.
	mux.HandleFunc("/api/graphql", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondGraphQL(w, http.StatusOK, issueContextJSON)
	})
	client := testutil.NewTestClient(t, mux)
	ctx := context.Background()
	_, ss, cleanup := setupFailingSamplingSession(t, ctx)
	defer cleanup()

	_, err := AnalyzeIssueScope(ctx, &mcp.CallToolRequest{Session: ss}, client, AnalyzeIssueScopeInput{ProjectID: "42", IssueIID: 7})
	if err == nil || !strings.Contains(err.Error(), "LLM analysis") {
		t.Errorf("error = %v, want 'LLM analysis' context", err)
	}
}

// TestAnalyzeIssueScope_RESTFallback_IssueError covers analyze_issue_scope.go:71-74.
func TestAnalyzeIssueScope_RESTFallback_IssueError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"internal error"}`)
	}))
	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	_, err := AnalyzeIssueScope(ctx, &mcp.CallToolRequest{Session: ss}, client, AnalyzeIssueScopeInput{ProjectID: "42", IssueIID: 7})
	if err == nil || !strings.Contains(err.Error(), "fetching issue") {
		t.Errorf("error = %v, want 'fetching issue' context", err)
	}
}
