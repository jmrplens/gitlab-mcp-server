package dependencyproxy

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for dependency proxy tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	options := dependencyProxyOptions("gitlab_purge_dependency_proxy")
	return []toolutil.ActionSpec{
		toolutil.NewDeleteActionSpec("dependency_proxy_delete", toolutil.DestructiveVoidAction(client, Purge), options),
	}
}

func dependencyProxyOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"dependency-proxy"},
		OpenWorld:      true,
		OwnerPackage:   "dependencyproxy",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
