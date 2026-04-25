// Package roots provides client workspace discovery via the MCP Roots capability.
//
// Roots is a client-side capability — the client declares workspace directories/files
// and the server can query them via ServerSession.ListRoots(). The Manager caches current
// roots per session and provides helpers for workspace-aware operations.
package roots

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Manager caches client-provided roots for a session and provides
// workspace-aware lookup helpers. Safe for concurrent use.
type Manager struct {
	mu    sync.RWMutex
	roots []*mcp.Root
}

// NewManager creates a Manager with an empty root set.
func NewManager() *Manager {
	return &Manager{}
}

// Refresh queries the client for its current roots via the session and caches
// the result. Returns nil with an empty root list if the client does not support
// roots or returns an error (graceful degradation).
//
// When the client did not advertise the roots capability during initialization,
// Refresh clears the cache and returns nil without contacting the client. This
// avoids a guaranteed-to-fail JSON-RPC request and the noisy warning that
// would follow.
func (m *Manager) Refresh(ctx context.Context, session *mcp.ServerSession) error {
	if session == nil {
		m.setRoots(nil)
		return nil
	}
	if !ClientSupportsRoots(session) {
		m.setRoots(nil)
		return nil
	}

	result, err := session.ListRoots(ctx, nil)
	if err != nil {
		slog.Warn("failed to list client roots, clearing cache", "error", err)
		m.setRoots(nil)
		return fmt.Errorf("listing roots: %w", err)
	}

	m.setRoots(result.Roots)
	slog.Info("client roots refreshed", "count", len(result.Roots))
	return nil
}

// ClientSupportsRoots reports whether the client advertised the roots
// capability during the MCP initialize handshake. Returns false when the
// session has not yet completed initialize, when capabilities are missing,
// or when the client did not declare a roots capability.
func ClientSupportsRoots(session *mcp.ServerSession) bool {
	if session == nil {
		return false
	}
	params := session.InitializeParams()
	if params == nil || params.Capabilities == nil {
		return false
	}
	if params.Capabilities.RootsV2 != nil {
		return true
	}
	// Legacy: SDK populates the deprecated Roots struct when the client sent
	// any "roots": {...} object in capabilities. Presence of either nested
	// flag is the only signal available pre-RootsV2.
	return params.Capabilities.Roots.ListChanged
}

// GetRoots returns a copy of the cached root list.
func (m *Manager) GetRoots() []*mcp.Root {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.roots) == 0 {
		return nil
	}
	out := make([]*mcp.Root, len(m.roots))
	copy(out, m.roots)
	return out
}

// FindGitRoot scans cached roots for a path that looks like a Git repository root
// (URI path ends with ".git" or the root name contains "git"/"repo" hints).
// Returns the URI and true if found, or ("", false) otherwise.
func (m *Manager) FindGitRoot() (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, r := range m.roots {
		if isGitRoot(r) {
			return r.URI, true
		}
	}
	return "", false
}

// HasRoot checks whether a specific URI exists in the cached root list.
func (m *Manager) HasRoot(uri string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, r := range m.roots {
		if r.URI == uri {
			return true
		}
	}
	return false
}

// setRoots replaces the cached root list under a write lock.
func (m *Manager) setRoots(roots []*mcp.Root) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roots = roots
}

// SetRootsForTest replaces the cached root list. Exported for use in tests only.
func (m *Manager) SetRootsForTest(roots []*mcp.Root) {
	m.setRoots(roots)
}

// ListClientRoots queries the client for its current workspace roots via the
// tool request's session. Returns nil with no error when the session is unavailable,
// enabling graceful degradation for clients without roots support.
func ListClientRoots(ctx context.Context, session *mcp.ServerSession) ([]*mcp.Root, error) {
	if session == nil {
		return nil, nil
	}

	result, err := session.ListRoots(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("listing client roots: %w", err)
	}
	return result.Roots, nil
}

// isGitRoot heuristically determines whether a Root likely represents a Git repository.
// It checks the URI path for ".git" suffix and common directory name patterns.
func isGitRoot(r *mcp.Root) bool {
	if r == nil || r.URI == "" {
		return false
	}

	parsed, err := url.Parse(r.URI)
	if err != nil {
		return false
	}

	p := filepath.ToSlash(parsed.Path)
	base := filepath.Base(p)

	// Direct .git directory
	if strings.HasSuffix(p, "/.git") || base == ".git" {
		return true
	}

	// Common git repository indicators in the path
	lower := strings.ToLower(p)
	gitIndicators := []string{"/repos/", "/repositories/", "/git/"}
	for _, ind := range gitIndicators {
		if strings.Contains(lower, ind) {
			return true
		}
	}

	return false
}
