// prompt_errors_test.go contains unit tests for error handling across all
// MCP prompt handlers. It covers two categories: API error responses (404,
// 403, 401, 500) and missing required arguments. It also tests the
// changeType helper and additional branch coverage for specific prompts.
package prompts

import (
	"context"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Shared test assertion messages for prompt error tests.
const (
	msgExpectedAPIErr             = "expected error for API failure"
	msgExpectedMissingProjectID   = "expected error for missing project_id"
	promptSummarizePipelineStatus = "summarize_pipeline_status"
	msgHandlerNoCallMissingArgs   = "handler should not be called with missing args"
)

// Prompt API error tests.

// TestSummarizeMRChangesPrompt_APIError verifies that the
// summarize_mr_changes prompt returns an error when the GitLab diffs API
// responds with 404 Not Found.
func TestSummarizeMRChangesPrompt_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "summarize_mr_changes",
		Arguments: map[string]string{"project_id": "42", "merge_request_iid": "1"},
	})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestReviewMRPrompt_APIError verifies that the review_mr prompt returns an
// error when the GitLab MR API responds with 404 Not Found.
func TestReviewMRPrompt_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "review_mr",
		Arguments: map[string]string{"project_id": "42", "merge_request_iid": "1"},
	})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSummarizePipelineStatusPrompt_APIError verifies that the
// summarize_pipeline_status prompt returns an error when the GitLab
// pipelines API responds with 404 Not Found.
func TestSummarizePipelineStatusPrompt_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      promptSummarizePipelineStatus,
		Arguments: map[string]string{"project_id": "42"},
	})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSuggestMRReviewersPrompt_APIError verifies that the
// suggest_mr_reviewers prompt returns an error when the GitLab MR API
// responds with 404 Not Found.
func TestSuggestMRReviewersPrompt_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "suggest_mr_reviewers",
		Arguments: map[string]string{"project_id": "42", "merge_request_iid": "1"},
	})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestGenerateReleaseNotesPrompt_APIError verifies that the
// generate_release_notes prompt returns an error when the GitLab repository
// compare API responds with 404 Not Found.
func TestGenerateReleaseNotesPrompt_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "generate_release_notes",
		Arguments: map[string]string{"project_id": "42", "tag": "v1.0"},
	})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSummarizeOpenMRsPrompt_APIError verifies that the summarize_open_mrs
// prompt returns an error when the GitLab merge requests API responds with
// 403 Forbidden.
func TestSummarizeOpenMRsPrompt_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "summarize_open_mrs",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestProjectHealthCheckPrompt_APIError verifies that the
// project_health_check prompt returns an error when the GitLab project API
// responds with 404 Not Found.
func TestProjectHealthCheckPrompt_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "project_health_check",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestCompareBranchesPrompt_APIError verifies that the compare_branches
// prompt returns an error when the GitLab repository compare API responds
// with 404 Not Found.
func TestCompareBranchesPrompt_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "compare_branches",
		Arguments: map[string]string{"project_id": "42", "from": "main", "to": "dev"},
	})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestDailyStandupPrompt_APIError verifies that the daily_standup prompt
// returns an error when the GitLab user API responds with 401 Unauthorized.
func TestDailyStandupPrompt_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "daily_standup",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestMRRiskAssessmentPrompt_APIError verifies that the mr_risk_assessment
// prompt returns an error when the GitLab MR API responds with 404 Not
// Found.
func TestMRRiskAssessmentPrompt_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "mr_risk_assessment",
		Arguments: map[string]string{"project_id": "42", "merge_request_iid": "1"},
	})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// Missing args tests.

// TestSummarizePipelineStatusPrompt_MissingArgs verifies that the
// summarize_pipeline_status prompt returns an error when the project_id
// argument is empty.
func TestSummarizePipelineStatusPrompt_MissingArgs(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(msgHandlerNoCallMissingArgs)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      promptSummarizePipelineStatus,
		Arguments: map[string]string{"project_id": ""},
	})
	if err == nil {
		t.Fatal(msgExpectedMissingProjectID)
	}
}

// TestSuggestMRReviewersPrompt_MissingArgs verifies that the
// suggest_mr_reviewers prompt returns an error when the project_id argument
// is empty.
func TestSuggestMRReviewersPrompt_MissingArgs(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(msgHandlerNoCallMissingArgs)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "suggest_mr_reviewers",
		Arguments: map[string]string{"project_id": ""},
	})
	if err == nil {
		t.Fatal(msgExpectedMissingProjectID)
	}
}

// TestGenerateReleaseNotesPrompt_MissingArgs verifies that the
// generate_release_notes prompt returns an error when the project_id
// argument is empty.
func TestGenerateReleaseNotesPrompt_MissingArgs(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(msgHandlerNoCallMissingArgs)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "generate_release_notes",
		Arguments: map[string]string{"project_id": ""},
	})
	if err == nil {
		t.Fatal(msgExpectedMissingProjectID)
	}
}

// TestSummarizeOpenMRsPrompt_MissingArgs verifies that the
// summarize_open_mrs prompt returns an error when the project_id argument
// is empty.
func TestSummarizeOpenMRsPrompt_MissingArgs(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(msgHandlerNoCallMissingArgs)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "summarize_open_mrs",
		Arguments: map[string]string{"project_id": ""},
	})
	if err == nil {
		t.Fatal(msgExpectedMissingProjectID)
	}
}

// TestProjectHealthCheckPrompt_MissingArgs verifies that the
// project_health_check prompt returns an error when the project_id argument
// is empty.
func TestProjectHealthCheckPrompt_MissingArgs(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(msgHandlerNoCallMissingArgs)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "project_health_check",
		Arguments: map[string]string{"project_id": ""},
	})
	if err == nil {
		t.Fatal(msgExpectedMissingProjectID)
	}
}

// TestCompareBranchesPrompt_MissingArgs verifies that the compare_branches
// prompt returns an error when the required from argument is empty.
func TestCompareBranchesPrompt_MissingArgs(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(msgHandlerNoCallMissingArgs)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "compare_branches",
		Arguments: map[string]string{"project_id": "42", "from": "", "to": "dev"},
	})
	if err == nil {
		t.Fatal("expected error for missing from")
	}
}

// TestDailyStandupPrompt_MissingArgs verifies that the daily_standup prompt
// returns an error when the project_id argument is empty.
func TestDailyStandupPrompt_MissingArgs(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(msgHandlerNoCallMissingArgs)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "daily_standup",
		Arguments: map[string]string{"project_id": ""},
	})
	if err == nil {
		t.Fatal(msgExpectedMissingProjectID)
	}
}

// TestMRRiskAssessmentPrompt_MissingArgs verifies that the
// mr_risk_assessment prompt returns an error when the project_id argument
// is empty.
func TestMRRiskAssessmentPrompt_MissingArgs(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(msgHandlerNoCallMissingArgs)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "mr_risk_assessment",
		Arguments: map[string]string{"project_id": ""},
	})
	if err == nil {
		t.Fatal(msgExpectedMissingProjectID)
	}
}

// changeType branch coverage.

// TestChangeType_AllBranches uses table-driven subtests to verify that
// changeType returns the correct label for new, renamed, deleted, and
// modified files.
func TestChangeType_AllBranches(t *testing.T) {
	tests := []struct {
		name string
		diff *gl.MergeRequestDiff
		want string
	}{
		{
			name: "new file",
			diff: &gl.MergeRequestDiff{NewFile: true},
			want: "new file",
		},
		{
			name: "renamed file",
			diff: &gl.MergeRequestDiff{RenamedFile: true},
			want: "renamed",
		},
		{
			name: "deleted file",
			diff: &gl.MergeRequestDiff{DeletedFile: true},
			want: "deleted",
		},
		{
			name: "modified file",
			diff: &gl.MergeRequestDiff{},
			want: "modified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := changeType(tt.diff)
			if got != tt.want {
				t.Errorf("changeType = %q, want %q", got, tt.want)
			}
		})
	}
}

// DailyStandupPrompt with explicit username.

// TestDailyStandupPrompt_WithUsername verifies that the daily_standup prompt
// resolves an explicit username via the users API and returns a non-empty
// standup report.
func TestDailyStandupPrompt_WithUsername(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/users":
			respondJSON(w, http.StatusOK, `[{"id":42,"username":"testuser"}]`)
		case "/api/v4/users/42/events":
			respondJSON(w, http.StatusOK, `[]`)
		case "/api/v4/projects/42/merge_requests":
			respondJSON(w, http.StatusOK, `[]`)
		case "/api/v4/projects/42/issues":
			respondJSON(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "daily_standup",
		Arguments: map[string]string{"project_id": "42", "username": "testuser"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if text == "" {
		t.Error("expected non-empty prompt result")
	}
}

// SummarizePipelineStatus with job branches.

// TestSummarizePipeline_StatusPromptJobStatusBranches verifies that the
// summarize_pipeline_status prompt correctly handles mixed job statuses
// including failed, success, and canceled jobs.
func TestSummarizePipeline_StatusPromptJobStatusBranches(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/42/pipelines/latest":
			respondJSON(w, http.StatusOK, `{"id":100,"iid":10,"status":"failed","ref":"main","sha":"abc12345","web_url":"https://example.com/p/100","source":"push"}`)
		case "/api/v4/projects/42/pipelines/100/jobs":
			respondJSON(w, http.StatusOK, `[
{"id":1,"name":"test","stage":"test","status":"failed","ref":"main","failure_reason":"script_failure"},
{"id":2,"name":"build","stage":"build","status":"success","ref":"main"},
{"id":3,"name":"deploy","stage":"deploy","status":"canceled","ref":"main"}
]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      promptSummarizePipelineStatus,
		Arguments: map[string]string{"project_id": "42"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if text == "" {
		t.Error("expected non-empty prompt result")
	}
}

// ReviewMR missing args.

// TestReviewMRPrompt_MissingArgs verifies that the review_mr prompt returns
// an error when the project_id argument is empty.
func TestReviewMRPrompt_MissingArgs(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(msgHandlerNoCallMissingArgs)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "review_mr",
		Arguments: map[string]string{"project_id": ""},
	})
	if err == nil {
		t.Fatal(msgExpectedMissingProjectID)
	}
}
