package tags

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionTagGet = "tag.get"

// ActionSpecs returns canonical specs for tag and protected tag actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		tagSpec("create", toolutil.RouteAction(client, Create), "gitlab_tag_create", false, false),
		tagSpec("get", tagGetRoute(client), "gitlab_tag_get", true, true),
		tagSpec("list", toolutil.RouteAction(client, List), "gitlab_tag_list", true, true),
		tagSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_tag_delete", false, true),
		tagSpec("get_signature", toolutil.RouteAction(client, GetSignature), "gitlab_tag_get_signature", true, true),
		tagSpec("list_protected", toolutil.RouteAction(client, ListProtectedTags), "gitlab_tag_list_protected", true, true),
		tagSpec("get_protected", toolutil.RouteAction(client, GetProtectedTag), "gitlab_tag_get_protected", true, true),
		tagSpec("protect", toolutil.RouteAction(client, ProtectTag), "gitlab_tag_protect", false, false),
		tagSpec("unprotect", toolutil.DestructiveVoidAction(client, UnprotectTag), "gitlab_tag_unprotect", false, true),
	}
}

func tagGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			tagName, _ := input["tag_name"].(string)
			projectID, _ := input["project_id"].(string)
			return tagNotFoundOutput{Identifier: fmt.Sprintf("%q in project %s", tagName, projectID)}, nil
		}
		return result, err
	}
	return route
}

func tagSpec(name string, route toolutil.ActionRoute, individualTool string, readOnly, idempotent bool) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute tags domain action.", Tags: []string{"tag"},
		RelatedActions: []string{"tag.list", actionTagGet, "release.get", "repository.commit_get"},
		OpenWorld:      true,
		OwnerPackage:   "tags",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch name {
	case "list":
		options.Usage = "List tags in one project. Use this to discover release points, version tags, and candidates for release/tag workflows."
		options.Aliases = []string{"list tags", "show repository tags", "find tags"}
		options.RelatedActions = []string{actionTagGet, "release.list", "repository.compare"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project ID or path containing tags.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
		}
	case "get":
		options.Usage = "Get one tag by project_id and tag_name. Use when a concrete tag is already known and detailed metadata/signature are needed."
		options.Aliases = []string{"get tag", "show tag details", "lookup tag"}
		options.RelatedActions = []string{"tag.list", "release.get", "tag.get_signature"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"tag_name": {
				SemanticRole:   "git_tag",
				ValueSource:    "Tag string from task context or tag list output.",
				ExampleBinding: `params.tag_name:"v1.2.0"`,
			},
		}
	case "create":
		options.Usage = "Create a new tag for a project ref. Use message only when creating annotated tags or when task requires tag annotations."
		options.Aliases = []string{"create tag", "new git tag", "tag release"}
		options.RelatedActions = []string{"release.create", actionTagGet, "repository.compare"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"tag_name": {
				SemanticRole:   "git_tag",
				ValueSource:    "Tag name requested for the release/version.",
				ExampleBinding: `params.tag_name:"v2.0.0"`,
			},
			"ref": {
				SemanticRole:     "git_ref",
				ValueSource:      "Branch, tag, or commit to tag.",
				ExampleBinding:   `params.ref:"main"`,
				CommonConfusions: []string{"Use ref for source revision; do not pass project paths or URLs."},
			},
		}
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
