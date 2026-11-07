// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolvisibility

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Withhold attaches to a meta surface's index what the session's
// configuration withheld from its tools' groups, so a call naming one of
// those actions is answered with its cause ([ToolActions.WithheldMessage])
// rather than refused by the SDK as a value outside the tool's action enum
// (issue 1282). keys is [gitlabtools.MetaActionKeys] for the server's tier and
// instance class, and withheld the record of what its filters removed, the
// one the dynamic surface answers from.
//
// It sets the index in place and returns it, so it is chained onto
// [NewToolActions] before the index is published, and a nil index or nil
// record leaves it answering nothing.
func (t *ToolActions) Withhold(keys map[string]map[string]string, withheld *actioncatalog.Withheld) *ToolActions {
	if t == nil || withheld == nil {
		return t
	}
	t.withheldKeys = keys
	t.withheld = withheld
	return t
}

// WithheldMessage answers a call to a registered meta tool whose action
// argument names an action the session's configuration withheld from the
// tool's group: the tier, the credential's scopes or the operator's
// read-only mode. The answer is the dynamic surface's, with the tool's name
// and the action as the call spelled it ([actioncatalog.Withheld.Message]).
//
// Everything else is false and left where it was going. A tool that is no
// meta group of this server is one: a group withheld whole was never
// registered, and the SDK's "unknown tool" is the answer to a call for it,
// as it is for every name this server does not serve. So are an action the
// group serves, under any spelling the dispatcher resolves, an action the
// operator excluded by name, which no record holds so that no answer names
// it (AUT-004), and an action argument this cannot read.
func (t *ToolActions) WithheldMessage(name string, arguments json.RawMessage) (string, bool) {
	group, grouped := t.groups[name]
	if !grouped {
		return "", false
	}
	action := actionArgument(arguments)
	if _, served := group.routes[toolutil.NormalizeActionAlias(action, group.routes)]; served {
		return "", false
	}
	return t.withheldAnswer(name, action)
}

// withheldAnswer is the answer for an action of the named tool that the
// tool's routes do not carry: its cause when the session withheld it, false
// otherwise, and false for every action of an index nothing was attached to.
// It is what the meta dispatcher consults too, through
// [toolutil.WithWithheldAnswer], for the action it arrives at. No key is
// empty, so an action argument this could not read is never answered.
func (t *ToolActions) withheldAnswer(name, action string) (string, bool) {
	if t.withheld == nil {
		return "", false
	}
	id, known := t.withheldKeys[name][strings.ToLower(strings.TrimSpace(action))]
	if !known {
		return "", false
	}
	return t.withheld.Message(name, action, id)
}

// NarrowedCallMiddleware answers a call the session's narrowing explains
// before the SDK validates its arguments against the tool's input schema,
// which would otherwise refuse it as invalid params and say nothing about
// why: today a meta tool's call naming an action its group withheld
// ([ToolActions.WithheldMessage], issue 1282). The answer is a tool result
// returned in the dispatcher's place, so it carries the resultType the
// call's revision requires ([toolutil.LabelForRevision]), and it is counted
// as the dynamic surface counts the same answer, as an unknown action.
//
// A call it lets through carries the same answer for the meta dispatcher,
// which consults it before it calls an action unknown. actions returns nil
// until registration is done, which lets every call through.
func NarrowedCallMiddleware(actions func() *ToolActions) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != methodToolsCall {
				return next(ctx, method, req)
			}
			raw, _ := req.GetParams().(*mcp.CallToolParamsRaw)
			index := actions()
			if raw == nil || index == nil {
				return next(ctx, method, req)
			}
			if message, withheld := index.WithheldMessage(raw.Name, raw.Arguments); withheld {
				toolutil.LogToolRefusal(ctx, nil, raw.Name+"/"+actionArgument(raw.Arguments), toolutil.RefusalUnknownAction)
				return toolutil.LabelForRevision(req, toolutil.ErrorResult(message)), nil
			}
			return next(toolutil.WithWithheldAnswer(ctx, index.withheldAnswer), method, req)
		}
	}
}
