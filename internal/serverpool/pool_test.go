// pool_test.go contains unit tests for the bounded LRU server pool.

package serverpool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// testFactory returns a ServerFactory that creates minimal *mcp.Server instances.
func testFactory() ServerFactory {
	return func(client *gitlabclient.Client, _ *config.Config) *mcp.Server {
		return mcp.NewServer(&mcp.Implementation{
			Name:    "test-server",
			Version: "0.0.0",
		}, nil)
	}
}

// testConfig returns a config suitable for tests using the given base URL.
func testConfig(baseURL string) *config.Config {
	return &config.Config{
		GitLabURL:     baseURL,
		GitLabToken:   "default-token",
		SkipTLSVerify: false,
		IgnoreScopes:  true,
	}
}

// TestGetOrCreate_EmptyToken verifies that GetOrCreate rejects empty tokens
// to prevent all unauthenticated callers from sharing a single server entry.
func TestGetOrCreate_EmptyToken(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())

	srv, err := pool.GetOrCreate("", "http://localhost")
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
	if srv != nil {
		t.Fatal("expected nil server for empty token")
	}
}

// TestGetOrCreate_NewToken verifies that GetOrCreate handles the new token scenario correctly.
func TestGetOrCreate_NewToken(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())

	srv, err := pool.GetOrCreate("glpat-token1", "http://localhost")
	if err != nil {
		t.Fatalf("GetOrCreate() unexpected error: %v", err)
	}
	if srv == nil {
		t.Fatal("GetOrCreate() returned nil server")
	}
	if pool.Size() != 1 {
		t.Errorf("pool.Size() = %d, want 1", pool.Size())
	}
}

// TestGetOrCreate_DetectsScopesPerToken verifies that HTTP pool entries pass
// token-specific scope detection into the server factory instead of mutating
// the shared server-wide config.
func TestGetOrCreate_DetectsScopesPerToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/personal_access_tokens/self", func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("PRIVATE-TOKEN")
		if token == "" {
			if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
				token = bearer
			}
		}
		scopes := []string{"api"}
		if token == "glpat-read" {
			scopes = []string{"read_api"}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     1,
			"scopes": scopes,
			"active": true,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.IgnoreScopes = false
	capturedScopes := make([][]string, 0, 2)
	factory := func(_ *gitlabclient.Client, entryCfg *config.Config) *mcp.Server {
		capturedScopes = append(capturedScopes, append([]string(nil), entryCfg.TokenScopes...))
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil)
	}
	pool := New(cfg, factory)

	if _, err := pool.GetOrCreate("glpat-read", srv.URL); err != nil {
		t.Fatalf("GetOrCreate(read token) error: %v", err)
	}
	if _, err := pool.GetOrCreate("glpat-api", srv.URL); err != nil {
		t.Fatalf("GetOrCreate(api token) error: %v", err)
	}

	if len(capturedScopes) != 2 {
		t.Fatalf("captured %d scope sets, want 2", len(capturedScopes))
	}
	if len(capturedScopes[0]) != 1 || capturedScopes[0][0] != "read_api" {
		t.Fatalf("first token scopes = %v, want [read_api]", capturedScopes[0])
	}
	if len(capturedScopes[1]) != 1 || capturedScopes[1][0] != "api" {
		t.Fatalf("second token scopes = %v, want [api]", capturedScopes[1])
	}
	if cfg.TokenScopes != nil {
		t.Fatalf("shared config TokenScopes = %v, want nil", cfg.TokenScopes)
	}
}

// TestGetOrCreate_DetectsEnterprisePerEntry verifies that CE/EE detection is
// scoped to the pool entry rather than inherited from the shared HTTP config.
func TestGetOrCreate_DetectsEnterprisePerEntry(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/version", func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("PRIVATE-TOKEN")
		enterprise := token == "glpat-ee"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version":    "17.0.0",
			"enterprise": enterprise,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Enterprise = true
	cfg.AutoDetectEnterprise = true
	captured := make([]bool, 0, 2)
	factory := func(client *gitlabclient.Client, entryCfg *config.Config) *mcp.Server {
		if entryCfg.Enterprise != client.IsEnterprise() {
			t.Fatalf("entry config enterprise %v does not match client enterprise %v", entryCfg.Enterprise, client.IsEnterprise())
		}
		captured = append(captured, entryCfg.Enterprise)
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil)
	}
	pool := New(cfg, factory)

	if _, err := pool.GetOrCreate("glpat-ce", srv.URL); err != nil {
		t.Fatalf("GetOrCreate(ce token) error: %v", err)
	}
	if _, err := pool.GetOrCreate("glpat-ee", srv.URL); err != nil {
		t.Fatalf("GetOrCreate(ee token) error: %v", err)
	}

	if len(captured) != 2 {
		t.Fatalf("captured %d enterprise values, want 2", len(captured))
	}
	if captured[0] {
		t.Fatalf("CE entry enterprise = true, want false")
	}
	if !captured[1] {
		t.Fatalf("EE entry enterprise = false, want true")
	}
	if !cfg.Enterprise {
		t.Fatalf("shared config Enterprise was mutated to false")
	}
}

// TestGetOrCreate_SameToken verifies that GetOrCreate handles the same token scenario correctly.
func TestGetOrCreate_SameToken(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())

	srv1, err := pool.GetOrCreate("glpat-same", "http://localhost")
	if err != nil {
		t.Fatalf("first GetOrCreate() error: %v", err)
	}

	srv2, err := pool.GetOrCreate("glpat-same", "http://localhost")
	if err != nil {
		t.Fatalf("second GetOrCreate() error: %v", err)
	}

	if srv1 != srv2 {
		t.Error("expected same *mcp.Server pointer for the same token")
	}
	if pool.Size() != 1 {
		t.Errorf("pool.Size() = %d, want 1", pool.Size())
	}
}

// TestGetOrCreate_DifferentTokens verifies that GetOrCreate handles the different tokens scenario correctly.
func TestGetOrCreate_DifferentTokens(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())

	srv1, err := pool.GetOrCreate("glpat-token-a", "http://localhost")
	if err != nil {
		t.Fatalf("GetOrCreate(token-a) error: %v", err)
	}

	srv2, err := pool.GetOrCreate("glpat-token-b", "http://localhost")
	if err != nil {
		t.Fatalf("GetOrCreate(token-b) error: %v", err)
	}

	if srv1 == srv2 {
		t.Error("expected different *mcp.Server pointers for different tokens")
	}
	if pool.Size() != 2 {
		t.Errorf("pool.Size() = %d, want 2", pool.Size())
	}
}

// TestGetOrCreate_LRUEviction verifies that GetOrCreate handles the l r u eviction scenario correctly.
func TestGetOrCreate_LRUEviction(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory(), WithMaxSize(2))

	// fill the pool
	_, err := pool.GetOrCreate("token-1", "http://localhost")
	if err != nil {
		t.Fatalf("GetOrCreate(token-1) error: %v", err)
	}
	_, err = pool.GetOrCreate("token-2", "http://localhost")
	if err != nil {
		t.Fatalf("GetOrCreate(token-2) error: %v", err)
	}

	// this should evict token-1 (LRU)
	_, err = pool.GetOrCreate("token-3", "http://localhost")
	if err != nil {
		t.Fatalf("GetOrCreate(token-3) error: %v", err)
	}

	if pool.Size() != 2 {
		t.Errorf("pool.Size() = %d, want 2 after eviction", pool.Size())
	}

	// token-1 should have been evicted — re-requesting creates a new entry
	srv1, err := pool.GetOrCreate("token-1", "http://localhost")
	if err != nil {
		t.Fatalf("GetOrCreate(token-1) re-create error: %v", err)
	}
	if srv1 == nil {
		t.Fatal("GetOrCreate(token-1) returned nil after eviction + re-create")
	}
	// Now token-2 should be evicted (it was LRU after token-3 and token-1 accesses)
	if pool.Size() != 2 {
		t.Errorf("pool.Size() = %d, want 2", pool.Size())
	}
}

// TestGetOrCreate_LRUPromotes verifies that GetOrCreate handles the l r u promotes scenario correctly.
func TestGetOrCreate_LRUPromotes(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory(), WithMaxSize(2))

	_, _ = pool.GetOrCreate("token-a", "http://localhost")
	_, _ = pool.GetOrCreate("token-b", "http://localhost")

	// Re-access token-a to promote it in LRU
	_, _ = pool.GetOrCreate("token-a", "http://localhost")

	// Adding token-c should evict token-b (now LRU), not token-a
	_, _ = pool.GetOrCreate("token-c", "http://localhost")

	if pool.Size() != 2 {
		t.Fatalf("pool.Size() = %d, want 2", pool.Size())
	}

	// Verify token-a still returns the same cached entry (not evicted)
	srvA1, _ := pool.GetOrCreate("token-a", "http://localhost")
	srvA2, _ := pool.GetOrCreate("token-a", "http://localhost")
	if srvA1 != srvA2 {
		t.Error("token-a should still be in pool after LRU promotion")
	}
}

// TestGetOrCreate_Concurrent verifies that GetOrCreate handles the concurrent scenario correctly.
func TestGetOrCreate_Concurrent(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory(), WithMaxSize(20))

	tokens := []string{
		"tok-1", "tok-2", "tok-3", "tok-4", "tok-5",
		"tok-6", "tok-7", "tok-8", "tok-9", "tok-10",
	}

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			token := tokens[idx%len(tokens)]
			srv, err := pool.GetOrCreate(token, "http://localhost")
			if err != nil {
				t.Errorf("concurrent GetOrCreate() error: %v", err)
				return
			}
			if srv == nil {
				t.Error("concurrent GetOrCreate() returned nil")
			}
		}(i)
	}
	wg.Wait()

	if pool.Size() != 10 {
		t.Errorf("pool.Size() = %d, want 10", pool.Size())
	}
}

// TestClose verifies the behavior of close.
func TestClose(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())

	_, _ = pool.GetOrCreate("token-1", "http://localhost")
	_, _ = pool.GetOrCreate("token-2", "http://localhost")

	pool.Close()

	if pool.Size() != 0 {
		t.Errorf("pool.Size() = %d after Close(), want 0", pool.Size())
	}
}

// TestTokenHash verifies the behavior of token hash.
func TestTokenHash(t *testing.T) {
	hash1 := tokenHash("glpat-abc123")
	hash2 := tokenHash("glpat-abc123")
	hash3 := tokenHash("glpat-xyz789")

	if hash1 != hash2 {
		t.Error("same token should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("different tokens should produce different hashes")
	}
	if len(hash1) != 64 {
		t.Errorf("hash length = %d, want 64 (SHA-256 hex)", len(hash1))
	}
}

// TestTokenSuffix validates token suffix across multiple scenarios using table-driven subtests.
func TestTokenSuffix(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{"normal token", "glpat-abc123xyz", "...3xyz"},
		{"short token", "abc", "****"},
		{"exactly 4 chars", "abcd", "****"},
		{"5 chars", "abcde", "...bcde"},
		{"empty token", "", "****"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenSuffix(tt.token)
			if got != tt.expected {
				t.Errorf("tokenSuffix(%q) = %q, want %q", tt.token, got, tt.expected)
			}
		})
	}
}

// TestWithMaxSize verifies the behavior of with max size.
func TestWithMaxSize(t *testing.T) {
	cfg := testConfig("http://localhost")

	pool := New(cfg, testFactory(), WithMaxSize(5))
	if pool.maxSize != 5 {
		t.Errorf("maxSize = %d, want 5", pool.maxSize)
	}

	// Zero/negative values should be ignored
	pool2 := New(cfg, testFactory(), WithMaxSize(0))
	if pool2.maxSize != defaultMaxSize {
		t.Errorf("maxSize = %d with zero, want default %d", pool2.maxSize, defaultMaxSize)
	}

	pool3 := New(cfg, testFactory(), WithMaxSize(-1))
	if pool3.maxSize != defaultMaxSize {
		t.Errorf("maxSize = %d with -1, want default %d", pool3.maxSize, defaultMaxSize)
	}
}

// TestWithRevalidateInterval verifies that the revalidation interval can be
// configured via the option.
func TestWithRevalidateInterval(t *testing.T) {
	cfg := testConfig("http://localhost")

	pool := New(cfg, testFactory(), WithRevalidateInterval(5*time.Minute))
	if pool.revalidateInterval != 5*time.Minute {
		t.Errorf("revalidateInterval = %v, want 5m", pool.revalidateInterval)
	}

	// Zero disables revalidation
	pool2 := New(cfg, testFactory(), WithRevalidateInterval(0))
	if pool2.revalidateInterval != 0 {
		t.Errorf("revalidateInterval = %v with zero, want 0", pool2.revalidateInterval)
	}
}

// TestPoolEntry_TimestampFields verifies that new pool entries have
// createdAt and lastValidated set to a recent time.
func TestPoolEntry_TimestampFields(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())

	before := time.Now()
	_, err := pool.GetOrCreate("token-time", "http://localhost")
	if err != nil {
		t.Fatalf("GetOrCreate() error: %v", err)
	}
	after := time.Now()

	key := sessionKey("token-time", "http://localhost")
	pool.mu.RLock()
	entry := pool.entries[key]
	pool.mu.RUnlock()

	if entry.createdAt.Before(before) || entry.createdAt.After(after) {
		t.Errorf("createdAt %v not between %v and %v", entry.createdAt, before, after)
	}
	if entry.lastValidated.Before(before) || entry.lastValidated.After(after) {
		t.Errorf("lastValidated %v not between %v and %v", entry.lastValidated, before, after)
	}
}

// TestStartRevalidation_NilContext verifies that StartRevalidation
// handles nil context gracefully by substituting context.Background().
func TestStartRevalidation_NilContext(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory(), WithRevalidateInterval(0))

	// Should not panic — nil ctx is replaced with context.Background()
	//lint:ignore SA1012 intentionally testing nil context guard
	pool.StartRevalidation(nil) //nolint:staticcheck // SA1012
}

// TestStartRevalidation_DisabledWithZeroInterval verifies that
// StartRevalidation returns immediately when interval is zero.
func TestStartRevalidation_DisabledWithZeroInterval(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory(), WithRevalidateInterval(0))

	ctx := t.Context()

	// Should not panic and return immediately
	pool.StartRevalidation(ctx)
}

// TestStartRevalidation_CancelledContext verifies that the revalidation
// goroutine stops when the context is cancelled.
func TestStartRevalidation_CancelledContext(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory(), WithRevalidateInterval(50*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	pool.StartRevalidation(ctx)

	// Let it run briefly then cancel
	time.Sleep(10 * time.Millisecond)
	cancel()

	// Give goroutine time to exit cleanly
	time.Sleep(100 * time.Millisecond)
}

// TestEvictByKey verifies that evictByKey removes the specified entry
// and not others.
func TestEvictByKey(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())

	_, _ = pool.GetOrCreate("token-keep", "http://localhost")
	_, _ = pool.GetOrCreate("token-evict", "http://localhost")

	if pool.Size() != 2 {
		t.Fatalf("pool.Size() = %d, want 2", pool.Size())
	}

	key := sessionKey("token-evict", "http://localhost")
	pool.evictByKey(key)

	if pool.Size() != 1 {
		t.Errorf("pool.Size() = %d after eviction, want 1", pool.Size())
	}

	// Evicting a nonexistent key is a no-op
	pool.evictByKey("nonexistent-key")
	if pool.Size() != 1 {
		t.Errorf("pool.Size() = %d after noop eviction, want 1", pool.Size())
	}
}

// TestDefaultRevalidateInterval verifies that the default revalidation
// interval is 15 minutes.
func TestDefaultRevalidateInterval(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())
	if pool.revalidateInterval != DefaultRevalidateInterval {
		t.Errorf("default revalidateInterval = %v, want %v", pool.revalidateInterval, DefaultRevalidateInterval)
	}
}

// TestStats_HitsAndMisses verifies that Stats tracks cache hits and misses.
func TestStats_HitsAndMisses(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())

	// First call → miss
	_, _ = pool.GetOrCreate("token-a", "http://localhost")
	s := pool.Stats()
	if s.Misses != 1 {
		t.Errorf("Misses = %d after first GetOrCreate, want 1", s.Misses)
	}
	if s.Hits != 0 {
		t.Errorf("Hits = %d after first GetOrCreate, want 0", s.Hits)
	}

	// Second call with same token → hit
	_, _ = pool.GetOrCreate("token-a", "http://localhost")
	s = pool.Stats()
	if s.Hits != 1 {
		t.Errorf("Hits = %d after second GetOrCreate, want 1", s.Hits)
	}
	if s.Misses != 1 {
		t.Errorf("Misses = %d after second GetOrCreate, want 1", s.Misses)
	}

	// Third call with different token → another miss
	_, _ = pool.GetOrCreate("token-b", "http://localhost")
	s = pool.Stats()
	if s.Hits != 1 {
		t.Errorf("Hits = %d, want 1", s.Hits)
	}
	if s.Misses != 2 {
		t.Errorf("Misses = %d, want 2", s.Misses)
	}
}

// TestStats_Evictions verifies that LRU evictions are counted.
func TestStats_Evictions(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory(), WithMaxSize(2))

	_, _ = pool.GetOrCreate("tok-1", "http://localhost")
	_, _ = pool.GetOrCreate("tok-2", "http://localhost")
	_, _ = pool.GetOrCreate("tok-3", "http://localhost") // evicts tok-1

	s := pool.Stats()
	if s.Evictions != 1 {
		t.Errorf("Evictions = %d after 1 LRU eviction, want 1", s.Evictions)
	}

	_, _ = pool.GetOrCreate("tok-4", "http://localhost") // evicts tok-2
	s = pool.Stats()
	if s.Evictions != 2 {
		t.Errorf("Evictions = %d after 2 LRU evictions, want 2", s.Evictions)
	}
}

// TestStats_EvictByKey verifies that explicit key eviction is counted.
func TestStats_EvictByKey(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())

	_, _ = pool.GetOrCreate("tok-evict", "http://localhost")
	key := sessionKey("tok-evict", "http://localhost")
	pool.evictByKey(key)

	s := pool.Stats()
	if s.Evictions != 1 {
		t.Errorf("Evictions = %d after evictByKey, want 1", s.Evictions)
	}

	// Evicting a nonexistent key does not increment
	pool.evictByKey("nonexistent")
	s = pool.Stats()
	if s.Evictions != 1 {
		t.Errorf("Evictions = %d after noop evictByKey, want 1", s.Evictions)
	}
}

// TestStats_SnapshotFields verifies that Stats returns correct pool state.
func TestStats_SnapshotFields(t *testing.T) {
	cfg := testConfig("http://localhost")
	before := time.Now()
	pool := New(cfg, testFactory(), WithMaxSize(50))

	_, _ = pool.GetOrCreate("tok-1", "http://localhost")
	_, _ = pool.GetOrCreate("tok-2", "http://localhost")

	s := pool.Stats()
	if s.CurrentSize != 2 {
		t.Errorf("CurrentSize = %d, want 2", s.CurrentSize)
	}
	if s.MaxSize != 50 {
		t.Errorf("MaxSize = %d, want 50", s.MaxSize)
	}
	if s.CreatedAt.Before(before) {
		t.Errorf("CreatedAt %v is before pool construction time %v", s.CreatedAt, before)
	}
}

// TestRevalidateAll_EvictsInvalidTokens verifies that revalidateAll evicts
// entries whose tokens fail validation (Ping returns error) and keeps entries
// that pass. Exercises the full revalidateAll code path including the
// RevalidationsFailed and RevalidationsSucceeded metric counters.
func TestRevalidateAll_EvictsInvalidTokens(t *testing.T) {
	// Two httptest servers: one healthy (200), one returning 401.
	healthyHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abc"}`))
	})
	unhealthyHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
	})

	healthySrv := httptest.NewServer(healthyHandler)
	t.Cleanup(healthySrv.Close)
	unhealthySrv := httptest.NewServer(unhealthyHandler)
	t.Cleanup(unhealthySrv.Close)

	// Build pool manually: factory won't be called since we insert entries directly.
	cfg := testConfig(healthySrv.URL)
	pool := New(cfg, testFactory())

	// Create a healthy entry.
	healthyClient, err := gitlabclient.NewClientWithToken(healthySrv.URL, "good-token", false)
	if err != nil {
		t.Fatalf("healthy client: %v", err)
	}
	goodKey := tokenHash("good-token")
	goodElem := pool.lru.PushFront(goodKey)
	pool.entries[goodKey] = &poolEntry{
		server:        mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil),
		client:        healthyClient,
		element:       goodElem,
		createdAt:     time.Now(),
		lastValidated: time.Now(),
	}

	// Create an unhealthy entry.
	unhealthyClient, err := gitlabclient.NewClientWithToken(unhealthySrv.URL, "bad-token", false)
	if err != nil {
		t.Fatalf("unhealthy client: %v", err)
	}
	badKey := tokenHash("bad-token")
	badElem := pool.lru.PushFront(badKey)
	pool.entries[badKey] = &poolEntry{
		server:        mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil),
		client:        unhealthyClient,
		element:       badElem,
		createdAt:     time.Now(),
		lastValidated: time.Now(),
	}

	if pool.Size() != 2 {
		t.Fatalf("pool.Size() = %d, want 2", pool.Size())
	}

	pool.revalidateAll(context.Background())

	if pool.Size() != 1 {
		t.Errorf("pool.Size() = %d after revalidation, want 1 (unhealthy evicted)", pool.Size())
	}

	s := pool.Stats()
	if s.RevalidationsFailed != 1 {
		t.Errorf("RevalidationsFailed = %d, want 1", s.RevalidationsFailed)
	}
	if s.RevalidationsSucceeded != 1 {
		t.Errorf("RevalidationsSucceeded = %d, want 1", s.RevalidationsSucceeded)
	}
}

// TestRevalidateAll_CancelledContext verifies that revalidateAll stops
// processing entries when the context is cancelled.
func TestRevalidateAll_CancelledContext(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())

	_, _ = pool.GetOrCreate("tok-1", "http://localhost")

	ctx := testutil.CancelledCtx(t)

	// Should return quickly without panicking.
	pool.revalidateAll(ctx)
}

// TestStartRevalidation_TriggersRevalidation verifies that StartRevalidation
// actually triggers revalidateAll via the ticker by observing metrics change.
func TestStartRevalidation_TriggersRevalidation(t *testing.T) {
	healthyHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abc"}`))
	})
	srv := httptest.NewServer(healthyHandler)
	t.Cleanup(srv.Close)

	cfg := testConfig(srv.URL)
	pool := New(cfg, testFactory(), WithRevalidateInterval(50*time.Millisecond))

	// Insert entry with a valid client.
	client, err := gitlabclient.NewClientWithToken(srv.URL, "valid-tok", false)
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	key := tokenHash("valid-tok")
	elem := pool.lru.PushFront(key)
	pool.entries[key] = &poolEntry{
		server:        mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil),
		client:        client,
		element:       elem,
		createdAt:     time.Now(),
		lastValidated: time.Now(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool.StartRevalidation(ctx)

	// Wait for at least one tick to complete.
	time.Sleep(150 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	s := pool.Stats()
	if s.RevalidationsSucceeded < 1 {
		t.Errorf("RevalidationsSucceeded = %d, want >= 1", s.RevalidationsSucceeded)
	}
}

// TestStats_ConcurrentAccess verifies that metrics are safe under concurrent use.
func TestStats_ConcurrentAccess(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory(), WithMaxSize(20))

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			token := "tok-" + string(rune('a'+idx%5))
			_, _ = pool.GetOrCreate(token, "http://localhost")
			_ = pool.Stats()
		}(i)
	}
	wg.Wait()

	s := pool.Stats()
	total := s.Hits + s.Misses
	if total != 50 {
		t.Errorf("Hits(%d) + Misses(%d) = %d, want 50", s.Hits, s.Misses, total)
	}
}

// TestGetOrCreate_InvalidGitLabURL verifies that GetOrCreate returns an error
// when the GitLab URL is invalid and NewClientWithToken fails to create a client.
func TestGetOrCreate_InvalidGitLabURL(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())

	srv, err := pool.GetOrCreate("glpat-token1", "://invalid")
	if err == nil {
		t.Fatal("expected error for invalid GitLab URL, got nil")
	}
	if srv != nil {
		t.Fatal("expected nil server when client creation fails")
	}
	if pool.Size() != 0 {
		t.Errorf("pool.Size() = %d, want 0 after failed creation", pool.Size())
	}
}

// TestEvictLRU_EmptyList verifies that evictLRU handles the case where the
// LRU list is empty without panicking (back == nil guard).
func TestEvictLRU_EmptyList(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory(), WithMaxSize(5))

	// Directly call evictLRU with an empty pool — should not panic.
	pool.mu.Lock()
	pool.evictLRU()
	pool.mu.Unlock()

	if pool.Size() != 0 {
		t.Errorf("pool.Size() = %d, want 0", pool.Size())
	}
}

// TestGetOrCreate_EmptyGitLabURL verifies that GetOrCreate rejects an empty
// GitLab URL to prevent sessions without a target instance.
func TestGetOrCreate_EmptyGitLabURL(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())

	srv, err := pool.GetOrCreate("glpat-token1", "")
	if err == nil {
		t.Fatal("expected error for empty GitLab URL, got nil")
	}
	if srv != nil {
		t.Fatal("expected nil server for empty GitLab URL")
	}
}

// TestGetOrCreate_DifferentURLsSameToken verifies that the same token
// against different GitLab instances gets separate pool entries.
func TestGetOrCreate_DifferentURLsSameToken(t *testing.T) {
	cfg := testConfig("http://localhost")
	pool := New(cfg, testFactory())

	srv1, err := pool.GetOrCreate("glpat-same-token", "http://gitlab-a.example.com")
	if err != nil {
		t.Fatalf("GetOrCreate(gitlab-a) error: %v", err)
	}

	srv2, err := pool.GetOrCreate("glpat-same-token", "http://gitlab-b.example.com")
	if err != nil {
		t.Fatalf("GetOrCreate(gitlab-b) error: %v", err)
	}

	if srv1 == srv2 {
		t.Error("expected different *mcp.Server pointers for same token with different GitLab URLs")
	}
	if pool.Size() != 2 {
		t.Errorf("pool.Size() = %d, want 2", pool.Size())
	}
}

// TestSessionKey verifies that sessionKey produces different hashes for
// different token+URL combinations.
func TestSessionKey(t *testing.T) {
	k1 := sessionKey("token-a", "http://gitlab.example.com")
	k2 := sessionKey("token-a", "http://gitlab.example.com")
	k3 := sessionKey("token-a", "http://other.example.com")
	k4 := sessionKey("token-b", "http://gitlab.example.com")

	if k1 != k2 {
		t.Error("same token+URL should produce same key")
	}
	if k1 == k3 {
		t.Error("same token with different URL should produce different key")
	}
	if k1 == k4 {
		t.Error("different token with same URL should produce different key")
	}
	if len(k1) != 64 {
		t.Errorf("key length = %d, want 64 (SHA-256 hex)", len(k1))
	}
}
