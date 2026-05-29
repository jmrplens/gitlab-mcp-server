package groups

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionGroupGet = "group.get"

// ActionSpecs returns canonical specs for core group and group hook actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupReadSpec("list", toolutil.RouteAction(client, List), "gitlab_group_list"),
		groupReadSpec("get", groupGetRoute(client), "gitlab_group_get"),
		groupCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_group_create"),
		groupUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_group_update"),
		groupDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_group_delete"),
		groupUpdateSpec("restore", toolutil.RouteAction(client, Restore), "gitlab_group_restore"),
		groupUpdateSpec("archive", toolutil.RouteAction(client, ArchiveOutput), "gitlab_group_archive"),
		groupUpdateSpec("unarchive", toolutil.RouteAction(client, UnarchiveOutput), "gitlab_group_unarchive"),
		groupReadSpec("search", toolutil.RouteAction(client, Search), "gitlab_group_search"),
		groupUpdateSpec("transfer_project", toolutil.RouteAction(client, TransferProject), "gitlab_group_transfer_project"),
		groupReadSpec("projects", toolutil.RouteAction(client, ListProjects), "gitlab_group_projects"),
		groupReadSpec("members", toolutil.RouteAction(client, MembersList), "gitlab_group_members_list"),
		groupReadSpec("subgroups", toolutil.RouteAction(client, SubgroupsList), "gitlab_subgroups_list"),
		groupReadSpec("hook_list", toolutil.RouteAction(client, ListHooks), "gitlab_group_hook_list"),
		groupReadSpec("hook_get", toolutil.RouteAction(client, GetHook), "gitlab_group_hook_get"),
		groupCreateSpec("hook_add", toolutil.RouteAction(client, AddHook), "gitlab_group_hook_add"),
		groupUpdateSpec("hook_edit", toolutil.RouteAction(client, EditHook), "gitlab_group_hook_edit"),
		groupDeleteSpec("hook_delete", toolutil.DestructiveVoidAction(client, DeleteHook), "gitlab_group_hook_delete"),
	}
}

func groupGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return groupNotFoundOutput{Identifier: fmt.Sprint(input["group_id"])}, nil
		}
		return result, err
	}
	return route
}

// ArchiveOutput archives a GitLab group and returns the legacy success message shape.
func ArchiveOutput(ctx context.Context, client *gitlabclient.Client, input ArchiveInput) (toolutil.DeleteOutput, error) {
	if err := Archive(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Group %s archived successfully", input.GroupID)}, nil
}

// UnarchiveOutput unarchives a GitLab group and returns the legacy success message shape.
func UnarchiveOutput(ctx context.Context, client *gitlabclient.Client, input ArchiveInput) (toolutil.DeleteOutput, error) {
	if err := Unarchive(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Group %s unarchived successfully", input.GroupID)}, nil
}

func groupReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupOptionsForAction(name, individualTool))
}

func groupCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupOptionsForAction(name, individualTool))
}

func groupUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupOptionsForAction(name, individualTool))
}

func groupDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupOptionsForAction(name, individualTool))
}

func groupOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	_ = actionName

	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groups domain action.", Tags: []string{"group"},
		OpenWorld:      true,
		OwnerPackage:   "groups",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch individualTool {
	case "gitlab_group_get":
		options.Usage = "Get one exact group by group_id (numeric ID or full path). Use this when the prompt already targets a specific group and needs metadata such as visibility, parent, web URL, or statistics."
		options.Aliases = []string{"get group", "show group details", "lookup group by path"}
		options.RelatedActions = []string{"group.list", "group.members", "group.projects", "group.update"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Group numeric ID or full path from the prompt or prior discovery step.",
				ExampleBinding:   `params.group_id:"my-org/platform"`,
				CommonConfusions: []string{"Use group_id for path or ID; do not send project_id for group lookups."},
			},
		}
		options.IndividualTool.Description = "Get one GitLab group by ID or path. Returns: group metadata, visibility, parent information, and web URL. See also: gitlab_group_list, gitlab_group_members_list, gitlab_group_projects, gitlab_group_update."
	case "gitlab_group_list":
		options.Usage = "List groups visible to the authenticated user. Use search, owned, min_access_level, and pagination when the user asks for matching or accessible groups."
		options.Aliases = []string{"list groups", "show visible groups", "find groups"}
		options.RelatedActions = []string{actionGroupGet, "group.search", "group.create"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"search": {
				ValueSource:      "Group name/path keywords from the user query.",
				ExampleBinding:   `params.search:"platform"`,
				CommonConfusions: []string{"search filters visible groups; it does not accept project paths."},
			},
		}
		options.IndividualTool.Description = "List accessible GitLab groups with filtering and pagination. Returns: matching groups including path, name, and visibility metadata. See also: gitlab_group_get, gitlab_group_search, gitlab_group_create."
	case "gitlab_group_create":
		options.Usage = "Create a group with name and path. Optionally set parent_id, description, visibility, and project creation permissions when requested."
		options.Aliases = []string{"create group", "create subgroup", "new group"}
		options.RelatedActions = []string{actionGroupGet, "group.update", "group.delete"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"name": {
				SemanticRole:   "group_name",
				ValueSource:    "Human-readable group display name from user intent.",
				ExampleBinding: `params.name:"Platform Team"`,
			},
			"path": {
				SemanticRole:     "group_path_segment",
				ValueSource:      "URL-safe group path segment.",
				ExampleBinding:   `params.path:"platform-team"`,
				CommonConfusions: []string{"path is a slug segment, not a full URL or namespace with slashes unless creating nested groups via parent_id."},
			},
		}
		options.IndividualTool.Description = "Create a GitLab group or subgroup. Returns: created group metadata including ID, full path, and visibility. See also: gitlab_group_get, gitlab_group_update, gitlab_group_delete."
	case "gitlab_group_members_list":
		options.RelatedActions = []string{actionGroupGet, "group.projects", "group.member_add"}
	}

	return options
}
