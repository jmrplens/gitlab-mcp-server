package gitlab

import (
	"context"
	"log/slog"
	"slices"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// DetectScopes queries the GitLab PAT self endpoint to retrieve the scopes
// of the currently authenticated token. Returns nil on failure or when the
// endpoint is unavailable (GitLab < 16.0), allowing graceful fallback to
// registering all tools.
func DetectScopes(ctx context.Context, client *gl.Client) []string {
	token, _, err := client.PersonalAccessTokens.GetSinglePersonalAccessToken(gl.WithContext(ctx))
	if err != nil {
		slog.Warn("failed to detect PAT scopes, all tools will be registered", "error", err)
		return nil
	}
	slog.Info("detected PAT scopes", "scopes", token.Scopes)
	return token.Scopes
}

// ScopeAPI is the GitLab scope that permits writes; ScopeReadAPI permits
// reads only. A token carrying api can do everything read_api can, but
// GitLab reports only the granted name, so containment is checked against
// both rather than assumed.
const (
	ScopeAPI     = "api"
	ScopeReadAPI = "read_api"
)

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
