// prompts_test.go contains unit tests for the happy-path behavior of each
// MCP prompt handler. Tests use httptest to mock the GitLab API and verify
// that prompt responses contain expected content, formatting, and structure.
package prompts

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Test endpoint paths and reusable format strings shared across prompt tests.
const (
	pathMR5Diffs     = "/api/v4/projects/42/merge_requests/5/diffs"
	fmtUnexpectedErr = "unexpected error: %v"
	pathMR5          = "/api/v4/projects/42/merge_requests/5"
	pathRepoCompare  = "/api/v4/projects/42/repository/compare"
	pathMRs          = "/api/v4/projects/42/merge_requests"
	pathIssues       = "/api/v4/projects/42/issues"
	pathUsers        = "/api/v4/users"
	testHelloWorld   = "hello world"
)

// TestSummarizeMRChangesPrompt_Success verifies that the summarize_mr_changes
// prompt returns formatted diff summaries including file names and change
// types (e.g., "new file") when the GitLab diffs API responds successfully.
func TestSummarizeMRChangesPrompt_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathMR5Diffs {
			respondJSON(w, http.StatusOK, `[{"old_path":"main.go","new_path":"main.go","diff":"@@ -1 +1 @@\n-old\n+new","new_file":false,"renamed_file":false,"deleted_file":false},{"old_path":"","new_path":"README.md","diff":"","new_file":true,"renamed_file":false,"deleted_file":false}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "summarize_mr_changes",
		Arguments: map[string]string{"project_id": "42", "mr_iid": "5"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "main.go") {
		t.Errorf("expected output to contain 'main.go', got: %s", text)
	}
	if !strings.Contains(text, "new file") {
		t.Errorf("expected output to contain 'new file' for README.md")
	}
}

// TestSummarizeMRChangesPrompt_MissingArgs verifies that the
// summarize_mr_changes prompt returns an error when the required mr_iid
// argument is missing from the request.
func TestSummarizeMRChangesPrompt_MissingArgs(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "summarize_mr_changes",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err == nil {
		t.Fatal("expected error for missing mr_iid")
	}
}

// TestReviewMRPrompt_Success verifies that the review_mr prompt returns a
// structured code review containing the MR title, changed file names, a
// review plan, a checklist, and metrics when both the MR and diffs APIs
// respond successfully.
func TestReviewMRPrompt_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathMR5:
			respondJSON(w, http.StatusOK, `{"id":55,"iid":5,"title":"Add feature X","source_branch":"feature-x","target_branch":"main","description":"A great feature","author":{"username":"alice"}}`)
		case pathMR5Diffs:
			respondJSON(w, http.StatusOK, `[{"old_path":"handler.go","new_path":"handler.go","diff":"@@ -10,3 +10,5 @@\n func handle() {\n+  log.Println(\"new\")\n }","new_file":false,"renamed_file":false,"deleted_file":false}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "review_mr",
		Arguments: map[string]string{"project_id": "42", "mr_iid": "5"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Code Review: Add feature X") {
		t.Errorf("expected review title, got: %s", text)
	}
	if !strings.Contains(text, "handler.go") {
		t.Errorf("expected changed file name in output")
	}
	if !strings.Contains(text, "Review Plan") {
		t.Errorf("expected review plan section")
	}
	if !strings.Contains(text, "Review Checklist") {
		t.Errorf("expected review checklist in output")
	}
	if !strings.Contains(text, "Lines added") {
		t.Errorf("expected metrics section")
	}
}

// TestReviewMR_PromptCategorizedFiles verifies that the review_mr prompt
// categorizes changed files into High-Risk, Business Logic, Tests, and
// Documentation groups, and that high-risk files appear before business
// logic in the output.
func TestReviewMR_PromptCategorizedFiles(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathMR5:
			respondJSON(w, http.StatusOK, `{"id":55,"iid":5,"title":"Mixed changes","source_branch":"feat","target_branch":"main","description":"","author":{"username":"alice"}}`)
		case pathMR5Diffs:
			respondJSON(w, http.StatusOK, `[
				{"old_path":".env.example","new_path":".env.example","diff":"+SECRET=x","new_file":false,"renamed_file":false,"deleted_file":false},
				{"old_path":"main.go","new_path":"main.go","diff":"+code","new_file":false,"renamed_file":false,"deleted_file":false},
				{"old_path":"main_test.go","new_path":"main_test.go","diff":"+test","new_file":false,"renamed_file":false,"deleted_file":false},
				{"old_path":"README.md","new_path":"README.md","diff":"+docs","new_file":false,"renamed_file":false,"deleted_file":false}
			]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "review_mr",
		Arguments: map[string]string{"project_id": "42", "mr_iid": "5"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "High-Risk Files (1)") {
		t.Errorf("expected 1 high-risk file (.env.example)")
	}
	if !strings.Contains(text, "Business Logic (1)") {
		t.Errorf("expected 1 business logic file (main.go)")
	}
	if !strings.Contains(text, "Tests (1)") {
		t.Errorf("expected 1 test file (main_test.go)")
	}
	if !strings.Contains(text, "Documentation (1)") {
		t.Errorf("expected 1 documentation file (README.md)")
	}
	// Verify high-risk appears before business logic in the output
	highRiskIdx := strings.Index(text, "High-Risk Files")
	logicIdx := strings.Index(text, "Business Logic")
	if highRiskIdx > logicIdx {
		t.Error("high-risk files should appear before business logic")
	}
}

// TestSummarizePipelineStatusPrompt_Success verifies that the
// summarize_pipeline_status prompt returns pipeline status, failure reasons,
// and a "Failed Jobs" section when the pipeline and jobs APIs report a
// failed pipeline.
func TestSummarizePipelineStatusPrompt_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/42/pipelines/latest":
			respondJSON(w, http.StatusOK, `{"id":100,"iid":10,"status":"failed","ref":"main","sha":"abc12345def","web_url":"https://gitlab.example.com/pipelines/100","source":"push"}`)
		case "/api/v4/projects/42/pipelines/100/jobs":
			respondJSON(w, http.StatusOK, `[{"id":201,"name":"lint","stage":"test","status":"success"},{"id":202,"name":"build","stage":"build","status":"failed","failure_reason":"script_failure"}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "summarize_pipeline_status",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "FAILED") {
		t.Errorf("expected pipeline status FAILED in output")
	}
	if !strings.Contains(text, "script_failure") {
		t.Errorf("expected failure reason in output")
	}
	if !strings.Contains(text, "Failed Jobs") {
		t.Errorf("expected Failed Jobs section")
	}
}

// TestSuggestMRReviewersPrompt_Success verifies that the suggest_mr_reviewers
// prompt includes active members (excluding the MR author and blocked users)
// as reviewer candidates.
func TestSuggestMRReviewersPrompt_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathMR5:
			respondJSON(w, http.StatusOK, `{"id":55,"iid":5,"title":"Fix bug","author":{"username":"alice"}}`)
		case pathMR5Diffs:
			respondJSON(w, http.StatusOK, `[{"old_path":"auth.go","new_path":"auth.go","diff":"","new_file":false,"renamed_file":false,"deleted_file":false}]`)
		case "/api/v4/projects/42/members/all":
			respondJSON(w, http.StatusOK, `[{"id":1,"username":"alice","name":"Alice","state":"active","access_level":40},{"id":2,"username":"bob","name":"Bob","state":"active","access_level":30},{"id":3,"username":"carol","name":"Carol","state":"blocked","access_level":30}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "suggest_mr_reviewers",
		Arguments: map[string]string{"project_id": "42", "mr_iid": "5"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "bob") {
		t.Errorf("expected bob as reviewer candidate")
	}
	if strings.Contains(text, "carol") {
		t.Errorf("blocked user carol should be excluded")
	}
}

// TestGenerateReleaseNotesPrompt_Success verifies that the
// generate_release_notes prompt returns formatted release notes containing
// the version range, commit titles, and commit count when the repository
// compare API responds successfully.
func TestGenerateReleaseNotesPrompt_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoCompare {
			respondJSON(w, http.StatusOK, `{"commits":[{"id":"abc12345def67890","title":"feat: add login\nDetails here","author_name":"Alice"},{"id":"def67890abc12345","title":"fix: typo","author_name":"Bob"}],"diffs":[{"new_path":"login.go","new_file":true},{"new_path":"README.md","new_file":false}],"compare_timeout":false,"compare_same_ref":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "generate_release_notes",
		Arguments: map[string]string{"project_id": "42", "from": "v1.0", "to": "v2.0"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "v1.0 → v2.0") {
		t.Errorf("expected release range in output")
	}
	if !strings.Contains(text, "feat: add login") {
		t.Errorf("expected commit title in output")
	}
	if !strings.Contains(text, "Commits (2)") {
		t.Errorf("expected commit count")
	}
}

// TestSummarizeOpenMRsPrompt_Success verifies that the summarize_open_mrs
// prompt returns a heading with the MR count and includes MR titles when
// the merge requests API responds with open MRs.
func TestSummarizeOpenMRsPrompt_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathMRs {
			respondJSON(w, http.StatusOK, `[{"id":55,"iid":5,"title":"Add feature","state":"opened","source_branch":"feature","target_branch":"main","author":{"username":"alice"},"created_at":"2025-01-01T00:00:00Z","detailed_merge_status":"mergeable"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "summarize_open_mrs",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Open Merge Requests (1)") {
		t.Errorf("expected open MR count heading")
	}
	if !strings.Contains(text, "Add feature") {
		t.Errorf("expected MR title in output")
	}
}

// TestProjectHealthCheckPrompt_Success verifies that the project_health_check
// prompt returns the project name, latest pipeline status, and branch
// statistics when the project, pipeline, MR, and branches APIs all respond
// successfully.
func TestProjectHealthCheckPrompt_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/42":
			respondJSON(w, http.StatusOK, `{"id":42,"name":"my-proj","path_with_namespace":"user/my-proj"}`)
		case "/api/v4/projects/42/pipelines/latest":
			respondJSON(w, http.StatusOK, `{"id":100,"status":"success","ref":"main","sha":"abc12345","web_url":"https://gitlab.example.com/pipelines/100"}`)
		case pathMRs:
			respondJSON(w, http.StatusOK, `[{"id":55,"iid":5,"title":"Open MR","state":"opened","author":{"username":"alice"},"created_at":"2025-01-01T00:00:00Z"}]`)
		case "/api/v4/projects/42/repository/branches":
			respondJSON(w, http.StatusOK, `[{"name":"main","protected":true,"merged":false,"default":true,"commit":{"committed_date":"2025-07-01T00:00:00Z"}},{"name":"old-branch","protected":false,"merged":true,"default":false,"commit":{"committed_date":"2024-01-01T00:00:00Z"}}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "project_health_check",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Project Health Check: user/my-proj") {
		t.Errorf("expected project name in health check heading")
	}
	if !strings.Contains(text, "Latest Pipeline: SUCCESS") {
		t.Errorf("expected pipeline status")
	}
	if !strings.Contains(text, "merged") {
		t.Errorf("expected branch stats")
	}
}

// TestCompareBranchesPrompt_Success verifies that the compare_branches prompt
// returns a heading with the branch names and lists changed files when the
// repository compare API responds with commits and diffs.
func TestCompareBranchesPrompt_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoCompare {
			respondJSON(w, http.StatusOK, `{"commits":[{"id":"aaa11111bbb22222","title":"commit msg","author_name":"Alice"}],"diffs":[{"new_path":"file.go","new_file":false,"deleted_file":false,"renamed_file":false}],"compare_timeout":false,"compare_same_ref":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "compare_branches",
		Arguments: map[string]string{"project_id": "42", "from": "main", "to": "develop"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "main → develop") {
		t.Errorf("expected branch comparison heading")
	}
	if !strings.Contains(text, "file.go") {
		t.Errorf("expected changed file in output")
	}
}

// TestCompareBranches_PromptSameRef verifies that the compare_branches prompt
// returns a "No differences" message when the from and to refs point to the
// same commit.
func TestCompareBranches_PromptSameRef(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoCompare {
			respondJSON(w, http.StatusOK, `{"commits":[],"diffs":[],"compare_timeout":false,"compare_same_ref":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "compare_branches",
		Arguments: map[string]string{"project_id": "42", "from": "main", "to": "main"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No differences") {
		t.Errorf("expected 'No differences' for same ref comparison")
	}
}

// TestDailyStandupPrompt_Success verifies that the daily_standup prompt
// returns a standup report with the username heading, recent events,
// authored MRs, and assigned issues when all APIs respond successfully.
func TestDailyStandupPrompt_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/user":
			respondJSON(w, http.StatusOK, `{"id":1,"username":"alice"}`)
		case "/api/v4/events":
			respondJSON(w, http.StatusOK, `[{"id":1,"action_name":"pushed to","target_type":"Project","target_title":"my-project"}]`)
		case pathMRs:
			// Return same MR for all filter variants
			respondJSON(w, http.StatusOK, `[{"id":55,"iid":5,"title":"WIP MR","state":"opened","source_branch":"feature","target_branch":"main","author":{"username":"alice"},"detailed_merge_status":"draft"}]`)
		case pathIssues:
			respondJSON(w, http.StatusOK, `[{"id":10,"iid":3,"title":"Bug fix","state":"opened","created_at":"2025-01-01T00:00:00Z","author":{"username":"alice"}}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "daily_standup",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Daily Standup for @alice") {
		t.Errorf("expected standup heading with username")
	}
	if !strings.Contains(text, "pushed to") {
		t.Errorf("expected event action in output")
	}
	if !strings.Contains(text, "WIP MR") {
		t.Errorf("expected authored MR in output")
	}
	if !strings.Contains(text, "Bug fix") {
		t.Errorf("expected issue in output")
	}
}

// TestDailyStandupPrompt_WithExplicitUsername verifies that the daily_standup
// prompt resolves an explicit username via the users API and includes that
// user's events in the standup report.
func TestDailyStandupPrompt_WithExplicitUsername(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathUsers:
			respondJSON(w, http.StatusOK, `[{"id":99,"username":"bob"}]`)
		case "/api/v4/users/99/events":
			respondJSON(w, http.StatusOK, `[{"id":2,"action_name":"commented on","target_type":"MergeRequest","target_title":"Fix typos"}]`)
		case pathMRs:
			respondJSON(w, http.StatusOK, `[]`)
		case pathIssues:
			respondJSON(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "daily_standup",
		Arguments: map[string]string{"project_id": "42", "username": "bob"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Daily Standup for @bob") {
		t.Errorf("expected standup heading with 'bob', got: %s", text)
	}
	if !strings.Contains(text, "commented on") {
		t.Errorf("expected event action in output")
	}
}

// TestDailyStandupPrompt_MissingProjectID verifies that the daily_standup
// prompt returns an error when the required project_id argument is missing.
func TestDailyStandupPrompt_MissingProjectID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "daily_standup",
		Arguments: map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestMRRiskAssessmentPrompt_Success verifies that the mr_risk_assessment
// prompt returns a risk assessment heading, conflict flag, and sensitive
// files metric when the MR and diffs APIs respond successfully.
func TestMRRiskAssessmentPrompt_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathMR5:
			respondJSON(w, http.StatusOK, `{"id":55,"iid":5,"title":"Refactor auth","has_conflicts":true,"author":{"username":"alice"}}`)
		case pathMR5Diffs:
			respondJSON(w, http.StatusOK, `[{"old_path":"auth/handler.go","new_path":"auth/handler.go","diff":"+line1\n+line2\n-old","new_file":false,"renamed_file":false,"deleted_file":false},{"old_path":".env.example","new_path":".env.example","diff":"+SECRET=abc","new_file":false,"renamed_file":false,"deleted_file":false}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "mr_risk_assessment",
		Arguments: map[string]string{"project_id": "42", "mr_iid": "5"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Risk Assessment") {
		t.Errorf("expected risk assessment heading")
	}
	if !strings.Contains(text, "Has conflicts**: true") {
		t.Errorf("expected conflict flag in output")
	}
	if !strings.Contains(text, "Sensitive files touched") {
		t.Errorf("expected sensitive files metric")
	}
}

// TestTeamMemberWorkload_Success verifies that the team_member_workload
// prompt returns a workload summary with the username heading, period,
// event type counts, MR titles, and issue titles when all APIs respond
// successfully.
func TestTeamMemberWorkload_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathUsers:
			respondJSON(w, http.StatusOK, `[{"id":99,"username":"carol"}]`)
		case "/api/v4/users/99/events":
			respondJSON(w, http.StatusOK, `[{"id":1,"action_name":"pushed to","target_type":"Project","target_title":"proj"},{"id":2,"action_name":"commented on","target_type":"MergeRequest","target_title":"mr1"}]`)
		case pathMRs:
			respondJSON(w, http.StatusOK, `[{"id":10,"iid":1,"title":"Feature A","state":"opened","source_branch":"feat-a","target_branch":"main","detailed_merge_status":"mergeable"}]`)
		case pathIssues:
			respondJSON(w, http.StatusOK, `[{"id":20,"iid":7,"title":"Task X","state":"opened","created_at":"2025-01-01T00:00:00Z","author":{"username":"carol"}}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "team_member_workload",
		Arguments: map[string]string{"project_id": "42", "username": "carol", "days": "14"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "Workload Summary for @carol") {
		t.Errorf("expected workload heading for carol")
	}
	if !strings.Contains(text, "last 14 days") {
		t.Errorf("expected period in heading")
	}
	if !strings.Contains(text, "pushed to") {
		t.Errorf("expected event type count")
	}
	if !strings.Contains(text, "Quick Summary") {
		t.Errorf("expected quick summary table")
	}
	if !strings.Contains(text, "Feature A") {
		t.Errorf("expected MR title in output")
	}
	if !strings.Contains(text, "Task X") {
		t.Errorf("expected issue title in output")
	}
}

// TestTeamMemberWorkloadUser_NotFound verifies that the team_member_workload
// prompt returns an error when the users API returns an empty list for the
// requested username.
func TestTeamMemberWorkloadUser_NotFound(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathUsers:
			respondJSON(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "team_member_workload",
		Arguments: map[string]string{"project_id": "42", "username": "nonexistent"},
	})
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

// TestTeamMemberWorkload_MissingUsername verifies that the
// team_member_workload prompt returns an error when the required username
// argument is missing.
func TestTeamMemberWorkload_MissingUsername(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "team_member_workload",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err == nil {
		t.Fatal("expected error for missing username")
	}
}

// TestTeamMemberWorkload_InvalidDays verifies that the team_member_workload
// prompt returns an error when the days argument is not a valid integer.
func TestTeamMemberWorkload_InvalidDays(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "team_member_workload",
		Arguments: map[string]string{"project_id": "42", "username": "alice", "days": "abc"},
	})
	if err == nil {
		t.Fatal("expected error for invalid days parameter")
	}
}

// TestTeamMemberWorkload_EmptyActivity verifies that the team_member_workload
// prompt returns a "No contribution events found" message and the workload
// heading when the user has no recent activity.
func TestTeamMemberWorkload_EmptyActivity(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathUsers:
			respondJSON(w, http.StatusOK, `[{"id":50,"username":"dave"}]`)
		case "/api/v4/users/50/events":
			respondJSON(w, http.StatusOK, `[]`)
		case pathMRs:
			respondJSON(w, http.StatusOK, `[]`)
		case pathIssues:
			respondJSON(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "team_member_workload",
		Arguments: map[string]string{"project_id": "42", "username": "dave"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No contribution events found") {
		t.Errorf("expected no events message")
	}
	if !strings.Contains(text, "Workload Summary for @dave") {
		t.Errorf("expected workload heading for dave")
	}
}

// User Stats prompt tests.

// TestUserStats_Success verifies that the user_stats prompt returns a
// complete statistics report including activity summary, event type counts,
// MR stats, issue stats, daily activity Mermaid chart, and an overall
// summary when all APIs respond successfully.
func TestUserStats_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathUsers:
			respondJSON(w, http.StatusOK, `[{"id":77,"username":"eve"}]`)
		case "/api/v4/users/77/events":
			respondJSON(w, http.StatusOK, `[
				{"id":1,"action_name":"pushed to","target_type":"Project","target_title":"proj","created_at":"2026-02-28T10:00:00Z"},
				{"id":2,"action_name":"commented on","target_type":"MergeRequest","target_title":"mr1","created_at":"2026-02-28T14:00:00Z"},
				{"id":3,"action_name":"pushed to","target_type":"Project","target_title":"proj","created_at":"2026-02-27T09:00:00Z"}
			]`)
		case pathMRs:
			respondJSON(w, http.StatusOK, `[{"id":10,"iid":1,"title":"Feature X","state":"opened","source_branch":"feat-x","target_branch":"main","detailed_merge_status":"mergeable"}]`)
		case pathIssues:
			respondJSON(w, http.StatusOK, `[{"id":20,"iid":5,"title":"Bug Y","state":"opened","created_at":"2026-01-15T00:00:00Z","author":{"username":"eve"}}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "user_stats",
		Arguments: map[string]string{"project_id": "42", "username": "eve", "days": "30"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text

	checks := []struct {
		name, substr string
	}{
		{"heading", "User Statistics for @eve"},
		{"period", "last 30 days"},
		{"activity section", "Activity Summary"},
		{"event type", "pushed to"},
		{"event count table", "| pushed to |"},
		{"MR stats section", "Merge Request Stats"},
		{"issue stats section", "Issue Stats"},
		{"daily activity", "Daily Activity"},
		{"mermaid chart", "xychart-beta"},
		{"mermaid title", "Daily Activity for @eve"},
		{"overall summary", "Overall Summary"},
	}
	for _, c := range checks {
		if !strings.Contains(text, c.substr) {
			t.Errorf("[%s] expected output to contain %q", c.name, c.substr)
		}
	}
}

// TestUserStats_DefaultsToCurrentUser verifies that the user_stats prompt
// falls back to the authenticated user via the /user API when no username
// argument is provided, and uses a default period of 30 days.
func TestUserStats_DefaultsToCurrentUser(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/user":
			respondJSON(w, http.StatusOK, `{"id":1,"username":"currentuser"}`)
		case "/api/v4/events":
			respondJSON(w, http.StatusOK, `[]`)
		case pathMRs:
			respondJSON(w, http.StatusOK, `[]`)
		case pathIssues:
			respondJSON(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "user_stats",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "User Statistics for @currentuser") {
		t.Errorf("expected heading for current user")
	}
	if !strings.Contains(text, "last 30 days") {
		t.Errorf("expected default 30 days period")
	}
}

// TestUserStats_MissingProjectID verifies that the user_stats prompt returns
// an error when the required project_id argument is missing.
func TestUserStats_MissingProjectID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "user_stats",
		Arguments: map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestUserStats_InvalidDays verifies that the user_stats prompt returns an
// error when the days argument is a negative number.
func TestUserStats_InvalidDays(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "user_stats",
		Arguments: map[string]string{"project_id": "42", "days": "-5"},
	})
	if err == nil {
		t.Fatal("expected error for invalid days parameter")
	}
}

// TestUserStatsUser_NotFound verifies that the user_stats prompt returns an
// error when the users API returns an empty list for the requested username.
func TestUserStatsUser_NotFound(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathUsers:
			respondJSON(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "user_stats",
		Arguments: map[string]string{"project_id": "42", "username": "ghost"},
	})
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

// TestUserStats_EmptyActivity verifies that the user_stats prompt returns a
// "No contribution events found" message and omits the Mermaid chart when
// the user has no recent events, while still including the overall summary.
func TestUserStats_EmptyActivity(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathUsers:
			respondJSON(w, http.StatusOK, `[{"id":88,"username":"quiet"}]`)
		case "/api/v4/users/88/events":
			respondJSON(w, http.StatusOK, `[]`)
		case pathMRs:
			respondJSON(w, http.StatusOK, `[]`)
		case pathIssues:
			respondJSON(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "user_stats",
		Arguments: map[string]string{"project_id": "42", "username": "quiet"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "No contribution events found") {
		t.Errorf("expected no events message")
	}
	if strings.Contains(text, "xychart-beta") {
		t.Errorf("expected no Mermaid chart when there are no events")
	}
	if !strings.Contains(text, "Overall Summary") {
		t.Errorf("expected overall summary even with no activity")
	}
}

// TestUserStatsMermaidChart_Format verifies that the user_stats prompt
// generates a correctly structured Mermaid xychart-beta code block with
// dates in chronological order and accurate event counts per day.
func TestUserStatsMermaidChart_Format(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathUsers:
			respondJSON(w, http.StatusOK, `[{"id":33,"username":"frank"}]`)
		case "/api/v4/users/33/events":
			respondJSON(w, http.StatusOK, `[
				{"id":1,"action_name":"pushed to","target_type":"Project","target_title":"p","created_at":"2026-03-01T10:00:00Z"},
				{"id":2,"action_name":"pushed to","target_type":"Project","target_title":"p","created_at":"2026-03-01T12:00:00Z"},
				{"id":3,"action_name":"commented on","target_type":"Issue","target_title":"i","created_at":"2026-02-28T08:00:00Z"}
			]`)
		case pathMRs:
			respondJSON(w, http.StatusOK, `[]`)
		case pathIssues:
			respondJSON(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "user_stats",
		Arguments: map[string]string{"project_id": "42", "username": "frank", "days": "7"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text

	// Verify Mermaid chart structure
	if !strings.Contains(text, "```mermaid") {
		t.Fatal("expected mermaid code block")
	}
	if !strings.Contains(text, "xychart-beta") {
		t.Errorf("expected xychart-beta chart type")
	}
	// Events on 2 days: 2026-02-28 (1 event) and 2026-03-01 (2 events)
	if !strings.Contains(text, "2026-02-28") {
		t.Errorf("expected date 2026-02-28 in chart")
	}
	if !strings.Contains(text, "2026-03-01") {
		t.Errorf("expected date 2026-03-01 in chart")
	}
	// Chronological order: 02-28 before 03-01
	idx28 := strings.Index(text, "2026-02-28")
	idx01 := strings.Index(text, "2026-03-01")
	if idx28 >= idx01 {
		t.Errorf("expected dates in chronological order (02-28 before 03-01)")
	}
}

// TestAllPromptArguments_HaveTitle verifies that every PromptArgument across
// all registered prompts has a non-empty Title for human-readable UI display.
func TestAllPromptArguments_HaveTitle(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	}))
	result, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	for _, p := range result.Prompts {
		for _, arg := range p.Arguments {
			if arg.Title == "" {
				t.Errorf("prompt %q argument %q has empty Title", p.Name, arg.Name)
			}
		}
	}
}

// Prompt helper tests.

// TestParseIID uses table-driven subtests to verify that parseIID correctly
// converts string IID values to int64, returning 0 for invalid or empty
// inputs.
func TestParseIID(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"5", 5},
		{"100", 100},
		{"0", 0},
		{"abc", 0},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := parseIID(tt.input); got != tt.want {
				t.Errorf("parseIID(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestShortSHA uses table-driven subtests to verify that shortSHA truncates
// commit SHA strings to 8 characters, returning shorter strings unchanged.
func TestShortSHA(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"abc12345def67890", "abc12345"},
		{"short", "short"},
		{"12345678", "12345678"},
		{"123456789", "12345678"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := shortSHA(tt.input); got != tt.want {
				t.Errorf("shortSHA(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestPromptResult verifies that promptResult builds a single-message MCP
// prompt result with the "assistant" role and the expected text content.
func TestPromptResult(t *testing.T) {
	result := promptResult(testHelloWorld)
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	if result.Messages[0].Role != "assistant" {
		t.Errorf("role = %q, want %q", result.Messages[0].Role, "assistant")
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if text != testHelloWorld {
		t.Errorf("text = %q, want %q", text, testHelloWorld)
	}
}
