package actioncatalog

import (
	"log/slog"
	"sort"
	"strings"
)

// FilterExcludedToolNames returns a cloned catalog without the tools and
// actions named by excludeTools, together with the entries that matched
// nothing in this catalog.
//
// An entry is matched against three names, because three names reach the same
// action depending on which surface is registered:
//
//   - a group's meta-tool name ("gitlab_issue"), which removes the group and
//     every action in it;
//   - an action's individual tool name ("gitlab_issue_delete"), which removes
//     that one action;
//   - the canonical action ID ("issue.delete"), which the dynamic surface takes
//     directly, likewise.
//
// Only the group name matched before. The default surface is dynamic, where the
// two visible tools reach every action by canonical ID, so an operator who
// hardened a deployment with EXCLUDE_TOOLS=gitlab_issue_delete got a server
// that still executed it and a startup log that said nothing was excluded.
//
// Aliases are deliberately not matched. Alias resolution is fuzzy by design,
// and an exclusion that removes more than the operator named is a different
// defect rather than a fix for this one.
//
// A group whose every action was excluded is dropped: a dispatcher that can
// only refuse is worse than no tool at all. A group that was already empty is
// left alone so catalog validation can still report it.
func (c *Catalog) FilterExcludedToolNames(excludeTools []string) (filtered *Catalog, unmatched []string) {
	if c == nil {
		return nil, nil
	}
	patterns := excludedToolPatterns(excludeTools)
	if len(patterns) == 0 {
		return c.Clone(), nil
	}
	matched := make(map[string]struct{}, len(patterns))
	filtered = NewCatalog()
	for _, group := range c.Groups() {
		if _, ok := patterns[group.ToolName]; ok {
			matched[group.ToolName] = struct{}{}
			// The group takes its actions with it, so an entry naming one of
			// them named something too and must not be reported as unmatched.
			for _, action := range group.ActionsInOrder() {
				recordExcludedActionPatterns(action, patterns, matched)
			}
			continue
		}
		kept, removedAny := groupWithoutExcludedActions(group, patterns, matched)
		if removedAny && len(kept.Actions) == 0 {
			continue
		}
		mustAddCatalogGroup(filtered, kept, "filter excluded tools")
	}
	return filtered, unmatchedExcludedPatterns(excludeTools, matched)
}

// FilterExcludedTools returns a cloned catalog without the tools and actions
// named by excludeTools.
//
// Entries that name nothing on this surface are logged at WARN rather than
// refused: one configuration is routinely reused across Free, Premium and
// Ultimate instances, and a lower-tier catalog legitimately lacks tools the
// same file names. The warning is deliberately operator-facing only — the
// excluded actions must stay out of the client-facing withheld list, or the
// dynamic registry would name back the very actions the operator removed.
//
// Standalone utility tools (gitlab_discover_project, gitlab_interactive_*) are
// not in this catalog; they are filtered where they are added, so the warning
// names them too. Use [Catalog.FilterExcludedToolNames] when the caller can
// account for those before reporting.
func (c *Catalog) FilterExcludedTools(excludeTools []string) *Catalog {
	filtered, unmatched := c.FilterExcludedToolNames(excludeTools)
	if len(unmatched) > 0 {
		slog.Warn(
			"exclude-tools entries matched no catalog group, individual tool name or action ID",
			"entries", strings.Join(unmatched, ", "),
			"note", "standalone utility tools such as gitlab_discover_project are filtered where they are added, not here",
		)
	}
	return filtered
}

// ExcludedActionIDs returns the canonical action IDs excludeTools removes from
// this catalog, sorted.
//
// The tool surface is not the only request path to a GitLab object: a resource
// template returns the same data through the same credential, and its
// registration knows nothing about tool names. Resolving the operator's
// entries — group name, individual tool name or action ID alike — to canonical
// IDs here is what lets that other surface apply the same narrowing from one
// table keyed by one kind of name, instead of repeating this matcher.
func (c *Catalog) ExcludedActionIDs(excludeTools []string) []string {
	if c == nil {
		return nil
	}
	filtered, _ := c.FilterExcludedToolNames(excludeTools)
	kept := make(map[ActionID]struct{}, filtered.CountActions())
	for _, action := range filtered.Actions() {
		kept[action.ID] = struct{}{}
	}
	var removed []string
	for _, action := range c.Actions() {
		if _, ok := kept[action.ID]; !ok {
			removed = append(removed, string(action.ID))
		}
	}
	sort.Strings(removed)
	return removed
}

// groupWithoutExcludedActions returns the group with every excluded action
// removed, and reports whether anything was removed. Every pattern that hit is
// recorded in matched so the caller can report the ones that never did.
//
// The group is rebuilt only when an action is actually removed, so an untouched
// group keeps its exact action order and its own identity.
func groupWithoutExcludedActions(group Group, patterns, matched map[string]struct{}) (Group, bool) {
	kept := NewGroup(GroupOptions{
		ToolName:               group.ToolName,
		Title:                  group.Title,
		Description:            group.Description,
		Icons:                  group.Icons,
		ReadOnly:               group.ReadOnly,
		FormatResult:           group.FormatResult,
		BaseDomain:             group.BaseDomain,
		EnterpriseOnly:         group.EnterpriseOnly,
		GitLabDotComOnly:       group.GitLabDotComOnly,
		CapabilityRequirements: group.CapabilityRequirements,
		OwnerPackage:           group.OwnerPackage,
		SurfaceKind:            group.SurfaceKind,
	})
	removedAny := false
	for _, action := range group.ActionsInOrder() {
		if recordExcludedActionPatterns(action, patterns, matched) {
			removedAny = true
			continue
		}
		kept.SetAction(action)
	}
	if !removedAny {
		return group, false
	}
	return kept, true
}

// recordExcludedActionPatterns records every exclusion entry that names this
// action, its individual tool name and its canonical action ID alike, and
// reports whether any did.
//
// Every spelling is recorded, not just the first to hit, because more than one
// entry legitimately reaches the same action: a configuration merged from two
// sources carries both spellings, and a file that excludes a whole group often
// also names the member action that motivated it. Recording only the first made
// the others look like entries that named nothing, so the startup warning
// accused a configuration that was doing exactly what the operator wrote.
func recordExcludedActionPatterns(action Action, patterns, matched map[string]struct{}) bool {
	excluded := false
	for _, spelling := range [...]string{action.IndividualTool.Name, string(action.ID)} {
		name := strings.TrimSpace(spelling)
		if name == "" {
			continue
		}
		if _, ok := patterns[name]; ok {
			matched[name] = struct{}{}
			excluded = true
		}
	}
	return excluded
}

// excludedToolPatterns returns the exclusion entries as a lookup set, with
// surrounding space trimmed and blank entries dropped so a trailing comma in
// EXCLUDE_TOOLS cannot become a pattern that matches an unnamed action.
func excludedToolPatterns(excludeTools []string) map[string]struct{} {
	if len(excludeTools) == 0 {
		return nil
	}
	patterns := make(map[string]struct{}, len(excludeTools))
	for _, entry := range excludeTools {
		if entry = strings.TrimSpace(entry); entry != "" {
			patterns[entry] = struct{}{}
		}
	}
	return patterns
}

// unmatchedExcludedPatterns returns the exclusion entries that matched nothing,
// in the order the operator wrote them and without repeats, which is the order
// an operator reads a startup warning against their own configuration.
func unmatchedExcludedPatterns(excludeTools []string, matched map[string]struct{}) []string {
	var unmatched []string
	seen := make(map[string]struct{}, len(excludeTools))
	for _, entry := range excludeTools {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, ok := matched[entry]; ok {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		unmatched = append(unmatched, entry)
	}
	return unmatched
}
