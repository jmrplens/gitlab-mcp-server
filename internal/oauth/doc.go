// Package oauth provides GitLab-specific OAuth 2.0 support for HTTP mode.
//
// It verifies bearer tokens against GitLab's user endpoint, caches verified
// identities without storing raw token material, normalizes legacy GitLab
// PRIVATE-TOKEN headers into Authorization headers, and serves the RFC 9728
// Protected Resource Metadata endpoint used by MCP clients to discover the
// GitLab authorization server for a protected resource.
//
// # HTTP Mode Flow
//
// The package participates in the HTTP transport path as follows:
//
//	HTTP request
//	    |
//	    v
//	NormalizeAuthHeader
//	    |
//	    v
//	NewGitLabVerifier
//	    |
//	    v
//	GitLab /user endpoint and TokenCache
//
// [NormalizeAuthHeader] preserves Bearer tokens while converting GitLab
// PRIVATE-TOKEN headers into Authorization headers for clients that still use
// legacy authentication. [NewGitLabVerifier] validates Bearer tokens with
// GitLab and stores verified identity metadata in [TokenCache].
//
// [NewProtectedResourceHandler] serves OAuth Protected Resource Metadata so MCP
// clients can discover the GitLab authorization server associated with the
// requested resource URL.
package oauth
