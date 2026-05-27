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
	return toolutil.NewReadActionSpec(name, route, accessRequestOptionsForAction(name, individualTool))
}

func accessRequestCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, accessRequestOptionsForAction(name, individualTool))
}

func accessRequestUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, accessRequestOptionsForAction(name, individualTool))
}

func accessRequestDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, accessRequestOptionsForAction(name, individualTool))
}

func accessRequestOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute accessrequests domain action.", Tags: []string{"access", "request"},
		RelatedActions: []string{"project.member_list", "group.member_list"},
		OpenWorld:      true,
		OwnerPackage:   "accessrequests",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "request_list_project":
		options.Usage = "List pending project access requests. Use before approving or denying user access requests at project scope."
		options.Aliases = []string{"list project access requests", "project join requests", "pending project requests"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project numeric ID or path where access requests are pending.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
		}
	case "request_list_group":
		options.Usage = "List pending group access requests. Use before approving or denying user access requests at group scope."
		options.Aliases = []string{"list group access requests", "group join requests", "pending group requests"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:   "scope_group",
				ValueSource:    "Group numeric ID or full path where requests are pending.",
				ExampleBinding: `params.group_id:"my-group"`,
			},
		}
	case "approve_project":
		options.Usage = "Approve a user project access request. Use after listing pending requests and selecting the user_id to approve."
		options.Aliases = []string{"approve project access request", "grant project access request", "accept project join request"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"user_id": {
				SemanticRole:   "user_id",
				ValueSource:    "User ID from request_list_project output.",
				ExampleBinding: "params.user_id:123",
			},
		}
	case "approve_group":
		options.Usage = "Approve a user group access request. Use after listing pending group requests and selecting the user_id to approve."
		options.Aliases = []string{"approve group access request", "grant group access request", "accept group join request"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"user_id": {
				SemanticRole:   "user_id",
				ValueSource:    "User ID from request_list_group output.",
				ExampleBinding: "params.user_id:123",
			},
		}
	case "deny_project":
		options.Usage = "Deny a pending project access request. Use only after confirming the user and scope to avoid rejecting the wrong request."
		options.Aliases = []string{"deny project access request", "reject project join request", "decline project request"}
	case "deny_group":
		options.Usage = "Deny a pending group access request. Use only after confirming the user and scope to avoid rejecting the wrong request."
		options.Aliases = []string{"deny group access request", "reject group join request", "decline group request"}
	}

	return options
}
