package uploads

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project upload actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		uploadCreateSpec("upload", toolutil.RouteActionWithRequest(client, Upload), "gitlab_project_upload"),
		uploadReadSpec("upload_list", toolutil.RouteAction(client, List), "gitlab_project_upload_list"),
		uploadDeleteSpec("upload_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_project_upload_delete"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("upload %d from project %s", input.UploadID, input.ProjectID))
	return out, nil
}

func uploadReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, uploadOptions(individualTool))
}

func uploadCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, uploadOptions(individualTool))
}

func uploadDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, uploadOptions(individualTool))
}

func uploadOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute uploads domain action.", Tags: []string{"project", "upload"},
		RelatedActions: []string{"project.upload_list", "project.get"},
		OpenWorld:      true,
		OwnerPackage:   "uploads",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
