package tools

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// benchClient builds a GitLab client against a stub version endpoint for the
// registration benchmarks.
func benchClient(b *testing.B) *gitlabclient.Client {
	b.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0"}`))
	}))
	b.Cleanup(srv.Close)
	client, err := gitlabclient.NewClient(&config.Config{GitLabURL: srv.URL, GitLabToken: "bench-token"})
	if err != nil {
		b.Fatal(err)
	}
	return client
}

// BenchmarkBuildActionCatalog_Ultimate measures the per-server cost of
// building the full Ultimate action catalog (paid per token+URL in HTTP mode).
func BenchmarkBuildActionCatalog_Ultimate(b *testing.B) {
	client := benchClient(b)
	b.ResetTimer()
	for range b.N {
		if _, err := BuildActionCatalog(client, ActionCatalogOptions{Enterprise: true, IncludeMCP: true}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRegisterAll_Ultimate measures a cold individual-surface
// registration: catalog build plus MCP SDK schema conversion/resolution for
// every projected tool.
func BenchmarkRegisterAll_Ultimate(b *testing.B) {
	client := benchClient(b)
	b.ResetTimer()
	for range b.N {
		server := mcp.NewServer(&mcp.Implementation{Name: "bench", Version: "0"}, &mcp.ServerOptions{PageSize: 2000})
		RegisterAll(server, client, edition.Ultimate)
	}
}

// BenchmarkRegisterAllMeta_Ultimate measures a cold meta-surface
// registration (catalog build dominates; ~50 dispatcher tools).
func BenchmarkRegisterAllMeta_Ultimate(b *testing.B) {
	client := benchClient(b)
	b.ResetTimer()
	for range b.N {
		server := mcp.NewServer(&mcp.Implementation{Name: "bench", Version: "0"}, nil)
		if err := RegisterAllMeta(server, client, edition.Ultimate); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRegisterAll_Ultimate_Cached measures the steady-state per-token
// registration cost in HTTP mode: compiled schema pointers plus a warm
// mcp.SchemaCache skip the SDK remarshal and resolution.
func BenchmarkRegisterAll_Ultimate_Cached(b *testing.B) {
	client := benchClient(b)
	cache := mcp.NewSchemaCache()
	// warm-up: first registration pays the one-time schema compilation
	warm := mcp.NewServer(&mcp.Implementation{Name: "bench", Version: "0"}, &mcp.ServerOptions{PageSize: 2000, SchemaCache: cache})
	RegisterAll(warm, client, edition.Ultimate)
	b.ResetTimer()
	for range b.N {
		server := mcp.NewServer(&mcp.Implementation{Name: "bench", Version: "0"}, &mcp.ServerOptions{PageSize: 2000, SchemaCache: cache})
		RegisterAll(server, client, edition.Ultimate)
	}
}

// BenchmarkRegisterAllMeta_Ultimate_Cached is the warm-cache counterpart of
// BenchmarkRegisterAllMeta_Ultimate.
func BenchmarkRegisterAllMeta_Ultimate_Cached(b *testing.B) {
	client := benchClient(b)
	cache := mcp.NewSchemaCache()
	warm := mcp.NewServer(&mcp.Implementation{Name: "bench", Version: "0"}, &mcp.ServerOptions{SchemaCache: cache})
	if err := RegisterAllMeta(warm, client, edition.Ultimate); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		server := mcp.NewServer(&mcp.Implementation{Name: "bench", Version: "0"}, &mcp.ServerOptions{SchemaCache: cache})
		if err := RegisterAllMeta(server, client, edition.Ultimate); err != nil {
			b.Fatal(err)
		}
	}
}
