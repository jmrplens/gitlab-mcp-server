package toolvisibility

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"maps"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The MCP methods the fine-grained narrowing acts on.
const (
	methodToolsList = "tools/list"
	methodToolsCall = "tools/call"
)

// ToolActions is, for one registered tool surface, which catalog actions each
// tool runs: the one action of an individual or standalone tool, and every
// action of a meta tool's group, with the routes its action argument is
// resolved through. The dynamic surface's two tools run no action of their
// own and are not in it.
//
// It is what a fine-grained session's tools/list is narrowed by and what a
// call to a registered tool is checked against before the SDK decodes its
// arguments (issue 952). It is computed once per server, when registration is
// done, from the catalog the server registered, so it is shared by every
// credential the server serves and holds nothing of any of them: what differs
// per credential is the authority its client carries, which every read takes
// from the request.
type ToolActions struct {
	single map[string]string
	groups map[string]toolGroup
}

// toolGroup is one meta tool: the routes its action argument names, and the
// canonical IDs of every action it runs.
type toolGroup struct {
	routes toolutil.ActionMap
	ids    []string
}

// NewToolActions indexes the tools a server registered for surface from the
// catalog it registered them from.
//
// An individual tool is mapped to the action registration bound it to, which
// the served call identifier answers: an individual tool name can be declared
// by several actions and only the first in registration order runs under it.
// The standalone utilities the meta and individual surfaces register beside
// their catalogs are mapped by their tool names. On the dynamic surface
// nothing is mapped, since a call names its action through gitlab_execute_action,
// which decides in its own handler.
func NewToolActions(surface string, catalog *actioncatalog.Catalog) *ToolActions {
	actions := &ToolActions{single: map[string]string{}, groups: map[string]toolGroup{}}
	switch surface {
	case config.ToolSurfaceIndividual:
		identifier := gitlabtools.NewServedCallIdentifier(catalog, surface)
		// The identifier knows no empty name, so an action projected as no
		// individual tool is mapped to nothing here.
		for _, action := range catalog.Actions() {
			name := strings.TrimSpace(action.IndividualTool.Name)
			if identity, known := identifier.Identify(name, nil); known {
				actions.single[name] = identity.ActionID
			}
		}
	case config.ToolSurfaceMeta:
		for _, group := range catalog.Groups() {
			ids := make([]string, 0, len(group.Actions))
			for _, action := range group.Actions {
				ids = append(ids, string(action.ID))
			}
			slices.Sort(ids)
			actions.groups[group.ToolName] = toolGroup{routes: group.ActionMap(), ids: ids}
		}
	default:
		return actions
	}
	maps.Copy(actions.single, gitlabtools.StandaloneActionIDs())
	return actions
}

// ActionIDs returns, for each tool that runs catalog actions, their canonical
// IDs, sorted, which is what the gitlab://tools manifest narrows a
// fine-grained session's read by. The map is the caller's to keep.
func (t *ToolActions) ActionIDs() map[string][]string {
	ids := map[string][]string{}
	for name, id := range t.single {
		ids[name] = []string{id}
	}
	for name, group := range t.groups {
		ids[name] = slices.Clone(group.ids)
	}
	return ids
}

// Listed reports whether a session holding authority is listed the tool
// name: an individual or standalone tool when its action is, a meta tool when
// any action of its group is, and a tool that runs no catalog action always. A
// nil authority, which every session but a fine-grained one has, lists every
// tool.
func (t *ToolActions) Listed(authority *finegrained.Authority, name string) bool {
	if authority == nil {
		return true
	}
	if id, single := t.single[name]; single {
		return authority.Decide(id).Listed
	}
	group, grouped := t.groups[name]
	if !grouped {
		return true
	}
	return slices.ContainsFunc(group.ids, func(id string) bool { return authority.Decide(id).Listed })
}

// Filter returns tools without the ones authority is not listed, in their
// order, or tools itself when it removes none.
func (t *ToolActions) Filter(authority *finegrained.Authority, tools []*mcp.Tool) []*mcp.Tool {
	kept := make([]*mcp.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil || t.Listed(authority, tool.Name) {
			kept = append(kept, tool)
		}
	}
	if len(kept) == len(tools) {
		return tools
	}
	return kept
}

// CallAction returns the canonical ID of the action a call to the tool name
// with arguments runs, and false when the tool runs no catalog action or the
// call names none this resolves.
//
// A meta tool's action argument is read alone ([actionArgument]) and resolved
// through the group's routes the way the meta handler resolves it, its
// historical spellings included; the rewrite that depends on the call's other
// parameters (an environment read by name) is the handler's, which checks the
// action it finally runs again. An argument that resolves to nothing is left
// to the handler, which answers it as the unknown action it is.
func (t *ToolActions) CallAction(name string, arguments json.RawMessage) (string, bool) {
	if id, single := t.single[name]; single {
		return id, true
	}
	group, grouped := t.groups[name]
	if !grouped {
		return "", false
	}
	route, routed := group.routes[toolutil.NormalizeActionAlias(actionArgument(arguments), group.routes)]
	if !routed || route.ActionID == "" {
		return "", false
	}
	return route.ActionID, true
}

// actionArgument reads the action member of a tool call's arguments with a
// streaming decoder and decodes no other member, so what a call costs here
// does not grow with what its other arguments carry. Anything that is not an
// object with one string action member reads as "": a call this cannot read is
// left to the handler, which reads it whole and refuses it if it must. A name
// given twice is one such call, since the handler's decoder keeps the last
// and this one would have answered for the first.
func actionArgument(arguments json.RawMessage) string {
	decoder := jsontext.NewDecoder(bytes.NewReader(arguments))
	open, openErr := decoder.ReadToken()
	if openErr != nil {
		return ""
	}
	if open.Kind() != '{' {
		return ""
	}
	action := ""
	for decoder.PeekKind() != '}' {
		name, err := decoder.ReadToken()
		if err != nil {
			return ""
		}
		if name.String() != "action" {
			if decoder.SkipValue() != nil {
				return ""
			}
			continue
		}
		value, err := decoder.ReadToken()
		if err != nil || value.Kind() != '"' {
			return ""
		}
		action = value.String()
	}
	return action
}

// ListingMiddleware narrows a fine-grained session's tools/list to the tools
// [ToolActions.Listed] lists, and passes every other listing through as the
// SDK built it.
//
// It builds a new result rather than changing the SDK's: a copy of the result
// with its tools replaced, so the cursor, the cache scope and _meta survive,
// and the tools themselves are the SDK's own, so the schemas the server
// finalized on them are what is served. A listing the server makes of itself
// is never narrowed ([toolutil.IsInternalInspection]): registration counts,
// narrows and snapshots the whole surface it registered.
//
// actions returns nil until registration is done, which leaves a listing
// unfiltered; the readiness gate holds every listing back until then, and a
// middleware that reads after the inner chain answered, as this one does,
// reads after the gate opened.
func ListingMiddleware(actions func() *ToolActions) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if method != methodToolsList || err != nil {
				return result, err
			}
			listing, isListing := result.(*mcp.ListToolsResult)
			authority := gitlabclient.AuthorityFrom(ctx)
			index := actions()
			if !isListing || listing == nil || authority == nil || index == nil || toolutil.IsInternalInspection(ctx) {
				return result, nil
			}
			narrowed := *listing
			narrowed.Tools = index.Filter(authority, listing.Tools)
			return &narrowed, nil
		}
	}
}

// CallMiddleware answers a fine-grained session's call to a tool whose action
// it may not run with the reason, before the SDK decodes or validates the
// call's arguments, so one cause gets one refusal whatever the arguments
// (INV-012). The refusal is a tool result returned in the dispatcher's place,
// so it carries the resultType the call's revision requires, which the SDK
// adds only to what its own dispatcher answers ([toolutil.LabelForRevision]).
// Every other call passes through, and an action this cannot name from the
// call is left to the dispatcher, which checks the action it runs again
// ([toolutil.FineGrainedRefusal]).
//
// actions returns nil until registration is done, which lets a call through
// to the dispatcher's check.
func CallMiddleware(actions func() *ToolActions) mcp.Middleware {
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
			id, named := index.CallAction(raw.Name, raw.Arguments)
			if !named {
				return next(ctx, method, req)
			}
			if refusal := toolutil.FineGrainedRefusal(ctx, nil, raw.Name, id, ""); refusal != nil {
				return toolutil.LabelForRevision(req, refusal), nil
			}
			return next(ctx, method, req)
		}
	}
}
