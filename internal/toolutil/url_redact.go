package toolutil

import (
	"net/url"
	"strings"
)

// RedactedPlaceholder is what a formatter writes instead of a value it will
// not echo, so that a reader sees something was withheld rather than that a
// tool had nothing to report. It is the one spelling every such site uses, so
// a reader who has met it once recognizes it everywhere.
const RedactedPlaceholder = "[redacted]"

// urlElidedPathMarker is appended to an origin whose path, query or fragment
// was dropped, so a reader can tell a bare origin from one that carried more.
const urlElidedPathMarker = "/..."

// RedactURL renders u with the three parts a secret can hide in removed: the
// userinfo, the query and the fragment. What survives is the scheme, the host
// and the path, which is what identifies the destination.
//
// A URL reaches a model through this server whenever a tool reports where it
// is talking to, and each of the three dropped parts is somewhere a credential
// is routinely written: a proxy login in the userinfo, a signed token in the
// query, a fragment carried over from a browser. None of them identifies the
// destination, so none of them is worth the risk of echoing.
//
// The path survives because for an instance URL it names the installation (a
// GitLab served under a subpath) rather than the caller, and because client-go
// builds its own error text from the same three parts, which leaves the two
// agreeing instead of this being the one place that says more. Use
// [RedactURLToOrigin] for a destination whose path is itself the secret.
func RedactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Scheme + "://" + u.Host + u.EscapedPath()
}

// RedactURLToOrigin parses raw and renders its origin alone: the scheme and
// the host. It drops what [RedactURL] drops and the path with it, replacing a
// path, query or fragment that was there with "/..." so a reader can see that
// something was withheld.
//
// The path goes because for a webhook, an integration endpoint or any other
// callback URL the path is where the secret lives: a Slack or Teams hook is a
// public host with an unguessable path, and truncating such a URL to a fixed
// number of characters leaks the front of it, userinfo included. The host is
// what an auditor reads these values for, and the host is all that is left.
//
// Text that does not parse as a URL, or that names no scheme and host, is
// reported as [RedactedPlaceholder] rather than echoed: a value that is not a
// URL is a value nothing here has judged.
func RedactURLToOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return RedactedPlaceholder
	}
	origin := u.Scheme + "://" + u.Host
	if path := u.EscapedPath(); path != "" && path != "/" {
		return origin + urlElidedPathMarker
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return origin + urlElidedPathMarker
	}
	return origin
}
