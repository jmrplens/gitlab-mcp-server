// prompt_branches_test.go contains unit tests for edge cases and additional
// branch coverage across MCP prompt handlers. It exercises nil author fields,
// long diff preservation, empty descriptions, default parameter fallbacks,
// new/deleted/renamed file handling, API sub-errors (diffs, members, jobs,
// events), and pipeline status branches.
package prompts

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	pathMR1             = "/api/v4/projects/42/merge_requests/1"
	pathMR1Diffs        = "/api/v4/projects/42/merge_requests/1/diffs"
	pathUser            = "/api/v4/user"
	msgDiffsAPIFail     = "expected error when diffs API fails"
	pathPipelinesLatest = "/api/v4/projects/42/pipelines/latest"
)

// TestSummarizeOpenMRs_NilAuthorAndDescription exercises the nil author,
// description present, and long description truncation branches.
func TestSummarizeOpenMRs_NilAuthorAndDescription(t *testing.T) {
	longDesc := strings.Repeat("A", 250) // > 200 chars → triggers truncation
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathMRs {
			respondJSON(w, http.StatusOK, `[
				{"id":1,"iid":1,"title":"MR no author","state":"opened","source_branch":"a","target_branch":"main","author":null,"created_at":"2025-01-01T00:00:00Z","detailed_merge_status":"mergeable","description":"`+longDesc+`"},
				{"id":2,"iid":2,"title":"MR short desc","state":"opened","source_branch":"b","target_branch":"main","author":{"username":"bob"},"created_at":"2025-01-01T00:00:00Z","detailed_merge_status":"mergeable","description":"short desc"}
			]`)
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
	if !strings.Contains(text, "unknown") {
		t.Error("expected 'unknown' for nil author")
	}
	if !strings.Contains(text, "short desc") {
		t.Error("expected short description in output")
	}
	if !strings.Contains(text, "...") {
		t.Error("expected truncated long description with ellipsis")
	}
}

// TestReviewMR_LongDiffNotTruncated verifies that large diffs are NOT truncated.
func TestReviewMR_LongDiffNotTruncated(t *testing.T) {
	longDiff := "+" + strings.Repeat("x", 2500) // > 2000 chars — must remain intact
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathMR1:
			respondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"Big MR","source_branch":"dev","target_branch":"main","description":"","author":{"username":"test"}}`)
		case pathMR1Diffs:
			respondJSON(w, http.StatusOK, `[{"old_path":"big.go","new_path":"big.go","diff":"`+longDiff+`","new_file":false,"renamed_file":false,"deleted_file":false}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "review_mr",
		Arguments: map[string]string{"project_id": "42", "mr_iid": "1"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if strings.Contains(text, "truncated") {
		t.Error("diffs should NOT be truncated anymore")
	}
	if !strings.Contains(text, longDiff) {
		t.Error("expected full diff content to be present")
	}
}

// TestReviewMR_EmptyDescriptionAndEmptyDiff exercises empty description and empty diff branches.
func TestReviewMREmptyDescriptionAnd_EmptyDiff(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathMR1:
			respondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"Empty MR","source_branch":"dev","target_branch":"main","description":"","author":{"username":"test"}}`)
		case pathMR1Diffs:
			respondJSON(w, http.StatusOK, `[{"old_path":"f.go","new_path":"f.go","diff":"","new_file":false,"renamed_file":false,"deleted_file":false}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "review_mr",
		Arguments: map[string]string{"project_id": "42", "mr_iid": "1"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	// Empty description → no "Description:" line
	if strings.Contains(text, "Description") {
		t.Error("expected no description line for empty description")
	}
	// Empty diff → no diff block
	if strings.Contains(text, "```diff") {
		t.Error("expected no diff block for empty diff")
	}
}

// TestSuggestMRReviewers_NilAuthor exercises the nil author branch.
func TestSuggestMRReviewers_NilAuthor(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathMR1:
			respondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"MR","author":null}`)
		case pathMR1Diffs:
			respondJSON(w, http.StatusOK, `[{"old_path":"f.go","new_path":"f.go","diff":"","new_file":false,"renamed_file":false,"deleted_file":false}]`)
		case "/api/v4/projects/42/members/all":
			respondJSON(w, http.StatusOK, `[{"id":1,"username":"bob","name":"Bob","state":"active","access_level":30}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "suggest_mr_reviewers",
		Arguments: map[string]string{"project_id": "42", "mr_iid": "1"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "bob") {
		t.Error("expected bob as reviewer candidate")
	}
}

// TestGenerateReleaseNotes_DefaultTo exercises the default "to" = HEAD branch.
func TestGenerateReleaseNotes_DefaultTo(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/compare" {
			respondJSON(w, http.StatusOK, `{"commits":[{"id":"abc12345","title":"feat: x","author_name":"A"}],"diffs":[{"new_path":"f.go","new_file":false}],"compare_same_ref":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "generate_release_notes",
		Arguments: map[string]string{"project_id": "42", "from": "v1.0"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "HEAD") {
		t.Error("expected HEAD as default 'to' ref in output")
	}
}

// TestMRRiskAssessment_NewAndDeletedFiles exercises new_file and deleted_file branches.
func TestMRRisk_AssessmentNewAndDeletedFiles(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathMR1:
			respondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"Add/Remove Files","has_conflicts":false,"author":{"username":"test"}}`)
		case pathMR1Diffs:
			respondJSON(w, http.StatusOK, `[{"old_path":"","new_path":"new_file.go","diff":"+package main","new_file":true,"renamed_file":false,"deleted_file":false},{"old_path":"old_file.go","new_path":"old_file.go","diff":"-package main","new_file":false,"renamed_file":false,"deleted_file":true}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "mr_risk_assessment",
		Arguments: map[string]string{"project_id": "42", "mr_iid": "1"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "New files**: 1") {
		t.Error("expected 1 new file")
	}
	if !strings.Contains(text, "Deleted files**: 1") {
		t.Error("expected 1 deleted file")
	}
}

// TestDailyStandup_NoEventsNoMRs exercises empty events and empty MRs branches.
func TestDailyStandup_NoEventsNoMRs(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathUser:
			respondJSON(w, http.StatusOK, `{"id":1,"username":"alice"}`)
		case "/api/v4/events":
			respondJSON(w, http.StatusOK, `[]`)
		case pathMRs:
			respondJSON(w, http.StatusOK, `[]`)
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
	if !strings.Contains(text, "No events found") {
		t.Error("expected 'No events found' for empty events")
	}
	// No open MRs → should not contain "Open MRs" section
	if strings.Contains(text, "Open MRs by") {
		t.Error("expected no open MRs section when none exist")
	}
}

// TestReviewMR_DiffsAPIError exercises the diffs API failure branch (second API call).
func TestReviewMRDiffs_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathMR1:
			respondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"MR","source_branch":"dev","target_branch":"main","author":{"username":"test"}}`)
		case pathMR1Diffs:
			respondJSON(w, http.StatusBadRequest, `{"message":"Bad Request"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "review_mr",
		Arguments: map[string]string{"project_id": "42", "mr_iid": "1"},
	})
	if err == nil {
		t.Fatal(msgDiffsAPIFail)
	}
}

// TestSuggestMRReviewers_DiffsAPIError exercises the diffs API failure branch.
func TestSuggestMRReviewersDiffs_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathMR1:
			respondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"MR","author":{"username":"test"}}`)
		case pathMR1Diffs:
			respondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "suggest_mr_reviewers",
		Arguments: map[string]string{"project_id": "42", "mr_iid": "1"},
	})
	if err == nil {
		t.Fatal(msgDiffsAPIFail)
	}
}

// TestSuggestMRReviewers_MembersAPIError exercises the members API failure branch.
func TestSuggestMRReviewersMembers_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathMR1:
			respondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"MR","author":{"username":"test"}}`)
		case pathMR1Diffs:
			respondJSON(w, http.StatusOK, `[]`)
		case "/api/v4/projects/42/members/all":
			respondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "suggest_mr_reviewers",
		Arguments: map[string]string{"project_id": "42", "mr_iid": "1"},
	})
	if err == nil {
		t.Fatal("expected error when members API fails")
	}
}

// TestMRRiskAssessment_DiffsAPIError exercises the diffs API failure in risk assessment.
func TestMRRiskAssessmentDiffs_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathMR1:
			respondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"MR","author":{"username":"test"}}`)
		case pathMR1Diffs:
			respondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "mr_risk_assessment",
		Arguments: map[string]string{"project_id": "42", "mr_iid": "1"},
	})
	if err == nil {
		t.Fatal(msgDiffsAPIFail)
	}
}

// TestCompareBranches_AllChangeTypes exercises new, deleted, and renamed file change type branches.
func TestCompareBranches_AllChangeTypes(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/compare" {
			respondJSON(w, http.StatusOK, `{
				"commits":[{"id":"aaa1111bbb2222","title":"multi changes","author_name":"Alice"}],
				"diffs":[
					{"new_path":"new.go","new_file":true,"deleted_file":false,"renamed_file":false},
					{"new_path":"deleted.go","new_file":false,"deleted_file":true,"renamed_file":false},
					{"new_path":"renamed.go","new_file":false,"deleted_file":false,"renamed_file":true}
				],
				"compare_same_ref":false
			}`)
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
	if !strings.Contains(text, "new.go (new)") {
		t.Error("expected new file type")
	}
	if !strings.Contains(text, "deleted.go (deleted)") {
		t.Error("expected deleted file type")
	}
	if !strings.Contains(text, "renamed.go (renamed)") {
		t.Error("expected renamed file type")
	}
}

// TestPipelineStatus_OtherJobStatus exercises the default case in the job status switch.
func TestPipelineStatus_OtherJobStatus(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathPipelinesLatest:
			respondJSON(w, http.StatusOK, `{"id":100,"status":"running","ref":"main","sha":"abc12345","web_url":"https://gitlab.example.com/pipelines/100"}`)
		case "/api/v4/projects/42/pipelines/100/jobs":
			respondJSON(w, http.StatusOK, `[
				{"id":1,"name":"lint","stage":"test","status":"success"},
				{"id":2,"name":"build","stage":"build","status":"running"},
				{"id":3,"name":"deploy","stage":"deploy","status":"pending"}
			]`)
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
	if !strings.Contains(text, "Other Jobs") {
		t.Error("expected 'Other Jobs' section for running/pending statuses")
	}
}

// TestProjectHealthCheck_PipelineError exercises the pipeline N/A branch.
func TestProjectHealthCheckPipeline_Error(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/42":
			respondJSON(w, http.StatusOK, `{"id":42,"name":"proj","path_with_namespace":"ns/proj"}`)
		case pathPipelinesLatest:
			respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
		case pathMRs:
			respondJSON(w, http.StatusOK, `[]`)
		case "/api/v4/projects/42/repository/branches":
			respondJSON(w, http.StatusOK, `[]`)
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
	if !strings.Contains(text, "N/A") {
		t.Error("expected 'N/A' for pipeline when API fails")
	}
}

// TestProjectHealthCheck_NilAuthorAndNilCommit exercises nil-author MR and nil-commit branch.
func TestProjectHealthCheckNilAuthorAnd_NilCommit(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/42":
			respondJSON(w, http.StatusOK, `{"id":42,"name":"proj","path_with_namespace":"ns/proj"}`)
		case pathPipelinesLatest:
			respondJSON(w, http.StatusOK, `{"id":100,"status":"success","ref":"main","sha":"abc","web_url":"https://x.com/p/100"}`)
		case pathMRs:
			respondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"title":"MR","state":"opened","author":null,"created_at":"2025-01-01T00:00:00Z"}]`)
		case "/api/v4/projects/42/repository/branches":
			respondJSON(w, http.StatusOK, `[{"name":"no-commit","protected":false,"merged":false,"default":false,"commit":null}]`)
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
	if !strings.Contains(text, "unknown") {
		t.Error("expected 'unknown' for nil author in health check MR list")
	}
}

// TestPipelineStatus_JobsAPIError exercises the jobs API failure path
// (pipeline fetch succeeds but jobs fetch fails).
func TestPipelineStatusJobs_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathPipelinesLatest:
			respondJSON(w, http.StatusOK, `{"id":100,"status":"success","ref":"main","sha":"abc","web_url":"https://x.com/p/100"}`)
		case "/api/v4/projects/42/pipelines/100/jobs":
			respondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "summarize_pipeline_status",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err == nil {
		t.Fatal("expected error when jobs API fails")
	}
}

// TestDailyStandup_UserAPIError exercises the user API error path.
func TestDailyStandupUser_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathUser:
			respondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "daily_standup",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err == nil {
		t.Fatal("expected error when user API fails")
	}
}

// TestDailyStandup_EventsAPIError exercises the events API error path
// (user succeeds, events fail).
func TestDailyStandupEvents_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathUser:
			respondJSON(w, http.StatusOK, `{"id":1,"username":"alice"}`)
		case "/api/v4/events":
			respondJSON(w, http.StatusBadRequest, `{"message":"error"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	_, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "daily_standup",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err == nil {
		t.Fatal("expected error when events API fails")
	}
}
