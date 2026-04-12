// find_technical_debt_test.go contains unit tests for the samplingtools MCP tool handlers.
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/search"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestFormatTechnicalDebtForAnalysis verifies the Markdown output from blobs.
func TestFormatTechnicalDebtForAnalysis(t *testing.T) {
	blobs := []search.BlobOutput{
		{Path: "main.go", Startline: 10, Data: "// TODO: refactor this"},
		{Path: "auth.go", Startline: 25, Data: "// FIXME: handle error"},
	}
	result := FormatTechnicalDebtForAnalysis(blobs)
	checks := []struct {
		name, want string
	}{
		{"header", "# Technical Debt Markers (2 results)"},
		{"path1", "### main.go (line 10)"},
		{"data1", "TODO: refactor this"},
		{"path2", "### auth.go (line 25)"},
		{"data2", "FIXME: handle error"},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.want) {
			t.Errorf("FormatTechnicalDebtForAnalysis missing %s: want %q", c.name, c.want)
		}
	}
}

// TestFormatFindTechnicalDebtMarkdown verifies technical debt analysis rendering.
func TestFormatFindTechnicalDebtMarkdown(t *testing.T) {
	f := FindTechnicalDebtOutput{
		Analysis: "Found 5 TODO items", Model: "gpt-4o",
	}
	md := FormatFindTechnicalDebtMarkdown(f)
	checks := []string{"## Technical Debt Analysis", "Found 5 TODO items", "*Model: gpt-4o*"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("FormatFindTechnicalDebtMarkdown missing %q", c)
		}
	}
}

// TestFormatFindTechnicalDebtMarkdown_Truncated verifies truncation warning.
func TestFormatFindTechnicalDebtMarkdown_Truncated(t *testing.T) {
	f := FindTechnicalDebtOutput{Truncated: true}
	md := FormatFindTechnicalDebtMarkdown(f)
	if !strings.Contains(md, "truncated") {
		t.Error("missing truncation warning")
	}
}

// TestFindTechnicalDebt_EmptyProjectID verifies project_id validation.
func TestFindTechnicalDebt_EmptyProjectID(t *testing.T) {
	_, err := FindTechnicalDebt(context.Background(), &mcp.CallToolRequest{}, nil, FindTechnicalDebtInput{
		ProjectID: "",
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Errorf("error = %v, want project_id validation error", err)
	}
}

// TestFindTechnicalDebt_SamplingNotSupported verifies ErrSamplingNotSupported.
func TestFindTechnicalDebt_SamplingNotSupported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	req := &mcp.CallToolRequest{}
	_, err := FindTechnicalDebt(context.Background(), req, client, FindTechnicalDebtInput{
		ProjectID: "42",
	})
	if !errors.Is(err, sampling.ErrSamplingNotSupported) {
		t.Errorf("error = %v, want %v", err, sampling.ErrSamplingNotSupported)
	}
}

// TestFindTechnicalDebt_NoResults verifies early return with static message
// when no debt markers are found in any search.
func TestFindTechnicalDebt_NoResults(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/-/search", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := FindTechnicalDebt(ctx, req, client, FindTechnicalDebtInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.Analysis, "No technical debt markers") {
		t.Errorf("Analysis = %q, want static 'No technical debt markers' message", out.Analysis)
	}
	if out.Model != "" {
		t.Errorf("Model = %q, want empty (no LLM call)", out.Model)
	}
}

// TestFindTechnicalDebt_FullFlow verifies the complete technical debt analysis flow.
func TestFindTechnicalDebt_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/-/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("search")
		if q == "TODO" {
			testutil.RespondJSON(w, http.StatusOK, `[{"basename": "main.go", "data": "// TODO: refactor", "path": "main.go", "filename": "main.go", "ref": "main", "startline": 10, "project_id": 42}]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := FindTechnicalDebt(ctx, req, client, FindTechnicalDebtInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Model != testModelName {
		t.Errorf("Model = %q, want %q", out.Model, testModelName)
	}
	if out.Analysis == "" {
		t.Error("Analysis is empty")
	}
}
