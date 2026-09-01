package groupsshcerts

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionGroupGet = "group.get"

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
	opts := groupSSHCertOptions(individualTool)
	opts.Usage = "List the SSH CA certificates registered on a group so members can authenticate Git over SSH with certificates signed by that CA. Use this when the prompt asks which SSH certificate authorities a group trusts, before adding or revoking one. Requires the group Owner role. Supports offset and keyset pagination."
	opts.Aliases = []string{individualTool, "list group ssh ca certificates", "show group ssh certificate authorities", "view trusted ssh signing certificates"}
	opts.RelatedActions = []string{"ssh_cert_create", "ssh_cert_delete", actionGroupGet}
	opts.IndividualTool.Description = "List the SSH CA certificates registered on a group. Returns: each certificate's id, title, public key, and creation timestamp, with offset/keyset pagination. See also: gitlab_create_group_ssh_certificate, gitlab_delete_group_ssh_certificate, gitlab_group_get."
	return toolutil.NewReadActionSpec(name, route, opts)
}

func groupSSHCertCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	opts := groupSSHCertOptions(individualTool)
	opts.Usage = "Register an SSH CA certificate on a group by supplying the CA public key and a title. Members can then authenticate Git over SSH using user certificates signed by this CA. Use this when onboarding an SSH certificate authority for a group. Requires the group Owner role. The key must be a valid SSH public key."
	opts.Aliases = []string{individualTool, "add group ssh ca certificate", "register ssh certificate authority", "trust ssh signing key for group"}
	opts.RelatedActions = []string{"ssh_cert_list", "ssh_cert_delete", actionGroupGet}
	opts.IndividualTool.Description = "Register a new SSH CA certificate on a group from a CA public key and title. Returns: the created certificate's id, title, public key, and creation timestamp. See also: gitlab_list_group_ssh_certificates, gitlab_delete_group_ssh_certificate, gitlab_group_get."
	return toolutil.NewCreateActionSpec(name, route, opts)
}

func groupSSHCertDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	opts := groupSSHCertOptions(individualTool)
	opts.Usage = "Revoke an SSH CA certificate from a group by its certificate id, so user certificates signed by that CA can no longer authenticate Git over SSH. Use this when retiring an SSH certificate authority. Resolve the certificate id with ssh_cert_list first. Requires the group Owner role and is irreversible."
	opts.Aliases = []string{individualTool, "remove group ssh ca certificate", "revoke ssh certificate authority", "untrust ssh signing certificate"}
	opts.RelatedActions = []string{"ssh_cert_list", "ssh_cert_create", actionGroupGet}
	opts.IndividualTool.Description = "Revoke an SSH CA certificate from a group by certificate id. Returns: a success status and confirmation message. See also: gitlab_list_group_ssh_certificates, gitlab_create_group_ssh_certificate, gitlab_group_get."
	return toolutil.NewDeleteActionSpec(name, route, opts)
}

func groupSSHCertOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupsshcerts domain action.", Tags: []string{"group", "ssh-certificate"},
		RelatedActions: []string{actionGroupGet},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupsshcerts",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
