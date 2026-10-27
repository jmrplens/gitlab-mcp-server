// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// TestSpecsResolvingNextSteps_WrapsEveryFormatter verifies that the standalone
// tools' formatters resolve their next steps for the surface they are served
// on, the one a spec declares and the shared dispatcher that stands in for a
// spec declaring none alike (issue 1325): gitlab_discover_project names
// project.get, which neither the meta nor the individual surface calls by that
// ID.
func TestSpecsResolvingNextSteps_WrapsEveryFormatter(t *testing.T) {
	resolve := func(id string) (string, bool) { return "resolved " + id, true }
	section := func(hint string) string {
		var b strings.Builder
		toolutil.WriteHints(&b, hint)
		return b.String()
	}
	declared := func(any) *mcp.CallToolResult {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: section(toolutil.HintAction("project.get", "read it"))}}}
	}
	specs := specsResolvingNextSteps([]actioncatalog.SurfaceToolSpec{
		{Name: "declared", FormatResult: declared},
		{Name: "undeclared"},
	}, resolve)

	if text := specs[0].FormatResult(nil).Content[0].(*mcp.TextContent).Text; text != section("Use resolved project.get to read it") {
		t.Errorf("declared formatter text = %q", text)
	}
	if specs[1].FormatResult == nil {
		t.Fatal("a spec declaring no formatter was left with none, so registration would bypass the resolver")
	}
	// The shared dispatcher answers nil for a value no formatter is
	// registered for, and the dispatcher's own fallback renders it; the
	// wrapper hands that nil on rather than failing on it.
	if got := specs[1].FormatResult(struct{ unregistered bool }{}); got != nil {
		t.Errorf("shared dispatcher result for an unregistered value = %#v, want nil", got)
	}
}
