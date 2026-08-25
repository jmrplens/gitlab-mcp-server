package oauth

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// gitlabUserResponse holds the minimal fields from GitLab's /api/v4/user endpoint.
type gitlabUserResponse struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

// NewGitLabVerifier returns an [auth.TokenVerifier] that validates Bearer
// tokens by calling the GitLab /api/v4/user endpoint. Verified identities
// are cached in cache (if non-nil) to avoid redundant API calls.
//
// The returned verifier populates [auth.TokenInfo] with:
//   - UserID: the GitLab user's numeric ID (as string)
//   - Extra["username"]: the GitLab user's login name
//   - Extra["token"]: the raw token (for downstream GitLab client creation)
//   - Expiration: now + cacheTTL (so the SDK middleware honors TTL)
func NewGitLabVerifier(gitlabURL string, skipTLS bool, cacheTTL time.Duration, cache *TokenCache) auth.TokenVerifier {
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		baseTransport = &http.Transport{}
	}
	transport := baseTransport.Clone()
	if skipTLS {
		transport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, //#nosec G402 //nolint:gosec // user-configured opt-in for self-signed certificates
		}
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	userURL := gitlabURL + "/api/v4/user"

	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if cache != nil {
			if info, cached := cache.Get(token); cached {
				return info, nil
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, userURL, http.NoBody)
		if err != nil {
			return nil, fmt.Errorf("create verification request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("token verification request failed: %w", err)
		}
		defer resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusOK:
			// success — parse below
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return nil, fmt.Errorf("token rejected by GitLab (HTTP %d): %w", resp.StatusCode, auth.ErrInvalidToken)
		case resp.StatusCode == http.StatusTooManyRequests:
			return nil, fmt.Errorf("GitLab rate limit exceeded (HTTP 429) — retry later: %w", auth.ErrInvalidToken)
		case resp.StatusCode >= 500:
			return nil, fmt.Errorf("GitLab server error (HTTP %d)", resp.StatusCode)
		default:
			return nil, fmt.Errorf("unexpected GitLab response (HTTP %d): %w", resp.StatusCode, auth.ErrInvalidToken)
		}

		var user gitlabUserResponse
		if decErr := json.NewDecoder(resp.Body).Decode(&user); decErr != nil {
			return nil, fmt.Errorf("decode GitLab user response: %w", decErr)
		}
		if user.ID == 0 {
			return nil, fmt.Errorf("GitLab returned invalid user: %w", auth.ErrInvalidToken)
		}

		// The token's REAL scopes, introspected rather than assumed: a
		// read_user-scoped token passes /user, and stamping it api-scoped
		// would let the SDK's scope check wave through a token GitLab will
		// then reject on every write. PATs answer
		// /personal_access_tokens/self; OAuth tokens answer
		// /oauth/token/info. When neither endpoint yields scopes (older or
		// restricted instances), the historical api assumption is kept and
		// logged, so exotic deployments keep working while mainstream
		// tokens are checked for what they actually carry.
		scopes := introspectScopes(ctx, client, gitlabURL, token)

		info := &auth.TokenInfo{
			UserID:     strconv.Itoa(user.ID),
			Scopes:     scopes,
			Expiration: time.Now().Add(cacheTTL),
			Extra: map[string]any{
				"username": user.Username,
				"token":    token,
			},
		}

		if cache != nil {
			cache.Put(token, info, cacheTTL)
		}

		return info, nil
	}
}

// introspectScopes resolves a token's granted scopes. Personal access
// tokens answer GET /api/v4/personal_access_tokens/self; OAuth access
// tokens answer GET /oauth/token/info. Either endpoint failing to yield
// scopes falls back to the historical {"api"} assumption with a debug log —
// refusal here would brick instances where introspection is restricted.
func introspectScopes(ctx context.Context, client *http.Client, gitlabURL, token string) []string {
	if scopes := fetchScopes(ctx, client, gitlabURL+"/api/v4/personal_access_tokens/self", token, "scopes"); scopes != nil {
		return scopes
	}
	if scopes := fetchScopes(ctx, client, gitlabURL+"/oauth/token/info", token, "scope"); scopes != nil {
		return scopes
	}
	slog.Debug("token scope introspection unavailable; assuming api scope")
	return []string{"api"}
}

// fetchScopes reads one introspection endpoint and returns the named
// string-array field, or nil when the endpoint does not answer for this
// token kind.
func fetchScopes(ctx context.Context, client *http.Client, endpoint, token, field string) []string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var payload map[string]any
	if decodeErr := json.NewDecoder(resp.Body).Decode(&payload); decodeErr != nil {
		return nil
	}
	raw, _ := payload[field].([]any)
	scopes := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			scopes = append(scopes, s)
		}
	}
	if len(scopes) == 0 {
		return nil
	}
	return scopes
}
