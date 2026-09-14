// register_meta_test.go contains benchmarks for meta-surface registration,
// measuring it on the Ultimate catalog with a cold and a warm schema cache.
package tools

import (
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// BenchmarkRegisterMetaSurface_Ultimate measures a cold meta-surface
// registration (catalog build dominates; ~51 dispatcher tools).
func BenchmarkRegisterMetaSurface_Ultimate(b *testing.B) {
	client := benchClient(b)
	b.ResetTimer()
	for range b.N {
		server := mcp.NewServer(&mcp.Implementation{Name: "bench", Version: "0"}, nil)
		if err := registerMetaSurface(server, client, edition.Ultimate); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRegisterMetaSurface_Ultimate_Cached is the warm-cache counterpart
// of BenchmarkRegisterMetaSurface_Ultimate.
func BenchmarkRegisterMetaSurface_Ultimate_Cached(b *testing.B) {
	client := benchClient(b)
	cache := mcp.NewSchemaCache()
	warm := mcp.NewServer(&mcp.Implementation{Name: "bench", Version: "0"}, &mcp.ServerOptions{SchemaCache: cache})
	if err := registerMetaSurface(warm, client, edition.Ultimate); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		server := mcp.NewServer(&mcp.Implementation{Name: "bench", Version: "0"}, &mcp.ServerOptions{SchemaCache: cache})
		if err := registerMetaSurface(server, client, edition.Ultimate); err != nil {
			b.Fatal(err)
		}
	}
}

// TestRegisterMetaSurface_ReportsACatalogThatCannotBeBuilt verifies the meta
// registration returns the catalog failure rather than registering nothing,
// through the seam that stands in for a failure no real input can cause.
func TestRegisterMetaSurface_ReportsACatalogThatCannotBeBuilt(t *testing.T) {
	forced := errors.New("forced catalog failure")
	original := sharedBaseCatalog
	t.Cleanup(func() { sharedBaseCatalog = original })
	sharedBaseCatalog = func(bool, ActionCatalogOptions) (*actioncatalog.Catalog, error) { return nil, forced }

	err := registerMetaSurface(mcp.NewServer(&mcp.Implementation{Name: "failing", Version: "0"}, nil), nil, edition.Free)
	if !errors.Is(err, forced) || !strings.Contains(err.Error(), "build meta action catalog") {
		t.Fatalf("registerMetaSurface() error = %v, want the forced failure", err)
	}
}
