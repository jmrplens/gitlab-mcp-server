// clientcompat_wiring_test.go verifies that createServer installs the
// per-client compatibility middleware on the production path: a session that
// identifies as OpenAI Codex receives resource annotations with integer
// priorities, a generic session keeps the original floats, and the
// CLIENT_COMPAT=off kill switch disables the rewrite at server build time.
package main

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// connectSessionAs attaches an in-memory client with the given identity to a
// server built by createServer.
func connectSessionAs(t *testing.T, server *mcp.Server, impl *mcp.Implementation) *mcp.ClientSession {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })
	session, err := mcp.NewClient(impl, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// resourcePriorities lists the annotation priorities of the server's
// resources/list result (resources are served from the registry, so no
// GitLab backend call is involved) split into float and integer counts.
func resourcePriorities(t *testing.T, session *mcp.ClientSession) (floats, integers int) {
	t.Helper()
	res, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	for _, r := range res.Resources {
		if r.Annotations == nil || r.Annotations.Priority == 0 {
			continue
		}
		if r.Annotations.Priority == float64(int64(r.Annotations.Priority)) {
			integers++
		} else {
			floats++
		}
	}
	return floats, integers
}

// TestCreateServer_ClientCompatMiddlewareWiring verifies the middleware is
// active on servers built by createServer: Codex sessions see only integer
// priorities while generic sessions of the same server keep the floats.
func TestCreateServer_ClientCompatMiddlewareWiring(t *testing.T) {
	client := newMockGitLabClient(t)
	server := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceDynamic})

	codex := connectSessionAs(t, server, &mcp.Implementation{Name: "codex-mcp-client", Title: "Codex", Version: "0.148.0"})
	floats, integers := resourcePriorities(t, codex)
	if floats != 0 {
		t.Errorf("codex session sees %d float priorities, want 0", floats)
	}
	if integers == 0 {
		t.Error("codex session sees no annotated resources; fixture lost its annotations")
	}

	generic := connectSessionAs(t, server, &mcp.Implementation{Name: "test-client", Version: "1.0.0"})
	floats, _ = resourcePriorities(t, generic)
	if floats == 0 {
		t.Error("generic session sees no float priorities; registry was sanitized")
	}
}

// TestCreateServer_ClientCompatKillSwitch verifies CLIENT_COMPAT=off skips
// the middleware install, so even Codex sessions keep the float priorities.
func TestCreateServer_ClientCompatKillSwitch(t *testing.T) {
	t.Setenv("CLIENT_COMPAT", "off")
	client := newMockGitLabClient(t)
	server := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceDynamic})

	codex := connectSessionAs(t, server, &mcp.Implementation{Name: "codex-mcp-client", Title: "Codex", Version: "0.148.0"})
	floats, _ := resourcePriorities(t, codex)
	if floats == 0 {
		t.Error("CLIENT_COMPAT=off but codex session sees no float priorities; middleware still active")
	}
}
