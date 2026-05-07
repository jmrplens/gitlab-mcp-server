package dynamic

import (
	"slices"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/elicitationtools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectdiscovery"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// StandaloneOptions controls which standalone tools are added to the hidden
// dynamic action catalog.
type StandaloneOptions struct {
	ReadOnly     bool
	ExcludeTools []string
}

// AddStandaloneRoutes adds non-meta standalone tools to the hidden dynamic
// action catalog so dynamic mode can still execute them through
// gitlab_execute_tool without increasing the visible tool count.
func AddStandaloneRoutes(routes map[string]toolutil.ActionMap, client *gitlabclient.Client, opts StandaloneOptions) map[string]toolutil.ActionMap {
	if routes == nil {
		routes = make(map[string]toolutil.ActionMap)
	}

	if !standaloneExcluded(opts.ExcludeTools, "gitlab_discover_project") {
		routes["gitlab_discover_project"] = toolutil.ActionMap{
			"resolve": toolutil.RouteAction(client, projectdiscovery.Resolve),
		}
	}
	if opts.ReadOnly || standaloneExcluded(opts.ExcludeTools, "gitlab_interactive") {
		return routes
	}

	interactive := make(toolutil.ActionMap)
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_interactive_issue_create") {
		interactive["issue_create"] = toolutil.RouteActionWithRequest(client, elicitationtools.IssueCreate)
	}
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_interactive_mr_create") {
		interactive["mr_create"] = toolutil.RouteActionWithRequest(client, elicitationtools.MRCreate)
	}
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_interactive_project_create") {
		interactive["project_create"] = toolutil.RouteActionWithRequest(client, elicitationtools.ProjectCreate)
	}
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_interactive_release_create") {
		interactive["release_create"] = toolutil.RouteActionWithRequest(client, elicitationtools.ReleaseCreate)
	}
	if len(interactive) > 0 {
		routes["gitlab_interactive"] = interactive
	}
	return routes
}

func standaloneExcluded(excludeTools []string, name string) bool {
	return slices.Contains(excludeTools, name)
}
