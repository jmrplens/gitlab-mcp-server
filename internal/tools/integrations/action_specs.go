package integrations

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project integration actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		integrationReadSpec("integration_list", toolutil.RouteAction(client, List), "gitlab_list_integrations",
			"List all integrations (services) configured for a project, including their active status.\n\nReturns: JSON array of integration items with id, title, slug, and active flag.\n\nSee also: gitlab_get_integration, gitlab_set_jira_integration, gitlab_delete_integration"),
		integrationReadSpec("integration_get", toolutil.RouteAction(client, Get), "gitlab_get_integration",
			"Get details of a specific project integration by slug (e.g. jira, slack, discord, mattermost, microsoft-teams, telegram, datadog, jenkins, emails-on-push, pipelines-email, external-wiki, custom-issue-tracker, drone-ci, github, harbor, matrix, redmine, youtrack, slack-slash-commands, mattermost-slash-commands).\n\nReturns: JSON with integration details.\n\nSee also: gitlab_list_integrations, gitlab_delete_integration"),
		integrationDeleteSpec("integration_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_integration",
			"Delete (disable) a project integration by slug. Supports the same slugs as get, plus 'slack-application' for disabling the GitLab for Slack app.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_integrations, gitlab_get_integration"),
		integrationCreateSpec("integration_set_jira", toolutil.RouteAction(client, SetJira), "gitlab_set_jira_integration",
			"Configure the Jira integration for a project. Sets up the connection to a Jira instance with URL, credentials, and event triggers.\n\nReturns: JSON with the configured Jira integration details.\n\nSee also: gitlab_list_integrations, gitlab_get_integration"),
		integrationCreateSpec("integration_set", toolutil.RouteAction(client, SetIntegration), "gitlab_set_integration",
			"Configure (create or update) any project integration by slug, passing the integration's documented parameters in a free-form `config` object. Use this generic action for integrations without a dedicated tool (e.g. slack, harbor, jenkins, google-play, apple-app-store, prometheus, microsoft-teams, mattermost, discord).\n\nCommon slugs: "+commonIntegrationSlugs+".\n\nThe `config` object is sent verbatim as the request body; see doc/api/integrations.md for the exact fields each integration accepts (for example slack expects `webhook`; harbor expects `url`, `project_name`, `username`, `password`).\n\nReturns: JSON with the resulting integration details.\n\nSee also: gitlab_get_integration, gitlab_list_integrations, gitlab_delete_integration, gitlab_set_jira_integration"),

		// Generic group-level integration actions (group integration API mirrors the project one).
		groupIntegrationReadSpec("integration_list_group", toolutil.RouteAction(client, ListGroupIntegrations), "gitlab_list_group_integrations",
			"List all active integrations configured on a group. Requires Owner role (some integrations require GitLab Premium/Ultimate).\n\nReturns: JSON array of group integration items with id, title, slug, and active flag.\n\nSee also: gitlab_get_group_integration, gitlab_set_group_integration, gitlab_delete_group_integration"),
		groupIntegrationReadSpec("integration_get_group", toolutil.RouteAction(client, GetGroupIntegration), "gitlab_get_group_integration",
			"Get details of a specific group integration by slug. Requires Owner role on the group.\n\nReturns: JSON with the group integration details.\n\nSee also: gitlab_list_group_integrations, gitlab_set_group_integration, gitlab_delete_group_integration"),
		groupIntegrationCreateSpec("integration_set_group", toolutil.RouteAction(client, SetGroupIntegration), "gitlab_set_group_integration",
			"Configure (create or update) any group integration by slug, passing the integration's documented parameters in a free-form `config` object. Requires Owner role on the group (some integrations require GitLab Premium/Ultimate).\n\nThe `config` object is sent verbatim as the request body; see doc/api/group_integrations.md and doc/api/integrations.md for the per-integration fields.\n\nReturns: JSON with the resulting group integration details.\n\nSee also: gitlab_get_group_integration, gitlab_list_group_integrations, gitlab_delete_group_integration"),
		groupIntegrationDeleteSpec("integration_delete_group", toolutil.DestructiveAction(client, deleteGroupIntegrationOutput), "gitlab_delete_group_integration",
			"Delete (disable) a group integration by slug. Requires Owner role on the group.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_group_integrations, gitlab_get_group_integration, gitlab_set_group_integration"),

		// Group-level Datadog integration (requires GitLab Premium/Ultimate on self-managed EE or GitLab.com).
		groupDatadogReadSpec("integration_get_group_datadog", toolutil.RouteAction(client, GetGroupDatadog), "gitlab_get_group_datadog_integration",
			"Read the Datadog integration configured on a group. Requires Owner role and GitLab Premium/Ultimate (self-managed EE or GitLab.com). The api_key is never returned.\n\nReturns: JSON with the group's Datadog integration details (api_url, datadog_env, datadog_service, datadog_site, datadog_tags, archive_trace_events).\n\nSee also: gitlab_set_group_datadog_integration, gitlab_delete_group_datadog_integration"),
		groupDatadogCreateSpec("integration_set_group_datadog", toolutil.RouteAction(client, SetGroupDatadog), "gitlab_set_group_datadog_integration",
			"Create or update the Datadog integration on a group. Requires Owner role and GitLab Premium/Ultimate (self-managed EE or GitLab.com). At least one of api_key, api_url, datadog_env, datadog_service, datadog_site, datadog_tags, archive_trace_events, or use_inherited_settings=true must be supplied.\n\nReturns: JSON with the updated Datadog integration details.\n\nSee also: gitlab_get_group_datadog_integration, gitlab_delete_group_datadog_integration"),
		groupDatadogDeleteSpec("integration_delete_group_datadog", toolutil.DestructiveAction(client, deleteGroupDatadogOutput), "gitlab_delete_group_datadog_integration",
			"Remove the Datadog integration from a group. The stored api_key is cleared; deletion is irreversible. Requires Owner role and GitLab Premium/Ultimate (self-managed EE or GitLab.com).\n\nReturns: confirmation message.\n\nSee also: gitlab_get_group_datadog_integration, gitlab_set_group_datadog_integration"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("integration")
	return out, nil
}

func deleteGroupDatadogOutput(ctx context.Context, client *gitlabclient.Client, input DeleteGroupDatadogInput) (toolutil.DeleteOutput, error) {
	if err := DeleteGroupDatadog(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("integration")
	return out, nil
}

func deleteGroupIntegrationOutput(ctx context.Context, client *gitlabclient.Client, input DeleteGroupIntegrationInput) (toolutil.DeleteOutput, error) {
	if err := DeleteGroupIntegration(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("integration")
	return out, nil
}

func integrationReadSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, integrationOptions(individualTool, description))
}

func integrationCreateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, integrationOptions(individualTool, description))
}

func integrationDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, integrationOptions(individualTool, description))
}

func groupIntegrationReadSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupIntegrationOptions(individualTool, description))
}

func groupIntegrationCreateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupIntegrationOptions(individualTool, description))
}

func groupIntegrationDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupIntegrationOptions(individualTool, description))
}

func groupDatadogReadSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupDatadogOptions(individualTool, description))
}

func groupDatadogCreateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupDatadogOptions(individualTool, description))
}

func groupDatadogDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupDatadogOptions(individualTool, description))
}

// integrationActionMeta is the per-action discovery metadata (purpose-specific
// Usage sentence, natural-language aliases beyond the canonical tool name, and
// related canonical action IDs) used to satisfy the 1:1 metadata-completeness
// norm (R-META). The map is keyed by the individual tool name.
var integrationActionMeta = map[string]struct {
	usage   string
	aliases []string
	related []string
}{
	"gitlab_list_integrations": {
		usage:   "List every integration (service) wired up on a project to see what is active before reading or removing one.",
		aliases: []string{"list project integrations", "show project services", "what integrations are enabled"},
		related: []string{"project.integration_get", "project.integration_set_jira", "project.integration_delete"},
	},
	"gitlab_get_integration": {
		usage:   "Inspect one project integration by slug to read its active status and event-trigger configuration.",
		aliases: []string{"get integration details", "show service config", "view integration settings"},
		related: []string{"project.integration_list", "project.integration_delete", "project.integration_set_jira"},
	},
	"gitlab_delete_integration": {
		usage:   "Disable and remove a project integration by slug when it is no longer needed.",
		aliases: []string{"disable integration", "remove project service", "turn off integration"},
		related: []string{"project.integration_list", "project.integration_get"},
	},
	"gitlab_set_jira_integration": {
		usage:   "Connect a project to a Jira instance, setting the URL, credentials, and which events sync to Jira.",
		aliases: []string{"configure jira", "set up jira integration", "connect project to jira"},
		related: []string{"project.integration_list", "project.integration_get", "project.integration_delete"},
	},
	"gitlab_set_integration": {
		usage:   "Configure any project integration by slug with a free-form config object when no dedicated tool exists (e.g. slack, harbor, jenkins, google-play).",
		aliases: []string{"configure integration", "set up project integration", "enable integration", "update integration config", "set slack integration", "set harbor integration", "set jenkins integration"},
		related: []string{"project.integration_get", "project.integration_list", "project.integration_delete", "project.integration_set_jira"},
	},
	"gitlab_list_group_integrations": {
		usage:   "List every active integration wired up on a group to see what is enabled before reading or removing one.",
		aliases: []string{"list group integrations", "show group services", "what integrations are enabled on the group"},
		related: []string{"project.integration_get_group", "project.integration_set_group", "project.integration_delete_group"},
	},
	"gitlab_get_group_integration": {
		usage:   "Inspect one group integration by slug to read its active status and configuration.",
		aliases: []string{"get group integration details", "show group service config", "view group integration settings"},
		related: []string{"project.integration_list_group", "project.integration_set_group", "project.integration_delete_group"},
	},
	"gitlab_set_group_integration": {
		usage:   "Configure any group integration by slug with a free-form config object, applying it across the group's projects.",
		aliases: []string{"configure group integration", "set up group integration", "enable group integration", "update group integration config"},
		related: []string{"project.integration_get_group", "project.integration_list_group", "project.integration_delete_group"},
	},
	"gitlab_delete_group_integration": {
		usage:   "Disable and remove a group integration by slug when it is no longer needed.",
		aliases: []string{"disable group integration", "remove group service", "turn off group integration"},
		related: []string{"project.integration_list_group", "project.integration_get_group", "project.integration_set_group"},
	},
	"gitlab_get_group_datadog_integration": {
		usage:   "Read the Datadog integration on a group to confirm the configured site, env, service, and tags.",
		aliases: []string{"get group datadog config", "show group datadog integration", "view group datadog settings"},
		related: []string{"project.integration_set_group_datadog", "project.integration_delete_group_datadog", "group.get"},
	},
	"gitlab_set_group_datadog_integration": {
		usage:   "Create or update a group's Datadog integration so logs and CI traces forward to the chosen Datadog site.",
		aliases: []string{"configure group datadog", "set up group datadog", "update group datadog integration"},
		related: []string{"project.integration_get_group_datadog", "project.integration_delete_group_datadog", "group.get"},
	},
	"gitlab_delete_group_datadog_integration": {
		usage:   "Remove a group's Datadog integration and clear its stored API key when forwarding is no longer wanted.",
		aliases: []string{"disable group datadog", "remove group datadog integration", "delete group datadog config"},
		related: []string{"project.integration_get_group_datadog", "project.integration_set_group_datadog"},
	},
}

// applyIntegrationMeta overlays the per-action discovery metadata onto the
// shared spec options, replacing the generic Usage placeholder, the
// toolname-only alias, and the generic RelatedActions. Unknown tools keep the
// shared defaults.
func applyIntegrationMeta(opts toolutil.ActionSpecOptions, individualTool string) toolutil.ActionSpecOptions {
	meta, ok := integrationActionMeta[individualTool]
	if !ok {
		return opts
	}
	opts.Usage = meta.usage
	opts.Aliases = append([]string{individualTool}, meta.aliases...)
	opts.RelatedActions = append([]string(nil), meta.related...)
	return opts
}

func integrationOptions(individualTool, description string) toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute integrations domain action.", Tags: []string{"project", "integration"},
		RelatedActions: []string{"project.get"},
		OpenWorld:      true,
		OwnerPackage:   "integrations",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
	return applyIntegrationMeta(opts, individualTool)
}

// groupIntegrationOptions returns the spec options shared by the generic
// group-level integration actions (list/get/set/delete). The group integration
// management API is Free tier (doc/api/group_integrations.md page tier = Free,
// Premium, Ultimate); it is gated on Owner role, not on a licensing tier.
func groupIntegrationOptions(individualTool, description string) toolutil.ActionSpecOptions {
	opts := integrationOptions(individualTool, description)
	opts.Tags = []string{"group", "integration"}
	if _, ok := integrationActionMeta[individualTool]; !ok {
		opts.RelatedActions = []string{"group.get"}
	}
	return applyIntegrationMeta(opts, individualTool)
}

// groupDatadogOptions returns the spec options shared by the three group-level
// Datadog integration actions. The group integration management API is Free tier
// (doc/api/group_integrations.md page tier = Free, Premium, Ultimate).
func groupDatadogOptions(individualTool, description string) toolutil.ActionSpecOptions {
	opts := integrationOptions(individualTool, description)
	opts.Tags = []string{"group", "integration", "datadog"}
	// applyIntegrationMeta already set group-specific RelatedActions; only fall
	// back to the generic group.get when no per-action metadata was found.
	if _, ok := integrationActionMeta[individualTool]; !ok {
		opts.RelatedActions = []string{"group.get"}
	}
	return applyIntegrationMeta(opts, individualTool)
}
