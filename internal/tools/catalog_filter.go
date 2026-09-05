// catalog_filter.go narrows an action catalog to what one deployment serves,
// and records what the narrowing removed and on whose decision.
//
// It lives here rather than in cmd/server because two things build a catalog
// this way: the server, once per pool entry, and the end-to-end suite, once
// per mode it drives. When the suite assembled its own copy it added the
// standalone actions before filtering and never told the dynamic registry
// what read-only mode had withheld, so its read-only session answered a
// withheld write with "unknown action" while the shipped binary answered
// "exists but is not available". A test that builds its own copy of the thing
// under test is testing the copy; both now call this.

package tools

import (
	"log/slog"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

// WithheldActions records the catalog actions a filter removed, split by whose
// decision it was. Only the token-scope half is something the caller can act
// on, so the two must not be merged into one message.
type WithheldActions struct {
	ByTokenScope []string
	ByOperator   []string
	// ExcludedByName are the actions --exclude-tools removed. They are kept
	// apart from the two above and never reach the dynamic registry's
	// withheld lists: a withheld action is one the model is told about so it
	// can act on the reason, and naming an excluded tool would both leak the
	// configuration and contradict the exclusion. They are recorded because
	// the tool surface is not the only request path to the same GitLab
	// object, and resources/read has to be narrowed by the same decision.
	ExcludedByName []string
}

// FilterActionCatalog applies a deployment's narrowing to a catalog, in the
// order that keeps the causes apart: the operator's exclusions first, then the
// credential's scopes, then read-only mode, then safe-mode previews. The
// returned bookkeeping names what the scope and read-only steps removed.
func FilterActionCatalog(catalog *actioncatalog.Catalog, cfg *config.ServerConfig) (*actioncatalog.Catalog, WithheldActions, error) {
	var withheld WithheldActions
	// Tools the operator excluded by name are not "withheld": the point of the
	// exclusion is that they do not exist for this deployment, so naming them
	// in an error would both leak the configuration and contradict it.
	filtered := ExcludeFromCatalog(catalog, cfg.ExcludeTools)
	// Resolved against the catalog before any filtering, which is the only
	// place the operator's entries can be resolved at all: --exclude-tools
	// accepts a group name, a tool name or an action ID, and a catalog that
	// already dropped them can no longer map any of the three.
	withheld.ExcludedByName = catalog.ExcludedActionIDs(cfg.ExcludeTools)
	scoped, err := FilterScopeFilteredCatalog(filtered, cfg.TokenScopes)
	if err != nil {
		return nil, WithheldActions{}, err
	}
	withheld.ByTokenScope = RemovedActionKeys(filtered, scoped)
	filtered = scoped
	if cfg.ReadOnly {
		// Filter at action granularity, not group granularity: a domain that
		// mixes reads and writes must keep its read actions reachable instead
		// of disappearing with them.
		readable := filtered.FilterReadOnlyActions()
		removed := RemovedActionKeys(filtered, readable)
		if cfg.ReadOnlyFromTokenScope {
			withheld.ByTokenScope = append(withheld.ByTokenScope, removed...)
		} else {
			withheld.ByOperator = append(withheld.ByOperator, removed...)
		}
		filtered = readable
	}
	if cfg.SafeMode {
		// Same granularity argument: dispatcher tools cover reads and writes
		// alike, so safe mode is applied per action in the catalog rather than
		// by intercepting whole tools.
		filtered = filtered.WithSafeModePreviews()
	}
	return filtered, withheld, nil
}

// ExcludeFromCatalog removes the groups and actions an operator excluded, and
// reports how many actions that was.
//
// The count is the point. Removal already worked on the dynamic and meta
// surfaces, but the only line an operator saw came from the registered-tool
// filter, which counts registered tool names: on the dynamic surface there are
// two of them and neither is ever an exclusion target, so a working exclusion
// logged "excluded=0" and was indistinguishable from one that matched nothing.
func ExcludeFromCatalog(catalog *actioncatalog.Catalog, excludeTools []string) *actioncatalog.Catalog {
	if len(excludeTools) == 0 {
		return catalog
	}
	filtered := catalog.FilterExcludedTools(excludeTools)
	if removed := catalog.CountActions() - filtered.CountActions(); removed > 0 {
		slog.Info("excluded catalog actions by configuration", "excluded", removed, "patterns", excludeTools)
	}
	return filtered
}

// RemovedActionKeys lists every canonical action ID, and every alias resolving
// to one, that `before` carried and `after` does not.
//
// Aliases count because a caller who asked find for an action before the
// narrowing, or who is working from documentation, names the action the way the
// catalog used to: answering only the canonical form leaves the alias reported
// as a typo, which is the misdiagnosis this exists to prevent.
func RemovedActionKeys(before, after *actioncatalog.Catalog) []string {
	if before == nil || after == nil {
		return nil
	}
	kept := make(map[actioncatalog.ActionID]struct{})
	for _, action := range after.Actions() {
		kept[action.ID] = struct{}{}
	}
	var keys []string
	for _, action := range before.Actions() {
		if _, ok := kept[action.ID]; ok {
			continue
		}
		keys = append(keys, string(action.ID))
		keys = append(keys, action.Aliases...)
		// Compatibility aliases resolve in the dynamic registry exactly as the
		// declared ones do, so a caller working from an older action name
		// would otherwise be told the action is unknown, the misdiagnosis
		// this whole path exists to prevent.
		for _, alias := range action.Compatibility.ActionAliases {
			keys = append(keys, alias.Alias)
		}
	}
	return keys
}
