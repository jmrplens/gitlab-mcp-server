// analyze_mr_changes_test.go contains unit tests for the samplingtools MCP tool handlers.
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrchanges"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatMRForAnalysis tests.

// TestFormatMRForAnalysis_Basic verifies that FormatMRForAnalysis produces a
// Markdown document containing the MR title, state, branches, merge status,
// description, changed file list, and diff block for a standard modified file.
func TestFormatMRForAnalysis_Basic(t *testing.T) {
	mr := mergerequests.Output{
		IID:          1,
		Title:        testMRTitle,
		State:        "opened",
		SourceBranch: "feature/login",
		TargetBranch: "main",
		MergeStatus:  "can_be_merged",
		Description:  "Implements login screen",
	}
	changes := mrchanges.Output{
		Changes: []mrchanges.FileDiffOutput{
			{
				OldPath:     "main.go",
				NewPath:     "main.go",
				Diff:        "@@ -1,3 +1,5 @@\n+import \"fmt\"",
				NewFile:     false,
				DeletedFile: false,
				RenamedFile: false,
			},
		},
	}

	result := FormatMRForAnalysis(mr, changes)

	checks := []struct {
		name string
		want string
	}{
		{"title", "# Merge Request !1: feat: add login"},
		{"state", "**State**: opened"},
		{"branches", "**Source Branch**: feature/login → main"},
		{"merge status", "**Merge Status**: can_be_merged"},
		{"description", mdSectionDescription},
		{"description text", "Implements login screen"},
		{"changed files", "## Changed Files (1)"},
		{"file path", "### main.go (modified)"},
		{"diff block", "```diff"},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.want) {
			t.Errorf("FormatMRForAnalysis missing %s: want substring %q", c.name, c.want)
		}
	}
}

// TestFormatMRForAnalysis_NewFile verifies that FormatMRForAnalysis marks a
// newly added file with the "(added)" label.
func TestFormatMRForAnalysis_NewFile(t *testing.T) {
	mr := mergerequests.Output{IID: 2, Title: "add file"}
	changes := mrchanges.Output{
		Changes: []mrchanges.FileDiffOutput{
			{NewPath: "new.go", NewFile: true, Diff: "+package new"},
		},
	}
	result := FormatMRForAnalysis(mr, changes)
	if !strings.Contains(result, "(added)") {
		t.Error("new file not marked as 'added'")
	}
}

// TestFormatMRForAnalysis_DeletedFile verifies that FormatMRForAnalysis marks
// a removed file with the "(deleted)" label.
func TestFormatMRForAnalysis_DeletedFile(t *testing.T) {
	mr := mergerequests.Output{IID: 3, Title: "remove file"}
	changes := mrchanges.Output{
		Changes: []mrchanges.FileDiffOutput{
			{NewPath: "old.go", DeletedFile: true, Diff: "-package old"},
		},
	}
	result := FormatMRForAnalysis(mr, changes)
	if !strings.Contains(result, "(deleted)") {
		t.Error("deleted file not marked as 'deleted'")
	}
}

// TestFormatMRForAnalysis_RenamedFile verifies that FormatMRForAnalysis marks
// a renamed file with the "renamed from <old_path>" label.
func TestFormatMRForAnalysis_RenamedFile(t *testing.T) {
	mr := mergerequests.Output{IID: 4, Title: "rename file"}
	changes := mrchanges.Output{
		Changes: []mrchanges.FileDiffOutput{
			{OldPath: "old_name.go", NewPath: "new_name.go", RenamedFile: true, Diff: ""},
		},
	}
	result := FormatMRForAnalysis(mr, changes)
	if !strings.Contains(result, "renamed from old_name.go") {
		t.Error("renamed file not marked correctly")
	}
}

// TestFormatMRForAnalysis_EmptyChanges verifies that FormatMRForAnalysis
// correctly reports "Changed Files (0)" when there are no file diffs.
func TestFormatMRForAnalysis_EmptyChanges(t *testing.T) {
	mr := mergerequests.Output{IID: 5, Title: "no changes"}
	changes := mrchanges.Output{Changes: []mrchanges.FileDiffOutput{}}
	result := FormatMRForAnalysis(mr, changes)
	if !strings.Contains(result, "Changed Files (0)") {
		t.Error("empty changes not reflected correctly")
	}
}

// TestFormatMRForAnalysis_NoDescription verifies that FormatMRForAnalysis
// omits the Description section when the MR description is empty.
func TestFormatMRForAnalysis_NoDescription(t *testing.T) {
	mr := mergerequests.Output{IID: 6, Title: "no desc", Description: ""}
	changes := mrchanges.Output{}
	result := FormatMRForAnalysis(mr, changes)
	if strings.Contains(result, mdSectionDescription) {
		t.Error("empty description should not produce Description section")
	}
}

// TestAnalyzeMRChanges_EmptyProjectID verifies that AnalyzeMRChanges returns
// a validation error when project_id is empty.
func TestAnalyzeMRChanges_EmptyProjectID(t *testing.T) {
	_, err := AnalyzeMRChanges(context.Background(), &mcp.CallToolRequest{}, nil, AnalyzeMRChangesInput{
		ProjectID: "",
		MRIID:     1,
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Errorf("AnalyzeMRChanges() error = %v, want project_id validation error", err)
	}
}

// TestAnalyzeMRChanges_InvalidMRIID verifies that AnalyzeMRChanges returns
// a validation error when mr_iid is zero or negative.
func TestAnalyzeMRChanges_InvalidMRIID(t *testing.T) {
	_, err := AnalyzeMRChanges(context.Background(), &mcp.CallToolRequest{}, nil, AnalyzeMRChangesInput{
		ProjectID: "42",
		MRIID:     0,
	})
	if err == nil || !strings.Contains(err.Error(), "mr_iid") {
		t.Errorf("AnalyzeMRChanges() error = %v, want mr_iid validation error", err)
	}
}

// TestAnalyzeMRChanges_SamplingNotSupported verifies that AnalyzeMRChanges
// returns [sampling.ErrSamplingNotSupported] when the MCP client does not
// support the sampling capability.
func TestAnalyzeMRChanges_SamplingNotSupported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	req := &mcp.CallToolRequest{}
	_, err := AnalyzeMRChanges(context.Background(), req, client, AnalyzeMRChangesInput{
		ProjectID: "42",
		MRIID:     1,
	})
	if !errors.Is(err, sampling.ErrSamplingNotSupported) {
		t.Errorf("AnalyzeMRChanges() error = %v, want %v", err, sampling.ErrSamplingNotSupported)
	}
}

// TestAnalyzeMRChanges_MRNotFound verifies that AnalyzeMRChanges returns an
// error with "fetching MR" context when the GitLab API responds with 404.
func TestAnalyzeMRChanges_MRNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := AnalyzeMRChanges(ctx, req, client, AnalyzeMRChangesInput{
		ProjectID: "42",
		MRIID:     9999,
	})
	if err == nil {
		t.Fatal("AnalyzeMRChanges() expected error for 404 MR, got nil")
	}
	if !strings.Contains(err.Error(), "fetching MR") {
		t.Errorf("error = %v, want 'fetching MR' context", err)
	}
}

// TestAnalyzeMRChanges_FullFlow verifies the complete MR analysis flow:
// fetching MR details and diffs from a mocked GitLab API, then delegating to
// a mocked sampling session for LLM analysis. Asserts the output contains the
// MR title, model name, and non-empty analysis text.
func TestAnalyzeMRChanges_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/merge_requests/1", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": 100, "iid": 1, "title": "feat: add login",
			"description": "Login screen", "state": "opened",
			"source_branch": "feature/login", "target_branch": "main",
			"web_url": "https://gitlab.example.com/mr/1", "merge_status": "can_be_merged"
		}`)
	})
	mux.HandleFunc("/api/v4/projects/42/merge_requests/1/diffs", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{
			"old_path": "main.go", "new_path": "main.go",
			"diff": "@@ -1 +1 @@\n-old\n+new",
			"new_file": false, "deleted_file": false, "renamed_file": false
		}]`)
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := AnalyzeMRChanges(ctx, req, client, AnalyzeMRChangesInput{
		ProjectID: "42",
		MRIID:     1,
	})
	if err != nil {
		t.Fatalf("AnalyzeMRChanges() unexpected error: %v", err)
	}
	if out.MRIID != 1 {
		t.Errorf("out.MRIID = %d, want 1", out.MRIID)
	}
	if out.Title != testMRTitle {
		t.Errorf("out.Title = %q, want %q", out.Title, testMRTitle)
	}
	if out.Model != testModelName {
		t.Errorf("out.Model = %q, want %q", out.Model, testModelName)
	}
	if out.Analysis == "" {
		t.Error("out.Analysis is empty")
	}
}

// TestFormatAnalyzeMRChangesMarkdown verifies MR analysis rendering.
func TestFormatAnalyzeMRChangesMarkdown(t *testing.T) {
	a := AnalyzeMRChangesOutput{
		MRIID: 42, Title: "feat: add feature", Analysis: "This MR adds a new feature.",
		Model: "gpt-4o", Truncated: false,
	}
	md := FormatAnalyzeMRChangesMarkdown(a)
	checks := []string{"## MR Analysis: !42", "feat: add feature", "This MR adds a new feature.", "*Model: gpt-4o*"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("FormatAnalyzeMRChangesMarkdown missing %q", c)
		}
	}
}

// TestFormatAnalyzeMRChangesMarkdown_Truncated verifies truncation warning.
func TestFormatAnalyzeMRChangesMarkdown_Truncated(t *testing.T) {
	a := AnalyzeMRChangesOutput{MRIID: 1, Title: "x", Analysis: "text", Truncated: true}
	md := FormatAnalyzeMRChangesMarkdown(a)
	if !strings.Contains(md, "truncated") {
		t.Error("missing truncation warning")
	}
}

// TestAnalyzeMRChanges_LLMError covers analyze_mr_changes.go:92-94.
func TestAnalyzeMRChanges_LLMError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/merge_requests/1/diffs", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("/api/v4/projects/42/merge_requests/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"iid":1,"title":"feat"}`)
	})
	mux.HandleFunc("/api/graphql", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondGraphQL(w, http.StatusOK, mrContextJSON)
	})
	client := testutil.NewTestClient(t, mux)
	ctx := context.Background()
	_, ss, cleanup := setupFailingSamplingSession(t, ctx)
	defer cleanup()

	_, err := AnalyzeMRChanges(ctx, &mcp.CallToolRequest{Session: ss}, client, AnalyzeMRChangesInput{ProjectID: "42", MRIID: 1})
	if err == nil || !strings.Contains(err.Error(), "LLM analysis") {
		t.Errorf("error = %v, want 'LLM analysis' context", err)
	}
}

// TestAnalyzeMRChanges_RESTFallback_MRGetError covers analyze_mr_changes.go:82-84.
func TestAnalyzeMRChanges_RESTFallback_MRGetError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/merge_requests/1/diffs", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	// No GraphQL handler → BuildMRContext fails.
	// No MR GET handler → mergerequests.Get fails with 404.
	client := testutil.NewTestClient(t, mux)
	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	_, err := AnalyzeMRChanges(ctx, &mcp.CallToolRequest{Session: ss}, client, AnalyzeMRChangesInput{ProjectID: "42", MRIID: 1})
	if err == nil || !strings.Contains(err.Error(), "fetching MR") {
		t.Errorf("error = %v, want 'fetching MR' context", err)
	}
}
