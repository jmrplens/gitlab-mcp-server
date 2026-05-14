package dynamic

import (
	"fmt"
	"slices"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
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
func AddStandaloneRoutes(routes map[string]toolutil.ActionMap, client *gitlabclient.Client, opts StandaloneOptions) (map[string]toolutil.ActionMap, error) {
	catalog, err := AddStandaloneCatalog(actioncatalog.FromActionMaps(routes), client, opts)
	if err != nil {
		return nil, err
	}
	return catalog.ActionMaps(), nil
}

// AddStandaloneCatalog adds non-meta standalone tools to the canonical dynamic
// action catalog so dynamic mode can execute them without increasing the
// visible tool count.
func AddStandaloneCatalog(catalog *actioncatalog.Catalog, client *gitlabclient.Client, opts StandaloneOptions) (*actioncatalog.Catalog, error) {
	if catalog == nil {
		catalog = actioncatalog.NewCatalog()
	} else {
		catalog = catalog.Clone()
	}
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_discover_project") {
		group, groupErr := actioncatalog.GroupFromSpecs(actioncatalog.GroupOptions{
			ToolName:    "gitlab_discover_project",
			Description: "Resolve a full git remote URL to a GitLab project and return its project_id and metadata. Read-only; use only for complete git remote URLs from .git/config or git remote -v.",
			Icons:       toolutil.IconProject,
			ReadOnly:    true,
		}, projectdiscovery.ActionSpecs(client))
		if groupErr != nil {
			return nil, fmt.Errorf("build standalone dynamic group gitlab_discover_project: %w", groupErr)
		}
		if err := catalog.AddGroup(group); err != nil {
			return nil, fmt.Errorf("add standalone dynamic action gitlab_discover_project.resolve: %w", err)
		}
	}
	if opts.ReadOnly || standaloneExcluded(opts.ExcludeTools, "gitlab_interactive") {
		return catalog, nil
	}

	interactive := actioncatalog.NewGroup(actioncatalog.GroupOptions{
		ToolName:    "gitlab_interactive",
		Description: "Guided interactive creation flows for issues, merge requests, projects, and releases. Mutating; use only when the task explicitly asks for a guided flow.",
		Icons:       toolutil.IconServer,
	})
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_interactive_issue_create") {
		interactive.SetAction(actioncatalog.Action{Name: "issue_create", Route: toolutil.RouteActionWithRequest(client, elicitationtools.IssueCreate)})
	}
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_interactive_mr_create") {
		interactive.SetAction(actioncatalog.Action{Name: "mr_create", Route: toolutil.RouteActionWithRequest(client, elicitationtools.MRCreate)})
	}
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_interactive_project_create") {
		interactive.SetAction(actioncatalog.Action{Name: "project_create", Route: toolutil.RouteActionWithRequest(client, elicitationtools.ProjectCreate)})
	}
	if !standaloneExcluded(opts.ExcludeTools, "gitlab_interactive_release_create") {
		interactive.SetAction(actioncatalog.Action{Name: "release_create", Route: toolutil.RouteActionWithRequest(client, elicitationtools.ReleaseCreate)})
	}
	if len(interactive.Actions) > 0 {
		if err := catalog.AddGroup(interactive); err != nil {
			return nil, fmt.Errorf("add standalone dynamic group %q: %w", interactive.ToolName, err)
		}
	}
	return catalog, nil
}

func standaloneExcluded(excludeTools []string, name string) bool {
	return slices.Contains(excludeTools, name)
}
