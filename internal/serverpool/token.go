// token.go provides token and GitLab URL extraction for HTTP mode authentication.
package serverpool

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// RequestOptionGitLabURL identifies the per-request GitLab URL header option.
const RequestOptionGitLabURL = "GITLAB-URL"

// requestOptionAlias maps one canonical server-managed option name to all
// accepted HTTP header spellings for compatibility diagnostics.
type requestOptionAlias struct {
	name    string
	headers []string
}

// serverManagedRequestOptions enumerates request headers that are intentionally
// ignored because the server process owns those configuration decisions.
var serverManagedRequestOptions = []requestOptionAlias{
	{name: "GITLAB_URL", headers: []string{"GITLAB_URL"}},
	{name: "GITLAB_SKIP_TLS_VERIFY", headers: []string{"GITLAB_SKIP_TLS_VERIFY", "GITLAB-SKIP-TLS-VERIFY", "SKIP-TLS-VERIFY"}},
	{name: "META_TOOLS", headers: []string{"META_TOOLS", "META-TOOLS"}},
	{name: "META_PARAM_SCHEMA", headers: []string{"META_PARAM_SCHEMA", "META-PARAM-SCHEMA"}},
	{name: "GITLAB_ENTERPRISE", headers: []string{"GITLAB_ENTERPRISE", "GITLAB-ENTERPRISE", "ENTERPRISE"}},
	{name: "GITLAB_READ_ONLY", headers: []string{"GITLAB_READ_ONLY", "GITLAB-READ-ONLY", "READ-ONLY"}},
	{name: "GITLAB_SAFE_MODE", headers: []string{"GITLAB_SAFE_MODE", "GITLAB-SAFE-MODE", "SAFE-MODE"}},
	{name: "EMBEDDED_RESOURCES", headers: []string{"EMBEDDED_RESOURCES", "EMBEDDED-RESOURCES"}},
	{name: "EXCLUDE_TOOLS", headers: []string{"EXCLUDE_TOOLS", "EXCLUDE-TOOLS"}},
	{name: "GITLAB_IGNORE_SCOPES", headers: []string{"GITLAB_IGNORE_SCOPES", "GITLAB-IGNORE-SCOPES", "IGNORE-SCOPES"}},
	{name: "UPLOAD_MAX_FILE_SIZE", headers: []string{"UPLOAD_MAX_FILE_SIZE", "UPLOAD-MAX-FILE-SIZE"}},
	{name: "MAX_HTTP_CLIENTS", headers: []string{"MAX_HTTP_CLIENTS", "MAX-HTTP-CLIENTS"}},
	{name: "SESSION_TIMEOUT", headers: []string{"SESSION_TIMEOUT", "SESSION-TIMEOUT"}},
	{name: "SESSION_REVALIDATE_INTERVAL", headers: []string{"SESSION_REVALIDATE_INTERVAL", "SESSION-REVALIDATE-INTERVAL", "REVALIDATE-INTERVAL"}},
	{name: "AUTH_MODE", headers: []string{"AUTH_MODE", "AUTH-MODE"}},
	{name: "OAUTH_CACHE_TTL", headers: []string{"OAUTH_CACHE_TTL", "OAUTH-CACHE-TTL"}},
	{name: "TRUSTED_PROXY_HEADER", headers: []string{"TRUSTED_PROXY_HEADER", "TRUSTED-PROXY-HEADER"}},
	{name: "RATE_LIMIT_RPS", headers: []string{"RATE_LIMIT_RPS", "RATE-LIMIT-RPS"}},
	{name: "RATE_LIMIT_BURST", headers: []string{"RATE_LIMIT_BURST", "RATE-LIMIT-BURST"}},
	{name: "AUTO_UPDATE", headers: []string{"AUTO_UPDATE", "AUTO-UPDATE"}},
	{name: "AUTO_UPDATE_REPO", headers: []string{"AUTO_UPDATE_REPO", "AUTO-UPDATE-REPO"}},
	{name: "AUTO_UPDATE_INTERVAL", headers: []string{"AUTO_UPDATE_INTERVAL", "AUTO-UPDATE-INTERVAL"}},
	{name: "AUTO_UPDATE_TIMEOUT", headers: []string{"AUTO_UPDATE_TIMEOUT", "AUTO-UPDATE-TIMEOUT"}},
	{name: "LOG_LEVEL", headers: []string{"LOG_LEVEL", "LOG-LEVEL"}},
}

// RequestOptions contains the effective per-request options after applying
// server-wide MCP configuration precedence.
type RequestOptions struct {
	GitLabURL      string
	IgnoredOptions []string
}

// HasIgnoredOptions reports whether any request-provided options were ignored
// because server-wide MCP configuration is authoritative.
func (o RequestOptions) HasIgnoredOptions() bool {
	return len(o.IgnoredOptions) > 0
}

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

// ExtractGitLabURL resolves the GitLab instance URL for an HTTP request.
// It is a compatibility wrapper around [ResolveRequestOptions].
func ExtractGitLabURL(r *http.Request, defaultURL string) (string, error) {
	options, err := ResolveRequestOptions(r, defaultURL)
	if err != nil {
		return "", err
	}
	return options.GitLabURL, nil
}

// ResolveRequestOptions applies server-wide MCP configuration precedence to
// the request-provided options. When defaultURL is set, it is authoritative and
// any GITLAB-URL header is ignored. When defaultURL is empty, callers must
// provide GITLAB-URL per request. Effective URLs are normalized so equivalent
// values hash to the same server-pool session key.
func ResolveRequestOptions(r *http.Request, defaultURL string) (RequestOptions, error) {
	header := strings.TrimSpace(r.Header.Get(RequestOptionGitLabURL))
	trimmedDefault := strings.TrimSpace(defaultURL)
	ignoredOptions := ignoredServerManagedOptions(r)

	if trimmedDefault != "" {
		normalizedDefault, err := normalizeGitLabURL(trimmedDefault)
		if err != nil {
			return RequestOptions{}, err
		}

		options := RequestOptions{GitLabURL: normalizedDefault, IgnoredOptions: ignoredOptions}
		if header == "" {
			return options, nil
		}
		options.IgnoredOptions = appendOptionName(options.IgnoredOptions, RequestOptionGitLabURL)
		return options, nil
	}

	if header == "" {
		return RequestOptions{IgnoredOptions: ignoredOptions}, nil
	}
	normalizedHeader, err := normalizeGitLabURL(header)
	if err != nil {
		return RequestOptions{}, err
	}
	return RequestOptions{GitLabURL: normalizedHeader, IgnoredOptions: ignoredOptions}, nil
}

// IgnoredOptionsCopy returns a defensive copy of the ignored option names.
func (o RequestOptions) IgnoredOptionsCopy() []string {
	return slices.Clone(o.IgnoredOptions)
}

// ignoredServerManagedOptions returns canonical option names for request
// headers that tried to override server-managed settings.
func ignoredServerManagedOptions(r *http.Request) []string {
	ignoredOptions := make([]string, 0)
	for _, option := range serverManagedRequestOptions {
		if hasAnyHeader(r, option.headers) {
			ignoredOptions = appendOptionName(ignoredOptions, option.name)
		}
	}
	return ignoredOptions
}

// hasAnyHeader reports whether any alias header in headers is present with a
// non-empty value on r.
func hasAnyHeader(r *http.Request, headers []string) bool {
	for _, header := range headers {
		if strings.TrimSpace(r.Header.Get(header)) != "" {
			return true
		}
	}
	return false
}

// appendOptionName adds name once while preserving the first-seen order of
// ignored request options.
func appendOptionName(options []string, name string) []string {
	if !slices.Contains(options, name) {
		return append(options, name)
	}
	return options
}

// normalizeGitLabURL validates and canonicalizes a GitLab base URL. The
// returned string has trailing slashes stripped, no credentials, no query or
// fragment, and a guaranteed http/https scheme with non-empty host.
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
	if u.User != nil {
		return "", &InvalidGitLabURLError{URL: raw, Reason: "credentials are not allowed"}
	}
	if u.RawQuery != "" {
		return "", &InvalidGitLabURLError{URL: raw, Reason: "query parameters are not allowed"}
	}
	if u.Fragment != "" {
		return "", &InvalidGitLabURLError{URL: raw, Reason: "fragments are not allowed"}
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
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

// Error implements the [error] interface. The returned message contains only
// the validation [InvalidGitLabURLError.Reason], never the raw URL, to avoid
// leaking credentials in logs.
func (e *InvalidGitLabURLError) Error() string {
	return "invalid GITLAB-URL header: " + e.Reason
}
