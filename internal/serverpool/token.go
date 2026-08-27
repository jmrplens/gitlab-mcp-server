package serverpool

import (
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// RequestOptionGitLabURL identifies the per-request GitLab URL header option.
const RequestOptionGitLabURL = "GITLAB-URL"

// requestOptionAlias maps one canonical server-managed option name to all
// accepted HTTP header spellings for compatibility diagnostics.
type requestOptionAlias struct {
	name       string
	headers    []string
	deprecated bool
}

// serverManagedRequestOptions enumerates request headers that are intentionally
// ignored because the server process owns those configuration decisions.
var serverManagedRequestOptions = []requestOptionAlias{
	{name: "GITLAB_URL", headers: []string{"GITLAB_URL"}},
	{name: "GITLAB_SKIP_TLS_VERIFY", headers: []string{"GITLAB_SKIP_TLS_VERIFY", "GITLAB-SKIP-TLS-VERIFY", "SKIP-TLS-VERIFY"}},
	{name: "META_TOOLS", headers: []string{"META_TOOLS", "META-TOOLS"}, deprecated: true},
	{name: "TOOL_SURFACE", headers: []string{"TOOL_SURFACE", "TOOL-SURFACE"}},
	{name: "CAPABILITY_SURFACE", headers: []string{"CAPABILITY_SURFACE", "CAPABILITY-SURFACE"}},
	{name: "META_PARAM_SCHEMA", headers: []string{"META_PARAM_SCHEMA", "META-PARAM-SCHEMA"}},
	{name: "GITLAB_TIER", headers: []string{"GITLAB_TIER", "GITLAB-TIER", "TIER"}},
	{name: "GITLAB_READ_ONLY", headers: []string{"GITLAB_READ_ONLY", "GITLAB-READ-ONLY", "READ-ONLY"}},
	{name: "GITLAB_SAFE_MODE", headers: []string{"GITLAB_SAFE_MODE", "GITLAB-SAFE-MODE", "SAFE-MODE"}},
	{name: "EMBEDDED_RESOURCES", headers: []string{"EMBEDDED_RESOURCES", "EMBEDDED-RESOURCES"}},
	{name: "EXCLUDE_TOOLS", headers: []string{"EXCLUDE_TOOLS", "EXCLUDE-TOOLS"}},
	{name: "GITLAB_IGNORE_SCOPES", headers: []string{"GITLAB_IGNORE_SCOPES", "GITLAB-IGNORE-SCOPES", "IGNORE-SCOPES"}},
	{name: "UPLOAD_MAX_FILE_SIZE", headers: []string{"UPLOAD_MAX_FILE_SIZE", "UPLOAD-MAX-FILE-SIZE"}},
	{name: "MAX_HTTP_CLIENTS", headers: []string{"MAX_HTTP_CLIENTS", "MAX-HTTP-CLIENTS"}},
	{name: "SESSION_TIMEOUT", headers: []string{"SESSION_TIMEOUT", "SESSION-TIMEOUT"}},
	{name: "POOL_IDLE_TIMEOUT", headers: []string{"POOL_IDLE_TIMEOUT", "POOL-IDLE-TIMEOUT"}},
	{name: "HTTP_IDLE_TIMEOUT", headers: []string{"HTTP_IDLE_TIMEOUT", "HTTP-IDLE-TIMEOUT"}},
	{name: "STATELESS", headers: []string{"STATELESS"}},
	{name: "JSON_RESPONSE", headers: []string{"JSON_RESPONSE", "JSON-RESPONSE"}},
	{name: "MAX_REQUEST_BODY_BYTES", headers: []string{"MAX_REQUEST_BODY_BYTES", "MAX-REQUEST-BODY-BYTES"}},
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
	GitLabURL         string
	IgnoredOptions    []string
	DeprecatedOptions []string
}

// HasIgnoredOptions reports whether any request-provided options were ignored
// because server-wide MCP configuration is authoritative.
func (o RequestOptions) HasIgnoredOptions() bool {
	return len(o.IgnoredOptions) > 0
}

// HasDeprecatedOptions reports whether any ignored request options are also
// deprecated compatibility options.
func (o RequestOptions) HasDeprecatedOptions() bool {
	return len(o.DeprecatedOptions) > 0
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

	return bearerCredential(r)
}

// bearerCredential returns the token from an Authorization: Bearer header.
//
// The scheme is matched case-insensitively because RFC 9110 section 11.1 says
// it is case-insensitive, and because the SDK's own bearer middleware lowercases
// it before comparing. Matching only "Bearer " meant a client sending "bearer"
// was verified upstream by the SDK — at the cost of a real API call — and then
// refused here as though it had sent no credential at all.
func bearerCredential(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	scheme, token, found := strings.Cut(auth, " ")
	if !found || !strings.EqualFold(scheme, "bearer") {
		return ""
	}
	// RFC 9110 allows more than one space between the scheme and the
	// credential, and some clients emit one.
	return strings.TrimSpace(token)
}

// ExtractBearerToken returns only the Authorization: Bearer credential,
// ignoring PRIVATE-TOKEN. OAuth mode uses it so the gate authenticates as
// the identity the SDK middleware verified, never as a PRIVATE-TOKEN the
// same request might also carry.
func ExtractBearerToken(r *http.Request) string {
	return bearerCredential(r)
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
// any GITLAB-URL header is ignored. When defaultURL is empty, a GITLAB-URL
// header selects the instance per request; if that header is also absent, the
// public GitLab instance (config.DefaultGitLabURL) is used, mirroring the stdio
// default so HTTP clients may omit the URL entirely. Effective URLs are
// normalized so equivalent values hash to the same server-pool session key.
func ResolveRequestOptions(r *http.Request, defaultURL string) (RequestOptions, error) {
	header := strings.TrimSpace(r.Header.Get(RequestOptionGitLabURL))
	trimmedDefault := strings.TrimSpace(defaultURL)
	ignoredOptions, deprecatedOptions := ignoredServerManagedOptions(r)

	if trimmedDefault != "" {
		normalizedDefault, err := normalizeGitLabURL(trimmedDefault)
		if err != nil {
			return RequestOptions{}, err
		}

		options := RequestOptions{GitLabURL: normalizedDefault, IgnoredOptions: ignoredOptions, DeprecatedOptions: deprecatedOptions}
		if header == "" {
			return options, nil
		}
		options.IgnoredOptions = appendOptionName(options.IgnoredOptions, RequestOptionGitLabURL)
		return options, nil
	}

	if header == "" {
		// No fixed --gitlab-url and no per-request GITLAB-URL header: fall back to
		// the public GitLab instance instead of leaving the URL empty (which would
		// surface downstream as "no server available"). DefaultGitLabURL is already
		// canonical, so it needs no normalization.
		return RequestOptions{GitLabURL: config.DefaultGitLabURL, IgnoredOptions: ignoredOptions, DeprecatedOptions: deprecatedOptions}, nil
	}
	normalizedHeader, err := normalizeGitLabURL(header)
	if err != nil {
		return RequestOptions{}, err
	}
	return RequestOptions{GitLabURL: normalizedHeader, IgnoredOptions: ignoredOptions, DeprecatedOptions: deprecatedOptions}, nil
}

// IgnoredOptionsCopy returns a defensive copy of the ignored option names.
func (o RequestOptions) IgnoredOptionsCopy() []string {
	return slices.Clone(o.IgnoredOptions)
}

// DeprecatedOptionsCopy returns a defensive copy of deprecated ignored option
// names.
func (o RequestOptions) DeprecatedOptionsCopy() []string {
	return slices.Clone(o.DeprecatedOptions)
}

// ignoredServerManagedOptions returns canonical option names for request
// headers that tried to override server-managed settings.
func ignoredServerManagedOptions(r *http.Request) (ignoredOptions, deprecatedOptions []string) {
	ignoredOptions = make([]string, 0)
	deprecatedOptions = make([]string, 0)
	for _, option := range serverManagedRequestOptions {
		if hasAnyHeader(r, option.headers) {
			ignoredOptions = appendOptionName(ignoredOptions, option.name)
			if option.deprecated {
				deprecatedOptions = appendOptionName(deprecatedOptions, option.name)
			}
		}
	}
	return ignoredOptions, deprecatedOptions
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
