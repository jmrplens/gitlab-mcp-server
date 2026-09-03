// redirect_test.go contains unit tests for the redirect policy that keeps
// GitLab credential headers from leaving the configured instance.
package gitlab

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
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
		{name: "ipv6 zone literal spelled as a subdomain", baseURL: "https://gitlab.example.com", dest: "https://[::1%25.gitlab.example.com]/x", want: false},
		{name: "ipv6 zone literal on a plain http instance", baseURL: "http://gitlab.internal", dest: "http://[::1%25.gitlab.internal]:9102/x", want: false},
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
		{name: "zone suffixed ipv6 literal", sub: "::1%.gitlab.com", parent: "gitlab.com", want: false},
		{name: "bare ipv6 literal ending in the parent text", sub: "::1:gitlab.com", parent: "gitlab.com", want: false},
		{name: "identical ipv6 literal", sub: "::1", parent: "::1", want: true},
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

// captureRedirectLog redirects slog to a buffer for the duration of the test.
func captureRedirectLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(original) })
	return &buf
}

// TestCredentialSafeRedirect_RecordsTheHopThatLostTheHeaders verifies the
// policy says so when it withholds credentials, and says it once.
//
// The policy used to be completely silent, which left an operator looking at a
// 401 from object storage unable to tell a credential this server deliberately
// withheld from a credential that was never valid. The two need opposite
// responses, so the line is the difference between a diagnosable failure and a
// guess.
//
// The assertions are about what the record may and may not carry as much as
// about its presence. A presigned object-storage URL authenticates through its
// query parameters, so recording the destination URL would write a working
// credential to stderr while reporting that a credential was withheld; only the
// host and the scheme are recorded. Header names appear, header values must not.
func TestCredentialSafeRedirect_RecordsTheHopThatLostTheHeaders(t *testing.T) {
	buf := captureRedirectLog(t)
	policy := credentialSafeRedirect("https://gitlab.example.com")

	req := newRedirectRequest(t, "https://storage.example.net/artifacts/1?X-Amz-Signature=deadbeefsignature")
	if err := policy(req, nil); err != nil {
		t.Fatalf("policy() unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("log lines = %d, want 1: %s", len(lines), buf.String())
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("decoding the log record: %v (raw: %s)", err, lines[0])
	}

	for _, want := range []struct{ field, value string }{
		{"level", "INFO"},
		{"msg", "dropped credential headers on redirect"},
		{"instance_host", "gitlab.example.com"},
		{"redirect_host", "storage.example.net"},
		{"redirect_scheme", "https"},
	} {
		t.Run(want.field, func(t *testing.T) {
			if got, _ := record[want.field].(string); got != want.value {
				t.Errorf("%s = %q, want %q", want.field, got, want.value)
			}
		})
	}
	t.Run("names every header it dropped", func(t *testing.T) {
		headers, _ := record["headers"].(string)
		for _, name := range credentialHeaders {
			if !strings.Contains(headers, name) {
				t.Errorf("headers = %q, want it to name %s", headers, name)
			}
		}
	})
	t.Run("carries no credential value and no signed URL", func(t *testing.T) {
		for _, secret := range []string{"glpat-test", "gloas-test", "job-test", "X-Amz-Signature", "deadbeefsignature"} {
			t.Run(secret, func(t *testing.T) {
				if strings.Contains(buf.String(), secret) {
					t.Errorf("log record leaked %q: %s", secret, buf.String())
				}
			})
		}
	})
}

// TestCredentialSafeRedirect_LogsTheDowngradeReasonSeparately verifies an
// https-to-http hop on the configured host is reported as the downgrade it is
// rather than as an off-instance hop. net/http's own policy does not treat a
// downgrade as leaving the instance at all, so an operator reading
// "host outside the configured instance" for a hop whose host did not change
// would reasonably conclude the log was wrong and stop reading it.
func TestCredentialSafeRedirect_LogsTheDowngradeReasonSeparately(t *testing.T) {
	buf := captureRedirectLog(t)
	policy := credentialSafeRedirect("https://gitlab.example.com")

	req := newRedirectRequest(t, "http://gitlab.example.com/api/v4/projects")
	if err := policy(req, nil); err != nil {
		t.Fatalf("policy() unexpected error: %v", err)
	}

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record); err != nil {
		t.Fatalf("decoding the log record: %v (raw: %s)", err, buf.String())
	}
	if got, _ := record["reason"].(string); got != "redirect downgrades https to http" {
		t.Errorf("reason = %q, want the downgrade reason", got)
	}
}

// TestCredentialSafeRedirect_SaysNothingWhenNothingWasDropped verifies the two
// silent cases: a hop that stays on the instance, and a later hop off it that
// finds the headers already gone.
//
// The second is the one worth pinning. The policy runs per hop and the scope
// test keeps failing for every hop after the first, so reporting on the test
// rather than on the deletion would log the same withheld credential once per
// redirect in a chain, which reads as several separate incidents.
func TestCredentialSafeRedirect_SaysNothingWhenNothingWasDropped(t *testing.T) {
	tests := []struct {
		name string
		dest string
		bare bool // the request arrives with the credential headers already removed
	}{
		{name: "hop stays on the instance", dest: "https://gitlab.example.com/api/v4/projects"},
		{name: "second hop off the instance", dest: "https://storage.example.net/artifacts/2", bare: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureRedirectLog(t)
			req := newRedirectRequest(t, tt.dest)
			if tt.bare {
				for _, name := range credentialHeaders {
					req.Header.Del(name)
				}
			}
			if err := credentialSafeRedirect("https://gitlab.example.com")(req, nil); err != nil {
				t.Fatalf("policy() unexpected error: %v", err)
			}
			if buf.Len() != 0 {
				t.Errorf("log output = %q, want nothing", buf.String())
			}
		})
	}
}
