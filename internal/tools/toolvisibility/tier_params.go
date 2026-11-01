// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolvisibility

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ServedAt records the tier the server was built for and whether an operator
// pinned it, which a refusal of a param the tier left out of an action's
// schema is worded with ([ToolActions.TierPrunedRefusal]), and which a call
// the middleware lets through carries for the meta dispatcher
// ([toolutil.WithSessionTier]). It sets the index in place and returns it,
// and a nil index is returned as it came.
func (t *ToolActions) ServedAt(tier edition.Tier, pinned bool) *ToolActions {
	if t == nil {
		return nil
	}
	t.tier, t.tierPinned, t.tierKnown = tier, pinned, true
	return t
}

// TierPrunedRefusal answers a call to a registered individual or meta tool
// that sends params its action's tier left out of the served schema
// ([toolutil.TierPrunedParams]), with the tier each needs (issue 1239). The
// SDK validates an individual tool's arguments, and a meta tool's in the full
// and compact modes its definition does, before any handler runs, and both
// would refuse such a param as an unexpected property, which reads as a typo
// of a param this session can never send; on the default opaque mode nothing
// refused it, and it was sent.
//
// The call's names are read with a streaming decoder first, the params of a
// meta call being the members of its params object, and the params are
// decoded whole only when one of those names is a pruned one, so a call
// sending none costs a scan of its names. It is nil for every other call, a
// tool this index runs no route of, and an index nothing told the tier.
func (t *ToolActions) TierPrunedRefusal(name string, arguments json.RawMessage) *mcp.CallToolResult {
	if !t.tierKnown {
		return nil
	}
	route, callName, params := t.paramsRoute(name, arguments)
	if !namesAny(params, route.TierPruned) {
		return nil
	}
	decoded := map[string]any{}
	if json.Unmarshal(params, &decoded) != nil {
		return nil
	}
	return toolutil.TierPrunedParamsRefusal(callName, route, toolutil.TierPrunedParams(route, decoded), t.tier, t.tierPinned, true)
}

// paramsRoute is the route a call to the named tool runs, the name its
// refusals call it by, and the raw JSON of its params: an individual tool's
// arguments whole, a meta tool's params member under the route its action
// argument resolves to. A call to no route of this index gets the zero
// route, which prunes nothing, and no params.
func (t *ToolActions) paramsRoute(name string, arguments json.RawMessage) (route toolutil.ActionRoute, callName string, params json.RawMessage) {
	if singleRoute, single := t.singleRoutes[name]; single {
		return singleRoute, name, arguments
	}
	group, grouped := t.groups[name]
	if !grouped {
		return toolutil.ActionRoute{}, "", nil
	}
	action := toolutil.NormalizeActionAlias(actionArgument(arguments), group.routes)
	route, routed := group.routes[action]
	if !routed {
		return toolutil.ActionRoute{}, "", nil
	}
	return route, name + "/" + action, paramsMember(arguments)
}

// namesAny reports whether the JSON object raw has a member named in names,
// reading the member names with a streaming decoder and decoding no value.
// Anything that is not such an object reads as naming none.
func namesAny(raw json.RawMessage, names map[string]string) bool {
	if len(names) == 0 {
		return false
	}
	found := false
	eachMember(raw, func(name string, decoder *jsontext.Decoder) bool {
		_, found = names[name]
		return !found && decoder.SkipValue() == nil
	})
	return found
}

// paramsMember returns the raw JSON of the params member of the meta call
// arguments raw, or nil when raw is not an object this can read whole or has
// no such member. A name given twice is such an object: the handler's
// decoder keeps the last value and this one stops at the second name, so the
// call is left to the handler, as [actionArgument] leaves it.
func paramsMember(raw json.RawMessage) json.RawMessage {
	var value json.RawMessage
	read := eachMember(raw, func(name string, decoder *jsontext.Decoder) bool {
		if name != "params" {
			return decoder.SkipValue() == nil
		}
		body, err := decoder.ReadValue()
		value = json.RawMessage(bytes.Clone(body))
		return err == nil
	})
	if !read {
		return nil
	}
	return value
}

// eachMember calls visit with each member name of the JSON object raw and
// the decoder positioned at its value, which visit must read or skip, until
// visit returns false or the object ends, and reports whether it read the
// object to its end. Anything that is not an object is visited for nothing.
func eachMember(raw json.RawMessage, visit func(name string, decoder *jsontext.Decoder) bool) bool {
	decoder := jsontext.NewDecoder(bytes.NewReader(raw))
	if open, err := decoder.ReadToken(); err != nil || open.Kind() != '{' {
		return false
	}
	for decoder.PeekKind() == '"' {
		name, err := decoder.ReadToken()
		if err != nil || !visit(name.String(), decoder) {
			return false
		}
	}
	_, err := decoder.ReadToken()
	return err == nil
}
