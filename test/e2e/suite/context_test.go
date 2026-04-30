//go:build e2e

package suite

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
)

const defaultCleanupTimeout = 60 * time.Second

// E2EContext carries per-test E2E sessions, identity, and cleanup ownership.
type E2EContext struct {
	T        *testing.T
	RunID    string
	Name     string
	Sessions sessions
	GitLab   *gitlabclient.Client
	Ledger   *ResourceLedger
}

// NewE2EContext creates per-test E2E context and registers ledger cleanup.
func NewE2EContext(t *testing.T) *E2EContext {
	t.Helper()

	e2e := &E2EContext{
		T:        t,
		RunID:    e2eRunID,
		Name:     t.Name(),
		Sessions: sess,
		GitLab:   sess.glClient,
		Ledger:   &ResourceLedger{},
	}

	t.Cleanup(func() {
		ctx, cancel := cleanupContext(defaultCleanupTimeout)
		defer cancel()
		e2e.Ledger.CleanupAll(ctx, t)
	})

	return e2e
}

func cleanupContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// Individual returns the individual-tool MCP session or skips the test.
func (e2e *E2EContext) Individual() *mcp.ClientSession {
	return e2e.requiredSession("individual", e2e.Sessions.individual)
}

// Meta returns the meta-tool MCP session or skips the test.
func (e2e *E2EContext) Meta() *mcp.ClientSession {
	return e2e.requiredSession("meta", e2e.Sessions.meta)
}

// Sampling returns the sampling-enabled MCP session or skips the test.
func (e2e *E2EContext) Sampling() *mcp.ClientSession {
	return e2e.requiredSession("sampling", e2e.Sessions.sampling)
}

// Elicitation returns the elicitation-enabled MCP session or skips the test.
func (e2e *E2EContext) Elicitation() *mcp.ClientSession {
	return e2e.requiredSession("elicitation", e2e.Sessions.elicitation)
}

// SafeMode returns the safe-mode MCP session or skips the test.
func (e2e *E2EContext) SafeMode() *mcp.ClientSession {
	return e2e.requiredSession("safe mode", e2e.Sessions.safeMode)
}

func (e2e *E2EContext) requiredSession(name string, session *mcp.ClientSession) *mcp.ClientSession {
	e2e.T.Helper()
	if session == nil {
		e2e.T.Skipf("%s MCP session not configured", name)
	}
	return session
}
