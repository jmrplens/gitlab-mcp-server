package runnercontrollertokens

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for runner controller token actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		runnerControllerTokenReadSpec("controller_token_list", toolutil.RouteAction(client, List), "gitlab_runner_controller_token_list"),
		runnerControllerTokenReadSpec("controller_token_get", toolutil.RouteAction(client, Get), "gitlab_runner_controller_token_get"),
		runnerControllerTokenCreateSpec("controller_token_create", toolutil.RouteAction(client, Create), "gitlab_runner_controller_token_create"),
		runnerControllerTokenUpdateSpec("controller_token_rotate", toolutil.RouteAction(client, Rotate), "gitlab_runner_controller_token_rotate"),
		runnerControllerTokenDeleteSpec("controller_token_revoke", toolutil.DestructiveAction(client, revokeOutput), "gitlab_runner_controller_token_revoke"),
	}
}

func revokeOutput(ctx context.Context, client *gitlabclient.Client, input RevokeInput) (toolutil.DeleteOutput, error) {
	if err := Revoke(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("runner controller token")
	return out, nil
}

func runnerControllerTokenReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, runnerControllerTokenOptions(individualTool))
}

func runnerControllerTokenCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, runnerControllerTokenOptions(individualTool))
}

func runnerControllerTokenUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, runnerControllerTokenOptions(individualTool))
}

func runnerControllerTokenDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, runnerControllerTokenOptions(individualTool))
}

func runnerControllerTokenOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"runner", "controller", "token"},
		OpenWorld:      true,
		OwnerPackage:   "runnercontrollertokens",
		Edition:        "ultimate",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateRunnerControllerTokenMeta(&options, individualTool)
	return options
}

// decorateRunnerControllerTokenMeta fills non-generic Usage, natural-language
// Aliases, RelatedActions, and the "Returns: … See also: …" individual-tool
// description for runner controller token actions, replacing the generic
// placeholder metadata. Aliases use distinctive "runner controller token"
// phrasing so they do not collide with the accesstokens, impersonationtokens,
// runnercontrollers, or runnercontrollerscopes domains. Falls back to a generic
// Usage only if a tool has no dedicated entry (which would re-flag the audit).
func decorateRunnerControllerTokenMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := runnerControllerTokenActionMeta[individualTool]
	if !ok {
		options.Usage = "Use to execute runnercontrollertokens domain action."
		return
	}
	options.Usage = meta.usage
	options.Aliases = append([]string{individualTool}, meta.aliases...)
	options.RelatedActions = append([]string(nil), meta.related...)
	options.IndividualTool.Description = meta.description
}

// runnerControllerTokenActionMetaEntry is the discovery metadata for one runner
// controller token action.
type runnerControllerTokenActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// runnerControllerTokenActionMeta maps each individual runner controller token
// tool to its discovery metadata. RelatedActions reference the canonical action
// names in this package (controller_token_*). This is an admin-only,
// experimental GitLab API for runner controller authentication tokens.
var runnerControllerTokenActionMeta = map[string]runnerControllerTokenActionMetaEntry{
	"gitlab_runner_controller_token_list": {
		usage:       "List every authentication token issued for a runner controller (admin-only). Use when the prompt asks to audit, enumerate, or review the tokens belonging to a known runner controller before rotating or revoking one.",
		aliases:     []string{"list runner controller tokens", "show tokens for a runner controller", "audit runner controller authentication tokens"},
		related:     []string{"controller_token_get", "controller_token_create", "controller_token_revoke"},
		description: "List runner controller tokens for a controller (admin-only). Returns: each token's id, runner controller id, description, last-used time, and timestamps, with pagination metadata. See also: gitlab_runner_controller_token_get, gitlab_runner_controller_token_create, gitlab_runner_controller_token_revoke.",
	},
	"gitlab_runner_controller_token_get": {
		usage:       "Retrieve a single runner controller token by its id (admin-only). Use after a list result or when the prompt already names a concrete runner controller token id.",
		aliases:     []string{"get runner controller token", "show runner controller token", "fetch a runner controller token"},
		related:     []string{"controller_token_list", "controller_token_rotate", "controller_token_revoke"},
		description: "Get one runner controller token by id (admin-only). Returns: the token's id, runner controller id, description, last-used time, and timestamps. See also: gitlab_runner_controller_token_list, gitlab_runner_controller_token_rotate, gitlab_runner_controller_token_revoke.",
	},
	"gitlab_runner_controller_token_create": {
		usage:       "Create a new authentication token for a runner controller (admin-only). Use when provisioning a runner controller or adding an additional credential; the secret token value is returned only once at creation.",
		aliases:     []string{"create runner controller token", "mint a runner controller token", "issue a new runner controller authentication token"},
		related:     []string{"controller_token_list", "controller_token_rotate", "controller_token_revoke"},
		description: "Create a runner controller token (admin-only). Returns: the new token including its one-time secret value, id, runner controller id, and description. See also: gitlab_runner_controller_token_list, gitlab_runner_controller_token_rotate, gitlab_runner_controller_token_revoke.",
	},
	"gitlab_runner_controller_token_rotate": {
		usage:       "Rotate a runner controller token, invalidating the old secret and issuing a fresh one (admin-only). Use to roll a credential without changing the token id; the new secret value is returned only once.",
		aliases:     []string{"rotate runner controller token", "roll runner controller token secret", "regenerate a runner controller authentication token"},
		related:     []string{"controller_token_get", "controller_token_list", "controller_token_revoke"},
		description: "Rotate a runner controller token (admin-only). Returns: the token with its newly issued one-time secret value and unchanged id. See also: gitlab_runner_controller_token_get, gitlab_runner_controller_token_list, gitlab_runner_controller_token_revoke.",
	},
	"gitlab_runner_controller_token_revoke": {
		usage:       "Permanently revoke a runner controller token (admin-only). Destructive and irreversible; confirm controller_id and token_id before calling, as any runner controller using the token will lose access.",
		aliases:     []string{"revoke runner controller token", "delete runner controller token", "invalidate a runner controller authentication token"},
		related:     []string{"controller_token_list", "controller_token_get", "controller_token_rotate"},
		description: "Revoke a runner controller token permanently (admin-only). Returns: a success confirmation that the runner controller token was deleted. See also: gitlab_runner_controller_token_list, gitlab_runner_controller_token_get, gitlab_runner_controller_token_rotate.",
	},
}
