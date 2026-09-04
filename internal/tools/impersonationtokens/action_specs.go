package impersonationtokens

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionImpersonationTokenRevoke = "impersonationtokens.revoke_impersonation_token"
	actionImpersonationTokenList   = "impersonationtokens.list_impersonation_tokens"
)

// ActionSpecs returns canonical specs for impersonation and user PAT actions exposed through gitlab_user.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		userTokenReadSpec("list_impersonation_tokens", toolutil.RouteAction(client, List), "gitlab_list_impersonation_tokens"),
		userTokenReadSpec("get_impersonation_token", toolutil.RouteAction(client, Get), "gitlab_get_impersonation_token"),
		userTokenCreateSpec("create_impersonation_token", toolutil.RouteAction(client, Create), "gitlab_create_impersonation_token"),
		userTokenDeleteSpec("revoke_impersonation_token", toolutil.DestructiveAction(client, Revoke), "gitlab_revoke_impersonation_token"),
		userTokenCreateSpec("create_personal_access_token", toolutil.RouteAction(client, CreatePAT), "gitlab_create_personal_access_token"),
	}
}

func userTokenReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, userTokenOptions(individualTool))
}

func userTokenCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, userTokenOptions(individualTool))
}

func userTokenDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, userTokenOptions(individualTool))
}

func userTokenOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute impersonationtokens domain action.", Tags: []string{"user", "token"},
		OpenWorld:      true,
		OwnerPackage:   "impersonationtokens",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if meta, ok := userTokenActionMeta[individualTool]; ok {
		if meta.usage != "" {
			options.Usage = meta.usage
		}
		if len(meta.aliases) > 0 {
			options.Aliases = append([]string(nil), meta.aliases...)
		}
		if len(meta.related) > 0 {
			options.RelatedActions = append([]string(nil), meta.related...)
		}
		if meta.description != "" {
			options.IndividualTool.Description = meta.description
		}
	}
	if individualTool == "gitlab_list_impersonation_tokens" {
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("state", "all", "active", "inactive"),
		}
	}
	return options
}

// userTokenActionMetaEntry is the discovery metadata for one user-token action.
type userTokenActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// userTokenActionMeta maps each individual user-token tool to non-generic
// discovery metadata, including the "Returns: … See also: …" description form
// (R-META; 1:1 audit).
var userTokenActionMeta = map[string]userTokenActionMetaEntry{
	"gitlab_list_impersonation_tokens": {
		usage:       "List all impersonation tokens for a user (admin token required). Use to audit a user's impersonation tokens, optionally filtering by state and paginating with offset or keyset pagination.",
		aliases:     []string{"list impersonation tokens", "show user impersonation tokens", "audit impersonation tokens"},
		related:     []string{"impersonationtokens.get_impersonation_token", "impersonationtokens.create_impersonation_token", actionImpersonationTokenRevoke},
		description: "List all impersonation tokens for a user. Returns: each token's id, name, active flag, scopes, revoked flag, created_at, expires_at, and last_used_at. See also: gitlab_get_impersonation_token, gitlab_create_impersonation_token, gitlab_revoke_impersonation_token.",
	},
	"gitlab_get_impersonation_token": {
		usage:       "Retrieve one impersonation token for a user by token id (admin token required). Use after listing to inspect a single token's state and scopes.",
		aliases:     []string{"get impersonation token", "show impersonation token", "fetch impersonation token"},
		related:     []string{actionImpersonationTokenList, actionImpersonationTokenRevoke},
		description: "Get a single impersonation token for a user by token id. Returns: the token's id, name, active flag, scopes, revoked flag, created_at, expires_at, and last_used_at. See also: gitlab_list_impersonation_tokens, gitlab_revoke_impersonation_token.",
	},
	"gitlab_create_impersonation_token": {
		usage:       "Create an impersonation token for a user (admin token required). Use to mint a token with explicit scopes and an optional expiry that can act on behalf of the target user.",
		aliases:     []string{"create impersonation token", "mint impersonation token", "add impersonation token"},
		related:     []string{actionImpersonationTokenList, actionImpersonationTokenRevoke, "impersonationtokens.create_personal_access_token"},
		description: "Create an impersonation token for a user with the given scopes and optional expiry. Returns: the new token's id, name, active flag, scopes, revoked flag, created_at, expires_at, and the secret token value (shown once). See also: gitlab_list_impersonation_tokens, gitlab_revoke_impersonation_token, gitlab_create_personal_access_token.",
	},
	"gitlab_revoke_impersonation_token": {
		usage:       "Revoke (delete) an impersonation token for a user by token id (admin token required, destructive). Use to invalidate a token that should no longer be usable.",
		aliases:     []string{"revoke impersonation token", "delete impersonation token", "invalidate impersonation token"},
		related:     []string{actionImpersonationTokenList, "impersonationtokens.get_impersonation_token"},
		description: "Revoke an impersonation token for a user by token id. Returns: a confirmation naming the user_id and token_id with the revoked flag set. See also: gitlab_list_impersonation_tokens, gitlab_get_impersonation_token.",
	},
	"gitlab_create_personal_access_token": {
		usage:       "Create a personal access token for another user (admin token required). Use to provision a PAT with explicit scopes, an optional description, and an optional expiry on behalf of the target user.",
		aliases:     []string{"create personal access token for user", "create user PAT", "mint user personal access token"},
		related:     []string{"impersonationtokens.create_impersonation_token", actionImpersonationTokenList},
		description: "Create a personal access token for a user with the given scopes, optional description, and optional expiry. Returns: the new token's id, name, active flag, scopes, revoked flag, description, user_id, created_at, expires_at, and the secret token value (shown once). See also: gitlab_create_impersonation_token, gitlab_list_impersonation_tokens.",
	},
}
