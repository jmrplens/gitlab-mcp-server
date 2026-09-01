package accessrequests

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionProjectMemberList        = "project.member_list"
	actionGroupMemberList          = "group.member_list"
	actionAccessApproveProject     = "accessrequests.approve_project"
	actionAccessDenyProject        = "accessrequests.deny_project"
	actionAccessApproveGroup       = "accessrequests.approve_group"
	actionAccessDenyGroup          = "accessrequests.deny_group"
	actionAccessRequestListProject = "accessrequests.request_list_project"
	actionAccessRequestListGroup   = "accessrequests.request_list_group"
	paramUserID                    = "user_id"
)

// ActionSpecs returns canonical specs for project and group access request actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_access_request_list_project — list pending project access requests.
		accessRequestReadSpec("request_list_project", toolutil.RouteAction(client, ListProject), "gitlab_access_request_list_project"),
		// gitlab_access_request_list_group — list pending group access requests.
		accessRequestReadSpec("request_list_group", toolutil.RouteAction(client, ListGroup), "gitlab_access_request_list_group"),
		// gitlab_access_request_request_project — request access to a project.
		accessRequestCreateSpec("request_project", toolutil.RouteAction(client, RequestProject), "gitlab_access_request_request_project"),
		// gitlab_access_request_request_group — request access to a group.
		accessRequestCreateSpec("request_group", toolutil.RouteAction(client, RequestGroup), "gitlab_access_request_request_group"),
		// gitlab_access_request_approve_project — approve a pending project access request.
		accessRequestUpdateSpec("approve_project", toolutil.RouteAction(client, ApproveProject), "gitlab_access_request_approve_project"),
		// gitlab_access_request_approve_group — approve a pending group access request.
		accessRequestUpdateSpec("approve_group", toolutil.RouteAction(client, ApproveGroup), "gitlab_access_request_approve_group"),
		// gitlab_access_request_deny_project — deny a pending project access request.
		accessRequestDeleteSpec("deny_project", toolutil.RouteAction(client, DenyProjectOutput), "gitlab_access_request_deny_project"),
		// gitlab_access_request_deny_group — deny a pending group access request.
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
		RelatedActions: []string{actionProjectMemberList, actionGroupMemberList},
		OpenWorld:      true,
		OwnerPackage:   "accessrequests",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "request_list_project":
		options.Usage = "List pending project access requests. Use before approving or denying user access requests at project scope. Supports order_by, sort, and keyset pagination."
		options.Aliases = []string{"list project access requests", "project join requests", "pending project requests"}
		options.RelatedActions = []string{actionAccessApproveProject, actionAccessDenyProject, actionProjectMemberList}
		options.IndividualTool.Description = "List pending access requests for a project. Returns: access requests with id, username, name, state, access level, requested/created timestamps, and pagination metadata. See also: gitlab_access_request_approve_project, gitlab_access_request_deny_project, gitlab_project_members_list."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project numeric ID or path where access requests are pending.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
			"order_by": {
				ValueSource:      "Column to order keyset-paginated results by (e.g. id).",
				ExampleBinding:   `params.order_by:"id"`,
				CommonConfusions: []string{"Combine order_by with sort (asc/desc). Do not pass natural-language phrases as the field value."},
			},
		}
	case "request_list_group":
		options.Usage = "List pending group access requests. Use before approving or denying user access requests at group scope. Supports order_by, sort, and keyset pagination."
		options.Aliases = []string{"list group access requests", "group join requests", "pending group requests"}
		options.RelatedActions = []string{actionAccessApproveGroup, actionAccessDenyGroup, actionGroupMemberList}
		options.IndividualTool.Description = "List pending access requests for a group. Returns: access requests with id, username, name, state, access level, requested/created timestamps, and pagination metadata. See also: gitlab_access_request_approve_group, gitlab_access_request_deny_group, gitlab_group_members_list."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:   "scope_group",
				ValueSource:    "Group numeric ID or full path where requests are pending.",
				ExampleBinding: `params.group_id:"my-group"`,
			},
			"order_by": {
				ValueSource:      "Column to order keyset-paginated results by (e.g. id).",
				ExampleBinding:   `params.order_by:"id"`,
				CommonConfusions: []string{"Combine order_by with sort (asc/desc). Do not pass natural-language phrases as the field value."},
			},
		}
	case "request_project":
		options.Usage = "Request membership access to a project for the authenticated user. Use to self-request joining a project. A maintainer must later approve or deny the request."
		options.Aliases = []string{"request project access", "join a project", "ask to join project", "self-request project membership"}
		options.RelatedActions = []string{actionAccessRequestListProject, actionAccessApproveProject, actionAccessDenyProject}
		options.IndividualTool.Description = "Request access to a project as the authenticated user. Returns: the created access request with id, username, name, requested state, and requested_at timestamp. See also: gitlab_access_request_list_project, gitlab_access_request_approve_project, gitlab_access_request_deny_project."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project numeric ID or path to request access to.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
		}
	case "request_group":
		options.Usage = "Request membership access to a group for the authenticated user. Use to self-request joining a group. An owner must later approve or deny the request."
		options.Aliases = []string{"request group access", "join a group", "ask to join group", "self-request group membership"}
		options.RelatedActions = []string{actionAccessRequestListGroup, actionAccessApproveGroup, actionAccessDenyGroup}
		options.IndividualTool.Description = "Request access to a group as the authenticated user. Returns: the created access request with id, username, name, requested state, and requested_at timestamp. See also: gitlab_access_request_list_group, gitlab_access_request_approve_group, gitlab_access_request_deny_group."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:   "scope_group",
				ValueSource:    "Group numeric ID or full path to request access to.",
				ExampleBinding: `params.group_id:"my-group"`,
			},
		}
	case "approve_project":
		options.Usage = "Approve a user project access request. Use after listing pending requests and selecting the user_id to approve."
		options.Aliases = []string{"approve project access request", "grant project access request", "accept project join request"}
		options.RelatedActions = []string{actionAccessRequestListProject, actionAccessDenyProject, actionProjectMemberList}
		options.IndividualTool.Description = "Approve a pending project access request, granting the user membership. Returns: the approved access request with id, username, name, granted access level, and approved state. See also: gitlab_access_request_list_project, gitlab_access_request_deny_project, gitlab_project_members_list."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramUserID: {
				SemanticRole:   paramUserID,
				ValueSource:    "User ID from request_list_project output.",
				ExampleBinding: "params.user_id:123",
			},
		}
	case "approve_group":
		options.Usage = "Approve a user group access request. Use after listing pending group requests and selecting the user_id to approve."
		options.Aliases = []string{"approve group access request", "grant group access request", "accept group join request"}
		options.RelatedActions = []string{actionAccessRequestListGroup, actionAccessDenyGroup, actionGroupMemberList}
		options.IndividualTool.Description = "Approve a pending group access request, granting the user membership. Returns: the approved access request with id, username, name, granted access level, and approved state. See also: gitlab_access_request_list_group, gitlab_access_request_deny_group, gitlab_group_members_list."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramUserID: {
				SemanticRole:   paramUserID,
				ValueSource:    "User ID from request_list_group output.",
				ExampleBinding: "params.user_id:123",
			},
		}
	case "deny_project":
		options.Usage = "Deny a pending project access request. Use only after confirming the user and scope to avoid rejecting the wrong request."
		options.Aliases = []string{"deny project access request", "reject project join request", "decline project request"}
		options.RelatedActions = []string{actionAccessRequestListProject, actionAccessApproveProject, actionProjectMemberList}
		options.IndividualTool.Description = "Deny a pending project access request, removing it without granting membership. Returns: a success status and confirmation message. See also: gitlab_access_request_list_project, gitlab_access_request_approve_project, gitlab_project_members_list."
	case "deny_group":
		options.Usage = "Deny a pending group access request. Use only after confirming the user and scope to avoid rejecting the wrong request."
		options.Aliases = []string{"deny group access request", "reject group join request", "decline group request"}
		options.RelatedActions = []string{actionAccessRequestListGroup, actionAccessApproveGroup, actionGroupMemberList}
		options.IndividualTool.Description = "Deny a pending group access request, removing it without granting membership. Returns: a success status and confirmation message. See also: gitlab_access_request_list_group, gitlab_access_request_approve_group, gitlab_group_members_list."
	}

	return options
}
