// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// client_ip.go decides which address a request is charged to, and the key the
// authentication budgets read off it.
//
// The authentication-failure budget is per address, and behind a reverse proxy
// every connection arrives from the proxy, so --trusted-proxy-header names the
// header the proxy fills with the address it heard from. A header is believed
// only from a proxy the operator listed in --trusted-proxies: read from any
// other peer it is caller-supplied text, and a caller who can choose the
// address their failures are charged to can choose somebody else's, or a fresh
// one per request. The two flags are therefore required together.
//
// An address is not yet a key (issue 954). An IPv6 subnet is a /64 (RFC 4291),
// and a caller on one rotates the bits below it for nothing, so keying a
// budget on the whole IPv6 address gave such a caller a fresh budget on every
// request; [budgetKey] keys it on the /64, and the /48 aggregate counts the
// /64s of one /48 ([networkKey]). A unix socket's peer has no address at all,
// so every caller on one is the one key [unixPeer], unless unix is listed as a
// trusted proxy and the proxy on the socket names each caller.

package main

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// unixPeer is the peer of every connection accepted on a unix socket: the key
// its budgets are charged to when nothing vouches for another, and the entry
// of --trusted-proxies that trusts that peer.
//
// It is one name because such a peer has no address to tell callers apart by.
// RemoteAddr spells it "@" on Linux, an empty string on other systems and the
// socket's own path on Windows, and none of those is a caller: keying on the
// spelling the platform happens to report would give one deployment two
// behaviors on two hosts.
const unixPeer = "unix"

// The prefix lengths an IPv6 address is keyed on (issue 954). Both are fixed
// and documented rather than configurable, which the issue decided: the /64
// because it is the IPv6 subnet (RFC 4291), so the least a caller on IPv6
// holds, and the /48 because it is the larger assignment a provider makes to
// one end site, which RFC 6177 leaves to its policy, so a caller holding one
// holds 65536 /64s. Callers that share one /64 share its budgets, as callers
// behind one IPv4 NAT share an address's: the hosts of one IPv6 subnet, which
// on private IPv4 each had an address of their own, the customers a provider
// numbers from one shared /64, and every link-local caller, since the zone is
// dropped and fe80::/64 is the same prefix on every interface.
//
// A stateful NAT64 gateway changes nothing here: the server sees the
// gateway's IPv4 address, keyed whole as it always was. A stateless
// translator in front of an IPv6-only server (SIIT-DC, RFC 7755) does: it
// writes each IPv4 client into its RFC 6052 prefix, so under 64:ff9b::/96, or
// any prefix of /64 or longer, every IPv4 client it carries shares one /64,
// and under any prefix of /48 or longer one /48, where on IPv4 each had a key
// of its own.
const (
	clientPrefixBits  = 64
	networkPrefixBits = 48
)

// trustedProxies is the set of peers whose client-address header is believed.
type trustedProxies struct {
	prefixes []netip.Prefix
	// unix says the peer of a unix-socket connection is trusted, which the
	// entry unix asks for. It trusts no address: a proxy on the same host
	// that reaches this server over TCP is listed by its address, as any
	// other is.
	unix bool
}

// parseTrustedProxies reads addresses, CIDR ranges and the entry unix,
// comma-separated, with whitespace and empty entries ignored. A plain address
// becomes the range that holds only it.
func parseTrustedProxies(entries []string) (trustedProxies, error) {
	var t trustedProxies
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if entry == unixPeer {
			t.unix = true
			continue
		}
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			t.prefixes = append(t.prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return trustedProxies{}, fmt.Errorf("%q is neither an address, a CIDR range nor unix", entry)
		}
		addr = addr.Unmap()
		t.prefixes = append(t.prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return t, nil
}

// contains reports whether addr is one of the trusted proxies. IPv4 addresses
// that arrived mapped into IPv6, which a dual-stack listener produces, are
// compared as IPv4. It is the question asked of a forwarded hop; the peer of
// the connection itself is asked through [trustedProxies.trustsPeer].
func (t trustedProxies) contains(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, prefix := range t.prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// trustsPeer reports whether the request's connection came from a trusted
// proxy: a connection accepted on a unix socket when unix is listed, and any
// other when its peer is an address the list holds. It is the one question
// both the address a request is charged to ([clientIP]) and the Host it may
// forward ([hostGuard.permits]) ask of the connection, so the two cannot read
// the list differently.
func (t trustedProxies) trustsPeer(r *http.Request) bool {
	if arrivedOnUnixSocket(r) {
		return t.unix
	}
	addr, err := netip.ParseAddr(remoteHost(r))
	return err == nil && t.contains(addr)
}

// validateTrustedProxyListener holds the proxy list to the listener it is
// read on, refusing the two pairings that trust nobody while reading as if
// they trusted someone (issue 954).
//
// A unix entry on a TCP listener names a peer no connection to it can have,
// so the proxy the operator meant to trust is not trusted, and its header is
// ignored. A socket listener whose list names no unix is the reverse: every
// connection to it comes from the socket's peer, which no address or range
// matches, so --trusted-proxy-header is never believed there and every caller
// shares the one key unix. That second pairing started before this was
// written, with the header silently inert, and refusing it is the change of
// behavior the release notes name. A socket listener with no proxy flag at all
// still starts: its callers sharing one budget is what the flag help and the
// HTTP guide state.
//
// It runs after [validateTrustedProxyConfig], so the list is well formed and
// comes with a header.
func validateTrustedProxyListener(addr string, cfg *config.Config) error {
	proxies := trustedProxiesOf(cfg.TrustedProxies)
	socket := isUnixSocketAddr(addr)
	if proxies.unix && !socket {
		return fmt.Errorf("--trusted-proxies names unix and --http-addr %q is not a unix socket: no connection to it comes from a socket peer, "+
			"so that entry trusts nobody. Name the proxy by the address it connects from, or bind a socket path", addr)
	}
	if socket && !proxies.unix && len(proxies.prefixes) > 0 {
		return fmt.Errorf("--trusted-proxies does not name unix and --http-addr %q is a unix socket: every connection to it comes from the socket's peer, "+
			"which no address or range matches, so --trusted-proxy-header would never be believed and every caller would share one "+
			"authentication budget. Add unix to --trusted-proxies to believe the proxy on the socket, or drop both flags", addr)
	}
	return nil
}

// trustedProxiesOf parses the deployment's list. The list was validated at
// startup, so a parse failure here is unreachable and yields an empty set,
// which believes no header.
func trustedProxiesOf(entries []string) trustedProxies {
	t, err := parseTrustedProxies(entries)
	if err != nil {
		return trustedProxies{}
	}
	return t
}

// commaSeparated splits a flag value on commas, trimming each entry and
// dropping empty ones.
func commaSeparated(value string) []string {
	var out []string
	for entry := range strings.SplitSeq(value, ",") {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// clientIP returns the address a request is charged to, before [budgetKey]
// reads its key off it.
//
// Without a trusted header, or from a peer that is not a trusted proxy, it is
// the peer itself: the header is then caller-supplied text and is ignored
// rather than believed. From a trusted proxy the header is read from the
// right, since every proxy appends the peer it heard from: the first hop that
// is not itself a trusted proxy is the client, and if every hop is trusted the
// leftmost one is. A hop that does not parse as an address is a proxy sending
// something other than what the flag promised, and the request is charged to
// the peer rather than to text nobody vouched for.
//
// This is what makes a two-proxy deployment count per client: read rightmost
// without the list, the address of the inner proxy was the answer and every
// caller behind it shared one budget.
//
// Every field line of the header is read, joined in order, as RFC 9110
// section 5.3 makes a repeated field one comma-separated list. A proxy that
// appends a line of its own rather than extending the client's (HAProxy's
// option forwardfor) leaves the client's text in the first line and its own
// in the last; reading the first line alone charged the request to whatever
// the client wrote there. Joined, a single-value header the client also sent
// resolves to its last line, which is the proxy's.
func clientIP(r *http.Request, trustedHeader string, proxies trustedProxies) string {
	peer := remoteHost(r)
	if trustedHeader == "" || !proxies.trustsPeer(r) {
		return peer
	}
	value := strings.Join(r.Header.Values(trustedHeader), ",")
	if value == "" {
		return peer
	}
	candidate := ""
	for _, part := range slices.Backward(strings.Split(value, ",")) {
		hop := strings.TrimSpace(part)
		if hop == "" {
			continue
		}
		addr, ok := parseHop(hop)
		if !ok {
			return peer
		}
		candidate = addr.String()
		if !proxies.contains(addr) {
			return candidate
		}
	}
	if candidate != "" {
		return candidate
	}
	return peer
}

// budgetKey is the key an address's authentication budgets are charged to and
// consulted on (issue 954): an IPv4 address whole, an IPv6 address by the /64
// it lies in, and an IPv4 address that arrived mapped into IPv6 as the IPv4
// address it names. A zone is dropped with the low bits, since it names an
// interface of this host rather than the caller. Anything that is not an
// address, [unixPeer] among them, is its own key.
//
// The /64 is what one IPv6 caller holds, so every address of it is one
// caller's to the budgets, which is what stops a caller from rotating its low
// bits into a fresh budget per request. Its cost is the one an IPv4 NAT has
// always had, met wherever callers share a /64 ([clientPrefixBits] says who
// does): they share its budgets, while a credential the deployment already
// serves is exempt from a block (AUB-001).
func budgetKey(address string) string {
	addr, err := netip.ParseAddr(address)
	if err != nil {
		return address
	}
	addr = addr.Unmap()
	if addr.Is4() {
		return addr.String()
	}
	return netip.PrefixFrom(addr, clientPrefixBits).Masked().String()
}

// networkKey is the IPv6 /48 an address lies in, the key of the aggregate over
// its /64s (AUB-006), or empty for an address that has none: an IPv4 address,
// a mapped one included, and anything that is not an address. The aggregate
// counts nothing against the empty key.
func networkKey(address string) string {
	addr, err := netip.ParseAddr(address)
	if err != nil || addr.Unmap().Is4() {
		return ""
	}
	return netip.PrefixFrom(addr, networkPrefixBits).Masked().String()
}

// parseHop reads one entry of a forwarded-address header: an address, or an
// address with a port as some proxies write them, IPv6 bracketed either way.
func parseHop(hop string) (netip.Addr, bool) {
	if addr, err := netip.ParseAddr(strings.Trim(hop, "[]")); err == nil {
		return addr.Unmap(), true
	}
	if addrPort, err := netip.ParseAddrPort(hop); err == nil {
		return addrPort.Addr().Unmap(), true
	}
	return netip.Addr{}, false
}

// remoteHost is the connection's peer address without its port, or
// [unixPeer] for a connection accepted on a unix socket, whose peer has none.
func remoteHost(r *http.Request) string {
	if arrivedOnUnixSocket(r) {
		return unixPeer
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// arrivedOnUnixSocket reports whether net/http accepted the request's
// connection on a unix socket listener, which it records as the connection's
// local address. That is the one signal the platform does not spell
// differently: the peer's own spelling varies, as [unixPeer] says. A request
// whose context carries no local address (a handler driven directly rather
// than through net/http) did not arrive on one.
func arrivedOnUnixSocket(r *http.Request) bool {
	_, ok := r.Context().Value(http.LocalAddrContextKey).(*net.UnixAddr)
	return ok
}
