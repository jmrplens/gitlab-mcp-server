// graphql_context_test.go contains unit tests for the GraphQL context builders
// that aggregate multiple REST calls into single GraphQL queries.
package samplingtools

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const mrContextJSON = `{
  "project": {
    "mergeRequest": {
      "iid": "42",
      "title": "feat: add login",
      "description": "Adds login flow",
      "state": "opened",
      "sourceBranch": "feature/login",
      "targetBranch": "main",
      "mergeStatusEnum": "CAN_BE_MERGED",
      "diffStatsSummary": {"additions": 120, "deletions": 30, "fileCount": 5},
      "approvedBy": {"nodes": [{"username": "alice"}]},
      "approved": true,
      "approvalsRequired": 1,
      "headPipeline": {"status": "SUCCESS", "detailedStatus": {"text": "passed", "label": "passed"}},
      "discussions": {
        "nodes": [
          {
            "notes": {
              "nodes": [
                {"author": {"username": "bob"}, "body": "LGTM", "createdAt": "2024-06-01T10:00:00Z", "system": false, "resolvable": true, "resolved": true}
              ]
            }
          },
          {
            "notes": {
              "nodes": [
                {"author": {"username": "system"}, "body": "merged", "createdAt": "2024-06-01T11:00:00Z", "system": true, "resolvable": false, "resolved": false}
              ]
            }
          }
        ]
      }
    }
  }
}`

const issueContextJSON = `{
  "project": {
    "issue": {
      "iid": "7",
      "title": "Login bug",
      "description": "Users cannot log in",
      "state": "opened",
      "author": {"username": "alice"},
      "createdAt": "2024-01-10T08:00:00Z",
      "dueDate": "2024-02-01",
      "weight": 3,
      "labels": {"nodes": [{"title": "bug"}, {"title": "P1"}]},
      "assignees": {"nodes": [{"username": "bob"}]},
      "milestone": {"title": "v1.0", "dueDate": "2024-03-01"},
      "humanTimeEstimate": "2h",
      "humanTotalTimeSpent": "1h 30m",
      "participants": {"nodes": [{"username": "alice"}, {"username": "bob"}]},
      "notes": {
        "nodes": [
          {"author": {"username": "alice"}, "body": "I found the root cause", "createdAt": "2024-01-11T09:00:00Z", "system": false, "internal": false},
          {"author": {"username": "system"}, "body": "changed the title", "createdAt": "2024-01-11T10:00:00Z", "system": true, "internal": false}
        ]
      },
      "relatedMergeRequests": {
        "nodes": [
          {"iid": "10", "title": "fix: login flow", "state": "merged", "author": {"username": "bob"}}
        ]
      }
    }
  }
}`

const pipelineContextJSON = `{
  "project": {
    "pipeline": {
      "iid": "99",
      "status": "FAILED",
      "ref": "main",
      "sha": "abc123def",
      "duration": 120.0,
      "source": "push",
      "yamlErrors": "",
      "stages": {
        "nodes": [
          {
            "name": "build",
            "status": "SUCCESS",
            "jobs": {"nodes": [{"name": "compile", "status": "SUCCESS", "stage": {"name": "build"}, "duration": 30.0, "failureMessage": "", "webPath": "/group/proj/-/jobs/100"}]}
          },
          {
            "name": "test",
            "status": "FAILED",
            "jobs": {"nodes": [{"name": "unit-tests", "status": "FAILED", "stage": {"name": "test"}, "duration": 45.5, "failureMessage": "exit code 1", "webPath": "/group/proj/-/jobs/101"}]}
          }
        ]
      }
    }
  }
}`

// TestBuildMRContext_Success verifies that BuildMRContext fetches merge request
// data via GraphQL and produces a Markdown document with diff stats, pipeline
// status, approvals, and discussion threads.
func TestBuildMRContext_Success(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"mergeRequest": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, mrContextJSON)
		},
	}))

	result, err := BuildMRContext(context.Background(), client, "group/project", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IID != 42 {
		t.Errorf("IID = %d, want 42", result.IID)
	}
	if result.Title != "feat: add login" {
		t.Errorf("Title = %q, want %q", result.Title, "feat: add login")
	}

	checks := []string{
		"!42",
		"feat: add login",
		"feature/login",
		"CAN_BE_MERGED",
		"+120 -30",
		"passed",
		"Approved",
		"alice",
		"bob",
		"LGTM",
		"[RESOLVED]",
	}
	for _, want := range checks {
		if !strings.Contains(result.Content, want) {
			t.Errorf("content missing %q", want)
		}
	}

	// System notes should be filtered out.
	if strings.Contains(result.Content, "merged") && strings.Contains(result.Content, "system") {
		t.Error("system notes should be filtered from discussions")
	}
}

// TestBuildMRContext_NotFound verifies that BuildMRContext returns an error
// when the GraphQL API returns a null merge request (project or MR not found).
func TestBuildMRContext_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"mergeRequest": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"project":{"mergeRequest":null}}`)
		},
	}))

	_, err := BuildMRContext(context.Background(), client, "group/project", 999)
	if err == nil {
		t.Fatal("expected error for missing MR")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found' substring", err.Error())
	}
}

// TestBuildIssueContext_Success verifies that BuildIssueContext fetches issue
// data via GraphQL and produces a Markdown document with labels, milestone,
// assignees, related issues, and discussion notes.
func TestBuildIssueContext_Success(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"issue": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, issueContextJSON)
		},
	}))

	result, err := BuildIssueContext(context.Background(), client, "group/project", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IID != 7 {
		t.Errorf("IID = %d, want 7", result.IID)
	}
	if result.Title != "Login bug" {
		t.Errorf("Title = %q, want %q", result.Title, "Login bug")
	}

	checks := []string{
		"#7",
		"Login bug",
		"Users cannot log in",
		"alice",
		"bob",
		"bug",
		"P1",
		"v1.0",
		"2h",
		"1h 30m",
		"Weight",
		"Participants",
		"fix: login flow",
		"I found the root cause",
	}
	for _, want := range checks {
		if !strings.Contains(result.Content, want) {
			t.Errorf("content missing %q", want)
		}
	}

	// System notes should be filtered.
	if strings.Contains(result.Content, "changed the title") {
		t.Error("system notes should be filtered from discussion")
	}
}

// TestBuildIssueContext_NotFound verifies that BuildIssueContext returns an
// error when the GraphQL API returns a null issue (project or issue not found).
func TestBuildIssueContext_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"issue": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"project":{"issue":null}}`)
		},
	}))

	_, err := BuildIssueContext(context.Background(), client, "group/project", 999)
	if err == nil {
		t.Fatal("expected error for missing issue")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found' substring", err.Error())
	}
}

// TestBuildPipelineContext_Success verifies that BuildPipelineContext fetches
// pipeline data via GraphQL and produces a Markdown document with stage
// summaries, failed job details, and extracted failed job IDs.
func TestBuildPipelineContext_Success(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"pipeline": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, pipelineContextJSON)
		},
	}))

	result, err := BuildPipelineContext(context.Background(), client, "group/proj", 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.PipelineIID != 99 {
		t.Errorf("PipelineIID = %d, want 99", result.PipelineIID)
	}
	if result.Status != "FAILED" {
		t.Errorf("Status = %q, want FAILED", result.Status)
	}
	if result.Ref != "main" {
		t.Errorf("Ref = %q, want main", result.Ref)
	}

	checks := []string{
		"#99",
		"FAILED",
		"main",
		"abc123def",
		"120s",
		"build",
		"test",
		"unit-tests",
		"exit code 1",
	}
	for _, want := range checks {
		if !strings.Contains(result.Content, want) {
			t.Errorf("content missing %q", want)
		}
	}

	if len(result.FailedJobIDs) != 1 {
		t.Fatalf("FailedJobIDs length = %d, want 1", len(result.FailedJobIDs))
	}
	if result.FailedJobIDs[0] != 101 {
		t.Errorf("FailedJobIDs[0] = %d, want 101", result.FailedJobIDs[0])
	}
}

// TestBuildPipelineContext_NotFound verifies that BuildPipelineContext returns
// an error when the GraphQL API returns a null pipeline.
func TestBuildPipelineContext_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"pipeline": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"project":{"pipeline":null}}`)
		},
	}))

	_, err := BuildPipelineContext(context.Background(), client, "group/proj", 999)
	if err == nil {
		t.Fatal("expected error for missing pipeline")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found' substring", err.Error())
	}
}

// TestExtractJobIDFromWebPath verifies that extractJobIDFromWebPath correctly
// parses numeric job IDs from GitLab web paths like /project/-/jobs/123.
func TestExtractJobIDFromWebPath(t *testing.T) {
	tests := []struct {
		name    string
		webPath string
		want    int64
	}{
		{"valid path", "/group/project/-/jobs/123", 123},
		{"empty path", "", 0},
		{"no trailing number", "/group/project/-/jobs/", 0},
		{"no slash", "noslash", 0},
		{"non-numeric", "/group/project/-/jobs/abc", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJobIDFromWebPath(tt.webPath)
			if got != tt.want {
				t.Errorf("extractJobIDFromWebPath(%q) = %d, want %d", tt.webPath, got, tt.want)
			}
		})
	}
}

// TestExtractUsernames verifies that extractUsernames collects the Username
// field from a GraphQL user connection struct.
func TestExtractUsernames(t *testing.T) {
	nodes := gqlUsernameNodes{Nodes: []gqlUsername{{Username: "alice"}, {Username: "bob"}}}
	got := extractUsernames(nodes)
	if len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Errorf("extractUsernames = %v, want [alice bob]", got)
	}

	empty := extractUsernames(gqlUsernameNodes{})
	if empty != nil {
		t.Errorf("extractUsernames(empty) = %v, want nil", empty)
	}
}

// TestExtractLabels verifies that extractLabels collects the Title field
// from a GraphQL label connection struct.
func TestExtractLabels(t *testing.T) {
	nodes := gqlLabelNodes{Nodes: []gqlLabel{{Title: "bug"}, {Title: "P1"}}}
	got := extractLabels(nodes)
	if len(got) != 2 || got[0] != "bug" || got[1] != "P1" {
		t.Errorf("extractLabels = %v, want [bug P1]", got)
	}

	empty := extractLabels(gqlLabelNodes{})
	if empty != nil {
		t.Errorf("extractLabels(empty) = %v, want nil", empty)
	}
}
