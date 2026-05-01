// Package oauth provides GitLab-specific OAuth 2.0 support for HTTP mode.
//
// It verifies bearer tokens against GitLab's user endpoint, caches verified
// identities without storing raw token material, normalizes legacy GitLab
// PRIVATE-TOKEN headers into Authorization headers, and serves the RFC 9728
// Protected Resource Metadata endpoint used by MCP clients to discover the
// GitLab authorization server for a protected resource.
package oauth
