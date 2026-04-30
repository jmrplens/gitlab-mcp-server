// resource_extra_errors_test.go contains additional unit tests that target
// the remaining uncovered branches across MCP resource handlers. It focuses
// on three categories of error paths:
//
//  1. GitLab API failure responses (404/403/500) for resources that did not
//     yet have a dedicated API-error test.
//  2. Non-numeric identifiers in URIs that fail strconv parsing and return
//     mcp.ResourceNotFoundError.
//  3. Empty URI segments (empty project_id / group_id) caught by the
//     "extracted ID is empty" guard at the top of each handler.
//
// It also includes direct unit tests for [decodeFileContent] covering nil
// input, plain-text encoding, base64 decoding errors, and binary-file
// detection.
package resources

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// errAPIHandler returns an http.Handler that responds with the given status
// code and a generic GitLab error JSON body. Used to verify that resource
// handlers propagate GitLab API errors as MCP errors.
func errAPIHandler(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, status, `{"message":"error"}`)
	}
}

// noAPICallHandler returns an http.Handler that fails the test if the
// resource handler attempts to call the GitLab API. Used for empty-ID and
// invalid-ID URI tests where the handler must short-circuit before any
// network call.
func noAPICallHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("handler should not call API, got %s %s", r.Method, r.URL.Path)
	}
}

// API error tests for single-resource handlers that lacked one.

// TestReleaseResource_APIError verifies that the release resource returns an
// error when the GitLab API responds with a failure status.
func TestReleaseResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusInternalServerError))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/release/v1.0.0"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestBranchResource_APIError verifies that the branch resource returns an
// error when the GitLab API responds with 404.
func TestBranchResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusNotFound))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/branch/missing"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestTagResource_APIError verifies that the tag resource returns an error
// when the GitLab API responds with 404.
func TestTagResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusNotFound))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/tag/v9.9.9"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestLabelResource_APIError verifies that the label resource returns an
// error when the GitLab API responds with 404.
func TestLabelResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusNotFound))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/label/999"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestMilestoneResource_APIError verifies that the milestone resource
// returns an error when the GitLab API list call fails.
func TestMilestoneResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusInternalServerError))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/milestone/3"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestDeploymentResource_APIError verifies that the deployment resource
// returns an error when the GitLab API responds with 404.
func TestDeploymentResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusNotFound))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/deployment/17"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestEnvironmentResource_APIError verifies that the environment resource
// returns an error when the GitLab API responds with 404.
func TestEnvironmentResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusNotFound))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/environment/7"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestJobResource_APIError verifies that the job resource returns an error
// when the GitLab API responds with 404.
func TestJobResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusNotFound))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/job/555"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestSnippetResource_APIError verifies that the snippet resource returns
// an error when the GitLab API responds with 404.
func TestSnippetResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusNotFound))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://snippet/123"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestProjectSnippetResource_APIError verifies that the project_snippet
// resource returns an error when the GitLab API responds with 404.
func TestProjectSnippetResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusNotFound))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/snippet/123"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestFeatureFlagResource_APIError verifies that the feature_flag resource
// returns an error when the GitLab API responds with 404.
func TestFeatureFlagResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusNotFound))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/feature_flag/my_flag"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestDeployKeyResource_APIError verifies that the deploy_key resource
// returns an error when the GitLab API responds with 404.
func TestDeployKeyResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusNotFound))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/deploy_key/9"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestBoardResource_APIError verifies that the board resource returns an
// error when the GitLab API responds with 404.
func TestBoardResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusNotFound))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/board/3"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestGroupMilestoneResource_APIError verifies that the group_milestone
// resource returns an error when the GitLab API list call fails.
func TestGroupMilestoneResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusInternalServerError))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group/10/milestone/3"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// TestGroupMilestoneResource_NotFound verifies that the group_milestone
// resource returns ResourceNotFoundError when the IID does not exist (the
// list endpoint returns an empty array).
func TestGroupMilestoneResource_NotFound(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group/10/milestone/99"})
	if err == nil {
		t.Fatal("expected error for unknown group milestone IID")
	}
}

// TestGroupLabelResource_APIError verifies that the group_label resource
// returns an error when the GitLab API responds with 404.
func TestGroupLabelResource_APIError(t *testing.T) {
	session := newMCPSession(t, errAPIHandler(http.StatusNotFound))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group/10/label/bug"})
	if err == nil {
		t.Fatal(msgExpectedAPIErr)
	}
}

// Invalid-numeric-ID tests for handlers that call strconv.Atoi/ParseInt.

// TestMilestoneResource_InvalidIID verifies that the milestone resource
// returns ResourceNotFoundError when the milestone IID is not numeric.
func TestMilestoneResource_InvalidIID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/milestone/abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric milestone IID")
	}
}

// TestDeploymentResource_InvalidID verifies that the deployment resource
// returns ResourceNotFoundError when the deployment ID is not numeric.
func TestDeploymentResource_InvalidID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/deployment/abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric deployment ID")
	}
}

// TestEnvironmentResource_InvalidID verifies that the environment resource
// returns ResourceNotFoundError when the environment ID is not numeric.
func TestEnvironmentResource_InvalidID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/environment/abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric environment ID")
	}
}

// TestJobResource_InvalidID verifies that the job resource returns
// ResourceNotFoundError when the job ID is not numeric.
func TestJobResource_InvalidID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/job/abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric job ID")
	}
}

// TestSnippetResource_InvalidID verifies that the snippet resource returns
// ResourceNotFoundError when the snippet ID is not numeric.
func TestSnippetResource_InvalidID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://snippet/abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric snippet ID")
	}
}

// TestProjectSnippetResource_InvalidID verifies that the project_snippet
// resource returns ResourceNotFoundError when the snippet ID is not numeric.
func TestProjectSnippetResource_InvalidID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/snippet/abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric project snippet ID")
	}
}

// TestDeployKeyResource_InvalidID verifies that the deploy_key resource
// returns ResourceNotFoundError when the deploy key ID is not numeric.
func TestDeployKeyResource_InvalidID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/deploy_key/abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric deploy key ID")
	}
}

// TestBoardResource_InvalidID verifies that the board resource returns
// ResourceNotFoundError when the board ID is not numeric.
func TestBoardResource_InvalidID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/board/abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric board ID")
	}
}

// TestGroupMilestoneResource_InvalidIID verifies that the group_milestone
// resource returns ResourceNotFoundError when the milestone IID is not
// numeric.
func TestGroupMilestoneResource_InvalidIID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group/10/milestone/abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric group milestone IID")
	}
}

// Empty-URI guard tests for resources that did not have an empty-ID test.

// TestReleaseResource_EmptyProjectID verifies that the release resource
// returns ResourceNotFoundError when the project_id segment is empty.
func TestReleaseResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//release/v1.0.0"})
	if err == nil {
		t.Fatal("expected error for empty project_id in release URI")
	}
}

// TestBranchResource_EmptyProjectID verifies that the singleton branch
// resource returns ResourceNotFoundError when the project_id segment is empty.
func TestBranchResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//branch/main"})
	if err == nil {
		t.Fatal("expected error for empty project_id in branch URI")
	}
}

// TestTagResource_EmptyProjectID verifies that the singleton tag resource
// returns ResourceNotFoundError when the project_id segment is empty.
func TestTagResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//tag/v1.0.0"})
	if err == nil {
		t.Fatal("expected error for empty project_id in tag URI")
	}
}

// TestLabelResource_EmptyProjectID verifies that the singleton label
// resource returns ResourceNotFoundError when the project_id segment is empty.
func TestLabelResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//label/5"})
	if err == nil {
		t.Fatal("expected error for empty project_id in label URI")
	}
}

// TestMilestoneResource_EmptyProjectID verifies that the singleton milestone
// resource returns ResourceNotFoundError when the project_id segment is empty.
func TestMilestoneResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//milestone/3"})
	if err == nil {
		t.Fatal("expected error for empty project_id in milestone URI")
	}
}

// TestDeploymentResource_EmptyProjectID verifies that the deployment
// resource returns ResourceNotFoundError when the project_id segment is empty.
func TestDeploymentResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//deployment/17"})
	if err == nil {
		t.Fatal("expected error for empty project_id in deployment URI")
	}
}

// TestEnvironmentResource_EmptyProjectID verifies that the environment
// resource returns ResourceNotFoundError when the project_id segment is empty.
func TestEnvironmentResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//environment/7"})
	if err == nil {
		t.Fatal("expected error for empty project_id in environment URI")
	}
}

// TestJobResource_EmptyProjectID verifies that the job resource returns
// ResourceNotFoundError when the project_id segment is empty.
func TestJobResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//job/555"})
	if err == nil {
		t.Fatal("expected error for empty project_id in job URI")
	}
}

// TestSnippetResource_EmptyID verifies that the personal snippet resource
// returns ResourceNotFoundError when the snippet ID is missing.
func TestSnippetResource_EmptyID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://snippet/"})
	if err == nil {
		t.Fatal("expected error for empty snippet ID")
	}
}

// TestProjectSnippetResource_EmptyProjectID verifies that the project
// snippet resource returns ResourceNotFoundError when the project_id segment
// is empty.
func TestProjectSnippetResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//snippet/123"})
	if err == nil {
		t.Fatal("expected error for empty project_id in snippet URI")
	}
}

// TestFeatureFlagResource_EmptyProjectID verifies that the feature_flag
// resource returns ResourceNotFoundError when the project_id segment is empty.
func TestFeatureFlagResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//feature_flag/my_flag"})
	if err == nil {
		t.Fatal("expected error for empty project_id in feature_flag URI")
	}
}

// TestDeployKeyResource_EmptyProjectID verifies that the deploy_key resource
// returns ResourceNotFoundError when the project_id segment is empty.
func TestDeployKeyResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//deploy_key/9"})
	if err == nil {
		t.Fatal("expected error for empty project_id in deploy_key URI")
	}
}

// TestBoardResource_EmptyProjectID verifies that the board resource returns
// ResourceNotFoundError when the project_id segment is empty.
func TestBoardResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//board/3"})
	if err == nil {
		t.Fatal("expected error for empty project_id in board URI")
	}
}

// TestGroupMilestoneResource_EmptyGroupID verifies that the group_milestone
// resource returns ResourceNotFoundError when the group_id segment is empty.
func TestGroupMilestoneResource_EmptyGroupID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group//milestone/3"})
	if err == nil {
		t.Fatal("expected error for empty group_id in milestone URI")
	}
}

// TestGroupLabelResource_EmptyGroupID verifies that the group_label
// resource returns ResourceNotFoundError when the group_id segment is empty.
func TestGroupLabelResource_EmptyGroupID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://group//label/bug"})
	if err == nil {
		t.Fatal("expected error for empty group_id in group label URI")
	}
}

// TestPipelineResource_EmptyProjectID verifies that the pipeline resource
// returns ResourceNotFoundError when the project_id segment is empty.
func TestPipelineResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//pipeline/100"})
	if err == nil {
		t.Fatal("expected error for empty project_id in pipeline URI")
	}
}

// TestPipelineJobsResource_EmptyProjectID verifies that the pipeline_jobs
// resource returns ResourceNotFoundError when the project_id segment is empty.
func TestPipelineJobsResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//pipeline/100/jobs"})
	if err == nil {
		t.Fatal("expected error for empty project_id in pipeline_jobs URI")
	}
}

// TestMergeRequestNotesResource_EmptyProjectID verifies that the
// merge_request_notes resource returns ResourceNotFoundError when the
// project_id segment is empty.
func TestMergeRequestNotesResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//mr/7/notes"})
	if err == nil {
		t.Fatal("expected error for empty project_id in MR notes URI")
	}
}

// TestMergeRequestDiscussionsResource_EmptyProjectID verifies that the
// merge_request_discussions resource returns ResourceNotFoundError when the
// project_id segment is empty.
func TestMergeRequestDiscussionsResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//mr/7/discussions"})
	if err == nil {
		t.Fatal("expected error for empty project_id in MR discussions URI")
	}
}

// TestMergeRequestDiscussionsResource_BadIID verifies that a non-numeric MR
// IID returns a resource-not-found error from the discussions handler.
func TestMergeRequestDiscussionsResource_BadIID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project/42/mr/notanumber/discussions"})
	if err == nil {
		t.Fatal("expected error for non-numeric MR IID in discussions URI")
	}
}

// TestWikiResource_EmptyProjectID verifies that the wiki_page resource
// returns ResourceNotFoundError when the project_id segment is empty.
func TestWikiResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//wiki/some-page"})
	if err == nil {
		t.Fatal("expected error for empty project_id in wiki URI")
	}
}

// TestCommitResource_EmptyProjectID verifies that the commit resource
// returns ResourceNotFoundError when the project_id segment is empty.
func TestCommitResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//commit/abc123"})
	if err == nil {
		t.Fatal("expected error for empty project_id in commit URI")
	}
}

// TestFileBlobResource_EmptyProjectID verifies that the file_blob resource
// returns ResourceNotFoundError when the project_id segment is empty.
func TestFileBlobResource_EmptyProjectID(t *testing.T) {
	session := newMCPSession(t, noAPICallHandler(t))
	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://project//file/main/README.md"})
	if err == nil {
		t.Fatal("expected error for empty project_id in file_blob URI")
	}
}

// Direct decodeFileContent unit tests covering nil input, plain-text and
// binary encodings, base64 decode error, and post-decode binary detection.

// TestDecodeFileContent_Nil verifies that [decodeFileContent] returns
// ("", "binary") when given a nil [gl.File] pointer.
func TestDecodeFileContent_Nil(t *testing.T) {
	content, category := decodeFileContent(nil)
	if content != "" || category != "binary" {
		t.Errorf("decodeFileContent(nil) = (%q, %q), want (\"\", \"binary\")", content, category)
	}
}

// TestDecodeFileContent_PlainTextEncoding verifies that a file with a
// non-base64 encoding and a textual file name returns the raw content as text.
func TestDecodeFileContent_PlainTextEncoding(t *testing.T) {
	f := &gl.File{FileName: "README.md", Encoding: "text", Content: "hello world"}
	content, category := decodeFileContent(f)
	if content != "hello world" || category != "text" {
		t.Errorf("decodeFileContent(plain) = (%q, %q), want (\"hello world\", \"text\")", content, category)
	}
}

// TestDecodeFileContent_PlainTextEncoding_BinaryFile verifies that a file
// with a non-base64 encoding but a binary file extension returns
// ("", "binary"), suppressing content for the JSON response.
func TestDecodeFileContent_PlainTextEncoding_BinaryFile(t *testing.T) {
	f := &gl.File{FileName: "archive.zip", Encoding: "text", Content: "ignored"}
	content, category := decodeFileContent(f)
	if content != "" || category != "binary" {
		t.Errorf("decodeFileContent(binary plain) = (%q, %q), want (\"\", \"binary\")", content, category)
	}
}

// TestDecodeFileContent_Base64DecodeError verifies that an invalid base64
// payload causes [decodeFileContent] to return ("", "binary").
func TestDecodeFileContent_Base64DecodeError(t *testing.T) {
	f := &gl.File{FileName: "README.md", Encoding: "base64", Content: "!!!not-base64!!!"}
	content, category := decodeFileContent(f)
	if content != "" || category != "binary" {
		t.Errorf("decodeFileContent(invalid base64) = (%q, %q), want (\"\", \"binary\")", content, category)
	}
}

// TestDecodeFileContent_Base64BinaryFile verifies that a file with a binary
// extension (e.g. .pdf) returns ("", "binary") even when the base64 content
// decodes successfully, suppressing the binary payload.
func TestDecodeFileContent_Base64BinaryFile(t *testing.T) {
	// "aGVsbG8=" decodes to "hello"; the .pdf extension forces binary classification.
	f := &gl.File{FileName: "manual.pdf", Encoding: "base64", Content: "aGVsbG8="}
	content, category := decodeFileContent(f)
	if content != "" || category != "binary" {
		t.Errorf("decodeFileContent(binary base64) = (%q, %q), want (\"\", \"binary\")", content, category)
	}
}
