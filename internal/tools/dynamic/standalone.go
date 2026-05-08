package dynamic

import (
	"slices"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actionregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/elicitationtools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectdiscovery"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// StandaloneOptions controls which standalone tools are added to the canonical
// dynamic action catalog.
type StandaloneOptions struct {
	ReadOnly     bool
	ExcludeTools []string
}

// AddStandaloneRoutes adds non-meta standalone tools to the canonical dynamic
// action catalog so dynamic mode can still execute them through
// gitlab_execute_tool without increasing the visible tool count.
func AddStandaloneRoutes(routes map[string]toolutil.ActionMap, client *gitlabclient.Client, opts StandaloneOptions) map[string]toolutil.ActionMap {
	return AddStandaloneCatalog(actionregistry.FromActionMaps(routes), client, opts).ActionMaps()
}

// AddStandaloneCatalog adds non-meta standalone tools to the canonical dynamic
// action catalog so dynamic mode can execute them without increasing the
// visible tool count.
func AddStandaloneCatalog(catalog *actionregistry.Catalog, client *gitlabclient.Client, opts StandaloneOptions) *actionregistry.Catalog {
	if catalog == nil {
		catalog = actionregistry.NewCatalog()
	} else {
		catalog = catalog.Clone()
	}
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_discover_project") {
		_ = catalog.AddAction("gitlab_discover_project", actionregistry.Action{
			Name:  "resolve",
			Route: toolutil.RouteAction(client, projectdiscovery.Resolve),
		})
	}
	if opts.ReadOnly || standaloneExcluded(opts.ExcludeTools, "gitlab_interactive") {
		return catalog
	}

	interactive := actionregistry.NewGroup(actionregistry.GroupOptions{ToolName: "gitlab_interactive"})
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_interactive_issue_create") {
		interactive.SetAction(actionregistry.Action{Name: "issue_create", Route: toolutil.RouteActionWithRequest(client, elicitationtools.IssueCreate)})
	}
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_interactive_mr_create") {
		interactive.SetAction(actionregistry.Action{Name: "mr_create", Route: toolutil.RouteActionWithRequest(client, elicitationtools.MRCreate)})
	}
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_interactive_project_create") {
		interactive.SetAction(actionregistry.Action{Name: "project_create", Route: toolutil.RouteActionWithRequest(client, elicitationtools.ProjectCreate)})
	}
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_interactive_release_create") {
		interactive.SetAction(actionregistry.Action{Name: "release_create", Route: toolutil.RouteActionWithRequest(client, elicitationtools.ReleaseCreate)})
	}
	if len(interactive.Actions) > 0 {
		_ = catalog.AddGroup(interactive)
	}
	return catalog
}

func standaloneExcluded(excludeTools []string, name string) bool {
	return slices.Contains(excludeTools, name)
}
