package protectedenvs

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	options := protectedEnvironmentOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func protectedEnvironmentCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, protectedEnvironmentOptions(individualTool))
}

func protectedEnvironmentUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := protectedEnvironmentOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func protectedEnvironmentDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := protectedEnvironmentOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func protectedEnvironmentOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"environment", "protected_environment"},
		RelatedActions: []string{"environment.list", "environment.get", "deployment.list"},
		OpenWorld:      true,
		OwnerPackage:   "protectedenvs",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
