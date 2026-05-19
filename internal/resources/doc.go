// Package resources registers read-only MCP resources for GitLab and server
// metadata.
//
// Resources expose project data, tool manifests, workflow guides, and MCP
// workspace roots through stable gitlab:// URIs. They are
// intended for discovery and context loading rather than mutation, and their
// output is formatted for predictable use by MCP clients and LLMs.
//
// # Resource Families
//
// The package registers several groups of resources:
//
//   - Project and group resources backed by GitLab REST API calls.
//   - Tool manifest resources registered by [RegisterToolSurfaceResources].
//   - Legacy schema compatibility helpers registered by [RegisterMetaSchemaResources]
//     and [RegisterDynamicSchemaResources] in isolated tests and audits.
//   - Workflow guide resources registered by [RegisterWorkflowGuides].
//   - Workspace root resources registered by [RegisterWorkspaceRoots].
//
// The public tool manifest resources expose these URI shapes:
//
//	gitlab://tools
//	gitlab://tools/{id}
//
// [Register] wires the GitLab-backed resources into an MCP server.
package resources
