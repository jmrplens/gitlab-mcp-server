// Command server is the MCP server entry point for gitlab-mcp-server.
// In stdio mode, configuration comes from environment variables (.env / exports).
// In HTTP mode, configuration comes from CLI flags; no GITLAB_TOKEN is required
// at startup — each client provides its own token per-request.
// The --shutdown flag terminates running instances before external updaters
// replace the binary on disk.
//
// # Modes
//
// Stdio mode creates one MCP server from environment configuration and serves
// JSON-RPC over standard input and output. HTTP mode creates a streamable HTTP
// handler backed by a server pool so each token and GitLab URL pair receives an
// isolated MCP server configuration.
//
// # Startup Flow
//
// The command validates configuration, registers tools, resources, prompts,
// completions, progress, and elicitation support, then
// starts the selected transport:
//
//	server
//	    |
//	    v
//	configuration and startup
//	    |
//	    v
//	MCP capability registration
//	    |
//	    v
//	stdio or HTTP transport
package main
