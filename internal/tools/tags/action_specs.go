package tags

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionTagGet           = "tag.get"
	actionTagList          = "tag.list"
	actionTagUnprotect     = "tag.unprotect"
	actionTagProtect       = "tag.protect"
	actionTagListProtected = "tag.list_protected"
	paramTagName           = "tag_name"
)

// ActionSpecs returns canonical specs for tag and protected tag actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_tag_create — create a new tag from a branch, tag, or commit.
		tagSpec("create", toolutil.RouteAction(client, Create), "gitlab_tag_create", false, false),
		// gitlab_tag_get — fetch a single tag by name (returns a structured not-found result on 404).
		tagSpec("get", tagGetRoute(client), "gitlab_tag_get", true, true).
			WithEmbeddedResource("gitlab://project/{project_id}/tag/{tag_name}"),
		// gitlab_tag_list — list tags for a project with optional search and pagination.
		tagSpec("list", toolutil.RouteAction(client, List), "gitlab_tag_list", true, true),
		// gitlab_tag_delete — delete a tag (destructive).
		tagSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_tag_delete", false, true),
		// gitlab_tag_get_signature — fetch the X.509 signature attached to a tag.
		tagSpec("get_signature", toolutil.RouteAction(client, GetSignature), "gitlab_tag_get_signature", true, true),
		// gitlab_tag_list_protected — list protected tags for a project.
		tagSpec("list_protected", toolutil.RouteAction(client, ListProtectedTags), "gitlab_tag_list_protected", true, true),
		// gitlab_tag_get_protected — fetch a single protected tag by name.
		tagSpec("get_protected", toolutil.RouteAction(client, GetProtectedTag), "gitlab_tag_get_protected", true, true),
		// gitlab_tag_protect — protect a tag (or wildcard pattern) with create access levels.
		tagSpec("protect", toolutil.RouteAction(client, ProtectTag), "gitlab_tag_protect", false, false),
		// gitlab_tag_unprotect — remove protection from a tag (destructive).
		tagSpec("unprotect", toolutil.DestructiveVoidAction(client, UnprotectTag), "gitlab_tag_unprotect", false, true),
	}
}

// tagGetRoute wraps the canonical Get route to return a structured not-found output
// when GitLab responds with HTTP 404, mirroring the branches package behavior.
func tagGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	return toolutil.RouteAction(client, Get).WrapHandler(func(next toolutil.ActionFunc) toolutil.ActionFunc {
		return func(ctx context.Context, input map[string]any) (any, error) {
			result, err := next(ctx, input)
			if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
				tagName, _ := input[paramTagName].(string)
				projectID, _ := input["project_id"].(string)
				return tagNotFoundOutput{Identifier: fmt.Sprintf("%q in project %s", tagName, projectID)}, nil
			}
			return result, err
		}
	})
}

func tagSpec(name string, route toolutil.ActionRoute, individualTool string, readOnly, idempotent bool) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute tags domain action.", Tags: []string{"tag"},
		RelatedActions: []string{actionTagList, actionTagGet, "release.get", "repository.commit_get"},
		OpenWorld:      true,
		OwnerPackage:   "tags",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch name {
	case "list":
		options.Usage = "List tags in one project. Use this to discover release points, version tags, and candidates for release/tag workflows."
		options.Aliases = []string{"list tags", "show repository tags", "find tags"}
		options.RelatedActions = []string{actionTagGet, "release.list", "repository.compare"}
		options.IndividualTool.Description = "List tags in one project with optional search and offset or keyset pagination. Returns: matching tags with target, message, protection flag, the associated commit object, release note, and pagination metadata. See also: gitlab_tag_get, gitlab_tag_create, gitlab_release_list."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project ID or path containing tags.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
		}
		// Docs: https://docs.gitlab.com/api/tags/#list-project-repository-tags
		// SDK: ListTagsOptions.OrderBy *string (no consts); the API enforces
		// values %w[name updated version], default updated.
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("order_by", "name", "updated", "version"),
		}
	case "get":
		options.Usage = "Get one tag by project_id and tag_name. Use when a concrete tag is already known and detailed metadata/signature are needed."
		options.Aliases = []string{"get tag", "show tag details", "lookup tag"}
		options.RelatedActions = []string{actionTagList, "release.get", "tag.get_signature"}
		options.IndividualTool.Description = "Get a single tag from a project by name. Returns: the tag with target, message, protection flag, the full associated commit object, release note, and creation time. See also: gitlab_tag_list, gitlab_tag_get_signature, gitlab_release_get."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramTagName: {
				SemanticRole:   "git_tag",
				ValueSource:    "Tag string from task context or tag list output.",
				ExampleBinding: `params.tag_name:"v1.2.0"`,
			},
		}
	case "create":
		options.Usage = "Create a new tag for a project ref. Use message only when creating annotated tags or when task requires tag annotations."
		options.Aliases = []string{"create tag", "new git tag", "tag release"}
		options.RelatedActions = []string{"release.create", actionTagGet, "repository.compare"}
		options.IndividualTool.Description = "Create a new tag in a project pointing at a ref. Returns: the created tag with target, message, protection flag, and the associated commit object. See also: gitlab_tag_get, gitlab_tag_delete, gitlab_release_create."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramTagName: {
				SemanticRole:   "git_tag",
				ValueSource:    "Tag name requested for the release/version.",
				ExampleBinding: `params.tag_name:"v2.0.0"`,
			},
			"ref": {
				SemanticRole:     "git_ref",
				ValueSource:      "Branch, tag, or commit to tag.",
				ExampleBinding:   `params.ref:"main"`,
				CommonConfusions: []string{"Use ref for source revision. Do not pass project paths or URLs."},
			},
		}
	case "delete":
		options.Usage = "Delete a tag from a project by name. Destructive and not recoverable. Protected tags must be unprotected first."
		options.Aliases = []string{"delete tag", "remove tag", "drop tag"}
		options.RelatedActions = []string{actionTagList, actionTagGet, actionTagUnprotect}
		options.IndividualTool.Description = "Delete a tag from a project permanently. Returns: a success confirmation naming the tag and project. See also: gitlab_tag_get, gitlab_tag_list, gitlab_tag_unprotect."
	case "get_signature":
		options.Usage = "Get the X.509 signature attached to a tag. Use to verify tag provenance and certificate status."
		options.Aliases = []string{"get tag signature", "tag x509 signature", "verify tag signature"}
		options.RelatedActions = []string{actionTagGet, actionTagList}
		options.IndividualTool.Description = "Get the X.509 signature of a tag. Returns: the signature type, verification status, and the X.509 certificate with issuer detail. See also: gitlab_tag_get, gitlab_tag_list."
	case "list_protected":
		options.Usage = "List protected tags for a project with offset or keyset pagination. Use to audit which tag patterns are protected and their create access levels."
		options.Aliases = []string{"list protected tags", "show protected tags", "find protected tags"}
		options.RelatedActions = []string{"tag.get_protected", actionTagProtect, actionTagList}
		options.IndividualTool.Description = "List protected tags in one project with offset or keyset pagination. Returns: protected tag patterns with their create access levels and pagination metadata. See also: gitlab_tag_get_protected, gitlab_tag_protect, gitlab_tag_list."
	case "get_protected":
		options.Usage = "Get a single protected tag or wildcard rule by name. Use when a concrete protected pattern is already known."
		options.Aliases = []string{"get protected tag", "show protected tag", "lookup protected tag"}
		options.RelatedActions = []string{actionTagListProtected, actionTagProtect, actionTagUnprotect}
		options.IndividualTool.Description = "Get a single protected tag or wildcard rule by name. Returns: the protected tag with its create access levels (access level, user, group, and deploy key descriptions). See also: gitlab_tag_list_protected, gitlab_tag_protect, gitlab_tag_unprotect."
	case "protect":
		options.Usage = "Protect a tag or wildcard pattern with create access levels. Provide create_access_level for a simple rule, or allowed_to_create for granular user/group/deploy-key permissions."
		options.Aliases = []string{"protect tag", "add protected tag", "restrict tag creation"}
		options.RelatedActions = []string{actionTagUnprotect, actionTagListProtected, "tag.get_protected"}
		options.IndividualTool.Description = "Protect a tag or wildcard pattern with create access levels. Returns: the protected tag with its resolved create access levels. See also: gitlab_tag_unprotect, gitlab_tag_list_protected, gitlab_tag_get_protected."
	case "unprotect":
		options.Usage = "Remove protection from a tag or wildcard pattern by name. Destructive. Allows the matched tags to be created or deleted again."
		options.Aliases = []string{"unprotect tag", "remove tag protection", "delete protected tag"}
		options.RelatedActions = []string{actionTagProtect, actionTagListProtected, "tag.delete"}
		options.IndividualTool.Description = "Remove protection from a tag or wildcard pattern. Returns: a success confirmation naming the tag and project. See also: gitlab_tag_protect, gitlab_tag_list_protected, gitlab_tag_get_protected."
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
