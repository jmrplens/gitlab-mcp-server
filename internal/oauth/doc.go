// Package oauth provides GitLab-specific OAuth 2.0 support for HTTP mode.
//
// It verifies bearer tokens against GitLab's user endpoint, caches verified
// identities without storing raw token material, remembers rejections so a
// replayed bad token costs nothing upstream, and serves the RFC 9728
// Protected Resource Metadata endpoint MCP clients use to discover the
// GitLab authorization server for a protected resource.
//
// # HTTP Mode Flow
//
// The package participates in the HTTP transport path as follows:
//
//	HTTP request with Authorization: Bearer
//	    |
//	    v
//	RejectedTokens  --- hit ---> 401, no upstream call
//	    |
//	    | miss
//	    v
//	NewGitLabVerifier
//	    |
//	    +--> TokenCache hit ---> verified identity
//	    |
//	    v
//	GitLab /user (identity) and scope introspection
//
// [NewGitLabVerifier] validates Bearer tokens with GitLab and stores verified
// identity metadata in [TokenCache]. A definitive rejection is remembered by
// [RejectedTokens]; an [UpstreamError] never is, because it says nothing
// about the credential.
//
// [NewProtectedResourceHandler] serves OAuth Protected Resource Metadata so MCP
// clients can discover the GitLab authorization server associated with the
// requested resource URL, and advertises the scope the deployment actually
// requires — see [RequiredScope].
//
// Legacy PRIVATE-TOKEN headers are not normalized into Authorization here.
// OAuth mode is Bearer-only, so that what the WWW-Authenticate challenge
// advertises is exactly what is accepted, and legacy mode reads both headers
// directly.
package oauth
