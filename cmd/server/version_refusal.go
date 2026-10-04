package main

import (
	"context"
	"log/slog"
	"strings"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/oauth"
)

// versionRefusedMessage is stdio's one line for a token GitLab accepted and
// refused the instance version for a fine-grained permission it was not
// granted. Metadata: Read is the permission GitLab declares for
// /api/v4/version at v19.4.1-ee (lib/api/metadata.rb); the line's attributes
// carry what GitLab's own sentence listed, which is the authority when a
// release declares another.
//
// It says what the server loses and what it keeps doing, because the start is
// not degraded: the instance answered and authenticated the token, so the
// catalog is registered and every other call is GitLab's to judge. What is
// lost is the version, and with it the edition the version endpoint reports,
// which is what tells a licensed enterprise build the license could not be
// read from a CE build where Free is the truth.
//
// It is one literal rather than a concatenation, as is the message below: a
// constant has no statement to cover, so the mutation gate reports every
// operator of a concatenation as a mutant nothing can reach.
const versionRefusedMessage = "GitLab refused this token GET /api/v4/version, which a fine-grained personal access token reads only when it grants Metadata: Read, so the server runs without the instance version and edition: a tier the license and the namespace plans do not settle is Free (set GITLAB_MCP_TIER if this instance is licensed). A token's grant cannot be changed after it is created: create one that also grants Metadata: Read"

// fineGrainedDisabledMessage is the same line when GitLab's sentence says
// fine-grained tokens are not enabled for the token's user at all
// ([gitlabclient.GranularRefusalDisabled]): no permission is missing, and
// every call the token makes will be refused the same way until an
// administrator enables them.
const fineGrainedDisabledMessage = "GitLab answered that fine-grained personal access tokens are not yet enabled for this token's user on this instance, so it will refuse every call this token makes: use a classic token (an existing one, on an instance that no longer lets you create them), where the group does not refuse classic tokens, or ask the instance's administrator to enable them"

// versionUnsupportedMessage is the same line when GitLab's sentence says no
// fine-grained permission reaches the version endpoint on this instance
// ([gitlabclient.GranularRefusalUnsupported]), which is what a release that
// declares none for the route answers. No grant can satisfy it, so the way
// out is a classic token, which the route admits with read_user, read_api or
// api (lib/api/metadata.rb at v19.4.1-ee).
const versionUnsupportedMessage = "GitLab refused this token GET /api/v4/version because no fine-grained permission reaches it on this instance, so no fine-grained personal access token can read the instance version and the server runs without it and the edition: a tier the license and the namespace plans do not settle is Free (set GITLAB_MCP_TIER if this instance is licensed). A classic token with the read_api or api scope reads it (an existing one, on an instance that no longer lets you create them), where the group does not refuse classic tokens"

// warnVersionRefused writes the one warning a stdio start gives when GitLab
// refused its token the instance version for a fine-grained permission
// ([gitlabclient.Client.VersionRefusal]). Each of GitLab's sentences has a
// line of its own, since their ways out differ: a sentence that lists missing
// permissions, or one this server cannot read, gets the line that asks for a
// token granting Metadata: Read and names the permissions GitLab listed the
// way a door's log line does, a count and at most three names cut short
// ([sentencePermissions]), and never the sentence itself, which is the
// instance's text.
func warnVersionRefused(ctx context.Context, sentence string) {
	switch gitlabclient.ParseGranularRefusal(sentence).Kind {
	case gitlabclient.GranularRefusalDisabled:
		slog.WarnContext(ctx, fineGrainedDisabledMessage)
	case gitlabclient.GranularRefusalUnsupported:
		slog.WarnContext(ctx, versionUnsupportedMessage)
	default:
		count, named := sentencePermissions(oauth.QuotedDescription(sentence))
		slog.WarnContext(ctx, versionRefusedMessage, "permissions", count, "named", strings.Join(named, "; "))
	}
}
