package securitysettings

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ProjectActionSpecs returns canonical specs for project security settings actions.
func ProjectActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_get_project_security_settings — read the project's security settings (secret push protection, scanning).
		projectSecurityReadSpec("security_settings_get", toolutil.RouteAction(client, GetProject), "gitlab_get_project_security_settings"),
		// gitlab_update_project_secret_push_protection — toggle secret push protection on a project.
		projectSecurityUpdateSpec("security_settings_update", toolutil.RouteAction(client, UpdateProject), "gitlab_update_project_secret_push_protection"),
	}
}

// GroupActionSpecs returns canonical specs for group security setting actions.
func GroupActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_update_group_secret_push_protection — toggle secret push protection for a group (inherits to projects).
		groupSecuritySettingUpdateSpec("security_settings_update", toolutil.RouteAction(client, UpdateGroup), "gitlab_update_group_secret_push_protection"),
	}
}

// projectSecurityReadSpec builds the canonical read-only spec for a project security settings tool.
func projectSecurityReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, projectSecurityOptions(individualTool))
}

// projectSecurityUpdateSpec builds the canonical update spec for a project security settings tool.
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
	var description string
	switch individualTool {
	case "gitlab_get_project_security_settings":
		usage = "Reads the project's security settings (currently secret_push_protection_enabled and continuous_vulnerability_scans_enabled, among others). Use this when the prompt asks for the security posture, secret-push protection status, or vulnerability scanning config of a project. Do not use project.update for these."
		tags = []string{"project", "security", "secret_push_protection", "vulnerability_scan", "settings", "configuration"}
		aliases = []string{
			individualTool,
			"gitlab_get_project_security_settings",
			"project_security_settings_get",
			"project_secret_push_protection_get",
			"project_security_posture",
			"project_vulnerability_scan_settings",
		}
		related = []string{"project.get", "project.security_settings_update"}
		description = "Read a project's security settings (Ultimate). Returns: secret_push_protection_enabled, continuous_vulnerability_scans_enabled, container scanning, and per-analyzer auto-fix flags. See also: gitlab_update_project_secret_push_protection, gitlab_project_get."
	case "gitlab_update_project_secret_push_protection":
		usage = "Toggles the project's secret_push_protection_enabled setting so GitLab rejects pushes that contain detected secrets. Set secret_push_protection_enabled to true to block leaked credentials at push time, or false to allow them. Requires Maintainer role and an Ultimate license. Do not use project.update for this. It does not change secret push protection."
		tags = []string{"project", "security", "secret_push_protection", "settings", "configuration"}
		aliases = []string{
			individualTool,
			"enable secret push protection on a project",
			"disable secret push protection on a project",
			"block secrets on push for a project",
		}
		related = []string{"project.security_settings_get"}
		description = "Enable or disable secret push protection for a project (Ultimate). Returns: the project's security settings including secret_push_protection_enabled, continuous vulnerability scanning, and auto-fix flags. See also: gitlab_get_project_security_settings, gitlab_update_group_secret_push_protection."
	}
	return toolutil.ActionSpecOptions{
		Aliases:        aliases,
		Tags:           tags,
		Usage:          usage,
		RelatedActions: related,
		OpenWorld:      true,
		Edition:        "ultimate",
		OwnerPackage:   "securitysettings",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}

// groupSecuritySettingUpdateSpec builds the canonical update spec for a group security settings tool.
func groupSecuritySettingUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupSecuritySettingsOptions(individualTool))
}

func groupSecuritySettingsOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{
			individualTool,
			"enable secret push protection for a group",
			"disable secret push protection for a group",
			"block secrets on push across a group",
		},
		Tags:           []string{"group", "security", "secret_push_protection", "settings", "configuration"},
		Usage:          "Toggles a group's secret_push_protection_enabled setting, which is inherited by the group's projects so GitLab rejects pushes containing detected secrets. Set secret_push_protection_enabled to true to enforce protection group-wide, or false to disable it. Use projects_to_exclude to opt specific projects out. Requires Owner role and an Ultimate license. Do not use group.update for this. It does not change secret push protection.",
		RelatedActions: []string{"group.get", "project.security_settings_get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "securitysettings",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: "Enable or disable secret push protection for a whole group, inherited by its projects (Ultimate). Returns: the group's secret_push_protection_enabled state and any errors. See also: gitlab_update_project_secret_push_protection, gitlab_group_get.",
		},
	}
}
