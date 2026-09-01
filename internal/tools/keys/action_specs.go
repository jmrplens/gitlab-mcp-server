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
// action, filling in non-generic discovery metadata (Usage, natural-language
// Aliases, canonical RelatedActions into the user.* SSH-key surface, and the
// "Returns: … See also: …" individual-tool description) for each lookup tool.
func keyReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute keys domain action.", Tags: []string{"user", "ssh_key"},
		OpenWorld:      true,
		OwnerPackage:   "keys",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateKeyMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

// decorateKeyMeta fills non-generic Usage, distinctive natural-language
// Aliases, canonical RelatedActions, and the "Returns: … See also: …"
// individual-tool description for each admin key-lookup tool. It is a no-op
// for tools without a metadata entry so the generic placeholders remain.
func decorateKeyMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := keyActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// keyActionMetaEntry is the discovery metadata for one admin key-lookup action.
type keyActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// keyActionMeta maps each individual key-lookup tool to its discovery
// metadata. RelatedActions point at the per-user SSH-key surface (user.*)
// since the admin Keys API resolves a globally unique key to its owning user.
var keyActionMeta = map[string]keyActionMetaEntry{
	"gitlab_get_key_with_user": {
		usage:       "Look up a single SSH key by its global numeric key ID and return the owning user. Admin-only on self-managed instances. Use when a prompt or audit log gives a bare key ID and you need to identify who owns it. For a specific user's own keys use the per-user SSH-key tools instead.",
		aliases:     []string{"get ssh key by id", "look up ssh key owner by id", "identify ssh key owner", "whose ssh key is this id"},
		related:     []string{"user.get_ssh_key", "user.ssh_keys", "user.ssh_keys_for_user"},
		description: "Look up an SSH key by its global ID and return the owning user. Returns: the key ID, title, public key, creation time, and the owning user (ID, username, name). See also: gitlab_get_ssh_key, gitlab_list_ssh_keys, gitlab_list_ssh_keys_for_user.",
	},
	"gitlab_get_key_by_fingerprint": {
		usage:       "Look up a single SSH key or deploy key by its fingerprint and return the owning user. Admin-only on self-managed instances. Accepts the modern SHA256:base64 form or the legacy MD5:aa:bb:cc hex-pairs form. Use when you have only a key fingerprint (for example from an auth log) and need to identify its owner.",
		aliases:     []string{"get ssh key by fingerprint", "look up ssh key owner by fingerprint", "find ssh key from fingerprint", "whose ssh key has this fingerprint"},
		related:     []string{"user.get_ssh_key", "user.ssh_keys", "user.ssh_keys_for_user"},
		description: "Look up an SSH key or deploy key by fingerprint and return the owning user. Returns: the key ID, title, public key, creation time, and the owning user (ID, username, name). See also: gitlab_get_ssh_key, gitlab_list_ssh_keys, gitlab_list_ssh_keys_for_user.",
	},
}
