package securefiles

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for secure file tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		secureFileReadSpec("secure_file_list", toolutil.RouteAction(client, List), "gitlab_list_secure_files"),
		secureFileReadSpec("secure_file_get", toolutil.RouteAction(client, Show), "gitlab_show_secure_file"),
		secureFileCreateSpec("secure_file_create", toolutil.RouteAction(client, Create), "gitlab_create_secure_file"),
		secureFileDeleteSpec("secure_file_delete", toolutil.DestructiveAction(client, removeOutput), "gitlab_remove_secure_file"),
	}
}

func removeOutput(ctx context.Context, client *gitlabclient.Client, input RemoveInput) (toolutil.DeleteOutput, error) {
	if err := Remove(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("secure file")
	return out, nil
}

func secureFileReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := secureFileOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func secureFileCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, secureFileOptions(individualTool))
}

func secureFileDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := secureFileOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func secureFileOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"secure-file"},
		OpenWorld:      true,
		OwnerPackage:   "securefiles",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
