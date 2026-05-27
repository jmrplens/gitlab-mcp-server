package projectmirrors

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project remote mirror actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		mirrorReadSpec("mirror_list", toolutil.RouteAction(client, List), "gitlab_list_project_mirrors"),
		mirrorReadSpec("mirror_get", toolutil.RouteAction(client, Get), "gitlab_get_project_mirror"),
		mirrorReadSpec("mirror_get_public_key", toolutil.RouteAction(client, GetPublicKey), "gitlab_get_project_mirror_public_key"),
		mirrorCreateSpec("mirror_add", toolutil.RouteAction(client, Add), "gitlab_add_project_mirror"),
		mirrorUpdateSpec("mirror_edit", toolutil.RouteAction(client, Edit), "gitlab_edit_project_mirror"),
		mirrorDeleteSpec("mirror_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_project_mirror"),
		mirrorForcePushSpec(client),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("mirror %d from project %s", input.MirrorID, input.ProjectID))
	return out, nil
}

func forcePushOutput(ctx context.Context, client *gitlabclient.Client, input ForcePushInput) (toolutil.DeleteOutput, error) {
	if err := ForcePushUpdate(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Message: fmt.Sprintf("Force push update triggered for mirror %d in project %s", input.MirrorID, input.ProjectID)}, nil
}

func mirrorReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, mirrorOptions(individualTool))
}

func mirrorCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, mirrorOptions(individualTool))
}

func mirrorUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, mirrorOptions(individualTool))
}

func mirrorDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, mirrorOptions(individualTool))
}

func mirrorForcePushSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualDestructive := false
	options := mirrorOptions("gitlab_force_push_mirror_update")
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewDeleteActionSpec("mirror_force_push", toolutil.DestructiveAction(client, forcePushOutput), options)
}

func mirrorOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute projectmirrors domain action.", Tags: []string{"project", "mirror"},
		RelatedActions: []string{"project.pull_mirror_get", "repository.commit_list"},
		OpenWorld:      true,
		OwnerPackage:   "projectmirrors",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
