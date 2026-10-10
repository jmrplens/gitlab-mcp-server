package gitlab

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// maxRedirects is the number of hops a GitLab client follows before giving up.
//
// It is stated explicitly because setting [http.Client.CheckRedirect] replaces
// net/http's default policy wholesale, and the ten-hop cap lives inside that
// default. A custom policy that only edits headers and returns nil therefore
// follows redirects forever unless it counts them itself.
const maxRedirects = 10

// credentialHeaders are the request headers this server uses to prove who it
// is to GitLab, and the ones a redirect must not carry off the instance.
//
// PRIVATE-TOKEN is the one that matters: net/http strips Authorization,
// Www-Authenticate, Cookie, Cookie2, Proxy-Authorization and
// Proxy-Authenticate on a cross-host redirect and nothing else, so GitLab's
// personal-access-token header — the only credential stdio mode has, and HTTP
// legacy mode's default — rides along to whatever host answers the 302.
// Authorization is listed anyway because net/http compares hostnames alone and
// so keeps it across an https-to-http downgrade, and Sudo, Job-Token and
// Deploy-Token are listed because they are credentials of the same kind even
// though this server does not send them today: GitLab reads Deploy-Token on
// its package registry routes, and neither this server nor client-go sets it.
var credentialHeaders = []string{
	"PRIVATE-TOKEN",
	"Authorization",
	"Sudo",
	"Job-Token",
	"Deploy-Token",
}

// credentialSafeRedirect returns a redirect policy that keeps following
// redirects but drops the credential headers as soon as a hop leaves the
// configured GitLab instance.
//
// Refusing cross-host redirects outright would be simpler and is wrong here:
// job artifacts, job traces and package downloads are answered by GitLab with
// a 302 to object storage or a CDN whenever object storage is configured,
// which is GitLab.com and most self-managed instances, so six shipped
// read-only actions exist only because that redirect is followed. Those
// presigned URLs authenticate through query parameters, so the credential
// headers cost nothing to drop and the download still works.
//
// "Leaves the instance" means the destination host is neither the configured
// host nor a subdomain of it — the same relation net/http applies to its own
// sensitive headers — or the hop downgrades https to http, which net/http does
// not treat as leaving at all because it compares hostnames and ignores the
// scheme. Both hosts are compared as [FoldHostCase] leaves them, with ASCII
// letters folded and nothing else. A base URL that cannot be parsed, or
// carries no host, strips on every redirect: without a host to compare
// against there is no hop that can be shown to be safe.
//
// Once a hop has left, every later hop of the same chain is stripped too, the
// instance included, because net/http rebuilds each hop from the first
// request's headers: judged on its own, a hop back onto the instance would
// carry the credential again, to a path the host the chain left for chose.
func credentialSafeRedirect(baseURL string) func(*http.Request, []*http.Request) error {
	baseHost, baseHTTPS := credentialScope(baseURL)

	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		if !withinCredentialScope(baseHost, baseHTTPS, req.URL) || chainLeftScope(baseHost, baseHTTPS, via) {
			dropped := make([]string, 0, len(credentialHeaders))
			for _, name := range credentialHeaders {
				if req.Header.Get(name) == "" {
					continue
				}
				req.Header.Del(name)
				dropped = append(dropped, name)
			}
			logCredentialDrop(req, baseHost, baseHTTPS, dropped)
		}
		return nil
	}
}

// credentialScope reduces a configured base URL to the two facts the redirect
// policy compares against: the host with its case folded by [FoldHostCase],
// and whether the configured scheme was https.
func credentialScope(baseURL string) (host string, https bool) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", false
	}
	return FoldHostCase(u.Hostname()), strings.EqualFold(u.Scheme, "https")
}

// withinCredentialScope reports whether dest may still receive the credential
// headers.
func withinCredentialScope(baseHost string, baseHTTPS bool, dest *url.URL) bool {
	if baseHost == "" || dest == nil {
		return false
	}
	if baseHTTPS && !strings.EqualFold(dest.Scheme, "https") {
		return false
	}
	return isDomainOrSubdomain(FoldHostCase(dest.Hostname()), baseHost)
}

// chainLeftScope reports whether an earlier request of this redirect chain
// was outside the credential's scope.
//
// net/http builds every hop from the first request's headers, so a hop back
// onto the instance after one that left it would carry the credential again,
// to a path the host it left for chose: an object store a download was sent
// to could make this server run an authenticated request of its choosing on
// the instance, and hand the answer to the caller. net/http keeps its own
// sensitive headers stripped for the rest of a chain once a hop has left;
// this does the same for GitLab's. A request it cannot read counts as having
// left, since nothing shows that it stayed.
func chainLeftScope(baseHost string, baseHTTPS bool, via []*http.Request) bool {
	for _, prev := range via {
		if prev == nil || !withinCredentialScope(baseHost, baseHTTPS, prev.URL) {
			return true
		}
	}
	return false
}

// FoldHostCase returns host with its ASCII letters in lower case and every
// other byte as it was written, which is the form two spellings of a host are
// compared in wherever this server decides whether they name the same one.
//
// # Why not strings.ToLower
//
// strings.ToLower folds Unicode, and two runes outside ASCII fold into an
// ASCII letter: U+0130, the dotted capital I, becomes "i", and U+212A, the
// Kelvin sign, becomes "k". net/http dials "gİtlab.example.com" as
// xn--gitlab-qyd.example.com, while strings.ToLower makes it
// "gitlab.example.com". It dials the Kelvin sign as "k", but a plain-http
// request through an HTTP proxy names its host in the request line under the
// Punycode profile, which maps neither rune, so "Kube.example.com" is asked
// for as xn--ube-xk1a.example.com. A comparison through strings.ToLower
// therefore treated hosts somebody else can register as the configured
// instance, and let a redirect to them keep PRIVATE-TOKEN.
//
// # Why not the IDNA form either
//
// There is no single IDNA form to compare. net/http dials, names in a CONNECT
// and hands a SOCKS proxy the host's form under the Lookup profile, and writes
// its Host header, and the request line of a plain-http request through an
// HTTP proxy, under the Punycode profile, and the two differ for exactly these
// spellings: xn--gitlab-qyd against xn--gtlab-h4a. Folding ASCII alone can
// only be stricter than either. An ASCII host is dialed as written and DNS
// ignores its case; a name outside ASCII matches only the same bytes, which
// both profiles map alike; and an ASCII parent after a dot maps to itself, so
// a subdomain written outside ASCII is still a subdomain. Where it costs
// anything the cost is paid in the safe direction: the xn-- spelling of an
// instance configured in Unicode counts as another host, and so does the
// Kelvin-sign spelling, which a direct dial resolves to the instance and a
// proxied plain-http request does not.
//
// Byte by byte rather than rune by rune, so a host carrying bytes that are not
// UTF-8 keeps them: a rune-wise mapping would turn every such byte into
// U+FFFD and make two different hosts compare equal. No byte of a multi-byte
// UTF-8 sequence falls in the ASCII range, so none of them is touched.
//
// The copy is made at the first upper-case letter and not before, so a host
// already in lower case, which is nearly every one, is handed back as it came.
// One test of the byte decides both whether to copy and what to fold, since a
// separate scan for the first letter would ask the same question twice.
func FoldHostCase(host string) string {
	var folded []byte
	for i := range len(host) {
		if c := host[i]; 'A' <= c && c <= 'Z' {
			if folded == nil {
				folded = []byte(host)
			}
			folded[i] = c + ('a' - 'A')
		}
	}
	if folded == nil {
		return host
	}
	return string(folded)
}

// isDomainOrSubdomain reports whether sub is parent or a subdomain of it.
// It mirrors the unexported net/http helper of the same name, which decides
// whether an Authorization header survives a redirect.
func isDomainOrSubdomain(sub, parent string) bool {
	if sub == parent {
		return true
	}
	if sub == "" || parent == "" {
		return false
	}
	// An address literal is not a domain name, and one carrying an IPv6 zone
	// can be spelled to end in any parent: url.Hostname() reduces
	// "[::1%25.gitlab.example.com]" to "::1%.gitlab.example.com", which passes
	// both the dot boundary and the suffix test while Go dials it as ::1. This
	// is CVE-2023-45289, and net/http grew the same guard in the same place.
	if strings.ContainsAny(sub, ":%") {
		return false
	}
	if len(sub) <= len(parent) || sub[len(sub)-len(parent)-1] != '.' {
		return false
	}
	return strings.HasSuffix(sub, parent)
}

// logCredentialDrop records that a redirect left the configured instance
// carrying none of this server's credentials.
//
// Prevention is [credentialSafeRedirect]; this is only about being able to
// explain it afterwards. Without the line the policy is completely silent, and
// an operator looking at a 401 from object storage cannot tell a credential
// this server deliberately withheld from a credential that was never valid in
// the first place. Those two call for opposite responses: the first means the
// presigned URL is the credential and something is wrong with it, the second
// means the token needs replacing.
//
// INFO rather than DEBUG. The event is bounded by the tool-call rate, which is
// already one INFO line per call in this server, so a redirect line cannot be
// what makes the log noisy; and the question it answers is asked by someone who
// does not yet know to raise the level, which is exactly the case DEBUG does
// not serve.
//
// The destination is recorded as a host and a scheme, never as a URL. The whole
// reason this redirect is followed rather than refused is that GitLab answers
// artifact, trace and package reads with a 302 to object storage, and those
// URLs authenticate through query parameters: logging one would write a working
// credential to stderr in the course of reporting that a credential was
// withheld. Header names are recorded, values are not.
func logCredentialDrop(req *http.Request, baseHost string, baseHTTPS bool, dropped []string) {
	if len(dropped) == 0 {
		return
	}
	slog.InfoContext(req.Context(), "dropped credential headers on redirect",
		"reason", credentialDropReason(req.URL, baseHost, baseHTTPS),
		"instance_host", baseHost,
		"redirect_host", req.URL.Hostname(),
		"redirect_scheme", req.URL.Scheme,
		"headers", strings.Join(dropped, ", "),
	)
}

// credentialDropReason names why a redirect to dest lost its credentials: it
// left https for http, or it is inside the instance's scope and lost them
// because an earlier hop had left it, or it is a host outside the configured
// instance. The downgrade is asked first because it is the one a reader acts
// on whatever the host.
func credentialDropReason(dest *url.URL, baseHost string, baseHTTPS bool) string {
	if baseHTTPS && !strings.EqualFold(dest.Scheme, "https") {
		return "redirect downgrades https to http"
	}
	if withinCredentialScope(baseHost, baseHTTPS, dest) {
		return "an earlier hop left the configured instance"
	}
	return "host outside the configured instance"
}
