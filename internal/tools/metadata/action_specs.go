package metadata

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for metadata tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		metadataReadSpec("metadata_get", toolutil.RouteAction(client, Get), "gitlab_get_metadata"),
	}
}

func metadataReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, metadataOptions(individualTool))
}

func metadataOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        []string{"instance metadata", "gitlab version", "server metadata", "gitlab revision"},
		Tags:           []string{"admin", "metadata", "version"},
		Usage:          "Read GitLab instance metadata such as version and revision. Do not use this for application settings.",
		RelatedActions: []string{"admin.settings_get", "admin.app_statistics_get", "server.health_check"},
		OpenWorld:      true,
		OwnerPackage:   "metadata",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: "Get GitLab instance metadata such as version, revision, KAS endpoints, and enterprise edition flag. Returns: the current instance metadata object. See also: gitlab_server_status, gitlab_get_settings, gitlab_get_application_statistics.",
		},
	}
}
