package projectiterations

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// IssueActionSpecs returns canonical specs for project iteration actions exposed through gitlab_issue.
func IssueActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		issueIterationReadSpec("iteration_list_project", toolutil.RouteAction(client, List), "gitlab_list_project_iterations", "projectiterations"),
	}
}

func issueIterationReadSpec(name string, route toolutil.ActionRoute, individualTool, ownerPackage string) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Tags:           []string{"issue", "iteration"},
		ReadOnly:       true,
		Idempotent:     true,
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   ownerPackage,
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	return toolutil.NewActionSpec(name, route, options)
}
