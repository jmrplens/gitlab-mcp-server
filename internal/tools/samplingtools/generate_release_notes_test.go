// generate_release_notes_test.go contains unit tests for the samplingtools MCP tool handlers.
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/repository"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestFormatReleaseDataForAnalysis_Basic validates format release data for analysis basic across multiple scenarios using table-driven subtests.
func TestFormatReleaseDataForAnalysis_Basic(t *testing.T) {
	cmp := repository.CompareOutput{
		Commits: []commits.Output{
			{ID: "abc12345678", ShortID: "abc12345", Title: "feat: add login", AuthorName: "alice"},
			{ID: "def98765432", ShortID: "def98765", Title: "fix: password hash", AuthorName: "bob"},
		},
		Diffs: []repository.DiffOutput{
			{NewPath: "auth.go"},
			{NewPath: "auth_test.go"},
		},
	}
	mrs := mergerequests.ListOutput{
		MergeRequests: []mergerequests.Output{
			{IID: 10, Title: "feat: add login screen", Author: "alice", Labels: []string{"feature"}, Description: "Implements login"},
			{IID: 11, Title: "fix: password hashing", Author: "bob", Labels: []string{"bug"}, Description: ""},
		},
	}

	result := FormatReleaseDataForAnalysis("v1.0.0", "v1.1.0", cmp, mrs)

	checks := []struct {
		name string
		want string
	}{
		{"header", "# Release: v1.0.0 → v1.1.0"},
		{"mr count", "## Merged MRs (2)"},
		{"mr entry", "!10 — feat: add login screen (@alice) [feature]"},
		{"mr description", "> Implements login"},
		{"mr no-label", "!11 — fix: password hashing (@bob)"},
		{"commit count", "## Commits (2)"},
		{"commit entry", "abc12345 — feat: add login (alice)"},
		{"files count", "## Files Changed (2)"},
		{"file entry", "auth.go"},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.want) {
			t.Errorf("FormatReleaseDataForAnalysis missing %s: want %q", c.name, c.want)
		}
	}
}

// TestFormatReleaseDataForAnalysis_NoMRs verifies the output omits the MR
// section when no merge requests are provided.
func TestFormatReleaseDataForAnalysis_NoMRs(t *testing.T) {
	cmp := repository.CompareOutput{
		Commits: []commits.Output{
			{ID: "abc12345678", ShortID: "abc12345", Title: "initial", AuthorName: "bob"},
		},
	}
	result := FormatReleaseDataForAnalysis("v1.0.0", "v1.1.0", cmp, mergerequests.ListOutput{})
	if strings.Contains(result, "## Merged MRs") {
		t.Error("empty MRs should not produce Merged MRs section")
	}
	if !strings.Contains(result, "## Commits (1)") {
		t.Error("missing commits section")
	}
}

// TestFormatReleaseDataForAnalysis_LongDescription verifies that MR descriptions
// are truncated at 200 characters.
func TestFormatReleaseDataForAnalysis_LongDescription(t *testing.T) {
	longDesc := strings.Repeat("x", 300)
	cmp := repository.CompareOutput{}
	mrs := mergerequests.ListOutput{
		MergeRequests: []mergerequests.Output{
			{IID: 1, Title: "long desc", Author: "alice", Description: longDesc},
		},
	}
	result := FormatReleaseDataForAnalysis("a", "b", cmp, mrs)
	if !strings.Contains(result, "...") {
		t.Error("long description should be truncated with ellipsis")
	}
}

// TestFormatReleaseDataForAnalysis_CommitFallbackSHA verifies that when ShortID
// is empty, the first 8 chars of ID are used as the short SHA.
func TestFormatReleaseDataForAnalysis_CommitFallbackSHA(t *testing.T) {
	cmp := repository.CompareOutput{
		Commits: []commits.Output{
			{ID: "abcdef1234567890", ShortID: "", Title: "commit msg", AuthorName: "alice"},
		},
	}
	result := FormatReleaseDataForAnalysis("a", "b", cmp, mergerequests.ListOutput{})
	if !strings.Contains(result, "abcdef12") {
		t.Error("should use first 8 chars of ID when ShortID is empty")
	}
}

// FormatGenerateReleaseNotesMarkdown tests.

// TestFormatGenerateReleaseNotesMarkdown verifies basic rendering.
func TestFormatGenerateReleaseNotesMarkdown(t *testing.T) {
	r := GenerateReleaseNotesOutput{
		From: "v1.0.0", To: "v1.1.0",
		ReleaseNotes: "### Features\n- Added login",
		Model:        "gpt-4o", Truncated: false,
	}
	md := FormatGenerateReleaseNotesMarkdown(r)
	checks := []string{"## Release Notes: v1.0.0 → v1.1.0", "### Features", "Added login", "*Model: gpt-4o*"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("FormatGenerateReleaseNotesMarkdown missing %q", c)
		}
	}
}

// TestFormatGenerateReleaseNotesMarkdown_Truncated verifies truncation warning.
func TestFormatGenerateReleaseNotesMarkdown_Truncated(t *testing.T) {
	r := GenerateReleaseNotesOutput{From: "a", To: "b", ReleaseNotes: "notes", Truncated: true}
	md := FormatGenerateReleaseNotesMarkdown(r)
	if !strings.Contains(md, "truncated") {
		t.Error("missing truncation warning")
	}
}

// GenerateReleaseNotes input validation tests.

// TestGenerateReleaseNotes_EmptyProjectID verifies project_id validation.
func TestGenerateReleaseNotes_EmptyProjectID(t *testing.T) {
	_, err := GenerateReleaseNotes(context.Background(), &mcp.CallToolRequest{}, nil, GenerateReleaseNotesInput{
		ProjectID: "",
		From:      "v1.0.0",
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Errorf("GenerateReleaseNotes() error = %v, want project_id validation error", err)
	}
}

// TestGenerateReleaseNotes_EmptyFrom verifies from validation.
func TestGenerateReleaseNotes_EmptyFrom(t *testing.T) {
	_, err := GenerateReleaseNotes(context.Background(), &mcp.CallToolRequest{}, nil, GenerateReleaseNotesInput{
		ProjectID: "42",
		From:      "",
	})
	if err == nil || !strings.Contains(err.Error(), "from") {
		t.Errorf("GenerateReleaseNotes() error = %v, want from validation error", err)
	}
}

// TestGenerateReleaseNotes_SamplingNotSupported verifies the tool returns
// ErrSamplingNotSupported when the client does not support sampling.
func TestGenerateReleaseNotes_SamplingNotSupported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	req := &mcp.CallToolRequest{}
	_, err := GenerateReleaseNotes(context.Background(), req, client, GenerateReleaseNotesInput{
		ProjectID: "42",
		From:      "v1.0.0",
	})
	if !errors.Is(err, sampling.ErrSamplingNotSupported) {
		t.Errorf("GenerateReleaseNotes() error = %v, want %v", err, sampling.ErrSamplingNotSupported)
	}
}

// TestGenerateReleaseNotes_CompareError verifies error wrapping when the
// compare API fails.
func TestGenerateReleaseNotes_CompareError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := GenerateReleaseNotes(ctx, req, client, GenerateReleaseNotesInput{
		ProjectID: "42",
		From:      "v1.0.0",
		To:        "v1.1.0",
	})
	if err == nil {
		t.Fatal("expected error for compare failure")
	}
	if !strings.Contains(err.Error(), "comparing refs") {
		t.Errorf("error = %v, want 'comparing refs' context", err)
	}
}

// TestGenerateReleaseNotes_FullFlow verifies the complete release notes
// generation flow: compare refs, fetch MRs, delegate to LLM.
func TestGenerateReleaseNotes_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/repository/compare", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"commits": [
				{"id": "abc123", "short_id": "abc123", "title": "feat: login", "author_name": "alice", "authored_date": "2024-06-01T10:00:00Z"},
				{"id": "def456", "short_id": "def456", "title": "fix: auth", "author_name": "bob", "authored_date": "2024-06-02T10:00:00Z"}
			],
			"diffs": [
				{"old_path": "auth.go", "new_path": "auth.go", "diff": "@@ -1 +1 @@\n-old\n+new"}
			],
			"web_url": "https://gitlab.example.com/compare/v1.0.0...v1.1.0"
		}`)
	})
	mux.HandleFunc("/api/v4/projects/42/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"iid": 10, "title": "feat: add login", "state": "merged", "author": {"username": "alice"}, "labels": ["feature"]},
			{"iid": 11, "title": "fix: auth bug", "state": "merged", "author": {"username": "bob"}, "labels": ["bug"]}
		]`)
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := GenerateReleaseNotes(ctx, req, client, GenerateReleaseNotesInput{
		ProjectID: "42",
		From:      "v1.0.0",
		To:        "v1.1.0",
	})
	if err != nil {
		t.Fatalf("GenerateReleaseNotes() unexpected error: %v", err)
	}
	if out.From != "v1.0.0" {
		t.Errorf("out.From = %q, want %q", out.From, "v1.0.0")
	}
	if out.To != "v1.1.0" {
		t.Errorf("out.To = %q, want %q", out.To, "v1.1.0")
	}
	if out.Model != testModelName {
		t.Errorf("out.Model = %q, want %q", out.Model, testModelName)
	}
	if out.ReleaseNotes == "" {
		t.Error("out.ReleaseNotes is empty")
	}
}

// TestGenerateReleaseNotes_DefaultTo verifies that "to" defaults to HEAD when empty.
func TestGenerateReleaseNotes_DefaultTo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/repository/compare", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("to") != "HEAD" {
			t.Errorf("expected to=HEAD, got %q", r.URL.Query().Get("to"))
		}
		testutil.RespondJSON(w, http.StatusOK, `{"commits":[],"diffs":[]}`)
	})
	mux.HandleFunc("/api/v4/projects/42/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := GenerateReleaseNotes(ctx, req, client, GenerateReleaseNotesInput{
		ProjectID: "42",
		From:      "v1.0.0",
		To:        "",
	})
	if err != nil {
		t.Fatalf("GenerateReleaseNotes() unexpected error: %v", err)
	}
	if out.To != "HEAD" {
		t.Errorf("out.To = %q, want %q", out.To, "HEAD")
	}
}
