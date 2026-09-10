package main

import (
	"encoding/json"
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/oauth"
)

// The Server Card this file builds is the SEP-2127 document, and it is a
// different document from the one [buildServerCard] builds even though both
// are called a server card.
//
// SEP-2127 cards deliberately carry NO primitives. The SEP says so in
// writing: it "intentionally omits primitive definitions (tools, resources,
// and prompts)", because what a server exposes "can vary by authenticated
// user, session, configuration, feature flags, deployment state", leaving "no
// viable substitute for runtime listing via the protocol's standard
// operations ... with the logged-in user's identity". A card is identity and
// connection metadata, nothing more.
//
// Until this existed, both card routes answered the enumerating SEP-1649
// document and differed only in Content-Type, which put the older shape at
// `/server-card` — the location SEP-2127 reserves. A deployment that wanted
// to be conformant had to shadow that route with a static file in its reverse
// proxy, which is exactly what the public deployment was doing. Now the
// binary serves the right shape itself at each path:
//
//	/server-card                          this document, SEP-2127
//	/.well-known/mcp/server-card.json     the enumerating SEP-1649 document
//
// The legacy path keeps the enumerating document because scanners written
// against that draft already fetch it, and because it is the only
// unauthenticated answer to "what can this server do" that this project
// publishes. Nothing should be tempted to enrich the SEP-2127 card with a
// tool list to help such a scanner: a conformant card answering that question
// is a contradiction, and the scanner has the legacy URL.
const (
	// discoveryCardSchema is required, and the wire format pins it to this
	// exact string: schema.ts declares
	// `@pattern ^https://static\.modelcontextprotocol\.io/schemas/v1/server-card\.schema\.json$`.
	//
	// It answers 404 today. That is upstream and is not ours to work around:
	// the schema family is versioned by the `vN` segment rather than by date,
	// so there is no dated sibling to point at instead, and naming a private
	// mirror would still be wrong once the canonical URL goes live. The
	// trigger for revisiting is that URL starting to answer 200.
	discoveryCardSchema = "https://static.modelcontextprotocol.io/schemas/v1/server-card.schema.json"

	// discoveryCardName is the reverse-DNS identity the MCP Registry knows
	// this server by, matching `name` in server.json and the
	// io.modelcontextprotocol.server.name label the Dockerfile sets. It is
	// NOT the Implementation.Name of the handshake ("gitlab-mcp-server"),
	// which carries no namespace and would fail the card's pattern.
	discoveryCardName = "io.github.jmrplens/gitlab-mcp-server"

	// discoveryCardDescription exists because the card caps `description` at
	// 100 characters and [projectDescription] is about 180. Rather than
	// truncate that one at the cut a reader would notice, this is the text
	// server.json already publishes under the same cap, and
	// TestDiscoveryCard_AgreesWithServerJSON keeps the two from drifting.
	discoveryCardDescription = "Go MCP server for GitLab: 2 dynamic tools reach 1000+ REST/GraphQL actions. Free/CE, no paid tier."

	// discoveryCardRepositorySource is the hosting service identifier a
	// registry uses to decide how to validate and reach the repository.
	discoveryCardRepositorySource = "github"

	// discoveryCardRepositoryURL is the source tree, which is deliberately
	// not [projectWebsite]: the card carries both, and they answer different
	// questions (`repository` is for inspection, `websiteUrl` for a reader).
	discoveryCardRepositoryURL = "https://github.com/jmrplens/gitlab-mcp-server"
)

// The protocol versions the card declares come from
// [supportedProtocolVersionsFor], the same helper [protocolVersionMiddleware]
// refuses requests with, and they are narrowed by `--stateless` for a reason
// the card must not paper over: a stateful deployment does not serve
// 2026-07-28, and the middleware answers it 400.
//
// The extension asks for exactly this consistency: a card "SHOULD accurately
// reflect the server's runtime behavior", and the versions it declares "SHOULD
// NOT contradict the equivalent values" a client observes once connected. A
// card carrying its own copy of the list would satisfy that only by accident,
// and would advertise a version the deployment refuses the moment somebody
// passed --stateless=false.

// buildDiscoveryCard renders the SEP-2127 Server Card for this deployment.
//
// It is cheap and deterministic: identity constants, the running binary's own
// version, and whatever the deployment's flags say about how to reach it.
// Nothing here registers a catalog or opens a session, which is why it needs
// none of the lazy build-once machinery [buildServerCard] is wrapped in.
func buildDiscoveryCard(cfg *config.Config) ([]byte, error) {
	card := map[string]any{
		"$schema":     discoveryCardSchema,
		"name":        discoveryCardName,
		"version":     version,
		"description": discoveryCardDescription,
		"title":       serverDisplayTitle,
		"websiteUrl":  projectWebsite,
		"repository": map[string]any{
			"url":    discoveryCardRepositoryURL,
			"source": discoveryCardRepositorySource,
		},
	}
	if remote := discoveryCardRemote(cfg); remote != nil {
		card["remotes"] = []any{remote}
	}

	out, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling the server card: %w", err)
	}
	return out, nil
}

// discoveryCardRemote describes how to connect to THIS deployment, or nil
// when the deployment cannot say.
//
// A card describes a remote server, and the only address this process knows
// to be reachable from outside is the one --public-url states. A listen
// address is not that: it is frequently a loopback address or a unix socket
// behind a proxy, and publishing it would send a client somewhere it cannot
// go. `remotes` is optional in the schema, so a deployment that names no
// public URL publishes a card with identity and no connection block, which is
// true rather than misleading.
func discoveryCardRemote(cfg *config.Config) map[string]any {
	if cfg.PublicURL == "" {
		return nil
	}
	supported := supportedProtocolVersionsFor(cfg.Stateless)
	versions := make([]any, 0, len(supported))
	for _, v := range supported {
		versions = append(versions, v)
	}
	return map[string]any{
		"type":                      "streamable-http",
		"url":                       cfg.PublicURL,
		"headers":                   []any{discoveryCardCredentialHeader(cfg)},
		"supportedProtocolVersions": versions,
	}
}

// discoveryCardCredentialHeader describes the credential header, per auth
// mode, the way the CARD's schema defines these fields.
//
// `isRequired` is true here while server.json declares the same header on the
// same remote as false, and that is not a contradiction to be tidied away:
// the two documents are governed by different schemas that give the field
// different meanings. The card's wire format (experimental-ext-server-card,
// schema.ts) defines isRequired as whether the input "must be supplied for
// the connection to succeed", and a request here carrying no credential is
// answered 401 in either mode. The Registry schema gives isRequired no
// definition of its own and surrounds it with fields about configuration a
// user is prompted for, which is the reading server.json's false rests on,
// since an OAuth client obtains the token itself and the user supplies
// nothing. Do not reconcile them by copying one value into the other file.
func discoveryCardCredentialHeader(cfg *config.Config) map[string]any {
	if cfg.AuthMode == config.AuthModeOAuth {
		return map[string]any{
			"name": "Authorization",
			"description": "Bearer <token>. This endpoint runs in OAuth mode, so a client that speaks the " +
				"OAuth flow discovers the authorization server from the 401 challenge and sets this " +
				"header itself. A GitLab personal access token also works, sent as 'Bearer glpat-...'. " +
				"Scope " + oauth.MinimumScope + " is enough to list and call read actions; " +
				oauth.ScopeAPI + " is needed to write. The credential travels per request and is never " +
				"stored server-side.",
			"isRequired":  true,
			"isSecret":    true,
			"placeholder": "Bearer glpat-xxxxxxxxxxxxxxxxxxxx",
		}
	}
	return map[string]any{
		"name": "PRIVATE-TOKEN",
		"description": "A GitLab personal access token. Scope " + oauth.MinimumScope + " is enough to " +
			"list and call read actions; " + oauth.ScopeAPI + " is needed to write. It travels per " +
			"request and is never stored server-side.",
		"isRequired":  true,
		"isSecret":    true,
		"placeholder": "glpat-xxxxxxxxxxxxxxxxxxxx",
	}
}
