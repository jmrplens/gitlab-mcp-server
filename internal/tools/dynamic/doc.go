// Package dynamic implements low-token GitLab MCP tool surfaces over the
// canonical action catalog.
//
// Dynamic mode exposes a small discovery and execution interface instead of
// advertising every GitLab operation as an MCP tool. The stable dynamic and
// dynamic-3 surfaces register gitlab_search_tools, gitlab_describe_tools, and
// gitlab_execute_tool. The parked dynamic-2 comparison surface registers
// gitlab_find_action and gitlab_execute_tool.
//
// The package builds a deterministic search index from actioncatalog.Catalog,
// resolves canonical domain.action IDs and aliases, returns exact schemas on
// demand, and dispatches execution through the same ActionRoute metadata used
// by meta-tools. It does not wrap or call the visible individual MCP tools.
package dynamic
