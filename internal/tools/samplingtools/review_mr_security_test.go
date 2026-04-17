// review_mr_security_test.go contains unit tests for the samplingtools MCP tool handlers.
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

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestFormatReviewMRSecurityMarkdown verifies MR security review rendering.
func TestFormatReviewMRSecurityMarkdown(t *testing.T) {
	r := ReviewMRSecurityOutput{
		MRIID: 1, Title: "feat: auth",
		Review: "No security issues found", Model: "gpt-4o",
	}
	md := FormatReviewMRSecurityMarkdown(r)
	checks := []string{"## Security Review: !1", "feat: auth", "No security issues found", "*Model: gpt-4o*"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("FormatReviewMRSecurityMarkdown missing %q", c)
		}
	}
}

// TestFormatReviewMRSecurityMarkdown_Truncated verifies truncation warning.
func TestFormatReviewMRSecurityMarkdown_Truncated(t *testing.T) {
	r := ReviewMRSecurityOutput{Title: "x", Truncated: true}
	md := FormatReviewMRSecurityMarkdown(r)
	if !strings.Contains(md, "truncated") {
		t.Error("missing truncation warning")
	}
}

// TestReviewMRSecurity_EmptyProjectID verifies project_id validation.
func TestReviewMRSecurity_EmptyProjectID(t *testing.T) {
	_, err := ReviewMRSecurity(context.Background(), &mcp.CallToolRequest{}, nil, ReviewMRSecurityInput{
		ProjectID: "", MRIID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Errorf("error = %v, want project_id validation error", err)
	}
}

// TestReviewMRSecurity_InvalidMRIID verifies mr_iid validation.
func TestReviewMRSecurity_InvalidMRIID(t *testing.T) {
	_, err := ReviewMRSecurity(context.Background(), &mcp.CallToolRequest{}, nil, ReviewMRSecurityInput{
		ProjectID: "42", MRIID: 0,
	})
	if err == nil || !strings.Contains(err.Error(), "mr_iid") {
		t.Errorf("error = %v, want mr_iid validation error", err)
	}
}

// TestReviewMRSecurity_SamplingNotSupported verifies ErrSamplingNotSupported.
func TestReviewMRSecurity_SamplingNotSupported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	req := &mcp.CallToolRequest{}
	_, err := ReviewMRSecurity(context.Background(), req, client, ReviewMRSecurityInput{
		ProjectID: "42", MRIID: 1,
	})
	if !errors.Is(err, sampling.ErrSamplingNotSupported) {
		t.Errorf("error = %v, want %v", err, sampling.ErrSamplingNotSupported)
	}
}

// TestReviewMRSecurity_MRNotFound verifies error wrapping on 404.
func TestReviewMRSecurity_MRNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))
	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := ReviewMRSecurity(ctx, req, client, ReviewMRSecurityInput{
		ProjectID: "42", MRIID: 999,
	})
	if err == nil || !strings.Contains(err.Error(), "fetching MR") {
		t.Errorf("error = %v, want 'fetching MR' context", err)
	}
}

// TestReviewMRSecurity_FullFlow verifies the complete MR security review flow.
func TestReviewMRSecurity_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/merge_requests/1", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": 100, "iid": 1, "title": "feat: auth", "state": "opened",
			"source_branch": "feature/auth", "target_branch": "main",
			"author": {"username": "alice"}, "merge_status": "can_be_merged"
		}`)
	})
	mux.HandleFunc("/api/v4/projects/42/merge_requests/1/diffs", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{
			"old_path": "auth.go", "new_path": "auth.go",
			"diff": "@@ -1 +1 @@\n-old\n+new",
			"new_file": false, "deleted_file": false, "renamed_file": false
		}]`)
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := ReviewMRSecurity(ctx, req, client, ReviewMRSecurityInput{
		ProjectID: "42", MRIID: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.MRIID != 1 {
		t.Errorf("MRIID = %d, want 1", out.MRIID)
	}
	if out.Title != "feat: auth" {
		t.Errorf("Title = %q, want %q", out.Title, "feat: auth")
	}
	if out.Model != testModelName {
		t.Errorf("Model = %q, want %q", out.Model, testModelName)
	}
	if out.Review == "" {
		t.Error("Review is empty")
	}
}

// TestReviewMRSecurity_LLMError covers review_mr_security.go:92-94.
func TestReviewMRSecurity_LLMError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/merge_requests/1/diffs", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("/api/v4/projects/42/merge_requests/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"iid":1,"title":"feat"}`)
	})
	client := testutil.NewTestClient(t, mux)
	ctx := context.Background()
	_, ss, cleanup := setupFailingSamplingSession(t, ctx)
	defer cleanup()

	_, err := ReviewMRSecurity(ctx, &mcp.CallToolRequest{Session: ss}, client, ReviewMRSecurityInput{ProjectID: "42", MRIID: 1})
	if err == nil || !strings.Contains(err.Error(), "LLM security review") {
		t.Errorf("error = %v, want 'LLM security review' context", err)
	}
}

// TestReviewMRSecurity_MRGetError covers review_mr_security.go:82-84.
func TestReviewMRSecurity_MRGetError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))
	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	_, err := ReviewMRSecurity(ctx, &mcp.CallToolRequest{Session: ss}, client, ReviewMRSecurityInput{ProjectID: "42", MRIID: 1})
	if err == nil || !strings.Contains(err.Error(), "fetching MR") {
		t.Errorf("error = %v, want 'fetching MR' context", err)
	}
}

// TestReviewMRSecurity_ChangesGetError covers review_mr_security.go:82-84
// (MR GET succeeds but mrchanges.Get returns error).
func TestReviewMRSecurity_ChangesGetError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/merge_requests/5", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"iid":5,"title":"Fix","state":"opened"}`)
	})
	mux.HandleFunc("/api/v4/projects/42/merge_requests/5/diffs", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	_, err := ReviewMRSecurity(ctx, &mcp.CallToolRequest{Session: ss}, client, ReviewMRSecurityInput{ProjectID: "42", MRIID: 5})
	if err == nil || !strings.Contains(err.Error(), "fetching MR changes") {
		t.Errorf("error = %v, want 'fetching MR changes' context", err)
	}
}
