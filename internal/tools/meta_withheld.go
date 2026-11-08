// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package tools

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// MetaActionKeys returns, for every meta tool the catalog of a tier and
// instance class can register, each action argument one of its actions
// answers to, lowercased, mapped to that action's canonical ID: the route
// name and every historical spelling the dispatcher resolves
// ([toolutil.NormalizeActionAlias]), for the actions the meta surface starts
// from and the ones the tier removed before that catalog was built.
//
// It is how a meta call naming a withheld action is matched to the record
// that explains it (issue 1282). Those records are keyed by canonical ID, as
// the dynamic surface is asked, while a meta call names an action by its
// group's route name, which a narrowed catalog no longer carries: the action
// is gone from its tool's enum, which is why the call needs answering at all.
// The index is read from the catalogs the narrowing started from, so it holds
// every action a narrowing can have removed, and depends on nothing about
// the credential or the deployment but the tier and the instance class, the
// first two components of every shape key.
func MetaActionKeys(dotcom bool, tier edition.Tier) (map[string]map[string]string, error) {
	base, err := sharedBaseCatalog(dotcom, ActionCatalogOptions{Tier: tier, IncludeMCP: true})
	if err != nil {
		return nil, fmt.Errorf("index the meta action names: %w", err)
	}
	catalogs := []*actioncatalog.Catalog{base}
	if tier < edition.Ultimate {
		removed, removedErr := sharedTierRemoved(dotcom, tier)
		if removedErr != nil {
			return nil, fmt.Errorf("index the meta action names the tier removed: %w", removedErr)
		}
		catalogs = append(catalogs, removed)
	}
	return metaActionKeys(catalogs), nil
}

// metaActionKeys indexes the catalogs' actions under their meta tool by route
// name, and then by every compatibility alias a route carries that no route
// of the tool is named: a name the tool routes is an action of its own, which
// is how the dispatcher reads one too ([toolutil.NormalizeActionAlias]).
func metaActionKeys(catalogs []*actioncatalog.Catalog) map[string]map[string]string {
	keys := map[string]map[string]string{}
	for _, catalog := range catalogs {
		for _, group := range catalog.Groups() {
			names := keys[group.ToolName]
			if names == nil {
				names = map[string]string{}
				keys[group.ToolName] = names
			}
			for name, action := range group.Actions {
				names[strings.ToLower(name)] = string(action.ID)
			}
		}
	}
	for _, catalog := range catalogs {
		for _, group := range catalog.Groups() {
			names := keys[group.ToolName]
			for _, action := range group.Actions {
				for _, alias := range action.Route.CompatibilityAliases {
					if _, routed := names[alias]; !routed {
						names[alias] = string(action.ID)
					}
				}
			}
		}
	}
	return keys
}
