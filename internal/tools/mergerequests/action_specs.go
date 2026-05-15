package mergerequests

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewActionSpec("create",
			toolutil.RouteAction(client, Create),
			toolutil.ActionSpecOptions{
				Tags:           []string{"merge-request", "branch"},
				Usage:          "Use to open a merge request from a source branch into the target branch in a project.",
				RelatedActions: []string{"merge_request.get", "merge_request.list", "branch.create", "project.get"},
				ParameterGuidance: map[string]toolutil.ParameterGuidance{
					"source_branch": {
						SemanticRole:     "source_branch",
						ValueSource:      "Branch named after 'from'.",
						CommonConfusions: []string{"Do not use ref, tag_name, target_branch, or value for the source branch."},
						ExampleBinding:   "from feature/eval into main => source_branch=feature/eval.",
					},
					"target_branch": {
						SemanticRole:     "target_branch",
						ValueSource:      "Branch named after 'into' or the merge target.",
						CommonConfusions: []string{"Do not use source_branch, ref, tag_name, or to for the target branch."},
						ExampleBinding:   "from feature/eval into main => target_branch=main.",
					},
				},
				OpenWorld:      true,
				OwnerPackage:   "mergerequests",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_mr_create", Title: toolutil.TitleFromName("gitlab_mr_create")},
			}),
		mergeRequestReadSpec("get", mergeRequestGetRoute(client), "gitlab_mr_get"),
		mergeRequestReadSpec("list", toolutil.RouteAction(client, List), "gitlab_mr_list"),
		mergeRequestReadSpec("list_global", toolutil.RouteAction(client, ListGlobal), "gitlab_mr_list_global"),
		mergeRequestReadSpec("list_group", toolutil.RouteAction(client, ListGroup), "gitlab_mr_list_group"),
		mergeRequestUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_mr_update"),
		mergeRequestDestructiveUpdateIndividualSpec("merge", toolutil.DestructiveAction(client, Merge), "gitlab_mr_merge"),
		mergeRequestUpdateSpec("approve", toolutil.RouteAction(client, Approve), "gitlab_mr_approve"),
		mergeRequestDestructiveUpdateIndividualSpec("unapprove", toolutil.RouteAction(client, UnapproveOutput), "gitlab_mr_unapprove"),
		mergeRequestReadSpec("commits", toolutil.RouteAction(client, Commits), "gitlab_mr_commits"),
		mergeRequestReadSpec("pipelines", toolutil.RouteAction(client, Pipelines), "gitlab_mr_pipelines"),
		mergeRequestDeleteSpec("delete", toolutil.RouteAction(client, DeleteOutput), "gitlab_mr_delete"),
		mergeRequestUpdateSpec("rebase", toolutil.RouteAction(client, Rebase), "gitlab_mr_rebase"),
		mergeRequestReadSpec("participants", toolutil.RouteAction(client, Participants), "gitlab_mr_participants"),
		mergeRequestReadSpec("reviewers", toolutil.RouteAction(client, Reviewers), "gitlab_mr_reviewers"),
		mergeRequestCreateSpec("create_pipeline", toolutil.RouteAction(client, CreatePipeline), "gitlab_mr_create_pipeline"),
		mergeRequestReadSpec("issues_closed", toolutil.RouteAction(client, IssuesClosed), "gitlab_mr_issues_closed"),
		mergeRequestUpdateSpec("cancel_auto_merge", toolutil.RouteAction(client, CancelAutoMerge), "gitlab_mr_cancel_auto_merge"),
		mergeRequestUpdateSpec("subscribe", toolutil.RouteAction(client, Subscribe), "gitlab_mr_subscribe"),
		mergeRequestUpdateSpec("unsubscribe", toolutil.RouteAction(client, Unsubscribe), "gitlab_mr_unsubscribe"),
		mergeRequestUpdateSpec("time_estimate_set", toolutil.RouteAction(client, SetTimeEstimate), "gitlab_mr_set_time_estimate"),
		mergeRequestUpdateSpec("time_estimate_reset", toolutil.RouteAction(client, ResetTimeEstimate), "gitlab_mr_reset_time_estimate"),
		mergeRequestUpdateSpec("spent_time_add", toolutil.RouteAction(client, AddSpentTime), "gitlab_mr_add_spent_time"),
		mergeRequestUpdateSpec("spent_time_reset", toolutil.RouteAction(client, ResetSpentTime), "gitlab_mr_reset_spent_time"),
		mergeRequestReadSpec("time_stats", toolutil.RouteAction(client, GetTimeStats), "gitlab_mr_time_stats"),
		mergeRequestReadSpec("related_issues", toolutil.RouteAction(client, RelatedIssues), "gitlab_mr_related_issues"),
		mergeRequestCreateSpec("create_todo", toolutil.RouteAction(client, CreateTodo), "gitlab_mr_create_todo"),
		mergeRequestCreateSpec("dependency_create", toolutil.RouteAction(client, CreateDependency), "gitlab_mr_dependency_create"),
		mergeRequestDeleteSpec("dependency_delete", toolutil.RouteAction(client, DeleteDependencyOutput), "gitlab_mr_dependency_delete"),
		mergeRequestReadSpec("dependencies_list", toolutil.RouteAction(client, GetDependencies), "gitlab_mr_dependencies_list"),
	}
}

func mergeRequestGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return mergeRequestNotFoundOutput{Identifier: fmt.Sprintf("!%v in project %v", input["merge_request_iid"], input["project_id"])}, nil
		}
		return result, err
	}
	return route
}

// UnapproveOutput removes approval from a merge request and returns the legacy success message shape.
func UnapproveOutput(ctx context.Context, client *gitlabclient.Client, input ApproveInput) (toolutil.DeleteOutput, error) {
	if err := Unapprove(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted approval from MR !%d in project %s.", input.MRIID, input.ProjectID)}, nil
}

// DeleteOutput deletes a merge request and returns the legacy success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted MR !%d from project %s.", input.MRIID, input.ProjectID)}, nil
}

// DeleteDependencyOutput removes a merge request dependency and returns the legacy success message shape.
func DeleteDependencyOutput(ctx context.Context, client *gitlabclient.Client, input DeleteDependencyInput) (toolutil.DeleteOutput, error) {
	if err := DeleteDependency(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted dependency on blocking MR %d from MR !%d in project %s.", input.BlockingMergeRequestID, input.MRIID, input.ProjectID)}, nil
}

func mergeRequestReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mergeRequestOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func mergeRequestCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, mergeRequestOptions(individualTool))
}

func mergeRequestUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mergeRequestOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func mergeRequestDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mergeRequestOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func mergeRequestDestructiveUpdateIndividualSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	individualDestructive := false
	options := mergeRequestOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewActionSpec(name, route, options)
}

func mergeRequestOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"merge_request"},
		OpenWorld:      true,
		OwnerPackage:   "mergerequests",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
