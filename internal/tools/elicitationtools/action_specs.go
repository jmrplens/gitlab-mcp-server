package elicitationtools

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for standalone interactive elicitation actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		interactiveCreateSpec("issue_create", toolutil.RouteActionWithRequest(client, IssueCreate), "gitlab_interactive_issue_create", "Guided issue creation through MCP elicitation with explicit user confirmation."),
		interactiveCreateSpec("mr_create", toolutil.RouteActionWithRequest(client, MRCreate), "gitlab_interactive_mr_create", "Guided merge request creation through MCP elicitation with explicit user confirmation."),
		interactiveCreateSpec("project_create", toolutil.RouteActionWithRequest(client, ProjectCreate), "gitlab_interactive_project_create", "Guided project creation through MCP elicitation with explicit user confirmation."),
		interactiveCreateSpec("release_create", toolutil.RouteActionWithRequest(client, ReleaseCreate), "gitlab_interactive_release_create", "Guided release creation through MCP elicitation with explicit user confirmation."),
	}
}

func interactiveCreateSpec(name string, route toolutil.ActionRoute, individualTool, usage string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"interactive", "elicitation"},
		Usage:          usage,
		OpenWorld:      true,
		OwnerPackage:   "elicitationtools",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
