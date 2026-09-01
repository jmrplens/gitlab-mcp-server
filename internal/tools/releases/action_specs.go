package releases

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionReleaseGet      = "release.get"
	actionReleaseList     = "release.list"
	actionReleaseLinkList = "release_link.list"
	paramTagName          = "tag_name"
	roleGitTag            = "git_tag"
	hintReleaseTagName    = "Release tag name from list output or task context."
)

// ActionSpecs returns canonical specs for release actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		releaseCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_release_create"),
		releaseReadSpec("get", releaseGetRoute(client), "gitlab_release_get"),
		releaseReadSpec("get_latest", toolutil.RouteAction(client, GetLatest), "gitlab_release_latest"),
		releaseReadSpec("list", toolutil.RouteAction(client, List), "gitlab_release_list"),
		releaseUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_release_update"),
		releaseDeleteSpec("delete", toolutil.DestructiveAction(client, Delete), "gitlab_release_delete"),
	}
}

func releaseGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			tagName, _ := input[paramTagName].(string)
			return releaseNotFoundOutput{Identifier: fmt.Sprintf("tag %q in project %v", tagName, input["project_id"])}, nil
		}
		return result, err
	}
	return route
}

func releaseReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, releaseOptionsForAction(name, individualTool))
}

func releaseCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, releaseOptionsForAction(name, individualTool))
}

func releaseUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, releaseOptionsForAction(name, individualTool))
}

func releaseDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, releaseOptionsForAction(name, individualTool))
}

func releaseOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute releases domain action.", Tags: []string{"release", "tag", "asset"},
		RelatedActions: []string{"tag.get", "package.list", "project.milestone_list"},
		OpenWorld:      true,
		OwnerPackage:   "releases",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "list":
		options.Usage = "List releases for one project. Use this when the task asks for recent releases, release history, or release notes discovery."
		options.Aliases = []string{"list releases", "show project releases", "find releases"}
		options.RelatedActions = []string{actionReleaseGet, "tag.list", actionReleaseLinkList}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project ID or full path that owns releases.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
		}
		options.IndividualTool.Description = "List releases in a project with pagination. Returns: tag names, release names, release dates, and summary metadata. See also: gitlab_release_get, gitlab_tag_list, gitlab_release_link_list."
	case "get":
		options.Usage = "Get a release by project_id and tag_name. Use this when a specific tag is known and detailed release notes/assets are needed."
		options.Aliases = []string{"get release", "show release details", "lookup release"}
		options.RelatedActions = []string{actionReleaseList, "release.update", "release.delete", actionReleaseLinkList}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramTagName: {
				SemanticRole:     roleGitTag,
				ValueSource:      hintReleaseTagName,
				ExampleBinding:   `params.tag_name:"v1.2.0"`,
				CommonConfusions: []string{"Use tag_name for release lookup. Do not pass release title in this field."},
			},
		}
		options.IndividualTool.Description = "Get a single release by project_id and tag_name. Returns: release notes, author, commit, assets, milestones, evidences, and _links. See also: gitlab_release_list, gitlab_release_update, gitlab_release_delete, gitlab_release_link_list."
	case "get_latest":
		options.Usage = "Get the latest release for a project. Use this when the task asks for the most recent release without knowing the tag name."
		options.Aliases = []string{"latest release", "most recent release", "newest release"}
		options.RelatedActions = []string{actionReleaseList, actionReleaseGet, actionReleaseLinkList}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project ID or full path that owns releases.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
		}
		options.IndividualTool.Description = "Get the most recent release for a project. Returns: release notes, author, commit, assets, milestones, evidences, and _links. See also: gitlab_release_list, gitlab_release_get, gitlab_release_create."
	case "create":
		options.Usage = "Create a release for a tag (and optionally ref when creating a new tag). Use this for publishing release notes and lifecycle artifacts."
		options.Aliases = []string{"create release", "publish release", "new release"}
		options.RelatedActions = []string{"tag.create", "release_link.create", "repository.changelog_add"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramTagName: {
				SemanticRole:   roleGitTag,
				ValueSource:    "Release tag to publish.",
				ExampleBinding: `params.tag_name:"v2.0.0"`,
			},
			"ref": {
				SemanticRole:     "git_ref",
				ValueSource:      "Branch/tag/commit used to create tag when tag_name does not exist.",
				ExampleBinding:   `params.ref:"main"`,
				CommonConfusions: []string{"ref is only needed when creating a new tag. Omit it when releasing an existing tag."},
			},
		}
		options.IndividualTool.Description = "Create a release for a tag, optionally creating the tag from a ref and attaching asset links. Returns: the created release with author, commit, assets, milestones, and _links. See also: gitlab_release_update, gitlab_release_link_create, gitlab_tag_create."
	case "update":
		options.Usage = "Update a release identified by project_id and tag_name. Use this to edit the title, notes, release date, or associated milestones."
		options.Aliases = []string{"update release", "edit release", "modify release"}
		options.RelatedActions = []string{actionReleaseGet, actionReleaseList, "release_link.create", "release.delete"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramTagName: {
				SemanticRole:     roleGitTag,
				ValueSource:      hintReleaseTagName,
				ExampleBinding:   `params.tag_name:"v1.2.0"`,
				CommonConfusions: []string{"tag_name identifies the release to edit. It cannot be changed by this action."},
			},
		}
		options.IndividualTool.Description = "Update an existing release's title, notes, release date, or milestones. Returns: the updated release with author, commit, assets, milestones, and _links. See also: gitlab_release_get, gitlab_release_create, gitlab_release_delete."
	case "delete":
		options.Usage = "Delete a release identified by project_id and tag_name. Use this to remove a release record. The underlying Git tag is not deleted."
		options.Aliases = []string{"delete release", "remove release", "destroy release"}
		options.RelatedActions = []string{actionReleaseGet, actionReleaseList, "tag.delete"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramTagName: {
				SemanticRole:     roleGitTag,
				ValueSource:      hintReleaseTagName,
				ExampleBinding:   `params.tag_name:"v1.2.0"`,
				CommonConfusions: []string{"Deleting the release does not delete the Git tag. Use gitlab_tag_delete to remove the tag."},
			},
		}
		options.IndividualTool.Description = "Delete a release by tag_name (the Git tag itself is preserved). Returns: the deleted release details. See also: gitlab_release_get, gitlab_release_list, gitlab_tag_delete."
	}

	return options
}
