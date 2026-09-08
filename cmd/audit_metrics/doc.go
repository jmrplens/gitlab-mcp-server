// Command audit_metrics generates a comprehensive metrics summary for the
// gitlab-mcp-server MCP server. It counts tools, meta-tools, resources, and
// prompts by reading the served surface through cmd/internal/mcpsurface —
// listing what a client receives is the only reliable counting method, and
// that package is where the listing is pinned and the schema chain applied.
// It also scans the filesystem for Go packages, source files, and test files.
//
// Usage:
//
//	go run ./cmd/audit_metrics/
package main
