package mrapprovals

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request approval actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		approvalReadSpec("approval_state", toolutil.RouteAction(client, State), "gitlab_mr_approval_state"),
		approvalReadSpec("approval_rules", toolutil.RouteAction(client, Rules), "gitlab_mr_approval_rules"),
		approvalReadSpec("approval_config", toolutil.RouteAction(client, Config), "gitlab_mr_approval_config"),
		approvalDeleteSpec("approval_reset", toolutil.DestructiveVoidAction(client, Reset), "gitlab_mr_approval_reset"),
		approvalCreateSpec("approval_rule_create", toolutil.RouteAction(client, CreateRule), "gitlab_mr_approval_rule_create"),
		approvalUpdateSpec("approval_rule_update", toolutil.RouteAction(client, UpdateRule), "gitlab_mr_approval_rule_update"),
		approvalDeleteSpec("approval_rule_delete", toolutil.DestructiveVoidAction(client, DeleteRule), "gitlab_mr_approval_rule_delete"),
	}
}

func approvalReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := approvalOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func approvalCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, approvalOptions(individualTool))
}

func approvalUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := approvalOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func approvalDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := approvalOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func approvalOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"merge_request", "approval"},
		OpenWorld:      true,
		OwnerPackage:   "mrapprovals",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
