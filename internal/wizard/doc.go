// Package wizard implements the setup wizard that configures GitLab MCP Server
// credentials, binary installation, and IDE client configuration when the
// binary runs interactively instead of as an MCP stdio server.
//
// The package supports CLI, terminal UI, and browser-based flows; detects
// interactive terminals; reads and writes the wizard .env file; resolves
// platform-specific client configuration paths; opens the web UI in a browser;
// and merges MCP server JSON blocks into existing client configuration files.
package wizard
