// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// TierPrunedFields returns the input fields of rt, by JSON name, whose
// `tier:` struct tag ([FieldTiers]) asks for a tier above tier, each with the
// tier it asks for ("premium" or "ultimate"), and nil when there are none.
// They are what the catalog prunes from the action's served input schema at
// that tier, computed once when the catalog is built and carried on the route
// ([ActionRoute.TierPruned]), so a call sending one is refused with the tier
// it needs rather than sent, or refused as unknown (issue 1239).
func TierPrunedFields(rt reflect.Type, tier edition.Tier) map[string]string {
	var pruned map[string]string
	for name, fieldTier := range FieldTiers(rt) {
		if edition.TierFromEdition(fieldTier) <= tier {
			continue
		}
		if pruned == nil {
			pruned = map[string]string{}
		}
		pruned[name] = fieldTier
	}
	return pruned
}

// TierPrunedParams returns, sorted, the params of a call to route that the
// served input schema left out for a higher tier ([ActionRoute.TierPruned]),
// read after the compatibility aliases the handler accepts are resolved
// against the route's input type, so an alias of a field the tier serves is
// never refused. It is nil when the call sends none.
func TierPrunedParams(route ActionRoute, params map[string]any) []string {
	if len(route.TierPruned) == 0 {
		return nil
	}
	var pruned []string
	for name := range normalizeParamAliases(stripReservedKeys(params), route.InputType) {
		if _, higher := route.TierPruned[name]; higher {
			pruned = append(pruned, name)
		}
	}
	slices.Sort(pruned)
	return pruned
}

// TierPrunedParamsRefusal is the answer to a call sending params its action's
// tier left out of the served schema: what each needs, the tier the session
// was built for and how the operator corrects a tier, in the words every tier
// refusal uses ([TierRequired], [SessionTierClause], [TierWayOut]), worded for
// a detected tier as well as a pinned one. callName names the call as the
// surface's other refusals do ("gitlab_project/board_update",
// "gitlab_execute_action/project.board_update", or an individual tool's name).
// known is false when nothing told the caller the session's tier, which a
// handler driven without the call middleware in front of it is: the answer
// then says what each param needs and that this session's tier does not serve
// it, and asks the operator to set the tier the instance has. It is nil when
// pruned names none.
func TierPrunedParamsRefusal(callName string, route ActionRoute, pruned []string, session edition.Tier, pinned, known bool) *mcp.CallToolResult {
	if len(pruned) == 0 {
		return nil
	}
	needs := make([]string, 0, len(pruned))
	for _, name := range pruned {
		needs = append(needs, fmt.Sprintf("param %q needs %s", name, TierRequired(route.TierPruned[name])))
	}
	pronoun := "it"
	if len(pruned) > 1 {
		pronoun = "them"
	}
	clause := "this session's tier does not serve " + pronoun
	wayOut := "If the instance is licensed for " + pronoun + ", ask the operator to configure the tier it has with GITLAB_MCP_TIER or --tier."
	if known {
		clause = SessionTierClause(session, pinned)
		wayOut = TierWayOut(pinned)
	}
	return ErrorResult(fmt.Sprintf("%s: %s, and %s, so the action's schema at this tier leaves %s out and nothing was sent. %s Send the call without %s to run the action at this tier.",
		callName, strings.Join(needs, " and "), clause, pronoun, wayOut, pronoun))
}

// TierRequired names the lowest tier an action or a field is served at, as
// its spec or tag spells it ("premium" or "ultimate"), the way every tier
// refusal names it: "GitLab Premium or Ultimate", or "GitLab Ultimate".
func TierRequired(needs string) string {
	if edition.TierFromEdition(needs) == edition.Ultimate {
		return "GitLab Ultimate"
	}
	return "GitLab Premium or Ultimate"
}

// SessionTierClause says which tier the session was built for and how it was
// chosen: "this deployment is configured for GitLab Free" when an operator
// pinned it, "this instance's tier was detected as GitLab Free" when the
// server detected it, which is what lets the answer allow that the detection
// may be wrong (issue 956).
func SessionTierClause(session edition.Tier, pinned bool) string {
	if pinned {
		return "this deployment is configured for GitLab " + TierName(session)
	}
	return "this instance's tier was detected as GitLab " + TierName(session)
}

// TierWayOut is the way out every tier refusal ends with: only the operator
// can change the tier, so it asks the operator, to configure the tier the
// instance has where it was pinned, and to pin it where the detection was
// wrong.
func TierWayOut(pinned bool) string {
	if pinned {
		return "If the instance is licensed for it, ask the operator to configure the tier it has with GITLAB_MCP_TIER or --tier."
	}
	return "If the instance is licensed for it, the detection was wrong: ask the operator to pin the tier with GITLAB_MCP_TIER or --tier."
}

// TierName spells a tier as GitLab names it in prose.
func TierName(tier edition.Tier) string {
	switch tier {
	case edition.Premium:
		return "Premium"
	case edition.Ultimate:
		return "Ultimate"
	default:
		return "Free"
	}
}

// sessionTierKey is the context key the session's tier travels under.
type sessionTierKey struct{}

// sessionTier is the tier a session was built for and whether an operator
// pinned it.
type sessionTier struct {
	tier   edition.Tier
	pinned bool
}

// WithSessionTier returns ctx carrying the tier the session was built for and
// whether an operator pinned it, which the meta dispatcher words a refusal of
// a tier-pruned param with. The call middleware installs it on every call it
// passes, since the dispatcher is registered from a catalog shared by servers
// whose tier was pinned and servers whose tier was detected.
func WithSessionTier(ctx context.Context, tier edition.Tier, pinned bool) context.Context {
	return context.WithValue(ctx, sessionTierKey{}, sessionTier{tier: tier, pinned: pinned})
}

// sessionTierFrom is the tier ctx carries, and false when it carries none.
func sessionTierFrom(ctx context.Context) (tier edition.Tier, pinned, known bool) {
	carried, known := ctx.Value(sessionTierKey{}).(sessionTier)
	return carried.tier, carried.pinned, known
}
