// client_ip_test.go covers which address a request is charged to: the peer
// unless a trusted proxy vouched for another, and then the first hop from the
// right that is not itself a trusted proxy.
package main

import (
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"testing"
)

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
		{name: "an IPv4 hop with a port", value: "203.0.113.7:51000, 10.0.0.77", want: "203.0.113.7"},
		{name: "an IPv4-mapped hop compares as IPv4", value: "::ffff:203.0.113.8, ::ffff:10.0.0.77", want: "203.0.113.8"},
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

// TestClientIP_UnixSocketPeer_HasNoAddress verifies a peer without a port,
// which a unix socket listener reports, is charged as it is rather than
// crashing the split.
func TestClientIP_UnixSocketPeer_HasNoAddress(t *testing.T) {
	t.Parallel()
	r := &http.Request{RemoteAddr: "@", Header: http.Header{}}
	if got := clientIP(r, "X-Real-IP", loopbackProxies(t)); got != "@" {
		t.Errorf("clientIP() = %q, want the peer as reported", got)
	}
}

// TestParseTrustedProxies_ReadsAddressesAndRanges covers the accepted spellings
// and the refused ones: whitespace and empty entries are ignored, a plain
// address is the range holding only it, and anything else fails naming the
// entry, so a typo refuses startup rather than trusting nobody in silence.
func TestParseTrustedProxies_ReadsAddressesAndRanges(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		entries []string
		wantErr string
		wantIn  []string
		wantOut []string
	}{
		{name: "addresses and ranges", entries: []string{" 127.0.0.1 ", "10.0.0.0/8", "", "fd00::/8"}, wantIn: []string{"127.0.0.1", "10.1.2.3", "fd00::1"}, wantOut: []string{"127.0.0.2", "192.0.2.1", "2001:db8::1"}},
		{name: "a range not on its boundary still covers the block", entries: []string{"192.0.2.77/24"}, wantIn: []string{"192.0.2.1"}, wantOut: []string{"192.0.3.1"}},
		{name: "nothing", entries: nil, wantOut: []string{"127.0.0.1"}},
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
	if !trustedProxiesOf([]string{"not an address"}).empty() {
		t.Error("an unparseable list produced a non-empty trusted set")
	}
	if trustedProxiesOf([]string{"127.0.0.1"}).empty() {
		t.Error("a valid list produced an empty trusted set")
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
