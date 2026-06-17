package keys

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for SSH key lookup actions exposed
// as MCP tools. The two read routes (get by ID, get by fingerprint) are
// projected into the dynamic, meta, individual, and audit surfaces by
// the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_get_key_with_user — fetch an SSH key (with the owning user) by numeric key ID.
		keyReadSpec("key_get_with_user", toolutil.RouteAction(client, GetKeyWithUser), "gitlab_get_key_with_user"),
		// gitlab_get_key_by_fingerprint — fetch an SSH key by its fingerprint.
		keyReadSpec("key_get_by_fingerprint", toolutil.RouteAction(client, GetKeyByFingerprint), "gitlab_get_key_by_fingerprint"),
	}
}

// keyReadSpec builds a read-only [toolutil.ActionSpec] for a key lookup
// action using the package's default options.
func keyReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute keys domain action.", Tags: []string{"user", "ssh_key"},
		OpenWorld:      true,
		OwnerPackage:   "keys",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	return toolutil.NewReadActionSpec(name, route, options)
}
