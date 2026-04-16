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
