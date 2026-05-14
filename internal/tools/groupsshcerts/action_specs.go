package groupsshcerts

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for group SSH certificate actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupSSHCertReadSpec("ssh_cert_list", toolutil.RouteAction(client, List), "gitlab_list_group_ssh_certificates"),
		groupSSHCertCreateSpec("ssh_cert_create", toolutil.RouteAction(client, Create), "gitlab_create_group_ssh_certificate"),
		groupSSHCertDeleteSpec("ssh_cert_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_group_ssh_certificate"),
	}
}

func groupSSHCertReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupSSHCertOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupSSHCertCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, groupSSHCertOptions(individualTool))
}

func groupSSHCertDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupSSHCertOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupSSHCertOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "ssh-certificate"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupsshcerts",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
