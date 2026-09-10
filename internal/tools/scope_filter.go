package tools

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// addFilteredGroup re-adds a group to the catalog a filter is rebuilding. It is
// a variable so a test can drive the failure the guard around it exists for:
// every real call hands back a group the source catalog already normalized,
// under a name that is unique there, so no configuration an operator can write
// makes it fail. The guard stays because group normalization is free to gain a
// rule, and a filter that swallowed such a failure would serve a catalog
// missing a domain with nothing said about it.
var addFilteredGroup = (*actioncatalog.Catalog).AddGroup

// MetaToolScopes maps meta-tool names to the PAT scopes required for that
// tool. Tools not listed here have no scope requirement and are always
// registered. A tool is removed when ANY of its required scopes is missing
// from the detected token scopes (i.e. all listed scopes must be present).
//
// Scope reference (GitLab PAT scopes):
//
//	api              — Full API access (read+write)
//	read_api         — Read-only API access
//	read_user        — Read user information
//	read_repository  — Read repository contents
//	write_repository — Write to repository
//	read_registry    — Pull container registry images
//	write_registry   — Push container registry images
//	create_runner    — Create runners
//	manage_runner    — Manage runners
//	admin_mode       — Admin operations
//	ai_features      — GitLab Duo API access
//	k8s_proxy        — Kubernetes API calls via agent
//	sudo             — Impersonate users
//
// The keys are meta-tool group names, and the filter is applied to the
// **catalog** rather than to registered tool names, which is what lets one set
// of keys reach all three surfaces. [FilterScopeFilteredCatalog] matches them
// against Group.ToolName, so on the dynamic and meta surfaces it removes the
// group a caller would name, and on the individual surface it removes every
// action projected from that group before any of them is registered.
//
// That last part is the whole reason this comment is here. Until 3.0.0 the
// individual surface was filtered by a second pass over registered tool names,
// and nothing on that surface is ever named gitlab_admin: one tool is
// registered per action, so the pass matched nothing and every admin tool
// stayed listed for a token with no admin_mode. The calls themselves were
// refused, by GitLab, with a 403, so what was wrong was the listing rather than
// the authorization, which is the worse half to leave: a model reads tools/list
// to decide what is possible and concluded the capability was there. The fix is
// the shape the exclusion filter already had, and lives in
// [SharedIndividualCatalog].
//
// A group belongs in this map only when **every** action in it needs the
// scopes, because the removal is all-or-nothing per group. A domain that mixes
// read and write actions (gitlab_runner, say) is deliberately absent: removing
// it whole because the token cannot write would hide the reads that work.
//
// gitlab_admin is the entry that tests that rule hardest, and the granularity
// is known to be slightly over-broad there: a handful of its 92 actions are
// reads GitLab serves to any authenticated token (topic_list, topic_get,
// broadcast_message_list, broadcast_message_get), and a token with no
// admin_mode loses them along with the 88 that genuinely need it. That is the
// direction to err in, and it is not new: the same removal has applied on the
// meta and dynamic surfaces since the filter existed, so what changed in 3.0.0
// is that the individual surface stopped being the exception. Splitting those
// reads out means per-action requirements rather than per-group, which is a
// change to what all three surfaces serve and wants deciding as such.
var MetaToolScopes = map[string][]string{
	"gitlab_admin":           {"admin_mode"},
	"gitlab_enterprise_user": {"admin_mode"},
	"gitlab_project_alias":   {"admin_mode"},
	"gitlab_geo":             {"admin_mode"},
	"gitlab_storage_move":    {"admin_mode"},
}

// FilterScopeFilteredCatalog removes catalog groups whose required scopes
// are not satisfied by the detected token scopes. It returns a non-nil
// error when rebuilding the filtered catalog fails, which means callers
// could not safely evaluate the scope filter and should propagate the
// failure. A nil input catalog returns an empty catalog; nil token
// scopes return a defensive clone of the source catalog.
func FilterScopeFilteredCatalog(catalog *actioncatalog.Catalog, tokenScopes []string) (*actioncatalog.Catalog, error) {
	if catalog == nil {
		return actioncatalog.NewCatalog(), nil
	}
	if tokenScopes == nil {
		// Return a defensive clone because callers may further filter the returned
		// catalog; nil scopes mean detection was unavailable, not that the source
		// catalog may be mutated in place.
		return catalog.Clone(), nil
	}

	scopeSet := buildScopeSet(tokenScopes)

	filtered := actioncatalog.NewCatalog()
	var removed []string
	for _, group := range catalog.Groups() {
		if required := MetaToolScopes[group.ToolName]; len(required) > 0 && !allScopesPresent(scopeSet, required) {
			removed = append(removed, group.ToolName)
			slog.Debug(
				"catalog group requires missing PAT scope",
				"tool", group.ToolName,
				"required", required,
				"available", tokenScopes,
			)
			continue
		}
		if err := addFilteredGroup(filtered, group); err != nil {
			return nil, fmt.Errorf("add scope-filtered catalog group %q: %w", group.ToolName, err)
		}
	}
	if len(removed) > 0 {
		slog.Info(
			"scope-filtered catalog groups removed",
			"removed", len(removed),
			"tools", strings.Join(removed, ", "),
			"scopes", strings.Join(tokenScopes, ", "),
		)
	}
	return filtered, nil
}

// catalogRelevantScopes returns the part of a token's scope list that can
// change what [FilterScopeFilteredCatalog] removes: the sorted, deduplicated
// intersection of tokenScopes with the union of every scope [MetaToolScopes]
// requires. A token's other scopes are invisible to the filter, so two tokens
// differing only in those narrow the catalog identically.
//
// This exists for [CatalogFilterKey], which names a shared catalog. Keying
// that cache on the whole scope list would let a caller mint personal access
// tokens with arbitrary scope subsets and pin one catalog per subset for the
// life of the process. Derived from MetaToolScopes rather than written out, so
// a scope added to a requirement there widens the key with it and cannot be
// forgotten here.
//
// What makes the narrowing safe is an equivalence: two tokens with the same
// canonical components get the same filtered catalog, so serving both from
// one cache entry serves neither a catalog it did not earn. That equivalence
// holds because of exactly how the filter reads a scope list, and these are
// the changes that would break it silently, each of which needs this
// derivation changed with it:
//
//   - a prefix or wildcard match in [allScopesPresent], which would make a
//     scope not spelled in MetaToolScopes decide a removal;
//   - a scope-implication rule (api implies read_api, say), since the
//     implying scope need not appear anywhere in the union this reads;
//   - a rule keyed on a scope's absence, or on the length of the list, since
//     both read the scopes this drops;
//   - a second requirements map beside MetaToolScopes, which this derivation
//     does not read at all;
//   - admin detection from anything other than the scope list, such as an
//     instance probe, which would make two equal lists filter differently.
//
// [TestCatalogRelevantScopes_EqualComponentsFilterIdentically] runs the
// filter over the equivalence rather than asserting it, so a change of that
// kind fails there rather than reaching a shared catalog.
func catalogRelevantScopes(tokenScopes []string) []string {
	required := make(map[string]struct{}, len(MetaToolScopes))
	for _, scopes := range MetaToolScopes {
		for _, scope := range scopes {
			required[scope] = struct{}{}
		}
	}
	relevant := make(map[string]struct{}, len(required))
	for _, scope := range tokenScopes {
		if _, matters := required[scope]; matters {
			relevant[scope] = struct{}{}
		}
	}
	out := make([]string, 0, len(relevant))
	for scope := range relevant {
		out = append(out, scope)
	}
	slices.Sort(out)
	return out
}

// buildScopeSet returns a set of token scope strings for O(1) membership
// tests. Used by [FilterScopeFilteredCatalog].
func buildScopeSet(tokenScopes []string) map[string]struct{} {
	scopeSet := make(map[string]struct{}, len(tokenScopes))
	for _, scope := range tokenScopes {
		scopeSet[scope] = struct{}{}
	}
	return scopeSet
}

// allScopesPresent reports whether every scope in required is present in
// the scope set. An empty required slice returns true so callers can use
// the helper without first checking the required length.
func allScopesPresent(scopeSet map[string]struct{}, required []string) bool {
	for _, s := range required {
		if _, ok := scopeSet[s]; !ok {
			return false
		}
	}
	return true
}
