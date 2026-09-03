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
// so keeps it across an https-to-http downgrade, and Sudo and Job-Token are
// listed because they are credentials of the same kind even though this server
// does not send them today.
var credentialHeaders = []string{
	"PRIVATE-TOKEN",
	"Authorization",
	"Sudo",
	"Job-Token",
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
// scheme. A base URL that cannot be parsed, or carries no host, strips on
// every redirect: without a host to compare against there is no hop that can
// be shown to be safe.
func credentialSafeRedirect(baseURL string) func(*http.Request, []*http.Request) error {
	baseHost, baseHTTPS := credentialScope(baseURL)

	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		if !withinCredentialScope(baseHost, baseHTTPS, req.URL) {
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
// policy compares against: the lower-cased host, and whether the configured
// scheme was https.
func credentialScope(baseURL string) (host string, https bool) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", false
	}
	return strings.ToLower(u.Hostname()), strings.EqualFold(u.Scheme, "https")
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
	return isDomainOrSubdomain(strings.ToLower(dest.Hostname()), baseHost)
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
	reason := "host outside the configured instance"
	if baseHTTPS && !strings.EqualFold(req.URL.Scheme, "https") {
		reason = "redirect downgrades https to http"
	}
	slog.InfoContext(req.Context(), "dropped credential headers on redirect",
		"reason", reason,
		"instance_host", baseHost,
		"redirect_host", req.URL.Hostname(),
		"redirect_scheme", req.URL.Scheme,
		"headers", strings.Join(dropped, ", "),
	)
}
