package releaselinks

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	return toolutil.NewReadActionSpec(name, route, releaseLinkOptions(name, individualTool))
}

func releaseLinkCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, releaseLinkOptions(name, individualTool))
}

func releaseLinkUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, releaseLinkOptions(name, individualTool))
}

func releaseLinkDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, releaseLinkOptions(name, individualTool))
}

func releaseLinkOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute releaselinks domain action.", Tags: []string{"release", "asset", "link"},
		RelatedActions: []string{"release.get", "release.update", "package.list"},
		OpenWorld:      true,
		OwnerPackage:   "releaselinks",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if actionName == "link_get" {
		options.Usage = "Get one release asset link by link_id. Use when the task references a specific release asset link."
	}
	if actionName == "link_create" || actionName == "link_update" {
		if actionName == "link_create" {
			options.Usage = "Create a single release asset link. The url must be an absolute http, https, or ftp URL; do not pass local file paths or relative paths as url."
			options.Aliases = []string{"create release link", "add release asset link", "link release asset"}
		} else {
			options.Usage = "Update an existing release asset link by link_id. When changing url, use an absolute http, https, or ftp URL; do not pass local file paths or relative paths as url."
			options.Aliases = []string{"update release link", "edit release asset link", "modify release asset link"}
		}
		options.RelatedActions = []string{"release.create", "release.link_list", "package.publish", "package.publish_directory"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"url": {
				SemanticRole: "release_asset_absolute_url",
				ValueSource:  "Absolute URL accepted by GitLab release links. For package assets, use the URL returned by package publish actions.",
				CommonConfusions: []string{
					"Do not use local file paths, relative paths, or package file names as url.",
					"Do not construct package registry URLs manually when a package publish action returned the asset URL.",
				},
			},
		}
		if actionName == "link_update" {
			options.ParameterGuidance["link_id"] = toolutil.ParameterGuidance{
				SemanticRole: "release_asset_link_identifier",
				ValueSource:  "Use the release link ID returned by release.link_create, release.link_create_batch, or release.link_list.",
				CommonConfusions: []string{
					"Do not use link_update to create a new release asset link; call link_create or link_create_batch first.",
				},
			}
		}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("url", map[string]any{
				"description": "Absolute http, https, or ftp URL of the link target. Do not use local file paths or relative paths.",
				"format":      "uri",
				"pattern":     "^(https?|ftp)://",
			}),
			toolutil.SchemaPropertyOverride("link_type", map[string]any{
				"enum":        []any{"other", "runbook", "image", "package"},
				"description": "Type of the release link: other, runbook, image, or package.",
			}),
		}
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
			toolutil.SchemaPropertyOverride("links.url", map[string]any{
				"description": "Absolute http, https, or ftp URL of the link target. For package assets, use the URL returned by gitlab_package publish actions; do not construct package URLs manually.",
				"format":      "uri",
				"pattern":     "^(https?|ftp)://",
			}),
			toolutil.SchemaPropertyOverride("links.link_type", map[string]any{"description": "Type of the link: package, runbook, image, or other. Use package for package registry assets."}),
		}
	}
	return options
}
