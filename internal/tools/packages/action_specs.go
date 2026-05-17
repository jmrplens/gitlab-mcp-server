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
	options := packageOptions(name, individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func packageCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, packageOptions(name, individualTool))
}

func packageDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := packageOptions(name, individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func packageOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Tags:           []string{"package"},
		OpenWorld:      true,
		OwnerPackage:   "packages",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if actionName == "publish_directory" {
		options.Usage = "Publish all regular files from a local directory to Generic Packages. Omit include_pattern to upload every file; include_pattern is one glob, not a comma-separated file list."
		options.Aliases = []string{"publish local directory", "upload package directory", "generic package directory upload", "publish multiple package files"}
		options.Tags = append(options.Tags, "generic_package", "directory_upload")
		options.RelatedActions = []string{"release.create", "release.link_create_batch", "package.publish"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"include_pattern": {
				SemanticRole: "single_glob_filter",
				ValueSource:  "Optional single glob matched against file names inside directory_path; omit it to include all regular files.",
				CommonConfusions: []string{
					"Do not pass comma-separated filenames.",
					"Do not use include_pattern to enumerate exact files; omit it when all fixture files should be uploaded.",
				},
			},
		}
	}
	return options
}
