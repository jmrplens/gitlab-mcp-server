// completions_test.go contains unit tests for the [Handler.Complete] dispatch
// logic. Tests cover prompt and resource argument completion for project IDs,
// group IDs, usernames, MR IIDs, issue IIDs, branches, and tags using
// httptest mocks.
package completions

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// Shared test assertion messages and endpoint paths.
const (
	fmtUnexpectedErr        = "unexpected error: %v"
	fmtEmptyValuesNoProject = "expected empty values without project_id, got %d"
	fmtEmptyValues          = "expected empty values, got %d"
	fmtExpected2Values      = "expected 2 values, got %d: %v"
	pathRepoBranches        = "/api/v4/projects/42/repository/branches"
	pathRepoTags            = "/api/v4/projects/42/repository/tags"
	refPrompt               = "ref/prompt"
	refResource             = "ref/resource"
	fmtUnexpectedValue      = "unexpected value: %s"
)

// TestComplete_NilRef verifies that [Handler.Complete] returns empty results
// when the request has no reference.
func TestComplete_NilRef(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Argument: mcp.CompleteParamsArgument{Name: "project_id", Value: "test"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValues, len(result.Completion.Values))
	}
}

// TestComplete_UnknownRefType verifies that [Handler.Complete] returns empty
// results for an unrecognized reference type.
func TestComplete_UnknownRefType(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: "ref/unknown"},
		Argument: mcp.CompleteParamsArgument{Name: "x", Value: "y"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValues, len(result.Completion.Values))
	}
}

// TestComplete_PromptProjectID verifies that completing a prompt's project_id
// argument returns matching projects from the GitLab API.
func TestComplete_PromptProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"path_with_namespace":"group/my-project"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "review_mr"},
		Argument: mcp.CompleteParamsArgument{Name: "project_id", Value: "my"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 1 {
		t.Fatalf(fmtExpected1Value, len(result.Completion.Values))
	}
	if result.Completion.Values[0] != "group/my-project" {
		t.Errorf(fmtUnexpectedValue, result.Completion.Values[0])
	}
}

// TestComplete_PromptUsername verifies that completing a prompt's username
// argument returns matching GitLab users.
func TestComplete_PromptUsername(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"username":"alice"},{"id":11,"username":"bob"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "daily_standup"},
		Argument: mcp.CompleteParamsArgument{Name: "username", Value: "al"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(result.Completion.Values))
	}
	if result.Completion.Values[0] != "alice" {
		t.Errorf("unexpected first value: %s", result.Completion.Values[0])
	}
}

// TestComplete_PromptMRIID verifies that completing a prompt's merge_request_iid argument
// returns merge requests filtered by IID prefix when project_id is provided.
func TestComplete_PromptMRIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests" {
			testutil.RespondJSON(w, http.StatusOK, `[{"iid":1,"title":"Fix bug"},{"iid":12,"title":"Add feature"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "review_mr"},
		Argument: mcp.CompleteParamsArgument{Name: "merge_request_iid", Value: "1"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 2 {
		t.Fatalf("expected 2 values (1 and 12 both start with '1'), got %d: %v", len(result.Completion.Values), result.Completion.Values)
	}
}

// TestComplete_PromptMRIIDWithoutProjectID verifies that merge_request_iid completion
// returns empty results when no project_id is in the resolved arguments.
func TestComplete_PromptMRIIDWithoutProjectID(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "review_mr"},
		Argument: mcp.CompleteParamsArgument{Name: "merge_request_iid", Value: "1"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValuesNoProject, len(result.Completion.Values))
	}
}

// TestComplete_PromptUnknownArg verifies that completing an unrecognized
// prompt argument returns empty results.
func TestComplete_PromptUnknownArg(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "review_mr"},
		Argument: mcp.CompleteParamsArgument{Name: "unknown_arg", Value: "x"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf("expected empty values for unknown arg, got %d", len(result.Completion.Values))
	}
}

// TestComplete_ResourceProjectID verifies that completing a resource template's
// project_id parameter returns matching projects.
func TestComplete_ResourceProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":5,"path_with_namespace":"team/backend"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refResource, URI: "gitlab://{project_id}/branches"},
		Argument: mcp.CompleteParamsArgument{Name: "project_id", Value: "back"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 1 {
		t.Fatalf(fmtExpected1Value, len(result.Completion.Values))
	}
	if result.Completion.Values[0] != "team/backend" {
		t.Errorf(fmtUnexpectedValue, result.Completion.Values[0])
	}
}

// TestComplete_ResourceGroupID verifies that completing a resource template's
// group_id parameter returns matching groups.
func TestComplete_ResourceGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":3,"full_path":"engineering/backend"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refResource, URI: "gitlab://{group_id}/milestones"},
		Argument: mcp.CompleteParamsArgument{Name: "group_id", Value: "eng"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 1 {
		t.Fatalf(fmtExpected1Value, len(result.Completion.Values))
	}
	if result.Completion.Values[0] != "engineering/backend" {
		t.Errorf(fmtUnexpectedValue, result.Completion.Values[0])
	}
}

// TestComplete_PromptGroupID verifies that completing a prompt's group_id
// argument returns matching groups, covering the group_id case in completePromptArg.
func TestComplete_PromptGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":5,"full_path":"platform/infra"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "team_overview"},
		Argument: mcp.CompleteParamsArgument{Name: "group_id", Value: "plat"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 1 {
		t.Fatalf(fmtExpected1Value, len(result.Completion.Values))
	}
	if result.Completion.Values[0] != "platform/infra" {
		t.Errorf(fmtUnexpectedValue, result.Completion.Values[0])
	}
}

// TestComplete_PromptGroupMilestoneTitle verifies milestone completion falls
// back to group scope when project_id is absent and group_id is resolved.
func TestComplete_PromptGroupMilestoneTitle(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/99/milestones" {
			if gotSearch := r.URL.Query().Get("search"); gotSearch != "v" {
				t.Errorf("search query = %q, want v", gotSearch)
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"title":"v1.0","state":"active"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "group_milestone_progress"},
		Argument: mcp.CompleteParamsArgument{Name: "milestone", Value: "v"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"group_id": "99"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 1 {
		t.Fatalf(fmtExpected1Value, len(result.Completion.Values))
	}
	if result.Completion.Values[0] != "v1.0" {
		t.Errorf(fmtUnexpectedValue, result.Completion.Values[0])
	}
}

// TestComplete_PromptMilestoneWithoutScope verifies milestone completion is
// empty when neither project_id nor group_id is resolved.
func TestComplete_PromptMilestoneWithoutScope(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "group_milestone_progress"},
		Argument: mcp.CompleteParamsArgument{Name: "milestone", Value: "v"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValues, len(result.Completion.Values))
	}
}

// TestCompleteGroupMilestoneTitle_APIErrorReturnsEmpty verifies group milestone
// search failures are handled as empty completions.
func TestCompleteGroupMilestoneTitle_APIErrorReturnsEmpty(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})))

	result, err := h.completeGroupMilestoneTitle(context.Background(), "99", "v")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValues, len(result.Completion.Values))
	}
}

// TestComplete_APIErrorReturnsEmpty verifies that [Handler.Complete] returns
// empty results instead of an error when the GitLab API call fails.
func TestComplete_APIErrorReturnsEmpty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "review_mr"},
		Argument: mcp.CompleteParamsArgument{Name: "project_id", Value: "x"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error on API failure, got: %v", err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf("expected empty values on API error, got %d", len(result.Completion.Values))
	}
}

// TestComplete_ContextCancelled verifies that [Handler.Complete] returns empty
// results gracefully when the context is already canceled.
func TestComplete_ContextCancelled(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))

	ctx := testutil.CancelledCtx(t)

	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt},
		Argument: mcp.CompleteParamsArgument{Name: "project_id", Value: "x"},
	}

	result, err := h.Complete(ctx, req)
	if err != nil {
		t.Fatalf("expected no error (graceful), got: %v", err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf("expected empty values on canceled context, got %d", len(result.Completion.Values))
	}
}

// TestComplete_PromptIssueIID verifies that completing a prompt's issue_iid
// argument returns issues filtered by IID prefix when project_id is provided.
func TestComplete_PromptIssueIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/issues" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":200,"iid":7,"title":"Login bug"},{"id":201,"iid":71,"title":"Perf issue"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "issue_detail"},
		Argument: mcp.CompleteParamsArgument{Name: "issue_iid", Value: "7"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 2 {
		t.Fatalf("expected 2 values matching '7', got %d: %v", len(result.Completion.Values), result.Completion.Values)
	}
}

// TestComplete_PromptIssueIIDWithoutProjectID verifies that issue_iid
// completion returns empty results when no project_id is resolved.
func TestComplete_PromptIssueIIDWithoutProjectID(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "issue_detail"},
		Argument: mcp.CompleteParamsArgument{Name: "issue_iid", Value: "7"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValuesNoProject, len(result.Completion.Values))
	}
}

// TestComplete_PromptFrom verifies that completing the "from" argument returns
// both branches and tags from the specified project.
func TestComplete_PromptFrom(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathRepoBranches:
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"main","default":true}]`)
		case pathRepoTags:
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"v1.0.0"}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "compare_commits"},
		Argument: mcp.CompleteParamsArgument{Name: "from", Value: "m"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 2 {
		t.Fatalf("expected 2 values (branch + tag), got %d: %v", len(result.Completion.Values), result.Completion.Values)
	}
}

// TestComplete_PromptToWithoutProjectID verifies that "to" argument completion
// returns empty results when no project_id is resolved.
func TestComplete_PromptToWithoutProjectID(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "compare_commits"},
		Argument: mcp.CompleteParamsArgument{Name: "to", Value: "x"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValuesNoProject, len(result.Completion.Values))
	}
}

// TestComplete_PromptTag verifies that completing the "tag" argument returns
// matching tags from the specified project.
func TestComplete_PromptTag(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoTags {
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"v1.0.0"},{"name":"v2.0.0"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "release_detail"},
		Argument: mcp.CompleteParamsArgument{Name: "tag", Value: "v"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 2 {
		t.Fatalf(fmtExpected2Values, len(result.Completion.Values), result.Completion.Values)
	}
}

// TestComplete_PromptTagWithoutProjectID verifies that tag completion returns
// empty results when no project_id is resolved.
func TestComplete_PromptTagWithoutProjectID(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "release_detail"},
		Argument: mcp.CompleteParamsArgument{Name: "tag", Value: "v"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValuesNoProject, len(result.Completion.Values))
	}
}

// TestComplete_ResourceMRIID verifies that completing a resource template's
// merge_request_iid parameter returns matching merge requests.
func TestComplete_ResourceMRIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/merge_requests" {
			testutil.RespondJSON(w, http.StatusOK, `[{"iid":5,"title":"Hotfix"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refResource, URI: "gitlab://{project_id}/mr/{merge_request_iid}"},
		Argument: mcp.CompleteParamsArgument{Name: "merge_request_iid", Value: "5"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "10"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result.Completion.Values), result.Completion.Values)
	}
}

// TestComplete_ResourceMRIIDWithoutProjectID verifies that resource merge_request_iid
// completion returns empty results when no project_id is resolved.
func TestComplete_ResourceMRIIDWithoutProjectID(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refResource, URI: "gitlab://{project_id}/mr/{merge_request_iid}"},
		Argument: mcp.CompleteParamsArgument{Name: "merge_request_iid", Value: "5"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValues, len(result.Completion.Values))
	}
}

// TestComplete_ResourceIssueIID verifies that completing a resource template's
// issue_iid parameter returns matching issues.
func TestComplete_ResourceIssueIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/issues" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":300,"iid":9,"title":"Bug report"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refResource, URI: "gitlab://{project_id}/issues/{issue_iid}"},
		Argument: mcp.CompleteParamsArgument{Name: "issue_iid", Value: "9"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "10"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result.Completion.Values), result.Completion.Values)
	}
}

// TestComplete_ResourceIssueIIDWithoutProjectID verifies that resource
// issue_iid completion returns empty results when no project_id is resolved.
func TestComplete_ResourceIssueIIDWithoutProjectID(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refResource, URI: "gitlab://{project_id}/issues/{issue_iid}"},
		Argument: mcp.CompleteParamsArgument{Name: "issue_iid", Value: "9"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValues, len(result.Completion.Values))
	}
}

// TestComplete_ResourceUnknownArg verifies that completing an unrecognized
// resource parameter returns empty results.
func TestComplete_ResourceUnknownArg(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refResource, URI: "gitlab://{project_id}/unknown"},
		Argument: mcp.CompleteParamsArgument{Name: "unknown_param", Value: "x"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf("expected empty values for unknown resource arg, got %d", len(result.Completion.Values))
	}
}

// TestComplete_PromptPipelineID verifies that completing a prompt's pipeline_id
// argument returns pipelines filtered by ID prefix when project_id is provided.
func TestComplete_PromptPipelineID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/pipelines" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":100,"ref":"main","status":"success"},{"id":101,"ref":"develop","status":"running"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "pipeline_status"},
		Argument: mcp.CompleteParamsArgument{Name: "pipeline_id", Value: "10"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 2 {
		t.Fatalf("expected 2 values matching '10', got %d: %v", len(result.Completion.Values), result.Completion.Values)
	}
	if result.Completion.Values[0] != "100" {
		t.Errorf(fmtUnexpectedValue, result.Completion.Values[0])
	}
}

// TestComplete_PromptPipelineIDWithoutProjectID verifies that pipeline_id
// completion returns empty results when no project_id is resolved.
func TestComplete_PromptPipelineIDWithoutProjectID(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "pipeline_status"},
		Argument: mcp.CompleteParamsArgument{Name: "pipeline_id", Value: "10"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValuesNoProject, len(result.Completion.Values))
	}
}

// TestComplete_PromptSHA verifies that completing a prompt's sha argument
// returns commits filtered by SHA prefix when project_id is provided.
func TestComplete_PromptSHA(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/repository/commits" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":"abc123def","short_id":"abc123d","title":"Fix bug"},
				{"id":"abc999fff","short_id":"abc999f","title":"Add feature"},
				{"id":"def456ghi","short_id":"def456g","title":"Docs"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt, Name: "commit_detail"},
		Argument: mcp.CompleteParamsArgument{Name: "sha", Value: "abc"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 2 {
		t.Fatalf("expected 2 values matching 'abc', got %d: %v", len(result.Completion.Values), result.Completion.Values)
	}
}

// TestComplete_PromptSHAWithoutProjectID verifies that sha completion returns
// empty results when no project_id is resolved.
func TestComplete_PromptSHAWithoutProjectID(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt},
		Argument: mcp.CompleteParamsArgument{Name: "sha", Value: "abc"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValuesNoProject, len(result.Completion.Values))
	}
}

// TestComplete_PromptRef verifies that completing a prompt's ref argument
// returns both branches and tags.
func TestComplete_PromptRef(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathRepoBranches:
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"main","default":true}]`)
		case pathRepoTags:
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"v1.0.0"}]`)
		default:
			http.NotFound(w, r)
		}
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt},
		Argument: mcp.CompleteParamsArgument{Name: "ref", Value: ""},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 2 {
		t.Fatalf("expected 2 values (branch + tag), got %d: %v", len(result.Completion.Values), result.Completion.Values)
	}
}

// TestComplete_PromptBranch verifies that completing a prompt's branch argument
// returns only branches (not tags).
func TestComplete_PromptBranch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoBranches {
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"main"},{"name":"develop"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)

	for _, argName := range []string{"branch", "source_branch", "target_branch"} {
		t.Run(argName, func(t *testing.T) {
			req := &mcp.CompleteRequest{}
			req.Params = &mcp.CompleteParams{
				Ref:      &mcp.CompleteReference{Type: refPrompt},
				Argument: mcp.CompleteParamsArgument{Name: argName, Value: ""},
				Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}},
			}

			result, err := h.Complete(context.Background(), req)
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if len(result.Completion.Values) != 2 {
				t.Fatalf("expected 2 branch values, got %d: %v", len(result.Completion.Values), result.Completion.Values)
			}
		})
	}
}

// TestComplete_PromptBranchWithoutProjectID verifies that branch completion
// returns empty results when no project_id is resolved.
func TestComplete_PromptBranchWithoutProjectID(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))

	for _, argName := range []string{"branch", "source_branch", "target_branch"} {
		t.Run(argName, func(t *testing.T) {
			req := &mcp.CompleteRequest{}
			req.Params = &mcp.CompleteParams{
				Ref:      &mcp.CompleteReference{Type: refPrompt},
				Argument: mcp.CompleteParamsArgument{Name: argName, Value: "main"},
			}

			result, err := h.Complete(context.Background(), req)
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if len(result.Completion.Values) != 0 {
				t.Errorf(fmtEmptyValuesNoProject, len(result.Completion.Values))
			}
		})
	}
}

// TestComplete_PromptLabel verifies that completing a prompt's label argument
// returns project labels.
func TestComplete_PromptLabel(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/labels" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"bug"},{"id":2,"name":"enhancement"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt},
		Argument: mcp.CompleteParamsArgument{Name: "label", Value: "bug"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 2 {
		t.Fatalf(fmtExpected2Values, len(result.Completion.Values), result.Completion.Values)
	}
	if result.Completion.Values[0] != "bug" {
		t.Errorf(fmtUnexpectedValue, result.Completion.Values[0])
	}
}

// TestComplete_PromptLabelWithoutProjectID verifies that label completion
// returns empty results when no project_id is resolved.
func TestComplete_PromptLabelWithoutProjectID(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt},
		Argument: mcp.CompleteParamsArgument{Name: "label", Value: "bug"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValuesNoProject, len(result.Completion.Values))
	}
}

// TestComplete_PromptMilestoneID verifies that completing a prompt's
// milestone_id argument returns project milestones.
func TestComplete_PromptMilestoneID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/milestones" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"title":"v1.0","state":"active"},{"id":2,"title":"v2.0","state":"active"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt},
		Argument: mcp.CompleteParamsArgument{Name: "milestone_id", Value: "v1"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 2 {
		t.Fatalf(fmtExpected2Values, len(result.Completion.Values), result.Completion.Values)
	}
	if result.Completion.Values[0] != "1" {
		t.Errorf(fmtUnexpectedValue, result.Completion.Values[0])
	}
}

// TestComplete_PromptMilestoneIDWithoutProjectID verifies that milestone_id
// completion returns empty results when no project_id is resolved.
func TestComplete_PromptMilestoneIDWithoutProjectID(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt},
		Argument: mcp.CompleteParamsArgument{Name: "milestone_id", Value: "v1"},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValuesNoProject, len(result.Completion.Values))
	}
}

// TestComplete_PromptMilestoneTitle verifies that completing a prompt's
// "milestone" argument (title-based, used by the milestone_progress prompt)
// returns plain milestone titles. Distinct from "milestone_id" which returns
// numeric IDs.
func TestComplete_PromptMilestoneTitle(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/milestones" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"title":"Sprint 1","state":"active"},
				{"id":2,"title":"Sprint 2","state":"active"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt},
		Argument: mcp.CompleteParamsArgument{Name: "milestone", Value: "Sprint"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 2 {
		t.Fatalf(fmtExpected2Values, len(result.Completion.Values), result.Completion.Values)
	}
	if result.Completion.Values[0] != "Sprint 1" {
		t.Errorf("milestone title[0] = %q, want %q", result.Completion.Values[0], "Sprint 1")
	}
	if result.Completion.Values[1] != "Sprint 2" {
		t.Errorf("milestone title[1] = %q, want %q", result.Completion.Values[1], "Sprint 2")
	}
}

// TestComplete_PromptMilestoneTitle_APIError verifies that completion errors
// from the GitLab API are swallowed and an empty result is returned (so the
// LLM-facing UI degrades gracefully).
func TestComplete_PromptMilestoneTitle_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt},
		Argument: mcp.CompleteParamsArgument{Name: "milestone", Value: "x"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error on API failure, got: %v", err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf(fmtEmptyValues, len(result.Completion.Values))
	}
}

// TestComplete_PromptJobID verifies that completing a prompt's job_id argument
// returns jobs for a pipeline when both project_id and pipeline_id are provided.
func TestComplete_PromptJobID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/pipelines/10/jobs" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":501,"name":"build","status":"success","pipeline":{"id":10}},
				{"id":502,"name":"test","status":"running","pipeline":{"id":10}}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	h := NewHandler(client)
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt},
		Argument: mcp.CompleteParamsArgument{Name: "job_id", Value: "50"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42", "pipeline_id": "10"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 2 {
		t.Fatalf(fmtExpected2Values, len(result.Completion.Values), result.Completion.Values)
	}
	if result.Completion.Values[0] != "501" {
		t.Errorf(fmtUnexpectedValue, result.Completion.Values[0])
	}
}

// TestComplete_PromptJobIDWithoutDependencies verifies that job_id completion
// returns empty results when project_id or pipeline_id is missing.
func TestComplete_PromptJobIDWithoutDependencies(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))

	tests := []struct {
		name string
		args map[string]string
	}{
		{"no context", nil},
		{"only project_id", map[string]string{"project_id": "42"}},
		{"only pipeline_id", map[string]string{"pipeline_id": "10"}},
		{"empty pipeline_id", map[string]string{"project_id": "42", "pipeline_id": ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &mcp.CompleteRequest{}
			params := &mcp.CompleteParams{
				Ref:      &mcp.CompleteReference{Type: refPrompt},
				Argument: mcp.CompleteParamsArgument{Name: "job_id", Value: "50"},
			}
			if tt.args != nil {
				params.Context = &mcp.CompleteContext{Arguments: tt.args}
			}
			req.Params = params

			result, err := h.Complete(context.Background(), req)
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if len(result.Completion.Values) != 0 {
				t.Errorf(fmtEmptyValues, len(result.Completion.Values))
			}
		})
	}
}

// TestComplete_PromptJobIDInvalidPipelineID verifies that job_id completion
// returns empty results when pipeline_id is not a valid integer.
func TestComplete_PromptJobIDInvalidPipelineID(t *testing.T) {
	h := NewHandler(testutil.NewTestClient(t, http.NotFoundHandler()))
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: refPrompt},
		Argument: mcp.CompleteParamsArgument{Name: "job_id", Value: "50"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42", "pipeline_id": "not-a-number"}},
	}

	result, err := h.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf("expected empty values for invalid pipeline_id, got %d", len(result.Completion.Values))
	}
}

// TestComplete_APIErrorPaths verifies that each complete* method returns empty
// results (not an error) when the underlying GitLab API returns an error.
// This covers the error-handling branches in all complete* methods.
func TestComplete_APIErrorPaths(t *testing.T) {
	errHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	tests := []struct {
		name    string
		refType string
		argName string
		context *mcp.CompleteContext
	}{
		{"group_id", refResource, "group_id", nil},
		{"merge_request_iid", refPrompt, "merge_request_iid", &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}}},
		{"issue_iid", refPrompt, "issue_iid", &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}}},
		{"username", refPrompt, "username", nil},
		{"from (branch+tag)", refPrompt, "from", &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}}},
		{"tag", refPrompt, "tag", &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}}},
		{"pipeline_id", refPrompt, "pipeline_id", &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}}},
		{"sha", refPrompt, "sha", &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}}},
		{"branch", refPrompt, "branch", &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}}},
		{"label", refPrompt, "label", &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}}},
		{"milestone_id", refPrompt, "milestone_id", &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42"}}},
		{"job_id", refPrompt, "job_id", &mcp.CompleteContext{Arguments: map[string]string{"project_id": "42", "pipeline_id": "99"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(testutil.NewTestClient(t, errHandler))
			req := &mcp.CompleteRequest{}
			req.Params = &mcp.CompleteParams{
				Ref:      &mcp.CompleteReference{Type: tt.refType},
				Argument: mcp.CompleteParamsArgument{Name: tt.argName, Value: "x"},
				Context:  tt.context,
			}

			result, err := h.Complete(context.Background(), req)
			if err != nil {
				t.Fatalf("expected no error on API failure, got: %v", err)
			}
			if len(result.Completion.Values) != 0 {
				t.Errorf("expected empty values, got %d", len(result.Completion.Values))
			}
		})
	}
}

// Shared test assertion message for expected value counts.
const fmtExpected1Value = "expected 1 value, got %d"

// Shared assertion format for unexpected values in format helper tests.
const fmtUnexpected = "unexpected: %s"

// TestToResult_LimitEnforced verifies that [toResult] caps values at
// [maxCompletionResults] and sets HasMore when input exceeds the limit.
// Total is omitted (zero) because [toResult] does not know the upstream count.
func TestToResult_LimitEnforced(t *testing.T) {
	values := make([]string, 15)
	for i := range values {
		values[i] = "item"
	}

	result := toResult(values)
	if len(result.Completion.Values) != maxCompletionResults {
		t.Errorf("expected %d values, got %d", maxCompletionResults, len(result.Completion.Values))
	}
	if !result.Completion.HasMore {
		t.Error("expected HasMore=true when results exceed max")
	}
	if result.Completion.Total != 0 {
		t.Errorf("expected Total=0 (unknown) when not provided, got %d", result.Completion.Total)
	}
}

// TestToResult_UnderLimit verifies that [toResult] returns all values and
// HasMore=false when input is within the limit. Total is omitted because
// [toResult] does not know the upstream count.
func TestToResult_UnderLimit(t *testing.T) {
	values := []string{"a", "b", "c"}
	result := toResult(values)
	if len(result.Completion.Values) != 3 {
		t.Errorf("expected 3 values, got %d", len(result.Completion.Values))
	}
	if result.Completion.HasMore {
		t.Error("expected HasMore=false when under limit")
	}
	if result.Completion.Total != 0 {
		t.Errorf("expected Total=0 (unknown), got %d", result.Completion.Total)
	}
}

// TestToResult_Empty verifies that [toResult] handles an empty input slice.
func TestToResult_Empty(t *testing.T) {
	result := toResult([]string{})
	if len(result.Completion.Values) != 0 {
		t.Errorf("expected 0 values, got %d", len(result.Completion.Values))
	}
	if result.Completion.HasMore {
		t.Error("expected HasMore=false for empty")
	}
}

// TestEmptyResult verifies that [emptyResult] returns a non-nil result with
// no completion values.
func TestEmptyResult(t *testing.T) {
	result := emptyResult()
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Completion.Values) != 0 {
		t.Errorf("expected 0 values in empty result, got %d", len(result.Completion.Values))
	}
}

// TestFormatProjectEntry verifies that project entries are the canonical
// path-with-namespace (the value GitLab API accepts as project_id).
func TestFormatProjectEntry(t *testing.T) {
	got := formatProjectEntry(42, "team/backend")
	if got != "team/backend" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatGroupEntry verifies that group entries are the canonical full path
// (the value GitLab API accepts as group_id).
func TestFormatGroupEntry(t *testing.T) {
	got := formatGroupEntry(10, "engineering/platform")
	if got != "engineering/platform" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatMREntry verifies that merge request entries are the bare IID.
func TestFormatMREntry(t *testing.T) {
	got := formatMREntry(5, "Fix login issue")
	if got != "5" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatMREntry_LongTitleIgnored verifies that [formatMREntry] returns
// only the IID even when the title is long (titles are not part of the value).
func TestFormatMREntry_LongTitleIgnored(t *testing.T) {
	longTitle := "This is a very long merge request title that exceeds the sixty character limit for display"
	got := formatMREntry(1, longTitle)
	if got != "1" {
		t.Errorf("expected bare IID '1', got %q", got)
	}
}

// TestFormatIssueEntry verifies that issue entries are the bare IID.
func TestFormatIssueEntry(t *testing.T) {
	got := formatIssueEntry(10, "Bug report")
	if got != "10" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFilterByPrefix verifies that [filterByPrefix] performs case-insensitive
// substring matching and returns all values when the query is empty.
func TestFilterByPrefix(t *testing.T) {
	values := []string{"alpha", "beta", "GAMMA", "delta-alpha"}

	t.Run("match", func(t *testing.T) {
		got := filterByPrefix(values, "alpha")
		if len(got) != 2 {
			t.Fatalf("expected 2 matches, got %d: %v", len(got), got)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		got := filterByPrefix(values, "GAMMA")
		if len(got) != 1 {
			t.Fatalf("expected 1 match, got %d: %v", len(got), got)
		}
	})

	t.Run("empty query returns all", func(t *testing.T) {
		got := filterByPrefix(values, "")
		if len(got) != len(values) {
			t.Errorf("expected all values for empty query, got %d", len(got))
		}
	})

	t.Run("no match", func(t *testing.T) {
		got := filterByPrefix(values, "zeta")
		if len(got) != 0 {
			t.Errorf("expected 0 matches, got %d", len(got))
		}
	})
}

// TestResolvedArguments_Nil uses table-driven subtests to verify that
// [resolvedArguments] returns an empty map for nil context and nil arguments.
func TestResolvedArguments_Nil(t *testing.T) {
	tests := []struct {
		name string
		ctx  *mcp.CompleteContext
	}{
		{"nil context", nil},
		{"nil arguments", &mcp.CompleteContext{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &mcp.CompleteRequest{}
			req.Params = &mcp.CompleteParams{
				Context: tt.ctx,
			}
			got := resolvedArguments(req)
			if got == nil {
				t.Error("expected non-nil map")
			}
			if len(got) != 0 {
				t.Errorf("expected empty map, got %d entries", len(got))
			}
		})
	}
}

// TestFormatPipelineEntry verifies that pipeline entries are the bare ID.
func TestFormatPipelineEntry(t *testing.T) {
	got := formatPipelineEntry(100, "main", "success")
	if got != "100" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatCommitEntry verifies that commit entries are the bare short SHA.
func TestFormatCommitEntry(t *testing.T) {
	got := formatCommitEntry("abc123d", "Fix login bug")
	if got != "abc123d" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatCommitEntry_LongTitleIgnored verifies that long titles do not
// alter the bare-SHA value.
func TestFormatCommitEntry_LongTitleIgnored(t *testing.T) {
	longTitle := "This is a very long commit title that exceeds the sixty character limit for display purposes"
	got := formatCommitEntry("abc123d", longTitle)
	if got != "abc123d" {
		t.Errorf("expected bare SHA 'abc123d', got %q", got)
	}
}

// TestFormatMilestoneEntry verifies that milestone entries are the bare ID.
func TestFormatMilestoneEntry(t *testing.T) {
	got := formatMilestoneEntry(5, "Sprint 1")
	if got != "5" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestFormatJobEntry verifies that job entries are the bare ID.
func TestFormatJobEntry(t *testing.T) {
	got := formatJobEntry(501, "build", "success")
	if got != "501" {
		t.Errorf(fmtUnexpected, got)
	}
}

// TestToResultWithTotal verifies that [toResultWithTotal] propagates the
// upstream total and infers HasMore when total exceeds the cap. Spec
// 2025-11-25: total may exceed values length.
func TestToResultWithTotal(t *testing.T) {
	tests := []struct {
		name        string
		values      []string
		total       int
		wantValues  int
		wantHasMore bool
		wantTotal   int
	}{
		{"unknown total omitted", []string{"a"}, 0, 1, false, 0},
		{"negative total omitted", []string{"a"}, -5, 1, false, 0},
		{"total equal len", []string{"a", "b"}, 2, 2, false, 2},
		{"total exceeds values", []string{"a", "b"}, 50, 2, true, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := toResultWithTotal(tt.values, tt.total)
			if len(r.Completion.Values) != tt.wantValues {
				t.Errorf("values=%d, want %d", len(r.Completion.Values), tt.wantValues)
			}
			if r.Completion.HasMore != tt.wantHasMore {
				t.Errorf("hasMore=%v, want %v", r.Completion.HasMore, tt.wantHasMore)
			}
			if r.Completion.Total != tt.wantTotal {
				t.Errorf("total=%d, want %d", r.Completion.Total, tt.wantTotal)
			}
		})
	}
}

// TestFormatEntries_BareValuesSpec is a guard test asserting that completion
// entry helpers return bare canonical identifiers, never human-readable
// "id: title" labels. MCP spec 2025-11-25 requires `values` in completion
// results to be argument values (the literal that replaces the partial input),
// not labels. Regressing this would silently break chained completions because
// resolved arguments would arrive as e.g. "1234: group/p" instead of
// "group/p", and downstream GitLab API calls would reject the malformed ID.
func TestFormatEntries_BareValuesSpec(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"project", formatProjectEntry(99, "group/proj"), "group/proj"},
		{"group", formatGroupEntry(99, "g/sub"), "g/sub"},
		{"mr", formatMREntry(7, "title"), "7"},
		{"issue", formatIssueEntry(8, "title"), "8"},
		{"pipeline", formatPipelineEntry(9, "main", "ok"), "9"},
		{"commit", formatCommitEntry("abc1234", "title"), "abc1234"},
		{"milestone", formatMilestoneEntry(11, "v1"), "11"},
		{"job", formatJobEntry(12, "build", "ok"), "12"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("%s entry = %q, want bare value %q (spec 2025-11-25 §completion/complete)", c.name, c.got, c.want)
			}
		})
	}
}
