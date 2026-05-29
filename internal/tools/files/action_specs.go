package files

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for repository file actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		fileReadSpec("file_get", fileGetRoute(client), "gitlab_file_get"),
		fileCreateSpec("file_create", toolutil.RouteAction(client, Create), "gitlab_file_create"),
		fileUpdateSpec("file_update", toolutil.RouteAction(client, Update), "gitlab_file_update"),
		fileDeleteSpec("file_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_file_delete"),
		fileReadSpec("file_blame", toolutil.RouteAction(client, Blame), "gitlab_file_blame"),
		fileReadSpec("file_metadata", toolutil.RouteAction(client, GetMetaData), "gitlab_file_metadata"),
		fileReadSpec("file_raw", toolutil.RouteAction(client, GetRaw), "gitlab_file_raw"),
		fileReadSpec("file_raw_metadata", toolutil.RouteAction(client, GetRawFileMetaData), "gitlab_file_raw_metadata"),
	}
}

func fileGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return fileNotFoundOutput{Identifier: fmt.Sprintf("%q in project %v", input["file_path"], input["project_id"])}, nil
		}
		return result, err
	}
	return route
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("file %q from project %s", input.FilePath, input.ProjectID))
	return out, nil
}

func fileReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, fileOptions(individualTool))
}

func fileCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, fileOptions(individualTool))
}

func fileUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, fileOptions(individualTool))
}

func fileDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, fileOptions(individualTool))
}

func fileOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute files domain action.", Tags: []string{"repository", "file"},
		RelatedActions: []string{"repository.tree", "repository.commit_list"},
		OpenWorld:      true,
		OwnerPackage:   "files",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
