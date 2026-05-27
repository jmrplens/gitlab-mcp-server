package enterpriseusers

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for enterprise user actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		enterpriseUserReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_enterprise_users"),
		enterpriseUserReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_enterprise_user"),
		enterpriseUserDisable2FASpec(client),
		enterpriseUserDestructiveSpec("delete", toolutil.DestructiveAction(client, DeleteOutput), "gitlab_delete_enterprise_user"),
	}
}

// Disable2FAOutput disables two-factor authentication and returns the canonical success message shape.
func Disable2FAOutput(ctx context.Context, client *gitlabclient.Client, input Disable2FAInput) (toolutil.VoidOutput, error) {
	if err := Disable2FA(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: "success", Message: fmt.Sprintf("Disabled 2FA for enterprise user %d in group %s.", input.UserID, input.GroupID)}, nil
}

// DeleteOutput deletes an enterprise user and returns the canonical success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted enterprise user %d from group %s.", input.UserID, input.GroupID)}, nil
}

func enterpriseUserReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, enterpriseUserOptions(individualTool))
}

func enterpriseUserDestructiveSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, enterpriseUserOptions(individualTool))
}

func enterpriseUserDisable2FASpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualDestructive := false
	options := enterpriseUserOptions("gitlab_disable_2fa_enterprise_user")
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewDeleteActionSpec("disable_2fa", toolutil.DestructiveAction(client, Disable2FAOutput), options)
}

func enterpriseUserOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute enterpriseusers domain action.", Tags: []string{"enterprise_user"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "enterpriseusers",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
