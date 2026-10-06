package actioncatalog

import (
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
)

// ScopeWithheld is one action key a credential's scopes withheld, with the
// scopes it would need and does not carry, kept apart by who asks for them:
// GitLab, which requires a classic API scope for what the action sends, and
// this server, which serves a catalog group that demands a scope (admin_mode)
// only to a credential carrying it. The answer to a withheld action words the
// two apart, because only the first is GitLab's rule: GitLab serves some
// actions of an admin_mode group to any authenticated token, and an instance
// with Admin Mode off asks admin_mode of none. The key is a canonical action
// ID or an alias that resolved to one, as a caller may ask for either.
type ScopeWithheld struct {
	ID string
	// ByGitLab is the classic API scope GitLab requires for what the action
	// sends that the credential lacks: api, or nothing.
	ByGitLab []string
	// ByServer are the scopes the action's catalog group demands before this
	// server serves it that the credential lacks: admin_mode, or nothing.
	ByServer []string
}

// Missing is every scope the credential lacks for the action, sorted and each
// once, which is what a caller reauthorizes with.
func (w ScopeWithheld) Missing() []string {
	missing := slices.Concat(w.ByGitLab, w.ByServer)
	slices.Sort(missing)
	return slices.Compact(missing)
}

// ClassicNeed is the classic scope the action needs: what the generated
// table derived from the requests it sends ([finegrained.Requirement.Classic]),
// or, for an action the table holds no known scope for, the one its read or
// write classification gives, read_api for a read and api for anything else.
// R-GRANT fails the build on an action with no row, so the fallback is what an
// action added since the last generation is judged by until the table is
// regenerated.
func (a Action) ClassicNeed() finegrained.ClassicScope {
	return classicNeed(a.FineGrained, a.ReadOnly)
}

// ClassicNeedOf is [Action.ClassicNeed] for an action known by its canonical
// ID and classification alone, which is how a standalone surface tool is
// held to the same rule as the catalog's actions.
func ClassicNeedOf(id string, readOnly bool) finegrained.ClassicScope {
	return classicNeed(actiongrants.Requirement(id), readOnly)
}

// classicNeed reads a row's classic scope, falling back on the
// classification when the row holds none.
func classicNeed(row *finegrained.Requirement, readOnly bool) finegrained.ClassicScope {
	if row != nil && row.Classic != finegrained.ClassicUnknown {
		return row.Classic
	}
	if readOnly {
		return finegrained.ClassicReadAPI
	}
	return finegrained.ClassicAPI
}

// FilterReachableWith returns a catalog of the actions a token whose
// strongest classic scope is token reaches ([Action.ClassicNeed]), with the
// groups left empty dropped. A kept group is read-only only when every action
// it keeps is, since a token carrying read_api reaches some actions that write
// (one that writes a file on the server's machine, one GitLab authenticates by
// another credential the caller passes) and the meta tool's annotation must
// not claim otherwise.
func (c *Catalog) FilterReachableWith(token finegrained.ClassicScope) *Catalog {
	if c == nil {
		return nil
	}
	filtered := NewCatalog()
	for _, group := range c.Groups() {
		reachable := NewGroup(GroupOptions{
			ToolName:               group.ToolName,
			Title:                  group.Title,
			Description:            group.Description,
			Icons:                  group.Icons,
			ReadOnly:               true,
			FormatResult:           group.FormatResult,
			BaseDomain:             group.BaseDomain,
			EnterpriseOnly:         group.EnterpriseOnly,
			GitLabDotComOnly:       group.GitLabDotComOnly,
			CapabilityRequirements: group.CapabilityRequirements,
			OwnerPackage:           group.OwnerPackage,
			SurfaceKind:            group.SurfaceKind,
		})
		for _, action := range group.ActionsInOrder() {
			if !action.ClassicNeed().ReachableWith(token) {
				continue
			}
			reachable.ReadOnly = reachable.ReadOnly && action.ReadOnly
			reachable.SetAction(action)
		}
		if len(reachable.Actions) == 0 {
			continue
		}
		mustAddCatalogGroup(filtered, reachable, "filter actions reachable with a classic scope")
	}
	return filtered
}
