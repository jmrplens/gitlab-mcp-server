package main

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

var errNoop = errors.New("noop")

// TestCollectRouteOutputSchemaFindings_MixedRoutes_ReturnsOneMissingSchemaFinding verifies
// that route output-schema auditing reports only routes without schemas.
func TestCollectRouteOutputSchemaFindings_MixedRoutes_ReturnsOneMissingSchemaFinding(t *testing.T) {
	t.Parallel()

	noop := func(context.Context, map[string]any) (any, error) { return nil, errNoop }
	routes := map[string]toolutil.ActionMap{
		"gitlab_analyze": {
			"issue_summary": {
				Handler:      noop,
				OutputSchema: toolutil.SchemaForRoute[toolutil.VoidOutput](),
			},
		},
		"gitlab_package": {
			"missing": {Handler: noop},
			"valid": {
				Handler:      noop,
				OutputSchema: toolutil.SchemaForRoute[toolutil.VoidOutput](),
			},
		},
	}

	got := collectRouteOutputSchemaFindings(routes)
	if len(got) != 1 {
		t.Fatalf("collectRouteOutputSchemaFindings returned %d findings, want 1: %#v", len(got), got)
	}
	if got[0].tool != "gitlab_package" {
		t.Fatalf("finding tool = %q, want gitlab_package", got[0].tool)
	}
	if got[0].category != "route-output-schema" {
		t.Fatalf("finding category = %q, want route-output-schema", got[0].category)
	}
}

// TestCollectRouteOutputSchemaFindings_DoesNotSkipAnalyzeRoutes verifies that
// analyze meta-tool routes are included in output-schema auditing.
func TestCollectRouteOutputSchemaFindings_DoesNotSkipAnalyzeRoutes(t *testing.T) {
	t.Parallel()

	noop := func(context.Context, map[string]any) (any, error) { return nil, errNoop }
	routes := map[string]toolutil.ActionMap{
		"gitlab_analyze": {
			"issue_summary": {Handler: noop},
		},
	}

	got := collectRouteOutputSchemaFindings(routes)
	if len(got) != 1 {
		t.Fatalf("collectRouteOutputSchemaFindings returned %d findings, want 1: %#v", len(got), got)
	}
	if got[0].tool != "gitlab_analyze" {
		t.Fatalf("finding tool = %q, want gitlab_analyze", got[0].tool)
	}
}

// TestPct_ZeroTotal_ReturnsZero verifies that percentage rendering handles an
// empty denominator without division by zero.
func TestPct_ZeroTotal_ReturnsZero(t *testing.T) {
	t.Parallel()
	if got := pct(5, 0); got != 0 {
		t.Fatalf("pct(5,0) = %d, want 0", got)
	}
}

// TestPct_HalfCoverage_ReturnsFifty verifies that percentage rendering rounds a
// half-covered ratio to fifty percent.
func TestPct_HalfCoverage_ReturnsFifty(t *testing.T) {
	t.Parallel()
	if got := pct(50, 100); got != 50 {
		t.Fatalf("pct(50,100) = %d, want 50", got)
	}
}

// TestPct_FullCoverage_ReturnsHundred verifies that percentage rendering
// reports one hundred percent when all items are covered.
func TestPct_FullCoverage_ReturnsHundred(t *testing.T) {
	t.Parallel()
	if got := pct(10, 10); got != 100 {
		t.Fatalf("pct(10,10) = %d, want 100", got)
	}
}

// TestAuditOutputSchema_MissingSchema_ReturnsFindings verifies that individual
// MCP tools without an output schema are reported.
func TestAuditOutputSchema_MissingSchema_ReturnsFindings(t *testing.T) {
	t.Parallel()

	tools := []*mcp.Tool{
		{Name: "tool_no_schema"},
		{Name: "tool_with_schema", OutputSchema: map[string]any{"type": "object"}},
	}
	got := auditOutputSchema(tools, "individual")
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].tool != "tool_no_schema" {
		t.Fatalf("finding.tool = %q, want tool_no_schema", got[0].tool)
	}
}

// TestAuditOutputSchema_AllPresent_NoFindings verifies that complete output
// schemas produce no findings.
func TestAuditOutputSchema_AllPresent_NoFindings(t *testing.T) {
	t.Parallel()

	tools := []*mcp.Tool{
		{Name: "a", OutputSchema: map[string]any{"type": "object"}},
	}
	if got := auditOutputSchema(tools, "individual"); len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(got))
	}
}

// TestAuditDescriptionReturns_Missing_ReturnsFindings verifies that tool
// descriptions without a Returns section are reported.
func TestAuditDescriptionReturns_Missing_ReturnsFindings(t *testing.T) {
	t.Parallel()

	tools := []*mcp.Tool{
		{Name: "no_returns", Description: "Does something."},
		{Name: "has_returns", Description: "Does something. Returns: the result."},
	}
	got := auditDescriptionReturns(tools, "individual")
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(got), got)
	}
	if got[0].tool != "no_returns" {
		t.Fatalf("finding.tool = %q, want no_returns", got[0].tool)
	}
}

// TestAuditDescriptionReturns_AllPresent_NoFindings verifies that descriptions
// with Returns sections produce no findings.
func TestAuditDescriptionReturns_AllPresent_NoFindings(t *testing.T) {
	t.Parallel()

	tools := []*mcp.Tool{
		{Name: "ok", Description: "Does something. Returns: result."},
	}
	if got := auditDescriptionReturns(tools, "individual"); len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(got))
	}
}

// TestAuditTitle_Missing_ReturnsFindings verifies that untitled tools are
// reported by the title audit.
func TestAuditTitle_Missing_ReturnsFindings(t *testing.T) {
	t.Parallel()

	tools := []*mcp.Tool{
		{Name: "no_title"},
		{Name: "has_title", Title: "My Tool"},
	}
	got := auditTitle(tools, "individual")
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].tool != "no_title" {
		t.Fatalf("finding.tool = %q, want no_title", got[0].tool)
	}
}

// TestAuditTitle_AllPresent_NoFindings verifies that titled tools produce no
// title audit findings.
func TestAuditTitle_AllPresent_NoFindings(t *testing.T) {
	t.Parallel()

	tools := []*mcp.Tool{{Name: "ok", Title: "OK Tool"}}
	if got := auditTitle(tools, "individual"); len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(got))
	}
}

// TestAuditSeeAlso_Missing_ReturnsFindings verifies that tool descriptions
// without related-tool guidance are reported.
func TestAuditSeeAlso_Missing_ReturnsFindings(t *testing.T) {
	t.Parallel()

	tools := []*mcp.Tool{
		{Name: "no_seealso", Description: "Does something. Returns: result."},
		{Name: "has_seealso", Description: "Does something. See also: other_tool."},
	}
	got := auditSeeAlso(tools, "individual")
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].tool != "no_seealso" {
		t.Fatalf("finding.tool = %q, want no_seealso", got[0].tool)
	}
}

// TestAuditSeeAlso_AllPresent_NoFindings verifies that descriptions with See
// also guidance produce no findings.
func TestAuditSeeAlso_AllPresent_NoFindings(t *testing.T) {
	t.Parallel()

	tools := []*mcp.Tool{{Name: "ok", Description: "See also: other."}}
	if got := auditSeeAlso(tools, "individual"); len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(got))
	}
}

// TestAuditRouteOutputSchema_AllSchemasPresent_ReturnsNoFindings verifies that
// all registered meta-routes expose output schemas.
func TestAuditRouteOutputSchema_AllSchemasPresent_ReturnsNoFindings(t *testing.T) {
	t.Parallel()
	// The full registered meta-routes all have OutputSchema after the refactor.
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	if got := collectRouteOutputSchemaFindings(catalog.ActionMaps()); len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %+v", len(got), got)
	}
}

// TestCollectToolQualityStats_CountsAllDimensions verifies that the summary
// aggregator counts schema, returns, title, and see-also independently.
func TestCollectToolQualityStats_CountsAllDimensions(t *testing.T) {
	t.Parallel()

	toolList := []*mcp.Tool{
		{Name: "full", Description: "Returns: ok. See also: x.", Title: "T", OutputSchema: map[string]any{"type": "object"}},
		{Name: "partial", Description: "Returns: ok.", Title: "P"},
		{Name: "empty"},
	}

	got := collectToolQualityStats(toolList)
	if got.Schema != 1 {
		t.Errorf("Schema = %d, want 1", got.Schema)
	}
	if got.Returns != 2 {
		t.Errorf("Returns = %d, want 2", got.Returns)
	}
	if got.Title != 2 {
		t.Errorf("Title = %d, want 2", got.Title)
	}
	if got.SeeAlso != 1 {
		t.Errorf("SeeAlso = %d, want 1", got.SeeAlso)
	}
}

// TestCollectToolQualityStats_EmptyInput verifies the summary returns zero
// stats when no tools are present.
func TestCollectToolQualityStats_EmptyInput(t *testing.T) {
	t.Parallel()

	got := collectToolQualityStats(nil)
	if got != (toolQualityStats{}) {
		t.Fatalf("empty stats = %+v, want zero value", got)
	}
}

// TestPrintReport_EmptyFindingsWritesNoFindingsMessage verifies the report
// prints the success message when no findings are present.
func TestPrintReport_EmptyFindingsWritesNoFindingsMessage(t *testing.T) {
	// Not t.Parallel: captureStdout rebinds os.Stdout and parallel tests would
	// race for the global writer.

	output := captureStdout(t, func() {
		printReport([]*mcp.Tool{{Name: "ok", Title: "OK"}}, nil, nil)
	})

	for _, want := range []string{
		"# MCP Output Quality Audit Report",
		"| Total tools | 1 | 0 |",
		"**No findings — all quality checks pass.**",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("printReport() output missing %q:\n%s", want, output)
		}
	}
}

// TestPrintReport_GroupsFindingsByCategory verifies findings are grouped and
// listed in the report.
func TestPrintReport_GroupsFindingsByCategory(t *testing.T) {
	// Not t.Parallel: captureStdout rebinds os.Stdout and parallel tests would
	// race for the global writer.

	findings := []finding{
		{tool: "tool_a", category: "title", detail: "missing Title"},
		{tool: "tool_b", category: "title", detail: "missing Title"},
		{tool: "tool_c", category: "description-returns", detail: "no Returns"},
	}

	output := captureStdout(t, func() {
		printReport([]*mcp.Tool{{Name: "tool_a"}}, []*mcp.Tool{{Name: "tool_b"}}, findings)
	})

	for _, want := range []string{
		"| description-returns | 1 |",
		"| title | 2 |",
		"| **Total findings** | **3** |",
		"## title (2)",
		"## description-returns (1)",
		"`tool_a` | missing Title",
		"`tool_c` | no Returns",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("printReport() output missing %q:\n%s", want, output)
		}
	}
}

// TestAuditOutputSchema_MetaKindProducesExpectedDetail verifies the meta kind
// label appears in findings.
func TestAuditOutputSchema_MetaKindProducesExpectedDetail(t *testing.T) {
	t.Parallel()

	got := auditOutputSchema([]*mcp.Tool{{Name: "x"}}, "meta")
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if !strings.Contains(got[0].detail, "meta tool missing OutputSchema") {
		t.Fatalf("detail = %q, want meta kind label", got[0].detail)
	}
}

// TestAuditDescriptionReturns_MetaKindProducesExpectedDetail verifies the meta
// label appears in description-returns findings.
func TestAuditDescriptionReturns_MetaKindProducesExpectedDetail(t *testing.T) {
	t.Parallel()

	got := auditDescriptionReturns([]*mcp.Tool{{Name: "x"}}, "meta")
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if !strings.Contains(got[0].detail, "meta description lacks") {
		t.Fatalf("detail = %q", got[0].detail)
	}
}

// TestAuditTitle_MetaKindProducesExpectedDetail verifies the meta label appears
// in title findings.
func TestAuditTitle_MetaKindProducesExpectedDetail(t *testing.T) {
	t.Parallel()

	got := auditTitle([]*mcp.Tool{{Name: "x"}}, "meta")
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if !strings.Contains(got[0].detail, "meta tool missing Title") {
		t.Fatalf("detail = %q", got[0].detail)
	}
}

// TestAuditSeeAlso_MetaKindProducesExpectedDetail verifies the meta label
// appears in see-also findings.
func TestAuditSeeAlso_MetaKindProducesExpectedDetail(t *testing.T) {
	t.Parallel()

	got := auditSeeAlso([]*mcp.Tool{{Name: "x"}}, "meta")
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if !strings.Contains(got[0].detail, "meta description lacks") {
		t.Fatalf("detail = %q", got[0].detail)
	}
}

// TestAuditRouteOutputSchema_EmptyRoutesProducesNoFindings verifies the
// collector returns no findings for an empty route map.
func TestAuditRouteOutputSchema_EmptyRoutesProducesNoFindings(t *testing.T) {
	t.Parallel()

	if got := collectRouteOutputSchemaFindings(map[string]toolutil.ActionMap{}); len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(got))
	}
}

// captureStdout captures the output written to os.Stdout while fn runs.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = oldStdout })

	fn()
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatalf("writer.Close() error: %v", closeErr)
	}
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error: %v", err)
	}
	if closeErr := reader.Close(); closeErr != nil {
		t.Fatalf("reader.Close() error: %v", closeErr)
	}
	return string(output)
}
