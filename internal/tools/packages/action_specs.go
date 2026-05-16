package packages

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for Generic Package Registry actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		packageCreateSpec("publish", toolutil.RouteActionWithRequest(client, Publish), "gitlab_package_publish"),
		packageReadSpec("download", toolutil.RouteActionWithRequest(client, Download), "gitlab_package_download"),
		packageReadSpec("list", toolutil.RouteAction(client, List), "gitlab_package_list"),
		packageReadSpec("file_list", toolutil.RouteAction(client, FileList), "gitlab_package_file_list"),
		packageDeleteSpec("delete", toolutil.DestructiveActionWithRequest(client, deleteOutput), "gitlab_package_delete"),
		packageDeleteSpec("file_delete", toolutil.DestructiveActionWithRequest(client, fileDeleteOutput), "gitlab_package_file_delete"),
		packageCreateSpec("publish_and_link", toolutil.RouteActionWithRequest(client, PublishAndLink), "gitlab_package_publish_and_link"),
		packageCreateSpec("publish_directory", toolutil.RouteActionWithRequest(client, PublishDirectory), "gitlab_package_publish_directory"),
	}
}

func deleteOutput(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, req, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("package %s from project %s", input.PackageID, input.ProjectID))
	return out, nil
}

func fileDeleteOutput(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input FileDeleteInput) (toolutil.DeleteOutput, error) {
	if err := FileDelete(ctx, req, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("file %s from package %s in project %s", input.PackageFileID, input.PackageID, input.ProjectID))
	return out, nil
}

func packageReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := packageOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func packageCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, packageOptions(individualTool))
}

func packageDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := packageOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func packageOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"package"},
		OpenWorld:      true,
		OwnerPackage:   "packages",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
