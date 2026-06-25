package mrapprovalsettings

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request approval settings actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		approvalSettingsReadSpec("approval_settings_group_get", toolutil.RouteAction(client, GetGroupSettings), "gitlab_get_group_mr_approval_settings"),
		approvalSettingsUpdateSpec("approval_settings_group_update", toolutil.RouteAction(client, UpdateGroupSettings), "gitlab_update_group_mr_approval_settings"),
		approvalSettingsReadSpec("approval_settings_project_get", toolutil.RouteAction(client, GetProjectSettings), "gitlab_get_project_mr_approval_settings"),
		approvalSettingsUpdateSpec("approval_settings_project_update", toolutil.RouteAction(client, UpdateProjectSettings), "gitlab_update_project_mr_approval_settings"),
	}
}

func approvalSettingsReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, approvalSettingsOptions(individualTool))
}

func approvalSettingsUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, approvalSettingsOptions(individualTool))
}

// approvalSettingsMetaEntry holds the non-generic discovery metadata for one
// MR approval settings action: human usage guidance, natural-language aliases,
// related-action cross-links, and the "Returns: … See also: …" individual-tool
// description required by the 1:1 audit (R-META).
type approvalSettingsMetaEntry struct {
	usage          string
	aliases        []string
	relatedActions []string
	description    string
}

// approvalSettingsMeta maps each individual MR approval settings tool to its
// discovery metadata so none of the four canonical actions inherits the generic
// placeholder metadata.
var approvalSettingsMeta = map[string]approvalSettingsMetaEntry{
	"gitlab_get_group_mr_approval_settings": {
		usage:          "Read the merge request approval settings for a group. Use this to inspect inherited and locked approval policy before updating a group or any of its projects.",
		aliases:        []string{"get group mr approval settings", "show group approval settings", "view group merge request approval policy"},
		relatedActions: []string{"approval_settings_group_update", "mrapprovals.get_state", "group.get"},
		description:    "Read the merge request approval settings of a group. Returns: each setting (allow author approval, allow committer approval, allow approver-list overrides, retain approvals on push, selective code owner removals, require password to approve, require reauthentication to approve) with its value, locked flag, and inherited-from source. See also: gitlab_update_group_mr_approval_settings, gitlab_get_project_mr_approval_settings.",
	},
	"gitlab_update_group_mr_approval_settings": {
		usage:          "Update the merge request approval settings for a group. Set only the fields you want to change; omitted fields are left unchanged. Group-level changes can lock the corresponding setting for projects in the group.",
		aliases:        []string{"update group mr approval settings", "change group approval settings", "set group merge request approval policy"},
		relatedActions: []string{"approval_settings_group_get", "mrapprovals.get_state", "group.get"},
		description:    "Update the merge request approval settings of a group. Returns: the resulting settings (value, locked flag, and inherited-from source for each setting). See also: gitlab_get_group_mr_approval_settings, gitlab_update_project_mr_approval_settings.",
	},
	"gitlab_get_project_mr_approval_settings": {
		usage:          "Read the merge request approval settings for a project. Use this to inspect the effective approval policy, including which settings are locked by the parent group.",
		aliases:        []string{"get project mr approval settings", "show project approval settings", "view project merge request approval policy"},
		relatedActions: []string{"approval_settings_project_update", "mrapprovals.get_state", "project.get"},
		description:    "Read the merge request approval settings of a project. Returns: each setting (allow author approval, allow committer approval, allow approver-list overrides, retain approvals on push, selective code owner removals, require password to approve, require reauthentication to approve) with its value, locked flag, and inherited-from source. See also: gitlab_update_project_mr_approval_settings, gitlab_get_group_mr_approval_settings.",
	},
	"gitlab_update_project_mr_approval_settings": {
		usage:          "Update the merge request approval settings for a project. Set only the fields you want to change; omitted fields are left unchanged. Settings locked by the parent group cannot be overridden here.",
		aliases:        []string{"update project mr approval settings", "change project approval settings", "set project merge request approval policy"},
		relatedActions: []string{"approval_settings_project_get", "mrapprovals.get_state", "project.get"},
		description:    "Update the merge request approval settings of a project. Returns: the resulting settings (value, locked flag, and inherited-from source for each setting). See also: gitlab_get_project_mr_approval_settings, gitlab_update_group_mr_approval_settings.",
	},
}

func approvalSettingsOptions(individualTool string) toolutil.ActionSpecOptions {
	meta := approvalSettingsMeta[individualTool]

	options := toolutil.ActionSpecOptions{
		Aliases:        meta.aliases,
		Tags:           []string{"merge_request", "approval_settings"},
		Usage:          meta.usage,
		RelatedActions: meta.relatedActions,
		OpenWorld:      true,
		OwnerPackage:   "mrapprovalsettings",
		// The merge_request_approval_settings API is page-badged Premium,
		// Ultimate with no per-section override, so every group/project MR
		// approval-settings action is Premium minimum.
		Edition: "premium",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: meta.description,
		},
	}
	return options
}
