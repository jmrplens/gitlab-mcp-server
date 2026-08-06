// main_test.go contains focused tests for the audit_tokens command. Tests use
// a local GitLab version mock and exercise the resource token measurement path
// that depends on the surface-aware tool manifest resources.
//
// Coverage focuses on the resource registration options (including the
// minimal candidate) and the small pure helpers (domain parsing, total
// tokens, number formatting, table printing) that compose the audit report.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// newAuditTokensClient creates a [gitlabclient.Client] backed by a mock
// /api/v4/version endpoint for audit_tokens tests.
func newAuditTokensClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	t.Cleanup(srv.Close)

	client, err := gitlabclient.NewClient(&config.Config{GitLabURL: srv.URL, GitLabToken: "audit-token"})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	return client
}

// TestMeasureResources_IncludesToolManifest verifies the token audit measures
// the surface-aware tool manifest in addition to static resources.
func TestMeasureResources_IncludesToolManifest(t *testing.T) {
	client := newAuditTokensClient(t)
	routes := buildMetaActionMaps(client, false)
	dynamicCatalog := actioncatalog.FromActionMaps(routes)
	dynamicTools := listDynamicTools(dynamicCatalog)
	manifestTokens := measureResourcesWithOptions(client, routes, resourceRegistrationOptions{
		ToolManifest: true,
		ToolSurface:  config.ToolSurfaceDynamic,
		ToolList:     dynamicTools,
		ToolCatalog:  dynamicCatalog,
	})
	bareTokens := measureResourcesWithOptions(client, nil, resourceRegistrationOptions{})
	if manifestTokens <= bareTokens {
		t.Fatalf("manifest resource tokens = %d, want greater than bare server %d", manifestTokens, bareTokens)
	}
}

// TestMeasureResourcesWithOptions_MinimalCandidate verifies the dynamic-minimal
// candidate keeps the tool manifest while dropping the
// heavier optional resource groups.
func TestMeasureResourcesWithOptions_MinimalCandidate(t *testing.T) {
	client := newAuditTokensClient(t)
	routes := buildMetaActionMaps(client, false)
	dynamicCatalog := actioncatalog.FromActionMaps(routes)
	dynamicTools := listDynamicTools(dynamicCatalog)
	fullDynamicTokens := measureResources(client, routes, dynamicCatalog, dynamicTools, config.ToolSurfaceDynamic)
	minimalTokens := measureResourcesWithOptions(client, routes, resourceRegistrationOptions{
		ToolManifest: true,
		ToolSurface:  config.ToolSurfaceDynamic,
		ToolList:     dynamicTools,
		ToolCatalog:  dynamicCatalog,
	})

	if minimalTokens <= 0 {
		t.Fatalf("minimal resource tokens = %d, want positive tool-manifest estimate", minimalTokens)
	}
	if minimalTokens >= fullDynamicTokens {
		t.Fatalf("minimal resource tokens = %d, want less than full dynamic %d", minimalTokens, fullDynamicTokens)
	}
}

// TestListDynamicTools_ExposesLowTokenSurface verifies the dynamic audit path
// measures the find/execute tools backed by the canonical action catalog.
func TestListDynamicTools_ExposesLowTokenSurface(t *testing.T) {
	client := newAuditTokensClient(t)
	routes := buildMetaActionMaps(client, false)
	if countActions(routes) == 0 {
		t.Fatal("buildMetaActionMaps() returned no actions")
	}

	toolList := listDynamicTools(actioncatalog.FromActionMaps(routes))
	names := make([]string, 0, len(toolList))
	for _, tool := range toolList {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "gitlab_execute_action,gitlab_find_action" {
		t.Fatalf("dynamic tools = %q, want find/execute", got)
	}
}

// TestExtractDomain_ParsesGitlabToolNames verifies the domain extractor returns
// the second segment for gitlab_{domain}_{action} names and "unknown" for
// malformed inputs.
func TestExtractDomain_ParsesGitlabToolNames(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "two segment", in: "gitlab_project", want: "project"},
		{name: "three segment", in: "gitlab_project_list", want: "project"},
		{name: "four segment", in: "gitlab_merge_request_approvals", want: "merge"},
		{name: "single segment", in: "gitlab", want: "unknown"},
		{name: "empty", in: "", want: "unknown"},
		{name: "no prefix", in: "project_list", want: "list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractDomain(tt.in); got != tt.want {
				t.Fatalf("extractDomain(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestTotalTokens_SumsTokenEstimates verifies the aggregator sums the Tokens
// field across the provided tool info records.
func TestTotalTokens_SumsTokenEstimates(t *testing.T) {
	got := totalTokens([]toolTokenInfo{
		{Name: "a", Tokens: 10},
		{Name: "b", Tokens: 20},
		{Name: "c", Tokens: 30},
	})
	if got != 60 {
		t.Fatalf("totalTokens() = %d, want 60", got)
	}
}

// TestTotalTokens_EmptyInput verifies the aggregator returns zero for an
// empty input slice.
func TestTotalTokens_EmptyInput(t *testing.T) {
	if got := totalTokens(nil); got != 0 {
		t.Fatalf("totalTokens(nil) = %d, want 0", got)
	}
}

// TestCountActions_AggregatesAcrossRoutes verifies the route count aggregator
// sums action counts across all route keys.
func TestCountActions_AggregatesAcrossRoutes(t *testing.T) {
	// Build a route map with three actions split across two tools.
	noop := func(_ context.Context, _ map[string]any) (any, error) { return nil, nil } //nolint:nilnil // test fixture: always no-ops
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {
			"get":   {Handler: noop},
			"list":  {Handler: noop},
			"stats": {Handler: noop},
		},
		"gitlab_issue": {
			"create": {Handler: noop},
		},
	}
	if got := countActions(routes); got != 4 {
		t.Fatalf("countActions() = %d, want 4", got)
	}
}

// TestCountActions_EmptyRoutesReturnsZero verifies the aggregator returns zero
// for an empty route map.
func TestCountActions_EmptyRoutesReturnsZero(t *testing.T) {
	if got := countActions(map[string]toolutil.ActionMap{}); got != 0 {
		t.Fatalf("countActions() = %d, want 0", got)
	}
}

// TestFmtNum_AddsThousandsSeparators verifies the format helper inserts comma
// separators in the right positions for the supported ranges.
func TestFmtNum_AddsThousandsSeparators(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{in: 0, want: "0"},
		{in: 1, want: "1"},
		{in: 42, want: "42"},
		{in: 999, want: "999"},
		{in: 1000, want: "1,000"},
		{in: 1234, want: "1,234"},
		{in: 12345, want: "12,345"},
		{in: 123456, want: "123,456"},
		{in: 1234567, want: "1,234,567"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := fmtNum(tt.in); got != tt.want {
				t.Fatalf("fmtNum(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestMeasureTools_AssignsDomainAndComputesTokens verifies the tool token
// estimator captures name, domain, byte length, and token count.
func TestMeasureTools_AssignsDomainAndComputesTokens(t *testing.T) {
	toolList := []*mcp.Tool{
		{Name: "gitlab_project_get", Description: "Get a project."},
		{Name: "gitlab_issue_create", Description: "Create an issue."},
	}

	got := measureTools(toolList)
	if len(got) != 2 {
		t.Fatalf("measureTools() returned %d items, want 2", len(got))
	}
	// Results are sorted descending by tokens; assert names appear with
	// the expected metadata.
	names := map[string]bool{}
	for _, info := range got {
		names[info.Name] = true
		if info.Tokens <= 0 {
			t.Errorf("info %q: Tokens=%d, want > 0 (real tokenizer)", info.Name, info.Tokens)
		}
		if info.Domain == "" {
			t.Errorf("info %q: Domain is empty", info.Name)
		}
		if info.Bytes <= 0 {
			t.Errorf("info %q: Bytes = %d, want positive", info.Name, info.Bytes)
		}
	}
	for _, want := range []string{"gitlab_project_get", "gitlab_issue_create"} {
		if !names[want] {
			t.Errorf("measureTools() missing %q in results: %+v", want, got)
		}
	}
}

// TestMeasureTools_EmptyInputReturnsEmpty verifies the estimator returns an
// empty slice for an empty tool list.
func TestMeasureTools_EmptyInputReturnsEmpty(t *testing.T) {
	got := measureTools(nil)
	if len(got) != 0 {
		t.Fatalf("measureTools(nil) = %d items, want 0", len(got))
	}
}

// TestMeasurePrompts_ReturnsTokenEstimateForRegisteredPrompts verifies the
// prompt token estimator produces a positive count for a real client.
func TestMeasurePrompts_ReturnsTokenEstimateForRegisteredPrompts(t *testing.T) {
	got := measurePrompts(newAuditTokensClient(t))
	if got <= 0 {
		t.Fatalf("measurePrompts() = %d, want positive token estimate", got)
	}
}

// TestPrintTopTools_TruncatesToRequestedLimit verifies the printer caps the
// number of output rows at the requested n parameter.
func TestPrintTopTools_TruncatesToRequestedLimit(t *testing.T) {
	infos := []toolTokenInfo{
		{Name: "tool_a", Tokens: 300, Bytes: 1200},
		{Name: "tool_b", Tokens: 200, Bytes: 800},
		{Name: "tool_c", Tokens: 100, Bytes: 400},
	}

	output := captureStdoutAudit(t, func() {
		printTopTools(infos, 2)
	})

	if !strings.Contains(output, "tool_a") || !strings.Contains(output, "tool_b") {
		t.Fatalf("printTopTools() missing first two rows:\n%s", output)
	}
	if strings.Contains(output, "tool_c") {
		t.Fatalf("printTopTools() included rows beyond n=2:\n%s", output)
	}
}

// TestPrintTopTools_NLargerThanLength verifies the printer uses the full input
// length when n exceeds the available records.
func TestPrintTopTools_NLargerThanLength(t *testing.T) {
	infos := []toolTokenInfo{{Name: "only", Tokens: 10, Bytes: 40}}

	output := captureStdoutAudit(t, func() {
		printTopTools(infos, 100)
	})

	if !strings.Contains(output, "only") {
		t.Fatalf("printTopTools() missing single record:\n%s", output)
	}
}

// TestPrintDomainTotals_AggregatesAndSortsByTokenCost verifies the printer
// groups tools by domain, sums tokens, sorts descending, and limits rows.
func TestPrintDomainTotals_AggregatesAndSortsByTokenCost(t *testing.T) {
	infos := []toolTokenInfo{
		{Name: "gitlab_project_get", Domain: "project", Tokens: 100, Bytes: 400},
		{Name: "gitlab_project_list", Domain: "project", Tokens: 50, Bytes: 200},
		{Name: "gitlab_issue_get", Domain: "issue", Tokens: 30, Bytes: 120},
	}

	output := captureStdoutAudit(t, func() {
		printDomainTotals(infos, 10)
	})

	if !strings.Contains(output, "project") || !strings.Contains(output, "issue") {
		t.Fatalf("printDomainTotals() missing domain rows:\n%s", output)
	}
	// project should appear before issue because total tokens (150) > 30.
	assertBefore(t, output, "project", "issue")
	// Count column should be 2 for project (two tools) and 1 for issue.
	if !strings.Contains(output, "2") {
		t.Fatalf("printDomainTotals() output missing count column:\n%s", output)
	}
}

// TestPrintDomainTotals_RespectsLimit verifies the printer caps the row count
// at the requested n parameter.
func TestPrintDomainTotals_RespectsLimit(t *testing.T) {
	infos := []toolTokenInfo{
		{Name: "gitlab_a", Domain: "a", Tokens: 100},
		{Name: "gitlab_b", Domain: "b", Tokens: 80},
		{Name: "gitlab_c", Domain: "c", Tokens: 60},
	}

	output := captureStdoutAudit(t, func() {
		printDomainTotals(infos, 1)
	})

	if !strings.Contains(output, "a") {
		t.Fatalf("printDomainTotals() missing first row:\n%s", output)
	}
	if strings.Contains(output, "| b ") || strings.Contains(output, "| c ") {
		t.Fatalf("printDomainTotals() included rows beyond n=1:\n%s", output)
	}
}

// TestPrintDomainTotals_EmptyInput verifies the printer renders the table
// header without data rows for an empty input.
func TestPrintDomainTotals_EmptyInput(t *testing.T) {
	output := captureStdoutAudit(t, func() {
		printDomainTotals(nil, 5)
	})

	if !strings.Contains(output, "Domain") {
		t.Fatalf("printDomainTotals() missing header for empty input:\n%s", output)
	}
}

// captureStdoutAudit captures os.Stdout while fn runs and returns the result
// as a string.
func captureStdoutAudit(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stdout = w

	fn()
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("writer.Close() error: %v", closeErr)
	}
	os.Stdout = oldStdout
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("io.ReadAll() error: %v", err)
	}
	if closeErr := r.Close(); closeErr != nil {
		t.Fatalf("reader.Close() error: %v", closeErr)
	}
	return string(out)
}

// assertBefore verifies that the before substring occurs before the after
// substring in s.
func assertBefore(t *testing.T, s, before, after string) {
	t.Helper()
	bi := strings.Index(s, before)
	if bi < 0 {
		t.Fatalf("%q not found in:\n%s", before, s)
	}
	ai := strings.Index(s, after)
	if ai < 0 {
		t.Fatalf("%q not found in:\n%s", after, s)
	}
	if bi >= ai {
		t.Fatalf("%q should appear before %q in:\n%s", before, after, s)
	}
}

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

// TestRunMetaSchemaSizing_Completes verifies the meta-schema sizing can build
// the full base-plus-enterprise meta-tool registry and measure schema sizes.
// Migrated from the former cmd/audit_meta_schema/main_test.go.
func TestRunMetaSchemaSizing_Completes(t *testing.T) {
	if err := runMetaSchemaSizing(); err != nil {
		t.Fatalf("runMetaSchemaSizing() error: %v", err)
	}
}

// TestHumanBytes_AllMagnitudes verifies the humanBytes byte formatter emits
// expected B/KB/MB suffixes for the three supported magnitude ranges.
func TestHumanBytes_AllMagnitudes(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want string
	}{
		{name: "sub-kilobyte renders as B", n: 512, want: "512 B"},
		{name: "kilobyte threshold renders as KB", n: 1024, want: "1.0 KB"},
		{name: "megabyte threshold renders as MB", n: 1024 * 1024, want: "1.0 MB"},
		{name: "large value renders as MB", n: 3 * 1024 * 1024, want: "3.0 MB"},
		{name: "zero renders as B", n: 0, want: "0 B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := humanBytes(tt.n); got != tt.want {
				t.Fatalf("humanBytes(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}
