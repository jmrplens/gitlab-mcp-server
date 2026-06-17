package systemhooks

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for system hook tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_list_system_hooks — list all instance-wide system hooks.
		systemHookReadSpec("system_hook_list", toolutil.RouteAction(client, List), "gitlab_list_system_hooks"),
		// gitlab_get_system_hook — fetch a single system hook by ID.
		systemHookReadSpec("system_hook_get", toolutil.RouteAction(client, Get), "gitlab_get_system_hook"),
		// gitlab_add_system_hook — register a new system hook with event filters.
		systemHookCreateSpec("system_hook_add", toolutil.RouteAction(client, Add), "gitlab_add_system_hook"),
		// gitlab_edit_system_hook — update an existing system hook.
		systemHookUpdateSpec("system_hook_edit", toolutil.RouteAction(client, Edit), "gitlab_edit_system_hook"),
		// gitlab_test_system_hook — fire a sample event against the hook URL.
		systemHookTestSpec(client),
		// gitlab_set_system_hook_url_variable — set a URL template variable on a system hook.
		systemHookUpdateSpec("system_hook_set_url_variable", toolutil.RouteAction(client, setURLVariableOutput), "gitlab_set_system_hook_url_variable"),
		// gitlab_delete_system_hook_url_variable — delete a URL template variable from a system hook.
		systemHookDeleteSpec("system_hook_delete_url_variable", toolutil.DestructiveAction(client, deleteURLVariableOutput), "gitlab_delete_system_hook_url_variable"),
		// gitlab_delete_system_hook — delete a system hook (destructive).
		systemHookDeleteSpec("system_hook_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_system_hook"),
	}
}

// deleteOutput adapts the void [Delete] handler into the catalog DeleteOutput contract
// so it composes with [toolutil.DestructiveAction] and surfaces a confirmation message.
func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("system hook")
	return out, nil
}

// setURLVariableOutput wraps the void [SetURLVariable] handler into a VoidOutput
// with a confirmation message so it composes with the catalog update route.
func setURLVariableOutput(ctx context.Context, client *gitlabclient.Client, input SetURLVariableInput) (toolutil.VoidOutput, error) {
	if err := SetURLVariable(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: "success", Message: fmt.Sprintf("URL variable %q set on system hook %d", input.Key, input.ID)}, nil
}

// deleteURLVariableOutput wraps the void [DeleteURLVariable] handler into a VoidOutput
// with a confirmation message so it composes with the catalog destructive route.
func deleteURLVariableOutput(ctx context.Context, client *gitlabclient.Client, input DeleteURLVariableInput) (toolutil.VoidOutput, error) {
	if err := DeleteURLVariable(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: "success", Message: fmt.Sprintf("URL variable %q deleted from system hook %d", input.Key, input.ID)}, nil
}

// systemHookReadSpec builds the canonical read-only spec for a system hook tool.
func systemHookReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, systemHookOptions(individualTool))
}

// systemHookCreateSpec builds the canonical create spec for a system hook tool.
func systemHookCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, systemHookOptions(individualTool))
}

// systemHookUpdateSpec builds the canonical update spec for a system hook tool.
func systemHookUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, systemHookOptions(individualTool))
}

// systemHookTestSpec builds the canonical spec for the system hook test event tool.
// The annotations are overridden to read-only and idempotent because the test event
// does not modify state on the GitLab instance.
func systemHookTestSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualReadOnly := true
	individualIdempotent := true
	options := systemHookOptions("gitlab_test_system_hook")
	options.IndividualTool.AnnotationOverrides.ReadOnly = &individualReadOnly
	options.IndividualTool.AnnotationOverrides.Idempotent = &individualIdempotent
	return toolutil.NewCreateActionSpec("system_hook_test", toolutil.RouteAction(client, Test), options)
}

// systemHookDeleteSpec builds the canonical destructive delete spec for a system hook tool.
func systemHookDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, systemHookOptions(individualTool))
}

func systemHookOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute systemhooks domain action.", Tags: []string{"admin", "system-hook"},
		OpenWorld:      true,
		OwnerPackage:   "systemhooks",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
