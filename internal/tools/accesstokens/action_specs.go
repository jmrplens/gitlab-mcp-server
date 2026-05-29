package accesstokens

import (
	"context"
	"fmt"
	"strings"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project, group, and personal access token actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		accessTokenReadSpec("token_project_list", toolutil.RouteAction(client, ProjectList), "gitlab_project_access_token_list"),
		accessTokenReadSpec("token_project_get", toolutil.RouteAction(client, ProjectGet), "gitlab_project_access_token_get"),
		accessTokenCreateSpec("token_project_create", toolutil.RouteAction(client, ProjectCreate), "gitlab_project_access_token_create"),
		accessTokenRotateSpec("token_project_rotate", toolutil.RouteAction(client, ProjectRotate), "gitlab_project_access_token_rotate"),
		accessTokenRotateSpec("token_project_rotate_self", toolutil.RouteAction(client, ProjectRotateSelf), "gitlab_project_access_token_rotate_self"),
		accessTokenDeleteSpec("token_project_revoke", toolutil.RouteAction(client, ProjectRevokeOutput), "gitlab_project_access_token_revoke"),
		accessTokenReadSpec("token_group_list", toolutil.RouteAction(client, GroupList), "gitlab_group_access_token_list"),
		accessTokenReadSpec("token_group_get", toolutil.RouteAction(client, GroupGet), "gitlab_group_access_token_get"),
		accessTokenCreateSpec("token_group_create", toolutil.RouteAction(client, GroupCreate), "gitlab_group_access_token_create"),
		accessTokenRotateSpec("token_group_rotate", toolutil.RouteAction(client, GroupRotate), "gitlab_group_access_token_rotate"),
		accessTokenRotateSpec("token_group_rotate_self", toolutil.RouteAction(client, GroupRotateSelf), "gitlab_group_access_token_rotate_self"),
		accessTokenDeleteSpec("token_group_revoke", toolutil.RouteAction(client, GroupRevokeOutput), "gitlab_group_access_token_revoke"),
		accessTokenReadSpec("token_personal_list", toolutil.RouteAction(client, PersonalList), "gitlab_personal_access_token_list"),
		accessTokenReadSpec("token_personal_get", toolutil.RouteAction(client, PersonalGet), "gitlab_personal_access_token_get"),
		accessTokenRotateSpec("token_personal_rotate", toolutil.RouteAction(client, PersonalRotate), "gitlab_personal_access_token_rotate"),
		accessTokenRotateSpec("token_personal_rotate_self", toolutil.RouteAction(client, PersonalRotateSelf), "gitlab_personal_access_token_rotate_self"),
		accessTokenDeleteSpec("token_personal_revoke", toolutil.RouteAction(client, PersonalRevokeOutput), "gitlab_personal_access_token_revoke"),
		accessTokenDeleteSpec("token_personal_revoke_self", toolutil.RouteAction(client, PersonalRevokeSelfOutput), "gitlab_personal_access_token_revoke_self"),
	}
}

// ProjectRevokeOutput revokes a project access token and returns the legacy success message shape.
func ProjectRevokeOutput(ctx context.Context, client *gitlabclient.Client, input ProjectRevokeInput) (toolutil.DeleteOutput, error) {
	if err := ProjectRevoke(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted project access token."}, nil
}

// GroupRevokeOutput revokes a group access token and returns the legacy success message shape.
func GroupRevokeOutput(ctx context.Context, client *gitlabclient.Client, input GroupRevokeInput) (toolutil.DeleteOutput, error) {
	if err := GroupRevoke(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted group access token."}, nil
}

// PersonalRevokeOutput revokes a personal access token and returns the legacy success message shape.
func PersonalRevokeOutput(ctx context.Context, client *gitlabclient.Client, input PersonalRevokeInput) (toolutil.DeleteOutput, error) {
	if err := PersonalRevoke(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted personal access token."}, nil
}

// PersonalRevokeSelfOutput revokes the current personal access token and returns the legacy success message shape.
func PersonalRevokeSelfOutput(ctx context.Context, client *gitlabclient.Client, input PersonalRevokeSelfInput) (toolutil.DeleteOutput, error) {
	if err := PersonalRevokeSelf(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted personal access token."}, nil
}

func accessTokenReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, accessTokenOptions(name, individualTool))
}

func accessTokenCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, accessTokenOptions(name, individualTool))
}

func accessTokenRotateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, accessTokenOptions(name, individualTool))
}

func accessTokenDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, accessTokenOptions(name, individualTool))
}

func accessTokenOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute accesstokens domain action.", Tags: []string{"access", "access_token", "token"},
		OpenWorld:      true,
		OwnerPackage:   "accesstokens",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	scope, operation := accessTokenScopeAndOperation(actionName)
	if scope == "" {
		return options
	}
	operationText := strings.ReplaceAll(operation, "_", " ")
	options.Usage = fmt.Sprintf("Use for GitLab %s access tokens; this action %s a %s-scoped API token.", scope, accessTokenOperationPhrase(operation), scope)
	options.Aliases = []string{fmt.Sprintf("%s %s access token", operationText, scope)}
	if operation == "list" {
		options.Usage = fmt.Sprintf("Use for GitLab %s access tokens; this action lists %s-scoped API tokens.", scope, scope)
		options.Tags = append(options.Tags, scope+"_access_tokens")
		options.Aliases = append(options.Aliases, scope+" access tokens", fmt.Sprintf("list %s access tokens", scope))
	}
	options.RelatedActions = accessTokenRelatedActions(scope)
	if accessTokenNeedsTokenID(operation) {
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"token_id": {
				SemanticRole:     "access_token",
				ValueSource:      fmt.Sprintf("Access token ID returned by token_%s_list or token_%s_get.", scope, scope),
				CommonConfusions: []string{"Do not use project_id, group_id, or user_id as token_id; token_id identifies the access token itself."},
			},
		}
	}
	return options
}

func accessTokenScopeAndOperation(actionName string) (scope, operation string) {
	for _, scope := range []string{"project", "group", "personal"} {
		prefix := "token_" + scope + "_"
		suffix, found := strings.CutPrefix(actionName, prefix)
		if found {
			return scope, suffix
		}
	}
	return "", ""
}

func accessTokenOperationPhrase(operation string) string {
	switch operation {
	case "list":
		return "lists"
	case "get":
		return "gets"
	case "create":
		return "creates"
	case "rotate", "rotate_self":
		return "rotates"
	case "revoke", "revoke_self":
		return "revokes"
	default:
		return strings.ReplaceAll(operation, "_", " ")
	}
}

func accessTokenNeedsTokenID(operation string) bool {
	switch operation {
	case "get", "rotate", "revoke":
		return true
	default:
		return false
	}
}

func accessTokenRelatedActions(scope string) []string {
	switch scope {
	case "project":
		return []string{"project.get", "access.deploy_key_list_project", "access.deploy_token_list_project"}
	case "group":
		return []string{"group.get"}
	case "personal":
		return []string{"user.current"}
	default:
		return nil
	}
}
