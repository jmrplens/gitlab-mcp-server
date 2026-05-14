package projectdiscovery

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for standalone project discovery actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewActionSpec("resolve", toolutil.RouteAction(client, Resolve), toolutil.ActionSpecOptions{
			Tags:           []string{"discovery", "project"},
			Usage:          "Resolve a complete git remote URL from .git/config or git remote -v to GitLab project metadata.",
			ReadOnly:       true,
			Idempotent:     true,
			OpenWorld:      true,
			OwnerPackage:   "projectdiscovery",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_discover_project", Title: toolutil.TitleFromName("gitlab_discover_project")},
		}),
	}
}
