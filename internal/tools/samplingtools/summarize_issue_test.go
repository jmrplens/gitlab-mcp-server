// summarize_issue_test.go contains unit tests for the samplingtools MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package samplingtools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatIssueForSummary tests.

// TestFormatIssueForSummary_Basic verifies that FormatIssueForSummary produces
// a Markdown document containing the issue title, state, author, dates, labels,
// assignees, description, and discussion notes.
func TestFormatIssueForSummary_Basic(t *testing.T) {
	issue := issues.Output{
		IID:         10,
		Title:       testLoginBug,
		State:       "opened",
		Author:      "alice",
		CreatedAt:   "2026-01-15T10:00:00Z",
		DueDate:     "2026-02-01",
		Labels:      []string{"bug", "critical"},
		Assignees:   []string{"bob", "charlie"},
		Description: "Login fails on mobile",
	}
	notes := issuenotes.ListOutput{
		Notes: []issuenotes.Output{
			{
				ID:        100,
				Author:    "alice",
				Body:      "Found a fix",
				CreatedAt: "2026-01-16T10:00:00Z",
			},
		},
	}

	result := FormatIssueForSummary(issue, notes)

	checks := []struct {
		name string
		want string
	}{
		{"title", "# Issue #10: Login bug"},
		{"state", "**State**: opened"},
		{"author", "**Author**: alice"},
		{"created", "**Created**: 15 Jan 2026 10:00 UTC"},
		{"due date", "**Due Date**: 1 Feb 2026"},
		{"labels", "**Labels**: bug, critical"},
		{"assignees", "**Assignees**: bob, charlie"},
		{"description", mdSectionDescription},
		{"description text", "Login fails on mobile"},
		{"discussion", "## Discussion (1 notes)"},
		{"note author", "**alice**"},
		{"note body", "Found a fix"},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.want) {
			t.Errorf("FormatIssueForSummary missing %s: want substring %q", c.name, c.want)
		}
	}
}

// TestFormatIssueForSummary_NoNotes verifies that FormatIssueForSummary omits
// the Discussion section when there are no notes.
func TestFormatIssueForSummary_NoNotes(t *testing.T) {
	issue := issues.Output{IID: 11, Title: "No discussion"}
	notes := issuenotes.ListOutput{Notes: []issuenotes.Output{}}
	result := FormatIssueForSummary(issue, notes)
	if strings.Contains(result, "## Discussion") {
		t.Error("empty notes should not produce Discussion section")
	}
}

// TestFormatIssueForSummary_NoDueDate verifies that FormatIssueForSummary
// omits the Due Date field when it is empty.
func TestFormatIssueForSummary_NoDueDate(t *testing.T) {
	issue := issues.Output{IID: 12, Title: "No due date", DueDate: ""}
	notes := issuenotes.ListOutput{}
	result := FormatIssueForSummary(issue, notes)
	if strings.Contains(result, "Due Date") {
		t.Error("empty due date should not appear")
	}
}

// TestFormatIssueForSummary_NoLabels verifies that FormatIssueForSummary omits
// the Labels field when labels are nil.
func TestFormatIssueForSummary_NoLabels(t *testing.T) {
	issue := issues.Output{IID: 13, Title: "No labels", Labels: nil}
	notes := issuenotes.ListOutput{}
	result := FormatIssueForSummary(issue, notes)
	if strings.Contains(result, "Labels") {
		t.Error("nil labels should not appear")
	}
}

// TestFormatIssueForSummary_NoAssignees verifies that FormatIssueForSummary
// omits the Assignees field when assignees are nil.
func TestFormatIssueForSummary_NoAssignees(t *testing.T) {
	issue := issues.Output{IID: 14, Title: "No assignees", Assignees: nil}
	notes := issuenotes.ListOutput{}
	result := FormatIssueForSummary(issue, notes)
	if strings.Contains(result, "Assignees") {
		t.Error("nil assignees should not appear")
	}
}

// TestFormatIssueForSummary_NoDescription verifies that FormatIssueForSummary
// omits the Description section when the issue description is empty.
func TestFormatIssueForSummary_NoDescription(t *testing.T) {
	issue := issues.Output{IID: 15, Title: "No desc", Description: ""}
	notes := issuenotes.ListOutput{}
	result := FormatIssueForSummary(issue, notes)
	if strings.Contains(result, mdSectionDescription) {
		t.Error("empty description should not produce Description section")
	}
}

// TestSummarizeIssue_EmptyProjectID verifies that SummarizeIssue returns
// a validation error when project_id is empty.
func TestSummarizeIssue_EmptyProjectID(t *testing.T) {
	_, err := SummarizeIssue(context.Background(), &mcp.CallToolRequest{}, nil, SummarizeIssueInput{
		ProjectID: "",
		IssueIID:  10,
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Errorf("SummarizeIssue() error = %v, want project_id validation error", err)
	}
}

// TestSummarizeIssue_InvalidIssueIID verifies that SummarizeIssue returns
// a validation error when issue_iid is zero or negative.
func TestSummarizeIssue_InvalidIssueIID(t *testing.T) {
	_, err := SummarizeIssue(context.Background(), &mcp.CallToolRequest{}, nil, SummarizeIssueInput{
		ProjectID: "42",
		IssueIID:  -1,
	})
	if err == nil || !strings.Contains(err.Error(), "issue_iid") {
		t.Errorf("SummarizeIssue() error = %v, want issue_iid validation error", err)
	}
}

// TestSummarizeIssue_SamplingNotSupported verifies that SummarizeIssue returns
// [sampling.ErrSamplingNotSupported] when the MCP client does not support the
// sampling capability.
func TestSummarizeIssue_SamplingNotSupported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	req := &mcp.CallToolRequest{}
	_, err := SummarizeIssue(context.Background(), req, client, SummarizeIssueInput{
		ProjectID: "42",
		IssueIID:  10,
	})
	if !errors.Is(err, sampling.ErrSamplingNotSupported) {
		t.Errorf("SummarizeIssue() error = %v, want %v", err, sampling.ErrSamplingNotSupported)
	}
}

// TestSummarizeIssue_IssueNotFound verifies that SummarizeIssue returns an
// error with "fetching issue" context when the GitLab API responds with 404.
func TestSummarizeIssue_IssueNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := SummarizeIssue(ctx, req, client, SummarizeIssueInput{
		ProjectID: "42",
		IssueIID:  9999,
	})
	if err == nil {
		t.Fatal("SummarizeIssue() expected error for 404 issue, got nil")
	}
	if !strings.Contains(err.Error(), "fetching issue") {
		t.Errorf("error = %v, want 'fetching issue' context", err)
	}
}

// TestSummarizeIssue_FullFlow verifies the complete issue summarization flow:
// fetching issue details and notes from a mocked GitLab API, then delegating
// to a mocked sampling session for LLM summary. Asserts the output contains
// the issue title, model name, and non-empty summary text.
func TestSummarizeIssue_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/issues/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": 200, "iid": 10, "title": "Login bug",
			"description": "Login fails on mobile", "state": "opened",
			"author": {"username": "alice"},
			"created_at": "2026-01-15T10:00:00Z",
			"web_url": "https://gitlab.example.com/issues/10"
		}`)
	})
	mux.HandleFunc("/api/v4/projects/42/issues/10/notes", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`[%s]`, noteJSONSimple))
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := SummarizeIssue(ctx, req, client, SummarizeIssueInput{
		ProjectID: "42",
		IssueIID:  10,
	})
	if err != nil {
		t.Fatalf("SummarizeIssue() unexpected error: %v", err)
	}
	if out.IssueIID != 10 {
		t.Errorf("out.IssueIID = %d, want 10", out.IssueIID)
	}
	if out.Title != testLoginBug {
		t.Errorf("out.Title = %q, want %q", out.Title, testLoginBug)
	}
	if out.Model != testModelName {
		t.Errorf("out.Model = %q, want %q", out.Model, testModelName)
	}
	if out.Summary == "" {
		t.Error("out.Summary is empty")
	}
}

// TestFormatSummarizeIssueMarkdown verifies issue summary rendering.
func TestFormatSummarizeIssueMarkdown(t *testing.T) {
	s := SummarizeIssueOutput{
		IssueIID: 10, Title: "Bug report: crash on startup", Summary: "The issue describes a bug.",
		Model: "claude-4", Truncated: false,
	}
	md := FormatSummarizeIssueMarkdown(s)
	checks := []string{"## Issue Summary: #10", "Bug report: crash on startup", "The issue describes a bug.", "*Model: claude-4*"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("FormatSummarizeIssueMarkdown missing %q", c)
		}
	}
}
