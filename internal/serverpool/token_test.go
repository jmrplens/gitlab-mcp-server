// token_test.go contains unit tests for token extraction and validation.
package serverpool

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// TestExtractToken validates extract token across multiple scenarios using table-driven subtests.
func TestExtractToken(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "PRIVATE-TOKEN header",
			headers:  map[string]string{"PRIVATE-TOKEN": "glpat-abc123"},
			expected: "glpat-abc123",
		},
		{
			name:     "Bearer token",
			headers:  map[string]string{"Authorization": "Bearer glpat-xyz789"},
			expected: "glpat-xyz789",
		},
		{
			name: "PRIVATE-TOKEN takes precedence over Bearer",
			headers: map[string]string{
				"PRIVATE-TOKEN": "glpat-private",
				"Authorization": "Bearer glpat-bearer",
			},
			expected: "glpat-private",
		},
		{
			name:     "no headers returns empty",
			headers:  map[string]string{},
			expected: "",
		},
		{
			name:     "empty Bearer returns empty",
			headers:  map[string]string{"Authorization": "Bearer "},
			expected: "",
		},
		{
			name:     "Basic auth ignored",
			headers:  map[string]string{"Authorization": "Basic dXNlcjpwYXNz"},
			expected: "",
		},
		{
			name:     "Bearer without space ignored",
			headers:  map[string]string{"Authorization": "Bearertoken"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			got := ExtractToken(req)
			if got != tt.expected {
				t.Errorf("ExtractToken() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestExtractGitLabURL validates GitLab URL extraction from GITLAB-URL header.
func TestExtractGitLabURL(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		defaultURL string
		wantURL    string
		wantErr    bool
	}{
		{
			name:       "no header returns default",
			header:     "",
			defaultURL: "https://gitlab.com",
			wantURL:    "https://gitlab.com",
		},
		{
			name:       "valid HTTPS URL without default",
			header:     "https://gitlab.example.com",
			defaultURL: "",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "valid HTTP URL without default",
			header:     "http://gitlab.local:8080",
			defaultURL: "",
			wantURL:    "http://gitlab.local:8080",
		},
		{
			name:       "trailing slash stripped without default",
			header:     "https://gitlab.example.com/",
			defaultURL: "",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "whitespace trimmed without default",
			header:     "  https://gitlab.example.com  ",
			defaultURL: "",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "matching header accepted when default configured",
			header:     "https://gitlab.example.com/",
			defaultURL: "https://gitlab.example.com",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "different header ignored when default configured",
			header:     "https://other.gitlab.example.com",
			defaultURL: "https://gitlab.example.com",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "invalid scheme ignored when default configured",
			header:     "ftp://gitlab.example.com",
			defaultURL: "https://gitlab.com",
			wantURL:    "https://gitlab.com",
		},
		{
			name:       "invalid scheme rejected without default",
			header:     "ftp://gitlab.example.com",
			defaultURL: "",
			wantErr:    true,
		},
		{
			name:       "missing host rejected without default",
			header:     "https://",
			defaultURL: "",
			wantErr:    true,
		},
		{
			name:       "no header and no default is refused",
			header:     "",
			defaultURL: "",
			wantErr:    true,
		},
		{
			name:       "whitespace-only header falls back to default",
			header:     "   ",
			defaultURL: "https://gitlab.com",
			wantURL:    "https://gitlab.com",
		},
		{
			name:       "default URL with trailing slash is normalized",
			header:     "",
			defaultURL: "https://gitlab.example.com/",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "uppercase scheme accepted and canonicalized without default",
			header:     "HTTPS://gitlab.example.com",
			defaultURL: "",
			wantURL:    "https://gitlab.example.com",
		},
		{
			name:       "malformed URL rejected without default",
			header:     "://not-a-url",
			defaultURL: "",
			wantErr:    true,
		},
		{
			name:       "URL with path preserved without default",
			header:     "https://gitlab.example.com/api",
			defaultURL: "",
			wantURL:    "https://gitlab.example.com/api",
		},
		{
			name:       "credentials rejected without default",
			header:     "https://user:secret@gitlab.example.com",
			defaultURL: "",
			wantErr:    true,
		},
		{
			name:       "query rejected without default",
			header:     "https://gitlab.example.com?token=secret",
			defaultURL: "",
			wantErr:    true,
		},
		{
			name:       "fragment rejected without default",
			header:     "https://gitlab.example.com#internal.example.com",
			defaultURL: "",
			wantErr:    true,
		},
		{
			name:       "invalid default URL is also rejected",
			header:     "",
			defaultURL: "ftp://bad-default.example.com",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
			if tt.header != "" {
				req.Header.Set("GITLAB-URL", tt.header)
			}
			got, err := ExtractGitLabURL(req, tt.defaultURL)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantURL {
				t.Errorf("ExtractGitLabURL() = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

// TestResolveRequestOptions_IgnoredOptions verifies that server-wide MCP
// configuration records request options that were ignored.
func TestResolveRequestOptions_IgnoredOptions(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
	req.Header.Set("GITLAB-URL", "https://other.gitlab.example.com")
	req.Header.Set("META-TOOLS", "true")
	req.Header.Set("RATE-LIMIT-RPS", "999")
	req.Header.Set("TOOL-SURFACE", "dynamic")
	req.Header.Set("META-PARAM-SCHEMA", "full")

	options, err := ResolveRequestOptions(req, "https://gitlab.example.com/")
	if err != nil {
		t.Fatalf("ResolveRequestOptions() error: %v", err)
	}
	if options.GitLabURL != "https://gitlab.example.com" {
		t.Fatalf("GitLabURL = %q, want %q", options.GitLabURL, "https://gitlab.example.com")
	}
	if !options.HasIgnoredOptions() {
		t.Fatal("HasIgnoredOptions() = false, want true")
	}
	if !options.HasDeprecatedOptions() {
		t.Fatal("HasDeprecatedOptions() = false, want true")
	}
	ignored := options.IgnoredOptionsCopy()
	want := []string{"META_TOOLS", "TOOL_SURFACE", "META_PARAM_SCHEMA", "RATE_LIMIT_RPS", RequestOptionGitLabURL}
	if !slicesEqual(ignored, want) {
		t.Fatalf("IgnoredOptions = %v, want %v", ignored, want)
	}
	deprecated := options.DeprecatedOptionsCopy()
	if !slicesEqual(deprecated, []string{"META_TOOLS"}) {
		t.Fatalf("DeprecatedOptions = %v, want [META_TOOLS]", deprecated)
	}
}

// TestResolveRequestOptions_ServerManagedHeadersIgnoredWithoutDefault verifies
// that config-like request headers never override MCP server configuration,
// even in multi-instance mode where GITLAB-URL is accepted.
func TestResolveRequestOptions_ServerManagedHeadersIgnoredWithoutDefault(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
	req.Header.Set("GITLAB-URL", "https://gitlab.example.com")
	req.Header.Set("RATE_LIMIT_BURST", "999")
	req.Header.Set("META_PARAM_SCHEMA", "full")
	req.Header.Set("GITLAB-SAFE-MODE", "false")

	options, err := ResolveRequestOptions(req, "")
	if err != nil {
		t.Fatalf("ResolveRequestOptions() error: %v", err)
	}
	if options.GitLabURL != "https://gitlab.example.com" {
		t.Fatalf("GitLabURL = %q, want %q", options.GitLabURL, "https://gitlab.example.com")
	}
	want := []string{"META_PARAM_SCHEMA", "GITLAB_SAFE_MODE", "RATE_LIMIT_BURST"}
	if !slicesEqual(options.IgnoredOptionsCopy(), want) {
		t.Fatalf("IgnoredOptions = %v, want %v", options.IgnoredOptionsCopy(), want)
	}
}

// slicesEqual compares two string slices in order for ignored-option tests.
func slicesEqual(got, want []string) bool {
	return slices.Equal(got, want)
}

// TestAppendOptionName_DeduplicatesExisting verifies the internal option-name
// accumulator keeps the first occurrence when multiple aliases map to one
// server-managed option.
func TestAppendOptionName_DeduplicatesExisting(t *testing.T) {
	options := []string{"META_PARAM_SCHEMA"}
	got := appendOptionName(options, "META_PARAM_SCHEMA")
	if !slicesEqual(got, options) {
		t.Fatalf("appendOptionName() = %v, want %v", got, options)
	}
}

// TestInvalidGitLabURLError_DoesNotLeakURL verifies that [Error] never
// embeds the raw offending URL in its message — the URL may contain
// credentials in userinfo or sensitive query parameters that must not
// be copied verbatim into server logs (OWASP A09 logging hygiene).
func TestInvalidGitLabURLError_DoesNotLeakURL(t *testing.T) {
	t.Parallel()
	sensitive := "https://user:super-secret-password@gitlab.example.com/?token=abc123"
	err := &InvalidGitLabURLError{URL: sensitive, Reason: "scheme must be http or https"}
	msg := err.Error()
	if strings.Contains(msg, "super-secret-password") || strings.Contains(msg, "abc123") ||
		strings.Contains(msg, "user:") || strings.Contains(msg, "gitlab.example.com") {
		t.Errorf("Error() leaked URL contents: %q", msg)
	}
	if !strings.Contains(msg, "scheme must be http or https") {
		t.Errorf("Error() missing reason: %q", msg)
	}
}

// TestExtractBearerToken_IgnoresPrivateToken verifies the oauth-mode
// extractor reads only Authorization: Bearer and never PRIVATE-TOKEN — a
// request carrying both must yield the Bearer credential, since that is the
// one the SDK middleware verified.
func TestExtractBearerToken_IgnoresPrivateToken(t *testing.T) {
	tests := []struct {
		name    string
		bearer  string
		private string
		want    string
	}{
		{"bearer only", "gloas-abc", "", "gloas-abc"},
		{"both present prefers bearer", "gloas-verified", "glpat-unverified", "gloas-verified"},
		{"private only yields nothing", "", "glpat-x", ""},
		{"neither", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
			if tt.bearer != "" {
				r.Header.Set("Authorization", "Bearer "+tt.bearer)
			}
			if tt.private != "" {
				r.Header.Set("PRIVATE-TOKEN", tt.private)
			}
			if got := ExtractBearerToken(r); got != tt.want {
				t.Errorf("ExtractBearerToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveRequestOptionsFor_MultipleInstances pins the allow-list, which is
// what lets one oauth deployment serve gitlab.com and a self-managed instance
// without reopening the hole a free-form GITLAB-URL header would be.
//
// The last case is the point: an unlisted instance is REFUSED, not quietly
// replaced by the default. Serving the default would answer a question the
// client did not ask, with another instance's data.
// assertUnnamedInstanceRefusal checks a request that named no instance on a
// multi-instance deployment was refused by the resolver itself.
//
// The resolver used to answer it with the first published instance while the
// server's gate refused it, so a caller reaching the resolver without the gate
// was served an instance the request never named. The refusal is the
// resolver's now; the set travels on the error for a caller that may echo it,
// and the message names nobody, since whether the set is public depends on
// the auth mode.
func assertUnnamedInstanceRefusal(t *testing.T, err error, allowed []string) {
	t.Helper()
	var unnamed *UnnamedInstanceError
	if !errors.As(err, &unnamed) {
		t.Fatalf("error = %v, want an *UnnamedInstanceError", err)
	}
	if !slices.Equal(unnamed.Allowed, allowed) {
		t.Errorf("Allowed = %v, want the published set %v", unnamed.Allowed, allowed)
	}
	for _, instance := range allowed {
		if strings.Contains(unnamed.Error(), instance) {
			t.Errorf("the message names %s, which is the caller's decision to make: %q", instance, unnamed.Error())
		}
	}
}

// assertDisallowedInstanceRefusal checks a header naming an unpublished
// instance was refused with the published set named and the caller's value
// not echoed.
func assertDisallowedInstanceRefusal(t *testing.T, err error, published string) {
	t.Helper()
	var disallowed *DisallowedGitLabURLError
	if !errors.As(err, &disallowed) {
		t.Fatalf("error = %v, want a *DisallowedGitLabURLError", err)
	}
	if !strings.Contains(disallowed.Error(), published) {
		t.Errorf("error %q does not name the allowed instances", disallowed)
	}
	if strings.Contains(disallowed.Error(), "evil.example.com") {
		t.Error("the rejected value is caller-controlled and must not be echoed back")
	}
}

func TestResolveRequestOptionsFor_MultipleInstances(t *testing.T) {
	const (
		primary   = "https://gitlab.com"
		secondary = "https://gitlab.example.com"
	)
	allowed := []string{primary, secondary}

	tests := []struct {
		name        string
		header      string
		want        string
		wantErr     bool
		wantUnnamed bool
	}{
		{name: "no header is refused rather than served the first published instance", wantUnnamed: true},
		{name: "header selects the first published instance explicitly", header: primary, want: primary},
		{name: "header selects another published instance", header: secondary, want: secondary},
		{name: "header selects a trailing-slash spelling of one", header: secondary + "/", want: secondary},
		{name: "header naming an unpublished instance is refused", header: "https://evil.example.com", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
			if tt.header != "" {
				req.Header.Set(RequestOptionGitLabURL, tt.header)
			}

			options, err := ResolveRequestOptionsFor(req, allowed)
			if tt.wantUnnamed {
				assertUnnamedInstanceRefusal(t, err, allowed)
				return
			}
			if tt.wantErr {
				assertDisallowedInstanceRefusal(t, err, primary)
				return
			}
			if err != nil {
				t.Fatalf("ResolveRequestOptionsFor() error = %v", err)
			}
			if options.GitLabURL != tt.want {
				t.Errorf("GitLabURL = %q, want %q", options.GitLabURL, tt.want)
			}
		})
	}
}

// TestResolveRequestOptionsFor_SingleInstanceIgnoresTheHeader verifies that
// pinning exactly one instance behaves as --gitlab-url always has: the header
// is recorded as ignored rather than honored or refused, so no existing
// deployment changes behavior when the list gains its plural form.
func TestResolveRequestOptionsFor_SingleInstanceIgnoresTheHeader(t *testing.T) {
	const fixed = "https://gitlab.example.com"

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
	req.Header.Set(RequestOptionGitLabURL, "https://elsewhere.example.com")

	options, err := ResolveRequestOptionsFor(req, []string{fixed})
	if err != nil {
		t.Fatalf("ResolveRequestOptionsFor() error = %v", err)
	}
	if options.GitLabURL != fixed {
		t.Errorf("GitLabURL = %q, want %q", options.GitLabURL, fixed)
	}
	if !slices.Contains(options.IgnoredOptions, RequestOptionGitLabURL) {
		t.Errorf("IgnoredOptions = %v, want it to name %s", options.IgnoredOptions, RequestOptionGitLabURL)
	}
}

// TestNormalizeGitLabURL_CanonicalizesEquivalentSpellings verifies RFC 3986
// section 6.2.2 equivalence: scheme and host are case-insensitive and an
// explicit default port is equivalent to none.
//
// It matters twice. The allow-list compares canonical strings, so without this
// a header naming "https://GitLab.com" would be refused as an instance the
// deployment does not publish — while naming the one it does. And the server
// pool keys on this value, so one instance spelled two ways would build two
// entries for a single credential.
func TestNormalizeGitLabURL_CanonicalizesEquivalentSpellings(t *testing.T) {
	t.Parallel()

	const canonical = "https://gitlab.example.com"
	equivalent := []string{
		"https://gitlab.example.com",
		"https://GitLab.Example.com",
		"HTTPS://GITLAB.EXAMPLE.COM",
		"https://gitlab.example.com:443",
		"https://gitlab.example.com/",
		"https://GitLab.example.com:443/",
	}
	for _, raw := range equivalent {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeGitLabURL(raw)
			if err != nil {
				t.Fatalf("normalizeGitLabURL(%q) error = %v", raw, err)
			}
			if got != canonical {
				t.Errorf("normalizeGitLabURL(%q) = %q, want %q", raw, got, canonical)
			}
		})
	}

	// A non-default port is significant and must survive.
	if got, err := normalizeGitLabURL("https://gitlab.example.com:8443"); err != nil || got != "https://gitlab.example.com:8443" {
		t.Errorf("normalizeGitLabURL(:8443) = %q, %v; a non-default port must be kept", got, err)
	}
	// http's default is 80, not 443.
	if got, err := normalizeGitLabURL("http://gitlab.example.com:80"); err != nil || got != "http://gitlab.example.com" {
		t.Errorf("normalizeGitLabURL(http :80) = %q, %v", got, err)
	}
	if got, err := normalizeGitLabURL("http://gitlab.example.com:443"); err != nil || got != "http://gitlab.example.com:443" {
		t.Errorf("normalizeGitLabURL(http :443) = %q, %v; 443 is not http's default", got, err)
	}
}

// TestNormalizeGitLabURLs_DropsBlanksAndDuplicates verifies that the list is
// canonicalized while keeping order, since the first entry is the
// deployment's default instance and sorting would silently re-elect it.
func TestNormalizeGitLabURLs_DropsBlanksAndDuplicates(t *testing.T) {
	got, err := NormalizeGitLabURLs([]string{
		"https://gitlab.com/", "  ", "https://gitlab.example.com", "https://gitlab.com",
	})
	if err != nil {
		t.Fatalf("NormalizeGitLabURLs() error = %v", err)
	}
	want := []string{"https://gitlab.com", "https://gitlab.example.com"}
	if !slices.Equal(got, want) {
		t.Errorf("NormalizeGitLabURLs() = %v, want %v", got, want)
	}
}

// TestResolveRequestOptionsFor_MalformedHeaderAgainstAnAllowList_IsRejected
// covers the header failing to parse when several instances are published.
//
// The allow-list branch has to parse the header before it can ask whether the
// list contains it, and an unparseable value is refused with the parse error
// rather than silently falling back to the default instance: falling back would
// answer a question the client did not ask, with another instance's data.
func TestResolveRequestOptionsFor_MalformedHeaderAgainstAnAllowList_IsRejected(t *testing.T) {
	t.Parallel()

	allowed := []string{"https://gitlab.com", "https://gitlab.example.com"}
	tests := []struct {
		name   string
		header string
	}{
		{name: "not a URL at all", header: "://nonsense"},
		{name: "no scheme", header: "gitlab.example.com"},
		{name: "credentials embedded", header: "https://user:pass@gitlab.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
			r.Header.Set(RequestOptionGitLabURL, tt.header)

			options, err := ResolveRequestOptionsFor(r, allowed)

			if err == nil {
				t.Fatalf("resolved %q to %q instead of refusing it", tt.header, options.GitLabURL)
			}
			if _, ok := errors.AsType[*InvalidGitLabURLError](err); !ok {
				t.Errorf("error = %v (%T), want an InvalidGitLabURLError naming the header", err, err)
			}
			if options.GitLabURL != "" {
				t.Errorf("GitLabURL = %q, want nothing selected when the header cannot be read", options.GitLabURL)
			}
		})
	}
}

// TestResolveRequestOptionsFor_NoInstanceAndNoHeader_IsRefused pins the one
// case where nothing at all names an instance.
//
// An empty allow-list is --allow-any-gitlab-url: the operator has said any host
// the caller names is acceptable, which is not the same as saying gitlab.com
// is. Resolving to the public instance sent a self-managed deployment's
// credential to a third party whenever a proxy stripped the header or a client
// library could not set one, and the caller never learned it had happened.
func TestResolveRequestOptionsFor_NoInstanceAndNoHeader_IsRefused(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
	r.Header.Set(RequestOptionPrivateToken, "glpat-example")

	options, err := ResolveRequestOptionsFor(r, nil)

	if !errors.Is(err, ErrMissingGitLabURL) {
		t.Fatalf("error = %v, want ErrMissingGitLabURL", err)
	}
	if options.GitLabURL != "" {
		t.Errorf("GitLabURL = %q, want nothing selected when no instance was named", options.GitLabURL)
	}
}

// TestCanonicalHost_DropsOnlyTheSchemeDefaultPort pins the comparison key both
// the allow-list and the pool are built on.
//
// RFC 3986 section 6.2.2 makes the host case-insensitive and an explicit
// default port equivalent to none, so two spellings of one instance have to
// canonicalize alike or the allow-list refuses the very instance it publishes
// and the pool builds two entries for one credential. A non-default port is
// part of the address and must survive, an IPv6 literal keeps its brackets, and
// a scheme with no default port of its own has nothing to drop.
func TestCanonicalHost_DropsOnlyTheSchemeDefaultPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		scheme string
		host   string
		want   string
	}{
		{name: "https default port", scheme: "https", host: "GitLab.com:443", want: "gitlab.com"},
		{name: "http default port", scheme: "http", host: "GitLab.local:80", want: "gitlab.local"},
		{name: "https non-default port", scheme: "https", host: "gitlab.local:8443", want: "gitlab.local:8443"},
		{name: "http port that is https default", scheme: "http", host: "gitlab.local:443", want: "gitlab.local:443"},
		{name: "ipv6 literal keeps its brackets", scheme: "https", host: "[::1]:443", want: "[::1]"},
		{name: "a scheme with no default port keeps everything", scheme: "ssh", host: "GitLab.local:22", want: "gitlab.local:22"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := canonicalHost(tt.scheme, tt.host); got != tt.want {
				t.Errorf("canonicalHost(%q, %q) = %q, want %q", tt.scheme, tt.host, got, tt.want)
			}
		})
	}
}
