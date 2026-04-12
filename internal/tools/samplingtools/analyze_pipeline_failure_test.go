// analyze_pipeline_failure_test.go contains unit tests for the samplingtools MCP tool handlers.
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestFormatPipelineFailureForAnalysis verifies the Markdown document produced
// from pipeline details and failed job traces contains expected sections.
func TestFormatPipelineFailureForAnalysis(t *testing.T) {
	pipeline := pipelines.DetailOutput{
		ID: 100, Status: "failed", Ref: "main",
		SHA: "abc123", Source: "push", Duration: 120,
		YamlErrors: "some yaml error",
	}
	traces := []JobTrace{
		{
			Job:   jobs.Output{ID: 1, Name: "build", Stage: "build", Status: "failed", FailureReason: "script_failure", Duration: 30.5},
			Trace: "error: compilation failed\nexit code 1",
		},
	}
	result := FormatPipelineFailureForAnalysis(pipeline, traces)
	checks := []struct {
		name, want string
	}{
		{"header", "# Pipeline #100 — failed"},
		{"ref", "**Ref**: main"},
		{"sha", "**SHA**: abc123"},
		{"source", "**Source**: push"},
		{"duration", "**Duration**: 120s"},
		{"yaml_errors", "**YAML Errors**: some yaml error"},
		{"failed_jobs_section", "## Failed Jobs (1)"},
		{"job_name", "### build (stage: build)"},
		{"failure_reason", "**Failure Reason**: script_failure"},
		{"trace_content", "compilation failed"},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.want) {
			t.Errorf("FormatPipelineFailureForAnalysis missing %s: want %q", c.name, c.want)
		}
	}
}

// TestFormatAnalyzePipelineFailureMarkdown verifies pipeline failure analysis rendering.
func TestFormatAnalyzePipelineFailureMarkdown(t *testing.T) {
	a := AnalyzePipelineFailureOutput{
		PipelineID: 100, Status: "failed", Ref: "main",
		Analysis: "Root cause: compilation error", Model: "gpt-4o",
	}
	md := FormatAnalyzePipelineFailureMarkdown(a)
	checks := []string{"## Pipeline Failure Analysis: #100 (main)", "Root cause: compilation error", "*Model: gpt-4o*"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("FormatAnalyzePipelineFailureMarkdown missing %q", c)
		}
	}
}

// TestFormatAnalyzePipelineFailureMarkdown_Truncated verifies truncation warning.
func TestFormatAnalyzePipelineFailureMarkdown_Truncated(t *testing.T) {
	a := AnalyzePipelineFailureOutput{PipelineID: 1, Ref: "x", Truncated: true}
	md := FormatAnalyzePipelineFailureMarkdown(a)
	if !strings.Contains(md, "truncated") {
		t.Error("missing truncation warning")
	}
}

// TestAnalyzePipelineFailure_EmptyProjectID verifies project_id validation.
func TestAnalyzePipelineFailure_EmptyProjectID(t *testing.T) {
	_, err := AnalyzePipelineFailure(context.Background(), &mcp.CallToolRequest{}, nil, AnalyzePipelineFailureInput{
		ProjectID:  "",
		PipelineID: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Errorf("error = %v, want project_id validation error", err)
	}
}

// TestAnalyzePipelineFailure_InvalidPipelineID verifies pipeline_id validation.
func TestAnalyzePipelineFailure_InvalidPipelineID(t *testing.T) {
	_, err := AnalyzePipelineFailure(context.Background(), &mcp.CallToolRequest{}, nil, AnalyzePipelineFailureInput{
		ProjectID:  "42",
		PipelineID: 0,
	})
	if err == nil || !strings.Contains(err.Error(), "pipeline_id") {
		t.Errorf("error = %v, want pipeline_id validation error", err)
	}
}

// TestAnalyzePipelineFailure_SamplingNotSupported verifies the tool returns
// ErrSamplingNotSupported when the client does not support sampling.
func TestAnalyzePipelineFailure_SamplingNotSupported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	req := &mcp.CallToolRequest{}
	_, err := AnalyzePipelineFailure(context.Background(), req, client, AnalyzePipelineFailureInput{
		ProjectID: "42", PipelineID: 100,
	})
	if !errors.Is(err, sampling.ErrSamplingNotSupported) {
		t.Errorf("error = %v, want %v", err, sampling.ErrSamplingNotSupported)
	}
}

// TestAnalyzePipelineFailure_PipelineNotFound verifies error wrapping when
// the pipeline API returns 404.
func TestAnalyzePipelineFailure_PipelineNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))
	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := AnalyzePipelineFailure(ctx, req, client, AnalyzePipelineFailureInput{
		ProjectID: "42", PipelineID: 999,
	})
	if err == nil || !strings.Contains(err.Error(), "fetching pipeline") {
		t.Errorf("error = %v, want 'fetching pipeline' context", err)
	}
}

// TestAnalyzePipelineFailure_FullFlow verifies the complete pipeline failure
// analysis flow: pipeline details, failed jobs, job traces, and LLM analysis.
func TestAnalyzePipelineFailure_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/pipelines/100", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": 100, "iid": 10, "project_id": 42, "status": "failed",
			"ref": "main", "sha": "abc123", "source": "push", "duration": 120
		}`)
	})
	mux.HandleFunc("/api/v4/projects/42/pipelines/100/jobs", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{
			"id": 501, "name": "build", "stage": "build", "status": "failed",
			"failure_reason": "script_failure", "duration": 30.5, "ref": "main",
			"web_url": "https://gitlab.example.com/jobs/501", "pipeline_id": 100
		}]`)
	})
	mux.HandleFunc("/api/v4/projects/42/jobs/501/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Step 1/5: build\nerror: compilation failed\nexit code 1"))
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := AnalyzePipelineFailure(ctx, req, client, AnalyzePipelineFailureInput{
		ProjectID: "42", PipelineID: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.PipelineID != 100 {
		t.Errorf("PipelineID = %d, want 100", out.PipelineID)
	}
	if out.Status != "failed" {
		t.Errorf("Status = %q, want %q", out.Status, "failed")
	}
	if out.Model != testModelName {
		t.Errorf("Model = %q, want %q", out.Model, testModelName)
	}
	if out.Analysis == "" {
		t.Error("Analysis is empty")
	}
}
