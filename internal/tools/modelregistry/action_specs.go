package modelregistry

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for Model Registry actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewActionSpec("download", toolutil.RouteAction(client, Download), toolutil.ActionSpecOptions{
			Tags:           []string{"model_registry", "package", "download"},
			RelatedActions: []string{"package.list", "release.link_create", "repository.raw_blob"},
			ReadOnly:       true,
			Idempotent:     true,
			OpenWorld:      true,
			OwnerPackage:   "modelregistry",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_download_ml_model_package", Title: toolutil.TitleFromName("gitlab_download_ml_model_package")},
		}),
	}
}
