// Command eval_mcp_surfaces evaluates model behavior across MCP tool surfaces.
//
// It runs a typed registry of evaluation cases against the GitLab MCP server
// in either mock or live (Docker/self-hosted) mode, captures per-task traces,
// and emits Markdown reports and per-task trace artifacts for downstream
// review. See the README at the top of this directory for the supported flags
// and presets.
package main
