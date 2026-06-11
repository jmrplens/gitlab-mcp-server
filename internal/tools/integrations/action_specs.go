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

func integrationReadSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, integrationOptions(individualTool, description))
}

func integrationCreateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, integrationOptions(individualTool, description))
}

func integrationDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, integrationOptions(individualTool, description))
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

func integrationOptions(individualTool, description string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute integrations domain action.", Tags: []string{"project", "integration"},
		RelatedActions: []string{"project.get"},
		OpenWorld:      true,
		OwnerPackage:   "integrations",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}

// groupDatadogOptions returns the spec options shared by the three group-level
// Datadog integration actions. The endpoint requires GitLab Premium/Ultimate
// (self-managed EE or GitLab.com), so we mark the edition explicitly.
func groupDatadogOptions(individualTool, description string) toolutil.ActionSpecOptions {
	opts := integrationOptions(individualTool, description)
	opts.Tags = []string{"group", "integration", "datadog"}
	opts.RelatedActions = []string{"group.get"}
	opts.Edition = "premium"
	return opts
}
