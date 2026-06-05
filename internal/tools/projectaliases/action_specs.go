package projectaliases

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical cross-action IDs used by the dynamic catalog and surfaced
// via RelatedActions. Hoisted as constants so each case in
// projectAliasOptions can reference the same value (avoiding the
// `go:S1192` literal-duplication smell flagged by the SonarCloud scan
// on PR #158).
const (
	relatedProjectAliasList   = "project_alias.list"
	relatedProjectAliasGet    = "project_alias.get"
	relatedProjectAliasCreate = "project_alias.create"
	relatedProjectAliasDelete = "project_alias.delete"
)

// ActionSpecs returns canonical specs for project alias actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		projectAliasReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_project_aliases"),
		projectAliasReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_project_alias"),
		projectAliasCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_create_project_alias"),
		projectAliasDeleteSpec("delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_project_alias"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("project alias %q", input.Name))
	return out, nil
}

func projectAliasReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, projectAliasOptions(individualTool))
}

func projectAliasCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, projectAliasOptions(individualTool))
}

func projectAliasDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, projectAliasOptions(individualTool))
}

func projectAliasOptions(individualTool string) toolutil.ActionSpecOptions {
	usage := "Manage project aliases in a namespace."
	var related []string
	switch individualTool {
	case "gitlab_list_project_aliases":
		usage = "List project aliases visible in the configured scope (admin-only). The response includes the alias `name` and the `project_id` it points to; pass one of the returned `name` values to project_alias.get to fetch full details. This action does not accept per_page or page — it returns the full set."
		related = []string{relatedProjectAliasGet}
	case "gitlab_get_project_alias":
		usage = "Get details (id, project_id, name) for one project alias by its `name` (the path-style alias string, e.g. `e2e-enterprise-alias`). The name must come from a prior project_alias.list response or be supplied verbatim by the prompt — this action does not search or accept partial names."
		related = []string{relatedProjectAliasList}
	case "gitlab_create_project_alias":
		usage = "Create a new project alias that points to a target numeric project_id (not a project path). Admin-only."
		related = []string{relatedProjectAliasList, relatedProjectAliasDelete}
	case "gitlab_delete_project_alias":
		usage = "Delete a project alias by its `name`. The name must be an exact existing alias string; pass the name from a prior project_alias.list response."
		related = []string{relatedProjectAliasList, relatedProjectAliasGet}
	}

	return toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"project", "alias"},
		Usage:          usage,
		RelatedActions: related,
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "projectaliases",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
