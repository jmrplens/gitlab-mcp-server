// analyze_deployment_history_test.go contains unit tests for the samplingtools MCP tool handlers.
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deployments"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestFormatDeploymentHistoryForAnalysis verifies deployment history Markdown output.
func TestFormatDeploymentHistoryForAnalysis(t *testing.T) {
	depList := deployments.ListOutput{
		Deployments: []deployments.Output{
			{ID: 1, Status: "success", Ref: "main", SHA: "abc123", EnvironmentName: "production", UserName: "alice", CreatedAt: "2026-01-15T10:00:00Z"},
			{ID: 2, Status: "failed", Ref: "main", SHA: "def456", EnvironmentName: "production", UserName: "bob", CreatedAt: "2026-01-16T10:00:00Z"},
		},
	}
	result := FormatDeploymentHistoryForAnalysis(depList, "production")
	checks := []struct {
		name, want string
	}{
		{"header", "# Deployment History — production (2 deployments)"},
		{"success_count", "**Success**: 1"},
		{"failed_count", "**Failed**: 1"},
		{"deployment1", "#1"},
		{"deployment2", "#2"},
		{"status_success", "[success]"},
		{"status_failed", "[failed]"},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.want) {
			t.Errorf("FormatDeploymentHistoryForAnalysis missing %s: want %q", c.name, c.want)
		}
	}
}

// TestFormatDeploymentHistoryForAnalysis_NoEnv verifies output without environment filter.
func TestFormatDeploymentHistoryForAnalysis_NoEnv(t *testing.T) {
	depList := deployments.ListOutput{
		Deployments: []deployments.Output{
			{ID: 1, Status: "success", Ref: "main", SHA: "a"},
		},
	}
	result := FormatDeploymentHistoryForAnalysis(depList, "")
	if !strings.Contains(result, "# Deployment History (1 deployments)") {
		t.Error("missing header without environment")
	}
}

// TestFormatAnalyzeDeploymentHistoryMarkdown verifies deployment analysis rendering.
func TestFormatAnalyzeDeploymentHistoryMarkdown(t *testing.T) {
	a := AnalyzeDeploymentHistoryOutput{
		Environment: "production", Analysis: "Deployment frequency is stable", Model: "gpt-4o",
	}
	md := FormatAnalyzeDeploymentHistoryMarkdown(a)
	checks := []string{"## Deployment History Analysis — production", "Deployment frequency is stable", "*Model: gpt-4o*"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("FormatAnalyzeDeploymentHistoryMarkdown missing %q", c)
		}
	}
}

// TestFormatAnalyzeDeploymentHistoryMarkdown_Truncated verifies truncation warning.
func TestFormatAnalyzeDeploymentHistoryMarkdown_Truncated(t *testing.T) {
	a := AnalyzeDeploymentHistoryOutput{Truncated: true}
	md := FormatAnalyzeDeploymentHistoryMarkdown(a)
	if !strings.Contains(md, "truncated") {
		t.Error("missing truncation warning")
	}
}

// TestAnalyzeDeploymentHistory_EmptyProjectID verifies project_id validation.
func TestAnalyzeDeploymentHistory_EmptyProjectID(t *testing.T) {
	_, err := AnalyzeDeploymentHistory(context.Background(), &mcp.CallToolRequest{}, nil, AnalyzeDeploymentHistoryInput{
		ProjectID: "",
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Errorf("error = %v, want project_id validation error", err)
	}
}

// TestAnalyzeDeploymentHistory_SamplingNotSupported verifies ErrSamplingNotSupported.
func TestAnalyzeDeploymentHistory_SamplingNotSupported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	req := &mcp.CallToolRequest{}
	_, err := AnalyzeDeploymentHistory(context.Background(), req, client, AnalyzeDeploymentHistoryInput{
		ProjectID: "42",
	})
	if !errors.Is(err, sampling.ErrSamplingNotSupported) {
		t.Errorf("error = %v, want %v", err, sampling.ErrSamplingNotSupported)
	}
}

// TestAnalyzeDeploymentHistory_APIError verifies error wrapping on 404.
func TestAnalyzeDeploymentHistory_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))
	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := AnalyzeDeploymentHistory(ctx, req, client, AnalyzeDeploymentHistoryInput{ProjectID: "42"})
	if err == nil || !strings.Contains(err.Error(), "fetching deployments") {
		t.Errorf("error = %v, want 'fetching deployments' context", err)
	}
}

// TestAnalyzeDeploymentHistory_NoDeployments verifies early return with static
// message when no deployments exist.
func TestAnalyzeDeploymentHistory_NoDeployments(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/deployments", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := AnalyzeDeploymentHistory(ctx, req, client, AnalyzeDeploymentHistoryInput{
		ProjectID: "42", Environment: "staging",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.Analysis, "No deployments found") {
		t.Errorf("Analysis = %q, want 'No deployments found' message", out.Analysis)
	}
	if !strings.Contains(out.Analysis, "staging") {
		t.Errorf("Analysis = %q, want environment name in message", out.Analysis)
	}
	if out.Model != "" {
		t.Errorf("Model = %q, want empty (no LLM call)", out.Model)
	}
}

// TestAnalyzeDeploymentHistory_FullFlow verifies the complete deployment analysis flow.
func TestAnalyzeDeploymentHistory_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/deployments", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id": 1, "iid": 1, "ref": "main", "sha": "abc123", "status": "success", "environment": {"name": "production"}, "user": {"username": "alice"}, "created_at": "2026-01-15T10:00:00Z"},
			{"id": 2, "iid": 2, "ref": "main", "sha": "def456", "status": "failed", "environment": {"name": "production"}, "user": {"username": "bob"}, "created_at": "2026-01-16T10:00:00Z"}
		]`)
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := AnalyzeDeploymentHistory(ctx, req, client, AnalyzeDeploymentHistoryInput{
		ProjectID: "42",
	})
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

// TestAnalyzeDeploymentHistory_LLMError covers analyze_deployment_history.go:92-94.
func TestAnalyzeDeploymentHistory_LLMError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/deployments", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"status":"success","ref":"main","environment":{"name":"prod"}}]`)
	})
	client := testutil.NewTestClient(t, mux)
	ctx := context.Background()
	_, ss, cleanup := setupFailingSamplingSession(t, ctx)
	defer cleanup()

	_, err := AnalyzeDeploymentHistory(ctx, &mcp.CallToolRequest{Session: ss}, client, AnalyzeDeploymentHistoryInput{ProjectID: "42"})
	if err == nil || !strings.Contains(err.Error(), "LLM analysis") {
		t.Errorf("error = %v, want 'LLM analysis' context", err)
	}
}
