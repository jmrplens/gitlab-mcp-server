package groupsshcerts

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group SSH certificate actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupSSHCertReadSpec("ssh_cert_list", toolutil.RouteAction(client, List), "gitlab_list_group_ssh_certificates"),
		groupSSHCertCreateSpec("ssh_cert_create", toolutil.RouteAction(client, Create), "gitlab_create_group_ssh_certificate"),
		groupSSHCertDeleteSpec("ssh_cert_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_group_ssh_certificate"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted SSH certificate %d from group %s.", input.CertificateID, input.GroupID),
	}, nil
}

func groupSSHCertReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupSSHCertOptions(individualTool))
}

func groupSSHCertCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupSSHCertOptions(individualTool))
}

func groupSSHCertDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupSSHCertOptions(individualTool))
}

func groupSSHCertOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupsshcerts domain action.", Tags: []string{"group", "ssh-certificate"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupsshcerts",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
