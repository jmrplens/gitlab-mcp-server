package avatar

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for avatar lookup actions exposed through gitlab_user.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewActionSpec("avatar_get",
			toolutil.RouteAction(client, Get),
			toolutil.ActionSpecOptions{
				Tags:           []string{"user", "avatar"},
				ReadOnly:       true,
				Idempotent:     true,
				OpenWorld:      true,
				OwnerPackage:   "avatar",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_get_avatar", Title: toolutil.TitleFromName("gitlab_get_avatar")},
			}),
	}
}
