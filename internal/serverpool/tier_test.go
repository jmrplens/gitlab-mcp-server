// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package serverpool

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// gitLabDotComURL is the instance a test names to be served as GitLab.com.
// Plain HTTP, so a client hands the proxy TestMain names each request as it is
// rather than asking it for a tunnel.
const gitLabDotComURL = "http://" + gitlabclient.GitLabDotComHost

// gitLabDotComHandler answers the requests sent to the GitLab.com host for the
// test that installed it ([standInForGitLabDotCom]), and is nil between tests.
var gitLabDotComHandler atomic.Pointer[http.Handler]

// serveAsGitLabDotCom is what TestMain routes every request to the GitLab.com
// host to: the installed handler, and a 502 for a request when none is
// installed or for another host, so a request nothing expected fails loudly.
//
// It exists because the tier probe asks its membership step of GitLab.com
// alone ([gitlabclient.Client.IsGitLabDotCom] reads the client's host), and a
// test server cannot have that host. Routing the host through a proxy is the
// one way this package can have the real step asked by the real client the
// pool builds, without a hook in production code.
func serveAsGitLabDotCom(w http.ResponseWriter, r *http.Request) {
	handler := gitLabDotComHandler.Load()
	if handler == nil || r.URL.Host != gitlabclient.GitLabDotComHost {
		http.Error(w, "no test stands in for this host", http.StatusBadGateway)
		return
	}
	(*handler).ServeHTTP(w, r)
}

// routeGitLabDotComTo makes proxyURL the proxy of every request this test
// binary sends to the GitLab.com host, and leaves every other request to the
// proxy the environment names, as before.
//
// The route is the default transport's proxy function rather than the
// environment's variables: net/http reads those once per process, at the first
// request any package makes, which may come before TestMain runs, while every
// transport the client package builds is a clone of the default transport,
// taken when the first client is built. It runs in TestMain for that reason,
// before any test builds one.
func routeGitLabDotComTo(proxyURL string) {
	route, err := url.Parse(proxyURL)
	if err != nil {
		panic(err)
	}
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		panic("the default transport is not an *http.Transport")
	}
	environment := transport.Proxy
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		if req.URL.Hostname() == gitlabclient.GitLabDotComHost {
			return route, nil
		}
		return environment(req)
	}
}

// standInForGitLabDotCom makes handler answer what this test sends to the
// GitLab.com host.
func standInForGitLabDotCom(t *testing.T, handler http.Handler) {
	t.Helper()
	gitLabDotComHandler.Store(&handler)
	t.Cleanup(func() { gitLabDotComHandler.Store(nil) })
}

// dotComTierGitLab answers the tier probe as GitLab.com answers a classic
// token that administers only free namespaces and works in a group whose
// top-level group's plan carries Ultimate's features, the deployment issue
// 1782 records: the license refused, as GitLab.com refuses it to everyone,
// the namespaces on the free plan, and the membership page hanging until the
// client gives up, or, once healthy, naming that group.
type dotComTierGitLab struct {
	healthy atomic.Bool
	// probes counts the credential probes, the GET /user a build sends and a
	// re-detection does not.
	probes atomic.Int64
	// memberships counts the membership pages asked.
	memberships atomic.Int64
}

// ultimateMembershipPage is a membership page naming one top-level group whose
// plan carries the Premium and the Ultimate feature.
const ultimateMembershipPage = `{"data":{"groups":{"nodes":[{"fullPath":"acme",` +
	`"premium":{"available":true},"ultimate":{"available":true}}],` +
	`"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`

// newDotComTierGitLab installs the stand-in. A membership page still hanging
// when the test ends is let go then, so nothing outlives it.
func newDotComTierGitLab(t *testing.T) *dotComTierGitLab {
	t.Helper()
	g := &dotComTierGitLab{}
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		g.probes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"builder"}`))
	})
	mux.HandleFunc("GET /api/v4/license", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
	})
	mux.HandleFunc("GET /api/v4/namespaces", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"full_path":"builder","kind":"user","plan":"free"}]`))
	})
	mux.HandleFunc("POST /api/graphql", func(w http.ResponseWriter, r *http.Request) {
		g.memberships.Add(1)
		if !g.healthy.Load() {
			select {
			case <-r.Context().Done():
			case <-done:
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(ultimateMembershipPage))
	})
	standInForGitLabDotCom(t, mux)
	return g
}

// tierGitLab stands in for a self-managed instance whose answers to the tier
// probe a test changes between an entry's build and its re-detection: the
// license read and the plan of the one namespace the token administers. The
// credential probe accepts every token, and the token's own description goes
// unanswered until a test names its scopes.
type tierGitLab struct {
	url string

	mu            sync.Mutex
	license       func(w http.ResponseWriter)
	namespacePlan string

	selfScopes   atomic.Pointer[[]string]
	licenseReads atomic.Int64
	probes       atomic.Int64
}

// newTierGitLab starts the stand-in answering license and namespacePlan.
func newTierGitLab(t *testing.T, license func(w http.ResponseWriter), namespacePlan string) *tierGitLab {
	t.Helper()
	g := &tierGitLab{license: license, namespacePlan: namespacePlan}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		g.probes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"builder"}`))
	})
	mux.HandleFunc("GET /api/v4/license", func(w http.ResponseWriter, _ *http.Request) {
		g.licenseReads.Add(1)
		g.mu.Lock()
		answer := g.license
		g.mu.Unlock()
		answer(w)
	})
	mux.HandleFunc("GET /api/v4/namespaces", func(w http.ResponseWriter, _ *http.Request) {
		g.mu.Lock()
		plan := g.namespacePlan
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "full_path": "builder", "kind": "user", "plan": plan}})
	})
	mux.HandleFunc("GET /api/v4/personal_access_tokens/self", func(w http.ResponseWriter, _ *http.Request) {
		scopes := g.selfScopes.Load()
		if scopes == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "scopes": *scopes, "active": true})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	g.url = srv.URL
	return g
}

// set changes what the stand-in answers from the next request on.
func (g *tierGitLab) set(license func(w http.ResponseWriter), namespacePlan string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.license, g.namespacePlan = license, namespacePlan
}

// shapeFactory is a pool factory that records the tier of every shape it is
// asked for, and fails or hands back no server when a test says so.
type shapeFactory struct {
	mu       sync.Mutex
	tiers    []edition.Tier
	fail     error
	noServer bool
}

// breaks makes every later request fail with err, or hand back no server when
// err is nil.
func (f *shapeFactory) breaks(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail, f.noServer = err, err == nil
}

// build is the factory.
func (f *shapeFactory) build(_ *gitlabclient.Client, cfg *config.ServerConfig) (*mcp.Server, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tiers = append(f.tiers, cfg.Tier)
	if f.fail != nil {
		return nil, f.fail
	}
	if f.noServer {
		return nil, nil //nolint:nilnil // the case a test asks for: success reported with nothing built
	}
	return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
}

// asked returns the tiers the factory was asked for so far.
func (f *shapeFactory) asked() []edition.Tier {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.tiers)
}

// evictionLog records what the eviction callback is handed.
type evictionLog struct {
	mu      sync.Mutex
	entries []*Entry
	causes  []EvictionCause
}

// record is the eviction callback.
func (l *evictionLog) record(entry *Entry, cause EvictionCause) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
	l.causes = append(l.causes, cause)
}

// seen returns the entries and causes recorded so far.
func (l *evictionLog) seen() ([]*Entry, []EvictionCause) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.entries), slices.Clone(l.causes)
}

// tierPool is a pool against g that detects the tier, with factory f and the
// eviction callback writing to evicted.
func tierPool(t *testing.T, g *tierGitLab, f *shapeFactory, evicted *evictionLog, options ...Option) *ServerPool {
	t.Helper()
	cfg := testConfig(g.url)
	cfg.TierExplicit = false
	pool := New(cfg, f.build, append([]Option{WithOnEvict(evicted.record)}, options...)...)
	t.Cleanup(pool.Close)
	return pool
}

// buildTierEntry builds token's entry in pool against g and returns it with
// its key.
func buildTierEntry(t *testing.T, pool *ServerPool, g *tierGitLab, token string) (*Entry, string) {
	t.Helper()
	entry, err := pool.GetOrCreateEntry(token, g.url, nil)
	if err != nil {
		t.Fatalf("GetOrCreateEntry(): %v", err)
	}
	return entry, sessionKey(token, g.url)
}

// makeTierDue makes entry's provisional tier due now.
func makeTierDue(t *testing.T, pool *ServerPool, entry *Entry) {
	t.Helper()
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if entry.provisional == nil {
		t.Fatal("the entry carries no provisional tier to make due")
	}
	entry.provisional.due = time.Now()
}

// pooledUnder returns the entry pooled under key, nil when there is none.
func pooledUnder(pool *ServerPool, key string) *Entry {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	return pool.entries[key]
}

// markOf returns a copy of entry's provisional mark, read under the pool's lock,
// and whether it carries one.
func markOf(pool *ServerPool, entry *Entry) (provisionalTier, bool) {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	if entry.provisional == nil {
		return provisionalTier{}, false
	}
	return *entry.provisional, true
}

// TestRedetectDueTiers_TheProductionCase_IsServedUltimateWithoutARestart is the
// case issue 1782 reports, through the pool and the client it builds: an entry
// built on GitLab.com in OAuth mode while the membership query hung until the
// build's clock ran out was served the Free catalog until the replica was
// redeployed. Now the entry is built Free and provisional, naming the
// membership step; once GitLab.com answers again, one tick of the sweep asks
// the tier again, finds Ultimate, and serves the credential on the Ultimate
// shape, through the eviction callback with tier_resolved, keeping the client
// and the credential's age, under a new owner, with no restart, no --tier and
// no second credential probe.
//
// The pool measures the membership step's own clock from its lifetime, so a
// lifetime that ends two seconds in stands in for the eight seconds that clock
// gives the step: the query hangs until the clock runs out and fails with the
// context deadline exceeded the production log carried. The identity read
// after it fails with it, which this test does not look at.
func TestRedetectDueTiers_TheProductionCase_IsServedUltimateWithoutARestart(t *testing.T) {
	g := newDotComTierGitLab(t)
	captured := captureLogs(t)
	lifetime, end := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(end)
	cfg := testConfig(gitLabDotComURL)
	cfg.TierExplicit, cfg.AuthMode = false, config.AuthModeOAuth
	factory := &shapeFactory{}
	evicted := &evictionLog{}
	pool := New(cfg, factory.build,
		WithBaseContext(func() context.Context { return lifetime }),
		WithOnEvict(evicted.record))
	t.Cleanup(pool.Close)
	known := &gitlabclient.TokenFacts{Scopes: []string{"api"}}

	built, err := pool.GetOrCreateEntryWithFacts("glpat-production-case", gitLabDotComURL, known)
	if err != nil {
		t.Fatalf("GetOrCreateEntryWithFacts(): %v", err)
	}
	if built.Config().Tier != edition.Free || g.memberships.Load() == 0 {
		t.Fatalf("built on %v after %d membership pages, want Free with the membership asked", built.Config().Tier, g.memberships.Load())
	}
	assertProvisionalMark(t, pool, built, []string{"membership"})
	assertCreatedTierLine(t, captured, "detected_provisional", edition.Free, []string{"membership"})
	probes := g.probes.Load()

	g.healthy.Store(true)
	makeTierDue(t, pool, built)
	pool.redetectDueTiers(t.Context())

	served, err := pool.GetOrCreateEntryWithFacts("glpat-production-case", gitLabDotComURL, known)
	if err != nil {
		t.Fatalf("GetOrCreateEntryWithFacts() after the tick: %v", err)
	}
	if served == built || served.Config().Tier != edition.Ultimate || served.Client().Tier() != edition.Ultimate {
		t.Fatalf("after one tick the credential is served on %v (client %v), replaced %t; want Ultimate on a new entry",
			served.Config().Tier, served.Client().Tier(), served != built)
	}
	assertProvisionalMark(t, pool, served, nil)
	assertReplacedKeepingTheCredential(t, pool, built, served)
	if g.probes.Load() != probes || pool.Stats().Misses != 1 {
		t.Errorf("credential probes %d after the tick, misses %d; want %d and 1: nothing was rebuilt", g.probes.Load(), pool.Stats().Misses, probes)
	}
	entries, causes := evicted.seen()
	if !slices.Equal(causes, []EvictionCause{CauseTierResolved}) || entries[0] != built {
		t.Errorf("eviction callback handed %v, want the built entry with tier_resolved", causes)
	}
	if got := factory.asked(); !slices.Equal(got, []edition.Tier{edition.Free, edition.Ultimate}) {
		t.Errorf("shapes asked for %v, want Free at the build and Ultimate at the tick", got)
	}
	if stats := pool.Stats(); stats.TierResolvedEvictions != 1 || stats.TierRedetections != 1 || stats.Evictions != 0 {
		t.Errorf("snapshot %+v, want one re-detection and one tier_resolved, outside the legacy total", stats)
	}
	assertMovedLine(t, captured, "free", "ultimate", "detected")
}

// assertReplacedKeepingTheCredential holds a replacement to what it keeps of
// the entry it replaced: the client and the credential's age, under an owner
// of its own.
func assertReplacedKeepingTheCredential(t *testing.T, pool *ServerPool, old, next *Entry) {
	t.Helper()
	if next.Client() != old.Client() || next.Owner() == old.Owner() {
		t.Error("the replacement does not keep the client under a new owner")
	}
	if lastValidatedOf(pool, next) != lastValidatedOf(pool, old) {
		t.Error("the replacement restarted the credential's age, though no credential check was made")
	}
}

// assertMovedLine holds the line a replacement writes to the tiers it names and
// how the new one was selected.
func assertMovedLine(t *testing.T, captured *capturedRecords, from, to, source string) {
	t.Helper()
	record, found := captured.find("server pool: moved an entry to the tier its re-detection resolved")
	if !found {
		t.Fatal("the replacement was not logged")
	}
	got := func(key string) string {
		value, _ := logAttr(record, key)
		return value.String()
	}
	if got("tier_from") != from || got("tier_to") != to || got("tier_source") != source {
		t.Errorf("the line names %s to %s, %s; want %s to %s, %s", got("tier_from"), got("tier_to"), got("tier_source"), from, to, source)
	}
}

// TestSettleTier_ActsOnTheAnswerByTheAsymmetry covers the decision a
// re-detection's answer is put to (register row AUT-010): a higher tier, or
// another one GitLab answered whole, replaces the entry on that tier's shape;
// the same tier answered whole clears the mark; and a provisional answer no
// higher than the entry's keeps it and asks again later, since a step that
// fails again proves nothing lower.
func TestSettleTier_ActsOnTheAnswerByTheAsymmetry(t *testing.T) {
	free, premium := licenseAnswer(http.StatusBadGateway, ""), "premium"
	cases := []struct {
		name          string
		builtPlan     string
		got           gitlabclient.TierResolution
		wantReplaced  bool
		wantTier      edition.Tier
		wantMarked    bool
		wantAsked     int
		wantWait      time.Duration
		wantClientNow edition.Tier
	}{
		{
			name:         "a higher tier answered whole replaces it and clears the mark",
			got:          gitlabclient.TierResolution{Tier: edition.Ultimate, Answered: true},
			wantReplaced: true, wantTier: edition.Ultimate, wantClientNow: edition.Ultimate,
		},
		{
			name:         "a higher tier still provisional replaces it and asks again",
			got:          gitlabclient.TierResolution{Tier: edition.Premium, Answered: true, Provisional: true, Unresolved: []string{"license"}},
			wantReplaced: true, wantTier: edition.Premium, wantMarked: true, wantAsked: 1, wantWait: 2 * time.Minute,
			wantClientNow: edition.Premium,
		},
		{
			name:     "the same tier answered whole clears the mark",
			got:      gitlabclient.TierResolution{Tier: edition.Free},
			wantTier: edition.Free, wantClientNow: edition.Free,
		},
		{
			name:     "the same tier still provisional keeps it and asks again later",
			got:      gitlabclient.TierResolution{Tier: edition.Free, Provisional: true, Unresolved: []string{"license"}},
			wantTier: edition.Free, wantMarked: true, wantAsked: 1, wantWait: 2 * time.Minute, wantClientNow: edition.Free,
		},
		{
			name: "a lower tier still provisional never lowers it", builtPlan: premium,
			got:      gitlabclient.TierResolution{Tier: edition.Free, Provisional: true, Unresolved: []string{"license"}},
			wantTier: edition.Premium, wantMarked: true, wantAsked: 1, wantWait: 2 * time.Minute, wantClientNow: edition.Premium,
		},
		{
			name: "a lower tier answered whole replaces it: the plan was read", builtPlan: premium,
			got:          gitlabclient.TierResolution{Tier: edition.Free, Answered: true},
			wantReplaced: true, wantTier: edition.Free, wantClientNow: edition.Free,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newTierGitLab(t, free, tc.builtPlan)
			evicted := &evictionLog{}
			pool := tierPool(t, g, &shapeFactory{}, evicted)
			entry, key := buildTierEntry(t, pool, g, "glpat-settle")
			assertProvisionalMark(t, pool, entry, []string{"license"})

			pool.settleTier(t.Context(), key, entry, tc.got)

			current := pooledUnder(pool, key)
			if (current != entry) != tc.wantReplaced {
				t.Fatalf("replaced = %t, want %t", current != entry, tc.wantReplaced)
			}
			_, causes := evicted.seen()
			if tc.wantReplaced != slices.Equal(causes, []EvictionCause{CauseTierResolved}) {
				t.Errorf("eviction causes %v with replaced %t", causes, tc.wantReplaced)
			}
			if current.Config().Tier != tc.wantTier || current.Client().Tier() != tc.wantClientNow {
				t.Errorf("served on %v, client on %v; want %v and %v", current.Config().Tier, current.Client().Tier(), tc.wantTier, tc.wantClientNow)
			}
			mark, marked := markOf(pool, current)
			if marked != tc.wantMarked || mark.asked != tc.wantAsked || mark.wait != tc.wantWait {
				t.Errorf("mark %+v (marked %t), want marked %t asked %d waiting %v", mark, marked, tc.wantMarked, tc.wantAsked, tc.wantWait)
			}
		})
	}
}

// TestSettleTier_AnEntryNoLongerPooled_IsLeftAlone checks that an answer that
// arrives once its entry has been dropped neither clears the entry's mark nor
// schedules another ask: the entry is gone, and the pool's next entry for the
// credential carries a mark of its own if its tier needs one.
func TestSettleTier_AnEntryNoLongerPooled_IsLeftAlone(t *testing.T) {
	answers := []gitlabclient.TierResolution{
		{Tier: edition.Free},
		{Tier: edition.Free, Provisional: true, Unresolved: []string{"license"}},
	}
	for _, got := range answers {
		t.Run("provisional="+strconv.FormatBool(got.Provisional), func(t *testing.T) {
			g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
			pool := tierPool(t, g, &shapeFactory{}, &evictionLog{})
			entry, key := buildTierEntry(t, pool, g, "glpat-dropped")
			before, _ := markOf(pool, entry)
			pool.evictByKey(key)

			pool.settleTier(t.Context(), key, entry, got)

			after, marked := markOf(pool, entry)
			if !marked || after.asked != 0 || !after.due.Equal(before.due) {
				t.Errorf("the dropped entry's mark moved to %+v (marked %t), want it left as %+v", after, marked, before)
			}
		})
	}
}

// TestRedetectDueTiers_AnAnswerStillProvisional_BacksOffToTheCeiling checks the
// waits between the asks of a tier GitLab keeps failing to answer: one minute
// after the build, then two, four, eight, and every fifteen after that, each
// ask sending one probe.
func TestRedetectDueTiers_AnAnswerStillProvisional_BacksOffToTheCeiling(t *testing.T) {
	g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
	pool := tierPool(t, g, &shapeFactory{}, &evictionLog{})
	entry, _ := buildTierEntry(t, pool, g, "glpat-backoff")

	first, _ := markOf(pool, entry)
	waits := []time.Duration{first.wait}
	// sequential: each tick asks the answer the previous one left provisional
	for range 5 {
		makeTierDue(t, pool, entry)
		pool.redetectDueTiers(t.Context())
		mark, marked := markOf(pool, entry)
		if !marked {
			t.Fatal("the mark was cleared by an answer still provisional")
		}
		waits = append(waits, mark.wait)
	}
	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute}
	if !slices.Equal(waits, want) {
		t.Errorf("waits %v, want %v", waits, want)
	}
	if mark, _ := markOf(pool, entry); mark.asked != 5 || time.Until(mark.due) <= 14*time.Minute {
		t.Errorf("after five asks the mark is %+v, want five asks and the next one fifteen minutes away", mark)
	}
	if got := g.licenseReads.Load(); got != 6 {
		t.Errorf("license reads %d, want 6: the build's and one per ask", got)
	}
	if got := pool.Stats().TierRedetections; got != 5 {
		t.Errorf("TierRedetections = %d, want 5", got)
	}
}

// TestRedetectDueTiers_TheSameTierAnsweredWhole_IsNotAskedAgain checks that a
// re-detection that reads the entry's tier whole clears the mark, says so at
// INFO with the asks it took, and that the sweep asks nothing more of it.
func TestRedetectDueTiers_TheSameTierAnsweredWhole_IsNotAskedAgain(t *testing.T) {
	g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
	evicted := &evictionLog{}
	pool := tierPool(t, g, &shapeFactory{}, evicted)
	entry, key := buildTierEntry(t, pool, g, "glpat-same-tier")
	captured := captureLogs(t)

	g.set(licenseAnswer(http.StatusForbidden, ""), "")
	makeTierDue(t, pool, entry)
	pool.redetectDueTiers(t.Context())
	reads := g.licenseReads.Load()
	pool.redetectDueTiers(t.Context())

	if pooledUnder(pool, key) != entry {
		t.Error("an entry whose tier was answered the same was replaced")
	}
	assertProvisionalMark(t, pool, entry, nil)
	if g.licenseReads.Load() != reads {
		t.Error("a tier answered whole was asked again on the next tick")
	}
	if _, causes := evicted.seen(); len(causes) != 0 {
		t.Errorf("eviction callback handed %v, want nothing", causes)
	}
	record, found := captured.find("server pool: a provisional tier was answered whole and is kept")
	if !found {
		t.Fatal("clearing the mark was not logged")
	}
	if asked, _ := logAttr(record, "asked"); asked.Int64() != 1 {
		t.Errorf("asked = %d, want 1", asked.Int64())
	}
}

// TestRedetectDueTiers_AnEntryInUse_IsStillResolved checks that the protection
// WithInUse gives an entry with an open subscription keeps it from eviction
// for idleness and size pressure and not from having its tier resolved: an
// entry serving nothing but a listen is the one whose tier no request asked
// again before.
func TestRedetectDueTiers_AnEntryInUse_IsStillResolved(t *testing.T) {
	g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
	pool := tierPool(t, g, &shapeFactory{}, &evictionLog{}, WithInUse(func(*Entry) bool { return true }))
	entry, key := buildTierEntry(t, pool, g, "glpat-in-use")

	g.set(licenseAnswer(http.StatusOK, "ultimate"), "")
	makeTierDue(t, pool, entry)
	pool.redetectDueTiers(t.Context())

	if current := pooledUnder(pool, key); current == entry || current.Config().Tier != edition.Ultimate {
		t.Error("an entry in use kept its provisional tier when GitLab answered Ultimate")
	}
}

// TestRedetectDueTiers_AsksOnlyWhatIsDueAndOnlyWithASlot covers what holds an
// ask back: an entry whose wait is not over, one whose tier is not provisional,
// a sweep whose context has ended, and no credential probe slot free, which
// leaves the entry due for the next tick.
func TestRedetectDueTiers_AsksOnlyWhatIsDueAndOnlyWithASlot(t *testing.T) {
	t.Run("the wait is not over", func(t *testing.T) {
		g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
		pool := tierPool(t, g, &shapeFactory{}, &evictionLog{})
		buildTierEntry(t, pool, g, "glpat-not-due")
		pool.redetectDueTiers(t.Context())
		if g.licenseReads.Load() != 1 {
			t.Errorf("license reads %d, want the build's alone", g.licenseReads.Load())
		}
	})

	t.Run("a tier read whole", func(t *testing.T) {
		g := newTierGitLab(t, licenseAnswer(http.StatusOK, "premium"), "")
		pool := tierPool(t, g, &shapeFactory{}, &evictionLog{})
		buildTierEntry(t, pool, g, "glpat-whole")
		if due := pool.dueTiers(time.Now().Add(time.Hour)); len(due) != 0 {
			t.Errorf("dueTiers() = %v for a tier read whole", due)
		}
	})

	t.Run("the wait ends at its due time", func(t *testing.T) {
		g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
		pool := tierPool(t, g, &shapeFactory{}, &evictionLog{})
		entry, _ := buildTierEntry(t, pool, g, "glpat-edge")
		mark, _ := markOf(pool, entry)
		if due := pool.dueTiers(mark.due.Add(-time.Nanosecond)); len(due) != 0 {
			t.Error("an entry was due a nanosecond before its time")
		}
		if due := pool.dueTiers(mark.due); len(due) != 1 || due[0].entry != entry {
			t.Errorf("dueTiers() at the due time = %v, want the entry", due)
		}
	})

	t.Run("a sweep whose context ended", func(t *testing.T) {
		g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
		pool := tierPool(t, g, &shapeFactory{}, &evictionLog{})
		entry, _ := buildTierEntry(t, pool, g, "glpat-cancelled")
		makeTierDue(t, pool, entry)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		pool.redetectDueTiers(ctx)
		if g.licenseReads.Load() != 1 || pool.Stats().TierRedetections != 0 {
			t.Error("a sweep whose context had ended asked GitLab")
		}
	})

	t.Run("no probe slot free", func(t *testing.T) {
		g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
		pool := tierPool(t, g, &shapeFactory{}, &evictionLog{})
		entry, key := buildTierEntry(t, pool, g, "glpat-no-slot")
		g.set(licenseAnswer(http.StatusOK, "ultimate"), "")
		makeTierDue(t, pool, entry)
		for range cap(pool.probes) {
			pool.probes <- struct{}{}
		}
		pool.redetectDueTiers(t.Context())
		if g.licenseReads.Load() != 1 {
			t.Fatal("a re-detection ran with every probe slot taken")
		}
		if due := pool.dueTiers(time.Now()); len(due) != 1 {
			t.Fatal("the entry that found no slot is no longer due for the next tick")
		}
		for range cap(pool.probes) {
			<-pool.probes
		}
		pool.redetectDueTiers(t.Context())
		if current := pooledUnder(pool, key); current.Config().Tier != edition.Ultimate {
			t.Error("the next tick, with slots free, did not resolve the tier")
		}
		if len(pool.probes) != 0 {
			t.Errorf("%d probe slots still taken after the ask", len(pool.probes))
		}
	})
}

// TestRedetectTier_AnEntryNoLongerPooled_IsNotAsked checks that an entry
// dropped or replaced between the tick finding it and its ask is left alone:
// nothing is sent and nothing is counted.
func TestRedetectTier_AnEntryNoLongerPooled_IsNotAsked(t *testing.T) {
	g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
	pool := tierPool(t, g, &shapeFactory{}, &evictionLog{})
	entry, key := buildTierEntry(t, pool, g, "glpat-gone")
	pool.evictByKey(key)

	pool.redetectTier(t.Context(), key, entry)

	if g.licenseReads.Load() != 1 || pool.Stats().TierRedetections != 0 {
		t.Error("a re-detection asked about an entry the pool no longer holds")
	}
}

// TestReplaceTier_KeepsTheOldEntryWhenTheNewShapeCannotTakeIt covers the
// replacements that do not happen: the factory failing, the factory handing
// back no server, and the insert callback refusing the replacement, each of
// which keeps the old entry, tells the eviction callback nothing and asks
// again later; and an entry no longer pooled, or one whose credential GitLab
// has refused, which is left to the path that is ending it.
func TestReplaceTier_KeepsTheOldEntryWhenTheNewShapeCannotTakeIt(t *testing.T) {
	ultimate := gitlabclient.TierResolution{Tier: edition.Ultimate, Answered: true}
	const (
		factoryFailed = "server pool: could not build the shape a provisional tier resolved to; keeping the entry and asking again"
		insertRefused = "server pool: the shape a provisional tier resolved to could not take the entry; keeping it and asking again"
	)
	cases := []struct {
		name string
		// refuseInsert has the insert callback refuse every entry after the
		// first, which is the one the build inserts.
		refuseInsert bool
		// prepare runs once the entry is built, against the factory, the pool
		// and the entry.
		prepare   func(f *shapeFactory, pool *ServerPool, key string, entry *Entry)
		wantAsked int
		wantLog   string
	}{
		{
			name: "the factory fails",
			prepare: func(f *shapeFactory, _ *ServerPool, _ string, _ *Entry) {
				f.breaks(errors.New("the shape could not be built"))
			},
			wantAsked: 1, wantLog: factoryFailed,
		},
		{
			name:      "the factory hands back no server",
			prepare:   func(f *shapeFactory, _ *ServerPool, _ string, _ *Entry) { f.breaks(nil) },
			wantAsked: 1, wantLog: factoryFailed,
		},
		{
			name: "the insert callback refuses the replacement", refuseInsert: true,
			wantAsked: 1, wantLog: insertRefused,
		},
		{
			name:    "the entry was dropped meanwhile",
			prepare: func(_ *shapeFactory, pool *ServerPool, key string, _ *Entry) { pool.evictByKey(key) },
		},
		{
			name:    "GitLab refused the entry's credential",
			prepare: func(_ *shapeFactory, _ *ServerPool, _ string, entry *Entry) { entry.rejected.Store(true) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
			evicted := &evictionLog{}
			factory := &shapeFactory{}
			var options []Option
			if tc.refuseInsert {
				var inserts atomic.Int64
				options = append(options, WithOnInsert(func(*Entry) bool { return inserts.Add(1) == 1 }))
			}
			pool := tierPool(t, g, factory, evicted, options...)
			entry, key := buildTierEntry(t, pool, g, "glpat-replace-fails")
			if tc.prepare != nil {
				tc.prepare(factory, pool, key, entry)
			}
			before := pooledUnder(pool, key)
			_, causesBefore := evicted.seen()
			captured := captureLogs(t)

			pool.replaceTier(t.Context(), key, entry, ultimate)

			if pooledUnder(pool, key) != before {
				t.Error("the pool's entry changed though the replacement could not be made")
			}
			if _, causes := evicted.seen(); len(causes) != len(causesBefore) {
				t.Errorf("eviction callback handed %v, want nothing more", causes[len(causesBefore):])
			}
			assertNotReplaced(t, pool, entry, tc.wantAsked)
			if _, found := captured.find(tc.wantLog); tc.wantLog != "" && !found {
				t.Errorf("no %q line", tc.wantLog)
			}
		})
	}
}

// assertNotReplaced holds an entry whose replacement was not made to what it
// was: its client on the tier it had, the counter unmoved, and its mark asked
// wantAsked times.
func assertNotReplaced(t *testing.T, pool *ServerPool, entry *Entry, wantAsked int) {
	t.Helper()
	if entry.Client().Tier() != edition.Free || pool.Stats().TierResolvedEvictions != 0 {
		t.Error("a replacement that was not made moved the client's tier or the counter")
	}
	if mark, _ := markOf(pool, entry); mark.asked != wantAsked {
		t.Errorf("the kept entry was asked %d times, want %d", mark.asked, wantAsked)
	}
}

// TestReplaceTier_TheReplacementKeepsWhatBelongsToTheCredential checks what a
// replacement carries over and what it does not: the client, identity, token
// facts, credential handle, below-minimum hand-off, LRU position, credential
// age and last use are the credential's and are kept; the owner, the server,
// the configuration and the creation time are the shape's and are new.
func TestReplaceTier_TheReplacementKeepsWhatBelongsToTheCredential(t *testing.T) {
	g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
	g.selfScopes.Store(&[]string{"api"})
	var inserted []*Entry
	pool := tierPool(t, g, &shapeFactory{}, &evictionLog{},
		WithOnInsert(func(entry *Entry) bool { inserted = append(inserted, entry); return true }))
	var remembered []string
	pool.RememberBelowMinimum(func(gitlabURL, token string) { remembered = append(remembered, gitlabURL+" "+token) })
	old, key := buildTierEntry(t, pool, g, "glpat-keeps")
	// The old entry's three clocks are moved back, each by its own amount, and
	// the replacement's creation time is compared with a reading taken just
	// before the replacement rather than with the build's: the build and the
	// replacement both stamp time.Now(), and on a clock that advances in ticks
	// (Windows) the two readings can be equal. The replacement's own stamp is
	// never before that reading, while a copy of any of the three old clocks is.
	pool.mu.Lock()
	old.lastValidated = old.lastValidated.Add(-time.Minute)
	old.lastUsed = old.lastUsed.Add(-2 * time.Minute)
	old.createdAt = old.createdAt.Add(-3 * time.Minute)
	pool.mu.Unlock()

	before := time.Now()
	pool.replaceTier(t.Context(), key, old, gitlabclient.TierResolution{Tier: edition.Premium, Answered: true})

	next := pooledUnder(pool, key)
	if next == old {
		t.Fatal("the entry was not replaced")
	}
	if len(inserted) != 2 || inserted[1] != next {
		t.Errorf("the insert callback was handed %d entries, want the build's and then the replacement", len(inserted))
	}
	if next.Client() != old.Client() || next.Identity() != old.Identity() || !next.Identity().Resolved() ||
		next.credentialHash != old.credentialHash || next.tokenFacts().KindUnknown || !slices.Equal(next.tokenFacts().Scopes, []string{"api"}) {
		t.Error("the replacement lost the client, the identity, the credential handle or the token's facts")
	}
	pool.mu.RLock()
	sameElement, sameAge, sameUse := next.element == old.element, next.lastValidated.Equal(old.lastValidated), next.lastUsed.Equal(old.lastUsed)
	newer := !next.createdAt.Before(before)
	pool.mu.RUnlock()
	if !sameElement || !sameAge || !sameUse || !newer {
		t.Errorf("LRU position kept %t, credential age kept %t, last use kept %t, created anew %t; want all true", sameElement, sameAge, sameUse, newer)
	}
	if next.Owner() == old.Owner() || next.Server() == old.Server() || next.Config().Tier != edition.Premium || old.Config().Tier != edition.Free {
		t.Error("the replacement does not carry an owner, server and configuration of its own")
	}
	next.rememberBelowMinimum()
	if want := []string{g.url + " glpat-keeps"}; !slices.Equal(remembered, want) {
		t.Errorf("the replacement handed the below-minimum verdict as %v, want %v", remembered, want)
	}
}

// TestReplaceTier_ARefusalTheOldEntryIsHanded_EndsNothing checks the window a
// replacement leaves: a refusal of the credential that reaches the old entry,
// through the hook it carried before the swap, finds it no longer pooled, and
// ends nothing and counts nothing; the next refused call reaches the
// replacement ([TestReplaceTier_TheClientsHook_PointsAtTheReplacement]).
func TestReplaceTier_ARefusalTheOldEntryIsHanded_EndsNothing(t *testing.T) {
	g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
	evicted := &evictionLog{}
	pool := tierPool(t, g, &shapeFactory{}, evicted)
	old, key := buildTierEntry(t, pool, g, "glpat-refused-later")
	pool.replaceTier(t.Context(), key, old, gitlabclient.TierResolution{Tier: edition.Ultimate, Answered: true})
	next := pooledUnder(pool, key)

	old.rejected.Store(true)
	pool.evictRejectedCredential(key, old)

	if pooledUnder(pool, key) != next || pool.Stats().RejectedCredentialEvictions != 0 {
		t.Error("a refusal handed to the replaced entry ended its replacement")
	}
}

// TestReplaceTier_TheClientsHook_PointsAtTheReplacement drives a call GitLab
// refuses for the credential through the shared client after a replacement,
// and checks that it is the replacement that ends, as a rejected credential.
func TestReplaceTier_TheClientsHook_PointsAtTheReplacement(t *testing.T) {
	g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
	evicted := &evictionLog{}
	pool := tierPool(t, g, &shapeFactory{}, evicted)
	old, key := buildTierEntry(t, pool, g, "glpat-hook")
	pool.replaceTier(t.Context(), key, old, gitlabclient.TierResolution{Tier: edition.Ultimate, Answered: true})
	next := pooledUnder(pool, key)

	g.set(func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token","error_description":"Token was revoked. You have to re-authorize from the user."}`))
	}, "")
	_ = next.Client().ResolveTier(t.Context(), t.Context())

	awaitCondition(t, "the replacement to end", func() bool { return pooledUnder(pool, key) == nil })
	entries, causes := evicted.seen()
	if len(causes) != 2 || causes[1] != CauseRejectedCredential || entries[1] != next {
		t.Errorf("eviction callback handed %v, want the replacement ended as a rejected credential", causes)
	}
}

// TestLearnTokenFacts_LandsOnTheEntryCarryingTheClient covers where what the
// self endpoint said of a token is kept: the entry pooled under the key when it
// carries the asking entry's client, which a replacement on another tier's
// shape does, and the asking entry otherwise.
func TestLearnTokenFacts_LandsOnTheEntryCarryingTheClient(t *testing.T) {
	learned := gitlabclient.FactsFromScopes([]string{"api"}, 9)
	cases := []struct {
		name string
		// pooled is what the key holds when the answer arrives, given the
		// asking entry.
		pooled     func(t *testing.T, asking *Entry) *Entry
		wantOnPool bool
	}{
		{name: "the asking entry itself", pooled: func(_ *testing.T, asking *Entry) *Entry { return asking }, wantOnPool: true},
		{
			name:       "its replacement, which shares the client",
			pooled:     func(_ *testing.T, asking *Entry) *Entry { return &Entry{client: asking.client} },
			wantOnPool: true,
		},
		{
			name: "an entry rebuilt with a client of its own",
			pooled: func(t *testing.T, _ *Entry) *Entry {
				t.Helper()
				client, err := gitlabclient.NewClientWithTokenRetries(stubGitLabBase, "glpat-other", false, true)
				if err != nil {
					t.Fatalf("NewClientWithTokenRetries(): %v", err)
				}
				return &Entry{client: client}
			},
		},
		{name: "nothing at all", pooled: func(*testing.T, *Entry) *Entry { return nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := New(testConfig(stubGitLabBase), testFactory())
			asking, err := pool.GetOrCreateEntry("glpat-learns", stubGitLabBase, nil)
			if err != nil {
				t.Fatalf("GetOrCreateEntry(): %v", err)
			}
			key := sessionKey("glpat-learns", stubGitLabBase)
			pooled := tc.pooled(t, asking)
			pool.mu.Lock()
			if pooled == nil {
				delete(pool.entries, key)
			} else {
				pool.entries[key] = pooled
			}
			pool.mu.Unlock()

			pool.learnTokenFacts(key, asking, learned)

			holder := asking
			if tc.wantOnPool {
				holder = pooled
			}
			if got := holder.tokenFacts(); got.ID != learned.ID {
				t.Errorf("the facts were not kept by the entry that should hold them: %+v", got)
			}
			if pooled != nil && pooled != holder && pooled.tokenFacts().ID == learned.ID {
				t.Error("an entry carrying another client was handed this token's facts")
			}
		})
	}
}

// TestReplaceTier_RacesTheKindReread_WithoutLosingTheAnswer runs a
// revalidation that learns the token's kind beside a tick that replaces the
// entry, under the race detector: whichever lands first, the replacement ends
// up carrying the kind, and nothing is read or written unguarded.
func TestReplaceTier_RacesTheKindReread_WithoutLosingTheAnswer(t *testing.T) {
	// sequential: each round is a fresh pool; the rounds repeat the race
	for round := range 20 {
		g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
		pool := tierPool(t, g, &shapeFactory{}, &evictionLog{})
		entry, key := buildTierEntry(t, pool, g, "glpat-race")
		if !entry.tokenFacts().KindUnknown {
			t.Fatal("the build learned the token's kind; the race needs it unknown")
		}
		g.selfScopes.Store(&[]string{"api"})
		g.set(licenseAnswer(http.StatusOK, "ultimate"), "")
		makeTierDue(t, pool, entry)

		var both sync.WaitGroup
		both.Go(func() { pool.revalidateAll(t.Context()) })
		both.Go(func() { pool.redetectDueTiers(t.Context()) })
		both.Wait()

		current := pooledUnder(pool, key)
		if current == entry || current.Config().Tier != edition.Ultimate {
			t.Fatalf("round %d: the entry was not replaced on Ultimate", round)
		}
		if current.tokenFacts().KindUnknown {
			t.Fatalf("round %d: the kind the revalidation learned was lost by the replacement", round)
		}
	}
}

// TestTierRedetectDelay_DoublesUpToTheCeiling pins the waits between asks.
func TestTierRedetectDelay_DoublesUpToTheCeiling(t *testing.T) {
	cases := []struct {
		previous, want time.Duration
	}{
		{time.Minute, 2 * time.Minute},
		{4 * time.Minute, 8 * time.Minute},
		{8 * time.Minute, 15 * time.Minute},
		{15 * time.Minute, 15 * time.Minute},
		{10 * time.Millisecond, 20 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.previous.String(), func(t *testing.T) {
			if got := tierRedetectDelay(tc.previous); got != tc.want {
				t.Errorf("tierRedetectDelay(%v) = %v, want %v", tc.previous, got, tc.want)
			}
		})
	}
}

// TestWithTierRetry_KeepsTheRegisterValueUnlessATestShortensIt covers the
// option: the register's value by default and for a zero or negative one, a
// shorter one as given, and a longer one clamped to the ceiling.
func TestWithTierRetry_KeepsTheRegisterValueUnlessATestShortensIt(t *testing.T) {
	if tierProvisionalRetry != tenancy.TierProvisionalRetry || tierProvisionalRetryMax != tenancy.TierProvisionalRetryMax {
		t.Fatal("the pool's waits are not the register's")
	}
	cases := []struct {
		name    string
		options []Option
		want    time.Duration
	}{
		{name: "unset", want: tierProvisionalRetry},
		{name: "zero", options: []Option{WithTierRetry(0)}, want: tierProvisionalRetry},
		{name: "negative", options: []Option{WithTierRetry(-time.Second)}, want: tierProvisionalRetry},
		{name: "shorter", options: []Option{WithTierRetry(time.Millisecond)}, want: time.Millisecond},
		{name: "past the ceiling", options: []Option{WithTierRetry(time.Hour)}, want: tierProvisionalRetryMax},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := New(testConfig(stubGitLabBase), testFactory(), tc.options...).tierRetry; got != tc.want {
				t.Errorf("tierRetry = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestStartTierRedetection_ResolvesOnItsOwnClock covers the sweep's own
// goroutine: with revalidation switched off it still asks a provisional tier
// again once its wait is over and resolves it, and under --tier it is not
// started at all, whatever context it is handed.
func TestStartTierRedetection_ResolvesOnItsOwnClock(t *testing.T) {
	t.Run("with revalidation switched off", func(t *testing.T) {
		g := newTierGitLab(t, licenseAnswer(http.StatusBadGateway, ""), "")
		pool := tierPool(t, g, &shapeFactory{}, &evictionLog{}, WithRevalidateInterval(0), WithTierRetry(10*time.Millisecond))
		entry, key := buildTierEntry(t, pool, g, "glpat-own-clock")
		g.set(licenseAnswer(http.StatusOK, "ultimate"), "")

		pool.StartRevalidation(t.Context())
		pool.StartTierRedetection(t.Context())

		awaitCondition(t, "the sweep to resolve the tier", func() bool {
			current := pooledUnder(pool, key)
			return current != nil && current != entry && current.Config().Tier == edition.Ultimate
		})
	})

	t.Run("closing the pool ends it and waits for it", func(t *testing.T) {
		captured := captureLogs(t)
		cfg := testConfig(stubGitLabBase)
		cfg.TierExplicit = false
		pool := New(cfg, testFactory())
		pool.StartTierRedetection(t.Context())

		pool.Close()

		// Read at once, with no wait: Close returned after the sweep wrote
		// its last line, so nothing it writes or asks outlives the pool.
		if _, found := captured.find("server pool: tier re-detection stopped"); !found {
			t.Error("Close returned while the re-detection sweep was still running")
		}
		pool.Close()
	})

	t.Run("under --tier", func(t *testing.T) {
		captured := captureLogs(t)
		pool := New(testConfig(stubGitLabBase), testFactory())
		pool.StartTierRedetection(nil) //nolint:staticcheck // SA1012: the nil context is the input the guard under test exists for
		if _, found := captured.find("server pool: tier re-detection not started, the tier is configured"); !found {
			t.Error("a pool whose tier is configured did not say it started no re-detection")
		}
		if _, found := captured.find("server pool: starting tier re-detection"); found {
			t.Error("a pool whose tier is configured started a re-detection sweep")
		}
	})
}

// TestTierResolution_IsNamedAsItsOwnCause holds the cause a replacement hands
// the eviction callback to its name, which telemetry labels its series with.
func TestTierResolution_IsNamedAsItsOwnCause(t *testing.T) {
	if CauseTierResolved != "tier_resolved" {
		t.Errorf("CauseTierResolved = %q, want tier_resolved", CauseTierResolved)
	}
}
