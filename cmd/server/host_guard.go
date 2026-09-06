// host_guard.go decides which Host header this deployment answers.
//
// The check exists against DNS rebinding: an attacker resolves a name they
// control to the address a local server listens on, and a browser they have
// already loaded then reaches that server with the attacker's name in the Host
// header. Refusing a name the deployment never declared is what stops it.
//
// Every reverse proxy in the deployment guide forwards the client's Host
// (nginx's `proxy_set_header Host $host`, Apache's `ProxyPreserveHost On`,
// Caddy and Traefik by default) and connects over loopback, which is exactly
// the shape the guard was written to refuse. Two of them were refusing it: this
// middleware, and the SDK's own localhost protection inside the streamable
// handler. So the operator declares the host instead, with --public-url, and a
// hop they listed in --trusted-proxies may forward whatever host it heard. Both
// checks live here now, since only this layer can see either flag: the SDK's is
// switched off in [streamableHTTPOptions] and its rule is reproduced below.

package main

import (
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/netip"
	"net/url"
)

// hostGuard is a deployment's policy on the Host header a request may carry.
type hostGuard struct {
	// declared is every host this deployment answers to: the loopback names,
	// the host --http-addr binds when it names one, and the host --public-url
	// advertises. Never nil.
	declared map[string]bool
	// bound reports whether --http-addr named a single host. When it did, a
	// Host outside the declared set is refused outright, whatever address the
	// request arrived on: the operator named the interface, so a request for
	// another name is nobody's health check. A wildcard bind or a unix socket
	// names none, and only the rebinding rule below applies.
	bound bool
	// proxies are the addresses whose forwarded Host is believed, from
	// --trusted-proxies.
	proxies trustedProxies
}

// newHostGuard builds the policy from the listen address, the advertised
// public origin and the trusted proxy list.
func newHostGuard(addr, publicURL string, proxies trustedProxies) hostGuard {
	guard := hostGuard{
		declared: map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true},
		proxies:  proxies,
	}
	if bound := allowedHosts(addr); bound != nil {
		guard.bound = true
		maps.Copy(guard.declared, bound)
	}
	// The operator has already told clients to reach this deployment by that
	// name, in the RFC 9728 metadata and in every configuration snippet that
	// sets it, so the name is declared in the strongest sense the flags offer.
	if host := publicURLHost(publicURL); host != "" {
		guard.declared[host] = true
	}
	return guard
}

// permits reports whether the request's Host header is one this deployment
// answers.
func (g hostGuard) permits(r *http.Request) bool {
	// A request naming no host at all is not the attack this guards against.
	// DNS rebinding works by making a browser resolve a name the attacker
	// controls to the loopback address, and the browser then puts that name in
	// the Host header; no browser omits it. What does omit it is a health
	// check: HAProxy's `option httpchk` sends no Host unless one is
	// configured, so a listener bound to 127.0.0.1 answered every check with
	// 403 and was marked permanently DOWN by a balancer that was working
	// correctly. Refusing it bought nothing, since anything able to send a
	// header-less request can reach the listener directly and does not need a
	// browser to do it for it.
	if r.Host == "" {
		return true
	}
	if g.declared[hostOnly(r.Host)] {
		return true
	}
	// A proxy the operator listed is a hop they vouched for, and forwarding
	// the client's Host is what every proxy in the deployment guide is
	// configured to do. A browser carrying out a rebinding attack is not that
	// hop: it reaches the listener itself, from an address nobody listed.
	if g.trustsPeer(r) {
		return true
	}
	if g.bound {
		return false
	}
	// The rule the SDK applies inside its own handler, reproduced so that the
	// two exceptions above reach it as well: a listener reached over loopback
	// is a local server, and a local server is what a rebinding attack aims
	// at. Reached on any other address it is not, and a wildcard bind then
	// keeps answering whatever host a proxy in front of it forwards.
	return !arrivedOnLoopback(r) || isLoopbackHost(hostOnly(r.Host))
}

// trustsPeer reports whether the request arrived from an address in
// --trusted-proxies.
func (g hostGuard) trustsPeer(r *http.Request) bool {
	if g.proxies.empty() {
		return false
	}
	addr, err := netip.ParseAddr(remoteHost(r))
	if err != nil {
		return false
	}
	return g.proxies.contains(addr)
}

// hostOnly strips the port from a Host header value, leaving a bare host or
// bracket-free IPv6 literal. A value with no port is returned as it came.
func hostOnly(value string) string {
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	return value
}

// publicURLHost is the host --public-url advertises, without its port. An
// unparseable value yields "": the URL is validated at startup, so this is the
// unreachable half of a check that stays rather than trusting that.
func publicURLHost(publicURL string) string {
	if publicURL == "" {
		return ""
	}
	u, err := url.Parse(publicURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// isLoopbackHost reports whether a Host names the local machine. It mirrors
// the SDK's own test, which accepts every loopback address rather than the
// 127.0.0.1 spelling alone.
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

// arrivedOnLoopback reports whether the connection was accepted on a loopback
// address. A request whose context carries no local address (a handler driven
// directly rather than through net/http) is not treated as local, which is the
// SDK's answer for the same case.
func arrivedOnLoopback(r *http.Request) bool {
	local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok || local == nil {
		return false
	}
	return isLoopbackHost(hostOnly(local.String()))
}

// allowedHosts computes the set of valid Host header values based on the
// listen address. Returns nil when binding to all interfaces (0.0.0.0/::),
// which leaves the deployment naming no host of its own.
//
// A unix socket gets nil as well, and explicitly. The check exists against
// DNS rebinding, which needs a browser to reach the listener by name, and no
// name resolves to a file on disk; the Host a socket client sends is whatever
// its HTTP library needs to build a request, "unix" as often as not. Deriving
// a set from the path used to work on Linux only by accident, because
// SplitHostPort found no colon there; on Windows it found the drive letter's,
// and a server on C:\...\mcp.sock served nothing but a client naming host "C".
func allowedHosts(addr string) map[string]bool {
	if isUnixSocketAddr(addr) {
		return nil
	}
	host, _, _ := net.SplitHostPort(addr)
	if host == "" || host == "0.0.0.0" || host == "::" {
		return nil
	}
	return map[string]bool{
		host:        true,
		"localhost": true,
		"127.0.0.1": true,
		"::1":       true,
	}
}

// hostValidationMiddleware refuses requests whose Host header names a host
// this deployment does not serve, mitigating DNS rebinding attacks against a
// server a browser can reach.
func hostValidationMiddleware(guard hostGuard, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if guard.permits(r) {
			next.ServeHTTP(w, r)
			return
		}
		slog.WarnContext(r.Context(), "request blocked: invalid Host header", //#nosec G706 -- slog structured args are not interpolated
			"host", loggedHeaderPrefix(r.Host), "host_len", len(r.Host))
		// JSON-RPC rather than http.Error's plain text, for the same
		// reason the cross-origin refusal is: an unparseable 4xx body
		// reads to a Streamable HTTP client as a pre-negotiation server.
		(&gateFailure{
			status: http.StatusForbidden,
			code:   errCodeForbidden,
			message: "Request refused: the Host header names a host this deployment does not serve. " +
				"Behind a reverse proxy, pass --public-url with the origin clients use.",
		}).write(w, r)
	})
}
