package appearance

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for appearance tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		appearanceReadSpec("appearance_get", toolutil.RouteAction(client, Get), "gitlab_get_appearance"),
		appearanceUpdateSpec("appearance_update", toolutil.RouteAction(client, Update), "gitlab_update_appearance"),
	}
}

func appearanceReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, appearanceOptions(name, individualTool))
}

func appearanceUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, appearanceOptions(name, individualTool))
}

func appearanceOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute appearance domain action.", Tags: []string{"admin", "appearance", "branding"},
		OpenWorld:    true,
		OwnerPackage: "appearance",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:  individualTool,
			Title: toolutil.TitleFromName(individualTool),
		},
	}
	switch actionName {
	case "appearance_get":
		options.Aliases = []string{"appearance", "application appearance", "instance appearance", "branding settings", "gitlab appearance"}
		options.Usage = "Read the current GitLab application appearance and branding settings. Use this for logos, banners, PWA labels, and instance message colors rather than general application settings or version metadata."
		options.RelatedActions = []string{"admin.settings_get", "admin.metadata_get", "admin.appearance_update"}
		options.IndividualTool.Description = "Get the current GitLab application appearance and branding settings. Returns: the instance appearance object including title, messages, logos, and PWA labels. See also: gitlab_update_appearance, gitlab_get_settings, gitlab_get_metadata."
	case "appearance_update":
		options.Aliases = []string{"update appearance", "change appearance", "update branding", "change branding", "appearance settings update"}
		options.Usage = "Update GitLab application appearance and branding settings such as title, messages, colors, PWA labels, and profile guidance text. Requires administrator access and changes the instance UI immediately."
		options.RelatedActions = []string{"admin.appearance_get", "admin.settings_get", "admin.metadata_get"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"title": {
				SemanticRole:   "instance_brand_title",
				ValueSource:    "Instance branding title to display in the GitLab UI header and metadata surfaces.",
				ExampleBinding: `params.title:"GitLab Engineering"`,
			},
			"message_background_color": {
				SemanticRole:     "hex_color",
				ValueSource:      "Hex color string such as #e75e40 for the appearance banner background.",
				CommonConfusions: []string{"Provide a CSS-style hex color such as #ffffff; do not send color names or RGB tuples."},
			},
			"message_font_color": {
				SemanticRole:     "hex_color",
				ValueSource:      "Hex color string such as #ffffff for the appearance banner text.",
				CommonConfusions: []string{"Provide a CSS-style hex color such as #000000; do not send color names or RGB tuples."},
			},
		}
		options.IndividualTool.Description = "Update GitLab application appearance and branding settings. Returns: the updated appearance object after GitLab applies the change. See also: gitlab_get_appearance, gitlab_get_settings, gitlab_get_metadata."
	}
	return options
}
