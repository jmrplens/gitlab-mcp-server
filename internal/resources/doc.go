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
// The prompt surface, which was the third path to the same data, now takes the
// same options: [github.com/jmrplens/gitlab-mcp-server/v2/internal/prompts.RegisterOptions]
// carries the excluded actions and that package keeps its own prompt-to-action
// table.
//
// The rate limiter applied where tools are registered meters this surface too:
// resources/read, resources/subscribe and subscriptions/listen draw on the
// same bucket as tools/call, as prompts/get does, so none of them is an
// unmetered proxy to GitLab. A refused request here is a JSON-RPC error
// carrying the code that mirrors HTTP 429, since a resource result has no
// error flag of its own. See [toolutil.AttachRateLimit].
package resources
