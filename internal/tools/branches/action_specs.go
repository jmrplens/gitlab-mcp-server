package branches

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionBranchList = "branch.list"

// ActionSpecs returns canonical specs for branch and protected branch actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		branchSpec("create", toolutil.RouteAction(client, Create), "gitlab_branch_create", false, false),
		branchSpec("get", branchGetRoute(client), "gitlab_branch_get", true, true),
		branchSpec("list", toolutil.RouteAction(client, List), "gitlab_branch_list", true, true),
		branchSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_branch_delete", false, true),
		branchSpec("delete_merged", toolutil.DestructiveVoidAction(client, DeleteMerged), "gitlab_branch_delete_merged", false, true),
		branchSpec("protect", toolutil.RouteAction(client, Protect), "gitlab_branch_protect", false, true),
		branchSpec("unprotect", toolutil.DestructiveAction(client, Unprotect), "gitlab_branch_unprotect", false, true),
		branchSpec("list_protected", toolutil.RouteAction(client, ProtectedList), "gitlab_protected_branches_list", true, true),
		branchSpec("get_protected", toolutil.RouteAction(client, ProtectedGet), "gitlab_protected_branch_get", true, true),
		branchSpec("update_protected", toolutil.RouteAction(client, ProtectedUpdate), "gitlab_protected_branch_update", false, true),
	}
}

func branchGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			branchName, _ := input["branch_name"].(string)
			projectID, _ := input["project_id"].(string)
			return branchNotFoundOutput{Identifier: fmt.Sprintf("%q in project %s", branchName, projectID)}, nil
		}
		return result, err
	}
	return route
}

func branchSpec(name string, route toolutil.ActionRoute, individualTool string, readOnly, idempotent bool) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute branches domain action.", Tags: []string{"branch"},
		RelatedActions: []string{actionBranchList, "branch.get", "repository.tree", "merge_request.create"},
		OpenWorld:      true,
		OwnerPackage:   "branches",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch name {
	case "list":
		options.Usage = "List repository branches for one project with optional search and pagination. Use this before branch-level operations and when selecting refs for compare/MR flows."
		options.Aliases = []string{"list branches", "show project branches", "find branches"}
		options.RelatedActions = []string{"branch.get", "branch.create", "repository.compare"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project numeric ID or full path containing branches.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
		}
	case "get":
		options.Usage = "Get one branch by project_id and branch_name. Use when a specific branch is referenced and exact branch protection/default metadata is needed."
		options.Aliases = []string{"get branch", "show branch details", "lookup branch"}
		options.RelatedActions = []string{actionBranchList, "branch.protect", "branch.unprotect"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"branch_name": {
				SemanticRole:   "git_branch",
				ValueSource:    "Branch name from task context or branch list output.",
				ExampleBinding: `params.branch_name:"main"`,
			},
		}
	case "create":
		options.Usage = "Create a branch from a source ref (branch/tag/commit). Use when preparing feature branches or release branches."
		options.Aliases = []string{"create branch", "new branch", "branch from ref"}
		options.RelatedActions = []string{actionBranchList, "merge_request.create", "repository.compare"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"branch_name": {
				SemanticRole:   "git_branch",
				ValueSource:    "Name for the new branch to create.",
				ExampleBinding: `params.branch_name:"feature/new-api"`,
			},
			"ref": {
				SemanticRole:   "git_ref",
				ValueSource:    "Existing branch/tag/commit used as creation source.",
				ExampleBinding: `params.ref:"main"`,
			},
		}
	case "delete":
		options.Usage = "Delete one branch by name. Use only for confirmed cleanup tasks when the branch is no longer needed."
		options.Aliases = []string{"delete branch", "remove branch", "drop branch"}
		options.RelatedActions = []string{actionBranchList, "branch.unprotect"}
	case "delete_merged":
		options.Usage = "Delete merged branches in a project. Use with caution for branch hygiene after confirming merge status requirements."
		options.Aliases = []string{"delete merged branches", "cleanup merged branches", "prune merged branches"}
		options.RelatedActions = []string{actionBranchList, "merge_request.list"}
	}

	if name == "protect" {
		options = branchProtectOptions(options)
	}
	switch {
	case readOnly:
		return toolutil.NewReadActionSpec(name, route, options)
	case route.Destructive && idempotent:
		return toolutil.NewDeleteActionSpec(name, route, options)
	case idempotent:
		return toolutil.NewUpdateActionSpec(name, route, options)
	default:
		return toolutil.NewCreateActionSpec(name, route, options)
	}
}

func branchProtectOptions(options toolutil.ActionSpecOptions) toolutil.ActionSpecOptions {
	options.Tags = append(options.Tags, "protected_branch", "access_level")
	options.Usage = "Protect a branch and set branch protection access levels. Use numeric integers for access levels: 0 means No access, 30 means Developer, and 40 means Maintainer."
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"push_access_level":  branchProtectionAccessLevelGuidance("push"),
		"merge_access_level": branchProtectionAccessLevelGuidance("merge"),
	}
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		branchProtectionAccessLevelSchema("push_access_level", "Access level for push: 0=No access, 30=Developer, 40=Maintainer. Use an integer."),
		branchProtectionAccessLevelSchema("merge_access_level", "Access level for merge: 0=No access, 30=Developer, 40=Maintainer. Use an integer."),
	}
	return options
}

func branchProtectionAccessLevelGuidance(operation string) toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole: "branch_protection_" + operation + "_access_level",
		ValueSource:  "Use integer access levels only: 0 for No access, 30 for Developer, 40 for Maintainer.",
		CommonConfusions: []string{
			"Do not send labels such as maintainer or developers when an integer is possible.",
		},
	}
}

func branchProtectionAccessLevelSchema(name, description string) toolutil.InputSchemaOverride {
	return toolutil.SchemaPropertyOverride(name, map[string]any{
		"description": description,
		"enum":        []any{0, 30, 40},
	})
}
