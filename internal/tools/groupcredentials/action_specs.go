package groupcredentials

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for group credential inventory actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupCredentialReadSpec("credential_list_pats", toolutil.RouteAction(client, ListPATs), "gitlab_list_group_personal_access_tokens"),
		groupCredentialReadSpec("credential_list_ssh_keys", toolutil.RouteAction(client, ListSSHKeys), "gitlab_list_group_ssh_keys"),
		groupCredentialDeleteSpec("credential_revoke_pat", toolutil.DestructiveVoidAction(client, RevokePAT), "gitlab_revoke_group_personal_access_token"),
		groupCredentialDeleteSpec("credential_delete_ssh_key", toolutil.DestructiveVoidAction(client, DeleteSSHKey), "gitlab_delete_group_ssh_key"),
	}
}

func groupCredentialReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupCredentialOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupCredentialDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupCredentialOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupCredentialOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "credential"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupcredentials",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
