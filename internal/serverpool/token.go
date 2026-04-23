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
// GITLAB-URL header. Returns defaultURL if the header is absent or empty.
// Returns an error if the header value is not a valid HTTP(S) URL.
func ExtractGitLabURL(r *http.Request, defaultURL string) (string, error) {
	header := strings.TrimSpace(r.Header.Get("GITLAB-URL"))
	if header == "" {
		return defaultURL, nil
	}

	u, err := url.Parse(header)
	if err != nil {
		return "", &InvalidGitLabURLError{URL: header, Reason: "malformed URL"}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", &InvalidGitLabURLError{URL: header, Reason: "scheme must be http or https"}
	}
	if u.Host == "" {
		return "", &InvalidGitLabURLError{URL: header, Reason: "missing host"}
	}

	return strings.TrimRight(header, "/"), nil
}

// InvalidGitLabURLError is returned when the GITLAB-URL header contains an invalid URL.
type InvalidGitLabURLError struct {
	URL    string
	Reason string
}

func (e *InvalidGitLabURLError) Error() string {
	return "invalid GITLAB-URL header " + e.URL + ": " + e.Reason
}
