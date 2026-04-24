// middleware.go provides HTTP middleware for normalizing GitLab authentication
// headers, converting PRIVATE-TOKEN and Bearer tokens for downstream handlers.
package oauth

import "net/http"

// NormalizeAuthHeader is HTTP middleware that converts GitLab's PRIVATE-TOKEN
// header into a standard Authorization: Bearer header. This allows the SDK's
// RequireBearerToken middleware to handle both OAuth tokens and legacy
// PRIVATE-TOKEN headers through a unified pipeline.
//
// If the request already has an Authorization header, PRIVATE-TOKEN is ignored.
func NormalizeAuthHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			if token := r.Header.Get("PRIVATE-TOKEN"); token != "" {
				r.Header.Set("Authorization", "Bearer "+token)
			}
		}
		next.ServeHTTP(w, r)
	})
}
