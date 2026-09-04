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
//
// It must be a page that actually resolves, which is not a given: this pointed
// at .../guides/oauth-app-setup/ for as long as the field has existed, and that
// path has never been served. The documentation site has a flat structure with
// no guides/ segment, and the OAuth application walkthrough lives only in the
// repository, so the closest page that exists is the HTTP server one, whose
// OAuth section links onward to the guide. A 404 is worse here than anywhere
// else it could appear: this URL is published in the protected-resource
// metadata every OAuth client fetches, and some of them show it on the consent
// screen a person is being asked to trust.
const DefaultResourceDocumentation = "https://jmrp.io/docs/gitlab-mcp-server/operations/http-server/"

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
// The handler is registered at the single path [MetadataPathFor] derives from
// the resource identifier.
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

// MetadataPath is the RFC 9728 §3 well-known URI suffix for protected-resource
// metadata. On its own it is the metadata path of the resource that *is* the
// origin; a resource with a path of its own derives a longer one through
// [MetadataPathFor].
const MetadataPath = "/.well-known/oauth-protected-resource"

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
		return strings.TrimSuffix(resourceID, "/") + MetadataPath
	}
	u.Path = metadataPath(u)
	return u.String()
}

// MetadataPathFor returns the one path on which a deployment publishing
// resourceID serves its protected-resource document: the path component of
// [MetadataURLFor], so what a server mounts and what its challenge advertises
// cannot drift apart.
//
// It exists as its own function, rather than inline at the mount, because a
// host can run several MCP servers and each of them has to get this right.
// This one used to mount the document twice, the second time under a
// "/{rest...}" wildcard, so every suffix returned the same body: asked about
// the neighbor at /libgen, it answered with a document naming *this*
// deployment's resource and demanding OAuth against this deployment's GitLab.
// RFC 9728 §3.3 tells a client to discard a document whose resource value is
// not the identifier it derived the URL from, so the extra paths bought a
// conforming client nothing and told a lax one something false.
//
// The path-less form is served only when the resource identifier has no path,
// and that is a decision rather than an omission. /.well-known/oauth-protected-resource
// is by construction the metadata of the resource at the host root: a server
// mounted at /gitlab answering it is claiming an identity that is not its own,
// which is the wildcard's harm in a different spelling. Where --public-url has
// no path the bare form *is* the derived form, so this is one rule and not two
// cases, and a server that owns its hostname loses nothing.
//
// A trailing slash is trimmed because the derived path is mounted as an exact
// http.ServeMux pattern, and a pattern ending in "/" is a subtree match in Go:
// keeping the slash would quietly restore the wildcard this replaced.
func MetadataPathFor(resourceID string) string {
	u, err := url.Parse(resourceID)
	if err != nil || u.Host == "" {
		return MetadataPath
	}
	return metadataPath(u)
}

// metadataPath inserts the well-known segment between the host and the
// resource's own path, for an identifier already known to parse.
func metadataPath(u *url.URL) string {
	return MetadataPath + strings.TrimSuffix(u.Path, "/")
}
