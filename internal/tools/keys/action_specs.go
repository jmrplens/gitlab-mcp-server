package keys

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for SSH key lookup actions exposed through gitlab_user.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		keyReadSpec("key_get_with_user", toolutil.RouteAction(client, GetKeyWithUser), "gitlab_get_key_with_user"),
		keyReadSpec("key_get_by_fingerprint", toolutil.RouteAction(client, GetKeyByFingerprint), "gitlab_get_key_by_fingerprint"),
	}
}

func keyReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute keys domain action.", Tags: []string{"user", "ssh_key"},
		OpenWorld:      true,
		OwnerPackage:   "keys",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	return toolutil.NewReadActionSpec(name, route, options)
}
