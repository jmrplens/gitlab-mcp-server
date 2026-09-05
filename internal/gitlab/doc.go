// Package gitlab provides a wrapper around the GitLab REST API v4 client.
// Some domains additionally use the GitLab GraphQL API for endpoints not covered
// by client-go service wrappers (see ADR-0006).
//
// The package also detects Personal Access Token scopes so HTTP server entries
// can register tools according to the authenticated token's capabilities.
//
// # Resilience
//
// Client initialization is lazy and recoverable. When GitLab is temporarily
// unavailable at startup, the server can enter degraded mode and retry
// initialization on later API calls with rate-limited health checks. This keeps
// local MCP startup responsive while still surfacing actionable errors to tool
// handlers.
//
// # Edition and Scope Detection
//
// The wrapper tracks GitLab.com and Premium/Ultimate detection separately from
// token-scope detection. HTTP mode uses that information to register the correct
// tool catalog for each token and GitLab URL pair.
package gitlab
