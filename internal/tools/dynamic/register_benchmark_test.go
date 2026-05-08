package dynamic

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func BenchmarkSearch_BaselineMetaCatalog(b *testing.B) {
	registry := benchmarkRegistry(b)
	ctx := context.Background()

	queries := []string{
		"merge request list open author project",
		"list open issues",
		"pipeline run trigger",
		"ci variable secret",
		"project delete",
		"discover project from remote",
		"merje requesy", // typo-heavy query for current baseline.
	}
	allowZero := map[string]bool{
		"merje requesy": true,
	}

	for _, query := range queries {
		b.Run(benchmarkName(query), func(b *testing.B) {
			input := SearchInput{Query: query, Limit: 20}
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				result, output, err := registry.Search(ctx, nil, input)
				if err != nil {
					b.Fatalf("Search() error: %v", err)
				}
				if result == nil || result.IsError {
					b.Fatalf("Search() result = %+v, want non-error", result)
				}
				if output.Count == 0 && !allowZero[query] {
					b.Fatalf("Search() output.Count = 0 for query %q", query)
				}
			}
		})
	}
}

func benchmarkRegistry(b *testing.B) *Registry {
	b.Helper()

	routes := toolutil.CaptureMetaRoutes(func() {
		server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-benchmark", Version: "0.0.1"}, nil)
		tools.RegisterAllMeta(server, nil, false)
		tools.RegisterMCPMeta(server, nil, nil)
	})

	routes = AddStandaloneRoutes(routes, nil, StandaloneOptions{})
	registry := NewRegistry(routes)
	if len(registry.entries) == 0 {
		b.Fatal("benchmark registry is empty")
	}

	b.Logf("benchmark registry entries: %d", len(registry.entries))
	return registry
}

func benchmarkName(query string) string {
	parts := strings.Fields(strings.ToLower(query))
	if len(parts) == 0 {
		return "empty"
	}
	return fmt.Sprintf("q_%s", strings.Join(parts, "_"))
}
