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

// userEmailMeta carries the non-generic discovery metadata (natural-language
// usage, aliases, related actions, and the "Returns: … See also: …" individual
// tool description) for each user email action, mirroring the issues domain.
type userEmailMeta struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// userEmailActionMeta maps each individual tool name to its discovery metadata.
var userEmailActionMeta = map[string]userEmailMeta{
	"gitlab_list_emails_for_user": {
		usage:       "List every email address registered on a specific user's account by user_id. Use after resolving a user with gitlab_get_user; reading another user's emails requires an admin token. Supports offset and keyset pagination plus order_by and sort.",
		aliases:     []string{"list user emails", "show emails for user", "get user email addresses"},
		related:     []string{"user.get_email", "user.add_email_for_user", "user.delete_email_for_user", "user.get"},
		description: "List all email addresses registered to a specific user account. Returns: each email's ID, address, and confirmation timestamp, with offset/keyset pagination support. See also: gitlab_get_email, gitlab_add_email_for_user, gitlab_delete_email_for_user, gitlab_get_user.",
	},
	"gitlab_get_email": {
		usage:       "Fetch a single email address belonging to the authenticated user by email_id. Use after listing emails when the exact email ID is known.",
		aliases:     []string{"get email", "show email address", "fetch email"},
		related:     []string{"user.add_email", "user.delete_email", "user.emails_for_user"},
		description: "Get one email address from the authenticated user's account by ID. Returns: the email's ID, address, and confirmation timestamp. See also: gitlab_add_email, gitlab_delete_email, gitlab_list_emails_for_user.",
	},
	"gitlab_add_email": {
		usage:       "Add a new email address to the authenticated user's own account. The email must be a valid RFC 5322 address that is not already taken; skip_confirmation requires an admin token.",
		aliases:     []string{"add email", "create email address", "register email"},
		related:     []string{"user.get_email", "user.delete_email", "user.add_email_for_user"},
		description: "Add an email address to the authenticated user's account. Returns: the created email's ID, address, and confirmation timestamp. See also: gitlab_get_email, gitlab_delete_email, gitlab_add_email_for_user.",
	},
	"gitlab_add_email_for_user": {
		usage:       "Add a new email address to a specific user's account by user_id (admin only). The email must be a valid RFC 5322 address that is not already taken.",
		aliases:     []string{"add email for user", "create user email address", "register email for user"},
		related:     []string{"user.emails_for_user", "user.delete_email_for_user", "user.add_email"},
		description: "Add an email address to a specific user's account (admin only). Returns: the created email's ID, address, and confirmation timestamp. See also: gitlab_list_emails_for_user, gitlab_delete_email_for_user, gitlab_add_email.",
	},
	"gitlab_delete_email": {
		usage:       "Delete an email address from the authenticated user's own account by email_id. The primary email cannot be deleted.",
		aliases:     []string{"delete email", "remove email address", "unregister email"},
		related:     []string{"user.get_email", "user.add_email", "user.delete_email_for_user"},
		description: "Delete an email address from the authenticated user's account. Returns: a confirmation naming the deleted email ID. See also: gitlab_get_email, gitlab_add_email, gitlab_delete_email_for_user.",
	},
	"gitlab_delete_email_for_user": {
		usage:       "Delete an email address from a specific user's account by user_id and email_id (admin only). The primary email cannot be deleted.",
		aliases:     []string{"delete email for user", "remove user email address", "unregister email for user"},
		related:     []string{"user.emails_for_user", "user.add_email_for_user", "user.delete_email"},
		description: "Delete an email address from a specific user's account (admin only). Returns: a confirmation naming the deleted email ID. See also: gitlab_list_emails_for_user, gitlab_add_email_for_user, gitlab_delete_email.",
	},
}

func userEmailOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute useremails domain action.", Tags: []string{"user", "email"},
		OpenWorld:      true,
		OwnerPackage:   "useremails",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if meta, ok := userEmailActionMeta[individualTool]; ok {
		if meta.usage != "" {
			options.Usage = meta.usage
		}
		if len(meta.aliases) > 0 {
			options.Aliases = meta.aliases
		}
		if len(meta.related) > 0 {
			options.RelatedActions = meta.related
		}
		if meta.description != "" {
			options.IndividualTool.Description = meta.description
		}
	}
	return options
}
