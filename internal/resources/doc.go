// Package resources registers read-only MCP resources for GitLab and server
// metadata.
//
// Resources expose project data, tool manifests, and workflow guides
// through stable gitlab:// URIs. They are
// intended for discovery and context loading rather than mutation, and their
// output is formatted for predictable use by MCP clients and LLMs.
//
// # Resource Families
//
// The package registers several groups of resources:
//
//   - Project and group resources backed by GitLab REST API calls.
//   - Tool manifest resources registered by [RegisterToolSurfaceResources].
//   - Workflow guide resources registered by [RegisterWorkflowGuides].
//
// The public tool manifest resources expose these URI shapes:
//
//	gitlab://tools
//	gitlab://tools/{id}
//
// [Register] wires the GitLab-backed resources into an MCP server.
//
// # Narrowing
//
// A GitLab-backed resource is a second request path to data a tool also
// returns, with the same credential, so the operator's --exclude-tools must
// reach it: [RegisterOptions] carries the excluded catalog actions and
// resourceBackingActions is the table relating the two surfaces. Tool-manifest
// and workflow-guide resources are static documents about this server and are
// never narrowed.
//
// Two controls still do not reach this surface, both because they are applied
// where tools are registered rather than here: the tools/call rate limiter,
// which does not meter resources/read, and the prompt surface, which is a third
// path to the same data and takes no options at all. Both are noted so the
// silence does not read as coverage.
package resources
