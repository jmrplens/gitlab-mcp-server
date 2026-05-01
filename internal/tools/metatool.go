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

// destructiveVoidActionWithRequest wraps a request-aware void function as a destructive ActionRoute.
func destructiveVoidActionWithRequest[T any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) error) actionRoute {
	return toolutil.DestructiveVoidActionWithRequest(client, fn)
}

// addMetaTool registers a meta-tool with annotations derived from routes.
// If ANY route is destructive, the tool gets DestructiveHint: true.
// If NO route is destructive, it gets NonDestructiveMetaAnnotations.
func addMetaTool(server *mcp.Server, name, desc string, routes actionMap, icons []mcp.Icon) {
	toolutil.AddMetaTool(server, name, desc, routes, icons, markdownForResult)
}

// addReadOnlyMetaTool registers a meta-tool where all actions are read-only
// (list/get/search only). Uses ReadOnlyMetaAnnotations with ReadOnlyHint: true.
func addReadOnlyMetaTool(server *mcp.Server, name, desc string, routes actionMap, icons []mcp.Icon) {
	toolutil.AddReadOnlyMetaTool(server, name, desc, routes, icons, markdownForResult)
}

// validActionsString exposes the shared action-list formatter for package
// tests while keeping registration code on the local tools namespace.
var validActionsString = toolutil.ValidActionsString

// SetMetaParamSchema selects the meta-tool input schema strategy used by all
// meta-tool registrations in this package and its sub-packages. Accepts
// "opaque" (default), "compact", or "full". Unknown values are coerced to
// opaque so misconfiguration cannot break tools/list. Must be called before
// [RegisterAllMeta].
func SetMetaParamSchema(mode string) {
	toolutil.SetMetaParamSchemaMode(mode)
}
