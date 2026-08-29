package oauth

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// DefaultResourceDocumentation is the RFC 9728 resource_documentation value a
// deployment publishes when the operator names no page of their own.
const DefaultResourceDocumentation = "https://jmrp.io/docs/gitlab-mcp-server/guides/oauth-app-setup/"

// ResourceLinks carries the RFC 9728 URL fields an operator can point at their
// own pages.
//
// They are grouped because they answer one question — where a person goes to
// find out what this deployment is and what it does with their access — and
// because three positional strings on one constructor would be easy to pass in
// the wrong order.
//
// Documentation defaults to this project's OAuth setup guide when empty, since
// a client that finds no guidance at all is worse off than one sent to generic
// instructions. Policy and TermsOfService have no such default: they describe a
// specific deployment's undertakings, and this project cannot make them on an
// operator's behalf.
type ResourceLinks struct {
	// Documentation is resource_documentation: a page describing the OAuth
	// application a client should use here.
	Documentation string
	// Policy is resource_policy_uri: how this deployment handles the data
	// reached with the tokens it accepts.
	Policy string
	// TermsOfService is resource_tos_uri: the terms under which it is offered.
	TermsOfService string
}

// NewProtectedResourceHandler returns an http.Handler that serves RFC 9728
// Protected Resource Metadata. MCP clients use this endpoint to discover the
// GitLab authorization server associated with this resource.
//
// supportedScopes are the scopes a client may authorize with, most capable
// first, which it reads from scopes_supported to build its authorization
// request. A deployment that can write lists api and read_api: the first for a
// client that wants the whole surface, the second for one that wants a
// credential which cannot mutate anything and is served a read-only surface
// accordingly. A deployment that never mutates lists only read_api, so no user
// is asked to grant more than it can use.
//
// links.Documentation is as close as RFC 9728 permits a resource server to come
// to telling a client which client to be: the specification defines no field
// carrying a client identifier, and §5.3 leaves establishing one "out of
// scope".
//
// The handler is registered at /.well-known/oauth-protected-resource.
func NewProtectedResourceHandler(resourceURL string, gitlabURLs, supportedScopes []string, links ResourceLinks) http.Handler {
	documentationURL := links.Documentation
	if documentationURL == "" {
		documentationURL = DefaultResourceDocumentation
	}
	metadata := &oauthex.ProtectedResourceMetadata{
		Resource: resourceURL,
		// RFC 9728 defines authorization_servers as an array, so a
		// deployment publishing several GitLab instances lists them all and
		// a client picks the one it holds an account on. That is what makes
		// one endpoint able to serve gitlab.com and a self-managed instance
		// without a free-form header choosing where tokens get sent.
		AuthorizationServers:   gitlabURLs,
		BearerMethodsSupported: []string{"header"},
		ScopesSupported:        supportedScopes,
		// RFC 9728 RECOMMENDED fields: a human name and a docs URL let a
		// directory or consent screen label this resource instead of
		// showing only its URL.
		ResourceName:          "GitLab MCP Server",
		ResourceDocumentation: documentationURL,
		// Both are omitempty, and both stay empty unless an operator names a
		// page. A field pointing at a document that does not exist is worse
		// than an absent optional field: a consent screen would render a dead
		// link where a user is deciding whether to grant access.
		ResourcePolicyURI: links.Policy,
		ResourceTOSURI:    links.TermsOfService,
	}
	return auth.ProtectedResourceMetadataHandler(metadata)
}

// MetadataURLFor derives the RFC 9728 §3 protected-resource metadata URL
// from a resource identifier: the well-known path segment is inserted
// between the host component and the resource's path, so
// https://mcp.example.com/gitlab advertises its metadata at
// https://mcp.example.com/.well-known/oauth-protected-resource/gitlab and a
// path-less resource at the bare well-known URI. The identifier is
// validated at config load; a parse failure here would be a programming
// error, so the fallback simply appends the well-known path.
func MetadataURLFor(resourceID string) string {
	u, err := url.Parse(resourceID)
	if err != nil || u.Host == "" {
		return strings.TrimSuffix(resourceID, "/") + "/.well-known/oauth-protected-resource"
	}
	path := u.Path
	u.Path = "/.well-known/oauth-protected-resource" + path
	return u.String()
}
