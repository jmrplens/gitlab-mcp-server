// clientip.go decides which address a request is charged to.
//
// The authentication-failure budget is per address, and behind a reverse proxy
// every connection arrives from the proxy, so --trusted-proxy-header names the
// header the proxy fills with the address it heard from. A header is believed
// only from a proxy the operator listed in --trusted-proxies: read from any
// other peer it is caller-supplied text, and a caller who can choose the
// address their failures are charged to can choose somebody else's, or a fresh
// one per request. The two flags are therefore required together.
package main

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

// trustedProxies is the set of peers whose client-address header is believed.
type trustedProxies struct {
	prefixes []netip.Prefix
}

// parseTrustedProxies reads addresses and CIDR ranges, comma-separated, with
// whitespace and empty entries ignored. A plain address becomes the range that
// holds only it.
func parseTrustedProxies(entries []string) (trustedProxies, error) {
	var t trustedProxies
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			t.prefixes = append(t.prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return trustedProxies{}, fmt.Errorf("%q is neither an address nor a CIDR range", entry)
		}
		addr = addr.Unmap()
		t.prefixes = append(t.prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return t, nil
}

// empty reports whether nobody is trusted.
func (t trustedProxies) empty() bool { return len(t.prefixes) == 0 }

// contains reports whether addr is one of the trusted proxies. IPv4 addresses
// that arrived mapped into IPv6, which a dual-stack listener produces, are
// compared as IPv4.
func (t trustedProxies) contains(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, prefix := range t.prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
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

// clientIP returns the address a request is charged to.
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
func clientIP(r *http.Request, trustedHeader string, proxies trustedProxies) string {
	peer := remoteHost(r)
	if trustedHeader == "" || proxies.empty() {
		return peer
	}
	peerAddr, err := netip.ParseAddr(peer)
	if err != nil || !proxies.contains(peerAddr) {
		return peer
	}
	value := r.Header.Get(trustedHeader)
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

// remoteHost is the connection's peer address without its port. A listener
// on a unix socket reports no address, and every caller then shares one.
func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
