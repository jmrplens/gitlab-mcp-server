package groupimportexport

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs referenced by group import/export discovery metadata.
const (
	actionGroupExportSchedule = "group_export_schedule"
	actionGroupExportDownload = "group_export_download"
	actionGroupImportFile     = "group_import_file"
	actionGroupGet            = "group.get"
	actionGroupList           = "group.list"
)

// ActionSpecs returns canonical specs for group import and export actions.
//
// The three routes mirror the client-go GroupImportExportService one-to-one:
// ScheduleExport (POST groups/{id}/export), ExportDownload
// (GET groups/{id}/export/download), and ImportFile (POST groups/import). Each
// carries non-generic discovery metadata (Usage, Aliases, RelatedActions, and a
// "Returns: … See also: …" individual-tool description) so neither the dynamic,
// meta, nor individual surface inherits placeholder text (1:1 audit R-META).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_schedule_group_export — start an asynchronous export of a group.
		groupImportExportCreateSpec(actionGroupExportSchedule, toolutil.RouteAction(client, ScheduleExport), "gitlab_schedule_group_export"),
		// gitlab_download_group_export — download the finished export archive.
		groupImportExportReadSpec(actionGroupExportDownload, toolutil.RouteAction(client, ExportDownload), "gitlab_download_group_export"),
		// gitlab_import_group_from_file — import a group from a local export archive.
		groupImportExportCreateSpec(actionGroupImportFile, toolutil.RouteAction(client, ImportFile), "gitlab_import_group_from_file"),
	}
}

// groupImportExportReadSpec builds a read-only [toolutil.ActionSpec] for a
// group import/export action and decorates it with action-specific discovery
// metadata.
func groupImportExportReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupImportExportOptions(individualTool)
	decorateGroupImportExportMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

// groupImportExportCreateSpec builds a create-style [toolutil.ActionSpec] for a
// group import/export action and decorates it with action-specific discovery
// metadata.
func groupImportExportCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupImportExportOptions(individualTool)
	decorateGroupImportExportMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

// groupImportExportOptions returns the base [toolutil.ActionSpecOptions] shared
// by every group import/export action (tags, owner, individual tool metadata).
func groupImportExportOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupimportexport domain action.", Tags: []string{"group", "import", "export"},
		RelatedActions: []string{actionGroupGet},
		OpenWorld:      true,
		OwnerPackage:   "groupimportexport",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// decorateGroupImportExportMeta fills non-generic Usage, distinctive
// natural-language Aliases, canonical group.* RelatedActions, and the
// "Returns: … See also: …" individual-tool description for each group
// import/export tool. It is a no-op for unknown tools so the generic base
// metadata from [groupImportExportOptions] is preserved (1:1 audit R-META).
func decorateGroupImportExportMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := groupImportExportActionMeta[individualTool]
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

// groupImportExportActionMetaEntry is the discovery metadata for one group
// import/export action.
type groupImportExportActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// groupImportExportActionMeta maps each individual group import/export tool to
// its discovery metadata.
var groupImportExportActionMeta = map[string]groupImportExportActionMetaEntry{
	"gitlab_schedule_group_export": {
		usage:       "Start an asynchronous export of a whole group (subgroups, projects, and metadata) into a downloadable archive. Use this first, then poll and download with group_export_download once GitLab finishes building the archive.",
		aliases:     []string{"export a group", "schedule group export", "back up a group", "start group export"},
		related:     []string{actionGroupExportDownload, actionGroupImportFile, actionGroupGet},
		description: "Schedule an asynchronous export of a GitLab group. Returns: a confirmation that the export was scheduled (the archive is built in the background). See also: gitlab_download_group_export, gitlab_import_group_from_file, gitlab_group_get.",
	},
	"gitlab_download_group_export": {
		usage:       "Download the finished export archive for a group as a base64-encoded .tar.gz. Use this only after group_export_schedule has completed; the call fails until the archive is ready.",
		aliases:     []string{"download group export", "fetch group archive", "get group export file", "retrieve group export"},
		related:     []string{actionGroupExportSchedule, actionGroupImportFile, actionGroupGet},
		description: "Download a group's finished export archive. Returns: the export archive as base64 content plus its size in bytes. See also: gitlab_schedule_group_export, gitlab_import_group_from_file, gitlab_group_get.",
	},
	"gitlab_import_group_from_file": {
		usage:       "Import a new group from a local export archive (.tar.gz) produced by group_export_schedule. Provide a name, URL path, the archive file, and optionally a parent_id to nest the imported group under an existing one.",
		aliases:     []string{"import a group", "restore a group from archive", "upload group export", "create group from export file"},
		related:     []string{actionGroupExportSchedule, actionGroupExportDownload, actionGroupList},
		description: "Import a group from a local export archive. Returns: a confirmation that the import started (GitLab builds the group asynchronously). See also: gitlab_schedule_group_export, gitlab_download_group_export, gitlab_group_list.",
	},
}
