package accessrequests

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
		accessRequestDeleteSpec("deny_project", toolutil.RouteAction(client, DenyProjectOutput), "gitlab_access_request_deny_project"),
		accessRequestDeleteSpec("deny_group", toolutil.RouteAction(client, DenyGroupOutput), "gitlab_access_request_deny_group"),
	}
}

// DenyProjectOutput denies a project access request and returns the legacy success message shape.
func DenyProjectOutput(ctx context.Context, client *gitlabclient.Client, input DenyProjectInput) (toolutil.DeleteOutput, error) {
	if err := DenyProject(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted project access request."}, nil
}

// DenyGroupOutput denies a group access request and returns the legacy success message shape.
func DenyGroupOutput(ctx context.Context, client *gitlabclient.Client, input DenyGroupInput) (toolutil.DeleteOutput, error) {
	if err := DenyGroup(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted group access request."}, nil
}

func accessRequestReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, accessRequestOptions(individualTool))
}

func accessRequestCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, accessRequestOptions(individualTool))
}

func accessRequestUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, accessRequestOptions(individualTool))
}

func accessRequestDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, accessRequestOptions(individualTool))
}

func accessRequestOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"access", "request"},
		OpenWorld:      true,
		OwnerPackage:   "accessrequests",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
