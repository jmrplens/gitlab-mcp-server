package securityfindings

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for security finding actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewActionSpec("list", toolutil.RouteAction(client, List), toolutil.ActionSpecOptions{
			Tags:           []string{"security", "finding"},
			ReadOnly:       true,
			Idempotent:     true,
			OpenWorld:      true,
			Edition:        "premium",
			OwnerPackage:   "securityfindings",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_list_security_findings", Title: toolutil.TitleFromName("gitlab_list_security_findings")},
		}),
	}
}
