package appearance

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ActionSpecs returns canonical specs for appearance tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("appearance_get", toolutil.RouteAction(client, Get), appearanceGetOptions("gitlab_get_appearance")),
		toolutil.NewUpdateActionSpec("appearance_update", toolutil.RouteAction(client, Update), appearanceUpdateOptions("gitlab_update_appearance")),
	}
}

// appearanceOptions is what both appearance actions declare alike: the domain
// tags, the owning package, and the individual tool the action projects to.
// Each action fills in its own aliases, usage, related actions and description
// on top of it.
//
// The two actions call it rather than sharing one switch on the action name,
// because the caller already knows which action it is describing. Re-deriving
// that from a string gave the second case label no way of ever being false:
// only the two names reach it, and the first case takes one of them, so half
// of that branch was a state no test could put the package in, and a third
// action would have added another.
func appearanceOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:         []string{"admin", "appearance", "branding"},
		OpenWorld:    true,
		OwnerPackage: "appearance",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:  individualTool,
			Title: toolutil.TitleFromName(individualTool),
		},
	}
}

func appearanceGetOptions(individualTool string) toolutil.ActionSpecOptions {
	options := appearanceOptions(individualTool)
	options.Aliases = []string{"appearance", "application appearance", "instance appearance", "branding settings", "gitlab appearance"}
	options.Usage = "Read the current GitLab application appearance and branding settings. Use this for logos, banners, PWA labels, and instance message colors rather than general application settings or version metadata."
	options.RelatedActions = []string{"admin.settings_get", "admin.metadata_get", "admin.appearance_update"}
	options.IndividualTool.Description = "Get the current GitLab application appearance and branding settings. Returns: the instance appearance object including title, messages, logos, and PWA labels. See also: gitlab_update_appearance, gitlab_get_settings, gitlab_get_metadata."
	return options
}

func appearanceUpdateOptions(individualTool string) toolutil.ActionSpecOptions {
	options := appearanceOptions(individualTool)
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
			CommonConfusions: []string{"Provide a CSS-style hex color such as #ffffff. Do not send color names or RGB tuples."},
		},
		"message_font_color": {
			SemanticRole:     "hex_color",
			ValueSource:      "Hex color string such as #ffffff for the appearance banner text.",
			CommonConfusions: []string{"Provide a CSS-style hex color such as #000000. Do not send color names or RGB tuples."},
		},
	}
	options.IndividualTool.Description = "Update GitLab application appearance and branding settings. Returns: the updated appearance object after GitLab applies the change. See also: gitlab_get_appearance, gitlab_get_settings, gitlab_get_metadata."
	return options
}
