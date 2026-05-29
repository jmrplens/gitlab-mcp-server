package modelregistry

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for Model Registry actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("download", toolutil.RouteAction(client, Download), toolutil.ActionSpecOptions{
			Aliases:        []string{"gitlab_download_ml_model_package"},
			Tags:           []string{"model_registry", "package", "download"},
			Usage:          "Download a file from a model package version in the project model registry.",
			RelatedActions: []string{"package.list", "release.link_create", "repository.raw_blob"},
			ParameterGuidance: map[string]toolutil.ParameterGuidance{
				"project_id": {
					SemanticRole:   "scope_project",
					ValueSource:    "Project ID or path that owns the model registry package.",
					ExampleBinding: `params.project_id:"group/project"`,
				},
				"model_version_id": {
					SemanticRole:   "model_version_id",
					ValueSource:    "Numeric model package version identifier.",
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
			OpenWorld:      true,
			OwnerPackage:   "modelregistry",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_download_ml_model_package", Title: toolutil.TitleFromName("gitlab_download_ml_model_package")},
		}),
	}
}
