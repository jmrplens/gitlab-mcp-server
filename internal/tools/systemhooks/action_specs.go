package systemhooks

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for system hook tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		systemHookReadSpec("system_hook_list", toolutil.RouteAction(client, List), "gitlab_list_system_hooks"),
		systemHookReadSpec("system_hook_get", toolutil.RouteAction(client, Get), "gitlab_get_system_hook"),
		systemHookCreateSpec("system_hook_add", toolutil.RouteAction(client, Add), "gitlab_add_system_hook"),
		systemHookTestSpec(client),
		systemHookDeleteSpec("system_hook_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_system_hook"),
	}
}

func systemHookReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := systemHookOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func systemHookCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, systemHookOptions(individualTool))
}

func systemHookTestSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualReadOnly := true
	individualIdempotent := true
	options := systemHookOptions("gitlab_test_system_hook")
	options.IndividualTool.AnnotationOverrides.ReadOnly = &individualReadOnly
	options.IndividualTool.AnnotationOverrides.Idempotent = &individualIdempotent
	return toolutil.NewActionSpec("system_hook_test", toolutil.RouteAction(client, Test), options)
}

func systemHookDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := systemHookOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func systemHookOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin", "system-hook"},
		OpenWorld:      true,
		OwnerPackage:   "systemhooks",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
