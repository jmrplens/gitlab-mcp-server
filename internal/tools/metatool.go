package tools

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// unmarshalParams handles unmarshal params and returns [T].
func unmarshalParams[T any](params map[string]any) (T, error) {
	return toolutil.UnmarshalParams[T](params)
}

// wrapAction resolves wrap action for evaluator execution.
func wrapAction[T, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) actionFunc {
	return toolutil.WrapAction(client, fn)
}

// wrapVoidAction resolves wrap void action for evaluator execution.
func wrapVoidAction[T any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) error) actionFunc {
	return toolutil.WrapVoidAction(client, fn)
}

// Composite wrappers: combine wrapping + metadata in a single call.

// routeAction wraps a typed function as a non-destructive ActionRoute.
func routeAction[T, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) actionRoute {
	return toolutil.RouteAction(client, fn)
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

// SetMetaParamSchemaScoped selects the meta-tool input schema strategy and
// returns a restore function for tests that temporarily override the global
// mode. Valid modes match SetMetaParamSchema: "opaque", "compact", and
// "full". Use it with defer, for example:
// defer SetMetaParamSchemaScoped("full")().
func SetMetaParamSchemaScoped(mode string) func() {
	return toolutil.SetMetaParamSchemaModeScoped(mode)
}
