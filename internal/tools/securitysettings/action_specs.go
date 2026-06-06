package securitysettings

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ProjectActionSpecs returns canonical specs for project security settings actions.
func ProjectActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		projectSecurityReadSpec("security_settings_get", toolutil.RouteAction(client, GetProject), "gitlab_get_project_security_settings"),
		projectSecurityUpdateSpec("security_settings_update", toolutil.RouteAction(client, UpdateProject), "gitlab_update_project_secret_push_protection"),
	}
}

// GroupActionSpecs returns canonical specs for group security setting actions.
func GroupActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupSecuritySettingUpdateSpec("security_settings_update", toolutil.RouteAction(client, UpdateGroup), "gitlab_update_group_secret_push_protection"),
	}
}

func projectSecurityReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, projectSecurityOptions(individualTool))
}

func projectSecurityUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, projectSecurityOptions(individualTool))
}

func projectSecurityOptions(individualTool string) toolutil.ActionSpecOptions {
	usage := "Use project security settings for secret push protection and secret_push_protection_enabled changes. Do not use project.update for secret push protection."
	tags := []string{"project", "security"}
	// Default: every action keeps its own canonical individual-tool
	// name in the alias list. Per-action overrides below may add more
	// aliases; they must always preserve the individual tool name so
	// alias-based resolution still hits the action.
	aliases := []string{individualTool}
	var related []string
	switch individualTool {
	case "gitlab_get_project_security_settings":
		usage = "Reads the project's security settings (currently secret_push_protection_enabled and continuous_vulnerability_scans_enabled, among others). Use this when the prompt asks for the security posture, secret-push protection status, or vulnerability scanning config of a project. Do not use project.update for these."
		tags = []string{"project", "security", "secret_push_protection", "vulnerability_scan", "settings", "configuration"}
		aliases = []string{
			individualTool,
			"gitlab_get_project_secret_push_protection",
			"project_secret_push_protection_get",
			"project_security_posture",
		}
		related = []string{"project.get", "project.security_settings_update"}
	case "gitlab_update_project_secret_push_protection":
		related = []string{"project.security_settings_get"}
	}
	return toolutil.ActionSpecOptions{
		Aliases:        aliases,
		Tags:           tags,
		Usage:          usage,
		RelatedActions: related,
		OpenWorld:      true,
		Edition:        "ultimate",
		OwnerPackage:   "securitysettings",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

func groupSecuritySettingUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupSecuritySettingsOptions(individualTool))
}

func groupSecuritySettingsOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"group", "security"},
		Usage:          "Use group security settings for secret push protection and secret_push_protection_enabled changes inherited by projects. Do not use group.update for secret push protection.",
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "securitysettings",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
