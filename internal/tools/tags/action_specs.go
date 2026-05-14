package tags

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for tag and protected tag actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		tagSpec("create", toolutil.RouteAction(client, Create), "gitlab_tag_create", false),
		tagSpec("get", toolutil.RouteAction(client, Get), "gitlab_tag_get", true),
		tagSpec("list", toolutil.RouteAction(client, List), "gitlab_tag_list", true),
		tagSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_tag_delete", false),
		tagSpec("get_signature", toolutil.RouteAction(client, GetSignature), "gitlab_tag_get_signature", true),
		tagSpec("list_protected", toolutil.RouteAction(client, ListProtectedTags), "gitlab_tag_list_protected", true),
		tagSpec("get_protected", toolutil.RouteAction(client, GetProtectedTag), "gitlab_tag_get_protected", true),
		tagSpec("protect", toolutil.RouteAction(client, ProtectTag), "gitlab_tag_protect", false),
		tagSpec("unprotect", toolutil.DestructiveVoidAction(client, UnprotectTag), "gitlab_tag_unprotect", false),
	}
}

func tagSpec(name string, route toolutil.ActionRoute, individualTool string, readOnly bool) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"tag"},
		RelatedActions: []string{"tag.list", "tag.get", "release.get", "repository.commit_get"},
		ReadOnly:       readOnly,
		Idempotent:     readOnly,
		OpenWorld:      true,
		OwnerPackage:   "tags",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
