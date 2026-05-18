package releaselinks

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for release asset link actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		releaseLinkCreateSpec("link_create", toolutil.RouteAction(client, Create), "gitlab_release_link_create"),
		releaseLinkCreateSpec("link_create_batch", toolutil.RouteAction(client, CreateBatch), "gitlab_release_link_create_batch"),
		releaseLinkReadSpec("link_get", toolutil.RouteAction(client, Get), "gitlab_release_link_get"),
		releaseLinkReadSpec("link_list", toolutil.RouteAction(client, List), "gitlab_release_link_list"),
		releaseLinkUpdateSpec("link_update", toolutil.RouteAction(client, Update), "gitlab_release_link_update"),
		releaseLinkDeleteSpec("link_delete", toolutil.DestructiveAction(client, Delete), "gitlab_release_link_delete"),
	}
}

func releaseLinkReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := releaseLinkOptions(name, individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func releaseLinkCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, releaseLinkOptions(name, individualTool))
}

func releaseLinkUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := releaseLinkOptions(name, individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func releaseLinkDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := releaseLinkOptions(name, individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func releaseLinkOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Tags:           []string{"release", "asset", "link"},
		RelatedActions: []string{"release.get", "release.update", "package.list"},
		OpenWorld:      true,
		OwnerPackage:   "releaselinks",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if actionName == "link_create_batch" {
		options.Usage = "Create multiple release asset links in one call. Use absolute URLs returned by package publish actions for package assets."
		options.Aliases = []string{"batch release links", "release package asset links", "link package files to release", "create multiple release assets"}
		options.RelatedActions = []string{"release.create", "package.publish_directory", "package.publish", "release.link_list"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"links": {
				SemanticRole: "release_asset_links",
				ValueSource:  "Array of link objects. Each item supports only name, url, and link_type; url must be absolute.",
				CommonConfusions: []string{
					"Do not send direct_asset_path or filepath to link_create_batch.",
					"For package assets, use the package URLs returned by gitlab_package publish actions instead of constructing URLs manually.",
				},
			},
		}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("links", map[string]any{"description": "Array of release asset links. Each item supports only name, url, and link_type."}),
			toolutil.SchemaPropertyOverride("links.url", map[string]any{"description": "Absolute URL of the link target. For package assets, use the URL returned by gitlab_package publish actions; do not construct package URLs manually."}),
			toolutil.SchemaPropertyOverride("links.link_type", map[string]any{"description": "Type of the link: package, runbook, image, or other. Use package for package registry assets."}),
		}
	}
	return options
}
