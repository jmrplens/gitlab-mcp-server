package main

import (
	"context"
	"io"
	"log/slog"
	"maps"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// discoveryClock is a clock a test moves by hand, read from the test's own
// goroutine only.
type discoveryClock struct{ at time.Time }

func (c *discoveryClock) now() time.Time          { return c.at }
func (c *discoveryClock) advance(d time.Duration) { c.at = c.at.Add(d) }

// testDiscoveryLimiter is a limiter of the given rate, burst and table size on
// a clock the test moves, charging the document paths of a legacy deployment
// with no prefix and believing X-Forwarded-For from 192.0.2.1 alone.
func testDiscoveryLimiter(rps float64, burst, capacity int) (*discoveryRateLimiter, *discoveryClock) {
	clock := &discoveryClock{at: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)}
	limiter := newDiscoveryRateLimiter(&config.Config{
		DiscoveryRateLimitRPS:   rps,
		DiscoveryRateLimitBurst: burst,
		TrustedProxyHeader:      "X-Forwarded-For",
		TrustedProxies:          []string{"192.0.2.1"},
	})
	limiter.capacity = capacity
	limiter.now = clock.now
	return limiter, clock
}

// allowances is what n requests from key in a row were answered.
func allowances(d *discoveryRateLimiter, key string, n int) []bool {
	out := make([]bool, n)
	for i := range out {
		out[i] = d.allow(key)
	}
	return out
}

// TestDiscoveryRateLimiter_Allow_SpendsTheBurstThenRefillsAtTheRate covers the
// bucket itself: an address that has asked for nothing has the whole burst in
// hand, is refused once it is spent, and is given one request back for every
// interval the rate refills one in, and no more.
func TestDiscoveryRateLimiter_Allow_SpendsTheBurstThenRefillsAtTheRate(t *testing.T) {
	t.Parallel()
	limiter, clock := testDiscoveryLimiter(2, 3, 8)

	if got, want := allowances(limiter, "203.0.113.7", 4), []bool{true, true, true, false}; !slices.Equal(got, want) {
		t.Errorf("a fresh address = %v, want the burst of 3 served and the fourth refused", got)
	}
	clock.advance(500 * time.Millisecond)
	if got, want := allowances(limiter, "203.0.113.7", 2), []bool{true, false}; !slices.Equal(got, want) {
		t.Errorf("half a second at 2/s = %v, want one request back and no more", got)
	}
}

// TestDiscoveryRateLimiter_Allow_KeepsEachAddressApart covers the key: one
// address that spent its bucket costs another nothing, which is the whole
// difference between a bucket per address and one the process shares.
func TestDiscoveryRateLimiter_Allow_KeepsEachAddressApart(t *testing.T) {
	t.Parallel()
	limiter, _ := testDiscoveryLimiter(1, 1, 8)

	if got := allowances(limiter, "203.0.113.7", 2); !slices.Equal(got, []bool{true, false}) {
		t.Fatalf("the first address = %v, want one served and one refused", got)
	}
	if !limiter.allow("198.51.100.9") {
		t.Error("a second address was refused because the first spent its bucket")
	}
}

// TestDiscoveryRateLimiter_Allow_FullTableForgetsTheLeastRecentAddress covers
// the table's bound. A new address past it takes the place of the one used
// least recently, and the one dropped is the one that stopped asking: an
// address that keeps asking, refused or not, is moved to the front each time
// and so stays counted, and the newcomer is given a bucket of its own, where
// a table that stopped counting at its bound would serve it uncounted. The
// address dropped starts again with a full bucket, since what was forgotten
// is a record against it and not a holding of its, which is also why neither
// rule bounds a caller cycling more addresses than the table holds (F-36).
func TestDiscoveryRateLimiter_Allow_FullTableForgetsTheLeastRecentAddress(t *testing.T) {
	t.Parallel()
	limiter, _ := testDiscoveryLimiter(1, 1, 2)

	limiter.allow("flood")
	limiter.allow("quiet")
	// The flood asks again after the quiet address did, so the quiet one is
	// the least recent when a third address arrives.
	if limiter.allow("flood") {
		t.Fatal("the flood's second request was served from a bucket of one")
	}
	if !limiter.allow("newcomer") {
		t.Error("a new address was refused at a full table; it should take the least recent address's place")
	}
	if got := limiter.recency.Len(); got != 2 || len(limiter.buckets) != 2 {
		t.Errorf("the table holds %d buckets and indexes %d, want both at its bound of 2", got, len(limiter.buckets))
	}
	if limiter.allow("flood") {
		t.Error("the flood was forgotten to admit the newcomer, though it was the most recent before it")
	}
	// The quiet address was dropped, and comes back to a full bucket: it takes
	// the place of the newcomer, now the least recent.
	if !limiter.allow("quiet") {
		t.Error("the address the table dropped came back to an empty bucket; forgetting it should have cost it nothing")
	}
	if _, kept := limiter.buckets["newcomer"]; kept {
		t.Error("the newcomer is still held, though it was the least recent when the quiet address came back")
	}
}

// TestDiscoveryRateLimiter_Cleanup_ForgetsOnlyTheBucketsThatRefilled covers
// the sweep: a bucket that has refilled is the one a fresh address would get,
// so forgetting it changes no answer and gives its memory back, while a bucket
// still refilling is a record the next request depends on and is kept.
func TestDiscoveryRateLimiter_Cleanup_ForgetsOnlyTheBucketsThatRefilled(t *testing.T) {
	t.Parallel()
	limiter, clock := testDiscoveryLimiter(1, 2, 8)

	allowances(limiter, "drained", 2)
	allowances(limiter, "touched", 1)
	clock.advance(time.Second)
	limiter.cleanup()
	if _, kept := limiter.buckets["drained"]; !kept {
		t.Error("a bucket with one of its two requests back was forgotten; its next answer depends on it")
	}
	if _, kept := limiter.buckets["touched"]; kept {
		t.Error("a bucket that has refilled was kept; a fresh address would get the same bucket")
	}
	clock.advance(time.Second)
	limiter.cleanup()
	if limiter.recency.Len() != 0 || len(limiter.buckets) != 0 {
		t.Errorf("after both refilled the table holds %d buckets and indexes %d, want none", limiter.recency.Len(), len(limiter.buckets))
	}
}

// TestNewDiscoveryRateLimiter_ZeroForEitherSettingTurnsItOff covers what zero
// means for the two settings (INV-015): a bucket refilling at nothing, or one
// holding nothing, would refuse every request, which no operator asking for
// zero means, so either turns the limiter off and builds nothing. Otherwise
// the limiter carries the rate, the burst, the register's table size, the
// document paths and the proxy settings its key is read with.
func TestNewDiscoveryRateLimiter_ZeroForEitherSettingTurnsItOff(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		rps   float64
		burst int
	}{
		{name: "a zero rate", rps: 0, burst: 10},
		{name: "a zero burst", rps: 10, burst: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if limiter := newDiscoveryRateLimiter(&config.Config{DiscoveryRateLimitRPS: tc.rps, DiscoveryRateLimitBurst: tc.burst}); limiter != nil {
				t.Errorf("newDiscoveryRateLimiter(rps %g, burst %d) = %+v, want nil", tc.rps, tc.burst, limiter)
			}
		})
	}

	cfg := &config.Config{
		DiscoveryRateLimitRPS: 2.5, DiscoveryRateLimitBurst: 9,
		TrustedProxyHeader: "X-Real-IP", TrustedProxies: []string{"10.0.0.0/8"},
	}
	limiter := newDiscoveryRateLimiter(cfg)
	if limiter == nil {
		t.Fatal("newDiscoveryRateLimiter(rps 2.5, burst 9) = nil, want a limiter")
	}
	if limiter.limit != rate.Limit(2.5) || limiter.burst != 9 || limiter.capacity != tenancy.DiscoveryTrackedAddresses {
		t.Errorf("limit %g, burst %d, capacity %d; want 2.5, 9 and %d", float64(limiter.limit), limiter.burst, limiter.capacity, tenancy.DiscoveryTrackedAddresses)
	}
	var paths []string
	for path := range limiter.paths {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	want := slices.Sorted(slices.Values(discoveryDocumentPaths(cfg)))
	if !slices.Equal(paths, want) {
		t.Errorf("paths = %v, want the document paths %v", paths, want)
	}
	if limiter.trustedProxyHeader != "X-Real-IP" || !limiter.trustedProxies.contains(netip.MustParseAddr("10.1.2.3")) {
		t.Errorf("trusted proxy header %q and proxies %+v, want X-Real-IP believed from 10.0.0.0/8", limiter.trustedProxyHeader, limiter.trustedProxies)
	}
	if before := time.Now(); limiter.now().Before(before.Add(-time.Second)) {
		t.Error("the limiter's clock is not the wall clock")
	}
}

// TestDiscoveryDocumentPaths_AreThePathsTheDocumentsAreMountedAt covers which
// paths draw on the bucket: the SEP-2127 card at the root and after /mcp, the
// legacy card, each under the --public-url prefix as well, and in oauth mode
// the RFC 9728 metadata at the one path RFC 9728 derives from the resource
// identifier, which carries the prefix itself. /health and the MCP endpoint
// are not among them, and legacy mode publishes no metadata document.
func TestDiscoveryDocumentPaths_AreThePathsTheDocumentsAreMountedAt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		cfg  *config.Config
		want []string
	}{
		{
			name: "legacy, no prefix",
			cfg:  &config.Config{AuthMode: config.AuthModeLegacy},
			want: []string{"/server-card", "/mcp/server-card", "/.well-known/mcp/server-card.json"},
		},
		{
			name: "legacy, a prefix",
			cfg:  &config.Config{AuthMode: config.AuthModeLegacy, PublicURL: "https://mcp.example.test/gitlab"},
			want: []string{
				"/server-card", "/gitlab/server-card", "/mcp/server-card", "/gitlab/mcp/server-card",
				"/.well-known/mcp/server-card.json", "/gitlab/.well-known/mcp/server-card.json",
			},
		},
		{
			name: "oauth, no prefix",
			cfg:  &config.Config{AuthMode: config.AuthModeOAuth, PublicURL: "https://mcp.example.test"},
			want: []string{
				"/server-card", "/mcp/server-card", "/.well-known/mcp/server-card.json",
				"/.well-known/oauth-protected-resource",
			},
		},
		{
			name: "oauth, a prefix",
			cfg:  &config.Config{AuthMode: config.AuthModeOAuth, PublicURL: "https://mcp.example.test/gitlab"},
			want: []string{
				"/server-card", "/gitlab/server-card", "/mcp/server-card", "/gitlab/mcp/server-card",
				"/.well-known/mcp/server-card.json", "/gitlab/.well-known/mcp/server-card.json",
				"/.well-known/oauth-protected-resource/gitlab",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := discoveryDocumentPaths(tc.cfg); !slices.Equal(got, tc.want) {
				t.Errorf("discoveryDocumentPaths() = %v, want %v", got, tc.want)
			}
		})
	}
}

// discoveryRequest is a request to path from peer, carrying X-Forwarded-For
// when forwarded is not empty.
func discoveryRequest(t *testing.T, method, path, peer, forwarded string) *http.Request {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	r.RemoteAddr = net.JoinHostPort(peer, "40000")
	if forwarded != "" {
		r.Header.Set("X-Forwarded-For", forwarded)
	}
	return r
}

// countingHandler answers 200 and counts what reached it.
type countingHandler struct{ reached int }

func (h *countingHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.reached++
	w.WriteHeader(http.StatusOK)
}

// statusesOf sends each request through handler and returns the statuses.
func statusesOf(handler http.Handler, requests ...*http.Request) []int {
	out := make([]int, 0, len(requests))
	for _, r := range requests {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		out = append(out, rec.Code)
	}
	return out
}

// TestDiscoveryRateLimiter_Middleware_ChargesTheDocumentPathsAlone covers what
// draws on the bucket: every request to a document path, whatever its method,
// so a revalidation and a preflight cost what a fetch does, and nothing else,
// so /health, the MCP endpoint and a path nobody serves pass however spent the
// address's bucket is. Its subtests share the one spent bucket and the handler
// counting what reached it, so neither it nor they run in parallel.
func TestDiscoveryRateLimiter_Middleware_ChargesTheDocumentPathsAlone(t *testing.T) {
	limiter, _ := testDiscoveryLimiter(1, 2, 8)
	next := &countingHandler{}
	handler := limiter.middleware(next)
	const peer = "203.0.113.7"

	got := statusesOf(handler,
		discoveryRequest(t, http.MethodGet, "/server-card", peer, ""),
		discoveryRequest(t, http.MethodOptions, "/.well-known/mcp/server-card.json", peer, ""),
		discoveryRequest(t, http.MethodHead, "/mcp/server-card", peer, ""),
	)
	if want := []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests}; !slices.Equal(got, want) {
		t.Errorf("a fetch, a preflight and a HEAD on the document paths = %v, want %v: one bucket of two for the three", got, want)
	}
	for _, path := range []string{"/health", "/mcp", "/", "/nope", "/server-card/"} {
		t.Run(path, func(t *testing.T) {
			if passed := statusesOf(handler, discoveryRequest(t, http.MethodGet, path, peer, "")); !slices.Equal(passed, []int{http.StatusOK}) {
				t.Errorf("GET %s from an address that spent its bucket = %v, want it passed untouched", path, passed)
			}
		})
	}
	if next.reached != 2+5 {
		t.Errorf("%d requests reached the handler, want the 2 the bucket paid for and the 5 it does not charge", next.reached)
	}
}

// TestDiscoveryRateLimiter_Middleware_RefusesInTheGatesShape covers the
// refusal: the gate's 429, a JSON-RPC error with 429's code and the words the
// register holds (RTC-008), the register's fixed Retry-After, readable from
// any origin with its Retry-After exposed, written before anything downstream
// runs, and logged once a window with the limit's two figures as its only
// attributes, so the line never carries the address it refused (INV-013),
// under any key.
func TestDiscoveryRateLimiter_Middleware_RefusesInTheGatesShape(t *testing.T) {
	forgetRefusalLines()
	logged := captureLogAttrs(t)
	limiter, _ := testDiscoveryLimiter(1, 1, 8)
	next := &countingHandler{}
	handler := limiter.middleware(next)

	statusesOf(handler, discoveryRequest(t, http.MethodGet, "/server-card", "203.0.113.7", ""))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, discoveryRequest(t, http.MethodGet, "/server-card", "203.0.113.7", ""))

	if rec.Code != http.StatusTooManyRequests || next.reached != 1 {
		t.Fatalf("status %d with %d requests reaching the handler, want 429 and only the first through", rec.Code, next.reached)
	}
	refusal := decodeJSONRPCError(t, rec.Body.String())
	if refusal.Error.Code != tenancy.CodeTooManyRequests ||
		refusal.Error.Message != "Too many requests for the discovery documents from this address. Retry later." {
		t.Errorf("refusal = %+v, want 429's code and the register's words", refusal.Error)
	}
	for header, want := range map[string]string{
		"Retry-After":                   "1",
		"Content-Type":                  "application/json",
		"Access-Control-Allow-Origin":   "*",
		"Access-Control-Expose-Headers": "Retry-After",
	} {
		t.Run(header, func(t *testing.T) {
			if got := rec.Header().Get(header); got != want {
				t.Errorf("%s = %q, want %q", header, got, want)
			}
		})
	}
	attrs, ok := logged("request refused: too many requests for the discovery documents from one address")
	if !ok {
		t.Fatal("the refusal was not logged")
	}
	want := map[string]slog.Value{"limit_rps": slog.Float64Value(1), "burst": slog.IntValue(1)}
	if !maps.EqualFunc(attrs, want, slog.Value.Equal) {
		t.Errorf("the refusal was logged with %v, want the limit's figures %v and nothing else", attrs, want)
	}
	for key, value := range attrs {
		if strings.Contains(value.String(), "203.0.113.7") {
			t.Errorf("the refusal logged the address it refused under %q", key)
		}
	}
}

// TestDiscoveryRateLimiter_Middleware_KeysTheAddressTheBudgetsCharge covers
// the key, which is the authentication budgets' (IDN-004): the forwarded
// address only from a trusted proxy, the peer from anybody else whatever its
// header says, and an IPv6 address by the /64 it lies in, so two addresses of
// one /64 share a bucket and two /64s apart do not.
func TestDiscoveryRateLimiter_Middleware_KeysTheAddressTheBudgetsCharge(t *testing.T) {
	t.Parallel()
	const proxy, stranger = "192.0.2.1", "198.51.100.7"
	for _, tc := range []struct {
		name          string
		first, second [2]string // peer and forwarded address
		apart         bool
	}{
		{name: "two callers named by the trusted proxy", first: [2]string{proxy, "203.0.113.1"}, second: [2]string{proxy, "203.0.113.2"}, apart: true},
		{name: "a header from a peer nobody trusts", first: [2]string{stranger, "203.0.113.1"}, second: [2]string{stranger, "203.0.113.2"}},
		{name: "two addresses of one /64", first: [2]string{proxy, "2001:db8:1:2::1"}, second: [2]string{proxy, "2001:db8:1:2:ffff::9"}},
		{name: "two /64s of one /48", first: [2]string{proxy, "2001:db8:1:2::1"}, second: [2]string{proxy, "2001:db8:1:3::1"}, apart: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			limiter, _ := testDiscoveryLimiter(1, 1, 8)
			handler := limiter.middleware(&countingHandler{})
			got := statusesOf(handler,
				discoveryRequest(t, http.MethodGet, "/server-card", tc.first[0], tc.first[1]),
				discoveryRequest(t, http.MethodGet, "/server-card", tc.second[0], tc.second[1]),
			)
			want := []int{http.StatusOK, http.StatusTooManyRequests}
			if tc.apart {
				want = []int{http.StatusOK, http.StatusOK}
			}
			if !slices.Equal(got, want) {
				t.Errorf("two requests with one request in each bucket = %v, want %v", got, want)
			}
		})
	}
}

// TestDiscoveryRateLimiter_Middleware_NilIsTheNextHandler covers a deployment
// that turned the limiter off: its middleware is the handler it wraps, so a
// document is served however often it is asked for.
func TestDiscoveryRateLimiter_Middleware_NilIsTheNextHandler(t *testing.T) {
	t.Parallel()
	next := &countingHandler{}
	var off *discoveryRateLimiter
	if handler := off.middleware(next); handler != http.Handler(next) {
		t.Errorf("a nil limiter's middleware = %T, want the next handler itself", handler)
	}
}

// TestDiscoveryRateFor_SweepsOnlyALimiterItBuilt covers the sweep: a limiter
// the deployment asked for is swept on the authentication tables' interval,
// so a bucket that refilled is forgotten without anybody asking again, and a
// deployment that turned it off starts no loop, since a loop over a table
// nobody fills would be a goroutine for the life of the process.
func TestDiscoveryRateFor_SweepsOnlyALimiterItBuilt(t *testing.T) {
	restore := periodicCleanupInterval
	periodicCleanupInterval = time.Millisecond
	t.Cleanup(func() { periodicCleanupInterval = restore })
	self := currentGoroutineID()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if off := discoveryRateFor(ctx, &config.Config{}); off != nil {
		t.Errorf("discoveryRateFor(off) = %+v, want nil", off)
	}
	if loops := cleanupLoopsStartedBy(self); loops != 0 {
		t.Errorf("%d sweep loops started for a limiter turned off, want none", loops)
	}

	limiter := discoveryRateFor(ctx, &config.Config{DiscoveryRateLimitRPS: 1000, DiscoveryRateLimitBurst: 1})
	if loops := cleanupLoopsStartedBy(self); loops != 1 {
		t.Errorf("%d sweep loops started for a limiter in force, want one", loops)
	}
	limiter.allow("203.0.113.7")
	deadline := time.Now().Add(testHTTPLivenessTimeout)
	for tracked(limiter) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("a bucket that refilled in a millisecond was never swept")
		}
		time.Sleep(time.Millisecond)
	}
}

// tracked is how many addresses the limiter holds a bucket for, read under its
// lock since the sweep writes it from its own goroutine.
func tracked(d *discoveryRateLimiter) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.recency.Len()
}

// TestValidateDiscoveryRateBounds_HoldsTheFlagsToTheirVariablesBounds covers
// the two flags' bounds, which their variables are held to as well: zero is
// off and accepted, the maxima are inclusive, and a negative value, one that
// is not a number and one past the maximum are refused naming the flag.
func TestValidateDiscoveryRateBounds_HoldsTheFlagsToTheirVariablesBounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		rps   float64
		burst int
		want  string
	}{
		{name: "off", rps: 0, burst: 0},
		{name: "at the maxima", rps: config.MaxDiscoveryRateLimitRPS, burst: config.MaxDiscoveryRateLimitBurst},
		{name: "a negative rate", rps: -1, burst: 1, want: "--discovery-rate-limit-rps must be a number of zero or more, got -1"},
		{name: "a rate that is not a number", rps: math.NaN(), burst: 1, want: "--discovery-rate-limit-rps must be a number of zero or more, got NaN"},
		{name: "an infinite rate", rps: math.Inf(1), burst: 1, want: "--discovery-rate-limit-rps +Inf exceeds maximum of 1000"},
		{name: "a rate past the maximum", rps: 1000.5, burst: 1, want: "--discovery-rate-limit-rps 1000.5 exceeds maximum of 1000"},
		{name: "a negative burst", rps: 1, burst: -1, want: "--discovery-rate-limit-burst must not be negative, got -1"},
		{name: "a burst past the maximum", rps: 1, burst: 10001, want: "--discovery-rate-limit-burst 10001 exceeds maximum of 10000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateDiscoveryRateBounds(&config.Config{DiscoveryRateLimitRPS: tc.rps, DiscoveryRateLimitBurst: tc.burst})
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("validateDiscoveryRateBounds() = %v, want nil", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Errorf("validateDiscoveryRateBounds() = %v, want an error carrying %q", err, tc.want)
			}
		})
	}
}

// startDiscoveryTestServer serves cfg through serveHTTPOn on a loopback port,
// as a deployment does, and returns its base URL; the server is stopped when
// the test ends.
func startDiscoveryTestServer(t *testing.T, cfg *config.Config) string {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	errCh := make(chan error, 1)
	go func() { errCh <- serveHTTPOn(ctx, cfg, addr, listener, defaultHTTPIdleTimeout) }()
	waitForHTTPServerReady(t, addr, errCh)
	t.Cleanup(func() {
		cancel()
		select {
		case serveErr := <-errCh:
			if serveErr != nil {
				t.Errorf("serveHTTPOn() = %v, want a clean stop", serveErr)
			}
		case <-time.After(testHTTPLivenessTimeout):
			t.Error("serveHTTPOn did not stop in time")
		}
	})
	return "http://" + addr
}

// discoveryAnswer is what one request to the test server was answered with.
type discoveryAnswer struct {
	status int
	header http.Header
}

// discoveryFetch sends one request to base+path in the name of caller, the
// address the trusted proxy in front of the test server names, and returns
// its status and headers, the body read and closed. An empty caller sends no
// forwarded address, and a Host among the headers replaces the one the URL
// gives, which is the only way a client sends another.
func discoveryFetch(t *testing.T, base, method, path, caller string, headers map[string]string) discoveryAnswer {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, base+path, nil)
	if err != nil {
		t.Fatalf("building %s %s: %v", method, path, err)
	}
	if caller != "" {
		req.Header.Set("X-Forwarded-For", caller)
	}
	for name, value := range headers {
		if name == "Host" {
			req.Host = value
			continue
		}
		req.Header.Set(name, value)
	}
	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	readAndCloseBody(t, resp)
	return discoveryAnswer{status: resp.StatusCode, header: resp.Header}
}

// discoveryServingConfig is an HTTP deployment published under a prefix, in
// the given auth mode, behind a trusted proxy on the loopback, with the
// discovery documents' bucket at rps and burst.
func discoveryServingConfig(gitlabURL, authMode string, rps float64, burst int) *config.Config {
	return &config.Config{
		GitLabURL:      gitlabURL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		ToolSurface:    config.ToolSurfaceDynamic,
		Stateless:      true,
		AuthMode:       authMode,
		PublicURL:      "https://mcp.example.test/gitlab",
		// The public URL's origin is trusted, as buildTrustedOrigins makes it
		// on a deployment started from flags, so the CORS layer answers that
		// origin's preflight itself before the mux: a bucket placed inside
		// that layer would never see it.
		TrustedOrigins: []string{"https://mcp.example.test"},
		OAuthCacheTTL:  config.DefaultOAuthCacheTTL,
		// Every caller arrives from the test's loopback, so the proxy names
		// each one, and each path is fetched by an address of its own.
		TrustedProxyHeader:      "X-Forwarded-For",
		TrustedProxies:          []string{"127.0.0.1"},
		DiscoveryRateLimitRPS:   rps,
		DiscoveryRateLimitBurst: burst,
	}
}

// standInServerCard replaces the legacy card's build, which stands up the
// whole catalog, with a document of its own for the length of the test.
func standInServerCard(t *testing.T) {
	t.Helper()
	original := buildServerCardFn
	buildServerCardFn = func(context.Context, *config.Config) ([]byte, error) {
		return []byte(`{"serverInfo":{"name":"stand-in"}}`), nil
	}
	t.Cleanup(func() { buildServerCardFn = original })
}

// TestServeHTTP_DiscoveryDocuments_DrawOnOneBucketPerAddress drives the
// limiter where a deployment installs it, in both auth modes and at every path
// a document is mounted at, the prefix forms and the RFC 9728 metadata among
// them. Each path is fetched by an address of its own from a bucket of two: a
// fetch is served, a revalidation is answered 304 and costs a request too, and
// a CORS preflight from the deployment's trusted origin, which the CORS layer
// answers itself with a 204 when the bucket lets it through, is then refused
// 429, which only a bucket outside that layer can do. The spent address still
// reaches /health and the MCP endpoint, which the bucket does not charge.
func TestServeHTTP_DiscoveryDocuments_DrawOnOneBucketPerAddress(t *testing.T) {
	standInServerCard(t)
	gitlab := newMockGitLabServer(t)
	for _, authMode := range []string{config.AuthModeLegacy, config.AuthModeOAuth} {
		t.Run(authMode, func(t *testing.T) {
			cfg := discoveryServingConfig(gitlab.URL, authMode, 0.001, 2)
			base := startDiscoveryTestServer(t, cfg)
			for i, path := range discoveryDocumentPaths(cfg) {
				t.Run(path, func(t *testing.T) {
					assertDocumentDrawsOnItsBucket(t, base, path, "203.0.113."+strconv.Itoa(i+1))
				})
			}
		})
	}
}

// assertDocumentDrawsOnItsBucket spends caller's bucket of two on path, a fetch
// and its revalidation, and holds the preflight after them to the refusal, and
// /health and the MCP endpoint, which the bucket does not charge, to their own
// answers.
func assertDocumentDrawsOnItsBucket(t *testing.T, base, path, caller string) {
	t.Helper()
	fetched := discoveryFetch(t, base, http.MethodGet, path, caller, nil)
	if fetched.status != http.StatusOK || fetched.header.Get("ETag") == "" {
		t.Fatalf("GET %s = %d with ETag %q, want the document and its validator", path, fetched.status, fetched.header.Get("ETag"))
	}
	revalidated := discoveryFetch(t, base, http.MethodGet, path, caller, map[string]string{"If-None-Match": fetched.header.Get("ETag")})
	if revalidated.status != http.StatusNotModified {
		t.Errorf("a revalidation of %s = %d, want 304 from the bucket's second request", path, revalidated.status)
	}
	preflight := discoveryFetch(t, base, http.MethodOptions, path, caller, map[string]string{
		"Origin": "https://mcp.example.test", "Access-Control-Request-Method": http.MethodGet,
	})
	if preflight.status != http.StatusTooManyRequests || preflight.header.Get("Retry-After") != "1" {
		t.Errorf("a preflight of %s after two requests = %d with Retry-After %q, want 429 and 1",
			path, preflight.status, preflight.header.Get("Retry-After"))
	}
	if health := discoveryFetch(t, base, http.MethodGet, "/health", caller, nil); health.status != http.StatusOK {
		t.Errorf("GET /health from the spent address = %d, want 200", health.status)
	}
	if endpoint := discoveryFetch(t, base, http.MethodPost, "/mcp", caller, nil); endpoint.status != http.StatusUnauthorized {
		t.Errorf("POST /mcp with no token from the spent address = %d, want the endpoint's own 401", endpoint.status)
	}
}

// TestServeHTTP_DiscoveryDocuments_ARequestRefusedForItsHostCostsNothing
// covers the other side of where the limiter sits: inside host validation, so
// a request naming a host the deployment never declared is refused 403 before
// it reaches the bucket. No proxy is trusted here, so every request is the
// loopback peer's and a forwarded Host is not believed: one undeclared Host
// at every document path, then the one request a bucket of one holds, served,
// and the next refused, which shows the bucket in force and the 403s charged
// to it not at all.
func TestServeHTTP_DiscoveryDocuments_ARequestRefusedForItsHostCostsNothing(t *testing.T) {
	standInServerCard(t)
	gitlab := newMockGitLabServer(t)
	cfg := discoveryServingConfig(gitlab.URL, config.AuthModeLegacy, 0.001, 1)
	cfg.TrustedProxyHeader, cfg.TrustedProxies = "", nil
	base := startDiscoveryTestServer(t, cfg)

	undeclared := map[string]string{"Host": "rebinding.example.test"}
	for _, path := range discoveryDocumentPaths(cfg) {
		t.Run(path, func(t *testing.T) {
			if got := discoveryFetch(t, base, http.MethodGet, path, "", undeclared); got.status != http.StatusForbidden {
				t.Errorf("GET %s naming an undeclared Host = %d, want host validation's 403", path, got.status)
			}
		})
	}
	got := []int{
		discoveryFetch(t, base, http.MethodGet, "/server-card", "", nil).status,
		discoveryFetch(t, base, http.MethodGet, "/server-card", "", nil).status,
	}
	if want := []int{http.StatusOK, http.StatusTooManyRequests}; !slices.Equal(got, want) {
		t.Errorf("two fetches after the refused Hosts = %v, want %v: the bucket's one request still in hand", got, want)
	}
}

// BenchmarkDiscoveryDocuments measures what one request for each discovery
// document costs where a deployment serves it, which is what the defaults of
// RTC-008 were sized from: the real handler chain behind serveHTTPOn in oauth
// mode, on the dynamic surface, with the real legacy card built before the
// clock starts, fetched by one client over one keep-alive connection on the
// loopback. A fetch, a revalidation answered 304, and the refusal itself are
// measured apart; body-B/op is the document's size on the wire. The figures
// include the client, which runs in the same process, so they bound what the
// server spends rather than isolate it.
//
//	go test ./cmd/server -run '^$' -bench BenchmarkDiscoveryDocuments -benchmem
func BenchmarkDiscoveryDocuments(b *testing.B) {
	// The startup and refusal lines would land between the results.
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.DiscardHandler))
	b.Cleanup(func() { slog.SetDefault(previous) })
	for _, bc := range []struct {
		name       string
		path       string
		revalidate bool
		refused    bool
	}{
		{name: "server-card", path: "/server-card"},
		{name: "server-card-304", path: "/server-card", revalidate: true},
		{name: "legacy-card", path: "/.well-known/mcp/server-card.json"},
		{name: "legacy-card-304", path: "/.well-known/mcp/server-card.json", revalidate: true},
		{name: "metadata", path: "/.well-known/oauth-protected-resource"},
		{name: "metadata-304", path: "/.well-known/oauth-protected-resource", revalidate: true},
		{name: "refused", path: "/server-card", refused: true},
	} {
		b.Run(bc.name, func(b *testing.B) {
			rps, burst := 0.0, 0
			if bc.refused {
				// A bucket of one that never refills: every request after the
				// first is refused.
				rps, burst = 1e-9, 1
			}
			cfg := &config.Config{
				GitLabURL:               "https://gitlab.example.invalid",
				MaxHTTPClients:          config.DefaultMaxHTTPClients,
				SessionTimeout:          config.DefaultSessionTimeout,
				ToolSurface:             config.ToolSurfaceDynamic,
				Stateless:               true,
				AuthMode:                config.AuthModeOAuth,
				PublicURL:               "https://mcp.example.test",
				OAuthCacheTTL:           config.DefaultOAuthCacheTTL,
				DiscoveryRateLimitRPS:   rps,
				DiscoveryRateLimitBurst: burst,
			}
			base := startDiscoveryBenchServer(b, cfg)
			client := &http.Client{Timeout: testCatalogBuildTimeout}
			first := benchFetch(b, client, base+bc.path, "")
			want, tag := http.StatusOK, ""
			switch {
			case bc.refused:
				want = http.StatusTooManyRequests
			case bc.revalidate:
				want, tag = http.StatusNotModified, first.etag
			}
			var bodyBytes int64
			requests := 0
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				got := benchFetch(b, client, base+bc.path, tag)
				if got.status != want {
					b.Fatalf("GET %s = %d, want %d", bc.path, got.status, want)
				}
				bodyBytes += got.bytes
				requests++
			}
			b.ReportMetric(float64(bodyBytes)/float64(requests), "body-B/op")
		})
	}
}

// benchFetched is what one benchmarked request came back with.
type benchFetched struct {
	status int
	bytes  int64
	etag   string
}

// benchFetch sends one GET to url, revalidating against tag when it is set,
// and reads the whole body, as a client does.
func benchFetch(b *testing.B, client *http.Client, url, tag string) benchFetched {
	b.Helper()
	req, err := http.NewRequestWithContext(b.Context(), http.MethodGet, url, nil)
	if err != nil {
		b.Fatalf("building GET %s: %v", url, err)
	}
	req.Host = "mcp.example.test"
	if tag != "" {
		req.Header.Set("If-None-Match", tag)
	}
	resp, err := client.Do(req)
	if err != nil {
		b.Fatalf("GET %s: %v", url, err)
	}
	read, err := io.Copy(io.Discard, resp.Body)
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		b.Fatalf("reading GET %s: %v", url, err)
	}
	return benchFetched{status: resp.StatusCode, bytes: read, etag: resp.Header.Get("ETag")}
}

// startDiscoveryBenchServer is startDiscoveryTestServer for a benchmark: the
// server, and the legacy card it builds on its first request, are both ready
// before the clock starts.
func startDiscoveryBenchServer(b *testing.B, cfg *config.Config) string {
	b.Helper()
	ctx, cancel := context.WithCancel(b.Context())
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		b.Fatalf("listen: %v", err)
	}
	base := "http://" + listener.Addr().String()
	errCh := make(chan error, 1)
	go func() { errCh <- serveHTTPOn(ctx, cfg, listener.Addr().String(), listener, defaultHTTPIdleTimeout) }()
	b.Cleanup(func() {
		cancel()
		<-errCh
	})
	client := &http.Client{Timeout: testCatalogBuildTimeout}
	deadline := time.Now().Add(testCatalogBuildTimeout)
	for {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
		if reqErr != nil {
			b.Fatalf("building the readiness probe: %v", reqErr)
		}
		if resp, doErr := client.Do(req); doErr == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			b.Fatal("the server never answered /health")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return base
}

// TestServeHTTP_DiscoveryDocuments_AZeroSettingServesEveryRequest covers the
// switch issue 1053 asked for: a deployment whose reverse proxy limits these
// paths already sets either setting to zero, and the binary then serves every
// request for a document, however many one address sends.
func TestServeHTTP_DiscoveryDocuments_AZeroSettingServesEveryRequest(t *testing.T) {
	standInServerCard(t)
	gitlab := newMockGitLabServer(t)
	for _, tc := range []struct {
		name  string
		rps   float64
		burst int
	}{
		{name: "a zero rate", rps: 0, burst: 1},
		{name: "a zero burst", rps: 1, burst: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := startDiscoveryTestServer(t, discoveryServingConfig(gitlab.URL, config.AuthModeLegacy, tc.rps, tc.burst))
			for range 5 {
				if got := discoveryFetch(t, base, http.MethodGet, "/server-card", "203.0.113.1", nil); got.status != http.StatusOK {
					t.Fatalf("GET /server-card with the limit off = %d, want 200 every time", got.status)
				}
			}
		})
	}
}
