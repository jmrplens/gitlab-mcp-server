// Package autoupdate provides self-update capability for the gitlab-mcp-server
// MCP server. It uses the creativeprojects/go-selfupdate library with a GitHub
// release source to detect, download, validate, and apply new releases.
//
// Two operational modes are supported:
//
//   - Stdio mode: a single update check runs at startup with a short timeout.
//     If a newer version is found and auto-update is enabled, the binary is
//     replaced and the process re-executes itself.
//
//   - HTTP mode: a background goroutine periodically checks for updates at a
//     configurable interval. When a new version is detected it is downloaded
//     and applied; the operator is advised to restart the server.
//
// Configuration is provided through the [Config] struct, typically populated
// from environment variables (stdio) or CLI flags (HTTP).
//
// Internally the package stages downloads, uses a rename-based replacement path,
// stores a re-exec guard in the environment to avoid update loops, and provides
// platform-specific execution behavior for Unix and Windows.
package autoupdate
