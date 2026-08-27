// pool_test.go contains unit tests for the bounded LRU server pool.
package serverpool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// testFactory returns a ServerFactory that creates minimal *mcp.Server instances.
func testFactory() ServerFactory {
	return func(client *gitlabclient.Client, _ *config.ServerConfig) (*mcp.Server, error) {
		return mcp.NewServer(&mcp.Implementation{
			Name:    "test-server",
			Version: "0.0.0",
		}, nil), nil
	}
}

// testConfig returns a config suitable for tests using the given base URL.
//
// Tier detection and scope discovery are disabled by default (TierExplicit +
// IgnoreScopes) so GetOrCreate performs no GitLab network I/O for the common
// case. Tests that specifically exercise detection set up an httptest server and
// re-enable the relevant path (cfg.TierExplicit = false / cfg.IgnoreScopes =
// false).
func testConfig(baseURL string) *config.Config {
	return &config.Config{
		GitLabURL:     baseURL,
		GitLabToken:   "default-token",
		SkipTLSVerify: false,
		Tier:          edition.Free,
		TierExplicit:  true,
		IgnoreScopes:  true,
	}
}

// stubGitLabBase points every pool test at a loopback GitLab stub instead
// of the literal http://localhost these tests used to carry.
//
// The literal was a hidden dependency on the machine: it only worked while
// nothing answered on port 80. On a host running a web server there, Go's
// dialer may prefer ::1, and a vhost that answers 401/403 to the
// credential probe makes every GetOrCreate fail with ErrInvalidCredential
// before the code under test runs. The stub answers 200 to everything, so
// the probe passes verdict-free the way an unreachable port used to.
var stubGitLabBase string

func TestMain(m *testing.M) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	stubGitLabBase = stub.URL
	code := m.Run()
	stub.Close()
	os.Exit(code)
}

// TestGetOrCreate_EmptyToken verifies that GetOrCreate rejects empty tokens
// to prevent all unauthenticated callers from sharing a single server entry.
func TestGetOrCreate_EmptyToken(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())

	srv, err := pool.GetOrCreate("", stubGitLabBase)
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
	if srv != nil {
		t.Fatal("expected nil server for empty token")
	}
}

// TestGetOrCreate_NewToken verifies GetOrCreate when new token.
func TestGetOrCreate_NewToken(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())

	srv, err := pool.GetOrCreate("glpat-token1", stubGitLabBase)
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
	factory := func(_ *gitlabclient.Client, entryCfg *config.ServerConfig) (*mcp.Server, error) {
		capturedScopes = append(capturedScopes, append([]string(nil), entryCfg.TokenScopes...))
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
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
	if scopes := cfg.ServerConfig().TokenScopes; scopes != nil {
		t.Fatalf("shared config produced TokenScopes = %v, want nil", scopes)
	}
}

// licenseMux returns a mux that serves /version plus a /license endpoint whose
// plan depends on the token, so DetectTier can resolve a per-token tier.
func licenseMux(planForToken func(token string) (status int, plan string)) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "17.0.0"})
	})
	mux.HandleFunc("GET /api/v4/license", func(w http.ResponseWriter, r *http.Request) {
		status, plan := planForToken(r.Header.Get("PRIVATE-TOKEN"))
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"plan": plan})
	})
	return mux
}

// TestGetOrCreate_DetectsTierPerEntry verifies that Free/Premium/Ultimate
// detection is scoped to the pool entry rather than inherited from the shared
// HTTP config, with enterprise derived from the detected tier.
func TestGetOrCreate_DetectsTierPerEntry(t *testing.T) {
	mux := licenseMux(func(token string) (int, string) {
		if token == "glpat-ee" {
			return http.StatusOK, "ultimate"
		}
		return http.StatusForbidden, ""
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Tier = edition.Ultimate
	cfg.TierExplicit = false // detect per entry
	captured := make([]bool, 0, 2)
	factory := func(client *gitlabclient.Client, entryCfg *config.ServerConfig) (*mcp.Server, error) {
		if entryCfg.Enterprise() != client.IsEnterprise() {
			t.Fatalf("entry config enterprise %v does not match client enterprise %v", entryCfg.Enterprise(), client.IsEnterprise())
		}
		captured = append(captured, entryCfg.Enterprise())
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
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
	if cfg.Tier != edition.Ultimate {
		t.Fatalf("shared config Tier was mutated to %v", cfg.Tier)
	}
}

// TestGetOrCreate_TierConfigOverridesDetection verifies that an explicit
// configured tier wins and no license detection runs when TierExplicit is true.
func TestGetOrCreate_TierConfigOverridesDetection(t *testing.T) {
	cases := []struct {
		name       string
		configured edition.Tier
		wantEnt    bool
	}{
		{name: "force ultimate", configured: edition.Ultimate, wantEnt: true},
		{name: "force free", configured: edition.Free, wantEnt: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// License endpoint returns the opposite of the configured tier; an
			// explicit tier must ignore it entirely.
			mux := licenseMux(func(string) (int, string) {
				if tc.configured.IsEnterprise() {
					return http.StatusForbidden, ""
				}
				return http.StatusOK, "ultimate"
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()

			cfg := testConfig(srv.URL)
			cfg.Tier = tc.configured
			cfg.TierExplicit = true

			var captured bool
			factory := func(client *gitlabclient.Client, entryCfg *config.ServerConfig) (*mcp.Server, error) {
				if entryCfg.Enterprise() != client.IsEnterprise() {
					t.Fatalf("entry config enterprise %v does not match client enterprise %v", entryCfg.Enterprise(), client.IsEnterprise())
				}
				captured = entryCfg.Enterprise()
				return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
			}

			pool := New(cfg, factory)
			if _, err := pool.GetOrCreate("glpat-forced", srv.URL); err != nil {
				t.Fatalf("GetOrCreate() error: %v", err)
			}
			if captured != tc.wantEnt {
				t.Fatalf("captured enterprise = %v, want %v", captured, tc.wantEnt)
			}
		})
	}
}

// TestGetOrCreate_SameToken verifies GetOrCreate when same token.
func TestGetOrCreate_SameToken(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())

	srv1, err := pool.GetOrCreate("glpat-same", stubGitLabBase)
	if err != nil {
		t.Fatalf("first GetOrCreate() error: %v", err)
	}

	srv2, err := pool.GetOrCreate("glpat-same", stubGitLabBase)
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

// TestGetOrCreate_DifferentTokens verifies GetOrCreate when different tokens.
func TestGetOrCreate_DifferentTokens(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())

	srv1, err := pool.GetOrCreate("glpat-token-a", stubGitLabBase)
	if err != nil {
		t.Fatalf("GetOrCreate(token-a) error: %v", err)
	}

	srv2, err := pool.GetOrCreate("glpat-token-b", stubGitLabBase)
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

// TestGetOrCreate_LRUEviction verifies GetOrCreate when lru eviction.
func TestGetOrCreate_LRUEviction(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory(), WithMaxSize(2))

	// fill the pool
	_, err := pool.GetOrCreate("token-1", stubGitLabBase)
	if err != nil {
		t.Fatalf("GetOrCreate(token-1) error: %v", err)
	}
	_, err = pool.GetOrCreate("token-2", stubGitLabBase)
	if err != nil {
		t.Fatalf("GetOrCreate(token-2) error: %v", err)
	}

	// this should evict token-1 (LRU)
	_, err = pool.GetOrCreate("token-3", stubGitLabBase)
	if err != nil {
		t.Fatalf("GetOrCreate(token-3) error: %v", err)
	}

	if pool.Size() != 2 {
		t.Errorf("pool.Size() = %d, want 2 after eviction", pool.Size())
	}

	// token-1 should have been evicted — re-requesting creates a new entry
	srv1, err := pool.GetOrCreate("token-1", stubGitLabBase)
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

// TestGetOrCreate_LRUPromotes verifies GetOrCreate when lru promotes.
func TestGetOrCreate_LRUPromotes(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory(), WithMaxSize(2))

	_, _ = pool.GetOrCreate("token-a", stubGitLabBase)
	_, _ = pool.GetOrCreate("token-b", stubGitLabBase)

	// Re-access token-a to promote it in LRU
	_, _ = pool.GetOrCreate("token-a", stubGitLabBase)

	// Adding token-c should evict token-b (now LRU), not token-a
	_, _ = pool.GetOrCreate("token-c", stubGitLabBase)

	if pool.Size() != 2 {
		t.Fatalf("pool.Size() = %d, want 2", pool.Size())
	}

	// Verify token-a still returns the same cached entry (not evicted)
	srvA1, _ := pool.GetOrCreate("token-a", stubGitLabBase)
	srvA2, _ := pool.GetOrCreate("token-a", stubGitLabBase)
	if srvA1 != srvA2 {
		t.Error("token-a should still be in pool after LRU promotion")
	}
}

// TestGetOrCreate_Concurrent verifies GetOrCreate when concurrent.
func TestGetOrCreate_Concurrent(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
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
			srv, err := pool.GetOrCreate(token, stubGitLabBase)
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

// TestClose verifies Close.
func TestClose(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())

	_, _ = pool.GetOrCreate("token-1", stubGitLabBase)
	_, _ = pool.GetOrCreate("token-2", stubGitLabBase)

	pool.Close()

	if pool.Size() != 0 {
		t.Errorf("pool.Size() = %d after Close(), want 0", pool.Size())
	}
}

// TestTokenHash verifies TokenHash.
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

// TestTokenSuffix covers TokenSuffix with table-driven subtests.
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

// TestWithMaxSize verifies WithMaxSize.
func TestWithMaxSize(t *testing.T) {
	cfg := testConfig(stubGitLabBase)

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
	cfg := testConfig(stubGitLabBase)

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
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())

	before := time.Now()
	_, err := pool.GetOrCreate("token-time", stubGitLabBase)
	if err != nil {
		t.Fatalf("GetOrCreate() error: %v", err)
	}
	after := time.Now()

	key := sessionKey("token-time", stubGitLabBase)
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
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory(), WithRevalidateInterval(0))

	// Should not panic — nil ctx is replaced with context.Background()
	//lint:ignore SA1012 intentionally testing nil context guard
	pool.StartRevalidation(nil) //nolint:staticcheck // SA1012
}

// TestStartRevalidation_DisabledWithZeroInterval verifies that
// StartRevalidation returns immediately when interval is zero.
func TestStartRevalidation_DisabledWithZeroInterval(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory(), WithRevalidateInterval(0))

	ctx := t.Context()

	// Should not panic and return immediately
	pool.StartRevalidation(ctx)
}

// TestStartRevalidation_CancelledContext verifies that the revalidation
// goroutine stops when the context is cancelled.
func TestStartRevalidation_CancelledContext(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory(), WithRevalidateInterval(50*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	pool.StartRevalidation(ctx)

	// Let it run briefly then cancel
	time.Sleep(10 * time.Millisecond)
	cancel()

	// Give goroutine time to exit cleanly
	time.Sleep(100 * time.Millisecond)
}

// TestGetOrCreate_FactoryError_ReturnsError verifies GetOrCreate returns error with factory error.
func TestGetOrCreate_FactoryError_ReturnsError(t *testing.T) {
	cfg := testConfig("https://gitlab.example.com")
	factoryErr := errors.New("catalog unavailable")
	pool := New(cfg, func(_ *gitlabclient.Client, _ *config.ServerConfig) (*mcp.Server, error) {
		return nil, factoryErr
	})

	server, err := pool.GetOrCreate("token", cfg.GitLabURL)
	if err == nil {
		t.Fatal("GetOrCreate() error = nil, want error")
	}
	if server != nil {
		t.Fatalf("GetOrCreate() server = %v, want nil", server)
	}
	if !strings.Contains(err.Error(), factoryErr.Error()) {
		t.Fatalf("GetOrCreate() error = %q, want factory error", err)
	}
	if pool.Size() != 0 {
		t.Fatalf("pool.Size() = %d, want 0 after factory error", pool.Size())
	}
	if stats := pool.Stats(); stats.CurrentSize != 0 {
		t.Fatalf("pool.Stats().CurrentSize = %d, want 0 after factory error", stats.CurrentSize)
	}
}

// TestGetOrCreate_NilFactory_ReturnsError verifies GetOrCreate returns error with nil factory.
func TestGetOrCreate_NilFactory_ReturnsError(t *testing.T) {
	cfg := testConfig("https://gitlab.example.com")
	pool := New(cfg, nil)

	server, err := pool.GetOrCreate("token", cfg.GitLabURL)
	if err == nil {
		t.Fatal("GetOrCreate() error = nil, want error")
	}
	if server != nil {
		t.Fatalf("GetOrCreate() server = %v, want nil", server)
	}
	if !strings.Contains(err.Error(), "server factory is nil") {
		t.Fatalf("GetOrCreate() error = %q, want server factory error", err)
	}
	if pool.Size() != 0 {
		t.Fatalf("pool.Size() = %d, want 0 after nil factory error", pool.Size())
	}
	if stats := pool.Stats(); stats.CurrentSize != 0 {
		t.Fatalf("pool.Stats().CurrentSize = %d, want 0 after nil factory error", stats.CurrentSize)
	}
}

// TestEvictByKey verifies that evictByKey removes the specified entry
// and not others.
func TestEvictByKey(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())

	_, _ = pool.GetOrCreate("token-keep", stubGitLabBase)
	_, _ = pool.GetOrCreate("token-evict", stubGitLabBase)

	if pool.Size() != 2 {
		t.Fatalf("pool.Size() = %d, want 2", pool.Size())
	}
	key := sessionKey("token-evict", stubGitLabBase)
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

// TestInsertEntry_ExistingEntry verifies the slow-path double-check branch
// deterministically without depending on goroutine scheduling: when a key is
// already present, insertEntry returns the stored server (discarding the freshly
// built one), records a cache hit, and moves the existing entry to the LRU front.
func TestInsertEntry_ExistingEntry(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory(), WithMaxSize(20))

	existingKey := "existing"
	otherKey := "other"
	existingServer := mcp.NewServer(&mcp.Implementation{Name: "existing", Version: "0.0.0"}, nil)
	otherServer := mcp.NewServer(&mcp.Implementation{Name: "other", Version: "0.0.0"}, nil)

	pool.mu.Lock()
	existingElement := pool.lru.PushBack(existingKey)
	otherElement := pool.lru.PushFront(otherKey)
	pool.entries[existingKey] = &poolEntry{server: existingServer, element: existingElement}
	pool.entries[otherKey] = &poolEntry{server: otherServer, element: otherElement}
	pool.mu.Unlock()

	// A concurrent builder lost the race: its freshly built server must be
	// discarded in favor of the already-stored one.
	rebuilt := mcp.NewServer(&mcp.Implementation{Name: "rebuilt", Version: "0.0.0"}, nil)
	server := pool.insertEntry(existingKey, "unused-token", &poolEntry{server: rebuilt})

	if server != existingServer {
		t.Fatal("insertEntry() returned the rebuilt server; want the existing cached server")
	}

	pool.mu.Lock()
	missingServer, missingOK := pool.existingServerLocked("missing")
	pool.mu.Unlock()
	if missingOK || missingServer != nil {
		t.Fatalf("existingServerLocked(missing) = (%v, %v), want (nil, false)", missingServer, missingOK)
	}
	if front := pool.lru.Front(); front == nil || front.Value != existingKey {
		t.Fatalf("front LRU key = %v, want %s", front, existingKey)
	}
	if size := pool.Size(); size != 2 {
		t.Fatalf("pool.Size() = %d, want 2 (rebuilt entry discarded)", size)
	}
	// insertEntry counts nothing. It is only reached from the slow path in
	// GetOrCreate, where the miss has already been charged, so counting a hit
	// here as well would make one call show up as both and Hits plus Misses
	// would stop being the number of calls. This test drives insertEntry
	// directly, so no counter should have moved at all.
	if stats := pool.Stats(); stats.Hits != 0 || stats.Misses != 0 {
		t.Fatalf("Hits = %d, Misses = %d; insertEntry must not touch the counters", stats.Hits, stats.Misses)
	}
}

// TestDefaultRevalidateInterval verifies that the default revalidation
// interval is 15 minutes.
func TestDefaultRevalidateInterval(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())
	if pool.revalidateInterval != DefaultRevalidateInterval {
		t.Errorf("default revalidateInterval = %v, want %v", pool.revalidateInterval, DefaultRevalidateInterval)
	}
}

// TestStats_HitsAndMisses verifies that Stats tracks cache hits and misses.
func TestStats_HitsAndMisses(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())

	// First call → miss
	_, _ = pool.GetOrCreate("token-a", stubGitLabBase)
	s := pool.Stats()
	if s.Misses != 1 {
		t.Errorf("Misses = %d after first GetOrCreate, want 1", s.Misses)
	}
	if s.Hits != 0 {
		t.Errorf("Hits = %d after first GetOrCreate, want 0", s.Hits)
	}

	// Second call with same token → hit
	_, _ = pool.GetOrCreate("token-a", stubGitLabBase)
	s = pool.Stats()
	if s.Hits != 1 {
		t.Errorf("Hits = %d after second GetOrCreate, want 1", s.Hits)
	}
	if s.Misses != 1 {
		t.Errorf("Misses = %d after second GetOrCreate, want 1", s.Misses)
	}

	// Third call with different token → another miss
	_, _ = pool.GetOrCreate("token-b", stubGitLabBase)
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
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory(), WithMaxSize(2))

	_, _ = pool.GetOrCreate("tok-1", stubGitLabBase)
	_, _ = pool.GetOrCreate("tok-2", stubGitLabBase)
	_, _ = pool.GetOrCreate("tok-3", stubGitLabBase) // evicts tok-1

	s := pool.Stats()
	if s.Evictions != 1 {
		t.Errorf("Evictions = %d after 1 LRU eviction, want 1", s.Evictions)
	}

	_, _ = pool.GetOrCreate("tok-4", stubGitLabBase) // evicts tok-2
	s = pool.Stats()
	if s.Evictions != 2 {
		t.Errorf("Evictions = %d after 2 LRU evictions, want 2", s.Evictions)
	}
}

// TestStats_EvictByKey verifies that explicit key eviction is counted.
func TestStats_EvictByKey(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())

	_, _ = pool.GetOrCreate("tok-evict", stubGitLabBase)
	key := sessionKey("tok-evict", stubGitLabBase)
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
	cfg := testConfig(stubGitLabBase)
	before := time.Now()
	pool := New(cfg, testFactory(), WithMaxSize(50))

	_, _ = pool.GetOrCreate("tok-1", stubGitLabBase)
	_, _ = pool.GetOrCreate("tok-2", stubGitLabBase)

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
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())

	_, _ = pool.GetOrCreate("tok-1", stubGitLabBase)

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
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory(), WithMaxSize(20))

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			token := "tok-" + string(rune('a'+idx%5))
			_, _ = pool.GetOrCreate(token, stubGitLabBase)
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
	cfg := testConfig(stubGitLabBase)
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
	cfg := testConfig(stubGitLabBase)
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
	cfg := testConfig(stubGitLabBase)
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
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())

	// Two live stubs rather than example.com hosts: GetOrCreate probes the
	// URL it is given, so a made-up host would mean real DNS traffic from a
	// unit test — passing only because the probe fails open.
	stubA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{}"))
	}))
	defer stubA.Close()
	stubB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{}"))
	}))
	defer stubB.Close()

	srv1, err := pool.GetOrCreate("glpat-same-token", stubA.URL)
	if err != nil {
		t.Fatalf("GetOrCreate(stub A) error: %v", err)
	}

	srv2, err := pool.GetOrCreate("glpat-same-token", stubB.URL)
	if err != nil {
		t.Fatalf("GetOrCreate(stub B) error: %v", err)
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

// userProbeServer returns a GitLab stub whose GET /api/v4/user accepts exactly
// one token and answers `status` to every other one.
func userProbeServer(t *testing.T, validToken string, status int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("PRIVATE-TOKEN")
		if token == "" {
			if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
				token = bearer
			}
		}
		if token != validToken {
			http.Error(w, `{"message":"401 Unauthorized"}`, status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "username": "testuser"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestGetOrCreate_RejectsCredentialGitLabRefuses verifies the admission check:
// a token GitLab answers 401 or 403 to never gets a pool entry.
//
// Without it the pool builds an entry for any non-empty string, so an
// unauthenticated caller obtains a full MCP session with "PRIVATE-TOKEN: x"
// and a stream of invented tokens churns the LRU.
func TestGetOrCreate_RejectsCredentialGitLabRefuses(t *testing.T) {
	for name, status := range map[string]int{
		"unauthorized": http.StatusUnauthorized,
		"forbidden":    http.StatusForbidden,
	} {
		t.Run(name, func(t *testing.T) {
			srv := userProbeServer(t, "glpat-valid", status)
			pool := New(testConfig(srv.URL), testFactory())

			got, err := pool.GetOrCreate("glpat-invented", srv.URL)
			if !errors.Is(err, ErrInvalidCredential) {
				t.Errorf("error = %v, want it to wrap ErrInvalidCredential", err)
			}
			if got != nil {
				t.Error("a refused credential must not receive a server")
			}
			if size := pool.Size(); size != 0 {
				t.Errorf("pool size = %d, want 0 — a refused credential must not occupy an entry", size)
			}
		})
	}
}

// TestGetOrCreate_AdmitsCredentialGitLabAccepts is the positive half: a token
// GitLab accepts is pooled as before.
func TestGetOrCreate_AdmitsCredentialGitLabAccepts(t *testing.T) {
	srv := userProbeServer(t, "glpat-valid", http.StatusUnauthorized)
	pool := New(testConfig(srv.URL), testFactory())

	got, err := pool.GetOrCreate("glpat-valid", srv.URL)
	if err != nil {
		t.Fatalf("GetOrCreate(valid token) error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a server for an accepted credential")
	}
	if size := pool.Size(); size != 1 {
		t.Errorf("pool size = %d, want 1", size)
	}
}

// TestGetOrCreate_AdmitsWhenNoVerdictIsAvailable pins the fail-open policy.
//
// Only an explicit 401 or 403 is a verdict about the credential. A 404 from a
// stubbed instance, a 5xx, or an unreachable host means none was obtained, and
// the entry is admitted: failing closed whenever GitLab is unreachable would
// turn an instance outage into a total denial of service.
func TestGetOrCreate_AdmitsWhenNoVerdictIsAvailable(t *testing.T) {
	tests := map[string]int{
		"not found":           http.StatusNotFound,
		"internal error":      http.StatusInternalServerError,
		"service unavailable": http.StatusServiceUnavailable,
	}
	for name, status := range tests {
		t.Run(name, func(t *testing.T) {
			srv := userProbeServer(t, "glpat-nobody", status)
			pool := New(testConfig(srv.URL), testFactory())

			got, err := pool.GetOrCreate("glpat-unknown", srv.URL)
			if err != nil {
				t.Fatalf("GetOrCreate must admit when GitLab gives no verdict, got: %v", err)
			}
			if got == nil {
				t.Error("expected a server when no verdict is available")
			}
		})
	}
}

// TestEvictIdle_ReclaimsOnlyStaleEntries verifies that the idle sweep drops
// entries unused past the timeout and leaves recently used ones alone. Without
// it an abandoned entry lives until 100 distinct token+URL pairs push it out
// of the LRU, holding a registered server and drawing a revalidation ping at
// every interval on behalf of a client that is gone.
func TestEvictIdle_ReclaimsOnlyStaleEntries(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory(), WithIdleTimeout(30*time.Minute))

	for _, token := range []string{"stale-token", "fresh-token"} {
		if _, err := pool.GetOrCreate(token, stubGitLabBase); err != nil {
			t.Fatalf("GetOrCreate(%s) error: %v", token, err)
		}
	}

	// Age one entry past the timeout without waiting for wall-clock time.
	pool.mu.Lock()
	pool.entries[sessionKey("stale-token", stubGitLabBase)].lastUsed = time.Now().Add(-time.Hour)
	pool.mu.Unlock()

	pool.evictIdle()

	if pool.Size() != 1 {
		t.Fatalf("pool.Size() = %d, want 1 after the idle sweep", pool.Size())
	}
	if _, ok := pool.entries[sessionKey("fresh-token", stubGitLabBase)]; !ok {
		t.Error("the recently used entry was evicted")
	}
	if got := pool.Stats().IdleEvictions; got != 1 {
		t.Errorf("IdleEvictions = %d, want 1", got)
	}
	// The LRU list must shrink with the map, or the next eviction walks a
	// element whose entry no longer exists.
	if pool.lru.Len() != 1 {
		t.Errorf("lru.Len() = %d, want 1", pool.lru.Len())
	}
}

// TestEvictIdle_HitRefreshesLastUsed verifies that serving a request keeps an
// entry alive. GetOrCreate answers established entries from a fast path that
// bypasses existingServerLocked, so that path has to refresh lastUsed itself;
// if it does not, the sweep reclaims servers that are actively in use.
func TestEvictIdle_HitRefreshesLastUsed(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory(), WithIdleTimeout(30*time.Minute))

	if _, err := pool.GetOrCreate("token", stubGitLabBase); err != nil {
		t.Fatalf("GetOrCreate error: %v", err)
	}
	key := sessionKey("token", stubGitLabBase)

	pool.mu.Lock()
	pool.entries[key].lastUsed = time.Now().Add(-time.Hour)
	pool.mu.Unlock()

	// A hit through the public API must reset the clock.
	if _, err := pool.GetOrCreate("token", stubGitLabBase); err != nil {
		t.Fatalf("GetOrCreate (hit) error: %v", err)
	}

	pool.evictIdle()

	if pool.Size() != 1 {
		t.Fatalf("pool.Size() = %d, want 1: a served entry must survive the sweep", pool.Size())
	}
}

// TestEvictIdle_DisabledKeepsEverything verifies that a zero timeout turns the
// sweep off, leaving the LRU bound as the only reclamation path.
func TestEvictIdle_DisabledKeepsEverything(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory(), WithIdleTimeout(0))

	if _, err := pool.GetOrCreate("token", stubGitLabBase); err != nil {
		t.Fatalf("GetOrCreate error: %v", err)
	}
	pool.mu.Lock()
	pool.entries[sessionKey("token", stubGitLabBase)].lastUsed = time.Time{}
	pool.mu.Unlock()

	pool.evictIdle()

	if pool.Size() != 1 {
		t.Errorf("pool.Size() = %d, want 1 with idle eviction disabled", pool.Size())
	}
}

// TestStartIdleEviction_DisabledReturnsWithoutGoroutine verifies the start
// helper is a no-op when the timeout is non-positive, and that a cancelled
// context stops a running sweeper.
func TestStartIdleEviction_DisabledReturnsWithoutGoroutine(t *testing.T) {
	cfg := testConfig(stubGitLabBase)

	New(cfg, testFactory(), WithIdleTimeout(0)).StartIdleEviction(t.Context())

	ctx, cancel := context.WithCancel(t.Context())
	New(cfg, testFactory(), WithIdleTimeout(time.Hour)).StartIdleEviction(ctx)
	cancel()
}

// TestGetOrCreate_OAuthMode_BuildsBearerClient verifies the pool selects the
// Bearer-authenticated client constructor when the server runs in oauth
// mode: the entry's credential probe must arrive as "Authorization: Bearer"
// and never as PRIVATE-TOKEN, which GitLab rejects for gloas- OAuth access
// tokens. In legacy mode the probe stays PRIVATE-TOKEN.
func TestGetOrCreate_OAuthMode_BuildsBearerClient(t *testing.T) {
	tests := []struct {
		name       string
		authMode   string
		wantBearer bool
	}{
		{"oauth mode sends Bearer", "oauth", true},
		{"legacy mode keeps PRIVATE-TOKEN", "legacy", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bearer, private string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if bearer == "" && private == "" {
					bearer, private = r.Header.Get("Authorization"), r.Header.Get("PRIVATE-TOKEN")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte("{}"))
			}))
			defer srv.Close()

			cfg := testConfig(srv.URL)
			cfg.AuthMode = tt.authMode
			pool := New(cfg, testFactory())
			if _, err := pool.GetOrCreate("gloas-sometoken", srv.URL); err != nil {
				t.Fatalf("GetOrCreate() error: %v", err)
			}
			gotBearer := bearer != ""
			if gotBearer != tt.wantBearer {
				t.Errorf("probe auth: Authorization=%q PRIVATE-TOKEN set=%v, want bearer=%v", bearer, private != "", tt.wantBearer)
			}
			if tt.wantBearer && private != "" {
				t.Errorf("oauth-mode probe also sent PRIVATE-TOKEN %q", private)
			}
		})
	}
}

// TestIdentityFor_ResolvesTheUserBehindAPooledToken verifies that the pool
// records who a credential belongs to when it builds the entry, and answers
// for it afterwards from memory.
//
// HTTP legacy mode mounts no bearer middleware, so nothing populates
// req.Extra.TokenInfo there and tool handlers used to resolve the zero
// identity — log lines carried no user at all. This is where the answer comes
// from now.
func TestIdentityFor_ResolvesTheUserBehindAPooledToken(t *testing.T) {
	var userCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		userCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":4242,"username":"pooled-user"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.IgnoreScopes = true
	pool := New(cfg, func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	})

	if _, ok := pool.IdentityFor("glpat-x", srv.URL); ok {
		t.Error("IdentityFor should report no entry before GetOrCreate has run")
	}

	if _, err := pool.GetOrCreate("glpat-x", srv.URL); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	identity, ok := pool.IdentityFor("glpat-x", srv.URL)
	if !ok {
		t.Fatal("IdentityFor found no entry for a token the pool just built")
	}
	if !identity.Resolved() {
		t.Fatal("identity should be resolved")
	}
	if identity.UserID != "4242" || identity.Username != "pooled-user" {
		t.Errorf("identity = %+v, want {4242 pooled-user}", identity)
	}

	// Resolution happens once per entry, not once per request: a second
	// lookup must not reach GitLab again.
	before := userCalls.Load()
	if _, stillThere := pool.IdentityFor("glpat-x", srv.URL); !stillThere {
		t.Fatal("second IdentityFor lost the entry")
	}
	if got := userCalls.Load(); got != before {
		t.Errorf("/user calls went from %d to %d; the answer must come from the entry", before, got)
	}
}

// TestIdentityFor_UnresolvableUser_ReportsUnknown verifies that a /user
// response carrying no id leaves the identity unresolved rather than
// inventing one.
//
// The credential probe only asks whether GitLab accepts the token, so a 200
// with an empty body builds the entry perfectly well. Formatting the decoded
// zero would produce the string "0", which reads as a resolved user to every
// caller and puts a user that does not exist into the logs.
func TestIdentityFor_UnresolvableUser_ReportsUnknown(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.IgnoreScopes = true
	pool := New(cfg, func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	})

	if _, err := pool.GetOrCreate("glpat-y", srv.URL); err != nil {
		t.Fatalf("GetOrCreate should succeed when the credential is accepted: %v", err)
	}

	identity, ok := pool.IdentityFor("glpat-y", srv.URL)
	if !ok {
		t.Fatal("the entry should exist even without an identity")
	}
	if identity.Resolved() {
		t.Errorf("identity = %+v, want unresolved", identity)
	}
}

// TestWithBaseContext_ShutdownReleasesEntryConstruction verifies that the
// GitLab lookups which build an entry are bounded by a lifetime the caller
// owns, so shutdown does not leave them running.
//
// They are deliberately not derived from the request that triggered them — an
// entry is shared by every request carrying the same credential, so one client
// disconnecting must not abort work others are waiting on. That justifies
// ignoring the request context; it does not justify ignoring shutdown, which
// is what context.Background() did: the probe went on until its own five
// second timeout with nothing left to serve.
//
// Cancellation releases the call rather than failing construction. The
// credential probe reports "not rejected" when it cannot reach GitLab at all,
// which is the correct reading — an unreachable instance is not a verdict on
// the token — so what is asserted here is promptness, not an error.
func TestWithBaseContext_ShutdownReleasesEntryConstruction(t *testing.T) {
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, r *http.Request) {
		// Hold the probe open until the test cancels the base context, so
		// cancellation is what ends the call rather than a race with the
		// response.
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	defer close(release)

	baseCtx, shutdown := context.WithCancel(context.Background())
	cfg := testConfig(srv.URL)
	cfg.IgnoreScopes = true
	pool := New(cfg, func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	}, WithBaseContext(func() context.Context { return baseCtx }))

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		_, _ = pool.GetOrCreate("glpat-shutdown", srv.URL)
		done <- time.Since(start)
	}()

	// Give the probe time to reach the handler before shutting down.
	time.Sleep(50 * time.Millisecond)
	shutdown()

	select {
	case elapsed := <-done:
		// credentialCheckTimeout is 5s; anything near it means the lookups
		// ran to their own bound with the server already gone.
		if elapsed > 2*time.Second {
			t.Errorf("entry construction took %v after shutdown; it should be released promptly", elapsed)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("entry construction never returned after the base context was cancelled")
	}
}

// TestNew_WithoutBaseContext_StillBuildsEntries verifies the default: a pool
// created without the option behaves as before, so every existing caller and
// test keeps working.
func TestNew_WithoutBaseContext_StillBuildsEntries(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":9,"username":"default-ctx"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.IgnoreScopes = true
	pool := New(cfg, func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	})

	if _, err := pool.GetOrCreate("glpat-default", srv.URL); err != nil {
		t.Fatalf("GetOrCreate without WithBaseContext: %v", err)
	}
	// A nil function, and a function returning nil, both fall back to
	// Background rather than turning into a panic on the first lookup.
	pool2 := New(cfg, func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	}, WithBaseContext(nil))
	if _, err := pool2.GetOrCreate("glpat-nil-ctx", srv.URL); err != nil {
		t.Fatalf("GetOrCreate with a nil base-context function: %v", err)
	}
	pool3 := New(cfg, func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	}, WithBaseContext(func() context.Context { return nil }))
	if _, err := pool3.GetOrCreate("glpat-nil-return", srv.URL); err != nil {
		t.Fatalf("GetOrCreate with a base-context function returning nil: %v", err)
	}
}

// TestGetOrCreate_ConcurrentCallersShareOneBuild verifies that callers racing
// for the same credential collapse into a single build.
//
// A client that opens several connections at once — the normal startup burst —
// used to cost one credential probe, one tier lookup, one scope lookup and one
// identity lookup per connection, all for a single credential, with every
// result but one discarded. The waiters block exactly as long as they would
// have blocked building it themselves, so nothing is slower.
func TestGetOrCreate_ConcurrentCallersShareOneBuild(t *testing.T) {
	var builds atomic.Int32
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		builds.Add(1)
		// Long enough that every caller is inside the race window.
		time.Sleep(50 * time.Millisecond)
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	})

	const callers = 20
	var wg sync.WaitGroup
	servers := make([]*mcp.Server, callers)
	for i := range callers {
		wg.Go(func() {
			srv, _ := pool.GetOrCreate("glpat-same", stubGitLabBase)
			servers[i] = srv
		})
	}
	wg.Wait()

	if got := builds.Load(); got != 1 {
		t.Errorf("factory ran %d times for %d concurrent callers sharing one credential, want 1", got, callers)
	}
	for i, srv := range servers {
		if srv == nil {
			t.Errorf("caller %d got no server", i)
			continue
		}
		if srv != servers[0] {
			t.Errorf("caller %d got a different server; every caller must share the one entry", i)
		}
	}
	// Every caller found no entry when it asked, so every caller is a miss:
	// Hits plus Misses must still be the number of calls.
	if stats := pool.Stats(); stats.Hits+stats.Misses != callers {
		t.Errorf("Hits(%d) + Misses(%d) = %d, want %d", stats.Hits, stats.Misses, stats.Hits+stats.Misses, callers)
	}
}

// TestGetOrCreate_ConcurrentDistinctKeysDoNotSerialize verifies the other half
// of the trade: collapsing same-key builds must not put different credentials
// behind one another, which is what building under the pool lock would do.
func TestGetOrCreate_ConcurrentDistinctKeysDoNotSerialize(t *testing.T) {
	release := make(chan struct{})
	var started atomic.Int32

	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		started.Add(1)
		<-release
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	})

	const callers = 5
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			_, _ = pool.GetOrCreate(fmt.Sprintf("glpat-distinct-%d", i), stubGitLabBase)
		})
	}

	// All five builds must be in flight at once. If they were serialized, only
	// the first would have started.
	deadline := time.Now().Add(5 * time.Second)
	for started.Load() < callers && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	inFlight := started.Load()
	close(release)
	wg.Wait()

	if inFlight != callers {
		t.Errorf("%d of %d distinct-key builds were in flight together; distinct credentials are being serialized", inFlight, callers)
	}
}

// TestGetOrCreate_AfterLifetimeCancel_DoesNotBuild verifies that once the
// pool's base context is done, GetOrCreate refuses to build a new entry rather
// than running the credential probe, tier and scope lookups and the factory
// after shutdown has begun.
//
// The credential probe reports "not rejected" when it cannot reach GitLab,
// which on a cancelled context would wave the build through; the factory has
// no context of its own, so its tool-registration work would still run and
// could delay a graceful shutdown. The guard stops it at the door.
func TestGetOrCreate_AfterLifetimeCancel_DoesNotBuild(t *testing.T) {
	var builds atomic.Int32
	baseCtx, cancel := context.WithCancel(context.Background())
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		builds.Add(1)
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	}, WithBaseContext(func() context.Context { return baseCtx }))

	cancel()

	srv, err := pool.GetOrCreate("glpat-after-shutdown", stubGitLabBase)
	if err == nil {
		t.Error("GetOrCreate built an entry after the lifetime was cancelled")
	}
	if srv != nil {
		t.Error("a server was returned despite the cancelled lifetime")
	}
	if builds.Load() != 0 {
		t.Errorf("the factory ran %d times after shutdown; it should not run at all", builds.Load())
	}
	if pool.Size() != 0 {
		t.Errorf("pool.Size() = %d, want 0 — nothing should have been inserted", pool.Size())
	}
}

// TestGetOrCreate_FactoryReturnsNoServer_IsNeverCached verifies that a
// factory which reports success while handing back no server is refused at
// construction, and that nothing is left behind for the next caller.
//
// The second call is the assertion that matters. Rejecting the nil on the way
// out of the build only protects the caller who triggered it; if the entry is
// still cached, every later caller for that credential takes the fast path,
// finds it, and receives a nil server with a nil error — the dereference the
// check exists to prevent, now with nothing to diagnose it by. One transient
// factory fault would poison that key for the life of the process.
func TestGetOrCreate_FactoryReturnsNoServer_IsNeverCached(t *testing.T) {
	pool := New(testConfig(stubGitLabBase), func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return nil, nil //nolint:nilnil // the point of the test: success reported with nothing built
	})

	srv, err := pool.GetOrCreate("glpat-nil-server", stubGitLabBase)
	if err == nil {
		t.Fatal("GetOrCreate() error = nil, want a refusal when the factory built nothing")
	}
	if srv != nil {
		t.Errorf("GetOrCreate() server = %v, want nil alongside the error", srv)
	}
	if pool.Size() != 0 {
		t.Errorf("Size() = %d, want 0: a failed build must leave nothing cached", pool.Size())
	}

	// The same credential again must fail the same way, not silently hand
	// back the cached nil.
	srv, err = pool.GetOrCreate("glpat-nil-server", stubGitLabBase)
	if err == nil {
		t.Fatal("second GetOrCreate() error = nil: a poisoned entry survived the first failure")
	}
	if srv != nil {
		t.Errorf("second GetOrCreate() server = %v, want nil", srv)
	}
}

// TestLifetime_BaseContextYieldingNil_FallsBackToBackground verifies that a
// base-context function returning nil does not propagate that nil into entry
// construction.
//
// The function is supplied by the embedder, so it can legitimately return nil
// before the server's context exists. Passing nil down would panic inside the
// GitLab client rather than at the call site, so the pool substitutes a real
// context and carries on: losing shutdown propagation for that build is a far
// smaller failure than crashing the request.
func TestLifetime_BaseContextYieldingNil_FallsBackToBackground(t *testing.T) {
	pool := New(testConfig(stubGitLabBase), func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	}, WithBaseContext(func() context.Context { return nil }))

	if pool.lifetime() == nil {
		t.Fatal("lifetime() = nil, want a usable context")
	}
	srv, err := pool.GetOrCreate("glpat-nil-base-context", stubGitLabBase)
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v, want the build to proceed", err)
	}
	if srv == nil {
		t.Error("GetOrCreate() server = nil, want a server")
	}
}

// TestStartIdleEviction_NilContextAndDisabled_DoesNotPanic verifies that idle
// eviction tolerates a nil context and stays quiet when it is switched off.
//
// A caller that passes an uninitialized context gets a defensive substitute
// rather than a panic in a background goroutine, where it would take the
// process down with a stack trace pointing away from the mistake.
func TestStartIdleEviction_NilContextAndDisabled_DoesNotPanic(t *testing.T) {
	pool := New(testConfig(stubGitLabBase), func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	}, WithIdleTimeout(0))

	//nolint:staticcheck // deliberately passes a nil context to exercise the guard
	pool.StartIdleEviction(nil)

	// Disabled means no sweeper: an entry stays put however long it sits.
	if _, err := pool.GetOrCreate("glpat-never-evicted", stubGitLabBase); err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}
	if pool.Size() != 1 {
		t.Errorf("Size() = %d, want 1", pool.Size())
	}
}
