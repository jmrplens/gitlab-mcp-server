// metadata.go implements the RFC 9728 Protected Resource Metadata endpoint,
// which advertises the GitLab OAuth authorization server URL to MCP clients.

package oauth

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// NewProtectedResourceHandler returns an http.Handler that serves RFC 9728
// Protected Resource Metadata. MCP clients use this endpoint to discover
// the GitLab authorization server associated with this resource.
//
// The handler is registered at /.well-known/oauth-protected-resource.
func NewProtectedResourceHandler(resourceURL, gitlabURL string) http.Handler {
	metadata := &oauthex.ProtectedResourceMetadata{
		Resource:               resourceURL,
		AuthorizationServers:   []string{gitlabURL},
		BearerMethodsSupported: []string{"header"},
		ScopesSupported:        []string{"api"},
	}
	return auth.ProtectedResourceMetadataHandler(metadata)
}
