package tools

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

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
// The keys are meta-tool group names, which is why the filter reaches only two
// of the three surfaces. [FilterScopeFilteredCatalog] matches them against
// Group.ToolName and so covers the dynamic and meta surfaces, where a group is
// what a caller sees. The individual surface registers one tool per action and
// nothing there is ever named gitlab_admin, so [RemoveScopeFilteredTools],
// which matches registered tool names, removes nothing: every admin individual
// tool stays listed for a token with no admin_mode.
//
// It fails toward GitLab's own 403 rather than toward access, so the tools are
// listed and then refused by the instance, which is why this is a listing
// defect and not an authorization one. Closing it is the same shape as the
// exclusion fix already applied on that surface: filter the catalog before
// registration instead of the registered names afterwards, by passing the
// scoped catalog into the individual registration path. That call lives in
// cmd/server, not here.
var MetaToolScopes = map[string][]string{
	// Admin-only tools — every action in these tools requires admin_mode.
	// Meta-tools that mix read and write actions (e.g., gitlab_runner) are
	// intentionally excluded: removing the whole tool because the token lacks
	// write scope would also hide the read actions that work fine with read_api.
	"gitlab_admin":           {"admin_mode"},
	"gitlab_enterprise_user": {"admin_mode"},
	"gitlab_project_alias":   {"admin_mode"},
	"gitlab_geo":             {"admin_mode"},
	"gitlab_storage_move":    {"admin_mode"},
}

// RemoveScopeFilteredTools removes tools whose required scopes are not
// satisfied by the detected token scopes. Returns the number of tools
// removed. If tokenScopes is nil (detection unavailable), no tools are
// removed. Logs a debug entry per removed tool and an info entry
// summarizing the removal.
func RemoveScopeFilteredTools(server *mcp.Server, tokenScopes []string) int {
	if tokenScopes == nil {
		return 0
	}

	scopeSet := buildScopeSet(tokenScopes)

	var toRemove []string
	for name, required := range MetaToolScopes {
		if !allScopesPresent(scopeSet, required) {
			toRemove = append(toRemove, name)
			slog.Debug(
				"tool requires missing PAT scope",
				"tool", name,
				"required", required,
				"available", tokenScopes,
			)
		}
	}

	if len(toRemove) == 0 {
		return 0
	}

	server.RemoveTools(toRemove...)

	slog.Info(
		"scope-filtered tools removed",
		"removed", len(toRemove),
		"tools", strings.Join(toRemove, ", "),
		"scopes", strings.Join(tokenScopes, ", "),
	)

	return len(toRemove)
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
		if err := filtered.AddGroup(group); err != nil {
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
// tests. Used by [RemoveScopeFilteredTools] and
// [FilterScopeFilteredCatalog].
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
