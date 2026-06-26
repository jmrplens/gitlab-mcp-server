package geo

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionGeoGet       = "geo.get"
	actionGeoList      = "geo.list"
	actionGeoGetStatus = "geo.get_status"
	actionGeoEdit      = "geo.edit"
)

// ActionSpecs returns canonical specs for Geo site management actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		geoCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_create_geo_site"),
		geoReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_geo_sites"),
		geoReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_geo_site"),
		geoUpdateSpec("edit", toolutil.RouteAction(client, Edit), "gitlab_edit_geo_site"),
		geoDeleteSpec("delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_geo_site"),
		geoUpdateSpec("repair", toolutil.RouteAction(client, Repair), "gitlab_repair_geo_site"),
		geoReadSpec("list_status", toolutil.RouteAction(client, ListStatus), "gitlab_list_status_all_geo_sites"),
		geoReadSpec("get_status", toolutil.RouteAction(client, GetStatus), "gitlab_get_status_geo_site"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input IDInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted Geo site %d.", input.ID),
	}, nil
}

func geoReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := geoOptions(individualTool)
	decorateGeoMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func geoCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := geoOptions(individualTool)
	decorateGeoMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

func geoUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := geoOptions(individualTool)
	decorateGeoMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func geoDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := geoOptions(individualTool)
	decorateGeoMeta(&options, individualTool)
	return toolutil.NewDeleteActionSpec(name, route, options)
}

// decorateGeoMeta fills non-generic Usage, natural-language Aliases,
// RelatedActions, and the "Returns: … See also: …" individual-tool
// description for each Geo site action (1:1 audit R-META).
func decorateGeoMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := geoActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// geoActionMetaEntry is the discovery metadata for one Geo site action.
type geoActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// geoActionMeta maps each individual Geo site tool to its discovery metadata.
var geoActionMeta = map[string]geoActionMetaEntry{
	"gitlab_create_geo_site": {
		usage:       "Register a new Geo site (secondary or primary) on the instance. Provide a unique name and a reachable url; only one site may be primary. Requires admin access and GitLab Premium/Ultimate.",
		aliases:     []string{"create geo site", "add geo node", "register geo secondary"},
		related:     []string{actionGeoList, actionGeoGet, actionGeoEdit},
		description: "Create a new Geo site. Returns: the site with id, name, url, primary/enabled flags, capacities, selective-sync settings, and navigation links. See also: gitlab_list_geo_sites, gitlab_get_geo_site, gitlab_edit_geo_site.",
	},
	"gitlab_list_geo_sites": {
		usage:       "List all configured Geo sites on the instance. Use order_by/sort and offset or keyset pagination for large fleets. Requires admin access.",
		aliases:     []string{"list geo sites", "show geo nodes", "geo fleet overview"},
		related:     []string{actionGeoGet, "geo.list_status", "geo.create"},
		description: "List all Geo sites. Returns: each site with id, name, url, primary/enabled flags, capacities, and pagination metadata. See also: gitlab_get_geo_site, gitlab_list_status_all_geo_sites, gitlab_create_geo_site.",
	},
	"gitlab_get_geo_site": {
		usage:       "Fetch one Geo site by its numeric id. Use after listing sites or when the prompt names a concrete site id. Requires admin access.",
		aliases:     []string{"get geo site", "show geo node details", "fetch geo site"},
		related:     []string{actionGeoList, actionGeoEdit, actionGeoGetStatus},
		description: "Get a single Geo site by id. Returns: the site with name, url, primary/enabled flags, capacities, selective-sync settings, and navigation links. See also: gitlab_list_geo_sites, gitlab_edit_geo_site, gitlab_get_status_geo_site.",
	},
	"gitlab_edit_geo_site": {
		usage:       "Update settings of an existing Geo site (enabled, name, url, capacities, selective sync). The primary flag cannot be toggled; recreate the site to change it.",
		aliases:     []string{"edit geo site", "update geo node", "configure geo site"},
		related:     []string{actionGeoGet, actionGeoList, "geo.repair"},
		description: "Edit a Geo site's settings. Returns: the updated site with its capacities and selective-sync configuration. See also: gitlab_get_geo_site, gitlab_list_geo_sites, gitlab_repair_geo_site.",
	},
	"gitlab_delete_geo_site": {
		usage:       "Permanently remove a Geo site. Destructive and irreversible; the primary cannot be deleted while secondaries exist. Confirm the id before calling.",
		aliases:     []string{"delete geo site", "remove geo node", "deregister geo secondary"},
		related:     []string{actionGeoGet, actionGeoList},
		description: "Delete a Geo site permanently. Returns: a success confirmation naming the site. See also: gitlab_get_geo_site, gitlab_list_geo_sites.",
	},
	"gitlab_repair_geo_site": {
		usage:       "Repair the OAuth authentication of a Geo secondary by re-creating its OAuth application. Must be run from the primary site.",
		aliases:     []string{"repair geo site", "fix geo oauth", "rebuild geo oauth app"},
		related:     []string{actionGeoGet, actionGeoGetStatus, actionGeoEdit},
		description: "Repair a Geo site's OAuth application. Returns: the repaired site (or a hint to refresh via get when GitLab returns no body). See also: gitlab_get_geo_site, gitlab_get_status_geo_site.",
	},
	"gitlab_list_status_all_geo_sites": {
		usage:       "List the full replication and verification status of every Geo site. Use order_by/sort and offset or keyset pagination. Status is collected by the primary; secondaries may show stale data when lagging.",
		aliases:     []string{"list geo statuses", "geo replication overview", "show all geo node status"},
		related:     []string{actionGeoGetStatus, actionGeoList},
		description: "List replication status for all Geo sites. Returns: per-site health, replication lag, and detailed sync/verification/checksum counts and percentages, with pagination metadata. See also: gitlab_get_status_geo_site, gitlab_list_geo_sites.",
	},
	"gitlab_get_status_geo_site": {
		usage:       "Fetch the full replication and verification status of one Geo site by id. The site must have reported status at least once. Requires admin access.",
		aliases:     []string{"get geo status", "show geo node status", "geo replication status"},
		related:     []string{"geo.list_status", actionGeoGet},
		description: "Get replication status for one Geo site. Returns: health, replication lag, and detailed sync/verification/checksum counts and percentages plus navigation links. See also: gitlab_list_status_all_geo_sites, gitlab_get_geo_site.",
	},
}

func geoOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute geo domain action.", Tags: []string{"geo", "replication"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "geo",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
