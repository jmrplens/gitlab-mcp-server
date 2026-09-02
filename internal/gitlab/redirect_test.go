// redirect_test.go contains unit tests for the redirect policy that keeps
// GitLab credential headers from leaving the configured instance.
package gitlab

import (
	"net/http"
	"net/url"
	"testing"
)

// newRedirectRequest builds the request net/http would hand to a
// CheckRedirect policy: the destination URL, carrying the headers copied from
// the previous hop.
func newRedirectRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, http.NoBody) //nolint:noctx // no request is ever sent; this is the value a CheckRedirect policy receives
	if err != nil {
		t.Fatalf("http.NewRequest(%q) unexpected error: %v", rawURL, err)
	}
	req.Header.Set("PRIVATE-TOKEN", "glpat-test")
	req.Header.Set("Authorization", "Bearer gloas-test")
	req.Header.Set("Sudo", "root")
	req.Header.Set("Job-Token", "job-test")
	req.Header.Set("Accept", "application/json")
	return req
}

// TestCredentialSafeRedirect_StripsOutsideTheInstance verifies which redirect
// destinations keep the credential headers and which have them removed: the
// configured host and its subdomains keep them, any other host loses them, and
// an https-to-http downgrade loses them even though the hostname is unchanged.
// It also verifies that a non-credential header is never touched, so the
// policy cannot be passing by deleting everything.
func TestCredentialSafeRedirect_StripsOutsideTheInstance(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		dest    string
		want    bool // credential headers survive the hop
	}{
		{name: "same host and scheme", baseURL: "https://gitlab.example.com", dest: "https://gitlab.example.com/api/v4/user", want: true},
		{name: "same host different port", baseURL: "https://gitlab.example.com:8443", dest: "https://gitlab.example.com:9443/x", want: true},
		{name: "subdomain of the instance", baseURL: "https://gitlab.example.com", dest: "https://cdn.gitlab.example.com/x", want: true},
		{name: "host casing differs", baseURL: "https://GitLab.Example.COM", dest: "https://gitlab.example.com/x", want: true},
		{name: "plain http instance stays http", baseURL: "http://gitlab.internal", dest: "http://gitlab.internal/x", want: true},
		{name: "different host", baseURL: "https://gitlab.example.com", dest: "https://storage.example.net/x", want: false},
		{name: "parent of the instance", baseURL: "https://gitlab.example.com", dest: "https://example.com/x", want: false},
		{name: "suffix without a dot boundary", baseURL: "https://gitlab.example.com", dest: "https://evilgitlab.example.com/x", want: false},
		{name: "https downgraded to http", baseURL: "https://gitlab.example.com", dest: "http://gitlab.example.com/x", want: false},
		{name: "base url has no host", baseURL: "not a url at all", dest: "https://gitlab.example.com/x", want: false},
		{name: "empty base url", baseURL: "", dest: "https://gitlab.example.com/x", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := credentialSafeRedirect(tt.baseURL)
			req := newRedirectRequest(t, tt.dest)

			if err := policy(req, []*http.Request{}); err != nil {
				t.Fatalf("policy() unexpected error: %v", err)
			}

			for _, name := range credentialHeaders {
				got := req.Header.Get(name) != ""
				if got != tt.want {
					t.Errorf("header %s present = %v, want %v (base %q -> %q)", name, got, tt.want, tt.baseURL, tt.dest)
				}
			}
			if req.Header.Get("Accept") == "" {
				t.Error("Accept header was removed; the policy must only touch credential headers")
			}
		})
	}
}

// TestCredentialSafeRedirect_StopsAfterTenHops verifies the policy re-imposes
// the hop cap that setting CheckRedirect removes. net/http applies its
// ten-redirect limit inside the default policy, so a custom one that only
// edits headers and returns nil would otherwise follow a redirect loop
// forever.
func TestCredentialSafeRedirect_StopsAfterTenHops(t *testing.T) {
	tests := []struct {
		name    string
		hops    int
		wantErr bool
	}{
		{name: "first hop", hops: 0, wantErr: false},
		{name: "ninth hop", hops: 9, wantErr: false},
		{name: "tenth hop", hops: 10, wantErr: true},
		{name: "eleventh hop", hops: 11, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := credentialSafeRedirect("https://gitlab.example.com")
			via := make([]*http.Request, tt.hops)
			req := newRedirectRequest(t, "https://gitlab.example.com/x")

			err := policy(req, via)
			if (err != nil) != tt.wantErr {
				t.Errorf("policy() error = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

// TestWithinCredentialScope_NilDestination verifies the scope check refuses a
// destination it cannot read, so a malformed hop fails closed rather than
// being treated as same-instance.
func TestWithinCredentialScope_NilDestination(t *testing.T) {
	if withinCredentialScope("gitlab.example.com", true, nil) {
		t.Error("withinCredentialScope(nil destination) = true, want false")
	}
}

// TestIsDomainOrSubdomain_Boundaries verifies the host relation the policy
// shares with net/http: equality and dot-delimited suffixes match, bare
// suffixes and empty hosts do not.
func TestIsDomainOrSubdomain_Boundaries(t *testing.T) {
	tests := []struct {
		name   string
		sub    string
		parent string
		want   bool
	}{
		{name: "identical", sub: "gitlab.com", parent: "gitlab.com", want: true},
		{name: "one label deeper", sub: "cdn.gitlab.com", parent: "gitlab.com", want: true},
		{name: "two labels deeper", sub: "a.b.gitlab.com", parent: "gitlab.com", want: true},
		{name: "parent is deeper", sub: "gitlab.com", parent: "cdn.gitlab.com", want: false},
		{name: "suffix without dot", sub: "evilgitlab.com", parent: "gitlab.com", want: false},
		{name: "unrelated", sub: "example.net", parent: "gitlab.com", want: false},
		{name: "empty sub", sub: "", parent: "gitlab.com", want: false},
		{name: "empty parent", sub: "gitlab.com", parent: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDomainOrSubdomain(tt.sub, tt.parent); got != tt.want {
				t.Errorf("isDomainOrSubdomain(%q, %q) = %v, want %v", tt.sub, tt.parent, got, tt.want)
			}
		})
	}
}

// TestCredentialScope_ParsesBaseURL verifies the two facts the policy derives
// from a configured base URL, including the failure shape for a value that
// does not parse.
func TestCredentialScope_ParsesBaseURL(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		wantHost  string
		wantHTTPS bool
	}{
		{name: "https instance", baseURL: "https://gitlab.example.com/", wantHost: "gitlab.example.com", wantHTTPS: true},
		{name: "https with port", baseURL: "https://gitlab.example.com:8443", wantHost: "gitlab.example.com", wantHTTPS: true},
		{name: "http instance", baseURL: "http://gitlab.internal", wantHost: "gitlab.internal", wantHTTPS: false},
		{name: "surrounding whitespace", baseURL: "  https://gitlab.example.com  ", wantHost: "gitlab.example.com", wantHTTPS: true},
		{name: "uppercase scheme", baseURL: "HTTPS://gitlab.example.com", wantHost: "gitlab.example.com", wantHTTPS: true},
		{name: "unparseable", baseURL: "://%zz", wantHost: "", wantHTTPS: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, https := credentialScope(tt.baseURL)
			if host != tt.wantHost || https != tt.wantHTTPS {
				t.Errorf("credentialScope(%q) = (%q, %v), want (%q, %v)", tt.baseURL, host, https, tt.wantHost, tt.wantHTTPS)
			}
		})
	}
}

// TestCredentialSafeRedirect_DestinationWithoutHost verifies a destination URL
// carrying no host at all loses the credential headers rather than matching an
// empty configured host.
func TestCredentialSafeRedirect_DestinationWithoutHost(t *testing.T) {
	policy := credentialSafeRedirect("https://gitlab.example.com")
	req := newRedirectRequest(t, "https://gitlab.example.com/x")
	req.URL = &url.URL{Scheme: "https", Path: "/x"}

	if err := policy(req, nil); err != nil {
		t.Fatalf("policy() unexpected error: %v", err)
	}
	if req.Header.Get("PRIVATE-TOKEN") != "" {
		t.Error("PRIVATE-TOKEN survived a redirect to a URL with no host")
	}
}
