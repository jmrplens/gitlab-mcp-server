// resource_errors_test.go contains unit tests verifying that each MCP
// resource returns an appropriate error when the GitLab API responds with
// an error status code, and when URIs contain invalid or malformed
// identifiers.
package resources

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Resource API error tests.

// TestCurrentUserResource_APIError verifies that the current_user resource
// returns an error when the GitLab API responds with 401 Unauthorized.
func TestCurrentUserResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://user/current"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestGroupsResource_APIError verifies that the groups resource returns an
// error when the GitLab API responds with an error status code.
func TestGroupsResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusBadRequest, `{"message":"Bad Request"}`)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://groups"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestProjectResource_APIError verifies that the project resource returns an
// error when the GitLab API responds with 404 Not Found.
func TestProjectResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/999"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestProjectMembersResource_APIError verifies that the project_members
// resource returns an error when the GitLab API responds with 403 Forbidden.
func TestProjectMembersResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/members"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestLatestPipelineResource_APIError verifies that the latest_pipeline
// resource returns an error when the GitLab API responds with 404 Not Found.
func TestLatestPipelineResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/pipelines/latest"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestPipelineResource_APIError verifies that the pipeline resource returns
// an error when the GitLab API responds with 404 Not Found.
func TestPipelineResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/pipeline/100"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestPipelineJobsResource_APIError verifies that the pipeline_jobs resource
// returns an error when the GitLab API responds with 404 Not Found.
func TestPipelineJobsResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/pipeline/100/jobs"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestProjectLabelsResource_APIError verifies that the project_labels resource
// returns an error when the GitLab API responds with 404 Not Found.
func TestProjectLabelsResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/labels"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestProjectMilestonesResource_APIError verifies that the project_milestones
// resource returns an error when the GitLab API responds with 404 Not Found.
func TestProjectMilestonesResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/milestones"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestMergeRequestResource_APIError verifies that the merge_request resource
// returns an error when the GitLab API responds with 404 Not Found.
func TestMergeRequestResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/mr/1"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestProjectBranchesResource_APIError verifies that the project_branches
// resource returns an error when the GitLab API responds with 404 Not Found.
func TestProjectBranchesResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/branches"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// URI edge-case tests — empty and missing identifiers.

// TestProjectResource_EmptyID verifies that the project resource returns an
// error when the URI has an empty project identifier (gitlab://project/).
func TestProjectResource_EmptyID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for empty project ID")
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/"})
	if err == nil {
		t.Fatal("expected error for empty project ID")
	}
}

// TestLatestPipelineResource_EmptyProjectID verifies that the latest_pipeline
// resource returns an error when the project ID segment is empty.
func TestLatestPipelineResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for empty project ID")
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//pipelines/latest"})
	if err == nil {
		t.Fatal("expected error for empty project ID in latest pipeline URI")
	}
}

// TestGroupResource_EmptyID verifies that the group resource returns an error
// when the URI has an empty group identifier (gitlab://group/).
func TestGroupResource_EmptyID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for empty group ID")
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group/"})
	if err == nil {
		t.Fatal("expected error for empty group ID")
	}
}

// TestPipelineResource_InvalidPipelineID verifies that the pipeline resource
// returns an error when the pipeline ID in the URI is not a valid number.
func TestPipelineResource_InvalidPipelineID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for invalid pipeline ID")
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/pipeline/abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric pipeline ID")
	}
}

// TestPipelineJobsResource_InvalidPipelineID verifies that the pipeline_jobs
// resource returns an error when the pipeline ID in the URI is non-numeric.
func TestPipelineJobsResource_InvalidPipelineID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for invalid pipeline ID")
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/pipeline/abc/jobs"})
	if err == nil {
		t.Fatal("expected error for non-numeric pipeline ID")
	}
}

// TestMergeRequestResource_InvalidMRIID verifies that the merge_request
// resource returns an error when the MR IID in the URI is non-numeric.
func TestMergeRequestResource_InvalidMRIID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for invalid MR IID")
	}))

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/mr/abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric MR IID")
	}
}

// marshalResourceJSON error test.

// TestMarshalResourceJSON_Error verifies that [marshalResourceJSON] returns an
// error when given a value that cannot be serialized to JSON (a channel).
func TestMarshalResourceJSON_Error(t *testing.T) {
	_, err := marshalResourceJSON(make(chan int))
	if err == nil {
		t.Fatal("expected error for un-marshalable value")
	}
}

// extractSuffix/extractMiddle/extractTwoParts edge cases.

// TestExtractSuffix_EmptyResult verifies that [extractSuffix] returns an empty
// string when the URI exactly equals the prefix with no trailing content.
func TestExtractSuffix_EmptyResult(t *testing.T) {
	result := extractSuffix("gitlab://user/current", "gitlab://user/current")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

// TestExtractMiddle_EmptyResult verifies that [extractMiddle] returns an empty
// string when the middle segment between prefix and suffix is empty.
func TestExtractMiddle_EmptyResult(t *testing.T) {
	result := extractMiddle("gitlab://project//pipelines/latest", "gitlab://project/", "/pipelines/latest")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

// TestExtractMiddle_NoSuffix verifies that [extractMiddle] returns an empty
// string when the URI does not contain the expected suffix.
func TestExtractMiddle_NoSuffix(t *testing.T) {
	result := extractMiddle("gitlab://project/42", "gitlab://project/", "/pipelines/latest")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

// TestExtractTwoParts_MissingSeparator verifies that [extractTwoParts]
// returns empty strings when the URI does not contain the separator.
func TestExtractTwoParts_MissingSeparator(t *testing.T) {
	a, b := extractTwoParts("gitlab://project/42", "gitlab://project/", "/pipeline/")
	if a != "" || b != "" {
		t.Errorf("expected empty strings, got %q and %q", a, b)
	}
}

// TestExtractTwoParts_EmptySecondPart verifies that [extractTwoParts] returns
// empty strings when the second segment after the separator is empty.
func TestExtractTwoParts_EmptySecondPart(t *testing.T) {
	a, b := extractTwoParts("gitlab://project/42/pipeline/", "gitlab://project/", "/pipeline/")
	if a != "" || b != "" {
		t.Errorf("expected empty strings, got %q and %q", a, b)
	}
}

// Empty URI tests for remaining template resources — each covers the
// "extracted ID is empty" guard that returns mcp.ResourceNotFoundError.

// TestProjectMembersResource_EmptyProjectID verifies that ProjectMembersResource returns a validation error when project_id is empty.
func TestProjectMembersResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not call API for empty project ID")
	}))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//members"})
	if err == nil {
		t.Fatal("expected error for empty project ID in members URI")
	}
}

// TestProjectLabelsResource_EmptyProjectID verifies that ProjectLabelsResource returns a validation error when project_id is empty.
func TestProjectLabelsResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not call API for empty project ID")
	}))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//labels"})
	if err == nil {
		t.Fatal("expected error for empty project ID in labels URI")
	}
}

// TestProjectMilestonesResource_EmptyProjectID verifies that ProjectMilestonesResource returns a validation error when project_id is empty.
func TestProjectMilestonesResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not call API for empty project ID")
	}))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//milestones"})
	if err == nil {
		t.Fatal("expected error for empty project ID in milestones URI")
	}
}

// TestProjectBranchesResource_EmptyProjectID verifies that ProjectBranchesResource returns a validation error when project_id is empty.
func TestProjectBranchesResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not call API for empty project ID")
	}))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//branches"})
	if err == nil {
		t.Fatal("expected error for empty project ID in branches URI")
	}
}

// TestGroupMembersResource_EmptyGroupID verifies that GroupMembersResource returns a validation error when group_id is empty.
func TestGroupMembersResource_EmptyGroupID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not call API for empty group ID")
	}))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group//members"})
	if err == nil {
		t.Fatal("expected error for empty group ID in members URI")
	}
}

// TestGroupProjectsResource_EmptyGroupID verifies that GroupProjectsResource returns a validation error when group_id is empty.
func TestGroupProjectsResource_EmptyGroupID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not call API for empty group ID")
	}))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group//projects"})
	if err == nil {
		t.Fatal("expected error for empty group ID in projects URI")
	}
}

// TestProjectIssuesResource_EmptyProjectID verifies that ProjectIssuesResource returns a validation error when project_id is empty.
func TestProjectIssuesResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not call API for empty project ID")
	}))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//issues"})
	if err == nil {
		t.Fatal("expected error for empty project ID in issues URI")
	}
}

// TestIssueResource_EmptyProjectID verifies that IssueResource returns a validation error when project_id is empty.
func TestIssueResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not call API for empty project ID")
	}))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//issue/1"})
	if err == nil {
		t.Fatal("expected error for empty project ID in issue URI")
	}
}

// TestProjectReleasesResource_EmptyProjectID verifies that ProjectReleasesResource returns a validation error when project_id is empty.
func TestProjectReleasesResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not call API for empty project ID")
	}))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//releases"})
	if err == nil {
		t.Fatal("expected error for empty project ID in releases URI")
	}
}

// TestProjectTagsResource_EmptyProjectID verifies that ProjectTagsResource returns a validation error when project_id is empty.
func TestProjectTagsResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not call API for empty project ID")
	}))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//tags"})
	if err == nil {
		t.Fatal("expected error for empty project ID in tags URI")
	}
}

// TestGroupResource_APIError verifies that GroupResource returns an error when the GitLab API responds with a failure status.
func TestGroupResource_APIError(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group/10"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestWorkflowGuides_ReadSuccess verifies that RegisterWorkflowGuides creates
// resources that can be read back via MCP and return markdown content.
func TestWorkflowGuides_ReadSuccess(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterWorkflowGuides(server)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "tc", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "gitlab://guides/git-workflow"})
	if err != nil {
		t.Fatalf("unexpected error reading workflow guide: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("expected at least 1 content item")
	}
	if result.Contents[0].Text == "" {
		t.Error("expected non-empty markdown content")
	}
}
