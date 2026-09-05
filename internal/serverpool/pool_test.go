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
	"strconv"
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
		GitLabURL:      baseURL,
		GitLabToken:    "default-token",
		SkipTLSVerify:  false,
		Tier:           edition.Free,
		TierExplicit:   true,
		IgnoreScopes:   true,
		DisableRetries: true,
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

// TestGetOrCreate_A401OnACallDropsTheEntry verifies the first data call
// GitLab refuses drops the entry, so the next request re-verifies the
// credential instead of being served a revoked token until the periodic
// check notices, which could be an hour later. The signal is the 401 itself,
// seen by the client's own transport, so every tool, resource and prompt
// call counts and no text is matched.
func TestGetOrCreate_A401OnACallDropsTheEntry(t *testing.T) {
	var revoked atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if revoked.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
			return
		}
		_, _ = w.Write([]byte("[]"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	pool := New(testConfig(srv.URL), testFactory())
	if _, err := pool.GetOrCreate("glpat-live", srv.URL); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	pool.mu.RLock()
	entry := pool.entries[sessionKey("glpat-live", srv.URL)]
	pool.mu.RUnlock()
	if entry == nil {
		t.Fatal("no entry after GetOrCreate")
	}

	// A call GitLab answers changes nothing.
	if _, _, err := entry.client.GL().Projects.ListProjects(nil); err != nil {
		t.Fatalf("a call before revocation failed: %v", err)
	}
	if got := pool.Size(); got != 1 {
		t.Fatalf("pool size = %d after an answered call, want 1", got)
	}

	// The token is revoked: the next call is refused, and the entry with it.
	// The drop runs off the calling goroutine, so it is waited for rather
	// than read straight after the call.
	revoked.Store(true)
	if _, _, err := entry.client.GL().Projects.ListProjects(nil); err == nil {
		t.Fatal("the call after revocation succeeded, so the stub did not refuse it")
	}
	deadline := time.Now().Add(5 * time.Second)
	for pool.Size() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("pool size = %d five seconds after a 401 on a call, want 0: the entry outlived its credential", pool.Size())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := pool.Stats().RejectedCredentialEvictions; got != 1 {
		t.Errorf("RejectedCredentialEvictions = %d, want 1", got)
	}

	// The next request rebuilds, which is the re-verification. The old
	// client's later refusals fire nothing: the hook is once per client.
	revoked.Store(false)
	if _, err := pool.GetOrCreate("glpat-live", srv.URL); err != nil {
		t.Fatalf("GetOrCreate after the drop: %v", err)
	}
	revoked.Store(true)
	_, _, _ = entry.client.GL().Projects.ListProjects(nil)
	time.Sleep(50 * time.Millisecond)
	if got := pool.Size(); got != 1 {
		t.Errorf("pool size = %d after the old client's second 401, want the rebuilt entry kept", got)
	}
	if got := pool.Stats().RejectedCredentialEvictions; got != 1 {
		t.Errorf("RejectedCredentialEvictions = %d after the old client's second 401, want still 1", got)
	}
}

// TestGetOrCreate_ARejectedEntryIsRebuiltBeforeTheEvictionLands verifies the
// window between GitLab's 401 and the eviction it schedules: the entry is
// marked rejected on the calling goroutine, and a request that finds the mark
// rebuilds instead of reusing the refused credential. The eviction runs on a
// goroutine of its own, so without the mark a request in that window was
// served the entry GitLab had just refused.
func TestGetOrCreate_ARejectedEntryIsRebuiltBeforeTheEvictionLands(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	pool := New(testConfig(srv.URL), testFactory())
	first, err := pool.GetOrCreate("glpat-live", srv.URL)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	key := sessionKey("glpat-live", srv.URL)
	pool.mu.RLock()
	entry := pool.entries[key]
	pool.mu.RUnlock()
	if entry == nil {
		t.Fatal("no entry after GetOrCreate")
	}

	// The mark alone, with the eviction that would follow it still pending.
	entry.rejected.Store(true)
	second, err := pool.GetOrCreate("glpat-live", srv.URL)
	if err != nil {
		t.Fatalf("GetOrCreate after the mark: %v", err)
	}
	if second == first {
		t.Fatal("the request that found the rejected mark was served the refused entry")
	}
	if got := pool.Size(); got != 1 {
		t.Errorf("pool size = %d after the rebuild, want 1", got)
	}
	if got := pool.Stats().RejectedCredentialEvictions; got != 1 {
		t.Errorf("RejectedCredentialEvictions = %d, want 1: the rebuild is the eviction", got)
	}

	// The pending eviction finds another entry under the key and leaves the
	// rebuilt one alone.
	pool.evictRejectedCredential(key, entry)
	if got := pool.Size(); got != 1 {
		t.Errorf("pool size = %d after the late eviction, want the rebuilt entry kept", got)
	}
	if got := pool.Stats().RejectedCredentialEvictions; got != 1 {
		t.Errorf("RejectedCredentialEvictions = %d after the late eviction, want still 1", got)
	}
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

// TestGetOrCreate_ReadOnlyTokenGetsAReadOnlySurface verifies the half of the
// per-action write gate that actually protects GitLab: a token whose scopes
// cannot write is served a read-only entry, whatever the deployment's own
// mode is.
//
// This is what makes admitting a read_api token safe. The door no longer
// demands the deployment's scope, so the narrowing has to happen here — and
// per entry, since an entry is per token: one client's read_api credential
// must not narrow another client's api credential.
func TestGetOrCreate_ReadOnlyTokenGetsAReadOnlySurface(t *testing.T) {
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
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "scopes": scopes, "active": true})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.IgnoreScopes = false
	built := map[string]*config.ServerConfig{}
	factory := func(_ *gitlabclient.Client, entryCfg *config.ServerConfig) (*mcp.Server, error) {
		built[strings.Join(entryCfg.TokenScopes, ",")] = entryCfg
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	}
	pool := New(cfg, factory)

	tests := []struct {
		name         string
		token        string
		scopeKey     string
		wantReadOnly bool
	}{
		{
			name:         "a token that cannot write gets a read-only surface",
			token:        "glpat-read",
			scopeKey:     "read_api",
			wantReadOnly: true,
		},
		{
			name:         "a write-capable token keeps the full surface",
			token:        "glpat-api",
			scopeKey:     "api",
			wantReadOnly: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := pool.GetOrCreate(tt.token, srv.URL); err != nil {
				t.Fatalf("GetOrCreate(%s) error: %v", tt.token, err)
			}
			entryCfg, ok := built[tt.scopeKey]
			if !ok {
				t.Fatalf("no entry was built for scopes %q; built: %v", tt.scopeKey, built)
			}
			if entryCfg.ReadOnly != tt.wantReadOnly {
				t.Errorf("ReadOnly = %v, want %v for scopes %q", entryCfg.ReadOnly, tt.wantReadOnly, tt.scopeKey)
			}
		})
	}

	// Narrowing one entry must never reach the shared deployment config: an
	// entry is per token, so one client's read_api credential cannot narrow
	// another client's api one.
	if cfg.ReadOnly {
		t.Error("narrowing one entry mutated the shared deployment config")
	}
}

// TestGetOrCreate_UsesScopesTheCallerAlreadyResolved verifies that scopes
// handed in are used as-is, without asking GitLab again.
//
// OAuth mode depends on this: verifying the bearer token already read the
// scopes, and the PAT self endpoint the pool would otherwise ask does not
// answer for an OAuth access token at all — so a read_api OAuth token would
// look like "authority unknown" and be served the full catalog.
func TestGetOrCreate_UsesScopesTheCallerAlreadyResolved(t *testing.T) {
	var selfCalls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/personal_access_tokens/self", func(w http.ResponseWriter, _ *http.Request) {
		selfCalls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.IgnoreScopes = false
	var gotReadOnly bool
	var gotScopes []string
	factory := func(_ *gitlabclient.Client, entryCfg *config.ServerConfig) (*mcp.Server, error) {
		gotReadOnly = entryCfg.ReadOnly
		gotScopes = append([]string(nil), entryCfg.TokenScopes...)
		return mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil), nil
	}
	pool := New(cfg, factory)

	if _, err := pool.GetOrCreateWithScopes("gloas-read", srv.URL, []string{"read_api"}); err != nil {
		t.Fatalf("GetOrCreateWithScopes() error: %v", err)
	}

	if !gotReadOnly {
		t.Error("an entry built from read_api scopes must be read-only")
	}
	if len(gotScopes) != 1 || gotScopes[0] != "read_api" {
		t.Errorf("TokenScopes = %v, want [read_api]", gotScopes)
	}
	if n := selfCalls.Load(); n != 0 {
		t.Errorf("the PAT self endpoint was called %d times; supplied scopes must not be re-detected", n)
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
	pool.entries[existingKey] = &Entry{server: existingServer, element: existingElement}
	pool.entries[otherKey] = &Entry{server: otherServer, element: otherElement}
	pool.mu.Unlock()

	// A concurrent builder lost the race: its freshly built server must be
	// discarded in favor of the already-stored one.
	rebuilt := mcp.NewServer(&mcp.Implementation{Name: "rebuilt", Version: "0.0.0"}, nil)
	entry := pool.insertEntry(existingKey, "unused-token", &Entry{server: rebuilt})

	if entry.Server() != existingServer {
		t.Fatal("insertEntry() returned the rebuilt server; want the existing cached server")
	}

	pool.mu.Lock()
	missingEntry, missingOK := pool.existingEntryLocked("missing")
	pool.mu.Unlock()
	if missingOK || missingEntry != nil {
		t.Fatalf("existingEntryLocked(missing) = (%v, %v), want (nil, false)", missingEntry, missingOK)
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
	pool.entries[goodKey] = &Entry{
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
	pool.entries[badKey] = &Entry{
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
	pool.entries[key] = &Entry{
		server:        mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil),
		client:        client,
		element:       elem,
		createdAt:     time.Now(),
		lastValidated: time.Now(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // the wait below can end the test early, and the sweep must not outlive it
	pool.StartRevalidation(ctx)

	// Wait for the first tick to complete, rather than for a span the sweep
	// is expected to fit in.
	deadline := time.Now().Add(5 * time.Second)
	for pool.Stats().RevalidationsSucceeded < 1 {
		if time.Now().After(deadline) {
			t.Fatal("no revalidation completed within 5s")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()

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
		t.Run(token, func(t *testing.T) {
			if _, err := pool.GetOrCreate(token, stubGitLabBase); err != nil {
				t.Fatalf("GetOrCreate(%s) error: %v", token, err)
			}
		})
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
// entry alive. GetOrCreateEntry answers established entries from a fast path
// that bypasses existingEntryLocked, so that path has to refresh lastUsed itself;
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

// TestPool_OnEvictFiresOnEveryRemovalPath pins that a caller keeping its own
// per-server state is told about every way an entry can leave the pool.
//
// cmd/server maps each pooled server to the tag its session IDs carry. That map
// had no cleanup path, so it grew past --max-http-clients on credential churn
// and every stale key kept a whole server — its GitLab client and its
// registered tool surface — reachable. The pool's size bound only means
// something if every removal path reports itself, which is why dropEntry is now
// the single place an entry leaves the map.
func TestPool_OnEvictFiresOnEveryRemovalPath(t *testing.T) {
	newRecordingPool := func(evicted *[]*mcp.Server, mu *sync.Mutex, opts ...Option) *ServerPool {
		record := WithOnEvict(func(entry *Entry) {
			mu.Lock()
			defer mu.Unlock()
			*evicted = append(*evicted, entry.Server())
		})
		return New(testConfig(stubGitLabBase), testFactory(), append([]Option{record}, opts...)...)
	}

	t.Run("LRU pressure", func(t *testing.T) {
		var mu sync.Mutex
		var evicted []*mcp.Server

		pool := newRecordingPool(&evicted, &mu, WithMaxSize(1))
		first, err := pool.GetOrCreate("glpat-one", stubGitLabBase)
		if err != nil {
			t.Fatalf("first entry: %v", err)
		}
		if _, secondErr := pool.GetOrCreate("glpat-two", stubGitLabBase); secondErr != nil {
			t.Fatalf("second entry: %v", secondErr)
		}

		mu.Lock()
		defer mu.Unlock()
		if len(evicted) != 1 {
			t.Fatalf("onEvict fired %d times, want 1", len(evicted))
		}
		if evicted[0] != first {
			t.Error("onEvict named a server other than the one the pool dropped")
		}
	})

	t.Run("Close", func(t *testing.T) {
		var mu sync.Mutex
		var evicted []*mcp.Server

		pool := newRecordingPool(&evicted, &mu)
		for _, token := range []string{"glpat-a", "glpat-b"} {
			t.Run(token, func(t *testing.T) {
				if _, err := pool.GetOrCreate(token, stubGitLabBase); err != nil {
					t.Fatalf("entry %s: %v", token, err)
				}
			})
		}
		pool.Close()

		mu.Lock()
		defer mu.Unlock()
		if len(evicted) != 2 {
			t.Errorf("onEvict fired %d times on Close, want 2 — shutdown must release the caller's state too", len(evicted))
		}
	})

	t.Run("no callback registered", func(t *testing.T) {
		pool := New(testConfig(stubGitLabBase), testFactory(), WithMaxSize(1))
		if _, err := pool.GetOrCreate("glpat-one", stubGitLabBase); err != nil {
			t.Fatalf("first entry: %v", err)
		}
		if _, err := pool.GetOrCreate("glpat-two", stubGitLabBase); err != nil {
			t.Fatalf("second entry: %v", err)
		}
		if got := pool.Size(); got != 1 {
			t.Errorf("Size() = %d, want 1 — eviction must work with no callback registered", got)
		}
	})
}

// TestStartRevalidation_PanickingEvictionCallback_DoesNotKillTheProcess covers
// the recover the revalidation goroutine runs behind.
//
// Eviction calls back into code the pool does not own — the HTTP server's
// callback that forgets a session's tags — and a panic there arrives on the
// pool's own goroutine, where nothing above it can recover: an unrecovered
// panic on a background goroutine takes the whole process down, so a bug in a
// callback would stop a server that is otherwise serving every other
// credential. The test's assertion is that it returns at all; without the
// recover the test binary dies rather than failing.
func TestStartRevalidation_PanickingEvictionCallback_DoesNotKillTheProcess(t *testing.T) {
	var reachable atomic.Bool
	reachable.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !reachable.Load() {
			http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0"}`))
	}))
	defer srv.Close()

	pool := New(testConfig(srv.URL), testFactory(),
		WithRevalidateInterval(10*time.Millisecond),
		WithOnEvict(func(*Entry) { panic("a callback outside the pool") }),
	)
	if _, err := pool.GetOrCreate("glpat-revalidate", srv.URL); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reachable.Store(false)
	pool.StartRevalidation(ctx)

	deadline := time.Now().Add(10 * time.Second)
	for pool.Size() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("the entry whose token stopped verifying was never evicted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The panic unwinds after the entry leaves the map and the failure is
	// counted after that, so waiting for the count is what separates
	// "recovered" from "about to crash the binary".
	deadline = time.Now().Add(5 * time.Second)
	for pool.Stats().RevalidationsFailed == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the failed revalidation was not counted")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// revalidationEntry inserts one entry into pool pointing at baseURL, with
// lastValidated set to the given moment, and returns its key.
func revalidationEntry(t *testing.T, pool *ServerPool, baseURL, token string, validatedAt time.Time) string {
	t.Helper()

	client, err := gitlabclient.NewClientWithTokenRetries(baseURL, token, false, true)
	if err != nil {
		t.Fatalf("building a client for %q: %v", token, err)
	}
	key := tokenHash(token)
	pool.entries[key] = &Entry{
		server:        mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil),
		client:        client,
		element:       pool.lru.PushFront(key),
		createdAt:     validatedAt,
		lastValidated: validatedAt,
		lastUsed:      validatedAt,
	}
	return key
}

// TestRevalidateAll_EvictsOnlyOnCredentialVerdict verifies that revalidation
// drops an entry only when GitLab actually judged the credential, and leaves
// it alone whenever the check failed to reach a verdict.
//
// Evicting on any error at all makes an instance that is unreachable, or
// answers 500 for ten seconds, drop every tenant's entry at once — and each is
// then rebuilt on its next request with a fresh credential probe, tier lookup,
// scope lookup and identity lookup, a thundering herd against an instance that
// has just come back. Admission already draws this distinction; the periodic
// path did not.
//
// The retained rows also assert lastValidated did not move, so a transient
// failure cannot pass for a successful check and postpone the real one.
func TestRevalidateAll_EvictsOnlyOnCredentialVerdict(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		unreachable   bool
		wantEvicted   bool
		wantFailed    int64
		wantTransient int64
		wantSucceeded int64
		wantRevalid   bool // lastValidated moved forward
	}{
		{name: "gitlab rejects the credential with 401", status: http.StatusUnauthorized, wantEvicted: true, wantFailed: 1},
		{name: "gitlab rejects the credential with 403", status: http.StatusForbidden, wantEvicted: true, wantFailed: 1},
		{name: "instance answers 500", status: http.StatusInternalServerError, wantTransient: 1},
		{name: "instance answers 404", status: http.StatusNotFound, wantTransient: 1},
		{name: "instance is unreachable", unreachable: true, wantTransient: 1},
		{name: "instance answers 200", status: http.StatusOK, wantSucceeded: 1, wantRevalid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if tt.status == http.StatusOK {
					_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abc"}`))
					return
				}
				http.Error(w, fmt.Sprintf(`{"message":"%d"}`, tt.status), tt.status)
			}))
			baseURL := srv.URL
			if tt.unreachable {
				// Closing it first leaves a port nothing answers on, which is
				// a dial error rather than any HTTP status.
				srv.Close()
			} else {
				t.Cleanup(srv.Close)
			}

			pool := New(testConfig(stubGitLabBase), testFactory())
			validatedAt := time.Now().Add(-30 * time.Minute)
			key := revalidationEntry(t, pool, baseURL, "tok-"+tt.name, validatedAt)

			pool.revalidateAll(context.Background())

			entry, stillThere := pool.entries[key]
			if stillThere == tt.wantEvicted {
				t.Errorf("entry present = %v, want evicted = %v", stillThere, tt.wantEvicted)
			}
			assertRevalidationCounts(t, pool.Stats(), tt.wantFailed, tt.wantTransient, tt.wantSucceeded)
			if stillThere {
				if moved := entry.lastValidated.After(validatedAt); moved != tt.wantRevalid {
					t.Errorf("lastValidated moved = %v, want %v", moved, tt.wantRevalid)
				}
			}
		})
	}
}

// assertRevalidationCounts checks the three revalidation outcomes a round can
// record, so a verdict is never counted as a transient failure or the reverse.
func assertRevalidationCounts(t *testing.T, s Snapshot, wantFailed, wantTransient, wantSucceeded int64) {
	t.Helper()
	if s.RevalidationsFailed != wantFailed {
		t.Errorf("RevalidationsFailed = %d, want %d", s.RevalidationsFailed, wantFailed)
	}
	if s.RevalidationsTransient != wantTransient {
		t.Errorf("RevalidationsTransient = %d, want %d", s.RevalidationsTransient, wantTransient)
	}
	if s.RevalidationsSucceeded != wantSucceeded {
		t.Errorf("RevalidationsSucceeded = %d, want %d", s.RevalidationsSucceeded, wantSucceeded)
	}
}

// TestGetOrCreate_CredentialCeilingForcesRebuild verifies an entry stops
// answering from cache once its credential has gone unchecked for longer than
// the pool's ceiling, and is rebuilt — which re-runs the credential probe.
//
// This is the bound that survives an operator turning revalidation off.
// Without it the fast path returns a cached entry and refreshes lastUsed with
// no re-verification, so an entry in continuous use is never re-checked and a
// revoked token keeps its admitted surface for the life of the process.
func TestGetOrCreate_CredentialCeilingForcesRebuild(t *testing.T) {
	tests := []struct {
		name      string
		age       time.Duration
		wait      time.Duration
		wantStale int64
		wantProbe bool // the credential was probed again on the second call
	}{
		{name: "inside the ceiling", age: time.Hour, wantStale: 0},
		{name: "past the ceiling", age: time.Millisecond, wait: 20 * time.Millisecond, wantStale: 1, wantProbe: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var probes atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/user") {
					probes.Add(1)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":7,"username":"probe","version":"17.0.0"}`))
			}))
			t.Cleanup(srv.Close)

			pool := New(testConfig(srv.URL), testFactory(), WithMaxCredentialAge(tt.age))

			first := mustGetOrCreate(t, pool, "glpat-ceiling", srv.URL)
			afterFirst := probes.Load()
			if afterFirst == 0 {
				t.Fatal("the first call did not probe the credential at all")
			}

			if tt.wait > 0 {
				time.Sleep(tt.wait)
			}
			second := mustGetOrCreate(t, pool, "glpat-ceiling", srv.URL)

			if probed := probes.Load() > afterFirst; probed != tt.wantProbe {
				t.Errorf("credential re-probed = %v, want %v", probed, tt.wantProbe)
			}
			if got := pool.Stats().StaleCredentialEvictions; got != tt.wantStale {
				t.Errorf("StaleCredentialEvictions = %d, want %d", got, tt.wantStale)
			}
			if sameServer := first == second; sameServer == tt.wantProbe {
				t.Errorf("second call returned the same server = %v, want %v", sameServer, !tt.wantProbe)
			}
		})
	}
}

// mustGetOrCreate returns the pooled server for a credential, failing the test
// if the pool refuses it.
func mustGetOrCreate(t *testing.T, pool *ServerPool, token, gitlabURL string) *mcp.Server {
	t.Helper()
	server, err := pool.GetOrCreate(token, gitlabURL)
	if err != nil {
		t.Fatalf("GetOrCreate(%q): %v", token, err)
	}
	return server
}

// TestGetOrCreate_CredentialCeilingRefusesARevokedToken verifies the rebuild
// the ceiling forces is a real check: once GitLab starts refusing the
// credential, the next request past the ceiling is refused too rather than
// being served the entry admitted before the revocation.
func TestGetOrCreate_CredentialCeilingRefusesARevokedToken(t *testing.T) {
	var revoked atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if revoked.Load() && strings.HasSuffix(r.URL.Path, "/user") {
			http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"probe","version":"17.0.0"}`))
	}))
	t.Cleanup(srv.Close)

	// Revalidation off, which is the supported setting that used to remove
	// the upper bound on the revocation window altogether.
	pool := New(testConfig(srv.URL), testFactory(),
		WithRevalidateInterval(0),
		WithMaxCredentialAge(time.Millisecond),
	)

	if _, err := pool.GetOrCreate("glpat-revoked", srv.URL); err != nil {
		t.Fatalf("GetOrCreate before revocation: %v", err)
	}

	revoked.Store(true)
	time.Sleep(20 * time.Millisecond)

	_, err := pool.GetOrCreate("glpat-revoked", srv.URL)
	if !errors.Is(err, ErrInvalidCredential) {
		t.Errorf("GetOrCreate after revocation error = %v, want %v", err, ErrInvalidCredential)
	}
}

// TestWithMaxCredentialAge_CannotBeDisabled verifies the ceiling option
// narrows but never removes the bound: a non-positive value falls back to the
// default rather than turning the check off, and a value beyond the supported
// range is clamped down.
func TestWithMaxCredentialAge_CannotBeDisabled(t *testing.T) {
	tests := []struct {
		name string
		set  time.Duration
		want time.Duration
	}{
		{name: "zero keeps the default", set: 0, want: DefaultMaxCredentialAge},
		{name: "negative keeps the default", set: -time.Hour, want: DefaultMaxCredentialAge},
		{name: "a shorter ceiling is honored", set: 5 * time.Minute, want: 5 * time.Minute},
		{name: "a longer ceiling is clamped", set: 72 * time.Hour, want: maxCredentialAgeCeiling},
		{name: "exactly the ceiling", set: maxCredentialAgeCeiling, want: maxCredentialAgeCeiling},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := New(testConfig(stubGitLabBase), testFactory(), WithMaxCredentialAge(tt.set))

			if pool.maxCredentialAge != tt.want {
				t.Errorf("maxCredentialAge = %v, want %v", pool.maxCredentialAge, tt.want)
			}
		})
	}
}

// TestEvictStaleCredential_LeavesFreshEntriesAlone verifies the
// stale-credential path drops nothing it was not asked to: a key that is gone,
// and a key another request has already rebuilt between the read lock that
// spotted the staleness and the write lock that acts on it, are both left as
// they are.
func TestEvictStaleCredential_LeavesFreshEntriesAlone(t *testing.T) {
	tests := []struct {
		name    string
		present bool
	}{
		{name: "the key is gone"},
		{name: "the key was rebuilt in the meantime", present: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := New(testConfig(stubGitLabBase), testFactory())
			key := "a key nothing was ever stored under"
			if tt.present {
				key = revalidationEntry(t, pool, stubGitLabBase, "tok-fresh", time.Now())
			}

			pool.evictStaleCredential(key)

			if _, stillThere := pool.entries[key]; stillThere != tt.present {
				t.Errorf("entry present = %v, want %v", stillThere, tt.present)
			}
			if got := pool.Stats().StaleCredentialEvictions; got != 0 {
				t.Errorf("StaleCredentialEvictions = %d, want 0", got)
			}
		})
	}
}

// TestEvictServer_DropsTheEntryHoldingThatServer verifies the escape hatch a
// background build failure needs.
//
// Under HTTP the catalog is registered on a goroutine, so the entry is already
// cached by the time a registration can fail. Without this the poisoned entry
// would serve every later request for that credential until an idle timeout or
// a revalidation happened to replace it, which is an hour by default; dropping
// it makes the next request rebuild, exactly as a synchronous failure does.
func TestEvictServer_DropsTheEntryHoldingThatServer(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())

	srv, err := pool.GetOrCreate("glpat-evict-me", stubGitLabBase)
	if err != nil {
		t.Fatalf("GetOrCreate() unexpected error: %v", err)
	}
	if pool.Size() != 1 {
		t.Fatalf("pool.Size() = %d before eviction, want 1", pool.Size())
	}

	if !pool.EvictServer(srv) {
		t.Fatal("EvictServer() reported it found nothing, but the entry was just created")
	}
	if pool.Size() != 0 {
		t.Errorf("pool.Size() = %d after eviction, want 0", pool.Size())
	}

	// The credential still works: eviction drops a build, not a token.
	rebuilt, err := pool.GetOrCreate("glpat-evict-me", stubGitLabBase)
	if err != nil {
		t.Fatalf("GetOrCreate() after eviction: %v", err)
	}
	if rebuilt == srv {
		t.Error("GetOrCreate() returned the evicted server rather than rebuilding")
	}
}

// TestEvictServer_AServerThePoolDoesNotHold_IsReportedRatherThanGuessedAt
// verifies that the linear scan says so instead of evicting something else.
func TestEvictServer_AServerThePoolDoesNotHold_IsReportedRatherThanGuessedAt(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	pool := New(cfg, testFactory())

	if _, err := pool.GetOrCreate("glpat-keep-me", stubGitLabBase); err != nil {
		t.Fatalf("GetOrCreate() unexpected error: %v", err)
	}

	stranger, err := New(testConfig(stubGitLabBase), testFactory()).GetOrCreate("glpat-other-pool", stubGitLabBase)
	if err != nil {
		t.Fatalf("building a server in another pool: %v", err)
	}

	if pool.EvictServer(stranger) {
		t.Error("EvictServer() claimed to evict a server this pool never held")
	}
	if pool.EvictServer(nil) {
		t.Error("EvictServer(nil) claimed to evict something")
	}
	if pool.Size() != 1 {
		t.Errorf("pool.Size() = %d, want the untouched entry to survive", pool.Size())
	}
}

// blockingGitLabStub answers every request only once release is closed, and
// reports the highest number of requests it ever held at the same time.
//
// It is how the concurrency ceiling is observed: the pool's credential probe is
// a single GET, so "probes in flight" and "requests this stub is holding" are
// the same number.
type blockingGitLabStub struct {
	server   *httptest.Server
	release  chan struct{}
	inside   atomic.Int64
	peak     atomic.Int64
	entered  chan struct{}
	released sync.Once
}

// releaseAll unblocks every held request. It is safe to call more than once,
// which matters because a test releases explicitly and t.Cleanup releases again
// on the way out.
func (s *blockingGitLabStub) releaseAll() {
	s.released.Do(func() { close(s.release) })
}

// newBlockingGitLabStub starts the stub. Callers must close its release
// channel, which t.Cleanup does, or every held request leaks until the test
// binary exits.
func newBlockingGitLabStub(t *testing.T) *blockingGitLabStub {
	t.Helper()
	stub := &blockingGitLabStub{
		release: make(chan struct{}),
		entered: make(chan struct{}, 1024),
	}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := stub.inside.Add(1)
		for {
			high := stub.peak.Load()
			if now <= high || stub.peak.CompareAndSwap(high, now) {
				break
			}
		}
		select {
		case stub.entered <- struct{}{}:
		default:
		}
		select {
		case <-stub.release:
		case <-r.Context().Done():
		}
		stub.inside.Add(-1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(func() {
		stub.releaseAll()
		stub.server.Close()
	})
	return stub
}

// waitForRequests blocks until n requests have reached the handler, or fails
// the test if they do not arrive in time.
func (s *blockingGitLabStub) waitForRequests(t *testing.T, n int) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for range n {
		select {
		case <-s.entered:
		case <-deadline:
			t.Fatalf("only %d of %d probes reached GitLab", s.inside.Load(), n)
		}
	}
}

// TestGetOrCreate_BoundsConcurrentCredentialProbes verifies the pool never has
// more than maxConcurrentCredentialProbes credential probes in flight.
//
// The singleflight group collapses concurrent requests for the same credential
// and therefore bounds nothing here: every invented token is a distinct key, so
// before the ceiling existed an unauthenticated flood of them was relayed to
// the configured GitLab instance one probe per token, at whatever rate it
// arrived. This is the measure that covers the distributed variant of that,
// where each source stays under the front door's failure budget and no request
// is ever blocked.
func TestGetOrCreate_BoundsConcurrentCredentialProbes(t *testing.T) {
	stub := newBlockingGitLabStub(t)
	cfg := testConfig(stub.server.URL)
	pool := New(cfg, testFactory())

	const callers = maxConcurrentCredentialProbes * 3
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			// Distinct token per caller, so singleflight collapses nothing and
			// each one is a separate probe.
			_, _ = pool.GetOrCreate("glpat-flood-"+strconv.Itoa(i), stub.server.URL)
		})
	}

	stub.waitForRequests(t, maxConcurrentCredentialProbes)
	// Give any unbounded surplus a chance to arrive before reading the peak, so
	// a passing result means the ceiling held rather than that the test looked
	// early.
	time.Sleep(200 * time.Millisecond)
	peak := stub.peak.Load()

	stub.releaseAll()
	wg.Wait()

	if peak > maxConcurrentCredentialProbes {
		t.Errorf("peak concurrent credential probes = %d, want at most %d", peak, maxConcurrentCredentialProbes)
	}
	if peak == 0 {
		t.Error("no credential probe reached GitLab; the test observed nothing")
	}
}

// TestGetOrCreate_SaturatedProbeQueueIsRetryableNotUnauthorized verifies what a
// caller is told when every probe slot is taken.
//
// The distinction is the whole point of the separate error. Nothing was learned
// about the credential, so answering 401 would tell a client holding a
// perfectly good token to reauthorize because the server was busy, and would
// charge the failure to an authentication budget that exists to count rejected
// credentials. ErrCredentialProbeBusy is neither ErrInvalidCredential nor
// wrapped around it, so the HTTP gate's own branch maps it to 503.
func TestGetOrCreate_SaturatedProbeQueueIsRetryableNotUnauthorized(t *testing.T) {
	stub := newBlockingGitLabStub(t)
	cfg := testConfig(stub.server.URL)
	pool := New(cfg, testFactory(), func(p *ServerPool) {
		p.probeQueueTimeout = 50 * time.Millisecond
	})

	var wg sync.WaitGroup
	for i := range maxConcurrentCredentialProbes {
		wg.Go(func() {
			_, _ = pool.GetOrCreate("glpat-holder-"+strconv.Itoa(i), stub.server.URL)
		})
	}
	stub.waitForRequests(t, maxConcurrentCredentialProbes)

	_, err := pool.GetOrCreate("glpat-latecomer", stub.server.URL)

	stub.releaseAll()
	wg.Wait()

	if !errors.Is(err, ErrCredentialProbeBusy) {
		t.Fatalf("GetOrCreate() error = %v, want ErrCredentialProbeBusy", err)
	}
	if errors.Is(err, ErrInvalidCredential) {
		t.Error("a saturated probe queue must not read as a rejected credential")
	}
}

// TestAcquireProbeSlot_ShutdownDoesNotWaitOutTheQueue verifies a pool whose
// lifetime has ended stops waiting immediately instead of parking a build on a
// queue that will never move.
func TestAcquireProbeSlot_ShutdownDoesNotWaitOutTheQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pool := New(testConfig(stubGitLabBase), testFactory(), func(p *ServerPool) {
		p.baseContext = func() context.Context { return ctx }
		p.probeQueueTimeout = time.Hour
	})
	for range maxConcurrentCredentialProbes {
		if _, err := pool.acquireProbeSlot(); err != nil {
			t.Fatalf("acquireProbeSlot() unexpected error: %v", err)
		}
	}
	cancel()

	start := time.Now()
	release, err := pool.acquireProbeSlot()
	if err == nil {
		release()
		t.Fatal("acquireProbeSlot() error = nil, want the shutdown error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("acquireProbeSlot() error = %v, want it to wrap context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("acquireProbeSlot() waited %v after shutdown, want it to return at once", elapsed)
	}
}

// TestAcquireProbeSlot_ReleaseIsIdempotent verifies the returned release
// function gives the slot back exactly once.
//
// A release called twice would return a slot the pool never took, raising the
// effective ceiling by one every time it happened until the bound meant
// nothing.
func TestAcquireProbeSlot_ReleaseIsIdempotent(t *testing.T) {
	pool := New(testConfig(stubGitLabBase), testFactory())

	release, err := pool.acquireProbeSlot()
	if err != nil {
		t.Fatalf("acquireProbeSlot() unexpected error: %v", err)
	}
	release()
	release()

	if got := len(pool.probes); got != 0 {
		t.Errorf("slots held after a double release = %d, want 0", got)
	}
}

// TestEntry_AnswersForTheCredentialRatherThanForTheServer verifies the handle
// that replaced a bare *mcp.Server as the thing a caller holds.
//
// One server is now shared by every credential of a configuration shape, so a
// server pointer no longer answers "which credential is this". Everything that
// has to know keys on the entry instead, and on the opaque owner token it
// carries, so those have to be readable and distinct.
func TestEntry_AnswersForTheCredentialRatherThanForTheServer(t *testing.T) {
	cfg := testConfig(stubGitLabBase)
	// One server for every credential, which is what a shape-shared factory
	// does and what makes the entry rather than the server the identity.
	shared := mcp.NewServer(&mcp.Implementation{Name: "shape", Version: "0.0.0"}, nil)
	pool := New(cfg, func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return shared, nil
	})

	first, err := pool.GetOrCreateEntry("glpat-first", stubGitLabBase, nil)
	if err != nil {
		t.Fatalf("GetOrCreateEntry(first): %v", err)
	}
	second, err := pool.GetOrCreateEntry("glpat-second", stubGitLabBase, nil)
	if err != nil {
		t.Fatalf("GetOrCreateEntry(second): %v", err)
	}

	facts := []struct {
		name string
		ok   func() bool
		want string
	}{
		{"one server serves the first credential", func() bool { return first.Server() == shared }, "the entry is not served by the shared server"},
		{"one server serves the second credential", func() bool { return second.Server() == shared }, "the entry is not served by the shared server"},
		{"each entry has a client", func() bool { return first.Client() != nil && second.Client() != nil }, "an entry was built with no GitLab client"},
		{"the clients are not shared", func() bool { return first.Client() != second.Client() }, "two credentials share one GitLab client"},
		{"the configuration names the instance", func() bool { return first.Config() != nil && first.Config().GitLabURL == stubGitLabBase }, "Config() does not name the instance the entry was built for"},
		// Whatever the lookup produced, the accessor reports it rather than a
		// second, independently resolved answer.
		{"the identity is the entry's own", func() bool { return first.Identity() == first.identity }, "Identity() does not report the entry's own"},
		{"each entry has an owner token", func() bool { return first.Owner() != "" && second.Owner() != "" }, "an entry was built with no owner token"},
		{"the owner tokens are distinct", func() bool { return first.Owner() != second.Owner() }, "two credentials share one owner token"},
	}
	for _, fact := range facts {
		t.Run(fact.name, func(t *testing.T) {
			if !fact.ok() {
				t.Error(fact.want)
			}
		})
	}

	// Nothing downstream may see a credential change identity between requests:
	// the owner token is what its sessions and notifications are filed under.
	again, againErr := pool.GetOrCreateEntry("glpat-first", stubGitLabBase, nil)
	if againErr != nil {
		t.Fatalf("GetOrCreateEntry(first, again): %v", againErr)
	}
	if again != first {
		t.Error("a second request for one credential returned a different entry")
	}
}

// TestEntry_TheZeroHandleAnswersTheZeroValues verifies that the accessors
// survive a nil entry.
//
// An eviction callback and the authentication gate both reach for these before
// they know an entry exists, and a panic in either would take a request or a
// pool sweep with it.
func TestEntry_TheZeroHandleAnswersTheZeroValues(t *testing.T) {
	var absent *Entry

	cases := []struct {
		name string
		zero func() bool
	}{
		{"Server", func() bool { return absent.Server() == nil }},
		{"Client", func() bool { return absent.Client() == nil }},
		{"Config", func() bool { return absent.Config() == nil }},
		{"Owner", func() bool { return absent.Owner() == "" }},
		{"Identity", func() bool { return !absent.Identity().Resolved() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.zero() {
				t.Errorf("(*Entry)(nil).%s() answered something other than the zero value", tc.name)
			}
		})
	}
}

// TestWithOnInsert_FiresWithTheEntry verifies the hook that builds a
// credential's per-request state.
//
// It takes the entry rather than the server because the server is shared: told
// only "this server was inserted", the callback could not tell which credential
// it was being asked about, and every entry after the first would be missed.
func TestWithOnInsert_FiresWithTheEntry(t *testing.T) {
	var mu sync.Mutex
	var owners []string

	shared := mcp.NewServer(&mcp.Implementation{Name: "shape", Version: "0.0.0"}, nil)
	pool := New(testConfig(stubGitLabBase), func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return shared, nil
	}, WithOnInsert(func(entry *Entry) {
		mu.Lock()
		defer mu.Unlock()
		owners = append(owners, entry.Owner())
	}))

	for _, token := range []string{"glpat-one", "glpat-two"} {
		t.Run(token, func(t *testing.T) {
			if _, err := pool.GetOrCreateEntry(token, stubGitLabBase, nil); err != nil {
				t.Fatalf("GetOrCreateEntry(%s): %v", token, err)
			}
		})
	}

	mu.Lock()
	defer mu.Unlock()
	if len(owners) != 2 {
		t.Fatalf("the insert hook fired %d times for two credentials, want 2", len(owners))
	}
	if owners[0] == "" || owners[0] == owners[1] {
		t.Errorf("the hook was handed owners %q and %q, want two distinct non-empty tokens", owners[0], owners[1])
	}
}

// TestEvictServer_DropsEveryEntryServedByThatServer verifies the eviction a
// failed background registration needs, now that one failure is everybody's.
//
// A registration runs once per configuration shape and the server it produces
// serves every credential of that shape, so a registration that failed failed
// for all of them. Stopping at the first match would leave the rest holding a
// server with no tools, which is the exact condition this exists to clear.
func TestEvictServer_DropsEveryEntryServedByThatServer(t *testing.T) {
	shared := mcp.NewServer(&mcp.Implementation{Name: "shape", Version: "0.0.0"}, nil)
	var mu sync.Mutex
	var evicted []string
	pool := New(testConfig(stubGitLabBase), func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return shared, nil
	}, WithOnEvict(func(entry *Entry) {
		mu.Lock()
		defer mu.Unlock()
		evicted = append(evicted, entry.Owner())
	}))

	for _, token := range []string{"glpat-a", "glpat-b", "glpat-c"} {
		t.Run(token, func(t *testing.T) {
			if _, err := pool.GetOrCreateEntry(token, stubGitLabBase, nil); err != nil {
				t.Fatalf("GetOrCreateEntry(%s): %v", token, err)
			}
		})
	}
	if pool.Size() != 3 {
		t.Fatalf("pool.Size() = %d before eviction, want 3", pool.Size())
	}

	if !pool.EvictServer(shared) {
		t.Fatal("EvictServer() reported it found nothing, but three entries were just built")
	}
	if pool.Size() != 0 {
		t.Errorf("pool.Size() = %d after eviction, want 0: entries of the same shape survived", pool.Size())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(evicted) != 3 {
		t.Errorf("the evict hook fired %d times, want 3", len(evicted))
	}
}
