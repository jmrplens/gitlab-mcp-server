// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// discovery_rate.go bounds how fast one client address may ask for the
// documents this server publishes without a credential (issue 1053, register
// row RTC-008): the SEP-2127 server card, the legacy enumerating card and, in
// oauth mode, the RFC 9728 protected resource metadata.
//
// The server-card extension says a server SHOULD rate-limit requests for its
// card, and CLAUDE.md asks the binary to be correct with no reverse proxy in
// front of it. Each answer is cheap once built, but a caller that never
// authenticates could ask for them without any bound, and the legacy card is
// a hundred and forty kilobytes on the dynamic surface.

package main

import (
	"container/list"
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// discoveryTrackedAddresses is how many client addresses the buckets are kept
// for at once. A new address past it takes the place of the one used least
// recently, which forgets a record against that address rather than taking a
// holding from it: the address starts again with a full bucket when it comes
// back, and one that keeps asking is never the least recent. A caller cycling
// more addresses than this finds each one forgotten when it returns, which
// neither evicting nor stopping the count at the bound prevents, and finding
// F-36 accepts.
const discoveryTrackedAddresses = tenancy.DiscoveryTrackedAddresses // register row RTC-008

// discoveryRetryAfter is what a refusal tells the caller to wait: the whole
// second in which a bucket refilling at the shipped rate has a request to give
// again. It is fixed rather than read off the bucket per request, which issue
// 1053 decided.
const discoveryRetryAfter = tenancy.DiscoveryRetryAfter // register row RTC-008

// discoveryRateLimiter is one token bucket per client address, shared by the
// three discovery documents, so that no one address can spend the process on
// documents anybody may fetch. The address is the one the authentication
// budgets charge ([clientIP], with the trusted-proxy rules, keyed by
// [budgetKey], an IPv6 address by its /64).
//
// No bucket the whole process shares stands beside it, which issue 1053
// decided and finding F-36 records: a shared one would let a flood from many
// addresses deny discovery to every caller, and the hosted client whose many
// users fetch the RFC 9728 document from one egress address after a 401
// challenge would be the first refused. Every request to a document path
// draws on it, a revalidation answered 304 and a CORS preflight among them,
// since what is bounded is the request and not the body.
type discoveryRateLimiter struct {
	limit rate.Limit
	burst int
	// capacity is how many addresses the table holds, discoveryTrackedAddresses
	// unless a test says otherwise.
	capacity int
	// paths are the request paths a document is mounted at; anything else
	// passes untouched.
	paths              map[string]bool
	trustedProxyHeader string
	trustedProxies     trustedProxies
	// now is the clock, replaced in tests.
	now func() time.Time

	mu sync.Mutex
	// buckets indexes recency by address, and recency holds the buckets most
	// recent first, so the one to forget at capacity is its last element.
	buckets map[string]*list.Element
	recency *list.List
}

// discoveryBucket is one address's bucket, as the recency list holds it.
type discoveryBucket struct {
	key    string
	tokens *rate.Limiter
}

// newDiscoveryRateLimiter builds the limiter a deployment asked for, or nil
// when a rate or a burst of zero turns it off, for a deployment whose reverse
// proxy already limits these paths. A nil limiter's middleware is its next
// handler, so nothing downstream asks whether one exists.
func newDiscoveryRateLimiter(cfg *config.Config) *discoveryRateLimiter {
	if cfg.DiscoveryRateLimitRPS <= 0 || cfg.DiscoveryRateLimitBurst <= 0 {
		return nil
	}
	paths := map[string]bool{}
	for _, path := range discoveryDocumentPaths(cfg) {
		paths[path] = true
	}
	return &discoveryRateLimiter{
		limit:              rate.Limit(cfg.DiscoveryRateLimitRPS),
		burst:              cfg.DiscoveryRateLimitBurst,
		capacity:           discoveryTrackedAddresses,
		paths:              paths,
		trustedProxyHeader: cfg.TrustedProxyHeader,
		trustedProxies:     trustedProxiesOf(cfg.TrustedProxies),
		now:                time.Now,
		buckets:            map[string]*list.Element{},
		recency:            list.New(),
	}
}

// discoveryRateFor builds the deployment's limiter and starts its sweep, on
// the interval the authentication tables are swept on (AUB-005), since a
// bucket that has refilled is one a fresh address would get anyway. A
// deployment that turned it off gets nil and no sweep: a loop over a table
// nobody fills would be a goroutine for the life of the process.
func discoveryRateFor(ctx context.Context, cfg *config.Config) *discoveryRateLimiter {
	limiter := newDiscoveryRateLimiter(cfg)
	if limiter != nil {
		startPeriodicCleanup(ctx, limiter.cleanup)
	}
	return limiter
}

// discoveryDocumentPaths are the paths a deployment serves a discovery
// document at: the SEP-2127 card at the root and after /mcp, the legacy card,
// each also under --public-url's path prefix, and in oauth mode the RFC 9728
// metadata at the one path RFC 9728 derives from the resource identifier,
// which carries the prefix itself. They are the paths the mux mounts the
// documents at, read from the same helpers, so a document mounted somewhere
// new is counted there too. /health is not among them, nor is the MCP
// endpoint.
func discoveryDocumentPaths(cfg *config.Config) []string {
	paths := append(discoveryCardPaths(cfg), legacyCardPaths(cfg)...)
	if cfg.AuthMode == config.AuthModeOAuth {
		paths = append(paths, oauth.MetadataPathFor(cfg.PublicURL))
	}
	return paths
}

// discoveryCardPaths are the paths the SEP-2127 card is mounted at.
func discoveryCardPaths(cfg *config.Config) []string {
	return publicPaths(cfg, serverCardPath, mcpEndpointPath+serverCardPath)
}

// legacyCardPaths are the paths the enumerating SEP-1649 card is mounted at.
func legacyCardPaths(cfg *config.Config) []string {
	return publicPaths(cfg, serverCardLegacyPath)
}

// middleware charges every request to a document path to its address's bucket
// and refuses one the bucket cannot pay for, before anything answers it. It
// sits outside the CORS layer, which answers a trusted origin's preflight on
// its own, so a preflight is charged like the request it precedes.
func (d *discoveryRateLimiter) middleware(next http.Handler) http.Handler {
	if d == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !d.paths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		if !d.allow(budgetKey(clientIP(r, d.trustedProxyHeader, d.trustedProxies))) {
			// The line names the limit and not the address: a flood from many
			// addresses writes it once a window, and telemetry carries the
			// reason, never the caller (INV-013).
			refusalLog.log(r.Context(), slog.LevelWarn, "request refused: too many requests for the discovery documents from one address",
				"limit_rps", float64(d.limit), "burst", d.burst)
			discoveryRateFailure().write(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allow takes one request from key's bucket, reporting whether it had one.
// An address the table does not hold gets a full bucket, taking the place of
// the least recent one when the table is full.
func (d *discoveryRateLimiter) allow(key string) bool {
	now := d.now()
	d.mu.Lock()
	defer d.mu.Unlock()
	if element, tracked := d.buckets[key]; tracked {
		d.recency.MoveToFront(element)
		return bucketOf(element).tokens.AllowN(now, 1)
	}
	if d.recency.Len() >= d.capacity {
		d.forgetLocked(d.recency.Back())
	}
	bucket := &discoveryBucket{key: key, tokens: rate.NewLimiter(d.limit, d.burst)}
	d.buckets[key] = d.recency.PushFront(bucket)
	return bucket.tokens.AllowN(now, 1)
}

// cleanup forgets every bucket that has refilled, which a fresh one would
// equal, so a table a flood filled gives its memory back once the flood stops.
// It runs on the sweep the authentication tables run on (AUB-005).
func (d *discoveryRateLimiter) cleanup() {
	now := d.now()
	full := float64(d.burst)
	d.mu.Lock()
	defer d.mu.Unlock()
	for element := d.recency.Front(); element != nil; {
		older := element.Next()
		if bucketOf(element).tokens.TokensAt(now) >= full {
			d.forgetLocked(element)
		}
		element = older
	}
}

// forgetLocked drops one bucket from the index and the recency list. The
// caller holds d.mu and passes an element of this limiter's list.
func (d *discoveryRateLimiter) forgetLocked(element *list.Element) {
	d.recency.Remove(element)
	delete(d.buckets, bucketOf(element).key)
}

// bucketOf is the bucket an element of the recency list holds.
func bucketOf(element *list.Element) *discoveryBucket {
	bucket, _ := element.Value.(*discoveryBucket)
	return bucket
}

// discoveryRateFailure is the refusal of a request its address's bucket could
// not pay for: the gate's 429, in the shape every gate refusal takes, readable
// from any origin as the documents themselves are, with its Retry-After
// exposed to a script that wants to honor it.
func discoveryRateFailure() *gateFailure {
	return &gateFailure{
		status:  http.StatusTooManyRequests,
		code:    errCodeTooManyRequests,
		message: "Too many requests for the discovery documents from this address. Retry later.",
		header: newHeader(
			headerRetryAfter, strconv.Itoa(int(discoveryRetryAfter.Seconds())),
			headerAllowOrigin, "*",
			headerExposeHeaders, headerRetryAfter,
		),
	}
}

// validateDiscoveryRateBounds holds the two flags to the bounds their
// variables are held to. HTTP mode does not run (*config.Config).validate, so
// a bound enforced only in the configuration package would be one the flags
// escape. A negative value and one that is not a number are refused rather
// than read as off, because zero already says that.
func validateDiscoveryRateBounds(cfg *config.Config) error {
	rps := cfg.DiscoveryRateLimitRPS
	if math.IsNaN(rps) || rps < 0 {
		return fmt.Errorf("--discovery-rate-limit-rps must be a number of zero or more, got %g (0 disables the limit)", rps)
	}
	if rps > config.MaxDiscoveryRateLimitRPS {
		return fmt.Errorf("--discovery-rate-limit-rps %g exceeds maximum of %d", rps, config.MaxDiscoveryRateLimitRPS)
	}
	burst := cfg.DiscoveryRateLimitBurst
	if burst < 0 {
		return fmt.Errorf("--discovery-rate-limit-burst must not be negative, got %d (0 disables the limit)", burst)
	}
	if burst > config.MaxDiscoveryRateLimitBurst {
		return fmt.Errorf("--discovery-rate-limit-burst %d exceeds maximum of %d", burst, config.MaxDiscoveryRateLimitBurst)
	}
	return nil
}
