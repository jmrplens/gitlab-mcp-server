package mergerequests

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	pipelineGetAction  = "pipeline.get"
	pipelineWaitAction = "pipeline.wait"
)

// ActionSpecs returns canonical specs for merge request actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewCreateActionSpec("create",
			toolutil.RouteAction(client, Create),
			toolutil.ActionSpecOptions{
				Aliases: []string{"gitlab_mr_create"}, Tags: []string{"merge-request", "branch"},
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
	return toolutil.NewReadActionSpec(name, route, mergeRequestOptions(name, individualTool))
}

func mergeRequestCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, mergeRequestOptions(name, individualTool))
}

func mergeRequestUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, mergeRequestOptions(name, individualTool))
}

func mergeRequestDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, mergeRequestOptions(name, individualTool))
}

func mergeRequestDestructiveUpdateIndividualSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	individualDestructive := false
	options := mergeRequestOptions(name, individualTool)
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func mergeRequestOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute mergerequests domain action.", Tags: []string{"merge_request"},
		OpenWorld:      true,
		OwnerPackage:   "mergerequests",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	switch actionName {
	case "merge":
		options.Usage = "Use to merge a merge request now, or set params.auto_merge=true when the task asks to merge when pipeline succeeds. Do not use " + pipelineWaitAction + " unless the task only asks to wait for an existing pipeline."
		options.Aliases = []string{"merge merge request", "merge mr", "merge when pipeline succeeds"}
		options.RelatedActions = []string{"merge_request.pipelines", pipelineWaitAction, pipelineGetAction, "merge_request.cancel_auto_merge"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"auto_merge": {
				SemanticRole:     "merge_scheduling",
				ValueSource:      "Set true only when the user asks to merge when the pipeline succeeds or enable auto-merge.",
				CommonConfusions: []string{"Do not call " + pipelineWaitAction + " for merge-when-pipeline-succeeds requests; use merge_request.merge with auto_merge=true."},
				ExampleBinding:   "merge !7 when pipeline succeeds => action merge with auto_merge=true.",
			},
			"merge_request_iid": {
				SemanticRole:     "merge_request_iid",
				ValueSource:      "Project-scoped MR IID from merge_request.list or merge_request.get.",
				CommonConfusions: []string{"Do not use pipeline_id; merge_request.merge requires merge_request_iid."},
			},
		}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			{PropertyPath: "auto_merge", Values: map[string]any{"description": "Set true only when the user asks to merge when the pipeline succeeds or enable auto-merge."}},
		}
	case "pipelines":
		options.Usage = "Lists pipelines attached to a merge request; use " + pipelineWaitAction + " with the returned pipeline_id only when the task asks to wait for CI completion."
		options.RelatedActions = []string{pipelineWaitAction, pipelineGetAction, "merge_request.merge", "merge_request.create_pipeline"}
	case "create_pipeline":
		options.Usage = "Creates a new pipeline for a merge request; use " + pipelineWaitAction + " after receiving pipeline_id if the task asks to wait for completion."
		options.RelatedActions = []string{"merge_request.pipelines", pipelineWaitAction, pipelineGetAction}
	case "cancel_auto_merge":
		options.Usage = "Cancels auto-merge on a merge request; it does not cancel a running pipeline."
		options.RelatedActions = []string{"merge_request.merge", "merge_request.get"}
	}
	return options
}
