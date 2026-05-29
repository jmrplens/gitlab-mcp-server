package runners

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/runnercontrollers"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/runnercontrollerscopes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/runnercontrollertokens"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for runner and runner controller actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	specs := []toolutil.ActionSpec{
		runnerReadSpec("list", toolutil.RouteAction(client, List), "gitlab_runner_list"),
		runnerReadSpec("list_all", toolutil.RouteAction(client, ListAll), "gitlab_runner_list_all"),
		runnerReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_runner_get"),
		runnerUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_runner_update"),
		runnerUpdateSpec("remove", toolutil.DestructiveAction(client, removeOutput), "gitlab_runner_remove"),
		runnerReadSpec("jobs", toolutil.RouteAction(client, ListJobs), "gitlab_runner_jobs"),
		runnerReadSpec("list_project", toolutil.RouteAction(client, ListProject), "gitlab_runner_list_project"),
		runnerCreateSpec("enable_project", toolutil.RouteAction(client, EnableProject), "gitlab_runner_enable_project"),
		runnerUpdateSpec("disable_project", toolutil.DestructiveAction(client, disableProjectOutput), "gitlab_runner_disable_project"),
		runnerReadSpec("list_group", toolutil.RouteAction(client, ListGroup), "gitlab_runner_list_group"),
		runnerCreateSpec("register", toolutil.RouteAction(client, Register), "gitlab_runner_register"),
		runnerUpdateSpec("delete_registered", toolutil.DestructiveAction(client, deleteByIDOutput), "gitlab_runner_delete_registered"),
		runnerUpdateSpec("delete_by_token", toolutil.DestructiveAction(client, deleteByTokenOutput), "gitlab_runner_delete_by_token"),
		runnerReadSpec("verify", toolutil.RouteAction(client, verifyOutput), "gitlab_runner_verify"),
		runnerUpdateSpec("reset_token", toolutil.RouteAction(client, ResetAuthToken), "gitlab_runner_reset_token"),
		runnerUpdateSpec("reset_instance_reg_token", toolutil.RouteAction(client, ResetInstanceRegToken), "gitlab_runner_reset_instance_reg_token"),
		runnerUpdateSpec("reset_group_reg_token", toolutil.RouteAction(client, ResetGroupRegToken), "gitlab_runner_reset_group_reg_token"),
		runnerUpdateSpec("reset_project_reg_token", toolutil.RouteAction(client, ResetProjectRegToken), "gitlab_runner_reset_project_reg_token"),
		runnerReadSpec("list_managers", toolutil.RouteAction(client, ListManagers), "gitlab_runner_list_managers"),
	}
	specs = append(specs, runnercontrollers.ActionSpecs(client)...)
	specs = append(specs, runnercontrollerscopes.ActionSpecs(client)...)
	specs = append(specs, runnercontrollertokens.ActionSpecs(client)...)
	return specs
}

func removeOutput(ctx context.Context, client *gitlabclient.Client, input RemoveInput) (toolutil.DeleteOutput, error) {
	if err := Remove(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("runner")
	return out, nil
}

func disableProjectOutput(ctx context.Context, client *gitlabclient.Client, input DisableProjectInput) (toolutil.DeleteOutput, error) {
	if err := DisableProject(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("project runner assignment")
	return out, nil
}

func deleteByIDOutput(ctx context.Context, client *gitlabclient.Client, input DeleteByIDInput) (toolutil.DeleteOutput, error) {
	if err := DeleteByID(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("registered runner")
	return out, nil
}

func deleteByTokenOutput(ctx context.Context, client *gitlabclient.Client, input DeleteByTokenInput) (toolutil.DeleteOutput, error) {
	if err := DeleteByToken(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("registered runner")
	return out, nil
}

func verifyOutput(ctx context.Context, client *gitlabclient.Client, input VerifyInput) (toolutil.DeleteOutput, error) {
	if err := Verify(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Runner token is valid."}, nil
}

func runnerReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, runnerOptions(name, individualTool))
}

func runnerCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, runnerOptions(name, individualTool))
}

func runnerUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, runnerOptions(name, individualTool))
}

func runnerOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "Use to execute runners domain action."
	if actionName == "remove" {
		usage = "Remove (unregister) a runner by its numeric runner_id. Use runner.list or runner.list_project to obtain the runner_id first."
	}
	return toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Usage:             usage,
		Tags:              []string{"runner"},
		ParameterGuidance: runnerParameterGuidance(actionName),
		OpenWorld:         true,
		OwnerPackage:      "runners",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

func runnerParameterGuidance(actionName string) map[string]toolutil.ParameterGuidance {
	guidance := make(map[string]toolutil.ParameterGuidance)
	if runnerActionUsesRunnerID(actionName) {
		guidance["runner_id"] = toolutil.ParameterGuidance{
			SemanticRole: "runner_identifier",
			ValueSource:  "Use gitlab_runner_list, gitlab_runner_list_project, gitlab_runner_list_group, or gitlab_runner_get; runner_id is the global runner ID.",
			CommonConfusions: []string{
				"Do not pass project_id as runner_id.",
				"For project assignment actions, project_id identifies the project scope while runner_id identifies the runner.",
			},
		}
	}
	if runnerActionUsesProjectID(actionName) {
		guidance["project_id"] = toolutil.ParameterGuidance{
			SemanticRole: "scope_owner_project",
			ValueSource:  "Use gitlab_project get/list outputs; accepts numeric ID or namespace/project path.",
			CommonConfusions: []string{
				"project_id identifies the project scope, not the runner.",
				"Use runner_id for the runner to enable, disable, reset, or inspect.",
			},
		}
	}
	if runnerActionUsesGroupID(actionName) {
		guidance["group_id"] = toolutil.ParameterGuidance{
			SemanticRole: "scope_owner_group",
			ValueSource:  "Use gitlab_group get/list outputs; accepts numeric ID or full group path.",
		}
	}
	if len(guidance) == 0 {
		return nil
	}
	return guidance
}

func runnerActionUsesRunnerID(actionName string) bool {
	switch actionName {
	case "get", "update", "remove", "jobs", "enable_project", "disable_project", "delete_registered", "reset_token":
		return true
	default:
		return false
	}
}

func runnerActionUsesProjectID(actionName string) bool {
	switch actionName {
	case "list_project", "enable_project", "disable_project", "reset_project_reg_token":
		return true
	default:
		return false
	}
}

func runnerActionUsesGroupID(actionName string) bool {
	switch actionName {
	case "list_group", "reset_group_reg_token":
		return true
	default:
		return false
	}
}
