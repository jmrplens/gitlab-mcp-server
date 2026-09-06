package main

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
)

// loopbackRequest builds a request that arrived on a loopback listener, which
// is the shape a reverse proxy on the same machine produces and the one the
// rebinding rule is written against.
func loopbackRequest(t *testing.T, host string) *http.Request {
	t.Helper()
	return requestFrom(t, host, "127.0.0.1:54321", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080})
}

// requestFrom builds a request carrying the given Host, peer address and
// listener address, so a test can express every combination the guard reads.
func requestFrom(t *testing.T, host, peer string, local net.Addr) *http.Request {
	t.Helper()
	ctx := t.Context()
	if local != nil {
		ctx = context.WithValue(ctx, http.LocalAddrContextKey, local)
	}
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid/health", http.NoBody)
	req.Host = host
	req.RemoteAddr = peer
	return req
}

// servedBy runs the request through the middleware and reports the status and
// whether the wrapped handler was reached.
func servedBy(t *testing.T, guard hostGuard, req *http.Request) (int, bool) {
	t.Helper()
	reached := false
	handler := hostValidationMiddleware(guard, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr.Code, reached
}

// TestHostValidationMiddleware_BlockedHost verifies that the middleware
// returns 403 when the Host header does not match any allowed value.
func TestHostValidationMiddleware_BlockedHost(t *testing.T) {
	t.Parallel()

	status, reached := servedBy(t, newHostGuard("127.0.0.1:8080", "", trustedProxies{}),
		loopbackRequest(t, "evil.example.com"))

	if status != http.StatusForbidden {
		t.Errorf("expected 403 for blocked host, got %d", status)
	}
	if reached {
		t.Error("a Host nobody declared reached the handler")
	}
}

// TestHostValidationMiddleware_ARequestNamingNoHostIsServed covers the health
// check every polling balancer sends.
//
// The middleware exists against DNS rebinding, which needs a browser to
// resolve a name the attacker controls to the loopback address; the browser
// then puts that name in the Host header, and no browser omits it. A request
// carrying none is therefore outside what this guards, and refusing it had a
// cost: HAProxy's `option httpchk` sends no Host unless one is configured, so
// a listener bound to a specific address answered every check with 403 and was
// marked permanently DOWN by a balancer doing exactly what it was told.
func TestHostValidationMiddleware_ARequestNamingNoHostIsServed(t *testing.T) {
	t.Parallel()

	status, reached := servedBy(t, newHostGuard("127.0.0.1:8080", "", trustedProxies{}),
		loopbackRequest(t, ""))

	if !reached {
		t.Error("a request with no Host header was refused; a polling balancer's health check carries none")
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
}

// TestHostValidationMiddleware_AllowedHost verifies that the middleware
// passes through when the Host header matches, with and without a port: a
// browser sends the port it connected to, and the allow-list holds hosts.
func TestHostValidationMiddleware_AllowedHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
	}{
		{name: "the bare host", host: "localhost"},
		{name: "the host with the port it was reached on", host: "localhost:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status, reached := servedBy(t, newHostGuard("localhost:8080", "", trustedProxies{}),
				loopbackRequest(t, tt.host))
			if !reached || status != http.StatusOK {
				t.Errorf("status = %d reached = %v, want 200 and the handler reached", status, reached)
			}
		})
	}
}

// TestHostValidationMiddleware_PublicURLHostIsServed pins the decision this
// guard exists to make workable: the host an operator advertises with
// --public-url is a host this deployment serves.
//
// Every reverse proxy in the deployment guide forwards the client's Host and
// connects over loopback, so before this the documented configurations were
// answered 403 on both /mcp and /health, and no flag made them work. The
// operator has already published that name in the RFC 9728 metadata, so
// declaring it once is enough.
func TestHostValidationMiddleware_PublicURLHostIsServed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		addr      string
		publicURL string
		host      string
		want      int
	}{
		{
			name:      "the advertised host on a loopback bind",
			addr:      "127.0.0.1:8080",
			publicURL: "https://mcp.example.com",
			host:      "mcp.example.com",
			want:      http.StatusOK,
		},
		{
			name:      "the advertised host with the port the client used",
			addr:      "127.0.0.1:8080",
			publicURL: "https://mcp.example.com",
			host:      "mcp.example.com:443",
			want:      http.StatusOK,
		},
		{
			name:      "an advertised origin that carries a port of its own",
			addr:      "127.0.0.1:8080",
			publicURL: "https://mcp.example.com:8443/gitlab",
			host:      "mcp.example.com:8443",
			want:      http.StatusOK,
		},
		{
			name:      "the advertised host on a wildcard bind",
			addr:      ":8080",
			publicURL: "https://mcp.example.com",
			host:      "mcp.example.com",
			want:      http.StatusOK,
		},
		{
			name:      "a host nobody advertised",
			addr:      "127.0.0.1:8080",
			publicURL: "https://mcp.example.com",
			host:      "rebound.example.com",
			want:      http.StatusForbidden,
		},
		{
			name:      "a host nobody advertised, on a wildcard bind reached over loopback",
			addr:      ":8080",
			publicURL: "https://mcp.example.com",
			host:      "rebound.example.com",
			want:      http.StatusForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status, _ := servedBy(t, newHostGuard(tt.addr, tt.publicURL, trustedProxies{}),
				loopbackRequest(t, tt.host))
			if status != tt.want {
				t.Errorf("status = %d, want %d", status, tt.want)
			}
		})
	}
}

// TestHostValidationMiddleware_HostComparisonIgnoresCase pins that the guard
// asks the question DNS asks.
//
// Neither url.Hostname nor http.Request.Host folds case, so a comparison on
// the raw strings makes the spelling of a configured host part of the policy.
// A browser lowercases the authority before sending it, so an operator who
// wrote --public-url=https://MCP.Example.com would have published an origin
// their own deployment answers 403 to, and the message would tell them to pass
// the flag they had already passed. The listen address has the same problem
// from the other direction.
func TestHostValidationMiddleware_HostComparisonIgnoresCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		addr      string
		publicURL string
		host      string
		want      int
	}{
		{
			name:      "a mixed-case public URL against the host a browser sends",
			addr:      "127.0.0.1:8080",
			publicURL: "https://MCP.Example.COM",
			host:      "mcp.example.com",
			want:      http.StatusOK,
		},
		{
			name:      "a mixed-case Host against a lower-case public URL",
			addr:      "127.0.0.1:8080",
			publicURL: "https://mcp.example.com",
			host:      "MCP.Example.com",
			want:      http.StatusOK,
		},
		{
			name:      "a mixed-case public URL carrying a port, with the port the client used",
			addr:      "127.0.0.1:8080",
			publicURL: "https://MCP.Example.com:8443",
			host:      "mcp.example.com:8443",
			want:      http.StatusOK,
		},
		{
			name:      "a mixed-case listen address against the host a client sends",
			addr:      "MCP.Example.com:8080",
			publicURL: "",
			host:      "mcp.example.com:8080",
			want:      http.StatusOK,
		},
		{
			name:      "a different host in any case is still refused",
			addr:      "127.0.0.1:8080",
			publicURL: "https://MCP.Example.com",
			host:      "REBOUND.example.com",
			want:      http.StatusForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status, _ := servedBy(t, newHostGuard(tt.addr, tt.publicURL, trustedProxies{}),
				loopbackRequest(t, tt.host))
			if status != tt.want {
				t.Errorf("status = %d, want %d", status, tt.want)
			}
		})
	}
}

// TestHostValidationMiddleware_TrustedProxyForwardsItsHost pins the second
// exception: a hop the operator listed in --trusted-proxies may forward
// whatever host it heard.
//
// The list is already the operator's statement that requests from that address
// are relayed rather than authored, and forwarding the client's Host is what
// every proxy in the deployment guide is configured to do. A browser carrying
// out a rebinding attack is not that hop: it reaches the listener itself, from
// an address nobody listed.
func TestHostValidationMiddleware_TrustedProxyForwardsItsHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		proxies []string
		peer    string
		want    int
	}{
		{name: "from the listed proxy", proxies: []string{"127.0.0.1"}, peer: "127.0.0.1:9000", want: http.StatusOK},
		{name: "from a listed range", proxies: []string{"10.0.0.0/8"}, peer: "10.1.2.3:9000", want: http.StatusOK},
		{name: "from an address nobody listed", proxies: []string{"10.0.0.0/8"}, peer: "192.0.2.9:9000", want: http.StatusForbidden},
		{name: "with no proxy listed at all", proxies: nil, peer: "127.0.0.1:9000", want: http.StatusForbidden},
		{name: "from a peer that is not an address", proxies: []string{"127.0.0.1"}, peer: "not-an-address", want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			guard := newHostGuard("127.0.0.1:8080", "", trustedProxiesOf(tt.proxies))
			req := requestFrom(t, "mcp.example.com", tt.peer, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080})
			if status, _ := servedBy(t, guard, req); status != tt.want {
				t.Errorf("status = %d, want %d", status, tt.want)
			}
		})
	}
}

// TestHostValidationMiddleware_WildcardBindKeepsTheRebindingRule pins what a
// deployment that declares no host of its own still refuses.
//
// A wildcard bind names no interface, so there is no allow-list to check
// against, and the SDK used to apply the only rule left for the MCP endpoint:
// a connection accepted over loopback is a local server, and a local server is
// what a rebinding attack aims at. That rule is applied here now, for the
// whole handler chain rather than one endpoint, because the SDK's copy cannot
// see --public-url or --trusted-proxies and refused both exceptions above.
func TestHostValidationMiddleware_WildcardBindKeepsTheRebindingRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		host  string
		local net.Addr
		want  int
	}{
		{
			name:  "a foreign host on a connection accepted over loopback",
			host:  "rebound.example.com",
			local: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080},
			want:  http.StatusForbidden,
		},
		{
			name:  "a loopback host on a connection accepted over loopback",
			host:  "127.0.0.2:8080",
			local: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080},
			want:  http.StatusOK,
		},
		{
			name:  "a foreign host on a connection accepted on a routable address",
			host:  "mcp.example.com",
			local: &net.TCPAddr{IP: net.IPv4(192, 0, 2, 10), Port: 8080},
			want:  http.StatusOK,
		},
		{
			name:  "a foreign host on a unix socket, which no name resolves to",
			host:  "mcp.example.com",
			local: &net.UnixAddr{Name: "/run/gitlab-mcp/server.sock", Net: "unix"},
			want:  http.StatusOK,
		},
		{
			name:  "a foreign host on a connection with no listener address recorded",
			host:  "mcp.example.com",
			local: nil,
			want:  http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			guard := newHostGuard(":8080", "", trustedProxies{})
			if status, _ := servedBy(t, guard, requestFrom(t, tt.host, "127.0.0.1:9000", tt.local)); status != tt.want {
				t.Errorf("status = %d, want %d", status, tt.want)
			}
		})
	}
}

// TestHostValidationMiddleware_RefusalNamesTheFlagThatFixesIt pins that the
// 403 tells an operator what to do about it. The failure this guard produces
// is a proxy misconfiguration far more often than an attack, and a refusal
// that only says no leaves them reading source.
func TestHostValidationMiddleware_RefusalNamesTheFlagThatFixesIt(t *testing.T) {
	t.Parallel()

	handler := hostValidationMiddleware(newHostGuard("127.0.0.1:8080", "", trustedProxies{}), http.NotFoundHandler())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, loopbackRequest(t, "mcp.example.com"))

	if !strings.Contains(rr.Body.String(), "--public-url") {
		t.Errorf("the refusal does not name the flag that admits the host: %s", rr.Body.String())
	}
}

// TestHostValidationMiddleware_LogsABoundedHost verifies that the refusal an
// unauthenticated caller triggers cannot write an arbitrary amount to the log.
//
// The Host header is caller-supplied and was logged verbatim, so the refusal
// meant to protect against DNS rebinding was also the cheapest way to fill an
// operator's disk. The bounded prefix keeps the line diagnostic, since a
// rebinding attempt is recognizable from its first bytes, and host_len keeps
// the fact that it was truncated visible.
func TestHostValidationMiddleware_LogsABoundedHost(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))

	handler := hostValidationMiddleware(newHostGuard("localhost:8080", "", trustedProxies{}), http.NotFoundHandler())
	req := loopbackRequest(t, strings.Repeat("a", 32_000))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if logged.Len() > 4096 {
		t.Errorf("a single refused request wrote %d bytes of log for a %d byte Host", logged.Len(), len(req.Host))
	}
	if !strings.Contains(logged.String(), `"host_len":32000`) {
		t.Errorf("the log does not report the real Host length: %s", logged.String())
	}
}

// TestNewHostGuard_DeclaresTheLoopbackNamesAndTheAdvertisedHost pins what the
// guard is built from, since the middleware above can only show the verdicts.
func TestNewHostGuard_DeclaresTheLoopbackNamesAndTheAdvertisedHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		addr      string
		publicURL string
		wantBound bool
		declared  []string
		absent    []string
	}{
		{
			name:      "a loopback bind with no advertised origin",
			addr:      "127.0.0.1:8080",
			wantBound: true,
			declared:  []string{"localhost", "127.0.0.1", "::1"},
			absent:    []string{"mcp.example.com"},
		},
		{
			name:      "a routable bind advertising an origin",
			addr:      "10.0.0.5:8080",
			publicURL: "https://mcp.example.com/gitlab",
			wantBound: true,
			declared:  []string{"10.0.0.5", "mcp.example.com", "localhost"},
		},
		{
			name:      "a wildcard bind declares no interface of its own",
			addr:      ":8080",
			publicURL: "https://mcp.example.com",
			wantBound: false,
			declared:  []string{"mcp.example.com", "localhost", "127.0.0.1", "::1"},
		},
		{
			name:      "a unix socket declares no interface either",
			addr:      filepath.Join(t.TempDir(), "mcp.sock"),
			wantBound: false,
			declared:  []string{"localhost"},
		},
		{
			name:      "an unparseable origin declares nothing extra",
			addr:      "127.0.0.1:8080",
			publicURL: "://not a url",
			wantBound: true,
			declared:  []string{"localhost"},
			absent:    []string{"", "not a url"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			guard := newHostGuard(tt.addr, tt.publicURL, trustedProxies{})
			if guard.bound != tt.wantBound {
				t.Errorf("bound = %v, want %v", guard.bound, tt.wantBound)
			}
			for _, host := range tt.declared {
				if !guard.declared[host] {
					t.Errorf("%q is not declared: %v", host, guard.declared)
				}
			}
			for _, host := range tt.absent {
				if guard.declared[host] {
					t.Errorf("%q is declared and should not be: %v", host, guard.declared)
				}
			}
		})
	}
}

// TestHostGuard_TrustsPeer_ReadsTheConnectionsAddress covers the trusted-proxy
// exception at the level the middleware cannot reach: which peer addresses are
// believed, independent of what any of them forwards.
func TestHostGuard_TrustsPeer_ReadsTheConnectionsAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		proxies []string
		peer    string
		want    bool
	}{
		{name: "the listed address", proxies: []string{"127.0.0.1"}, peer: "127.0.0.1:1", want: true},
		{name: "an IPv4 address inside a listed range", proxies: []string{"10.0.0.0/8"}, peer: "10.9.9.9:1", want: true},
		{name: "an IPv6 peer against its own listed address", proxies: []string{"::1"}, peer: "[::1]:1", want: true},
		{name: "an address outside every listed range", proxies: []string{"10.0.0.0/8"}, peer: "192.0.2.1:1", want: false},
		{name: "an empty list believes nobody", proxies: nil, peer: "127.0.0.1:1", want: false},
		{name: "a peer with no address at all", proxies: []string{"127.0.0.1"}, peer: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			guard := hostGuard{declared: map[string]bool{}, proxies: trustedProxiesOf(tt.proxies)}
			req := requestFrom(t, "mcp.example.com", tt.peer, nil)
			if got := guard.trustsPeer(req); got != tt.want {
				t.Errorf("trustsPeer(%q) = %v, want %v", tt.peer, got, tt.want)
			}
		})
	}
}

// TestPublicURLHost_IsTheOriginWithoutItsPort pins the value the guard
// declares, since a Host header is compared without its port and an IPv6
// literal arrives bracketed.
func TestPublicURLHost_IsTheOriginWithoutItsPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "an origin", url: "https://mcp.example.com", want: "mcp.example.com"},
		{name: "an origin with a path", url: "https://mcp.example.com/gitlab", want: "mcp.example.com"},
		{name: "an origin with a port", url: "https://mcp.example.com:8443", want: "mcp.example.com"},
		{name: "an IPv6 origin", url: "https://[2001:db8::1]:8443", want: "2001:db8::1"},
		{name: "no origin at all", url: "", want: ""},
		{name: "a value that is not a URL", url: "://not a url", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := publicURLHost(tt.url); got != tt.want {
				t.Errorf("publicURLHost(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// TestIsLoopbackHost_MatchesEveryLocalName pins the rebinding rule's notion of
// "this machine", which is the SDK's: the name localhost and every loopback
// address, not the 127.0.0.1 spelling alone.
func TestIsLoopbackHost_MatchesEveryLocalName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		host string
		want bool
	}{
		{host: "localhost", want: true},
		{host: "127.0.0.1", want: true},
		{host: "127.0.0.2", want: true},
		{host: "::1", want: true},
		{host: "mcp.example.com", want: false},
		{host: "192.0.2.10", want: false},
		{host: "/run/gitlab-mcp/server.sock", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			t.Parallel()
			if got := isLoopbackHost(tt.host); got != tt.want {
				t.Errorf("isLoopbackHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

// TestHostOnly_StripsThePortAndNothingElse pins the split a Host header needs:
// a bracketed IPv6 literal keeps its address and loses its port, and a value
// with no port survives untouched.
func TestHostOnly_StripsThePortAndNothingElse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		want  string
	}{
		{value: "mcp.example.com:443", want: "mcp.example.com"},
		{value: "mcp.example.com", want: "mcp.example.com"},
		{value: "[::1]:8080", want: "::1"},
		{value: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()
			if got := hostOnly(tt.value); got != tt.want {
				t.Errorf("hostOnly(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestArrivedOnLoopback_ReadsTheListenerAddress pins the half of the rebinding
// rule that describes the server rather than the request: a connection
// accepted over loopback is a local server, whatever address the peer has.
func TestArrivedOnLoopback_ReadsTheListenerAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		local net.Addr
		want  bool
	}{
		{name: "a loopback listener", local: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}, want: true},
		{name: "an IPv6 loopback listener", local: &net.TCPAddr{IP: net.IPv6loopback, Port: 8080}, want: true},
		{name: "a routable listener", local: &net.TCPAddr{IP: net.IPv4(192, 0, 2, 10), Port: 8080}, want: false},
		{name: "a unix socket", local: &net.UnixAddr{Name: "/run/gitlab-mcp/server.sock", Net: "unix"}, want: false},
		{name: "no listener address recorded", local: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := arrivedOnLoopback(requestFrom(t, "example.invalid", "127.0.0.1:1", tt.local)); got != tt.want {
				t.Errorf("arrivedOnLoopback() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestArrivedOnLoopback_ANilListenerAddressIsNotLocal covers the value a
// context can carry that is a net.Addr and holds nothing. net/http never
// stores one, so this is the unreachable half of the check that keeps a
// handler driven directly from panicking on it.
func TestArrivedOnLoopback_ANilListenerAddressIsNotLocal(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(t.Context(), http.LocalAddrContextKey, net.Addr(nilAddr{}))
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid/health", http.NoBody)
	if arrivedOnLoopback(req) {
		t.Error("a listener address that names nothing was read as loopback")
	}
}

// nilAddr is a net.Addr that names nothing, which is what the check above
// guards against.
type nilAddr struct{}

func (nilAddr) Network() string { return "" }
func (nilAddr) String() string  { return "" }

// TestAllowedHosts_Localhost verifies that allowedHosts returns the expected
// set for a localhost binding.
func TestAllowedHosts_Localhost(t *testing.T) {
	t.Parallel()

	hosts := allowedHosts("127.0.0.1:8080")
	if hosts == nil {
		t.Fatal("expected non-nil hosts for localhost binding")
	}
	if !hosts["127.0.0.1"] {
		t.Error("missing 127.0.0.1")
	}
	if !hosts["localhost"] {
		t.Error("missing localhost")
	}
}

// TestAllowedHosts_AllInterfaces verifies that allowedHosts returns nil for
// 0.0.0.0 and for :: (bind to all interfaces), which name no host of their own.
func TestAllowedHosts_AllInterfaces(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"0.0.0.0:8080", "[::]:8080"} {
		t.Run(addr, func(t *testing.T) {
			t.Parallel()
			if hosts := allowedHosts(addr); hosts != nil {
				t.Errorf("allowedHosts(%q) = %v, want nil: a wildcard bind names no interface", addr, hosts)
			}
		})
	}
}

// TestAllowedHosts_EmptyHost verifies that allowedHosts returns nil
// for an empty host, which means all interfaces.
func TestAllowedHosts_EmptyHost(t *testing.T) {
	t.Parallel()

	hosts := allowedHosts(":8080")
	if hosts != nil {
		t.Error("expected nil hosts for empty host")
	}
}

// TestAllowedHosts_UnixSocket_SkipsValidation pins that a listener on a
// unix socket gets no Host allow-list, whatever form its path takes.
//
// The native form matters: on Windows it carries a drive letter, and
// SplitHostPort read that letter as the host, so a socket server there
// refused every request whose Host was not "C". The POSIX and relative
// forms passed before by accident, having no colon to split on, and are
// pinned so the decision no longer depends on that.
func TestAllowedHosts_UnixSocket_SkipsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		addr string
	}{
		{name: "an absolute POSIX path", addr: "/run/gitlab-mcp/mcp.sock"},
		{name: "a relative path", addr: "./mcp.sock"},
		{name: "the platform's native form", addr: filepath.Join(t.TempDir(), "mcp.sock")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if hosts := allowedHosts(tt.addr); hosts != nil {
				t.Errorf("allowedHosts(%q) = %v, want nil: a socket client's Host header is arbitrary", tt.addr, hosts)
			}
		})
	}
}

// TestHostGuard_PermitsMatchesTheSDKRuleItReplaced pins the case the SDK's own
// localhost protection used to decide, now that it is switched off: an
// undeclared host on a loopback connection is refused, and the same host on a
// connection accepted elsewhere is not.
//
// Expressed against netip rather than through the middleware so the rule is
// readable on its own, since it is the one piece of behavior taken over from
// a dependency rather than written here.
func TestHostGuard_PermitsMatchesTheSDKRuleItReplaced(t *testing.T) {
	t.Parallel()

	guard := newHostGuard(":8080", "", trustedProxies{})
	for _, tt := range []struct {
		name  string
		local netip.AddrPort
		want  bool
	}{
		{name: "accepted over loopback", local: netip.MustParseAddrPort("127.0.0.1:8080"), want: false},
		{name: "accepted on a routable address", local: netip.MustParseAddrPort("192.0.2.10:8080"), want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			local := net.TCPAddrFromAddrPort(tt.local)
			req := requestFrom(t, "rebound.example.com", "198.51.100.7:5000", local)
			if got := guard.permits(req); got != tt.want {
				t.Errorf("permits() = %v, want %v", got, tt.want)
			}
		})
	}
}
