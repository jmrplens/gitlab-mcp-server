package wikis

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project wiki actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		wikiSpec("list", toolutil.RouteAction(client, List), "gitlab_wiki_list", true),
		wikiSpec("get", toolutil.RouteAction(client, Get), "gitlab_wiki_get", true),
		wikiSpec("create", toolutil.RouteAction(client, Create), "gitlab_wiki_create", false),
		wikiSpec("update", toolutil.RouteAction(client, Update), "gitlab_wiki_update", false),
		wikiSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_wiki_delete", false),
		wikiSpec("upload_attachment", toolutil.RouteAction(client, UploadAttachment), "gitlab_wiki_upload_attachment", false),
	}
}

func wikiSpec(name string, route toolutil.ActionRoute, individualTool string, readOnly bool) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"wiki"},
		RelatedActions: []string{"wiki.list", "wiki.get", "project.get", "repository.file_get"},
		ReadOnly:       readOnly,
		Idempotent:     readOnly,
		OpenWorld:      true,
		OwnerPackage:   "wikis",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
