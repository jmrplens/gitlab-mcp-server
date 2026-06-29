// main_test.go verifies README generation helpers used by cmd/gen_readme.
//
// Tests cover the token-footprint renderer (row order, column structure,
// total tokens), the live measurement path against the mock-backed base
// catalog, the byte/4 token heuristic, and the META_PARAM_SCHEMA env
// parser (including invalid-value rejection).
package main

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestRenderTokenFootprint_IncludesOrderedConfigurationRows verifies the README
// token footprint table keeps the requested configuration order and schema-mode
// column without reintroducing the detailed meta-tool catalog.
func TestRenderTokenFootprint_IncludesOrderedConfigurationRows(t *testing.T) {
	rows := []tokenFootprintRow{
		{Configuration: "`dynamic` / `full` (default)", VisibleTools: 2, ReachableActions: 864, ToolSchemaTokens: 2180, SharedTokens: 31758},
		{Configuration: "`dynamic` / `minimal`", VisibleTools: 2, ReachableActions: 864, ToolSchemaTokens: 2180, SharedTokens: 1088},
		{Configuration: "`meta` / `full` (opaque)", MetaParamSchema: config.MetaParamSchemaOpaque, VisibleTools: 33, ReachableActions: 864, ToolSchemaTokens: 136890, SharedTokens: 31758},
		{Configuration: "`meta` / `minimal` (opaque)", MetaParamSchema: config.MetaParamSchemaOpaque, VisibleTools: 33, ReachableActions: 864, ToolSchemaTokens: 136890, SharedTokens: 1088},
		{Configuration: "`meta` / `full` (compact)", MetaParamSchema: config.MetaParamSchemaCompact, VisibleTools: 33, ReachableActions: 864, ToolSchemaTokens: 203471, SharedTokens: 31758},
		{Configuration: "`meta` / `minimal` (compact)", MetaParamSchema: config.MetaParamSchemaCompact, VisibleTools: 33, ReachableActions: 864, ToolSchemaTokens: 203471, SharedTokens: 1088},
		{Configuration: "`meta` / `full` (full)", MetaParamSchema: config.MetaParamSchemaFull, VisibleTools: 33, ReachableActions: 864, ToolSchemaTokens: 291158, SharedTokens: 31758},
		{Configuration: "`meta` / `minimal` (full)", MetaParamSchema: config.MetaParamSchemaFull, VisibleTools: 33, ReachableActions: 864, ToolSchemaTokens: 291158, SharedTokens: 1088},
		{Configuration: "`individual` / `full`", VisibleTools: 860, ReachableActions: 860, ToolSchemaTokens: 779989, SharedTokens: 31758},
	}

	got := renderTokenFootprint(rows)
	for _, want := range []string{
		"| Configuration (`TOOL_SURFACE` / `CAPABILITY_SURFACE`) | Visible tools | Reachable actions | `META_PARAM_SCHEMA` | Tool schema tokens | Shared tokens | Total tokens |",
		"`dynamic` / `full` (default)",
		"`meta` / `full` (opaque)",
		"`opaque`",
		"`compact`",
		"`full`",
		"`META_PARAM_SCHEMA` affects only visible meta-tool input schemas",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderTokenFootprint() missing %q:\n%s", want, got)
		}
	}
	assertBefore(t, got, "`dynamic` / `full`", "`dynamic` / `minimal`")
	assertBefore(t, got, "`dynamic` / `minimal`", "`meta` / `full` (opaque)")
	assertBefore(t, got, "`meta` / `full` (opaque)", "`meta` / `minimal` (opaque)")
	assertBefore(t, got, "`meta` / `minimal` (full)", "`individual` / `full`")
	if strings.Contains(got, "| Meta-Tool | Actions | Description |") {
		t.Fatalf("renderTokenFootprint() should not include detailed meta-tool table:\n%s", got)
	}
}

// TestMeasureTokenFootprintRows_BaseCatalog_ReturnsRequestedConfigurations
// verifies the real mock-backed measurement path produces the README rows in
// the intended order and with minimal/full capability differences preserved.
func TestMeasureTokenFootprintRows_BaseCatalog_ReturnsRequestedConfigurations(t *testing.T) {
	client, closeClient, err := newReadmeClient()
	if err != nil {
		t.Fatalf("newReadmeClient() error = %v", err)
	}
	defer closeClient()

	rows, err := measureTokenFootprintRows(client)
	if err != nil {
		t.Fatalf("measureTokenFootprintRows() error = %v", err)
	}
	wantOrder := []string{
		"`dynamic` / `full` (default)",
		"`dynamic` / `minimal`",
		"`meta` / `full` (opaque)",
		"`meta` / `minimal` (opaque)",
		"`meta` / `full` (compact)",
		"`meta` / `minimal` (compact)",
		"`meta` / `full` (full)",
		"`meta` / `minimal` (full)",
		"`individual` / `full`",
	}
	if len(rows) != len(wantOrder) {
		t.Fatalf("measureTokenFootprintRows() returned %d rows, want %d", len(rows), len(wantOrder))
	}
	for i, want := range wantOrder {
		if rows[i].Configuration != want {
			t.Fatalf("row[%d].Configuration = %q, want %q", i, rows[i].Configuration, want)
		}
	}
	if rows[0].VisibleTools != 2 || rows[1].VisibleTools != 2 {
		t.Fatalf("dynamic visible tools = %d/%d, want 2/2", rows[0].VisibleTools, rows[1].VisibleTools)
	}
	if rows[0].SharedTokens <= rows[1].SharedTokens {
		t.Fatalf("dynamic full shared tokens = %d, want greater than minimal %d", rows[0].SharedTokens, rows[1].SharedTokens)
	}
	if rows[3].SharedTokens != rows[1].SharedTokens {
		t.Fatalf("meta minimal shared tokens = %d, want same as dynamic minimal %d", rows[3].SharedTokens, rows[1].SharedTokens)
	}
	if rows[2].MetaParamSchema != config.MetaParamSchemaOpaque || rows[3].MetaParamSchema != config.MetaParamSchemaOpaque {
		t.Fatalf("meta opaque schema modes = %q/%q, want opaque", rows[2].MetaParamSchema, rows[3].MetaParamSchema)
	}
	if rows[4].MetaParamSchema != config.MetaParamSchemaCompact || rows[6].MetaParamSchema != config.MetaParamSchemaFull {
		t.Fatalf("meta schema modes = %q/%q, want compact/full", rows[4].MetaParamSchema, rows[6].MetaParamSchema)
	}
	indivIdx := len(rows) - 1
	if rows[indivIdx].MetaParamSchema != "" {
		t.Fatalf("individual schema mode = %q, want empty n/a marker", rows[indivIdx].MetaParamSchema)
	}
	if rows[indivIdx].VisibleTools <= rows[2].VisibleTools {
		t.Fatalf("individual visible tools = %d, want greater than meta %d", rows[indivIdx].VisibleTools, rows[2].VisibleTools)
	}

	// Verify all 3 META_PARAM_SCHEMA modes measured in one run, with increasing token costs.
	opaqueTokens := rows[2].ToolSchemaTokens
	compactTokens := rows[4].ToolSchemaTokens
	fullTokens := rows[6].ToolSchemaTokens
	if compactTokens <= opaqueTokens {
		t.Fatalf("compact meta tokens = %d, want greater than opaque %d", compactTokens, opaqueTokens)
	}
	if fullTokens <= compactTokens {
		t.Fatalf("full meta tokens = %d, want greater than compact %d", fullTokens, compactTokens)
	}
	if rows[4].MetaParamSchema != config.MetaParamSchemaCompact {
		t.Fatalf("compact meta schema mode = %q, want compact", rows[4].MetaParamSchema)
	}
	if rows[6].MetaParamSchema != config.MetaParamSchemaFull {
		t.Fatalf("full meta schema mode = %q, want full", rows[6].MetaParamSchema)
	}
}

// TestMeasureToolSchemaTokens_UsesAggregateBytesBeforeDivision verifies the
// token estimate follows the documented byte/4 heuristic over the aggregate
// payload instead of flooring each tool independently.
func TestMeasureToolSchemaTokens_UsesAggregateBytesBeforeDivision(t *testing.T) {
	toolList := []*mcp.Tool{{Name: "a"}, {Name: "bb"}, {Name: "ccc"}}

	got, err := measureToolSchemaTokens(toolList)
	if err != nil {
		t.Fatalf("measureToolSchemaTokens() error = %v", err)
	}
	if got <= 0 {
		t.Fatalf("measureToolSchemaTokens() = %d, want > 0 (real tokenizer)", got)
	}
}

// TestReadMetaParamSchemaMode_DefaultAndConfigured verifies gen_readme defaults
// to opaque schema mode and accepts documented configured values case-insensitively.
func TestReadMetaParamSchemaMode_DefaultAndConfigured(t *testing.T) {
	t.Setenv("META_PARAM_SCHEMA", "")
	got, err := readMetaParamSchemaMode()
	if err != nil {
		t.Fatalf("readMetaParamSchemaMode() error = %v", err)
	}
	if got != config.DefaultMetaParamSchema {
		t.Fatalf("default schema mode = %q, want %q", got, config.DefaultMetaParamSchema)
	}

	t.Setenv("META_PARAM_SCHEMA", " Compact ")
	got, err = readMetaParamSchemaMode()
	if err != nil {
		t.Fatalf("readMetaParamSchemaMode() configured error = %v", err)
	}
	if got != config.MetaParamSchemaCompact {
		t.Fatalf("configured schema mode = %q, want %q", got, config.MetaParamSchemaCompact)
	}
}

// TestReadMetaParamSchemaMode_InvalidRejectsValue verifies gen_readme fails
// fast when the configured schema mode cannot be measured accurately.
func TestReadMetaParamSchemaMode_InvalidRejectsValue(t *testing.T) {
	t.Setenv("META_PARAM_SCHEMA", "verbose")
	_, err := readMetaParamSchemaMode()
	if err == nil {
		t.Fatal("readMetaParamSchemaMode() error = nil, want invalid value error")
	}
	if !strings.Contains(err.Error(), "META_PARAM_SCHEMA must be one of") {
		t.Fatalf("readMetaParamSchemaMode() error = %v, want allowed-values message", err)
	}
}

func assertBefore(t *testing.T, s, before, after string) {
	t.Helper()
	beforeIndex := strings.Index(s, before)
	if beforeIndex < 0 {
		t.Fatalf("%q not found in:\n%s", before, s)
	}
	afterIndex := strings.Index(s, after)
	if afterIndex < 0 {
		t.Fatalf("%q not found in:\n%s", after, s)
	}
	if beforeIndex >= afterIndex {
		t.Fatalf("%q should appear before %q in:\n%s", before, after, s)
	}
}
