package attestations

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for build attestation actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		attestationReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_attestations"),
		attestationReadSpec("download", toolutil.RouteAction(client, Download), "gitlab_download_attestation"),
	}
}

func attestationReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:           []string{"security", "attestation"},
		ReadOnly:       true,
		Idempotent:     true,
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "attestations",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
