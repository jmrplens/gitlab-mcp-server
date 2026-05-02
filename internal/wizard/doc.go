// Package wizard implements the setup wizard that configures GitLab MCP Server
// credentials, binary installation, and IDE client configuration when the
// binary runs interactively instead of as an MCP stdio server.
//
// The package supports CLI, terminal UI, and browser-based flows; detects
// interactive terminals; reads and writes the wizard .env file; resolves
// platform-specific client configuration paths; opens the web UI in a browser;
// and merges MCP server JSON blocks into existing client configuration files.
//
// # Modes
//
// [Run] selects an explicit [UIMode] or cascades through the available user
// interfaces. [RunWebUI] starts the browser-based wizard, [RunTUI] starts the
// terminal UI, and [RunCLI] uses line-oriented prompts through [Prompter].
//
// # Configuration Flow
//
// The wizard collects GitLab credentials and client preferences, then writes the
// local environment file and merges MCP server entries into selected IDE or
// agent configuration files:
//
//	Run
//	    |
//	    v
//	Web UI, TUI, or CLI
//	    |
//	    v
//	.env file and client MCP configuration
package wizard
