// Package resources registers read-only MCP resources for GitLab and server
// metadata.
//
// Resources expose project data, meta-tool action schemas, workflow guides,
// and MCP workspace roots through stable gitlab:// URIs. They are intended for
// discovery and context loading rather than mutation, and their output is
// formatted for predictable use by MCP clients and LLMs.
//
// # Resource Families
//
// The package registers several groups of resources:
//
//   - Project and group resources backed by GitLab REST API calls.
//   - Meta-tool schema resources registered by [RegisterMetaSchemaResources].
//   - Workflow guide resources registered by [RegisterWorkflowGuides].
//   - Workspace root resources registered by [RegisterWorkspaceRoots].
//
// The meta-schema resources expose these URI shapes:
//
//	gitlab://schema/meta/
//	gitlab://schema/meta/{tool}/{action}
//
// [Register] wires the GitLab-backed resources into an MCP server.
package resources
