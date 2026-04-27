// resources_test.go contains integration tests verifying the happy-path
// behavior of each MCP resource registered by [Register]. Tests use httptest
// to mock GitLab API responses and an in-memory MCP transport to exercise
// the full resource read pipeline.

package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Shared format strings and URI prefix constants used across resource tests.
const (
	fmtUnexpectedErr     = "unexpected error: %v"
	fmtUnmarshal         = "unmarshal: %v"
	fmtNameWant          = "name = %q, want %q"
	fmtUsernameWant      = "username = %q, want %q"
	fmtTitleWant         = "title = %q, want %q"
	fmtAuthorWant        = "author = %q, want %q"
	testURIProjectPrefix = "gitlab://project/"
	msgExpectedAPIErr    = "expected error for API failure"
	testProjectName      = "my-project"
	testTagV100          = "v1.0.0"
)

// TestCurrentUserResource_Success verifies that the current_user resource
// returns the authenticated user's profile when the GitLab API responds
// with a valid user JSON payload.
func TestCurrentUserResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/user" {
			respondJSON(w, http.StatusOK, `{"id":1,"username":"testuser","name":"Test User","email":"test@example.com","state":"active","web_url":"https://gitlab.example.com/testuser","is_admin":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://user/current"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}

	var user UserResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &user); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if user.Username != "testuser" {
		t.Errorf(fmtUsernameWant, user.Username, "testuser")
	}
	if user.ID != 1 {
		t.Errorf("id = %d, want 1", user.ID)
	}
}

// TestGroupsResource_Success verifies that the groups resource returns a list
// of accessible groups when the GitLab API responds successfully.
func TestGroupsResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups" {
			respondJSON(w, http.StatusOK, `[{"id":10,"name":"DevOps","path":"devops","full_path":"devops","description":"DevOps team","visibility":"private","web_url":"https://gitlab.example.com/devops"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://groups"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var groups []GroupResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &groups); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "DevOps" {
		t.Errorf(fmtNameWant, groups[0].Name, "DevOps")
	}
}

// TestProjectResource_Success verifies that the project resource returns
// correct metadata when the GitLab API responds with a valid project payload.
func TestProjectResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42" {
			respondJSON(w, http.StatusOK, `{"id":42,"name":"my-project","path_with_namespace":"user/my-project","visibility":"private","web_url":"https://gitlab.example.com/user/my-project","description":"Test project","default_branch":"main"}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var project ProjectResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &project); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if project.ID != 42 {
		t.Errorf("id = %d, want 42", project.ID)
	}
	if project.Name != testProjectName {
		t.Errorf(fmtNameWant, project.Name, testProjectName)
	}
}

// TestProjectMembersResource_Success verifies that the project_members resource
// returns a list of members with their access levels when the API responds
// successfully.
func TestProjectMembersResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/members/all" {
			respondJSON(w, http.StatusOK, `[{"id":1,"username":"alice","name":"Alice","state":"active","access_level":40,"web_url":"https://gitlab.example.com/alice"},{"id":2,"username":"bob","name":"Bob","state":"active","access_level":30,"web_url":"https://gitlab.example.com/bob"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/members"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var members []MemberResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &members); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	if members[0].Username != "alice" {
		t.Errorf(fmtUsernameWant, members[0].Username, "alice")
	}
}

// TestLatestPipelineResource_Success verifies that the latest_pipeline resource
// returns the most recent pipeline when the GitLab API responds successfully.
func TestLatestPipelineResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/pipelines/latest" {
			respondJSON(w, http.StatusOK, `{"id":100,"iid":10,"status":"success","ref":"main","sha":"abc12345","web_url":"https://gitlab.example.com/pipelines/100","source":"push"}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/pipelines/latest"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var pipeline PipelineResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &pipeline); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if pipeline.Status != "success" {
		t.Errorf("status = %q, want %q", pipeline.Status, "success")
	}
}

// TestPipelineResource_Success verifies that the pipeline resource returns
// correct details when given a valid project and pipeline ID.
func TestPipelineResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/pipelines/100" {
			respondJSON(w, http.StatusOK, `{"id":100,"iid":10,"status":"failed","ref":"develop","sha":"def45678","web_url":"https://gitlab.example.com/pipelines/100","source":"merge_request_event"}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/pipeline/100"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var pipeline PipelineResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &pipeline); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if pipeline.ID != 100 {
		t.Errorf("id = %d, want 100", pipeline.ID)
	}
}

// TestPipelineJobsResource_Success verifies that the pipeline_jobs resource
// returns a list of jobs with statuses and failure reasons when the GitLab
// API responds successfully.
func TestPipelineJobsResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/pipelines/100/jobs" {
			respondJSON(w, http.StatusOK, `[{"id":201,"name":"test","stage":"test","status":"success","ref":"main","duration":45.2,"web_url":"https://gitlab.example.com/jobs/201"},{"id":202,"name":"build","stage":"build","status":"failed","ref":"main","duration":12.1,"failure_reason":"script_failure","web_url":"https://gitlab.example.com/jobs/202"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/pipeline/100/jobs"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var jobs []JobResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &jobs); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if jobs[1].FailureReason != "script_failure" {
		t.Errorf("failure_reason = %q, want %q", jobs[1].FailureReason, "script_failure")
	}
}

// TestProjectLabelsResource_Success verifies that the project_labels resource
// returns labels with their open issue and MR counts when the API responds
// successfully.
func TestProjectLabelsResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/labels" {
			respondJSON(w, http.StatusOK, `[{"id":1,"name":"bug","color":"#d9534f","description":"Bug reports","open_issues_count":3,"open_merge_requests_count":1}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/labels"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var labels []LabelResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &labels); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(labels))
	}
	if labels[0].Name != "bug" {
		t.Errorf(fmtNameWant, labels[0].Name, "bug")
	}
}

// TestProjectMilestonesResource_Success verifies that the project_milestones
// resource returns milestones with their state and title when the API
// responds successfully.
func TestProjectMilestonesResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/milestones" {
			respondJSON(w, http.StatusOK, `[{"id":5,"iid":1,"title":"v1.0","description":"First release","state":"active","web_url":"https://gitlab.example.com/milestones/1"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/milestones"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var milestones []MilestoneResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &milestones); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if milestones[0].Title != "v1.0" {
		t.Errorf(fmtTitleWant, milestones[0].Title, "v1.0")
	}
}

// TestMergeRequestResource_Success verifies that the merge_request resource
// returns correct MR details including author and merge status when the
// GitLab API responds successfully.
func TestMergeRequestResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/5" {
			respondJSON(w, http.StatusOK, `{"id":55,"iid":5,"title":"Add feature","state":"opened","source_branch":"feature","target_branch":"main","author":{"username":"alice"},"web_url":"https://gitlab.example.com/mr/5","detailed_merge_status":"mergeable"}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/mr/5"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var mr MRResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &mr); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if mr.Title != "Add feature" {
		t.Errorf(fmtTitleWant, mr.Title, "Add feature")
	}
	if mr.Author != "alice" {
		t.Errorf(fmtAuthorWant, mr.Author, "alice")
	}
}

// TestProjectBranchesResource_Success verifies that the project_branches
// resource returns branches with their protection and default status when
// the GitLab API responds successfully.
func TestProjectBranchesResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/branches" {
			respondJSON(w, http.StatusOK, `[{"name":"main","protected":true,"merged":false,"default":true,"web_url":"https://gitlab.example.com/branches/main"},{"name":"develop","protected":false,"merged":false,"default":false,"web_url":"https://gitlab.example.com/branches/develop"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/branches"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var branches []BranchResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &branches); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(branches))
	}
	if branches[0].Name != "main" {
		t.Errorf(fmtNameWant, branches[0].Name, "main")
	}
}

// Group resource tests.

// TestGroupResource_Success verifies that the group resource returns correct
// details for a specific group by its numeric ID.
func TestGroupResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10" {
			respondJSON(w, http.StatusOK, `{"id":10,"name":"DevOps","path":"devops","full_path":"org/devops","description":"DevOps team","visibility":"private","web_url":"https://gitlab.example.com/org/devops"}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group/10"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var group GroupResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &group); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if group.ID != 10 {
		t.Errorf("id = %d, want 10", group.ID)
	}
	if group.Name != "DevOps" {
		t.Errorf(fmtNameWant, group.Name, "DevOps")
	}
	if group.FullPath != "org/devops" {
		t.Errorf("full_path = %q, want %q", group.FullPath, "org/devops")
	}
}

// TestGroupResource_InvalidURI verifies that the group resource returns an
// error when the URI contains an empty group ID.
func TestGroupResource_InvalidURI(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group/"})
	if err == nil {
		t.Fatal("expected error for empty group ID")
	}
}

// TestGroupMembersResource_Success verifies that the group_members resource
// returns members with correct access levels when the API responds successfully.
func TestGroupMembersResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/members/all" {
			respondJSON(w, http.StatusOK, `[{"id":1,"username":"alice","name":"Alice","state":"active","access_level":50,"web_url":"https://gitlab.example.com/alice"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group/10/members"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var members []MemberResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &members); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0].Username != "alice" {
		t.Errorf(fmtUsernameWant, members[0].Username, "alice")
	}
	if members[0].AccessLevel != 50 {
		t.Errorf("access_level = %d, want 50", members[0].AccessLevel)
	}
}

// TestGroupMembersResource_APIError verifies that the group_members resource
// returns an error when the GitLab API responds with a server error.
func TestGroupMembersResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/members/all" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group/10/members"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestGroupProjectsResource_Success verifies that the group_projects resource
// returns a list of projects within the group when the API responds successfully.
func TestGroupProjectsResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/projects" {
			respondJSON(w, http.StatusOK, `[{"id":42,"name":"my-project","path_with_namespace":"org/my-project","visibility":"private","web_url":"https://gitlab.example.com/org/my-project","description":"A project","default_branch":"main"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group/10/projects"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var projects []ProjectResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &projects); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != testProjectName {
		t.Errorf(fmtNameWant, projects[0].Name, testProjectName)
	}
}

// TestGroupProjectsResource_APIError verifies that the group_projects resource
// returns an error when the GitLab API responds with a server error.
func TestGroupProjectsResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/projects" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group/10/projects"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// Issue resource tests.

// TestProjectIssuesResource_Success verifies that the project_issues resource
// returns open issues with labels, assignees, and author when the API
// responds successfully.
func TestProjectIssuesResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/issues" {
			respondJSON(w, http.StatusOK, `[{"id":100,"iid":1,"title":"Fix bug","state":"opened","labels":["bug"],"assignees":[{"username":"alice"}],"author":{"username":"bob"},"web_url":"https://gitlab.example.com/issues/1","created_at":"2026-01-15T10:00:00Z"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/issues"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var issues []IssueResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &issues); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Title != "Fix bug" {
		t.Errorf(fmtTitleWant, issues[0].Title, "Fix bug")
	}
	if issues[0].Author != "bob" {
		t.Errorf(fmtAuthorWant, issues[0].Author, "bob")
	}
	if len(issues[0].Assignees) != 1 || issues[0].Assignees[0] != "alice" {
		t.Errorf("assignees = %v, want [alice]", issues[0].Assignees)
	}
}

// TestProjectIssuesResource_APIError verifies that the project_issues resource
// returns an error when the GitLab API responds with a server error.
func TestProjectIssuesResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/issues" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/issues"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestIssueResource_Success verifies that the issue resource returns correct
// details for a specific issue by its project-scoped IID.
func TestIssueResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/issues/5" {
			respondJSON(w, http.StatusOK, `{"id":200,"iid":5,"title":"Add feature X","state":"opened","labels":["enhancement"],"assignees":[],"author":{"username":"charlie"},"web_url":"https://gitlab.example.com/issues/5","created_at":"2026-02-01T12:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/issue/5"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var issue IssueResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &issue); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if issue.IID != 5 {
		t.Errorf("iid = %d, want 5", issue.IID)
	}
	if issue.Title != "Add feature X" {
		t.Errorf(fmtTitleWant, issue.Title, "Add feature X")
	}
	if issue.Author != "charlie" {
		t.Errorf(fmtAuthorWant, issue.Author, "charlie")
	}
}

// TestIssueResource_InvalidURI verifies that the issue resource returns an
// error when the URI contains an empty issue IID.
func TestIssueResource_InvalidURI(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/issue/"})
	if err == nil {
		t.Fatal("expected error for empty issue IID")
	}
}

// TestIssueResource_NonNumericIID verifies that the issue resource returns an
// error when the issue IID is not a valid number.
func TestIssueResource_NonNumericIID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/issue/abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric issue IID")
	}
}

// Release resource tests.

// TestProjectReleasesResource_Success verifies that the project_releases
// resource returns releases with author and timestamps when the API
// responds successfully.
func TestProjectReleasesResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/releases" {
			respondJSON(w, http.StatusOK, `[{"tag_name":"v1.0.0","name":"Release 1.0","description":"First release","author":{"username":"alice"},"created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-02T00:00:00Z"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/releases"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var releases []ReleaseResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &releases); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}
	if releases[0].TagName != testTagV100 {
		t.Errorf("tag_name = %q, want %q", releases[0].TagName, testTagV100)
	}
	if releases[0].Author != "alice" {
		t.Errorf(fmtAuthorWant, releases[0].Author, "alice")
	}
}

// TestProjectReleasesResource_APIError verifies that the project_releases
// resource returns an error when the GitLab API responds with a server error.
func TestProjectReleasesResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/releases" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/releases"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// Tag resource tests.

// TestProjectTagsResource_Success verifies that the project_tags resource
// returns tags with their protection status and target SHA when the API
// responds successfully.
func TestProjectTagsResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/tags" {
			respondJSON(w, http.StatusOK, `[{"name":"v1.0.0","message":"Release tag","target":"abc123","protected":true,"created_at":"2026-01-01T00:00:00Z"},{"name":"v0.9.0","message":"","target":"def456","protected":false,"created_at":"2023-12-01T00:00:00Z"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/tags"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var tags []TagResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &tags); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
	if tags[0].Name != testTagV100 {
		t.Errorf(fmtNameWant, tags[0].Name, testTagV100)
	}
	if !tags[0].Protected {
		t.Error("expected first tag to be protected")
	}
	if tags[1].Protected {
		t.Error("expected second tag to not be protected")
	}
}

// TestProjectTagsResource_APIError verifies that the project_tags resource
// returns an error when the GitLab API responds with a server error.
func TestProjectTagsResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/tags" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/tags"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// Commit resource tests.

// TestCommitResource_Success verifies that the commit resource returns
// commit metadata, parent SHAs, and stats when the API responds successfully.
func TestCommitResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/commits/abc123" {
			respondJSON(w, http.StatusOK, `{"id":"abc123def456","short_id":"abc123","title":"Fix bug","message":"Fix bug\n\nDetails","author_name":"alice","author_email":"alice@example.com","authored_date":"2026-01-01T10:00:00Z","committed_date":"2026-01-01T10:05:00Z","web_url":"https://gitlab.example.com/group/project/-/commit/abc123","parent_ids":["parent1","parent2"],"stats":{"additions":10,"deletions":3,"total":13}}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/commit/abc123"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var c CommitResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &c); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if c.ShortID != "abc123" {
		t.Errorf("ShortID = %q, want abc123", c.ShortID)
	}
	if c.AuthorName != "alice" {
		t.Errorf(fmtAuthorWant, c.AuthorName, "alice")
	}
	if c.Stats == nil || c.Stats.Total != 13 {
		t.Errorf("expected stats.total=13, got %+v", c.Stats)
	}
	if len(c.ParentIDs) != 2 {
		t.Errorf("expected 2 parents, got %d", len(c.ParentIDs))
	}
}

// TestCommitResource_NotFound verifies that an unknown commit returns a
// resource-not-found error.
func TestCommitResource_NotFound(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/commit/missing"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// File blob resource tests.

// TestFileBlobResource_Success verifies that base64-encoded file content is
// decoded and returned as text along with file metadata.
func TestFileBlobResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/repository/files/") {
			respondJSON(w, http.StatusOK, `{"file_name":"main.go","file_path":"src/main.go","size":11,"encoding":"base64","ref":"main","blob_id":"blob1","commit_id":"c1","last_commit_id":"c1","content":"aGVsbG8gd29ybGQ="}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/file/main/src/main.go"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var f FileBlobResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &f); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if f.FilePath != "src/main.go" {
		t.Errorf("FilePath = %q, want src/main.go", f.FilePath)
	}
	if f.Content != "hello world" {
		t.Errorf("Content = %q, want %q", f.Content, "hello world")
	}
	if f.ContentCategory != "text" {
		t.Errorf("ContentCategory = %q, want text", f.ContentCategory)
	}
	if f.Truncated {
		t.Error("expected Truncated=false")
	}
}

// TestFileBlobResource_Truncated verifies that files larger than the size
// limit return metadata with truncated=true and content omitted.
func TestFileBlobResource_Truncated(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/files/big.bin" {
			respondJSON(w, http.StatusOK, `{"file_name":"big.bin","file_path":"big.bin","size":2097152,"encoding":"base64","ref":"main","blob_id":"b","commit_id":"c","last_commit_id":"c","content":"AAAA"}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/file/main/big.bin"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var f FileBlobResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &f); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if !f.Truncated {
		t.Error("expected Truncated=true")
	}
	if f.Content != "" {
		t.Errorf("expected empty content, got %q", f.Content)
	}
}

// TestFileBlobResource_BadURI verifies that a malformed file URI (missing
// path component) returns a resource-not-found error.
func TestFileBlobResource_BadURI(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/file/main/"})
	if err == nil {
		t.Fatal("expected error for malformed file URI")
	}
}

// TestExtractFileBlobURI verifies that [extractFileBlobURI] correctly splits
// a file blob URI into project_id, ref, and path components, including paths
// with multiple slashes.
func TestExtractFileBlobURI(t *testing.T) {
	tests := []struct {
		uri, projectID, ref, path string
	}{
		{"gitlab://project/42/file/main/src/main.go", "42", "main", "src/main.go"},
		{"gitlab://project/group%2Frepo/file/v1.0/README.md", "group%2Frepo", "v1.0", "README.md"},
		{"gitlab://project/42/file/main/", "", "", ""},
		{"gitlab://project/42/file/main", "", "", ""},
		{"gitlab://project/42/commit/abc", "", "", ""},
		{"", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			pid, ref, path := extractFileBlobURI(tt.uri)
			if pid != tt.projectID || ref != tt.ref || path != tt.path {
				t.Errorf("extractFileBlobURI(%q) = (%q,%q,%q), want (%q,%q,%q)",
					tt.uri, pid, ref, path, tt.projectID, tt.ref, tt.path)
			}
		})
	}
}

// Wiki resource tests.

// TestWikiResource_Success verifies that the wiki page resource returns
// title, slug, format, and content for an existing page.
func TestWikiResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/wikis/Home" {
			respondJSON(w, http.StatusOK, `{"title":"Home","slug":"Home","format":"markdown","content":"# Welcome"}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/wiki/Home"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var p WikiResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &p); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if p.Title != "Home" {
		t.Errorf(fmtTitleWant, p.Title, "Home")
	}
	if p.Format != "markdown" {
		t.Errorf("Format = %q, want markdown", p.Format)
	}
	if p.Content != "# Welcome" {
		t.Errorf("Content = %q, want '# Welcome'", p.Content)
	}
}

// TestWikiResource_NotFound verifies that an unknown wiki page returns a
// resource-not-found error.
func TestWikiResource_NotFound(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/wiki/missing"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// MR notes resource tests.

// TestMergeRequestNotesResource_Success verifies that the MR notes resource
// returns each note's id, author, body, and resolution flags.
func TestMergeRequestNotesResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/7/notes" {
			respondJSON(w, http.StatusOK, `[{"id":1,"body":"LGTM","system":false,"resolvable":true,"resolved":false,"author":{"username":"alice"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},{"id":2,"body":"merged","system":true,"resolvable":false,"resolved":false,"author":{"username":"bot"},"created_at":"2026-01-02T00:00:00Z"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/mr/7/notes"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var notes []MRNoteResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &notes); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(notes))
	}
	if notes[0].Author != "alice" {
		t.Errorf(fmtAuthorWant, notes[0].Author, "alice")
	}
	if !notes[1].System {
		t.Error("expected second note to be system")
	}
}

// TestMergeRequestNotesResource_BadIID verifies that a non-numeric MR IID
// returns a resource-not-found error.
func TestMergeRequestNotesResource_BadIID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/mr/notanumber/notes"})
	if err == nil {
		t.Fatal("expected error for non-numeric MR iid")
	}
}

// MR discussions resource tests.

// TestMergeRequestDiscussionsResource_Success verifies that the MR
// discussions resource returns thread metadata and nested notes.
func TestMergeRequestDiscussionsResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/7/discussions" {
			respondJSON(w, http.StatusOK, `[{"id":"d1","individual_note":false,"notes":[{"id":11,"body":"please fix","system":false,"resolved":false,"resolvable":true,"author":{"username":"alice"},"created_at":"2026-01-01T00:00:00Z"},{"id":12,"body":"done","system":false,"resolved":true,"resolvable":true,"author":{"username":"bob"},"created_at":"2026-01-01T01:00:00Z"}]},{"id":"d2","individual_note":true,"notes":[{"id":21,"body":"comment","system":false,"resolved":false,"resolvable":false,"author":{"username":"carol"},"created_at":"2026-01-02T00:00:00Z"}]}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/mr/7/discussions"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var ds []MRDiscussionResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &ds); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if len(ds) != 2 {
		t.Fatalf("expected 2 discussions, got %d", len(ds))
	}
	if ds[0].ID != "d1" || ds[0].IndividualNote {
		t.Errorf("first discussion = %+v, want id=d1 individual_note=false", ds[0])
	}
	if len(ds[0].Notes) != 2 {
		t.Errorf("expected 2 notes in first discussion, got %d", len(ds[0].Notes))
	}
	if !ds[0].Notes[1].Resolved {
		t.Error("expected second note in first discussion to be resolved")
	}
	if !ds[1].IndividualNote {
		t.Error("expected second discussion to be individual_note")
	}
}

// TestMergeRequestDiscussionsResource_APIError verifies that a server error
// from the discussions endpoint propagates as an error.
func TestMergeRequestDiscussionsResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/mr/7/discussions"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// URI helper tests.

// TestExtractSuffix uses table-driven subtests to verify that [extractSuffix]
// correctly returns the portion of a URI after a given prefix.
func TestExtractSuffix(t *testing.T) {
	tests := []struct {
		uri, prefix, want string
	}{
		{"gitlab://project/42", testURIProjectPrefix, "42"},
		{"gitlab://user/current", "gitlab://user/", "current"},
		{"other://something", "gitlab://", ""},
		{"", "gitlab://", ""},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.uri, tt.prefix), func(t *testing.T) {
			got := extractSuffix(tt.uri, tt.prefix)
			if got != tt.want {
				t.Errorf("extractSuffix(%q, %q) = %q, want %q", tt.uri, tt.prefix, got, tt.want)
			}
		})
	}
}

// TestExtractMiddle uses table-driven subtests to verify that [extractMiddle]
// correctly returns the portion of a URI between a prefix and suffix.
func TestExtractMiddle(t *testing.T) {
	tests := []struct {
		uri, prefix, suffix, want string
	}{
		{"gitlab://project/42/branches", testURIProjectPrefix, "/branches", "42"},
		{"gitlab://project/42/labels", testURIProjectPrefix, "/labels", "42"},
		{"wrong", testURIProjectPrefix, "/labels", ""},
	}
	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got := extractMiddle(tt.uri, tt.prefix, tt.suffix)
			if got != tt.want {
				t.Errorf("extractMiddle(%q, %q, %q) = %q, want %q", tt.uri, tt.prefix, tt.suffix, got, tt.want)
			}
		})
	}
}

// TestExtractTwoParts uses table-driven subtests to verify that
// [extractTwoParts] correctly splits a URI into two dynamic segments
// around a separator.
func TestExtractTwoParts(t *testing.T) {
	tests := []struct {
		uri, prefix, sep, wantA, wantB string
	}{
		{"gitlab://project/42/pipeline/100", testURIProjectPrefix, "/pipeline/", "42", "100"},
		{"gitlab://project/42/mr/5", testURIProjectPrefix, "/mr/", "42", "5"},
		{"invalid", testURIProjectPrefix, "/pipeline/", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			a, b := extractTwoParts(tt.uri, tt.prefix, tt.sep)
			if a != tt.wantA || b != tt.wantB {
				t.Errorf("extractTwoParts(%q, %q, %q) = (%q, %q), want (%q, %q)", tt.uri, tt.prefix, tt.sep, a, b, tt.wantA, tt.wantB)
			}
		})
	}
}

// TestReleaseResource_Success verifies that the singleton release resource
// returns release metadata when the GitLab API responds with a valid release
// payload at gitlab://project/{id}/release/{tag}.
func TestReleaseResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/releases/v1.0.0" {
			respondJSON(w, http.StatusOK, `{"tag_name":"v1.0.0","name":"Release 1.0","description":"First release","author":{"username":"alice"},"created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-02T00:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/release/v1.0.0"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}

	var rel ReleaseResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &rel); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if rel.TagName != testTagV100 {
		t.Errorf("tag_name = %q, want %q", rel.TagName, testTagV100)
	}
	if rel.Author != "alice" {
		t.Errorf(fmtAuthorWant, rel.Author, "alice")
	}
}

// TestBranchResource_Success verifies that the singleton branch resource
// returns branch metadata when the GitLab API responds with a valid branch
// payload at gitlab://project/{id}/branch/{name}.
func TestBranchResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/branches/main" {
			respondJSON(w, http.StatusOK, `{"name":"main","protected":true,"merged":false,"default":true,"web_url":"https://gitlab.example.com/u/p/-/tree/main"}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/branch/main"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var br BranchResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &br); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if br.Name != "main" {
		t.Errorf(fmtNameWant, br.Name, "main")
	}
	if !br.Default {
		t.Error("expected default = true")
	}
	if !br.Protected {
		t.Error("expected protected = true")
	}
}

// TestTagResource_Success verifies that the singleton tag resource returns
// tag metadata when the GitLab API responds with a valid tag payload at
// gitlab://project/{id}/tag/{name}.
func TestTagResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/tags/v1.0.0" {
			respondJSON(w, http.StatusOK, `{"name":"v1.0.0","message":"Release tag","target":"abc123","protected":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/tag/v1.0.0"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var tg TagResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &tg); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if tg.Name != testTagV100 {
		t.Errorf(fmtNameWant, tg.Name, testTagV100)
	}
	if tg.Target != "abc123" {
		t.Errorf("target = %q, want abc123", tg.Target)
	}
	if !tg.Protected {
		t.Error("expected protected = true")
	}
}

// TestLabelResource_Success verifies that the singleton label resource
// returns label metadata when the GitLab API responds with a valid label
// payload at gitlab://project/{id}/label/{id}.
func TestLabelResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/labels/5" {
			respondJSON(w, http.StatusOK, `{"id":5,"name":"bug","color":"#ff0000","description":"Defect","open_issues_count":3,"open_merge_requests_count":1}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/label/5"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var lb LabelResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &lb); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if lb.ID != 5 {
		t.Errorf("id = %d, want 5", lb.ID)
	}
	if lb.Name != "bug" {
		t.Errorf(fmtNameWant, lb.Name, "bug")
	}
	if lb.OpenIssuesCount != 3 {
		t.Errorf("open_issues_count = %d, want 3", lb.OpenIssuesCount)
	}
}

// TestMilestoneResource_Success verifies that the singleton milestone
// resource resolves an IID via list-with-iids and returns milestone metadata
// when the GitLab API responds with a valid payload at
// gitlab://project/{id}/milestone/{iid}.
func TestMilestoneResource_Success(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/milestones" && r.URL.Query().Get("iids[]") == "3" {
			respondJSON(w, http.StatusOK, `[{"id":99,"iid":3,"title":"Sprint 3","description":"Q1 goals","state":"active","web_url":"https://gitlab.example.com/u/p/-/milestones/3"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/milestone/3"})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var ms MilestoneResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &ms); err != nil {
		t.Fatalf(fmtUnmarshal, err)
	}
	if ms.IID != 3 {
		t.Errorf("iid = %d, want 3", ms.IID)
	}
	if ms.Title != "Sprint 3" {
		t.Errorf(fmtTitleWant, ms.Title, "Sprint 3")
	}
}

// TestMilestoneResource_NotFound verifies that the singleton milestone
// resource returns ResourceNotFoundError when the IID does not exist (empty
// list returned by GitLab).
func TestMilestoneResource_NotFound(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/milestones" {
			respondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/milestone/99"})
	if err == nil {
		t.Fatal("expected error for unknown milestone IID")
	}
}
