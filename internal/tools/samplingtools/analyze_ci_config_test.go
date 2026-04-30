// analyze_ci_config_test.go contains unit tests for the samplingtools MCP tool handlers.
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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cilint"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrapprovals"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrdiscussions"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestFormatCIConfigForAnalysis verifies CI config Markdown output.
func TestFormatCIConfigForAnalysis(t *testing.T) {
	lint := cilint.Output{
		Valid:      true,
		Errors:     []string{},
		Warnings:   []string{"job X has no rules"},
		MergedYaml: "stages:\n  - build\n  - test",
		Includes:   []cilint.Include{{Type: "remote", Location: "https://example.com/.ci.yml"}},
	}
	result := FormatCIConfigForAnalysis(lint)
	checks := []struct {
		name, want string
	}{
		{"header", "# CI/CD Configuration Analysis"},
		{"valid", "**Valid**: true"},
		{"warning", "job X has no rules"},
		{"includes_section", "## Includes (1)"},
		{"include_entry", "[remote] https://example.com/.ci.yml"},
		{"yaml_section", "## Merged YAML"},
		{"yaml_content", "stages:"},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.want) {
			t.Errorf("FormatCIConfigForAnalysis missing %s: want %q", c.name, c.want)
		}
	}
}

// TestFormatAnalyzeCIConfigMarkdown verifies CI config analysis rendering.
func TestFormatAnalyzeCIConfigMarkdown(t *testing.T) {
	a := AnalyzeCIConfigOutput{
		Valid: true, Analysis: "Config looks good", Model: "gpt-4o",
	}
	md := FormatAnalyzeCIConfigMarkdown(a)
	checks := []string{"## CI Configuration Analysis (Valid ✅)", "Config looks good", "*Model: gpt-4o*"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("FormatAnalyzeCIConfigMarkdown missing %q", c)
		}
	}
}

// TestFormatAnalyzeCIConfigMarkdown_Invalid verifies invalid status rendering.
func TestFormatAnalyzeCIConfigMarkdown_Invalid(t *testing.T) {
	a := AnalyzeCIConfigOutput{Valid: false, Analysis: "errors found"}
	md := FormatAnalyzeCIConfigMarkdown(a)
	if !strings.Contains(md, "Invalid ❌") {
		t.Error("missing Invalid marker")
	}
}

// TestFormatAnalyzeCIConfigMarkdown_Truncated verifies truncation warning.
func TestFormatAnalyzeCIConfigMarkdown_Truncated(t *testing.T) {
	a := AnalyzeCIConfigOutput{Truncated: true}
	md := FormatAnalyzeCIConfigMarkdown(a)
	if !strings.Contains(md, "truncated") {
		t.Error("missing truncation warning")
	}
}

// TestAnalyzeCIConfig_EmptyProjectID verifies project_id validation.
func TestAnalyzeCIConfig_EmptyProjectID(t *testing.T) {
	_, err := AnalyzeCIConfig(context.Background(), &mcp.CallToolRequest{}, nil, AnalyzeCIConfigInput{
		ProjectID: "",
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Errorf("error = %v, want project_id validation error", err)
	}
}

// TestAnalyzeCIConfig_SamplingNotSupported verifies ErrSamplingNotSupported.
func TestAnalyzeCIConfig_SamplingNotSupported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	req := &mcp.CallToolRequest{}
	_, err := AnalyzeCIConfig(context.Background(), req, client, AnalyzeCIConfigInput{
		ProjectID: "42",
	})
	if !errors.Is(err, sampling.ErrSamplingNotSupported) {
		t.Errorf("error = %v, want %v", err, sampling.ErrSamplingNotSupported)
	}
}

// TestAnalyzeCIConfig_LintError verifies error wrapping when CI lint fails.
func TestAnalyzeCIConfig_LintError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
	}))
	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := AnalyzeCIConfig(ctx, req, client, AnalyzeCIConfigInput{ProjectID: "42"})
	if err == nil || !strings.Contains(err.Error(), "linting CI config") {
		t.Errorf("error = %v, want 'linting CI config' context", err)
	}
}

// TestIsMissingCIConfig_DetectsLintError validates detection of the "no .gitlab-ci.yml" lint error.
func TestIsMissingCIConfig_DetectsLintError(t *testing.T) {
	tests := []struct {
		name string
		errs []string
		want bool
	}{
		{"exact GitLab message", []string{"Please provide content of .gitlab-ci.yml"}, true},
		{"mixed case", []string{"please Provide Content of .gitlab-ci.yml"}, true},
		{"among other errors", []string{"syntax error", "Please provide content of .gitlab-ci.yml"}, true},
		{"unrelated error", []string{"unknown keyword 'deploy_stage'"}, false},
		{"empty errors", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMissingCIConfig(tt.errs)
			if got != tt.want {
				t.Errorf("isMissingCIConfig(%v) = %v, want %v", tt.errs, got, tt.want)
			}
		})
	}
}

// TestAnalyzeCIConfig_MissingCIFile verifies that the handler returns an error
// instead of wasting a sampling call when the project has no .gitlab-ci.yml.
func TestAnalyzeCIConfig_MissingCIFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/ci/lint", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"valid": false,
			"errors": ["Please provide content of .gitlab-ci.yml"],
			"warnings": [], "merged_yaml": "", "includes": []
		}`)
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	_, err := AnalyzeCIConfig(ctx, req, client, AnalyzeCIConfigInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for missing .gitlab-ci.yml, got nil")
	}
	if !strings.Contains(err.Error(), "no .gitlab-ci.yml") {
		t.Errorf("error = %q, want message about missing .gitlab-ci.yml", err.Error())
	}
}

// TestAnalyzeCIConfig_FullFlow verifies the complete CI config analysis flow.
func TestAnalyzeCIConfig_FullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/ci/lint", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"valid": true, "errors": [], "warnings": [],
			"merged_yaml": "stages:\n  - build", "includes": []
		}`)
	})
	client := testutil.NewTestClient(t, mux)

	ctx := context.Background()
	_, ss, cleanup := setupSamplingSession(t, ctx)
	defer cleanup()

	req := &mcp.CallToolRequest{Session: ss}
	out, err := AnalyzeCIConfig(ctx, req, client, AnalyzeCIConfigInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Valid {
		t.Error("Valid = false, want true")
	}
	if out.Model != testModelName {
		t.Errorf("Model = %q, want %q", out.Model, testModelName)
	}
	if out.Analysis == "" {
		t.Error("Analysis is empty")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// FormatIssueForSummary — note with empty CreatedAt → "unknown"
// ---------------------------------------------------------------------------.

// TestFormatIssueForSummary_NoteUnknownTime verifies the behavior of cov format issue for summary note unknown time.
func TestFormatIssueForSummary_NoteUnknownTime(t *testing.T) {
	issue := issues.Output{IID: 20, Title: "empty ts"}
	notes := issuenotes.ListOutput{
		Notes: []issuenotes.Output{
			{ID: 1, Author: "bob", Body: "some comment", CreatedAt: ""},
		},
	}
	result := FormatIssueForSummary(issue, notes)
	if !strings.Contains(result, "(unknown)") {
		t.Error("empty CreatedAt should render as 'unknown'")
	}
}

// ---------------------------------------------------------------------------
// FormatAnalyzeMRChangesMarkdown — empty model
// ---------------------------------------------------------------------------.

// TestFormatAnalyzeMRChangesMarkdown_EmptyModel verifies the behavior of cov format analyze m r changes markdown empty model.
func TestFormatAnalyzeMRChangesMarkdown_EmptyModel(t *testing.T) {
	a := AnalyzeMRChangesOutput{
		MRIID:    1,
		Title:    "test",
		Analysis: "analysis text",
		Model:    "",
	}
	md := FormatAnalyzeMRChangesMarkdown(a)
	if strings.Contains(md, "*Model:") {
		t.Error("empty model should not produce Model line")
	}
	if !strings.Contains(md, "analysis text") {
		t.Error("missing analysis text")
	}
}

// ---------------------------------------------------------------------------
// FormatSummarizeIssueMarkdown — truncated
// ---------------------------------------------------------------------------.

// TestFormatSummarizeIssueMarkdown_Truncated verifies the behavior of cov format summarize issue markdown truncated.
func TestFormatSummarizeIssueMarkdown_Truncated(t *testing.T) {
	s := SummarizeIssueOutput{
		IssueIID:  5,
		Title:     "big issue",
		Summary:   "summary",
		Model:     "claude-4",
		Truncated: true,
	}
	md := FormatSummarizeIssueMarkdown(s)
	if !strings.Contains(md, "truncated") {
		t.Error("missing truncation warning")
	}
	if !strings.Contains(md, "*Model: claude-4*") {
		t.Error("missing model")
	}
}

// ---------------------------------------------------------------------------
// FormatSummarizeIssueMarkdown — empty model
// ---------------------------------------------------------------------------.

// TestFormatSummarizeIssueMarkdown_EmptyModel verifies the behavior of cov format summarize issue markdown empty model.
func TestFormatSummarizeIssueMarkdown_EmptyModel(t *testing.T) {
	s := SummarizeIssueOutput{
		IssueIID: 6,
		Title:    "no model",
		Summary:  "summary text",
		Model:    "",
	}
	md := FormatSummarizeIssueMarkdown(s)
	if strings.Contains(md, "*Model:") {
		t.Error("empty model should not produce Model line")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools no-panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip — without sampling (covers unsupported path in register.go)
// ---------------------------------------------------------------------------.

// TestMCPRound_TripNoSampling validates cov m c p round trip no sampling across multiple scenarios using table-driven subtests.
func TestMCPRound_TripNoSampling(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_analyze_mr_changes", map[string]any{"project_id": "42", "merge_request_iid": float64(1)}},
		{"gitlab_summarize_issue", map[string]any{"project_id": "42", "issue_iid": float64(10)}},
		{"gitlab_generate_release_notes", map[string]any{"project_id": "42", "from": "v1.0.0", "to": "v2.0.0"}},
		{"gitlab_analyze_pipeline_failure", map[string]any{"project_id": "42", "pipeline_id": float64(1)}},
		{"gitlab_summarize_mr_review", map[string]any{"project_id": "42", "merge_request_iid": float64(1)}},
		{"gitlab_generate_milestone_report", map[string]any{"project_id": "42", "milestone_iid": float64(1)}},
		{"gitlab_analyze_ci_configuration", map[string]any{"project_id": "42", "content_ref": "main"}},
		{"gitlab_analyze_issue_scope", map[string]any{"project_id": "42", "issue_iid": float64(10)}},
		{"gitlab_review_mr_security", map[string]any{"project_id": "42", "merge_request_iid": float64(1)}},
		{"gitlab_find_technical_debt", map[string]any{"project_id": "42", "ref": "main"}},
		{"gitlab_analyze_deployment_history", map[string]any{"project_id": "42"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var res *mcp.CallToolResult
			res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool %s: %v", tc.name, err)
			}
			if res == nil {
				t.Fatalf("nil result for %s", tc.name)
			}
			if !res.IsError {
				t.Errorf("%s should return IsError=true (sampling unsupported)", tc.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MCP round-trip — with sampling (covers success path in register.go)
// ---------------------------------------------------------------------------.

// TestMCPRoundTrip_WithSampling validates cov m c p round trip with sampling across multiple scenarios using table-driven subtests.
func TestMCPRoundTrip_WithSampling(t *testing.T) {
	mux := http.NewServeMux()

	// MR endpoints (analyze_mr_changes, review_mr_security, summarize_mr_review).
	mux.HandleFunc("/api/v4/projects/42/merge_requests/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":100,"iid":1,"title":"feat: login",
			"description":"Login feature","state":"opened",
			"source_branch":"feature/login","target_branch":"main",
			"web_url":"https://gitlab.example.com/mr/1","merge_status":"can_be_merged"
		}`)
	})
	mux.HandleFunc("/api/v4/projects/42/merge_requests/1/diffs", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{
			"old_path":"main.go","new_path":"main.go",
			"diff":"@@ -1 +1 @@\n-old\n+new",
			"new_file":false,"deleted_file":false,"renamed_file":false
		}]`)
	})
	mux.HandleFunc("/api/v4/projects/42/merge_requests/1/discussions", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":"d1","notes":[{"id":1,"body":"LGTM","author":{"username":"bob"},"system":false,"created_at":"2026-01-15T10:00:00Z"}]}]`)
	})
	mux.HandleFunc("/api/v4/projects/42/merge_requests/1/approval_state", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"rules":[]}`)
	})

	// Issue endpoints (summarize_issue, analyze_issue_scope).
	mux.HandleFunc("/api/v4/projects/42/issues/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":200,"iid":10,"title":"Login bug",
			"description":"Login fails","state":"opened",
			"author":{"username":"alice"},
			"created_at":"2026-01-15T10:00:00Z",
			"web_url":"https://gitlab.example.com/issues/10"
		}`)
	})
	mux.HandleFunc("/api/v4/projects/42/issues/10/notes", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{
			"id":100,"body":"Looks good","author":{"username":"bob"},
			"system":false,"internal":false,
			"created_at":"2026-01-16T10:00:00Z","updated_at":"2026-01-16T10:00:00Z"
		}]`)
	})
	mux.HandleFunc("/api/v4/projects/42/issues/10/time_stats", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"human_time_estimate":"2h","human_total_time_spent":"1h"}`)
	})
	mux.HandleFunc("/api/v4/projects/42/issues/10/participants", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"username":"alice"}]`)
	})
	mux.HandleFunc("/api/v4/projects/42/issues/10/closed_by", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("/api/v4/projects/42/issues/10/related_merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	// Compare endpoint (generate_release_notes).
	mux.HandleFunc("/api/v4/projects/42/repository/compare", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"commits":[{"id":"abc","short_id":"abc","title":"feat: add login","author_name":"alice","committed_date":"2026-01-15T00:00:00Z","web_url":"u"}],"diffs":[],"compare_timeout":false,"compare_same_ref":false,"web_url":"u"}`)
	})
	mux.HandleFunc("/api/v4/projects/42/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	// Pipeline endpoint (analyze_pipeline_failure).
	mux.HandleFunc("/api/v4/projects/42/pipelines/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"project_id":42,"status":"failed","ref":"main","sha":"abc","source":"push","web_url":"https://gitlab.example.com/pipeline/1","created_at":"2026-01-15T10:00:00Z","user":{"username":"alice"}}`)
	})
	mux.HandleFunc("/api/v4/projects/42/pipelines/1/jobs", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"build","stage":"build","status":"failed","ref":"main","web_url":"https://gitlab.example.com/job/10","pipeline":{"id":1},"created_at":"2026-01-15T10:00:00Z","user":{"username":"alice"}}]`)
	})
	mux.HandleFunc("/api/v4/projects/42/jobs/10/trace", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ERROR: build failed"))
	})

	// Milestone endpoints (generate_milestone_report) — uses two-step IID→ID resolution.
	mux.HandleFunc("/api/v4/projects/42/milestones", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"title":"v1.0","state":"active"}]`)
	})
	mux.HandleFunc("/api/v4/projects/42/milestones/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"v1.0","state":"active"}`)
	})
	mux.HandleFunc("/api/v4/projects/42/milestones/1/issues", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	mux.HandleFunc("/api/v4/projects/42/milestones/1/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	// CI lint endpoint (analyze_ci_configuration).
	mux.HandleFunc("/api/v4/projects/42/ci/lint", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"valid":true,"errors":[],"warnings":[],"merged_yaml":"stages:\n  - build","includes":[]}`)
	})

	// Search endpoint (find_technical_debt).
	mux.HandleFunc("/api/v4/projects/42/search", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	// Deployments endpoint (analyze_deployment_history).
	mux.HandleFunc("/api/v4/projects/42/deployments", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"ref":"main","sha":"abc","status":"success","created_at":"2026-01-15T00:00:00Z","updated_at":"2026-01-15T01:00:00Z","user":{"username":"alice"},"environment":{"name":"production"}}]`)
	})

	gitlabClient := testutil.NewTestClient(t, mux)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, gitlabClient)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, &mcp.ClientOptions{
		CreateMessageHandler: func(_ context.Context, _ *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Model:   testModelName,
				Content: &mcp.TextContent{Text: "LLM mock analysis response"},
			}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_analyze_mr_changes", map[string]any{"project_id": "42", "merge_request_iid": float64(1)}},
		{"gitlab_summarize_issue", map[string]any{"project_id": "42", "issue_iid": float64(10)}},
		{"gitlab_generate_release_notes", map[string]any{"project_id": "42", "from": "v1.0.0", "to": "v2.0.0"}},
		{"gitlab_analyze_pipeline_failure", map[string]any{"project_id": "42", "pipeline_id": float64(1)}},
		{"gitlab_summarize_mr_review", map[string]any{"project_id": "42", "merge_request_iid": float64(1)}},
		{"gitlab_generate_milestone_report", map[string]any{"project_id": "42", "milestone_iid": float64(1)}},
		{"gitlab_analyze_ci_configuration", map[string]any{"project_id": "42", "content_ref": "main"}},
		{"gitlab_analyze_issue_scope", map[string]any{"project_id": "42", "issue_iid": float64(10)}},
		{"gitlab_review_mr_security", map[string]any{"project_id": "42", "merge_request_iid": float64(1)}},
		{"gitlab_find_technical_debt", map[string]any{"project_id": "42", "ref": "main"}},
		{"gitlab_analyze_deployment_history", map[string]any{"project_id": "42"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var res *mcp.CallToolResult
			res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool %s: %v", tc.name, err)
			}
			if res == nil {
				t.Fatalf("nil result for %s", tc.name)
			}
			if res.IsError {
				t.Errorf("%s should not return IsError=true", tc.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatCIConfigForAnalysis — errors, ContextProject, large YAML truncation
// ---------------------------------------------------------------------------.

// TestFormatCIConfig_Errors verifies the Errors branch and ContextProject
// branch in FormatCIConfigForAnalysis, plus the large YAML truncation path.
func TestFormatCIConfig_Errors(t *testing.T) {
	lint := cilint.Output{
		Valid:    false,
		Errors:   []string{"unknown keyword 'deploy_stage'", "invalid syntax in job 'build'"},
		Warnings: []string{},
		Includes: []cilint.Include{
			{Type: "project", Location: ".ci-template.yml", ContextProject: "group/infra"},
			{Type: "local", Location: ".gitlab-ci.yml"},
		},
		MergedYaml: strings.Repeat("x", 50001),
	}
	result := FormatCIConfigForAnalysis(lint)
	checks := []struct {
		name, want string
	}{
		{"errors_section", "## Errors (2)"},
		{"error_entry", "unknown keyword 'deploy_stage'"},
		{"include_with_project", "(from group/infra)"},
		{"include_without_project", "[local] .gitlab-ci.yml"},
		{"yaml_truncated", "... (truncated)"},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.want) {
			t.Errorf("missing %s: want %q in output", c.name, c.want)
		}
	}
	// Warnings section must NOT appear (empty slice).
	if strings.Contains(result, "## Warnings") {
		t.Error("empty warnings should not produce a Warnings section")
	}
}

// ---------------------------------------------------------------------------
// FormatDeploymentHistoryForAnalysis — "other" status and empty env name
// ---------------------------------------------------------------------------.

// TestFormatDeployment_HistoryOtherStatus verifies the "other" counter and
// the empty EnvironmentName → "unknown" fallback.
func TestFormatDeployment_HistoryOtherStatus(t *testing.T) {
	depList := deployments.ListOutput{
		Deployments: []deployments.Output{
			{ID: 1, Status: "running", Ref: "main", SHA: "abc", EnvironmentName: "", UserName: "eve", CreatedAt: "2026-01-15"},
		},
	}
	result := FormatDeploymentHistoryForAnalysis(depList, "")
	if !strings.Contains(result, "**Other**: 1") {
		t.Error("missing Other count for non-success/non-failed status")
	}
	if !strings.Contains(result, "env=unknown") {
		t.Error("empty EnvironmentName should render as 'unknown'")
	}
}

// ---------------------------------------------------------------------------
// FormatMRReviewForAnalysis — unapproved rule, system note, unresolved note
// ---------------------------------------------------------------------------.

// TestFormatMR_ReviewBranches verifies the unapproved rule, system note
// skip, unresolved discussion, and non-resolvable note branches.
func TestFormatMR_ReviewBranches(t *testing.T) {
	mr := mergerequests.Output{
		IID: 99, Title: "refactor", State: "opened",
		Author: "alice", SourceBranch: "feat", TargetBranch: "main",
	}
	discussions := mrdiscussions.ListOutput{
		Discussions: []mrdiscussions.Output{
			{
				ID: "d1",
				Notes: []mrdiscussions.NoteOutput{
					{Author: "sys", Body: "approved", System: true},
					{Author: "bob", Body: "needs work", Resolvable: true, Resolved: false, CreatedAt: "2026-01-15"},
					{Author: "carol", Body: "general comment", Resolvable: false, CreatedAt: "2026-01-16"},
				},
			},
		},
	}
	approvals := mrapprovals.StateOutput{
		Rules: []mrapprovals.RuleOutput{
			{Name: "Security", Approved: false, ApprovalsRequired: 2, ApprovedByNames: []string{}},
		},
	}
	result := FormatMRReviewForAnalysis(mr, discussions, approvals)
	checks := []struct {
		name, want string
	}{
		{"unapproved", "❌ Not approved"},
		{"unresolved", "[UNRESOLVED]"},
		{"system_skipped_body", ""},
	}
	for _, c := range checks {
		if !strings.Contains(result, c.want) && c.want != "" {
			t.Errorf("missing %s: want %q", c.name, c.want)
		}
	}
	// System note body should NOT appear in output.
	if strings.Contains(result, "**sys**") {
		t.Error("system note should be skipped")
	}
	// Non-resolvable note should not have [RESOLVED] or [UNRESOLVED].
	for line := range strings.SplitSeq(result, "\n") {
		if strings.Contains(line, "carol") && (strings.Contains(line, "[RESOLVED]") || strings.Contains(line, "[UNRESOLVED]")) {
			t.Error("non-resolvable note should not have resolution tag")
		}
	}
}

// ---------------------------------------------------------------------------
// FormatIssueScopeForAnalysis — sparse issue with empty optional fields
// ---------------------------------------------------------------------------.

// TestFormatIssue_ScopeSparse verifies all the "empty" branches in
// FormatIssueScopeForAnalysis: no due date, no labels, no assignees,
// zero weight, empty time stats, no participants, empty description,
// no closing/related MRs, no notes.
func TestFormatIssue_ScopeSparse(t *testing.T) {
	issue := issues.Output{
		IID: 42, Title: "Minimal", State: "closed",
		Author: "alice", CreatedAt: "2026-01-01",
	}
	timeStats := issues.TimeStatsOutput{}
	participants := issues.ParticipantsOutput{}
	closingMRs := issues.RelatedMRsOutput{}
	relatedMRs := issues.RelatedMRsOutput{}
	notes := issuenotes.ListOutput{}

	result := FormatIssueScopeForAnalysis(issue, timeStats, participants, closingMRs, relatedMRs, notes)

	// Positive checks — these else-branches should appear.
	if !strings.Contains(result, "**Estimate**: not set") {
		t.Error("missing 'not set' for empty time estimate")
	}
	if !strings.Contains(result, "**Time Spent**: none recorded") {
		t.Error("missing 'none recorded' for empty time spent")
	}

	// Negative checks — these sections should NOT appear.
	absent := []string{"Due Date", "Labels", "Assignees", "Weight", "## Participants", "## Description", "## Closing MRs", "## Related MRs", "## Discussion"}
	for _, s := range absent {
		if strings.Contains(result, s) {
			t.Errorf("sparse issue should not contain %q", s)
		}
	}
}

// TestFormatIssueScopeNote_EmptyTimestamp verifies the note with empty
// CreatedAt renders as "unknown" in FormatIssueScopeForAnalysis.
func TestFormatIssueScopeNote_EmptyTimestamp(t *testing.T) {
	issue := issues.Output{IID: 50, Title: "ts test", State: "opened", Author: "a", CreatedAt: "2026-01-01"}
	notes := issuenotes.ListOutput{
		Notes: []issuenotes.Output{
			{Author: "bob", Body: "comment", CreatedAt: ""},
		},
	}
	result := FormatIssueScopeForAnalysis(issue, issues.TimeStatsOutput{}, issues.ParticipantsOutput{},
		issues.RelatedMRsOutput{}, issues.RelatedMRsOutput{}, notes)
	if !strings.Contains(result, "(unknown)") {
		t.Error("empty CreatedAt should render as 'unknown'")
	}
}

// TestAnalyzeCIConfig_LLMError covers analyze_ci_config.go:75-77.
func TestAnalyzeCIConfig_LLMError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/ci/lint", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"valid":true,"errors":[],"warnings":[],"merged_yaml":"x","includes":[]}`)
	})
	client := testutil.NewTestClient(t, mux)
	ctx := context.Background()
	_, ss, cleanup := setupFailingSamplingSession(t, ctx)
	defer cleanup()

	_, err := AnalyzeCIConfig(ctx, &mcp.CallToolRequest{Session: ss}, client, AnalyzeCIConfigInput{ProjectID: "42"})
	if err == nil || !strings.Contains(err.Error(), "LLM analysis") {
		t.Errorf("error = %v, want 'LLM analysis' context", err)
	}
}
