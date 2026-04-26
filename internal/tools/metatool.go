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

// actionRoute is an alias for [toolutil.ActionRoute].
type actionRoute = toolutil.ActionRoute

// actionMap is an alias for [toolutil.ActionMap].
type actionMap = toolutil.ActionMap

// route and destructiveRoute are constructors for ActionRoute.
var (
	route            = toolutil.Route
	destructiveRoute = toolutil.DestructiveRoute
)

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

// Composite wrappers: combine wrapping + metadata in a single call.

// routeAction wraps a typed function as a non-destructive ActionRoute.
func routeAction[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) actionRoute {
	return toolutil.RouteAction(client, fn)
}

// routeVoidAction wraps a typed void function as a non-destructive ActionRoute.
func routeVoidAction[T any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) error) actionRoute {
	return toolutil.RouteVoidAction(client, fn)
}

// routeActionWithRequest wraps a typed function that needs the MCP request as a non-destructive ActionRoute.
func routeActionWithRequest[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error)) actionRoute {
	return toolutil.RouteActionWithRequest(client, fn)
}

// destructiveAction wraps a typed function as a destructive ActionRoute.
func destructiveAction[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) actionRoute {
	return toolutil.DestructiveAction(client, fn)
}

// destructiveVoidAction wraps a typed void function as a destructive ActionRoute.
func destructiveVoidAction[T any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) error) actionRoute {
	return toolutil.DestructiveVoidAction(client, fn)
}

// makeMetaHandler creates a meta-tool handler using markdownForResult as the formatter.
func makeMetaHandler(toolName string, routes actionMap) func(ctx context.Context, req *mcp.CallToolRequest, input MetaToolInput) (*mcp.CallToolResult, any, error) {
	return toolutil.MakeMetaHandler(toolName, routes, markdownForResult)
}

// addMetaTool registers a meta-tool with annotations derived from routes.
// If ANY route is destructive, the tool gets DestructiveHint: true.
// If NO route is destructive, it gets NonDestructiveMetaAnnotations.
func addMetaTool(server *mcp.Server, name, desc string, routes actionMap, icons []mcp.Icon) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        name,
		Title:       toolutil.TitleFromName(name),
		Description: desc,
		Annotations: toolutil.DeriveAnnotationsWithTitle(name, routes),
		Icons:       icons,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, makeMetaHandler(name, routes))
}

// addReadOnlyMetaTool registers a meta-tool where all actions are read-only
// (list/get/search only). Uses ReadOnlyMetaAnnotations with ReadOnlyHint: true.
func addReadOnlyMetaTool(server *mcp.Server, name, desc string, routes actionMap, icons []mcp.Icon) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        name,
		Title:       toolutil.TitleFromName(name),
		Description: desc,
		Annotations: toolutil.ReadOnlyMetaAnnotationsWithTitle(name),
		Icons:       icons,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, makeMetaHandler(name, routes))
}

var validActionsString = toolutil.ValidActionsString
