// main_test.go verifies the meta-schema audit command can build and inspect the
// full base-plus-enterprise meta-tool catalog without requiring a real GitLab
// instance.
package main

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestRun_Completes verifies the meta-schema audit can build the full
// production-like enterprise meta-tool registry and measure schema sizes.
func TestRun_Completes(t *testing.T) {
	if err := run(); err != nil {
		t.Fatalf("run() error: %v", err)
	}
}

// TestCapturedRoutes_IncludeServerMetaTool verifies the audit route capture
// includes the standalone gitlab_server meta-tool registered by cmd/server.
func TestCapturedRoutes_IncludeServerMetaTool(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	routes := toolutil.CaptureMetaRoutes(func() {
		tools.RegisterAllMeta(server, nil, true)
		tools.RegisterMCPMeta(server, nil, nil)
	})
	serverRoutes, ok := routes["gitlab_server"]
	if !ok {
		t.Fatal("captured routes did not include gitlab_server")
	}
	if len(serverRoutes) != 4 {
		t.Fatalf("gitlab_server routes = %d, want 4", len(serverRoutes))
	}
}
