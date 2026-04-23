// token.go provides token and GitLab URL extraction for HTTP mode authentication.

package serverpool

import (
	"net/http"
	"net/url"
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

// ExtractGitLabURL retrieves the per-request GitLab instance URL from the
// GITLAB-URL header. Returns defaultURL (normalized) if the header is absent
// or empty. Returns an error if either the header or the defaultURL is not a
// valid HTTP(S) URL. Both the header and the default are normalized the same
// way (whitespace trimmed, trailing slashes removed) so that equivalent URLs
// hash to the same server-pool session key.
func ExtractGitLabURL(r *http.Request, defaultURL string) (string, error) {
	header := strings.TrimSpace(r.Header.Get("GITLAB-URL"))
	if header == "" {
		trimmed := strings.TrimSpace(defaultURL)
		if trimmed == "" {
			return "", nil
		}
		return normalizeGitLabURL(trimmed)
	}
	return normalizeGitLabURL(header)
}

// normalizeGitLabURL validates and canonicalizes a GitLab base URL. The
// returned string has trailing slashes stripped and a guaranteed http/https
// scheme and non-empty host.
func normalizeGitLabURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", &InvalidGitLabURLError{URL: raw, Reason: "malformed URL"}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", &InvalidGitLabURLError{URL: raw, Reason: "scheme must be http or https"}
	}
	if u.Host == "" {
		return "", &InvalidGitLabURLError{URL: raw, Reason: "missing host"}
	}
	return strings.TrimRight(raw, "/"), nil
}

// InvalidGitLabURLError is returned when the GITLAB-URL header contains an invalid URL.
// The raw URL value is intentionally not included in the error message to avoid
// leaking embedded credentials or sensitive query parameters into server logs.
type InvalidGitLabURLError struct {
	// URL is the offending URL value. It is retained for programmatic
	// inspection by callers but is deliberately omitted from [Error] output.
	URL    string
	Reason string
}

func (e *InvalidGitLabURLError) Error() string {
	return "invalid GITLAB-URL header: " + e.Reason
}
