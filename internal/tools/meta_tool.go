package tools

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// MetaToolInput is an alias for [toolutil.MetaToolInput] kept for
// backward compatibility with sub-package code that imports the alias
// from the tools namespace.
type MetaToolInput = toolutil.MetaToolInput

// actionFunc is an alias for [toolutil.ActionFunc]. Re-exported so the
// tools package can type its internal helpers without reaching into
// toolutil.
type actionFunc = toolutil.ActionFunc

// actionRoute is an alias for [toolutil.ActionRoute]. Re-exported so the
// tools package can type its internal helpers without reaching into
// toolutil.
type actionRoute = toolutil.ActionRoute

// actionMap is an alias for [toolutil.ActionMap]. Re-exported so the
// tools package can type its internal helpers without reaching into
// toolutil.
type actionMap = toolutil.ActionMap

// route constructs a [toolutil.ActionRoute].
//
// A destructiveRoute constructor sat beside it until the self-update actions
// were removed: apply_update was the only route built here that needed marking
// destructive, and every remaining destructive action carries the flag through
// its own ActionSpec instead.
var route = toolutil.Route

// unmarshalParams decodes a params map into a typed Go value of type T
// using [toolutil.UnmarshalParams]. Generic helper shared by every
// meta-tool registration.
func unmarshalParams[T any](params map[string]any) (T, error) {
	return toolutil.UnmarshalParams[T](params)
}

// wrapAction turns a typed client-and-input handler into a
// [toolutil.ActionFunc] for meta-tool dispatch.
func wrapAction[T, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) actionFunc {
	return toolutil.WrapAction(client, fn)
}

// wrapVoidAction is the void-returning variant of [wrapAction] for
// meta-tool handlers that return only an error.
func wrapVoidAction[T any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) error) actionFunc {
	return toolutil.WrapVoidAction(client, fn)
}

// Composite wrappers: combine wrapping + metadata in a single call.

// routeAction wraps a typed function as a non-destructive
// [toolutil.ActionRoute]. The result is safe to assign directly to a
// meta-tool routes map.
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

// MetaParamSchema reports the strategy [SetMetaParamSchema] selected:
// "opaque", "compact" or "full".
func MetaParamSchema() string {
	return toolutil.MetaParamSchemaMode()
}

// SetMetaParamSchemaScoped selects the meta-tool input schema strategy and
// returns a restore function for tests that temporarily override the global
// mode. Valid modes match SetMetaParamSchema: "opaque", "compact", and
// "full". Use it with defer, for example:
//
//	defer SetMetaParamSchemaScoped("full")()
func SetMetaParamSchemaScoped(mode string) func() {
	return toolutil.SetMetaParamSchemaModeScoped(mode)
}
