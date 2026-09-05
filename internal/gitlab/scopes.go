package gitlab

import (
	"context"
	"log/slog"
	"slices"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// DetectScopes queries the GitLab PAT self endpoint to retrieve the scopes
// of the currently authenticated token. Returns nil on failure or when the
// endpoint is unavailable (GitLab < 16.0), allowing graceful fallback to
// registering all tools.
func DetectScopes(ctx context.Context, client *gl.Client) []string {
	token, _, err := client.PersonalAccessTokens.GetSinglePersonalAccessToken(gl.WithContext(ctx))
	if err != nil {
		slog.WarnContext(ctx, "failed to detect PAT scopes, all tools will be registered", "error", err)
		return nil
	}
	slog.InfoContext(ctx, "detected PAT scopes", "scopes", token.Scopes)
	return token.Scopes
}

// ScopeAPI is the GitLab scope that permits writes.
//
// Only the write scope is named here, and deliberately: this package answers
// one question — can this token mutate GitLab — and the read scope is not
// part of that answer. internal/oauth owns the full scope vocabulary for the
// authorization layer; duplicating it here would be a second place to keep
// in step with GitLab.
const ScopeAPI = "api"

// WriteCapable reports whether a token's scopes permit mutating GitLab.
//
// Unknown scopes (nil: detection failed, was disabled, or the instance is
// too old to answer) count as write-capable. Assuming otherwise would
// silently strip every mutating tool from a deployment whose token is
// perfectly able to use them, and a wrong "no" is invisible — the tools are
// simply not there — while a wrong "yes" surfaces as GitLab's own 403 on the
// call that actually tried to write.
func WriteCapable(scopes []string) bool {
	if scopes == nil {
		return true
	}
	return slices.Contains(scopes, ScopeAPI)
}

// NarrowToTokenScope marks a server configuration read-only when the token
// it was built for cannot write, and reports whether it did. Both transports
// call it once the scopes are known: the HTTP pool per entry, since an entry
// is per token, and stdio once at startup for its single token. The catalog
// built from the configuration then withholds every write action and reports
// it as withheld by the token scope, rather than listing actions GitLab would
// refuse one by one with its own 403 (ADR-0018).
//
// A configuration already read-only is left alone, so the operator's setting
// and the token's limit never contradict each other in the log, and unknown
// scopes narrow nothing, for the reason WriteCapable gives.
func NarrowToTokenScope(cfg *config.ServerConfig) bool {
	if cfg == nil || cfg.ReadOnly || WriteCapable(cfg.TokenScopes) {
		return false
	}
	cfg.ReadOnly = true
	cfg.ReadOnlyFromTokenScope = true
	slog.Info("token cannot write; serving a read-only tool surface for it",
		"scopes", cfg.TokenScopes)
	return true
}

// ScopeSatisfied checks whether requiredScopes are all present in the
// detected tokenScopes. If tokenScopes is nil (detection failed or disabled),
// returns true (allow all). If requiredScopes is empty, returns true (no
// requirement).
func ScopeSatisfied(tokenScopes, requiredScopes []string) bool {
	if tokenScopes == nil || len(requiredScopes) == 0 {
		return true
	}
	scopeSet := make(map[string]struct{}, len(tokenScopes))
	for _, s := range tokenScopes {
		scopeSet[s] = struct{}{}
	}
	for _, req := range requiredScopes {
		if _, ok := scopeSet[req]; !ok {
			return false
		}
	}
	return true
}
