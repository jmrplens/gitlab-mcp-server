// pool.go implements a bounded LRU pool of per-token MCP servers for HTTP mode.

package serverpool

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
)

// ServerFactory creates a fully configured [*mcp.Server] with all tools,
// resources, and prompts registered for the given GitLab client and per-entry
// configuration.
// This is provided by the caller to decouple pool management from
// registration logic.
type ServerFactory func(client *gitlabclient.Client, cfg *config.ServerConfig) *mcp.Server

// poolEntry holds a server instance and its associated GitLab client,
// along with LRU tracking and session validation metadata.
type poolEntry struct {
	server        *mcp.Server
	client        *gitlabclient.Client
	serverConfig  *config.ServerConfig
	element       *list.Element
	createdAt     time.Time
	lastValidated time.Time
}

const defaultMaxSize = 100

// DefaultRevalidateInterval is the default period between token re-validation
// checks via a lightweight GitLab API call.
const DefaultRevalidateInterval = 15 * time.Minute

// Metrics holds operational counters for the [ServerPool]. All counters are
// monotonically increasing and use lock-free atomic increments.
type Metrics struct {
	Hits                   atomic.Int64
	Misses                 atomic.Int64
	Evictions              atomic.Int64
	RevalidationsFailed    atomic.Int64
	RevalidationsSucceeded atomic.Int64
}

// Snapshot is a point-in-time copy of pool [Metrics] plus current state.
// Safe for JSON serialization and cross-goroutine use.
type Snapshot struct {
	Hits                   int64     `json:"hits"`
	Misses                 int64     `json:"misses"`
	Evictions              int64     `json:"evictions"`
	RevalidationsFailed    int64     `json:"revalidations_failed"`
	RevalidationsSucceeded int64     `json:"revalidations_succeeded"`
	CurrentSize            int       `json:"current_size"`
	MaxSize                int       `json:"max_size"`
	CreatedAt              time.Time `json:"created_at"`
}

// ServerPool maintains a bounded set of [*mcp.Server] instances keyed by
// token plus GitLab URL hash (SHA-256). When the pool reaches maxSize, the
// least recently used entry is evicted. Entries are periodically re-validated
// against the GitLab API; entries with revoked tokens are evicted automatically.
type ServerPool struct {
	mu                 sync.RWMutex
	entries            map[string]*poolEntry
	lru                *list.List
	maxSize            int
	cfg                *config.Config
	factory            ServerFactory
	revalidateInterval time.Duration
	metrics            Metrics
	createdAt          time.Time
}

// Option configures pool behavior.
type Option func(*ServerPool)

// WithMaxSize sets the maximum number of unique token entries in the pool.
// Values ≤ 0 are ignored; the default is 100.
func WithMaxSize(n int) Option {
	return func(p *ServerPool) {
		if n > 0 {
			p.maxSize = n
		}
	}
}

// WithRevalidateInterval sets the interval between periodic token
// re-validation checks. Values ≤ 0 disable revalidation.
func WithRevalidateInterval(d time.Duration) Option {
	return func(p *ServerPool) {
		p.revalidateInterval = d
	}
}

// New creates a [ServerPool]. The cfg provides shared server-wide settings
// (GitLabURL, SkipTLSVerify, etc.). The factory function creates a fully
// registered [*mcp.Server] for each new GitLab client.
func New(cfg *config.Config, factory ServerFactory, opts ...Option) *ServerPool {
	p := &ServerPool{
		entries:            make(map[string]*poolEntry),
		lru:                list.New(),
		maxSize:            defaultMaxSize,
		cfg:                cfg,
		factory:            factory,
		revalidateInterval: DefaultRevalidateInterval,
		createdAt:          time.Now(),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// GetOrCreate returns the [*mcp.Server] for the given token and GitLab URL,
// creating one if it doesn't exist. The pool key is derived from both the
// token and gitlabURL, so the same token against different GitLab instances
// gets separate server entries. It is safe for concurrent use.
// Returns an error if the GitLab client cannot be created.
func (p *ServerPool) GetOrCreate(token, gitlabURL string) (*mcp.Server, error) {
	if token == "" {
		return nil, errors.New("empty token: authentication required")
	}
	if gitlabURL == "" {
		return nil, errors.New("empty GitLab URL: set --gitlab-url or send GITLAB-URL header")
	}

	key := sessionKey(token, gitlabURL)

	// Fast path: read lock to check existing entry.
	p.mu.RLock()
	if entry, ok := p.entries[key]; ok {
		p.mu.RUnlock()
		p.mu.Lock()
		p.lru.MoveToFront(entry.element)
		p.mu.Unlock()
		p.metrics.Hits.Add(1)
		return entry.server, nil
	}
	p.mu.RUnlock()

	// Slow path: write lock to create new entry.
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock.
	if entry, ok := p.entries[key]; ok {
		p.lru.MoveToFront(entry.element)
		p.metrics.Hits.Add(1)
		return entry.server, nil
	}

	p.metrics.Misses.Add(1)

	if p.lru.Len() >= p.maxSize {
		p.evictLRU()
	}

	client, err := gitlabclient.NewClientWithToken(
		gitlabURL, token, p.cfg.SkipTLSVerify,
	)
	if err != nil {
		return nil, fmt.Errorf("creating gitlab client for pool: %w", err)
	}
	client.SetEnterprise(p.cfg.Enterprise)

	entryCfg := p.entryConfig(client, gitlabURL)
	server := p.factory(client, entryCfg)
	element := p.lru.PushFront(key)
	now := time.Now()
	p.entries[key] = &poolEntry{
		server:        server,
		client:        client,
		serverConfig:  entryCfg,
		element:       element,
		createdAt:     now,
		lastValidated: now,
	}

	slog.Info("server pool: created new entry",
		"pool_size", len(p.entries),
		"gitlab_url", entryCfg.GitLabURL,
		"enterprise", entryCfg.Enterprise,
		"enterprise_source", p.enterpriseSource(),
		"scopes_detected", entryCfg.TokenScopes != nil,
		"token_suffix", tokenSuffix(token),
	)

	return server, nil
}

func (p *ServerPool) entryConfig(client *gitlabclient.Client, gitlabURL string) *config.ServerConfig {
	entryCfg := p.cfg.ServerConfig()
	entryCfg.GitLabURL = gitlabURL

	if p.cfg.AutoDetectEnterprise || !p.cfg.IgnoreScopes {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if p.cfg.AutoDetectEnterprise {
			entryCfg.Enterprise = client.DetectEnterprise(ctx, entryCfg.Enterprise)
		}
		client.SetEnterprise(entryCfg.Enterprise)

		if p.cfg.IgnoreScopes {
			return entryCfg
		}
		entryCfg.TokenScopes = gitlabclient.DetectScopes(ctx, client.GL())
	}
	return entryCfg
}

func (p *ServerPool) enterpriseSource() string {
	if p.cfg.AutoDetectEnterprise {
		return "detected"
	}
	return "configured"
}

// Size returns the current number of entries in the pool.
func (p *ServerPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.entries)
}

// Stats returns a point-in-time [Snapshot] of pool metrics and state.
func (p *ServerPool) Stats() Snapshot {
	p.mu.RLock()
	size := len(p.entries)
	p.mu.RUnlock()

	return Snapshot{
		Hits:                   p.metrics.Hits.Load(),
		Misses:                 p.metrics.Misses.Load(),
		Evictions:              p.metrics.Evictions.Load(),
		RevalidationsFailed:    p.metrics.RevalidationsFailed.Load(),
		RevalidationsSucceeded: p.metrics.RevalidationsSucceeded.Load(),
		CurrentSize:            size,
		MaxSize:                p.maxSize,
		CreatedAt:              p.createdAt,
	}
}

// Close removes all entries from the pool. Active MCP sessions for evicted
// servers are not forcefully terminated — they will expire naturally via
// [StreamableHTTPOptions.SessionTimeout].
func (p *ServerPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.entries {
		delete(p.entries, key)
	}
	p.lru.Init()
	slog.Info("server pool: closed all entries")
}

// evictLRU removes the least recently used entry. Must be called with
// write lock held.
func (p *ServerPool) evictLRU() {
	back := p.lru.Back()
	if back == nil {
		return
	}
	key, _ := back.Value.(string)
	if entry, ok := p.entries[key]; ok {
		gitlabURL, enterprise := poolEntryConfigLogValues(entry)
		delete(p.entries, key)
		p.metrics.Evictions.Add(1)
		slog.Info("server pool: evicted LRU entry",
			"pool_size", len(p.entries),
			"gitlab_url", gitlabURL,
			"enterprise", enterprise,
		)
	}
	p.lru.Remove(back)
}

// tokenHash returns a hex-encoded SHA-256 hash of the token.
func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// sessionKey returns a hex-encoded SHA-256 hash of the token combined with
// the GitLab URL. This ensures the same token against different GitLab
// instances results in separate pool entries.
func sessionKey(token, gitlabURL string) string {
	h := sha256.Sum256([]byte(token + "\x00" + gitlabURL))
	return hex.EncodeToString(h[:])
}

// tokenSuffix returns the last 4 characters of the token for safe logging.
func tokenSuffix(token string) string {
	if len(token) <= 4 {
		return "****"
	}
	return "..." + token[len(token)-4:]
}

// StartRevalidation launches a background goroutine that periodically
// checks all pool entries for token validity using a lightweight GitLab API
// call. Entries that fail validation are evicted. Cancel the context to stop.
func (p *ServerPool) StartRevalidation(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background() //nolint:contextcheck // defensive: nil-ctx guard for callers that pass uninitialized context
	}

	if p.revalidateInterval <= 0 {
		slog.Info("server pool: token revalidation disabled")
		return
	}

	slog.Info("server pool: starting token revalidation", "interval", p.revalidateInterval)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("server pool: revalidation goroutine panicked", "panic", r)
			}
		}()

		ticker := time.NewTicker(p.revalidateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("server pool: revalidation stopped")
				return
			case <-ticker.C:
				p.revalidateAll(ctx)
			}
		}
	}()
}

// revalidateAll checks each pool entry's token by calling the GitLab version
// endpoint. Entries that fail are evicted.
func (p *ServerPool) revalidateAll(ctx context.Context) {
	p.mu.RLock()
	snapshot := make(map[string]*poolEntry, len(p.entries))
	maps.Copy(snapshot, p.entries)
	p.mu.RUnlock()

	for key, entry := range snapshot {
		if ctx.Err() != nil {
			return
		}

		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, err := entry.client.Ping(checkCtx)
		cancel()

		if err != nil {
			slog.Warn("server pool: token revalidation failed, evicting entry",
				"error", err,
				"age", time.Since(entry.createdAt).Round(time.Second),
			)
			p.metrics.RevalidationsFailed.Add(1)
			p.evictByKey(key)
		} else {
			p.metrics.RevalidationsSucceeded.Add(1)
			p.mu.Lock()
			if e, ok := p.entries[key]; ok {
				e.lastValidated = time.Now()
			}
			p.mu.Unlock()
		}
	}
}

// evictByKey removes the entry with the given key from the pool.
func (p *ServerPool) evictByKey(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[key]; ok {
		gitlabURL, enterprise := poolEntryConfigLogValues(entry)
		p.lru.Remove(entry.element)
		delete(p.entries, key)
		p.metrics.Evictions.Add(1)
		slog.Info("server pool: evicted invalid entry",
			"pool_size", len(p.entries),
			"gitlab_url", gitlabURL,
			"enterprise", enterprise,
		)
	}
}

func poolEntryConfigLogValues(entry *poolEntry) (string, bool) {
	if entry == nil || entry.serverConfig == nil {
		return "", false
	}
	return entry.serverConfig.GitLabURL, entry.serverConfig.Enterprise
}
