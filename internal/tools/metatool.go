// metatool.go re-exports meta-tool dispatch utilities from toolutil
// and provides makeMetaHandler which uses the domain-coupled markdownForResult.

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// MetaToolInput is an alias for [toolutil.MetaToolInput].
type MetaToolInput = toolutil.MetaToolInput

// actionFunc is an alias for [toolutil.ActionFunc].
type actionFunc = toolutil.ActionFunc

// Generic function wrappers — Go does not support generic vars.

// unmarshalParams performs the unmarshal params operation using the GitLab API and returns [T].
func unmarshalParams[T any](params map[string]any) (T, error) {
	return toolutil.UnmarshalParams[T](params)
}

// wrapAction is an internal helper for the tools package.
func wrapAction[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) actionFunc {
	return toolutil.WrapAction(client, fn)
}

// wrapVoidAction is an internal helper for the tools package.
func wrapVoidAction[T any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) error) actionFunc {
	return toolutil.WrapVoidAction(client, fn)
}

// wrapActionWithRequest wraps a handler that also requires the MCP request
// (e.g., for progress tracking). The request is extracted from context.
func wrapActionWithRequest[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error)) actionFunc {
	return toolutil.WrapActionWithRequest(client, fn)
}

// metaAnnotations are annotations for meta-tools that combine read/write/delete.
var metaAnnotations = toolutil.MetaAnnotations

// readOnlyMetaAnnotations are for meta-tools with only list/get/search actions.
var readOnlyMetaAnnotations = toolutil.ReadOnlyMetaAnnotations

// makeMetaHandler creates a meta-tool handler using markdownForResult as the formatter.
func makeMetaHandler(toolName string, routes map[string]actionFunc) func(ctx context.Context, req *mcp.CallToolRequest, input MetaToolInput) (*mcp.CallToolResult, any, error) {
	return toolutil.MakeMetaHandler(toolName, routes, markdownForResult)
}

// addMetaTool registers a meta-tool with an InputSchema containing an enum
// constraint on the action field derived from the routes map.
func addMetaTool(server *mcp.Server, name, desc string, routes map[string]actionFunc, annotations *mcp.ToolAnnotations, icons []mcp.Icon) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        name,
		Title:       toolutil.TitleFromName(name),
		Description: desc,
		Annotations: annotations,
		Icons:       icons,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, makeMetaHandler(name, routes))
}

var validActionsString = toolutil.ValidActionsString
