package workitems

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for work item actions exposed through gitlab_issue.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		workItemReadSpec("work_item_get", toolutil.RouteAction(client, Get), "gitlab_get_work_item"),
		workItemReadSpec("work_item_list", toolutil.RouteAction(client, List), "gitlab_list_work_items"),
		workItemCreateSpec("work_item_create", toolutil.RouteAction(client, Create), "gitlab_create_work_item"),
		workItemUpdateSpec("work_item_update", toolutil.RouteAction(client, Update), "gitlab_update_work_item"),
		workItemDeleteSpec("work_item_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_work_item"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("work item #%d from %s", input.IID, input.FullPath))
	return out, nil
}

func workItemReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := workItemOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func workItemCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, workItemOptions(individualTool))
}

func workItemUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := workItemOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func workItemDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := workItemOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func workItemOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"issue", "work_item"},
		OpenWorld:      true,
		OwnerPackage:   "workitems",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
