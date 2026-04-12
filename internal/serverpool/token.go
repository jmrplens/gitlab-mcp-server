// token.go provides token extraction and validation for HTTP mode authentication.

package serverpool

import (
	"net/http"
	"strings"
)

// ExtractToken retrieves the GitLab Personal Access Token from the HTTP
// request. It checks the following sources in order:
//  1. PRIVATE-TOKEN header (GitLab standard)
//  2. Authorization header with Bearer scheme
//
// Returns the token string, or empty string if no token is found.
func ExtractToken(r *http.Request) string {
	if token := r.Header.Get("PRIVATE-TOKEN"); token != "" {
		return token
	}

	auth := r.Header.Get("Authorization")
	if after, found := strings.CutPrefix(auth, "Bearer "); found && after != "" {
		return after
	}

	return ""
}
