// footprint_test.go verifies the -footprint mode of audit_tokens, migrated
// from the former cmd/gen_readme binary.
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestRenderReadmeFootprint_DynamicOnlyWithTiers verifies the README footprint
// table keeps only the dynamic surface (default configuration) across all
// tiers and links to the detailed reference, without the meta/individual rows.
func TestRenderReadmeFootprint_DynamicOnlyWithTiers(t *testing.T) {
	rows := []tokenFootprintRow{
		{Tier: "Free/CE", Configuration: "`dynamic` / `full` (default)", VisibleTools: 2, ToolSchemaTokens: 2180, SharedTokens: 31758},
		{Tier: "Free/CE", Configuration: "`dynamic` / `minimal`", VisibleTools: 2, ToolSchemaTokens: 2180, SharedTokens: 1088},
		{Tier: "Free/CE", Configuration: "`meta` / `full` (opaque)", MetaParamSchema: "opaque", VisibleTools: 33, ToolSchemaTokens: 136890, SharedTokens: 31758},
		{Tier: "Ultimate", Configuration: "`individual` / `full`", VisibleTools: 1061, ToolSchemaTokens: 964044, SharedTokens: 31758},
	}
	got := renderReadmeFootprint(rows)
	for _, want := range []string{
		"`dynamic` / `full` (default)",
		"Free/CE",
		"Token Footprint Reference",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderReadmeFootprint() missing %q", want)
		}
	}
	if strings.Contains(got, "`meta`") {
		t.Fatal("renderReadmeFootprint() should not include meta rows")
	}
	if strings.Contains(got, "`individual`") {
		t.Fatal("renderReadmeFootprint() should not include individual rows")
	}
}

// TestFootprintStaleTargets verifies the drift detector backing
// -footprint -check: it reports no stale targets when the README section and
// detailed doc match the rendered content, and names exactly the target(s)
// that diverge otherwise.
func TestFootprintStaleTargets(t *testing.T) {
	rows := completeFootprintRows()
	// A README whose managed section already holds the rendered content.
	readme := footprintStartMarker + "\n\n" + renderReadmeFootprint(rows) + "\n" + footprintEndMarker + "\n"
	detailed := renderDetailedFootprint(rows)
	siteJSON, renderErr := renderSiteFootprintJSON(rows)
	if renderErr != nil {
		t.Fatalf("renderSiteFootprintJSON() error: %v", renderErr)
	}
	site := string(siteJSON)

	t.Run("current content reports no stale targets", func(t *testing.T) {
		stale, err := footprintStaleTargets(readme, detailed, site, rows)
		if err != nil {
			t.Fatalf("footprintStaleTargets(current) error: %v", err)
		}
		if len(stale) != 0 {
			t.Fatalf("expected no stale targets, got %v", stale)
		}
	})

	t.Run("detailed-doc drift reports only the detailed doc", func(t *testing.T) {
		stale, err := footprintStaleTargets(readme, detailed+"\nextra drift\n", site, rows)
		if err != nil {
			t.Fatalf("footprintStaleTargets(stale detailed) error: %v", err)
		}
		if len(stale) != 1 || !strings.Contains(stale[0], detailedFootprintPath) {
			t.Fatalf("expected only %q stale, got %v", detailedFootprintPath, stale)
		}
	})

	t.Run("site JSON drift reports only the site data file", func(t *testing.T) {
		stale, err := footprintStaleTargets(readme, detailed, `{"tokenizer":"stale"}`, rows)
		if err != nil {
			t.Fatalf("footprintStaleTargets(stale site) error: %v", err)
		}
		if len(stale) != 1 || !strings.Contains(stale[0], siteFootprintPath) {
			t.Fatalf("expected only %q stale, got %v", siteFootprintPath, stale)
		}
	})

	t.Run("missing README markers returns an error", func(t *testing.T) {
		if _, err := footprintStaleTargets("no markers here", detailed, site, rows); err == nil {
			t.Fatal("expected error when README lacks the footprint markers")
		}
	})
}

// completeFootprintRows returns a fixture covering every row the site-facing
// JSON needs: the dynamic default on the Ultimate tier plus one individual row
// per tier.
func completeFootprintRows() []tokenFootprintRow {
	return []tokenFootprintRow{
		{Tier: "Free/CE", Configuration: dynamicDefaultConfiguration, VisibleTools: 2, ToolSchemaTokens: 2180, SharedTokens: 31758},
		{Tier: "Free/CE", Configuration: individualConfiguration, VisibleTools: 847, ToolSchemaTokens: 767793, SharedTokens: 31758},
		{Tier: "Premium", Configuration: dynamicDefaultConfiguration, VisibleTools: 2, ToolSchemaTokens: 2180, SharedTokens: 31758},
		{Tier: "Premium", Configuration: individualConfiguration, VisibleTools: 999, ToolSchemaTokens: 917625, SharedTokens: 31758},
		{Tier: ultimateTierLabel, Configuration: dynamicDefaultConfiguration, VisibleTools: 2, ToolSchemaTokens: 2180, SharedTokens: 31758},
		{Tier: ultimateTierLabel, Configuration: individualConfiguration, VisibleTools: 1065, ToolSchemaTokens: 966698, SharedTokens: 31758},
	}
}

// TestRenderSiteFootprintJSON verifies the site-facing headline extract: the
// dynamic surface is taken from the Ultimate tier, every tier gets an
// individual-surface entry, and the reduction factor is derived rather than
// restated. This is the number the docs site publishes as its headline claim,
// so a wrong ratio would propagate straight into AI answers.
func TestRenderSiteFootprintJSON(t *testing.T) {
	raw, renderErr := renderSiteFootprintJSON(completeFootprintRows())
	if renderErr != nil {
		t.Fatalf("renderSiteFootprintJSON() error: %v", renderErr)
	}

	var got siteFootprint
	if unmarshalErr := json.Unmarshal(raw, &got); unmarshalErr != nil {
		t.Fatalf("unmarshal site footprint: %v", unmarshalErr)
	}

	if got.Tokenizer != "cl100k_base" {
		t.Errorf("Tokenizer = %q, want cl100k_base", got.Tokenizer)
	}
	if got.Dynamic.VisibleTools != 2 || got.Dynamic.ToolSchemaTokens != 2180 {
		t.Errorf("Dynamic = %+v, want {2 2180}", got.Dynamic)
	}
	if len(got.Individual) != 3 {
		t.Fatalf("len(Individual) = %d, want 3", len(got.Individual))
	}
	// 966698 / 2180 = 443.4 -> 443
	if f := got.Individual["ultimate"].ReductionFactor; f != 443 {
		t.Errorf("ultimate ReductionFactor = %d, want 443", f)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Error("site footprint JSON must end with a trailing newline for prettier")
	}
}

// TestRenderSiteFootprintJSON_RejectsIncompleteMatrix verifies generation fails
// loudly rather than publishing a partial headline claim.
func TestRenderSiteFootprintJSON_RejectsIncompleteMatrix(t *testing.T) {
	tests := []struct {
		name string
		rows []tokenFootprintRow
	}{
		{
			name: "missing the ultimate dynamic row",
			rows: []tokenFootprintRow{
				{Tier: ultimateTierLabel, Configuration: individualConfiguration, VisibleTools: 1065, ToolSchemaTokens: 966698},
			},
		},
		{
			name: "missing one individual tier",
			rows: []tokenFootprintRow{
				{Tier: ultimateTierLabel, Configuration: dynamicDefaultConfiguration, VisibleTools: 2, ToolSchemaTokens: 2180},
				{Tier: ultimateTierLabel, Configuration: individualConfiguration, VisibleTools: 1065, ToolSchemaTokens: 966698},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := renderSiteFootprintJSON(tt.rows); err == nil {
				t.Fatal("renderSiteFootprintJSON() error = nil, want error")
			}
		})
	}
}

// TestMeasureTokenFootprintRows_AllTiersAllModes verifies the full measurement
// matrix: 3 tiers \u00d7 9 configurations, tier ordering, meta schema-mode token
// scaling, and tier-based individual tool scaling.
func TestMeasureTokenFootprintRows_AllTiersAllModes(t *testing.T) {
	client := newAuditTokensClient(t)

	rows, err := measureTokenFootprintRows(client)
	if err != nil {
		t.Fatalf("measureTokenFootprintRows() error = %v", err)
	}
	if len(rows) != 27 {
		t.Fatalf("measureTokenFootprintRows() returned %d rows, want 27 (3 tiers \u00d7 9)", len(rows))
	}
	// Verify tier ordering: Free/CE first 9, Premium next 9, Ultimate last 9.
	tiers := []string{"Free/CE", "Premium", "Ultimate"}
	for ti, tier := range tiers {
		for i := range 9 {
			row := rows[ti*9+i]
			if row.Tier != tier {
				t.Fatalf("row[%d].Tier = %q, want %q", ti*9+i, row.Tier, tier)
			}
		}
	}
	// Verify mode ordering within each tier: opaque < compact < full tokens.
	for ti := range tiers {
		base := ti * 9
		opaque := rows[base+2].ToolSchemaTokens
		compact := rows[base+4].ToolSchemaTokens
		full := rows[base+6].ToolSchemaTokens
		if compact <= opaque || full <= compact {
			t.Fatalf("tier %s: meta tokens not increasing opaque(%d) < compact(%d) < full(%d)", tiers[ti], opaque, compact, full)
		}
	}
	// Verify tier scaling: Free individual tools < Premium < Ultimate.
	freeIndiv := rows[8].VisibleTools
	premIndiv := rows[17].VisibleTools
	ultIndiv := rows[26].VisibleTools
	if freeIndiv >= premIndiv || premIndiv >= ultIndiv {
		t.Fatalf("individual tool count not increasing by tier: Free(%d) < Premium(%d) < Ultimate(%d)", freeIndiv, premIndiv, ultIndiv)
	}
}

// TestMeasureToolSchemaTokens_UsesRealTokenizer verifies the schema token
// estimator runs the cl100k_base tokenizer, not the bytes/4 fallback. It counts
// the same serialized tools both ways and asserts they differ, so a tokenizer
// init/encode regression (silently dropping to bytes/4) fails here. The
// tokenizer itself is unit-tested in tokens_test.go.
func TestMeasureToolSchemaTokens_UsesRealTokenizer(t *testing.T) {
	toolList := []*mcp.Tool{{Name: "a"}, {Name: "bb"}, {Name: "ccc"}}

	got, err := measureToolSchemaTokens(toolList)
	if err != nil {
		t.Fatalf("measureToolSchemaTokens() error = %v", err)
	}
	if got <= 0 {
		t.Fatalf("measureToolSchemaTokens() = %d, want > 0", got)
	}

	fallback := 0
	for _, tl := range toolList {
		b, marshalErr := json.Marshal(tl)
		if marshalErr != nil {
			t.Fatalf("marshal: %v", marshalErr)
		}
		fallback += len(b) / 4
	}
	if got == fallback {
		t.Fatalf("measureToolSchemaTokens() = %d == bytes/4 fallback; cl100k_base tokenizer did not engage", got)
	}
}
