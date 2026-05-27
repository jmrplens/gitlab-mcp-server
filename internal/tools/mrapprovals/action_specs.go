package mrapprovals

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request approval actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		approvalReadSpec("approval_state", toolutil.RouteAction(client, State), "gitlab_mr_approval_state"),
		approvalReadSpec("approval_rules", toolutil.RouteAction(client, Rules), "gitlab_mr_approval_rules"),
		approvalReadSpec("approval_config", toolutil.RouteAction(client, Config), "gitlab_mr_approval_config"),
		approvalResetSpec(client),
		approvalCreateSpec("approval_rule_create", toolutil.RouteAction(client, CreateRule), "gitlab_mr_approval_rule_create"),
		approvalUpdateSpec("approval_rule_update", toolutil.RouteAction(client, UpdateRule), "gitlab_mr_approval_rule_update"),
		approvalDeleteSpec("approval_rule_delete", toolutil.DestructiveAction(client, deleteRuleOutput), "gitlab_mr_approval_rule_delete"),
	}
}

func resetOutput(ctx context.Context, client *gitlabclient.Client, input ResetInput) (toolutil.DeleteOutput, error) {
	if err := Reset(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("MR approvals")
	return out, nil
}

func deleteRuleOutput(ctx context.Context, client *gitlabclient.Client, input DeleteRuleInput) (toolutil.DeleteOutput, error) {
	if err := DeleteRule(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("approval rule")
	return out, nil
}

func approvalReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, approvalOptions(individualTool))
}

func approvalCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, approvalOptions(individualTool))
}

func approvalUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, approvalOptions(individualTool))
}

func approvalDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, approvalOptions(individualTool))
}

func approvalResetSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualDestructive := false
	options := approvalOptions("gitlab_mr_approval_reset")
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewDeleteActionSpec("approval_reset", toolutil.DestructiveAction(client, resetOutput), options)
}

func approvalOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute mrapprovals domain action.", Tags: []string{"merge_request", "approval"},
		OpenWorld:      true,
		OwnerPackage:   "mrapprovals",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
