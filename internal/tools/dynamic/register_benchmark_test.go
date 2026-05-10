package dynamic

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
)

// BenchmarkSearch_BaselineMetaCatalog measures dynamic search throughput and
// allocations against the captured meta catalog plus standalone routes.
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
		"merje requesy", // Known low-signal typo-heavy query kept in the baseline until fuzzy matching handles both misspelled terms.
	}
	allowZero := map[string]bool{
		// TODO(dynamic-search): remove this exception when fuzzy matching can recover both malformed terms.
		"merje requesy": true,
	}

	for _, query := range queries {
		b.Run(benchmarkName(query), func(b *testing.B) {
			input := SearchInput{Query: query, Limit: 20}
			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
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

	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{IncludeMCP: true})
	if err != nil {
		b.Fatalf("BuildActionCatalog() error: %v", err)
	}
	registry := NewRegistryFromCatalog(AddStandaloneCatalog(catalog, nil, StandaloneOptions{}))
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
