// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// RegisterMetaCatalog registers visible meta-tools from the canonical
// action catalog. Each catalog group becomes one visible MCP meta-tool
// whose name matches the group ToolName and whose routes come from the
// group's [actioncatalog.Group.ActionMap]. Read-only groups use
// [toolutil.AddReadOnlyMetaTool] so the registered annotations reflect
// the read-only semantics; mutating groups use [toolutil.AddMetaTool].
// Nil server or catalog inputs are accepted as no-ops.
func RegisterMetaCatalog(server *mcp.Server, catalog *actioncatalog.Catalog) {
	// There is deliberately no nil guard in front of this loop. Either nil is
	// already a no-op one call down — Groups() answers nil for a nil catalog,
	// and AddMetaTool and AddReadOnlyMetaTool both refuse a nil server — so an
	// early return here decided nothing any caller could observe, which is a
	// branch no test can hold to its meaning. The contract is stated by
	// [TestRegisterMetaCatalog_NilInputs] instead, over all three nil
	// combinations, so the day one of those refusals goes away it fails there
	// rather than passing here. Its sibling in RegisterIndividualCatalogTools
	// keeps its guard, since mcp.AddTool dereferences the server it is given.
	calls := MetaActionCalls(catalog)
	for _, group := range catalog.Groups() {
		formatResult := group.FormatResult
		if formatResult == nil {
			formatResult = markdownForResult
		}
		formatResult = toolutil.WithResolvedNextSteps(formatResult, calls)
		if group.ReadOnly {
			toolutil.AddReadOnlyMetaTool(server, group.ToolName, group.Description, group.ActionMap(), group.Icons, formatResult)
			continue
		}
		toolutil.AddMetaTool(server, group.ToolName, group.Description, group.ActionMap(), group.Icons, formatResult)
	}
}

// MetaActionCalls answers how the meta surface calls each action of catalog:
// the meta tool of the action's group with the action's name within it,
// "gitlab_issue with action 'update'" for issue.update (issue 1325). The meta
// tool's action argument refuses a canonical ID, so a next step written as one
// was a call the model could not make as written. An action catalog does not
// hold, which the surface therefore does not serve, is not answered, and the
// step that names it is left as it was written.
func MetaActionCalls(catalog *actioncatalog.Catalog) toolutil.ActionCallResolver {
	calls := make(map[string]string)
	for _, action := range catalog.Actions() {
		calls[string(action.ID)] = action.ToolName + " with action '" + action.Name + "'"
	}
	return func(actionID string) (string, bool) {
		call, ok := calls[actionID]
		return call, ok
	}
}
