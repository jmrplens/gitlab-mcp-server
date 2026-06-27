package deploytokens

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical related-action IDs referenced across deploy-token discovery metadata.
const (
	actionDeployTokenListProject = "access.deploy_token_list_project"
	actionProjectGet             = "project.get"
)

// ActionSpecs returns canonical specs for deploy token actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		deployTokenReadSpec("deploy_token_list_all", toolutil.RouteAction(client, ListAll), "gitlab_deploy_token_list_all"),
		deployTokenReadSpec("deploy_token_list_project", toolutil.RouteAction(client, ListProject), "gitlab_deploy_token_list_project"),
		deployTokenReadSpec("deploy_token_list_group", toolutil.RouteAction(client, ListGroup), "gitlab_deploy_token_list_group"),
		deployTokenReadSpec("deploy_token_get_project", toolutil.RouteAction(client, GetProject), "gitlab_deploy_token_get_project"),
		deployTokenReadSpec("deploy_token_get_group", toolutil.RouteAction(client, GetGroup), "gitlab_deploy_token_get_group"),
		deployTokenCreateSpec("deploy_token_create_project", toolutil.RouteAction(client, CreateProject), "gitlab_deploy_token_create_project"),
		deployTokenCreateSpec("deploy_token_create_group", toolutil.RouteAction(client, CreateGroup), "gitlab_deploy_token_create_group"),
		deployTokenDeleteProjectSpec(client),
		deployTokenDeleteSpec("deploy_token_delete_group", toolutil.DestructiveAction(client, DeleteGroupOutput), "gitlab_deploy_token_delete_group"),
	}
}

// DeleteProjectOutput deletes a project deploy token and returns the canonical success message shape.
func DeleteProjectOutput(ctx context.Context, client *gitlabclient.Client, input DeleteProjectInput) (toolutil.DeleteOutput, error) {
	if err := DeleteProject(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted project deploy token."}, nil
}

// DeleteGroupOutput deletes a group deploy token and returns the canonical success message shape.
func DeleteGroupOutput(ctx context.Context, client *gitlabclient.Client, input DeleteGroupInput) (toolutil.DeleteOutput, error) {
	if err := DeleteGroup(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted group deploy token."}, nil
}

func deployTokenReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, deployTokenOptions(name, individualTool))
}

func deployTokenCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, deployTokenOptions(name, individualTool))
}

func deployTokenDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, deployTokenOptions(name, individualTool))
}

func deployTokenDeleteProjectSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := deployTokenOptions("deploy_token_delete_project", "gitlab_deploy_token_delete_project")
	options.RelatedActions = []string{actionDeployTokenListProject, "access.deploy_token_get_project", "access.deploy_token_create_project"}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole: "scope_owner_project",
			ValueSource:  "Project that owns the deploy token.",
		},
		"deploy_token_id": {
			SemanticRole:     "deploy_token",
			ValueSource:      "Deploy token ID, not a project, deploy key, personal token, or runner ID.",
			CommonConfusions: []string{"Do not use deploy_key_id or token_id for project deploy token deletion."},
		},
	}
	return toolutil.NewDeleteActionSpec("deploy_token_delete_project", toolutil.DestructiveAction(client, DeleteProjectOutput), options)
}

func deployTokenOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "Manage deploy tokens across instance, project, and group scopes."
	relatedActions := []string{"access.deploy_key_list_project", actionProjectGet, "group.get"}
	guidance := map[string]toolutil.ParameterGuidance{}

	switch actionName {
	case "deploy_token_list_project", "deploy_token_get_project", "deploy_token_create_project", "deploy_token_delete_project":
		guidance["project_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "scope_project",
			ValueSource:    "Project ID or path owning the deploy token.",
			ExampleBinding: `params.project_id:"group/project"`,
		}
		relatedActions = []string{actionDeployTokenListProject, actionProjectGet}
	case "deploy_token_list_group", "deploy_token_get_group", "deploy_token_create_group", "deploy_token_delete_group":
		guidance["group_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "scope_group",
			ValueSource:    "Group ID or path owning the deploy token.",
			ExampleBinding: `params.group_id:"my-group"`,
		}
		relatedActions = []string{"access.deploy_token_list_group", "group.get"}
	case "deploy_token_list_all":
		relatedActions = []string{actionDeployTokenListProject, "access.deploy_token_list_group", actionProjectGet}
	}

	if actionName == "deploy_token_get_project" || actionName == "deploy_token_get_group" || actionName == "deploy_token_delete_project" || actionName == "deploy_token_delete_group" {
		guidance["deploy_token_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "deploy_token",
			ValueSource:    "Deploy token ID returned by deploy token list/get actions.",
			ExampleBinding: "params.deploy_token_id:2",
			CommonConfusions: []string{
				"Do not use deploy_key_id; deploy keys are a different access resource.",
			},
		}
	}

	options := toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"access", "deploy_token"},
		Usage:             usage,
		RelatedActions:    relatedActions,
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "deploytokens",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: deployTokenDescription(actionName),
		},
	}

	if actionName == "deploy_token_create_project" || actionName == "deploy_token_create_group" {
		// GitLab deploy tokens take a full ISO 8601 timestamp for expires_at
		// (client-go *time.Time), unlike the date-only canonical default.
		options.InputSchemaOverrides = append(options.InputSchemaOverrides,
			toolutil.SchemaFormatOverride("expires_at", "date-time"))
	}

	decorateDeployTokenMeta(&options, actionName)
	return options
}

// deployTokenActionMetaEntry is the discovery metadata for one deploy-token
// action: an action-specific Usage line and 2-4 distinctive natural-language
// aliases. Deploy-token phrasing is deliberately distinct from the
// accesstokens ("access token") and deploykeys ("deploy key") domains so the
// dynamic find surface does not collide across these neighboring resources.
type deployTokenActionMetaEntry struct {
	usage   string
	aliases []string
}

// deployTokenActionMeta maps each deploy-token action name to its non-generic
// Usage and distinctive aliases (1:1 audit R-META). RelatedActions and the
// "Returns: … See also: …" individual-tool description are set elsewhere
// (deployTokenOptions / deployTokenDescription).
var deployTokenActionMeta = map[string]deployTokenActionMetaEntry{
	"deploy_token_list_all": {
		usage:   "List ALL deploy tokens across the whole GitLab instance in one call (admin only). Use this instead of deploy_token_list_project or deploy_token_list_group when you need every instance-wide token, not just those owned by a single project or group.",
		aliases: []string{"list all deploy tokens", "instance deploy tokens", "audit deploy tokens instance-wide", "enumerate every deploy token"},
	},
	"deploy_token_list_project": {
		usage:   "List the deploy tokens owned by one project. Use this to inventory a project's registry/repository deploy credentials before creating or revoking one.",
		aliases: []string{"list project deploy tokens", "show deploy tokens for project", "project deploy credentials"},
	},
	"deploy_token_list_group": {
		usage:   "List the deploy tokens owned by one group. Use this to inventory a group's shared deploy credentials across its projects.",
		aliases: []string{"list group deploy tokens", "show deploy tokens for group", "group deploy credentials"},
	},
	"deploy_token_get_project": {
		usage:   "Get one project deploy token by its deploy_token_id. Use this after listing to inspect a single token's scopes, expiry, and revoked state.",
		aliases: []string{"get project deploy token", "show project deploy token", "fetch project deploy token by id"},
	},
	"deploy_token_get_group": {
		usage:   "Get one group deploy token by its deploy_token_id. Use this after listing to inspect a single token's scopes, expiry, and revoked state.",
		aliases: []string{"get group deploy token", "show group deploy token", "fetch group deploy token by id"},
	},
	"deploy_token_create_project": {
		usage:   "Create a deploy token for a project so CI or external clients can pull/push the registry or repository. Provide name, scopes, and optional expires_at; the secret token value is returned only once.",
		aliases: []string{"create project deploy token", "issue project deploy token", "generate deploy token for project"},
	},
	"deploy_token_create_group": {
		usage:   "Create a deploy token for a group so its projects share one set of pull/push credentials. Provide name, scopes, and optional expires_at; the secret token value is returned only once.",
		aliases: []string{"create group deploy token", "issue group deploy token", "generate deploy token for group"},
	},
	"deploy_token_delete_project": {
		usage:   "Delete (revoke) a project deploy token by deploy_token_id. Use this to immediately invalidate leaked or unused project pull/push credentials; pass the deploy token ID, not another token type.",
		aliases: []string{"delete project deploy token", "revoke project deploy token", "remove deploy token from project"},
	},
	"deploy_token_delete_group": {
		usage:   "Delete (revoke) a group deploy token by deploy_token_id. Use this to immediately invalidate leaked or unused group pull/push credentials.",
		aliases: []string{"delete group deploy token", "revoke group deploy token", "remove deploy token from group"},
	},
}

// decorateDeployTokenMeta overrides the placeholder Usage and the
// tool-name-only Aliases with action-specific discovery metadata for every
// deploy-token action (1:1 audit R-META). The canonical individual-tool name
// is preserved as the first alias so individual-mode lookups keep working.
func decorateDeployTokenMeta(options *toolutil.ActionSpecOptions, actionName string) {
	meta, ok := deployTokenActionMeta[actionName]
	if !ok {
		return
	}
	options.Usage = meta.usage
	aliases := make([]string, 0, len(options.Aliases)+len(meta.aliases))
	aliases = append(aliases, options.Aliases...)
	aliases = append(aliases, meta.aliases...)
	options.Aliases = aliases
}

// deployTokenDescription returns the "Returns: … See also: …" individual-tool
// description for each deploy-token action (1:1 audit R-META).
func deployTokenDescription(actionName string) string {
	switch actionName {
	case "deploy_token_list_all":
		return "List ALL deploy tokens across the GitLab instance in one call (admin only). Use this instead of gitlab_deploy_token_list_project or gitlab_deploy_token_list_group when you need every instance-wide token. Returns: deploy tokens with id, name, username, scopes, revoked/expired state, and pagination metadata. See also: gitlab_deploy_token_list_project, gitlab_deploy_token_list_group, gitlab_deploy_token_create_project."
	case "deploy_token_list_project":
		return "List deploy tokens owned by a project. Returns: deploy tokens with id, name, username, scopes, revoked/expired state, expiry, and pagination metadata. See also: gitlab_deploy_token_get_project, gitlab_deploy_token_create_project, gitlab_deploy_token_delete_project."
	case "deploy_token_list_group":
		return "List deploy tokens owned by a group. Returns: deploy tokens with id, name, username, scopes, revoked/expired state, expiry, and pagination metadata. See also: gitlab_deploy_token_get_group, gitlab_deploy_token_create_group, gitlab_deploy_token_delete_group."
	case "deploy_token_get_project":
		return "Get a single project deploy token by ID. Returns: the deploy token with id, name, username, scopes, revoked/expired state, and expiry. See also: gitlab_deploy_token_list_project, gitlab_deploy_token_create_project, gitlab_deploy_token_delete_project."
	case "deploy_token_get_group":
		return "Get a single group deploy token by ID. Returns: the deploy token with id, name, username, scopes, revoked/expired state, and expiry. See also: gitlab_deploy_token_list_group, gitlab_deploy_token_create_group, gitlab_deploy_token_delete_group."
	case "deploy_token_create_project":
		return "Create a deploy token for a project. Returns: the created deploy token including its one-time secret token value, plus id, name, username, scopes, and expiry. See also: gitlab_deploy_token_list_project, gitlab_deploy_token_get_project, gitlab_deploy_token_delete_project."
	case "deploy_token_create_group":
		return "Create a deploy token for a group. Returns: the created deploy token including its one-time secret token value, plus id, name, username, scopes, and expiry. See also: gitlab_deploy_token_list_group, gitlab_deploy_token_get_group, gitlab_deploy_token_delete_group."
	case "deploy_token_delete_project":
		return "Permanently delete a project deploy token by ID. Returns: a success confirmation; deletion is irreversible. See also: gitlab_deploy_token_list_project, gitlab_deploy_token_get_project, gitlab_deploy_token_create_project."
	case "deploy_token_delete_group":
		return "Permanently delete a group deploy token by ID. Returns: a success confirmation; deletion is irreversible. See also: gitlab_deploy_token_list_group, gitlab_deploy_token_get_group, gitlab_deploy_token_create_group."
	default:
		return ""
	}
}
