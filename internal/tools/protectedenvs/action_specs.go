package protectedenvs

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for protected environment actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		protectedEnvironmentReadSpec("protected_list", toolutil.RouteAction(client, List), "gitlab_protected_environment_list"),
		protectedEnvironmentReadSpec("protected_get", toolutil.RouteAction(client, Get), "gitlab_protected_environment_get"),
		protectedEnvironmentCreateSpec("protected_protect", toolutil.RouteAction(client, Protect), "gitlab_protected_environment_protect"),
		protectedEnvironmentUpdateSpec("protected_update", toolutil.RouteAction(client, Update), "gitlab_protected_environment_update"),
		protectedEnvironmentDeleteSpec("protected_unprotect", toolutil.DestructiveAction(client, unprotectOutput), "gitlab_protected_environment_unprotect"),
	}
}

func unprotectOutput(ctx context.Context, client *gitlabclient.Client, input UnprotectInput) (toolutil.DeleteOutput, error) {
	if err := Unprotect(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("protected environment")
	return out, nil
}

func protectedEnvironmentReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, protectedEnvironmentOptions(individualTool))
}

func protectedEnvironmentCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, protectedEnvironmentOptions(individualTool))
}

func protectedEnvironmentUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, protectedEnvironmentOptions(individualTool))
}

func protectedEnvironmentDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, protectedEnvironmentOptions(individualTool))
}

func protectedEnvironmentOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"environment", "protected_environment"},
		Usage:          "Use project protected environment actions for project deployment gates. deploy_access_levels must be an array of objects such as [{\"access_level\":40}]. To require approvals, use approval_rules with required_approvals, not top-level required_approval_count.",
		RelatedActions: []string{"environment.list", "environment.get", "deployment.list"},
		OpenWorld:      true,
		OwnerPackage:   "protectedenvs",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: protectedEnvironmentDescription(individualTool),
		},
	}
}

// protectedEnvironmentDescription returns the "Returns: … See also: …" tool
// description for each protected-environment action (R-META).
func protectedEnvironmentDescription(individualTool string) string {
	switch individualTool {
	case "gitlab_protected_environment_list":
		return "List protected environments in a project with order_by/sort and offset or keyset pagination. Returns: protected environments with their deploy access levels, required approval count, approval rules, and pagination metadata. See also: gitlab_protected_environment_get, gitlab_protected_environment_protect, gitlab_environment_list."
	case "gitlab_protected_environment_get":
		return "Get a single protected (or wildcard) environment by name. Returns: the environment with its deploy access levels (id, access level, user/group, group inheritance) and approval rules. See also: gitlab_protected_environment_list, gitlab_protected_environment_update, gitlab_protected_environment_unprotect."
	case "gitlab_protected_environment_protect":
		return "Protect a single environment or wildcard with deploy access levels and approval rules. Returns: the newly protected environment with its deploy access levels, required approval count, and approval rules. See also: gitlab_protected_environment_get, gitlab_protected_environment_update, gitlab_protected_environment_unprotect."
	case "gitlab_protected_environment_update":
		return "Update a protected environment's deploy access levels and approval rules (use _destroy to remove an entry). Returns: the updated environment with its deploy access levels, required approval count, and approval rules. See also: gitlab_protected_environment_get, gitlab_protected_environment_protect, gitlab_protected_environment_unprotect."
	case "gitlab_protected_environment_unprotect":
		return "Unprotect a single environment or wildcard, removing its deployment gates. Returns: a success confirmation naming the unprotected environment. See also: gitlab_protected_environment_list, gitlab_protected_environment_protect."
	default:
		return ""
	}
}
