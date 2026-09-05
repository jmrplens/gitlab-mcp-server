package modelregistry

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for Model Registry actions
// exposed as MCP tools. The download route is projected into the
// dynamic, meta, individual, and audit surfaces by the action catalog
// (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_download_ml_model_package — download a file from a model package version.
		toolutil.NewReadActionSpec("download", toolutil.RouteAction(client, Download), toolutil.ActionSpecOptions{
			Aliases: []string{
				"gitlab_download_ml_model_package",
				"download ml model artifact",
				"fetch machine learning model file",
				"get model registry package file",
				"download model version artifact",
				"pull ml model binary",
			},
			Tags:           []string{"model_registry", "package", "download"},
			Usage:          "Download a single artifact file from a model package version in the project ML model registry, returning the raw bytes base64-encoded.",
			RelatedActions: []string{"package.list", "release.link_create", "repository.raw_blob"},
			ParameterGuidance: map[string]toolutil.ParameterGuidance{
				"project_id": {
					SemanticRole:   "scope_project",
					ValueSource:    "Project ID or path that owns the model registry package.",
					ExampleBinding: `params.project_id:"group/project"`,
				},
				"model_version_id": {
					SemanticRole:   "model_version_id",
					ValueSource:    "Numeric model version ID from the model version URL, or a candidate run ID prefixed with candidate: such as candidate:5.",
					ExampleBinding: `params.model_version_id:"7"`,
				},
				"path": {
					SemanticRole:   "model_artifact_path",
					ValueSource:    "Subdirectory path in the package where the artifact is stored.",
					ExampleBinding: `params.path:"models"`,
				},
				"filename": {
					SemanticRole:   "model_artifact_filename",
					ValueSource:    "Artifact file name to download from the package.",
					ExampleBinding: `params.filename:"model.bin"`,
				},
			},
			OpenWorld:    true,
			OwnerPackage: "modelregistry",
			IndividualTool: toolutil.IndividualToolSpec{
				Name:        "gitlab_download_ml_model_package",
				Title:       toolutil.TitleFromName("gitlab_download_ml_model_package"),
				Description: "Download one artifact file from a project ML model registry package version by project, model version, path, and filename. Returns: the artifact bytes base64-encoded in content_base64, plus project, model version, path, filename, and decoded size_bytes. See also: gitlab_package_list, gitlab_release_link_create, gitlab_repository_raw_blob.",
			},
		}),
	}
}
