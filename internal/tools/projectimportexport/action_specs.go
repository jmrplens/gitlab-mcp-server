package projectimportexport

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project import and export actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		projectImportExportCreateSpec("export_schedule", toolutil.RouteAction(client, ScheduleExport), "gitlab_schedule_project_export"),
		projectImportExportReadSpec("export_status", toolutil.RouteAction(client, GetExportStatus), "gitlab_get_project_export_status"),
		projectImportExportReadSpec("export_download", toolutil.RouteAction(client, ExportDownload), "gitlab_download_project_export"),
		projectImportExportCreateSpec("import_from_file", toolutil.RouteAction(client, ImportFromFile), "gitlab_import_project_from_file"),
		projectImportExportReadSpec("import_status", toolutil.RouteAction(client, GetImportStatus), "gitlab_get_project_import_status"),
	}
}

func projectImportExportReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, projectImportExportOptions(individualTool))
}

func projectImportExportCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, projectImportExportOptions(individualTool))
}

// projectIDGuidance is the shared parameter guidance for the project_id input
// used by every project import/export action.
func projectIDGuidance() map[string]toolutil.ParameterGuidance {
	return map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole:     "scope_project",
			ValueSource:      "Project ID or URL-encoded full path of the project being exported or imported.",
			ExampleBinding:   `params.project_id:"group/project"`,
			CommonConfusions: []string{"Use the source project for export status/download. For import_status use the newly created target project, not the archive."},
		},
	}
}

func projectImportExportOptions(individualTool string) toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{
		Tags:           []string{"project", "import", "export"},
		OpenWorld:      true,
		OwnerPackage:   "projectimportexport",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	meta := projectImportExportMeta[individualTool]
	opts.Usage = meta.usage
	opts.Aliases = append([]string{individualTool}, meta.aliases...)
	opts.RelatedActions = meta.related
	opts.ParameterGuidance = meta.guidance
	opts.IndividualTool.Description = meta.description
	return opts
}

// projectImportExportMetaEntry is the discovery metadata for one project
// import/export action.
type projectImportExportMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	guidance    map[string]toolutil.ParameterGuidance
	description string
}

// Canonical action IDs used for cross-linking related actions.
const (
	actionExportSchedule = "projectimportexport.export_schedule"
	actionExportStatus   = "projectimportexport.export_status"
	actionExportDownload = "projectimportexport.export_download"
	actionImportFromFile = "projectimportexport.import_from_file"
	actionImportStatus   = "projectimportexport.import_status"
	actionProjectGet     = "project.get"
)

// projectImportExportMeta maps each individual tool to its discovery metadata
// (R-META: action-specific usage, natural-language aliases, canonical related
// actions, parameter guidance, and a "Returns: … See also: …" description).
var projectImportExportMeta = map[string]projectImportExportMetaEntry{
	"gitlab_schedule_project_export": {
		usage:    "Schedule an asynchronous export archive for a project. Optionally upload the archive to a URL when it is ready. Then poll gitlab_get_project_export_status until 'finished' and download with gitlab_download_project_export.",
		aliases:  []string{"export project", "schedule project export", "create project export archive", "back up project"},
		related:  []string{actionExportStatus, actionExportDownload, actionProjectGet},
		guidance: projectIDGuidance(),
		description: "Schedule an asynchronous project export. Returns: a confirmation message. " +
			"See also: gitlab_get_project_export_status, gitlab_download_project_export.",
	},
	"gitlab_get_project_export_status": {
		usage:    "Get the current export status of a project. Use after gitlab_schedule_project_export to poll until export_status is 'finished'. Status values are none, started, finished, regeneration_in_progress.",
		aliases:  []string{"export status", "check project export progress", "is project export ready"},
		related:  []string{actionExportSchedule, actionExportDownload, actionProjectGet},
		guidance: projectIDGuidance(),
		description: "Get the export status of a project. Returns: export id, name, path, status, message, and download links. " +
			"See also: gitlab_schedule_project_export, gitlab_download_project_export.",
	},
	"gitlab_download_project_export": {
		usage:    "Download the generated export archive (.tar.gz) of a project as base64 once the export status is 'finished'. Archives are retained for 24 hours after generation.",
		aliases:  []string{"download project export", "get project export archive", "fetch export tarball"},
		related:  []string{actionExportStatus, actionExportSchedule, actionImportFromFile},
		guidance: projectIDGuidance(),
		description: "Download a finished project export archive as base64. Returns: base64 archive content and size in bytes. " +
			"See also: gitlab_get_project_export_status, gitlab_import_project_from_file.",
	},
	"gitlab_import_project_from_file": {
		usage:   "Import a project from a GitLab export archive (.tar.gz) supplied as a local file path or base64 content. Optionally override the namespace, name, path, and project attributes via override_params. Poll gitlab_get_project_import_status afterward.",
		aliases: []string{"import project", "import project from file", "restore project from export", "upload project archive"},
		related: []string{actionImportStatus, actionExportDownload, actionProjectGet},
		guidance: map[string]toolutil.ParameterGuidance{
			"path": {
				SemanticRole:     "new_project_path",
				ValueSource:      "URL-safe path (slug) for the imported project.",
				ExampleBinding:   `params.path:"imported-project"`,
				CommonConfusions: []string{"This names the NEW project. The source archive is supplied via file_path or content_base."},
			},
			"namespace": {
				SemanticRole:     "target_namespace",
				ValueSource:      "Group/user namespace (id or full path) to import into. Defaults to the caller's namespace when omitted.",
				ExampleBinding:   `params.namespace:"my-group"`,
				CommonConfusions: []string{"Omit to import into your own namespace."},
			},
			"file_path": {
				SemanticRole:     "archive_source",
				ValueSource:      "Local filesystem path to the .tar.gz export archive (alternative to content_base).",
				ExampleBinding:   `params.file_path:"/tmp/project-export.tar.gz"`,
				CommonConfusions: []string{"Supply exactly one of file_path or content_base."},
			},
		},
		description: "Import a project from an export archive. Returns: the new project's import status, type, correlation id, and any import error. " +
			"See also: gitlab_get_project_import_status, gitlab_download_project_export.",
	},
	"gitlab_get_project_import_status": {
		usage:    "Get the import status of a project created by gitlab_import_project_from_file. Poll until import_status is 'finished' or 'failed'. Inspect import_error on failure. Status values are none, scheduled, started, finished, failed.",
		aliases:  []string{"import status", "check project import progress", "did project import succeed"},
		related:  []string{actionImportFromFile, actionProjectGet},
		guidance: projectIDGuidance(),
		description: "Get the import status of a project. Returns: import id, name, path, status, type, correlation id, and import error. " +
			"See also: gitlab_import_project_from_file.",
	},
}
