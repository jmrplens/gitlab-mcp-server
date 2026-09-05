// Command audit_metrics generates a comprehensive metrics summary for the
// gitlab-mcp-server MCP server. It creates in-memory MCP servers to count tools,
// meta-tools, resources, and prompts at runtime — the only reliable counting
// method. It also scans the filesystem for Go packages, source files, and test
// files.
//
// Usage:
//
//	go run ./cmd/audit_metrics/
package main
