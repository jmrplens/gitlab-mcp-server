package releaselinks

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionReleaseCreate     = "release.create"
	actionReleaseLinkList   = "release_link.list"
	actionReleaseLinkListID = "release.link_list"
	actionPackagePublish    = "package.publish"
	actionPackagePublishDir = "package.publish_directory"
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
		options.Aliases = []string{"get release link", "show release asset link", "fetch release download link"}
	}
	if actionName == "link_list" {
		options.Usage = "List asset links attached to a release tag. Use to enumerate downloadable assets for a release before getting, updating, or deleting a specific link."
		options.Aliases = []string{"list release links", "show release asset links", "list release downloads", "enumerate release attachments"}
	}
	if actionName == "link_delete" {
		options.Usage = "Delete a release asset link by link_id. Use to detach a downloadable asset from a release; the underlying file or package is not removed."
		options.Aliases = []string{"delete release link", "remove release asset link", "detach release download"}
		options.RelatedActions = []string{"release.link_get", actionReleaseLinkListID, "release.get"}
	}
	if actionName == "link_create_batch" {
		options.Usage = "Create MULTIPLE release asset links in one call. Provide the release tag_name and a links array, each entry with a name and an absolute url. Use this instead of repeated link_create when attaching several assets at once, for example one link per file uploaded by package.publish_directory."
		options.Aliases = []string{"create multiple release links", "batch create release asset links", "add several release links at once", "link multiple package files to a release", "create release asset links in one call", "attach multiple assets to release", "link each uploaded file to release"}
		options.RelatedActions = []string{"release.link_create", actionReleaseCreate, actionPackagePublishDir, actionPackagePublish}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"links": {
				SemanticRole: "release_asset_link_batch",
				ValueSource:  "An array of {name, url} objects, one per asset. For package assets, use the absolute URLs returned by package publish actions; do not construct package registry URLs manually.",
				CommonConfusions: []string{
					"Do not call link_create once per asset when several are requested; pass them all in the links array of link_create_batch.",
					"Do not put a single name/url at top level; each link goes inside the links array.",
				},
			},
		}
	}
	if actionName == "link_create" || actionName == "link_update" {
		if actionName == "link_create" {
			options.Usage = "Create a single release asset link. The url must be an absolute http, https, or ftp URL; do not pass local file paths or relative paths as url. To create several links at once (e.g. one per uploaded package file), use link_create_batch instead."
			options.Aliases = []string{"create release link", "add release asset link", "link release asset"}
		} else {
			options.Usage = "Update an existing release asset link by link_id. When changing url, use an absolute http, https, or ftp URL; do not pass local file paths or relative paths as url."
			options.Aliases = []string{"update release link", "edit release asset link", "modify release asset link"}
		}
		options.RelatedActions = []string{actionReleaseCreate, actionReleaseLinkListID, actionPackagePublish, actionPackagePublishDir}
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
		options.RelatedActions = []string{actionReleaseCreate, actionPackagePublishDir, actionPackagePublish, actionReleaseLinkListID}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"links": {
				SemanticRole: "release_asset_links",
				ValueSource:  "Array of link objects. Each item supports name, url, link_type, and an optional direct_asset_path (prefer it over the deprecated filepath); url must be absolute.",
				CommonConfusions: []string{
					"Prefer direct_asset_path over the deprecated filepath when setting a direct asset link.",
					"For package assets, use the package URLs returned by gitlab_package publish actions instead of constructing URLs manually.",
				},
			},
		}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("links", map[string]any{"description": "Array of release asset links. Each item supports name, url, link_type, direct_asset_path, and the deprecated filepath."}),
			toolutil.SchemaPropertyOverride("links.url", map[string]any{
				"description": "Absolute http, https, or ftp URL of the link target. For package assets, use the URL returned by gitlab_package publish actions; do not construct package URLs manually.",
				"format":      "uri",
				"pattern":     "^(https?|ftp)://",
			}),
			toolutil.SchemaPropertyOverride("links.link_type", map[string]any{"description": "Type of the link: package, runbook, image, or other. Use package for package registry assets."}),
		}
	}
	if description := releaseLinkDescriptions[actionName]; description != "" {
		options.IndividualTool.Description = description
	}
	return options
}

// releaseLinkDescriptions maps each release-link action to its
// "Returns: … See also: …" individual-tool description (R-META; 1:1 audit).
// Returned objects mirror the GitLab ReleaseLink schema: id, name, url,
// direct_asset_url, link_type, and external.
var releaseLinkDescriptions = map[string]string{
	"link_create":       "Create a single release asset link. Returns: the created link with id, name, url, direct_asset_url, link_type, and external. See also: gitlab_release_link_create_batch, gitlab_release_link_list, gitlab_release_link_update.",
	"link_create_batch": "Create multiple release asset links in one call. Returns: the created links (id, name, url, direct_asset_url, link_type, external) and any failed entries. See also: gitlab_release_link_create, gitlab_release_link_list, gitlab_package_publish.",
	"link_get":          "Get one release asset link by link_id. Returns: the link with id, name, url, direct_asset_url, link_type, and external. See also: gitlab_release_link_list, gitlab_release_link_update, gitlab_release_link_delete.",
	"link_list":         "List asset links for a release with offset or keyset pagination. Returns: matching links (id, name, url, direct_asset_url, link_type, external) and pagination metadata. See also: gitlab_release_link_get, gitlab_release_link_create, gitlab_release_link_create_batch.",
	"link_update":       "Update an existing release asset link by link_id. Returns: the updated link with id, name, url, direct_asset_url, link_type, and external. See also: gitlab_release_link_get, gitlab_release_link_list, gitlab_release_link_delete.",
	"link_delete":       "Delete a release asset link by link_id. Returns: the deleted link with id, name, url, direct_asset_url, link_type, and external. See also: gitlab_release_link_get, gitlab_release_link_list.",
}
