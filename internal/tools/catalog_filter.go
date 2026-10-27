// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// WithheldActions records the catalog actions a filter removed, split by whose
// decision it was. Only the token-scope half is something the caller can act
// on, so the two must not be merged into one message, and that half names the
// scopes each action needs that the credential lacks, so the message can say
// which to reauthorize with.
type WithheldActions struct {
	ByTokenScope []actioncatalog.ScopeWithheld
	ByOperator   []string
	// ByTier are the actions the instance's licensing tier removed, each key
	// with the lowest tier that serves its action, recorded by [TierWithheld]
	// rather than by [FilterActionCatalog], since the tier filter runs before
	// any catalog that function sees exists. Only the operator can change the
	// tier, so the answer to one asks the operator, as for ByOperator, and
	// says which tier the action needs (issue 956). No action ExcludedByName
	// is among them.
	ByTier []actioncatalog.TierWithheld
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
// scopes the credential's catalog groups demand (admin_mode), then read-only
// mode, then the API scope a credential carrying read_api and not api reaches
// (ADR-0026), then safe-mode previews. The returned bookkeeping names what the
// scope, read-only and reach steps removed.
//
// Read-only mode runs before the reach step so that an action both would
// remove is reported as the operator's: a write in a read-only deployment is
// not the credential's to fix, and blaming it would send a caller to
// reauthorize for nothing. What the reach step removes after it is what
// GitLab refuses read_api although the catalog classifies it as a read. The
// group step runs before both and removes writes too, so a write it removes
// from a read-only deployment is filed under the operator for the same
// reason, whatever scope the credential lacks besides.
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
	withheld.ByTokenScope, withheld.ByOperator = groupScopeWithheld(filtered, scoped, cfg)
	filtered, withheld = NarrowForReading(scoped, cfg, withheld)
	if cfg.SafeMode {
		// Same granularity argument: dispatcher tools cover reads and writes
		// alike, so safe mode is applied per action in the catalog rather than
		// by intercepting whole tools.
		filtered = filtered.WithSafeModePreviews()
	}
	return filtered, withheld, nil
}

// ExcludeFromCatalog removes the groups and actions an operator excluded,
// reports how many actions that was, and warns about the entries that named
// nothing.
//
// The count is the point. Removal already worked on the dynamic and meta
// surfaces, but the only line an operator saw came from the registered-tool
// filter, which counts registered tool names: on the dynamic surface there are
// two of them and neither is a catalog action, so a working catalog exclusion
// logged "excluded=0" and was indistinguishable from one that matched nothing.
//
// The warning is raised here because every surface's catalog passes through
// this function whenever a shared catalog is built: [FilterActionCatalog] for
// the dynamic and meta surfaces, the individual assembler directly. That is
// once per catalog key rather than once per deployment, and the key carries
// the tier, the instance class and the part of the credential's scopes that
// narrows the catalog, so in HTTP mode, where the tier is detected per pool
// entry unless it is pinned, one deployment can write the warning several
// times, and an entry reported as naming nothing for a Free caller may still
// remove actions for an Ultimate one. An entry this catalog
// does not match may still name a standalone utility, which no surface keeps
// in this catalog, so what is left is put to [ExcludedStandaloneTools] before
// anything is reported. The warning used to be computed against this catalog
// alone. It named every standalone entry, the working ones among them, and
// added a note excusing them all as filtered where they are added, which was
// false of the canonical ID on every surface, so a reader could not tell a
// dead entry from a live one.
//
// One entry is still reported although it removes something: a name of the
// dynamic surface's own two tools, gitlab_find_action and
// gitlab_execute_action, which the pass over registered tools removes by name
// there. This function knows no surface, and on the other two those names do
// name nothing.
//
// It stays a warning rather than a refusal: one configuration is routinely
// reused across Free, Premium and Ultimate instances, and a lower tier's
// catalog legitimately lacks names the same file carries.
func ExcludeFromCatalog(catalog *actioncatalog.Catalog, excludeTools []string) *actioncatalog.Catalog {
	if len(excludeTools) == 0 {
		return catalog
	}
	filtered, unmatched := catalog.FilterExcludedToolNames(excludeTools)
	warnExclusionsNamingNothing(unmatched)
	if removed := catalog.CountActions() - filtered.CountActions(); removed > 0 {
		slog.Info("excluded catalog actions by configuration", "excluded", removed, "patterns", excludeTools)
	}
	return filtered
}

// warnExclusionsNamingNothing logs, in the operator's order, the exclusion
// entries the catalog did not match that no standalone utility answers
// either, and logs nothing when there are none.
func warnExclusionsNamingNothing(unmatched []string) {
	if len(unmatched) == 0 {
		return
	}
	_, namedNothing := ExcludedStandaloneTools(unmatched)
	if len(namedNothing) == 0 {
		return
	}
	slog.Warn("exclude-tools entries matched no group name, tool name or action ID",
		"entries", strings.Join(namedNothing, ", "))
}

// NarrowForReading applies the two narrowings that turn on whether an action
// writes, in the order [FilterActionCatalog] runs them: read-only mode, which
// keeps the actions the catalog classifies as reads and files the rest under
// the operator, then the reach of a credential carrying read_api and not api,
// which keeps the actions GitLab accepts from read_api and files the rest
// under the token's scope. It adds to withheld what each removed and returns
// it with the narrowed catalog.
//
// It is exported for the dynamic surface, which adds the standalone utilities
// after the catalog is filtered and applies these two to the whole catalog
// again so that a guided flow they withhold is reported with its cause rather
// than as unknown. Both steps keep what they already kept, so the actions the
// first pass narrowed are not filed twice.
func NarrowForReading(catalog *actioncatalog.Catalog, cfg *config.ServerConfig, withheld WithheldActions) (*actioncatalog.Catalog, WithheldActions) {
	if cfg.ReadOnly {
		// Filter at action granularity, not group granularity: a domain that
		// mixes reads and writes must keep its read actions reachable instead
		// of disappearing with them.
		readable := catalog.FilterReadOnlyActions()
		withheld.ByOperator = append(withheld.ByOperator, RemovedActionKeys(catalog, readable)...)
		catalog = readable
	}
	if cfg.ReadAPIOnly {
		reachable := catalog.FilterReachableWith(finegrained.ClassicReadAPI)
		withheld.ByTokenScope = append(withheld.ByTokenScope, scopeWithheld(catalog, reachable, cfg)...)
		catalog = reachable
	}
	return catalog, withheld
}

// groupScopeWithheld files what the group step of [FilterActionCatalog]
// removed: under the operator every write of a read-only deployment, which
// read-only mode would withhold whatever the credential carried, and the rest
// under the token's scope as [scopeWithheld] files it.
func groupScopeWithheld(before, after *actioncatalog.Catalog, cfg *config.ServerConfig) (byScope []actioncatalog.ScopeWithheld, byOperator []string) {
	for _, action := range removedActions(before, after) {
		if cfg.ReadOnly && !action.ReadOnly {
			byOperator = append(byOperator, actionKeys(action)...)
			continue
		}
		byScope = append(byScope, actionScopeWithheld(action, cfg)...)
	}
	return byScope, byOperator
}

// scopeWithheld lists, as [RemovedActionKeys] does, every key of an action a
// scope step removed, each with the scopes the credential lacks for it, kept
// apart by who asks for them: api when GitLab requires it for what the action
// sends and the credential was narrowed to read_api, and those the action's
// catalog group demands before this server serves it ([MetaToolScopes]).
func scopeWithheld(before, after *actioncatalog.Catalog, cfg *config.ServerConfig) []actioncatalog.ScopeWithheld {
	var withheld []actioncatalog.ScopeWithheld
	for _, action := range removedActions(before, after) {
		withheld = append(withheld, actionScopeWithheld(action, cfg)...)
	}
	return withheld
}

// actionScopeWithheld is every key of one action, each with the scopes the
// credential lacks for it, as [scopeWithheld] reports them.
func actionScopeWithheld(action actioncatalog.Action, cfg *config.ServerConfig) []actioncatalog.ScopeWithheld {
	byGitLab, byServer := gitlabclient.MissingClassicScopes(action.ClassicNeed(), MetaToolScopes[action.ToolName], cfg.TokenScopes, cfg.ReadAPIOnly)
	keys := actionKeys(action)
	withheld := make([]actioncatalog.ScopeWithheld, 0, len(keys))
	for _, key := range keys {
		withheld = append(withheld, actioncatalog.ScopeWithheld{ID: key, ByGitLab: byGitLab, ByServer: byServer})
	}
	return withheld
}

// TierWithheld lists every key of an action the licensing tier of cfg removes
// from the catalog an instance class starts from, each with the lowest tier
// that serves it, leaving out every action cfg's exclusions name (issue 956).
//
// The tier filter runs while the catalog is built, before anything
// [FilterActionCatalog] records, so an action it removed reached none of the
// withheld bookkeeping: the dynamic surface answered a call for one with
// "unknown action" and near-miss suggestions, which reads as a capability the
// server lacks, and dropped every cross-link to one. Recording them lets it
// answer that the action exists and needs a higher tier, which an operator can
// pin when detection was wrong, and publish the links as it publishes the ones
// the other filters withheld.
//
// An action the operator excluded by name is left out whatever tier it needs,
// as [WithheldActions.ExcludedByName] is kept out of the other two lists:
// naming it back would contradict the exclusion (register row AUT-004). The
// operator's entries are resolved against the removed actions themselves,
// since the catalog the exclusions are otherwise resolved against no longer
// holds them.
//
// The removed actions are read once per tier and instance class for the
// process; at Ultimate nothing is removed and nothing is read.
func TierWithheld(dotcom bool, cfg *config.ServerConfig) ([]actioncatalog.TierWithheld, error) {
	if cfg.Tier >= edition.Ultimate {
		return nil, nil
	}
	removed, err := sharedTierRemoved(dotcom, cfg.Tier)
	if err != nil {
		return nil, err
	}
	excluded := removed.ExcludedActionIDs(cfg.ExcludeTools)
	var withheld []actioncatalog.TierWithheld
	for _, action := range removed.Actions() {
		if _, found := slices.BinarySearch(excluded, string(action.ID)); found {
			continue
		}
		entry := tierWithheldEntry(action, cfg)
		for _, key := range actionKeys(action) {
			entry.ID = key
			withheld = append(withheld, entry)
		}
	}
	return withheld, nil
}

// tierWithheldEntry is what [TierWithheld] records of one action the tier
// removed, but for the key: the tier it needs and what would still withhold it
// from this session once the tier served it, filed as [FilterActionCatalog]
// files a narrowing. A write of a read-only deployment goes under the operator
// whatever scope the credential lacks besides, so read-only mode is named
// alone; otherwise the scopes the credential lacks are named as the scope
// narrowing names them. Without this a call for a Premium write on a
// read-only Free deployment asked the operator to pin the tier, and the pinned
// deployment then answered that it withholds the action, which sent the model
// to the operator twice.
func tierWithheldEntry(action actioncatalog.Action, cfg *config.ServerConfig) actioncatalog.TierWithheld {
	entry := actioncatalog.TierWithheld{Edition: action.Edition}
	if cfg.ReadOnly && !action.ReadOnly {
		entry.ReadOnlyMode = true
		return entry
	}
	entry.ByGitLab, entry.ByServer = gitlabclient.MissingClassicScopes(action.ClassicNeed(), MetaToolScopes[action.ToolName], cfg.TokenScopes, cfg.ReadAPIOnly)
	return entry
}

// sharedTierRemoved is [sharedTierRemovedCatalog], a seam the tests replace to
// reach the failure path of [TierWithheld].
var sharedTierRemoved = sharedTierRemovedCatalog

// sharedTierRemovedCatalog is the catalog of the actions the tier removes,
// built once per tier and instance class and shared like the base catalog.
func sharedTierRemovedCatalog(dotcom bool, tier edition.Tier) (*actioncatalog.Catalog, error) {
	removed, _, err := ShareCatalog("tier-removed|"+BaseCatalogKey(tier, dotcom, false), func() (*actioncatalog.Catalog, WithheldActions, error) {
		built, buildErr := tierRemovedCatalog(UnboundClient(dotcom), tier)
		return built, WithheldActions{}, buildErr
	})
	return removed, err
}

// tierRemovedGroup materializes one group of removed actions, a seam the tests
// replace to reach the failure path of [tierRemovedCatalog].
var tierRemovedGroup = groupFromActionSpecGroup

// tierRemovedCatalog builds a catalog of the actions the tier filter removes
// at tier and nothing else, through the path a served catalog's groups take,
// so each action carries the ID, the aliases, the individual tool name and the
// edition a served one would. It is never served, so it is not validated as
// one: it only names what was removed.
func tierRemovedCatalog(client *gitlabclient.Client, tier edition.Tier) (*actioncatalog.Catalog, error) {
	catalog := actioncatalog.NewCatalog()
	for _, specGroup := range tierRemovedSpecGroups(CollectActionSpecs(client), tier) {
		group, err := tierRemovedGroup(specGroup)
		if err != nil {
			return nil, fmt.Errorf("build tier-removed group %q: %w", specGroup.ToolName, err)
		}
		if err = addFilteredGroup(catalog, group); err != nil {
			return nil, fmt.Errorf("add tier-removed group %q: %w", group.ToolName, err)
		}
	}
	return catalog, nil
}

// RemovedActionKeys lists every canonical action ID, and every alias resolving
// to one, that `before` carried and `after` does not.
//
// Aliases count because a caller who asked find for an action before the
// narrowing, or who is working from documentation, names the action the way the
// catalog used to: answering only the canonical form leaves the alias reported
// as a typo, which is the misdiagnosis this exists to prevent.
func RemovedActionKeys(before, after *actioncatalog.Catalog) []string {
	var keys []string
	for _, action := range removedActions(before, after) {
		keys = append(keys, actionKeys(action)...)
	}
	return keys
}

// removedActions lists the actions `before` carried and `after` does not.
func removedActions(before, after *actioncatalog.Catalog) []actioncatalog.Action {
	if before == nil || after == nil {
		return nil
	}
	kept := make(map[actioncatalog.ActionID]struct{})
	for _, action := range after.Actions() {
		kept[action.ID] = struct{}{}
	}
	var removed []actioncatalog.Action
	for _, action := range before.Actions() {
		if _, ok := kept[action.ID]; !ok {
			removed = append(removed, action)
		}
	}
	return removed
}

// actionKeys are the names a caller may ask for an action by: its canonical
// ID and every alias that resolves to it.
func actionKeys(action actioncatalog.Action) []string {
	keys := append([]string{string(action.ID)}, action.Aliases...)
	// Compatibility aliases resolve in the dynamic registry exactly as the
	// declared ones do, so a caller working from an older action name would
	// otherwise be told the action is unknown, the misdiagnosis this whole
	// path exists to prevent.
	for _, alias := range action.Compatibility.ActionAliases {
		keys = append(keys, alias.Alias)
	}
	return keys
}
