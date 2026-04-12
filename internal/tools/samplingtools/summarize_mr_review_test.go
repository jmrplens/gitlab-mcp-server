// summarize_mr_review_test.go contains unit tests for the samplingtools MCP tool handlers.
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrapprovals"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdiscussions"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestFormatMRReviewForAnalysis verifies the Markdown document produced from
// MR details, discussions, and approval state.
func TestFormatMRReviewForAnalysis(t *testing.T) {
	mr := mergerequests.Output{
		IID: 5, Title: "feat: new feature", State: "opened",
		Author: "alice", SourceBranch: "feature/x", TargetBranch: "main",
	}
	discussions := mrdiscussions.ListOutput{
		Discussions: []mrdiscussions.Output{
			{
				ID: "d1",
				Notes: []mrdiscussions.NoteOutput{
					{Author: "bob", Body: "Looks good", CreatedAt: "2024-01-15T10:00:00Z", Resolvable: true, Resolved: true},
				},
			},
		},
	}
	approvals := mrapprovals.StateOutput{
		Rules: []mrapprovals.RuleOutput{
			{Name: "Code Review", Approved: true, ApprovalsRequired: 1, ApprovedByNames: []string{"bob"}},
		},
	}
	result := FormatMRReviewForAnalysis(mr, discussions, approvals)
	checks := []struct {
		name, want string
	}{
		{"header", "# MR Review: !5 — feat: new feature"},
		{"state", "**State**: opened"},
		{"author", "**Author**: alice"},
		{"branches", "feature/x → main"},
		{"approval_rule", "**Code Review**: ✅ Approved"},
		{"discussion_author", "**bob**"},
		{"resolved_tag", "[RESOLVED]"},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.want) {
			t.Errorf("FormatMRReviewForAnalysis missing %s: want %q", c.name, c.want)
		}
	}
}

// TestFormatSummarizeMRReviewMarkdown verifies MR review summary rendering.
func TestFormatSummarizeMRReviewMarkdown(t *testing.T) {
	s := SummarizeMRReviewOutput{
		MRIID: 5, Title: "feat: new feature",
		Summary: "Review is positive", Model: "gpt-4o",
	}
	md := FormatSummarizeMRReviewMarkdown(s)
	checks := []string{"## MR Review Summary: !5", "feat: new feature", "Review is positive", "*Model: gpt-4o*"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("FormatSummarizeMRReviewMarkdown missing %q", c)
		}
	}
}

// TestFormatSummarizeMRReviewMarkdown_Truncated verifies truncation warning.
func TestFormatSummarizeMRReviewMarkdown_Truncated(t *testing.T) {
	s := SummarizeMRReviewOutput{MRIID: 1, Title: "x", Truncated: true}
	md := FormatSummarizeMRReviewMarkdown(s)
	if !strings.Contains(md, "truncated") {
		t.Error("missing truncation warning")
	}
}

// TestSummarizeMRReview_EmptyProjectID verifies project_id validation.
func TestSummarizeMRReview_EmptyProjectID(t *testing.T) {
	_, err := SummarizeMRReview(context.Background(), &mcp.CallToolRequest{}, nil, SummarizeMRReviewInput{
		ProjectID: "", MRIID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Errorf("error = %v, want project_id validation error", err)
	}
}

// TestSummarizeMRReview_InvalidMRIID verifies mr_iid validation.
func TestSummarizeMRReview_InvalidMRIID(t *testing.T) {
	_, err := SummarizeMRReview(context.Background(), &mcp.CallToolRequest{}, nil, SummarizeMRReviewInput{
		ProjectID: "42", MRIID: 0,
	})
	if err == nil || !strings.Contains(err.Error(), "mr_iid") {
		t.Errorf("error = %v, want mr_iid validation error", err)
	}
}

// TestSummarizeMRReview_SamplingNotSupported verifies ErrSamplingNotSupported.
func TestSummarizeMRReview_SamplingNotSupported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	req := &mcp.CallToolRequest{}
	_, err := SummarizeMRReview(context.Background(), req, client, SummarizeMRReviewInput{
		ProjectID: "42", MRIID: 1,
	})
	if !errors.Is(err, sampling.ErrSamplingNotSupported) {
		t.Errorf("error = %v, want %v", err, sampling.ErrSamplingNotSupported)
	}
}

// TestSummarizeMRReview_MRNotFound verifies error wrapping on 404.
func TestSummarizeMRReview_MRNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))
	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := SummarizeMRReview(ctx, req, client, SummarizeMRReviewInput{
		ProjectID: "42", MRIID: 999,
	})
	if err == nil || !strings.Contains(err.Error(), "fetching MR") {
		t.Errorf("error = %v, want 'fetching MR' context", err)
	}
}

// TestSummarizeMRReview_FullFlow verifies the complete MR review summarization flow.
func TestSummarizeMRReview_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/merge_requests/5", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": 100, "iid": 5, "title": "feat: new feature", "state": "opened",
			"source_branch": "feature/x", "target_branch": "main",
			"author": {"username": "alice"}
		}`)
	})
	mux.HandleFunc("/api/v4/projects/42/merge_requests/5/discussions", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{
			"id": "d1", "individual_note": false,
			"notes": [{"id": 1, "body": "LGTM", "author": {"username": "bob"}, "system": false, "created_at": "2024-01-15T10:00:00Z"}]
		}]`)
	})
	mux.HandleFunc("/api/v4/projects/42/merge_requests/5/approval_state", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"approval_rules_overwritten": false, "rules": []}`)
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := SummarizeMRReview(ctx, req, client, SummarizeMRReviewInput{
		ProjectID: "42", MRIID: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.MRIID != 5 {
		t.Errorf("MRIID = %d, want 5", out.MRIID)
	}
	if out.Title != "feat: new feature" {
		t.Errorf("Title = %q, want %q", out.Title, "feat: new feature")
	}
	if out.Model != testModelName {
		t.Errorf("Model = %q, want %q", out.Model, testModelName)
	}
	if out.Summary == "" {
		t.Error("Summary is empty")
	}
}
