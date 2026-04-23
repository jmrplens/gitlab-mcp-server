// scope_filter.go defines PAT scope requirements per meta-tool and provides
// a function to remove tools the current token cannot execute.

package tools

import (
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MetaToolScopes maps meta-tool names to the PAT scopes required for that
// tool. Tools not listed here have no scope requirement and are always
// registered. A tool is removed only when ALL its required scopes are missing
// from the detected token scopes.
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
// satisfied by the detected token scopes. Returns the number of tools removed.
// If tokenScopes is nil (detection unavailable), no tools are removed.
func RemoveScopeFilteredTools(server *mcp.Server, tokenScopes []string) int {
	if tokenScopes == nil {
		return 0
	}

	scopeSet := make(map[string]struct{}, len(tokenScopes))
	for _, s := range tokenScopes {
		scopeSet[s] = struct{}{}
	}

	var toRemove []string
	for name, required := range MetaToolScopes {
		if !allScopesPresent(scopeSet, required) {
			toRemove = append(toRemove, name)
			slog.Debug("tool requires missing PAT scope",
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

	slog.Info("scope-filtered tools removed",
		"removed", len(toRemove),
		"tools", strings.Join(toRemove, ", "),
		"scopes", strings.Join(tokenScopes, ", "),
	)

	return len(toRemove)
}

// allScopesPresent checks if all required scopes exist in the set.
func allScopesPresent(scopeSet map[string]struct{}, required []string) bool {
	for _, s := range required {
		if _, ok := scopeSet[s]; !ok {
			return false
		}
	}
	return true
}
