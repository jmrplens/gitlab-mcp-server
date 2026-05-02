package main

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	if got := auditRouteOutputSchema(); len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %+v", len(got), got)
	}
}
