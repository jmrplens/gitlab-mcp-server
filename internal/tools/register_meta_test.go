// register_meta_test.go contains benchmarks for RegisterAllMeta, measuring
// meta-tool registration on the Ultimate catalog with a cold and a warm
// schema cache.
package tools

import (
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

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

// TestRegisterAllMeta_ReportsACatalogThatCannotBeBuilt verifies the meta
// registration returns the catalog failure rather than registering nothing,
// through the seam that stands in for a failure no real input can cause.
func TestRegisterAllMeta_ReportsACatalogThatCannotBeBuilt(t *testing.T) {
	forced := errors.New("forced catalog failure")
	original := sharedBaseCatalog
	t.Cleanup(func() { sharedBaseCatalog = original })
	sharedBaseCatalog = func(bool, ActionCatalogOptions) (*actioncatalog.Catalog, error) { return nil, forced }

	err := RegisterAllMeta(mcp.NewServer(&mcp.Implementation{Name: "failing", Version: "0"}, nil), nil, edition.Free)
	if !errors.Is(err, forced) || !strings.Contains(err.Error(), "failed to build action catalog") {
		t.Fatalf("RegisterAllMeta() error = %v, want the forced failure", err)
	}
}
