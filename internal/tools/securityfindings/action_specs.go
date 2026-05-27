package securityfindings

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for security finding actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("list", toolutil.RouteAction(client, List), toolutil.ActionSpecOptions{
			Aliases: []string{"gitlab_list_security_findings"}, Usage: "Use to execute list action.", Tags: []string{"security", "finding"},
			OpenWorld:      true,
			Edition:        "premium",
			OwnerPackage:   "securityfindings",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_list_security_findings", Title: toolutil.TitleFromName("gitlab_list_security_findings")},
		}),
	}
}
