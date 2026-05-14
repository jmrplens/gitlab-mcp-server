package accessrequests

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project and group access request actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		accessRequestReadSpec("request_list_project", toolutil.RouteAction(client, ListProject), "gitlab_access_request_list_project"),
		accessRequestReadSpec("request_list_group", toolutil.RouteAction(client, ListGroup), "gitlab_access_request_list_group"),
		accessRequestCreateSpec("request_project", toolutil.RouteAction(client, RequestProject), "gitlab_access_request_request_project"),
		accessRequestCreateSpec("request_group", toolutil.RouteAction(client, RequestGroup), "gitlab_access_request_request_group"),
		accessRequestUpdateSpec("approve_project", toolutil.RouteAction(client, ApproveProject), "gitlab_access_request_approve_project"),
		accessRequestUpdateSpec("approve_group", toolutil.RouteAction(client, ApproveGroup), "gitlab_access_request_approve_group"),
		accessRequestDeleteSpec("deny_project", toolutil.DestructiveVoidAction(client, DenyProject), "gitlab_access_request_deny_project"),
		accessRequestDeleteSpec("deny_group", toolutil.DestructiveVoidAction(client, DenyGroup), "gitlab_access_request_deny_group"),
	}
}

func accessRequestReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := accessRequestOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func accessRequestCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, accessRequestOptions(individualTool))
}

func accessRequestUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := accessRequestOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func accessRequestDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := accessRequestOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func accessRequestOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"access", "request"},
		OpenWorld:      true,
		OwnerPackage:   "accessrequests",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
