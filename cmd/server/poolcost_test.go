package main

import (
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// TestPoolEntryCost_SplitsShellFromRegistration measures the two halves of
// building one pooled server, because which half dominates decides what an
// HTTP-mode readiness gate could actually buy.
//
// The shell is everything a server needs before it has spoken to GitLab:
// options, capabilities, middleware. Registration is the tool catalog, which
// is the part a client currently waits through on the first request of every
// new credential.
//
// It is a test rather than a Benchmark so it runs in the ordinary suite and
// reports its numbers, and it asserts only the ordering it exists to
// establish. Absolute timings vary per machine, so nothing here fails on a
// duration.
func TestPoolEntryCost_SplitsShellFromRegistration(t *testing.T) {
	mock := newMockGitLabServer(t)
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:   mock.URL,
		GitLabToken: testToken,
	})
	if err != nil {
		t.Fatalf("creating the client: %v", err)
	}

	surfaces := []struct {
		name    string
		surface string
	}{
		{name: "dynamic", surface: config.ToolSurfaceDynamic},
		{name: "individual", surface: config.ToolSurfaceIndividual},
	}

	for _, s := range surfaces {
		t.Run(s.name, func(t *testing.T) {
			cfg := &config.ServerConfig{ToolSurface: s.surface, Tier: edition.Free}

			shellStart := time.Now()
			shell, shellErr := newServerShell(t.Context(), client, cfg)
			shellCost := time.Since(shellStart)
			if shellErr != nil {
				t.Fatalf("newServerShell() error = %v", shellErr)
			}

			registerStart := time.Now()
			if registerErr := shell.register(t.Context()); registerErr != nil {
				t.Fatalf("register() error = %v", registerErr)
			}
			registerCost := time.Since(registerStart)

			t.Logf("%s surface: shell %v, registration %v (%.0f%% of the build is registration)",
				s.name, shellCost.Round(time.Millisecond), registerCost.Round(time.Millisecond),
				100*float64(registerCost)/float64(shellCost+registerCost))

			if registerCost <= shellCost {
				t.Errorf("registration (%v) did not dominate the shell (%v); "+
					"if this ever holds, deferring registration buys nothing and the gate should go",
					registerCost, shellCost)
			}
		})
	}
}
