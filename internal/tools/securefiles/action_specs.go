package securefiles

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for secure file tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_list_secure_files — list secure files for a project with pagination.
		secureFileReadSpec("secure_file_list", toolutil.RouteAction(client, List), "gitlab_list_secure_files"),
		// gitlab_show_secure_file — fetch a single secure file by numeric ID.
		secureFileReadSpec("secure_file_get", toolutil.RouteAction(client, Show), "gitlab_show_secure_file"),
		// gitlab_create_secure_file — upload a new secure file (base64 or local file path).
		secureFileCreateSpec("secure_file_create", toolutil.RouteAction(client, Create), "gitlab_create_secure_file"),
		// gitlab_remove_secure_file — delete a secure file by numeric ID (destructive).
		secureFileDeleteSpec("secure_file_delete", toolutil.DestructiveAction(client, removeOutput), "gitlab_remove_secure_file"),
	}
}

// removeOutput adapts the void [Remove] handler into the catalog DeleteOutput contract
// so it composes with [toolutil.DestructiveAction] and surfaces a confirmation message.
func removeOutput(ctx context.Context, client *gitlabclient.Client, input RemoveInput) (toolutil.DeleteOutput, error) {
	if err := Remove(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("secure file")
	return out, nil
}

// secureFileReadSpec builds the canonical read-only spec for a secure file tool.
func secureFileReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, secureFileOptions(individualTool))
}

// secureFileCreateSpec builds the canonical create spec for a secure file tool.
func secureFileCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, secureFileOptions(individualTool))
}

// secureFileDeleteSpec builds the canonical destructive delete spec for a secure file tool.
func secureFileDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, secureFileOptions(individualTool))
}

// secureFileOptions returns the shared [toolutil.ActionSpecOptions] for secure file tools,
// applying the package's tags, ownership, and individual-tool metadata defaults.
func secureFileOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute securefiles domain action.", Tags: []string{"secure-file"},
		OpenWorld:      true,
		OwnerPackage:   "securefiles",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
