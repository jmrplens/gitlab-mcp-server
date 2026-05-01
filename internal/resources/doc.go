// Package resources registers read-only MCP resources for GitLab and server
// metadata.
//
// Resources expose project data, meta-tool action schemas, workflow guides,
// and MCP workspace roots through stable gitlab:// URIs. They are intended for
// discovery and context loading rather than mutation, and their output is
// formatted for predictable use by MCP clients and LLMs.
package resources
