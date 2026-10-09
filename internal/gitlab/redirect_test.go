// redirect_test.go contains unit tests for the redirect policy that keeps
// GitLab credential headers from leaving the configured instance.
package gitlab

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
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
	req.Header.Set("Deploy-Token", "deploy-test")
	req.Header.Set("Accept", "application/json")
	return req
}

// gitlabCredentialHeaders are the request headers GitLab authenticates a
// caller by, written out here rather than read from [credentialHeaders] so
// that a header missing from the policy's list fails the tests below instead
// of being skipped by them.
var gitlabCredentialHeaders = []string{"PRIVATE-TOKEN", "Authorization", "Sudo", "Job-Token", "Deploy-Token"}

// TestCredentialSafeRedirect_StripsOutsideTheInstance verifies which redirect
// destinations keep the credential headers and which have them removed: the
// configured host and its subdomains keep them, any other host loses them, and
// an https-to-http downgrade loses them even though the hostname is unchanged.
// It also verifies that a non-credential header is never touched, so the
// policy cannot be passing by deleting everything.
//
// The spelling rows are the comparison itself. Only ASCII letters are folded
// before two hosts are compared: strings.ToLower also turns U+0130, the
// dotted capital I, into a plain "i", so "gİtlab.example.com" compared equal
// to the instance while net/http dialed its IDNA form,
// xn--gitlab-qyd.example.com, which is another host with another owner. A
// name outside ASCII therefore matches only when it is the configured name
// byte for byte, and the Kelvin sign, which net/http dials as "k" but names
// in a proxied plain-http request line as xn--ube-xk1a, loses the headers
// too.
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
		{name: "destination in ascii upper case", baseURL: "https://gitlab.example.com", dest: "https://GITLAB.Example.com/x", want: true},
		{name: "dotted capital I written raw", baseURL: "https://gitlab.example.com", dest: "https://gİtlab.example.com/x", want: false},
		{name: "dotted capital I percent-encoded", baseURL: "https://gitlab.example.com", dest: "https://g%C4%B0tlab.example.com/x", want: false},
		{name: "dotted capital I under a subdomain label", baseURL: "https://cdn.gitlab.example.com", dest: "https://cdn.gİtlab.example.com/x", want: false},
		{name: "dotted capital I above a subdomain label", baseURL: "https://gitlab.example.com", dest: "https://cdn.gİtlab.example.com/x", want: false},
		{name: "dotted capital I in the configured instance", baseURL: "https://gİtlab.example.com", dest: "https://gitlab.example.com/x", want: false},
		{name: "an instance written with a dotted capital I keeps its own spelling", baseURL: "https://gİtlab.example.com", dest: "https://g%C4%B0tlab.example.com/x", want: true},
		{name: "a label outside ascii below the instance", baseURL: "https://gitlab.example.com", dest: "https://café.gitlab.example.com/x", want: true},
		{name: "kelvin sign spelling of the instance", baseURL: "https://kube.example.com", dest: "https://Kube.example.com/x", want: false},
		{name: "rooted name of the instance", baseURL: "https://gitlab.example.com", dest: "https://gitlab.example.com./x", want: false},
		{name: "rooted instance and rooted destination", baseURL: "https://gitlab.example.com.", dest: "https://GitLab.Example.com./x", want: true},
		{name: "ipv6 zone literal with an upper-case subdomain", baseURL: "https://gitlab.example.com", dest: "https://[::1%25.GITLAB.example.com]/x", want: false},
		{name: "ipv6 zone literal with a dotted capital I", baseURL: "https://gitlab.example.com", dest: "https://[::1%25.GİTLAB.example.com]/x", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := credentialSafeRedirect(tt.baseURL)
			req := newRedirectRequest(t, tt.dest)

			if err := policy(req, []*http.Request{}); err != nil {
				t.Fatalf("policy() unexpected error: %v", err)
			}

			for _, name := range gitlabCredentialHeaders {
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
		// An empty parent is a base URL this server could not read a host
		// out of, and every hostname ends with the empty string. The emptiness
		// guard is the only thing between that and "matches everything": the
		// dot-boundary test passes any name written with a trailing dot, and
		// the suffix test passes them all, so a rooted FQDN reaches the end of
		// the function and comes back true if the guard is not doing its job.
		{name: "rooted name against an unreadable parent", sub: "storage.example.net.", parent: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDomainOrSubdomain(tt.sub, tt.parent); got != tt.want {
				t.Errorf("isDomainOrSubdomain(%q, %q) = %v, want %v", tt.sub, tt.parent, got, tt.want)
			}
		})
	}
}

// TestFoldHostCase_FoldsASCIILettersAndNothingElse pins the comparison form
// every host decision in the server shares.
//
// The rows outside ASCII are the reason it exists: strings.ToLower turns
// U+0130 into "i" and U+212A into "k", and net/http sends the first to
// xn--gitlab-qyd rather than gitlab, and the second, through an HTTP proxy,
// to xn--ube-xk1a rather than kube. The "Z" row and the row that puts "A"
// before the bytes either side of the capitals hold the range's ends, so a
// fold that missed "A" or "Z" or took in "@" or "[" is caught. The row that
// is not UTF-8 is about
// bytes rather than runes: a fold written over runes would replace each byte
// that is not UTF-8 with U+FFFD, so two different hosts would come out equal.
func TestFoldHostCase_FoldsASCIILettersAndNothingElse(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{name: "already lower case", host: "gitlab.example.com", want: "gitlab.example.com"},
		{name: "ascii upper case", host: "GitLab.Example.COM", want: "gitlab.example.com"},
		{name: "upper case after the first byte only", host: "gitlab.example.CoM", want: "gitlab.example.com"},
		{name: "digits, hyphens and a port", host: "GITLAB-01.example.com:8443", want: "gitlab-01.example.com:8443"},
		{name: "the last ascii capital", host: "ZONE.example.com", want: "zone.example.com"},
		{name: "the first ascii capital and the bytes either side of the capitals", host: "A@[`{", want: "a@[`{"},
		{name: "bracketed ipv6 literal", host: "[FD00:EC2::254]:443", want: "[fd00:ec2::254]:443"},
		{name: "dotted capital I", host: "GİTLAB.example.com", want: "gİtlab.example.com"},
		{name: "kelvin sign", host: "KUBE.example.com", want: "Kube.example.com"},
		{name: "other letters outside ascii", host: "CAFÉ.example.com", want: "cafÉ.example.com"},
		{name: "bytes that are not utf-8", host: "G\xc4TLAB", want: "g\xc4tlab"},
		{name: "empty", host: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FoldHostCase(tt.host); got != tt.want {
				t.Errorf("FoldHostCase(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}

	t.Run("two hosts differing only outside ascii stay apart", func(t *testing.T) {
		if FoldHostCase("G\xc4TLAB") == FoldHostCase("G\xc5TLAB") {
			t.Error("two hosts that differ in a byte outside ascii were folded together")
		}
	})
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
		{name: "uppercase host", baseURL: "https://GitLab.Example.COM", wantHost: "gitlab.example.com", wantHTTPS: true},
		{name: "dotted capital I written raw", baseURL: "https://gİtlab.example.com", wantHost: "gİtlab.example.com", wantHTTPS: true},
		{name: "dotted capital I percent-encoded", baseURL: "https://G%C4%B0TLAB.example.com", wantHost: "gİtlab.example.com", wantHTTPS: true},
		{name: "rooted name", baseURL: "https://GitLab.example.com.", wantHost: "gitlab.example.com.", wantHTTPS: true},
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

// TestCredentialSafeRedirect_LookalikeOfTheInstance_ReceivesNoToken follows a
// redirect through the whole client chain and reads what arrived on the other
// side of it.
//
// The lookalike is the instance's own name spelled with U+0130, the dotted
// capital I, percent-encoded as a Location header carries it. The policy used
// to fold that spelling into the instance's name and keep PRIVATE-TOKEN on
// the hop, while net/http sent the hop to the name's IDNA form, a host
// whoever registered it controls. net/http's own stripping does not cover the
// header, since PRIVATE-TOKEN is not one of the headers it knows to be
// sensitive.
//
// The second row is why the instance's administrator is not the only one who
// could send a request there. net/http builds every hop from the headers of
// the first request and the policy judges each hop afresh, so the object
// store an artifact download is redirected to, which received no token, could
// answer with a redirect of its own to the lookalike, and the token came back
// for that hop.
//
// A forward proxy stands in for the network, because it is the one place a
// unit test can see the request for a name nothing resolves: it records the
// host each request was addressed to and the token that came with it.
func TestCredentialSafeRedirect_LookalikeOfTheInstance_ReceivesNoToken(t *testing.T) {
	const (
		instanceURL = "http://gitlab.example.com"
		lookalike   = "http://g%C4%B0tlab.example.com" + versionAPIPath
	)
	tests := []struct {
		name      string
		locations map[string]string // host the proxy was asked for -> where it redirects
		hops      int
	}{
		{
			name:      "the instance redirects to the lookalike",
			locations: map[string]string{"gitlab.example.com": lookalike},
			hops:      2,
		},
		{
			name: "the object store the instance redirects to bounces to the lookalike",
			locations: map[string]string{
				"gitlab.example.com":  "http://storage.example.net" + versionAPIPath,
				"storage.example.net": lookalike,
			},
			hops: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			tokens := map[string]string{}
			stub := newProxyStub(t, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				tokens[r.Host] = r.Header.Get("PRIVATE-TOKEN")
				mu.Unlock()
				if location, ok := tt.locations[r.Host]; ok {
					http.Redirect(w, r, location, http.StatusFound)
					return
				}
				versionAnswer(w, r)
			})
			client := proxiedClient(instanceURL, proxiedPools(t, stub.url), publicPolicy(instanceURL))

			if _, err := client.versionDirect(t.Context()); err != nil {
				t.Fatalf("versionDirect() unexpected error: %v", err)
			}

			asked := stub.asked()
			if len(asked) != tt.hops || asked[0] != "gitlab.example.com" {
				t.Fatalf("the proxy was asked for %v, want the instance and then %d redirect targets", asked, tt.hops-1)
			}
			mu.Lock()
			defer mu.Unlock()
			if tokens["gitlab.example.com"] == "" {
				t.Error("the instance itself received no token, so the test proves nothing about the hops")
			}
			for _, host := range asked[1:] {
				if got := tokens[host]; got != "" {
					t.Errorf("the hop to %s carried PRIVATE-TOKEN %q, a host other than the instance", host, got)
				}
			}
		})
	}
}

// TestCredentialSafeRedirect_KelvinSignThroughAnHTTPProxy_ReceivesNoToken pins
// why the Kelvin sign, U+212A, is not the same host as the "k" it folds to.
//
// net/http dials "Kube.example.com" (Kelvin sign) as kube.example.com, but a
// plain-http request through an HTTP proxy names its host in the request line
// under the Punycode profile, which maps neither U+212A nor U+0130, so the
// proxy is asked for xn--ube-xk1a.example.com, a different host. A comparison
// through strings.ToLower took that spelling for the instance and kept
// PRIVATE-TOKEN on the hop. The first assertion holds the premise: if the
// proxy were asked for the instance's own name, the row would prove nothing.
func TestCredentialSafeRedirect_KelvinSignThroughAnHTTPProxy_ReceivesNoToken(t *testing.T) {
	const instanceURL = "http://kube.example.com"
	var mu sync.Mutex
	tokens := map[string]string{}
	stub := newProxyStub(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		tokens[r.Host] = r.Header.Get("PRIVATE-TOKEN")
		mu.Unlock()
		if r.Host == "kube.example.com" {
			http.Redirect(w, r, "http://Kube.example.com"+versionAPIPath, http.StatusFound)
			return
		}
		versionAnswer(w, r)
	})
	client := proxiedClient(instanceURL, proxiedPools(t, stub.url), publicPolicy(instanceURL))

	if _, err := client.versionDirect(t.Context()); err != nil {
		t.Fatalf("versionDirect() unexpected error: %v", err)
	}

	if got := stub.asked(); !slices.Equal(got, []string{"kube.example.com", "xn--ube-xk1a.example.com"}) {
		t.Fatalf("the proxy was asked for %v, want the instance and then the Punycode form of the Kelvin spelling", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if tokens["kube.example.com"] == "" {
		t.Error("the instance itself received no token, so the test proves nothing about the hop")
	}
	if got := tokens["xn--ube-xk1a.example.com"]; got != "" {
		t.Errorf("the hop to xn--ube-xk1a.example.com carried PRIVATE-TOKEN %q", got)
	}
}

// TestCredentialSafeRedirect_ChainThatLeftTheInstance_ComesBackWithoutToken
// follows a chain that leaves the instance and comes back to it, through the
// whole client.
//
// net/http builds every hop from the headers of the first request, so a hop
// the policy judges on its own merits, the instance again, would carry
// PRIVATE-TOKEN once more, to a path the host the chain had left for chose.
// net/http keeps its own sensitive headers stripped for the rest of a chain
// once a hop has left, and the policy does the same for GitLab's: the object
// store an artifact download is sent to must not be able to make this server
// run an authenticated request of its choosing on the instance.
func TestCredentialSafeRedirect_ChainThatLeftTheInstance_ComesBackWithoutToken(t *testing.T) {
	const instanceURL = "http://gitlab.example.com"
	var mu sync.Mutex
	tokens := map[string]string{}
	stub := newProxyStub(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		tokens[r.Host+r.URL.Path] = r.Header.Get("PRIVATE-TOKEN")
		mu.Unlock()
		switch r.Host + r.URL.Path {
		case "gitlab.example.com" + versionAPIPath:
			http.Redirect(w, r, "http://storage.example.net/bucket/object", http.StatusFound)
		case "storage.example.net/bucket/object":
			http.Redirect(w, r, "http://gitlab.example.com/api/v4/user", http.StatusFound)
		default:
			versionAnswer(w, r)
		}
	})
	client := proxiedClient(instanceURL, proxiedPools(t, stub.url), publicPolicy(instanceURL))

	if _, err := client.versionDirect(t.Context()); err != nil {
		t.Fatalf("versionDirect() unexpected error: %v", err)
	}

	if got := stub.asked(); !slices.Equal(got, []string{"gitlab.example.com", "storage.example.net", "gitlab.example.com"}) {
		t.Fatalf("the proxy was asked for %v, want the instance, the object store and the instance again", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if tokens["gitlab.example.com"+versionAPIPath] == "" {
		t.Fatal("the instance received no token on the first request, so the test proves nothing")
	}
	if got := tokens["storage.example.net/bucket/object"]; got != "" {
		t.Errorf("the hop to the object store carried PRIVATE-TOKEN %q", got)
	}
	if got := tokens["gitlab.example.com/api/v4/user"]; got != "" {
		t.Errorf("the hop back onto the instance carried PRIVATE-TOKEN %q after the chain had left it", got)
	}
}

// TestCredentialSafeRedirect_AfterAHopLeftTheInstance_StripsEveryLaterHop
// pins the chain rule at the policy itself, one row per way an earlier hop can
// have left the scope.
//
// The via slice is what net/http hands the policy: every request of the chain
// so far, the first one included. A hop back onto the instance keeps the
// headers only when no earlier request left it, by host or by a downgrade to
// http, and a request the policy cannot read counts as having left, since
// nothing shows it stayed. An empty string in a row's chain stands for such a
// request, a nil one.
func TestCredentialSafeRedirect_AfterAHopLeftTheInstance_StripsEveryLaterHop(t *testing.T) {
	const (
		base  = "https://gitlab.example.com"
		first = base + "/api/v4/jobs/1/artifacts"
	)
	tests := []struct {
		name  string
		chain []string // the URLs of the earlier requests, "" for one that cannot be read
		dest  string
		want  bool
	}{
		{name: "every earlier hop stayed on the instance", chain: []string{first, "https://cdn.gitlab.example.com/x"}, dest: first, want: true},
		{name: "an earlier hop went to another host", chain: []string{first, "https://storage.example.net/bucket/object"}, dest: base + "/api/v4/user"},
		{name: "an earlier hop went to the lookalike", chain: []string{first, "https://g%C4%B0tlab.example.com/x"}, dest: base + "/api/v4/user"},
		{name: "an earlier hop downgraded to http", chain: []string{first, "http://gitlab.example.com/x"}, dest: base + "/api/v4/user"},
		{name: "an earlier hop the policy cannot read", chain: []string{first, ""}, dest: base + "/api/v4/user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			via := make([]*http.Request, len(tt.chain))
			for i, rawURL := range tt.chain {
				if rawURL != "" {
					via[i] = newRedirectRequest(t, rawURL)
				}
			}
			req := newRedirectRequest(t, tt.dest)
			if err := credentialSafeRedirect(base)(req, via); err != nil {
				t.Fatalf("policy() unexpected error: %v", err)
			}
			for _, name := range gitlabCredentialHeaders {
				if got := req.Header.Get(name) != ""; got != tt.want {
					t.Errorf("header %s present = %v, want %v", name, got, tt.want)
				}
			}
			if req.Header.Get("Accept") == "" {
				t.Error("Accept was removed; the policy must only touch credential headers")
			}
		})
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
		// The hop stays on https, so the downgrade reason must not be the one
		// reported: an operator told a same-scheme hop downgraded to http
		// would conclude the log is wrong and stop reading it. The downgrade
		// wording has its own test below, and the two must not collapse into
		// one another.
		{"reason", "host outside the configured instance"},
	} {
		t.Run(want.field, func(t *testing.T) {
			if got, _ := record[want.field].(string); got != want.value {
				t.Errorf("%s = %q, want %q", want.field, got, want.value)
			}
		})
	}
	t.Run("names every header it dropped", func(t *testing.T) {
		headers, _ := record["headers"].(string)
		for _, name := range gitlabCredentialHeaders {
			if !strings.Contains(headers, name) {
				t.Errorf("headers = %q, want it to name %s", headers, name)
			}
		}
	})
	t.Run("carries no credential value and no signed URL", func(t *testing.T) {
		for _, secret := range []string{"glpat-test", "gloas-test", "job-test", "deploy-test", "X-Amz-Signature", "deadbeefsignature"} {
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

// TestCredentialSafeRedirect_LogsTheChainReasonForAHopBackOntoTheInstance
// verifies a hop that lost the headers only because an earlier hop of its
// chain left the instance says so. Its own host is the instance's and its
// scheme is the instance's, so either of the other two reasons would describe
// a hop that did not happen.
func TestCredentialSafeRedirect_LogsTheChainReasonForAHopBackOntoTheInstance(t *testing.T) {
	buf := captureRedirectLog(t)
	policy := credentialSafeRedirect("https://gitlab.example.com")

	via := []*http.Request{
		newRedirectRequest(t, "https://gitlab.example.com/api/v4/jobs/1/artifacts"),
		newRedirectRequest(t, "https://storage.example.net/bucket/object"),
	}
	req := newRedirectRequest(t, "https://gitlab.example.com/api/v4/user")
	if err := policy(req, via); err != nil {
		t.Fatalf("policy() unexpected error: %v", err)
	}

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record); err != nil {
		t.Fatalf("decoding the log record: %v (raw: %s)", err, buf.String())
	}
	if got, _ := record["reason"].(string); got != "an earlier hop left the configured instance" {
		t.Errorf("reason = %q, want the chain reason", got)
	}
	if got, _ := record["redirect_host"].(string); got != "gitlab.example.com" {
		t.Errorf("redirect_host = %q, want the instance the hop came back to", got)
	}
}

// TestCredentialSafeRedirect_SaysNothingWhenNothingWasDropped verifies the two
// silent cases: a hop that stays on the instance, and a hop off it whose
// request carries no credential at all.
//
// The second is the one worth pinning, because the line reports a deletion
// and not a failed scope test: a request that never carried a credential has
// nothing withheld from it, and a line saying otherwise would send an operator
// looking for a token problem that does not exist. A request that did carry
// one is reported on every hop from the first that leaves the instance to the
// end of the chain, since net/http builds each hop from the first request's
// headers and so restores what the previous hop had removed.
func TestCredentialSafeRedirect_SaysNothingWhenNothingWasDropped(t *testing.T) {
	tests := []struct {
		name string
		dest string
		bare bool // the request carries no credential header
	}{
		{name: "hop stays on the instance", dest: "https://gitlab.example.com/api/v4/projects"},
		{name: "hop off the instance with no credential", dest: "https://storage.example.net/artifacts/2", bare: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureRedirectLog(t)
			req := newRedirectRequest(t, tt.dest)
			if tt.bare {
				for _, name := range gitlabCredentialHeaders {
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

// TestCredentialSafeRedirect_HopToMetadataAddress_Refused pins the half of the
// redirect policy that stripping credentials never covered.
//
// Following a cross-host 302 is not optional: six shipped read-only actions
// exist only because GitLab answers artifact, trace and package reads with a
// redirect to object storage. What the credential policy cannot decide is
// WHERE that hop goes, because the destination is chosen by whatever answered
// rather than by the operator, and the body comes back to the caller —
// job.trace returns up to 100 KiB of it raw. A GitLab that is compromised, or
// a cleartext leg where a response can be injected, therefore reached a cloud
// metadata endpoint and reflected it, on an ordinary pinned deployment the
// instance allow-list had nothing to say about.
//
// The two rows are the whole trade. Object storage on the same private network
// as a self-managed instance still works, because a deployment whose instance
// is itself private is already inside that network. The metadata address does
// not, on the same deployment, in the same test: tier A applies to every hop
// of every client, and is not what --allow-private-instances permits.
func TestCredentialSafeRedirect_HopToMetadataAddress_Refused(t *testing.T) {
	// "localhost" and "127.0.0.1" name the same loopback address and are
	// different hosts, which is what makes the hop leave the instance without
	// either end becoming unreachable. httptest listens on the literal, so the
	// instance is the end addressed by name.
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("artifact bytes"))
	}))
	t.Cleanup(storage.Close)

	tests := []struct {
		name        string
		location    string
		wantRefused bool
		wantBody    string
	}{
		{
			name:     "a hop to ordinary object storage still succeeds",
			location: storage.URL + "/artifact.zip",
			wantBody: "artifact bytes",
		},
		{
			name:        "a hop to the cloud metadata address is refused",
			location:    "http://169.254.169.254/latest/meta-data/iam/security-credentials/",
			wantRefused: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := followOneRedirect(t, tt.location)
			if resp != nil {
				defer resp.Body.Close()
			}

			if tt.wantRefused {
				if !errors.Is(err, ErrDestinationRefused) {
					t.Fatalf("err = %v, want a refusal: the hop reached a metadata address", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("a hop to object storage on the instance's own private network was refused: %v", err)
			}
			body, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				t.Fatalf("reading the redirected body: %v", readErr)
			}
			if string(body) != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

// followOneRedirect stands up a GitLab that answers /api/v4/version with a 302
// to location, and reads that redirect through a client configured for it.
//
// The instance is addressed as "localhost" while httptest listens on the
// literal 127.0.0.1, which is what makes any hop to another loopback service a
// hop that left the instance: the two names are different hosts and the same
// address.
func followOneRedirect(t *testing.T, location string) (*http.Response, error) {
	t.Helper()

	instance := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, location, http.StatusFound)
	}))
	t.Cleanup(instance.Close)

	instanceURL := strings.Replace(instance.URL, "127.0.0.1", "localhost", 1)
	client, err := NewClient(&config.Config{GitLabURL: instanceURL, GitLabToken: "glpat-x", DisableRetries: true})
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, instanceURL+"/api/v4/version", http.NoBody)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext() unexpected error: %v", err)
	}
	return client.healthClient.Do(req)
}
