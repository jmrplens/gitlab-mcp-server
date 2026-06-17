package useremails

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for user email actions exposed through gitlab_user.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_list_emails_for_user — list the email addresses on a user's account.
		userEmailReadSpec("emails_for_user", toolutil.RouteAction(client, ListForUser), "gitlab_list_emails_for_user"),
		// gitlab_get_email — fetch a single email address by ID.
		userEmailReadSpec("get_email", toolutil.RouteAction(client, Get), "gitlab_get_email"),
		// gitlab_add_email — add an email address to the current user's account.
		userEmailCreateSpec("add_email", toolutil.RouteAction(client, Add), "gitlab_add_email"),
		// gitlab_add_email_for_user — add an email to a specific user's account (admin-only).
		userEmailCreateSpec("add_email_for_user", toolutil.RouteAction(client, AddForUser), "gitlab_add_email_for_user"),
		// gitlab_delete_email — delete an email address from the current user's account.
		userEmailDeleteSpec("delete_email", toolutil.DestructiveAction(client, Delete), "gitlab_delete_email"),
		// gitlab_delete_email_for_user — delete an email from a specific user's account (admin-only).
		userEmailDeleteSpec("delete_email_for_user", toolutil.DestructiveAction(client, DeleteForUser), "gitlab_delete_email_for_user"),
	}
}

// userEmailReadSpec builds the canonical read-only spec for a user email tool.
func userEmailReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, userEmailOptions(individualTool))
}

// userEmailCreateSpec builds the canonical create spec for a user email tool.
func userEmailCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, userEmailOptions(individualTool))
}

// userEmailDeleteSpec builds the canonical destructive delete spec for a user email tool.
func userEmailDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, userEmailOptions(individualTool))
}

func userEmailOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute useremails domain action.", Tags: []string{"user", "email"},
		OpenWorld:      true,
		OwnerPackage:   "useremails",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
