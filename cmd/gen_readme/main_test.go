// main_test.go verifies README generation helpers used by cmd/gen_readme.
package main

import (
	"encoding/json"
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
		{Configuration: "`dynamic` / `full` (default)", VisibleTools: 2, ReachableActions: 867, ToolSchemaTokens: 1962, SharedTokens: 18198},
		{Configuration: "`dynamic` / `minimal`", VisibleTools: 2, ReachableActions: 867, ToolSchemaTokens: 1962, SharedTokens: 184},
		{Configuration: "`meta` / `full`", MetaParamSchema: config.MetaParamSchemaOpaque, VisibleTools: 34, ReachableActions: 867, ToolSchemaTokens: 63932, SharedTokens: 18198},
		{Configuration: "`meta` / `minimal`", MetaParamSchema: config.MetaParamSchemaOpaque, VisibleTools: 34, ReachableActions: 867, ToolSchemaTokens: 63932, SharedTokens: 760},
		{Configuration: "`individual` / `full`", VisibleTools: 863, ReachableActions: 863, ToolSchemaTokens: 451000, SharedTokens: 17622},
	}

	got := renderTokenFootprint(config.MetaParamSchemaOpaque, rows)
	for _, want := range []string{
		"| Configuration (`TOOL_SURFACE` / `CAPABILITY_SURFACE`) | Visible tools | Reachable actions | `META_PARAM_SCHEMA` | Tool schema tokens | Shared tokens | Total tokens |",
		"`dynamic` / `full` (default)",
		"20,160",
		"`meta` / `full`",
		"`opaque`",
		"82,130",
		"`META_PARAM_SCHEMA=opaque` affects only visible meta-tool input schemas",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderTokenFootprint() missing %q:\n%s", want, got)
		}
	}
	assertBefore(t, got, "`dynamic` / `full`", "`dynamic` / `minimal`")
	assertBefore(t, got, "`dynamic` / `minimal`", "`meta` / `full`")
	assertBefore(t, got, "`meta` / `full`", "`meta` / `minimal`")
	assertBefore(t, got, "`meta` / `minimal`", "`individual` / `full`")
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

	rows, err := measureTokenFootprintRows(client, config.MetaParamSchemaOpaque)
	if err != nil {
		t.Fatalf("measureTokenFootprintRows() error = %v", err)
	}
	wantOrder := []string{
		"`dynamic` / `full` (default)",
		"`dynamic` / `minimal`",
		"`meta` / `full`",
		"`meta` / `minimal`",
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
		t.Fatalf("meta schema modes = %q/%q, want opaque", rows[2].MetaParamSchema, rows[3].MetaParamSchema)
	}
	if rows[4].MetaParamSchema != "" {
		t.Fatalf("individual schema mode = %q, want empty n/a marker", rows[4].MetaParamSchema)
	}
	if rows[4].VisibleTools <= rows[2].VisibleTools {
		t.Fatalf("individual visible tools = %d, want greater than meta %d", rows[4].VisibleTools, rows[2].VisibleTools)
	}

	compactRows, err := measureTokenFootprintRows(client, config.MetaParamSchemaCompact)
	if err != nil {
		t.Fatalf("measureTokenFootprintRows(compact) error = %v", err)
	}
	if compactRows[0].ToolSchemaTokens != rows[0].ToolSchemaTokens {
		t.Fatalf("dynamic tool schema tokens changed with META_PARAM_SCHEMA: compact %d, opaque %d", compactRows[0].ToolSchemaTokens, rows[0].ToolSchemaTokens)
	}
	if compactRows[2].ToolSchemaTokens <= rows[2].ToolSchemaTokens {
		t.Fatalf("compact meta tool schema tokens = %d, want greater than opaque %d", compactRows[2].ToolSchemaTokens, rows[2].ToolSchemaTokens)
	}
	if compactRows[2].MetaParamSchema != config.MetaParamSchemaCompact {
		t.Fatalf("compact meta schema mode = %q, want compact", compactRows[2].MetaParamSchema)
	}
}

// TestMeasureToolSchemaTokens_UsesAggregateBytesBeforeDivision verifies the
// token estimate follows the documented byte/4 heuristic over the aggregate
// payload instead of flooring each tool independently.
func TestMeasureToolSchemaTokens_UsesAggregateBytesBeforeDivision(t *testing.T) {
	toolList := []*mcp.Tool{{Name: "a"}, {Name: "bb"}, {Name: "ccc"}}

	totalBytes := 0
	perToolFlooredTokens := 0
	for _, tool := range toolList {
		payload, err := json.Marshal(tool)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		totalBytes += len(payload)
		perToolFlooredTokens += len(payload) / readmeBytesPerToken
	}
	want := totalBytes / readmeBytesPerToken
	if perToolFlooredTokens == want {
		t.Fatalf("test fixture does not distinguish aggregate and per-tool rounding: both = %d", want)
	}

	got, err := measureToolSchemaTokens(toolList)
	if err != nil {
		t.Fatalf("measureToolSchemaTokens() error = %v", err)
	}
	if got != want {
		t.Fatalf("measureToolSchemaTokens() = %d, want aggregate byte estimate %d", got, want)
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
