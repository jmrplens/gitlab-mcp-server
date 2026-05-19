package useremails

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for user email actions exposed through gitlab_user.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		userEmailReadSpec("emails_for_user", toolutil.RouteAction(client, ListForUser), "gitlab_list_emails_for_user"),
		userEmailReadSpec("get_email", toolutil.RouteAction(client, Get), "gitlab_get_email"),
		userEmailCreateSpec("add_email", toolutil.RouteAction(client, Add), "gitlab_add_email"),
		userEmailCreateSpec("add_email_for_user", toolutil.RouteAction(client, AddForUser), "gitlab_add_email_for_user"),
		userEmailDeleteSpec("delete_email", toolutil.DestructiveAction(client, Delete), "gitlab_delete_email"),
		userEmailDeleteSpec("delete_email_for_user", toolutil.DestructiveAction(client, DeleteForUser), "gitlab_delete_email_for_user"),
	}
}

func userEmailReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userEmailOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func userEmailCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, userEmailOptions(individualTool))
}

func userEmailDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userEmailOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func userEmailOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"user", "email"},
		OpenWorld:      true,
		OwnerPackage:   "useremails",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
