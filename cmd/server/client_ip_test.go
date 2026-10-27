// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// client_ip_test.go covers which address a request is charged to: the peer
// unless a trusted proxy vouched for another, and then the first hop from the
// right that is not itself a trusted proxy; and the keys the authentication
// budgets read off that address (issue 954).
package main

import (
	"context"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// unixSocketRequest builds a request accepted on a unix socket listener, as
// net/http hands one to a handler: the listener's address on the context and
// the peer Linux reports for an unnamed client socket, with header set.
func unixSocketRequest(t *testing.T, header http.Header) *http.Request {
	t.Helper()
	ctx := context.WithValue(t.Context(), http.LocalAddrContextKey, &net.UnixAddr{Name: "/run/gitlab-mcp/m.sock", Net: "unix"})
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/mcp", http.NoBody)
	r.RemoteAddr = "@"
	maps.Copy(r.Header, header)
	return r
}

// TestBudgetKey_KeysAnIPv4AddressWholeAndAnIPv6AddressByItsSlash64 pins the
// key every authentication budget is charged to (issue 954). An IPv6 caller
// holds at least a /64 and rotates the low bits for nothing, so keying on the
// whole address gave it a fresh budget per request; an IPv4 address is still
// the unit one caller holds. A mapped address is keyed as the IPv4 address it
// names, a zone says nothing about who is calling, and a value that is not an
// address, the shared unix key among them, is its own key.
func TestBudgetKey_KeysAnIPv4AddressWholeAndAnIPv6AddressByItsSlash64(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		address string
		want    string
	}{
		{"an IPv4 address is its own key", "203.0.113.7", "203.0.113.7"},
		{"an IPv6 address is keyed by its /64", "2001:db8:1:2:a:b:c:d", "2001:db8:1:2::/64"},
		{"another address of the same /64 shares that key", "2001:db8:1:2:ffff:ffff:ffff:ffff", "2001:db8:1:2::/64"},
		{"the next /64 is another key", "2001:db8:1:3::1", "2001:db8:1:3::/64"},
		{"an IPv4-mapped address is keyed as the IPv4 address it names", "::ffff:198.51.100.7", "198.51.100.7"},
		{"a zone is dropped with the low bits", "fe80::1%eth0", "fe80::/64"},
		{"the loopback is keyed by its /64 like any IPv6 address", "::1", "::/64"},
		{"the unix-socket peer keeps its one shared key", unixPeer, unixPeer},
		{"an empty address stays empty", "", ""},
		{"anything else is charged as reported", "@", "@"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := budgetKey(tc.address); got != tc.want {
				t.Errorf("budgetKey(%q) = %q, want %q", tc.address, got, tc.want)
			}
		})
	}
}

// TestNetworkKey_IsTheSlash48OfAnIPv6AddressAndNothingElse pins the key of the
// aggregate that bounds rotation across /64s (AUB-006): the /48 an IPv6
// address lies in. An IPv4 address, a mapped one included, and a peer with no
// address have none, and the empty key is what the aggregate counts nothing
// for.
func TestNetworkKey_IsTheSlash48OfAnIPv6AddressAndNothingElse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		address string
		want    string
	}{
		{"an IPv6 address is keyed by its /48", "2001:db8:1:2::1", "2001:db8:1::/48"},
		{"another /64 of the same /48 shares that key", "2001:db8:1:ffff:1:2:3:4", "2001:db8:1::/48"},
		{"the next /48 is another key", "2001:db8:2::1", "2001:db8:2::/48"},
		{"an IPv4 address has none", "203.0.113.7", ""},
		{"an IPv4-mapped address has none either", "::ffff:198.51.100.7", ""},
		{"the unix-socket peer has none", unixPeer, ""},
		{"an empty address has none", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := networkKey(tc.address); got != tc.want {
				t.Errorf("networkKey(%q) = %q, want %q", tc.address, got, tc.want)
			}
		})
	}
}

// TestRemoteHost_AUnixSocketConnection_IsTheSharedKeyUnix pins the one key
// every caller on a unix socket shares when nothing vouches for another: the
// peer of such a connection has no address, whatever RemoteAddr spells, so it
// is named once rather than charged as "@" on Linux and as an empty string or
// a drive letter elsewhere. The transport source is that key too, and a header
// from it is ignored unless unix is a trusted proxy.
func TestRemoteHost_AUnixSocketConnection_IsTheSharedKeyUnix(t *testing.T) {
	t.Parallel()
	for _, remote := range []string{"@", "", `C:\run\m.sock`} {
		t.Run(strconv.Quote(remote), func(t *testing.T) {
			t.Parallel()
			r := unixSocketRequest(t, http.Header{"X-Real-Ip": {"203.0.113.42"}})
			r.RemoteAddr = remote
			if got := remoteHost(r); got != unixPeer {
				t.Errorf("remoteHost() = %q, want %q", got, unixPeer)
			}
			if got := transportSource(r); got != unixPeer {
				t.Errorf("transportSource() = %q, want %q", got, unixPeer)
			}
			if got := clientIP(r, "X-Real-IP", loopbackProxies(t)); got != unixPeer {
				t.Errorf("clientIP() = %q, want %q: unix is not among the trusted proxies, so the header is the caller's own text", got, unixPeer)
			}
		})
	}
}

// TestClientIP_TrustedUnixSocket_BelievesTheHeader covers the proxy on the
// same host that reaches this server over its socket (issue 954): with unix in
// --trusted-proxies the header is believed from the socket's peer and read by
// the rule a TCP proxy's is, so each caller behind the proxy keeps a budget of
// its own. The entry trusts the socket and no address, so a TCP peer is still
// charged as itself.
func TestClientIP_TrustedUnixSocket_BelievesTheHeader(t *testing.T) {
	t.Parallel()
	proxies, err := parseTrustedProxies([]string{"unix", "10.0.0.0/8"})
	if err != nil {
		t.Fatalf("parseTrustedProxies: %v", err)
	}
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{"a single forwarded address", "2001:db8:5::9", "2001:db8:5::9"},
		{"the first untrusted hop from the right", "198.51.100.1, 203.0.113.9, 10.0.0.7", "203.0.113.9"},
		{"a hop that is not an address charges the socket", "client.example.com", unixPeer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := unixSocketRequest(t, http.Header{"X-Forwarded-For": {tc.value}})
			if got := clientIP(r, "X-Forwarded-For", proxies); got != tc.want {
				t.Errorf("clientIP(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}

	tcp := &http.Request{RemoteAddr: "127.0.0.1:12345", Header: http.Header{"X-Forwarded-For": {"203.0.113.9"}}}
	if got := clientIP(tcp, "X-Forwarded-For", trustedProxiesOf([]string{"unix"})); got != "127.0.0.1" {
		t.Errorf("clientIP() over TCP with only unix trusted = %q, want the peer 127.0.0.1", got)
	}
}

// TestTrustedProxies_TrustsPeer_ReadsTheConnection is the one question both the
// address a request is charged to and the Host it may forward ask of the
// connection: did it come from a proxy the operator listed? A TCP peer is
// looked up among the addresses and ranges, a unix-socket connection is
// trusted exactly when unix is listed, and a peer that is neither is trusted
// by nothing.
func TestTrustedProxies_TrustsPeer_ReadsTheConnection(t *testing.T) {
	t.Parallel()
	tcp := func(remote string) *http.Request { return &http.Request{RemoteAddr: remote, Header: http.Header{}} }
	for _, tc := range []struct {
		name    string
		proxies []string
		request *http.Request
		want    bool
	}{
		{"a listed TCP peer", []string{"127.0.0.1"}, tcp("127.0.0.1:1"), true},
		{"a TCP peer inside a listed range", []string{"10.0.0.0/8"}, tcp("10.9.9.9:1"), true},
		{"a TCP peer nobody listed", []string{"10.0.0.0/8"}, tcp("192.0.2.1:1"), false},
		{"a peer that is not an address", []string{"127.0.0.1"}, tcp("@"), false},
		{"a TCP peer when only unix is listed", []string{"unix"}, tcp("127.0.0.1:1"), false},
		{"a unix-socket connection when unix is listed", []string{"unix"}, unixSocketRequest(t, nil), true},
		{"a unix-socket connection when only addresses are listed", []string{"127.0.0.1", "::1"}, unixSocketRequest(t, nil), false},
		{"nobody listed", nil, unixSocketRequest(t, nil), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := trustedProxiesOf(tc.proxies).trustsPeer(tc.request); got != tc.want {
				t.Errorf("trustsPeer() = %v, want %v", got, tc.want)
			}
		})
	}
}

// mustAddr parses a literal the test wrote.
func mustAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	addr, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatalf("netip.ParseAddr(%q): %v", s, err)
	}
	return addr
}

// loopbackProxies trusts the loopback addresses only.
func loopbackProxies(t *testing.T) trustedProxies {
	t.Helper()
	proxies, err := parseTrustedProxies([]string{"127.0.0.1", "::1"})
	if err != nil {
		t.Fatalf("parseTrustedProxies: %v", err)
	}
	return proxies
}

// TestClientIP_NoTrustedHeader_ChargesThePeer verifies the peer is charged
// when no header is trusted, whatever the request carries.
func TestClientIP_NoTrustedHeader_ChargesThePeer(t *testing.T) {
	t.Parallel()
	r := &http.Request{
		RemoteAddr: "203.0.113.1:12345",
		Header:     http.Header{"X-Real-Ip": {"198.51.100.7"}},
	}
	if got := clientIP(r, "", loopbackProxies(t)); got != "203.0.113.1" {
		t.Errorf("clientIP() = %q, want the peer 203.0.113.1", got)
	}
}

// TestClientIP_NoTrustedHeader_ReadsNoHeaderEvenFromATrustedPeer is the case
// the test above cannot reach, since its peer is not trusted: a deployment
// that named no header believes none, even on a connection from a proxy it
// lists. The request carries an entry under the empty name, which is exactly
// what looking up the unnamed header would find.
func TestClientIP_NoTrustedHeader_ReadsNoHeaderEvenFromATrustedPeer(t *testing.T) {
	t.Parallel()
	r := &http.Request{
		RemoteAddr: "127.0.0.1:12345",
		Header:     http.Header{"": {"198.51.100.7"}},
	}
	if got := clientIP(r, "", loopbackProxies(t)); got != "127.0.0.1" {
		t.Errorf("clientIP() = %q, want the peer 127.0.0.1: no header was named, so none is believed", got)
	}
}

// TestClientIP_UntrustedPeer_IgnoresTheHeader verifies that a header from a
// peer that is not a trusted proxy is ignored, which is what stops a caller
// reaching the listener directly from choosing the address their failures are
// charged to.
func TestClientIP_UntrustedPeer_IgnoresTheHeader(t *testing.T) {
	t.Parallel()
	r := &http.Request{
		RemoteAddr: "203.0.113.1:12345",
		Header:     http.Header{"X-Real-Ip": {"198.51.100.7"}},
	}
	if got := clientIP(r, "X-Real-IP", loopbackProxies(t)); got != "203.0.113.1" {
		t.Errorf("clientIP() = %q, want the peer 203.0.113.1: the header came from nobody the operator trusts", got)
	}
}

// TestClientIP_NobodyTrusted_IgnoresTheHeader verifies an empty trusted set
// believes no header even when one is named.
func TestClientIP_NobodyTrusted_IgnoresTheHeader(t *testing.T) {
	t.Parallel()
	r := &http.Request{
		RemoteAddr: "127.0.0.1:12345",
		Header:     http.Header{"X-Real-Ip": {"198.51.100.7"}},
	}
	if got := clientIP(r, "X-Real-IP", trustedProxies{}); got != "127.0.0.1" {
		t.Errorf("clientIP() = %q, want the peer 127.0.0.1", got)
	}
}

// TestClientIP_TrustedPeer_BelievesASingleValueHeader verifies a single-value
// header such as X-Real-IP from a trusted proxy names the client.
func TestClientIP_TrustedPeer_BelievesASingleValueHeader(t *testing.T) {
	t.Parallel()
	r := &http.Request{
		RemoteAddr: "127.0.0.1:12345",
		Header:     http.Header{"X-Real-Ip": {"203.0.113.42"}},
	}
	if got := clientIP(r, "X-Real-IP", loopbackProxies(t)); got != "203.0.113.42" {
		t.Errorf("clientIP() = %q, want 203.0.113.42", got)
	}
}

// TestClientIP_HeaderSplitAcrossFieldLines_IsReadAsOneList covers a header the
// client sent and a proxy then appended to as a second field line, which is
// what HAProxy's option forwardfor does. Reading only the first line handed
// the walk the client's own text, so the address a request was charged to
// was the one the client chose. Joined, the lines are the list the proxy
// meant, and the walk from the right reaches the proxy's entry first.
func TestClientIP_HeaderSplitAcrossFieldLines_IsReadAsOneList(t *testing.T) {
	t.Parallel()
	proxies, err := parseTrustedProxies([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("parseTrustedProxies: %v", err)
	}
	cases := []struct {
		name   string
		header string
		lines  []string
		want   string
	}{
		{
			name: "X-Forwarded-For with the client's line first", header: "X-Forwarded-For",
			lines: []string{"198.51.100.77", "203.0.113.9"}, want: "203.0.113.9",
		},
		{
			name: "X-Forwarded-For with a list in each line", header: "X-Forwarded-For",
			lines: []string{"198.51.100.77, 198.51.100.78", "203.0.113.9, 10.0.0.6"}, want: "203.0.113.9",
		},
		{
			name: "a single-value header the client sent too", header: "X-Real-IP",
			lines: []string{"198.51.100.77", "203.0.113.9"}, want: "203.0.113.9",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := &http.Request{RemoteAddr: "10.0.0.5:12345", Header: http.Header{}}
			for _, line := range tc.lines {
				r.Header.Add(tc.header, line)
			}
			if got := clientIP(r, tc.header, proxies); got != tc.want {
				t.Errorf("clientIP() = %q, want %q: the proxy's line is the one it vouches for", got, tc.want)
			}
		})
	}
}

// TestClientIP_XForwardedFor_TakesTheFirstUntrustedHopFromTheRight covers the
// shapes a forwarded chain takes, including the one that motivated the list:
// two proxies in front, where the rightmost entry is the inner proxy and the
// client is the hop before it.
func TestClientIP_XForwardedFor_TakesTheFirstUntrustedHopFromTheRight(t *testing.T) {
	t.Parallel()
	proxies, err := parseTrustedProxies([]string{"127.0.0.1", "10.0.0.0/8", "2001:db8::/32"})
	if err != nil {
		t.Fatalf("parseTrustedProxies: %v", err)
	}
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{name: "one proxy appended the client", value: "203.0.113.5", want: "203.0.113.5"},
		{name: "a forged leftmost entry is skipped", value: "1.2.3.4, 203.0.113.55", want: "203.0.113.55"},
		{name: "the inner proxy is skipped and the client before it taken", value: "1.2.3.4, 203.0.113.9, 10.0.0.77", want: "203.0.113.9"},
		{name: "two trusted hops behind the client", value: "203.0.113.9, 10.0.0.77, 10.0.0.78", want: "203.0.113.9"},
		{name: "every hop trusted charges the leftmost", value: "10.0.0.5, 10.0.0.77", want: "10.0.0.5"},
		{name: "trailing comma and spaces are ignored", value: "203.0.113.6, ", want: "203.0.113.6"},
		{name: "an IPv6 client behind an IPv6 proxy", value: "2001:db8:1::5, 2001:db8::1", want: "2001:db8:1::5"},
		{name: "a bracketed IPv6 hop with a port", value: "[2001:db8:1::6]:443, 10.0.0.77", want: "2001:db8:1::6"},
		{name: "a bracketed IPv6 hop without a port", value: "[2001:db8:1::7], 10.0.0.77", want: "2001:db8:1::7"},
		{name: "an IPv4 hop with a port", value: "203.0.113.7:51000, 10.0.0.77", want: "203.0.113.7"},
		{name: "an IPv4-mapped hop compares as IPv4", value: "::ffff:203.0.113.8, ::ffff:10.0.0.77", want: "203.0.113.8"},
		{name: "an IPv4-mapped hop with a port is charged as IPv4", value: "[::ffff:203.0.113.9]:4711, 10.0.0.77", want: "203.0.113.9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := &http.Request{
				RemoteAddr: "127.0.0.1:12345",
				Header:     http.Header{"X-Forwarded-For": {tc.value}},
			}
			if got := clientIP(r, "X-Forwarded-For", proxies); got != tc.want {
				t.Errorf("clientIP(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestClientIP_UnparseableHop_ChargesThePeer verifies that a header a trusted
// proxy filled with something other than addresses is not used as a key: the
// key would then be text nobody vouched for, which is the situation the list
// exists to prevent.
func TestClientIP_UnparseableHop_ChargesThePeer(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, value string }{
		{name: "a hostname", value: "client.example.com"},
		{name: "garbage in the client position", value: "not-an-address, 10.0.0.77"},
		{name: "empty", value: ""},
	}
	proxies, err := parseTrustedProxies([]string{"127.0.0.1", "10.0.0.0/8"})
	if err != nil {
		t.Fatalf("parseTrustedProxies: %v", err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := &http.Request{
				RemoteAddr: "127.0.0.1:12345",
				Header:     http.Header{"X-Forwarded-For": {tc.value}},
			}
			if got := clientIP(r, "X-Forwarded-For", proxies); got != "127.0.0.1" {
				t.Errorf("clientIP(%q) = %q, want the peer 127.0.0.1", tc.value, got)
			}
		})
	}
}

// TestClientIP_MappedPeer_IsRecognised verifies a dual-stack listener's
// IPv4-mapped peer matches an IPv4 entry in the list.
func TestClientIP_MappedPeer_IsRecognised(t *testing.T) {
	t.Parallel()
	r := &http.Request{
		RemoteAddr: "[::ffff:127.0.0.1]:12345",
		Header:     http.Header{"X-Real-Ip": {"203.0.113.42"}},
	}
	if got := clientIP(r, "X-Real-IP", loopbackProxies(t)); got != "203.0.113.42" {
		t.Errorf("clientIP() = %q, want 203.0.113.42: the mapped loopback peer is the trusted loopback", got)
	}
}

// TestClientIP_UnixSocketPeer_HasNoAddress verifies a peer without a port, on a
// connection that did not say it arrived on a unix socket listener (a handler
// driven directly rather than through net/http), is charged as it is rather
// than crashing the split.
func TestClientIP_UnixSocketPeer_HasNoAddress(t *testing.T) {
	t.Parallel()
	r := &http.Request{RemoteAddr: "@", Header: http.Header{}}
	if got := clientIP(r, "X-Real-IP", loopbackProxies(t)); got != "@" {
		t.Errorf("clientIP() = %q, want the peer as reported", got)
	}
}

// TestParseTrustedProxies_ReadsAddressesAndRanges covers the accepted spellings
// and the refused ones: whitespace and empty entries are ignored, a plain
// address is the range holding only it, unix names the peer of a unix-socket
// connection and no address, and anything else fails naming the entry, so a
// typo refuses startup rather than trusting nobody in silence.
func TestParseTrustedProxies_ReadsAddressesAndRanges(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		entries  []string
		wantErr  string
		wantIn   []string
		wantOut  []string
		wantUnix bool
	}{
		{name: "addresses and ranges", entries: []string{" 127.0.0.1 ", "10.0.0.0/8", "", "fd00::/8"}, wantIn: []string{"127.0.0.1", "10.1.2.3", "fd00::1"}, wantOut: []string{"127.0.0.2", "192.0.2.1", "2001:db8::1"}},
		{name: "a range not on its boundary still covers the block", entries: []string{"192.0.2.77/24"}, wantIn: []string{"192.0.2.1"}, wantOut: []string{"192.0.3.1"}},
		{name: "a plain address trusts that address alone, on either family", entries: []string{"192.0.2.10", "2001:db8::10"}, wantIn: []string{"192.0.2.10", "2001:db8::10"}, wantOut: []string{"192.0.2.11", "2001:db8::11"}},
		{name: "an IPv4-mapped address is trusted as the IPv4 peer it names", entries: []string{"::ffff:192.0.2.20"}, wantIn: []string{"192.0.2.20", "::ffff:192.0.2.20"}, wantOut: []string{"192.0.2.21"}},
		{name: "nothing", entries: nil, wantOut: []string{"127.0.0.1"}},
		{name: "unix trusts the socket's peer and no address", entries: []string{" unix "}, wantOut: []string{"127.0.0.1", "::1"}, wantUnix: true},
		{name: "unix beside a range", entries: []string{"unix", "10.0.0.0/8"}, wantIn: []string{"10.1.2.3"}, wantOut: []string{"127.0.0.1"}, wantUnix: true},
		{name: "a socket path is not the unix entry", entries: []string{"/run/gitlab-mcp/m.sock"}, wantErr: "/run/gitlab-mcp/m.sock"},
		{name: "unix is spelled in lower case", entries: []string{"UNIX"}, wantErr: "UNIX"},
		{name: "a hostname", entries: []string{"proxy.example.com"}, wantErr: "proxy.example.com"},
		{name: "a malformed range", entries: []string{"10.0.0.0/33"}, wantErr: "10.0.0.0/33"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			proxies, err := parseTrustedProxies(tc.entries)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("parseTrustedProxies(%v) error = %v, want it to name %q", tc.entries, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTrustedProxies(%v) error = %v", tc.entries, err)
			}
			assertTrustedSet(t, proxies, tc.wantIn, tc.wantOut)
			if proxies.unix != tc.wantUnix {
				t.Errorf("unix = %v, want %v", proxies.unix, tc.wantUnix)
			}
		})
	}
}

// assertTrustedSet checks which addresses a parsed list does and does not
// trust.
func assertTrustedSet(t *testing.T, proxies trustedProxies, wantIn, wantOut []string) {
	t.Helper()
	for _, in := range wantIn {
		if !proxies.contains(mustAddr(t, in)) {
			t.Errorf("%s is not trusted, want it in the set", in)
		}
	}
	for _, out := range wantOut {
		if proxies.contains(mustAddr(t, out)) {
			t.Errorf("%s is trusted, want it outside the set", out)
		}
	}
}

// TestValidateTrustedProxyListener_RefusesThePairingsThatTrustNobody pins the
// two pairings of the proxy flags with --http-addr that issue 954 refuses at
// startup, since each trusts nobody while reading as if it trusted someone: a
// unix entry on a TCP listener, whose connections never come from a socket
// peer, and a socket listener whose list names no unix, whose peer no address
// or range matches, so its header is never believed and every caller shares
// one budget. Every other pairing starts. That each refusal stops main is
// TestMain_HTTPFlags_EachRefusalNamesTheFlagThatCarriedTheValue's.
func TestValidateTrustedProxyListener_RefusesThePairingsThatTrustNobody(t *testing.T) {
	t.Parallel()
	const socket = "/run/gitlab-mcp/m.sock"
	for _, tc := range []struct {
		name    string
		addr    string
		proxies []string
		want    string
	}{
		{name: "unix on a TCP listener", addr: ":8080", proxies: []string{"unix"}, want: `--trusted-proxies names unix and --http-addr ":8080" is not a unix socket`},
		{name: "unix beside a range on a TCP listener", addr: "127.0.0.1:8080", proxies: []string{"10.0.0.0/8", "unix"}, want: "--trusted-proxies names unix"},
		{name: "addresses on a socket listener", addr: socket, proxies: []string{"127.0.0.1", "::1"}, want: `--trusted-proxies does not name unix and --http-addr "` + socket + `" is a unix socket`},
		{name: "addresses on a TCP listener", addr: ":8080", proxies: []string{"127.0.0.1", "10.0.0.0/8"}},
		{name: "unix on a socket listener", addr: socket, proxies: []string{"unix"}},
		{name: "unix beside a range on a socket listener", addr: socket, proxies: []string{"unix", "10.0.0.0/8"}},
		{name: "no proxy on a socket listener", addr: socket},
		{name: "no proxy on a TCP listener", addr: ":8080"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{TrustedProxies: tc.proxies}
			if len(tc.proxies) > 0 {
				cfg.TrustedProxyHeader = "X-Forwarded-For"
			}
			err := validateTrustedProxyListener(tc.addr, cfg)
			if tc.want == "" {
				if err != nil {
					t.Errorf("validateTrustedProxyListener(%q, %v) = %v, want it to start", tc.addr, tc.proxies, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("validateTrustedProxyListener(%q, %v) = %v, want it to carry %q", tc.addr, tc.proxies, err, tc.want)
			}
		})
	}
}

// TestCommaSeparated_TrimsAndDropsEmptyEntries covers the flag's list syntax.
func TestCommaSeparated_TrimsAndDropsEmptyEntries(t *testing.T) {
	t.Parallel()
	got := commaSeparated(" 127.0.0.1, 10.0.0.0/8 ,, ")
	if strings.Join(got, "|") != "127.0.0.1|10.0.0.0/8" {
		t.Errorf("commaSeparated = %v, want the two entries", got)
	}
	if empty := commaSeparated(""); empty != nil {
		t.Errorf("commaSeparated(\"\") = %v, want nil", empty)
	}
}

// TestTrustedProxiesOf_UnparseableList_TrustsNobody verifies the startup
// helper's fallback: a list validation refused earlier cannot reach it, and if
// it did, the safe answer is a set that believes no header.
func TestTrustedProxiesOf_UnparseableList_TrustsNobody(t *testing.T) {
	t.Parallel()
	if got := trustedProxiesOf([]string{"unix", "not an address"}); got.unix || len(got.prefixes) != 0 {
		t.Errorf("an unparseable list produced the trusted set %+v, want one that trusts nobody", got)
	}
	if got := trustedProxiesOf([]string{"127.0.0.1"}); len(got.prefixes) != 1 {
		t.Errorf("a valid list produced the trusted set %+v, want the one address", got)
	}
}

// TestClientIP_HeaderWithNoHops_ChargesThePeer covers a header that is present
// and carries only separators and whitespace: there is no hop to believe, so
// the trusted peer itself is charged, exactly as if the header were absent.
func TestClientIP_HeaderWithNoHops_ChargesThePeer(t *testing.T) {
	t.Parallel()
	proxies, err := parseTrustedProxies([]string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("parseTrustedProxies: %v", err)
	}
	for _, value := range []string{" , ", ",", " ,, "} {
		t.Run(strconv.Quote(value), func(t *testing.T) {
			t.Parallel()
			r := &http.Request{
				RemoteAddr: "127.0.0.1:12345",
				Header:     http.Header{"X-Forwarded-For": {value}},
			}
			if got := clientIP(r, "X-Forwarded-For", proxies); got != "127.0.0.1" {
				t.Errorf("clientIP(%q) = %q, want the peer 127.0.0.1", value, got)
			}
		})
	}
}
