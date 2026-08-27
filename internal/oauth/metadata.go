package oauth

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// NewProtectedResourceHandler returns an http.Handler that serves RFC 9728
// Protected Resource Metadata. MCP clients use this endpoint to discover
// the GitLab authorization server associated with this resource.
//
// requiredScope is the GitLab scope this deployment actually needs, which a
// client reads from scopes_supported to build its authorization request. It
// is the least privilege that works: a server that can only read asks for
// read_api rather than making every user grant full api.
//
// The handler is registered at /.well-known/oauth-protected-resource.
func NewProtectedResourceHandler(resourceURL, gitlabURL, requiredScope string) http.Handler {
	metadata := &oauthex.ProtectedResourceMetadata{
		Resource:               resourceURL,
		AuthorizationServers:   []string{gitlabURL},
		BearerMethodsSupported: []string{"header"},
		ScopesSupported:        []string{requiredScope},
		// RFC 9728 RECOMMENDED fields: a human name and a docs URL let a
		// directory or consent screen label this resource instead of
		// showing only its URL.
		ResourceName:          "GitLab MCP Server",
		ResourceDocumentation: "https://jmrp.io/docs/gitlab-mcp-server/guides/oauth-app-setup/",
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
