package evaluator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestComparisonFormattingHelpers_NormalizeReportValues verifies report parsing
// helpers tolerate markdown table formatting and empty comparison values.
func TestComparisonFormattingHelpers_NormalizeReportValues(t *testing.T) {
	if got := parseReportPercent(" `1,234.5%` "); got != 1234.5 {
		t.Fatalf("parseReportPercent() = %v, want 1234.5", got)
	}
	if got := parseReportInt(" `1,234 calls` "); got != 1234 {
		t.Fatalf("parseReportInt() = %d, want 1234", got)
	}
	if got := formatMetric(12.34); got != "12.3%" {
		t.Fatalf("formatMetric() = %q, want 12.3%%", got)
	}
	if got := formatDelta(-2.25); got != "-2.2 pp" {
		t.Fatalf("formatDelta() = %q, want -2.2 pp", got)
	}
	if got := emptyDash(" "); got != "-" {
		t.Fatalf("emptyDash(blank) = %q, want dash", got)
	}
	if got := valueOrZero(" "); got != "0" {
		t.Fatalf("valueOrZero(blank) = %q, want zero", got)
	}
}

// TestWriteComparisonReport_BuildsEvaluationAndTokenSections verifies comparison
// reports parse evaluation and token audit inputs and write a combined summary.
func TestWriteComparisonReport_BuildsEvaluationAndTokenSections(t *testing.T) {
	dir := t.TempDir()
	evalA := filepath.Join(dir, "dynamic.md")
	evalB := filepath.Join(dir, "meta.md")
	contentA := "# Dynamic Surface Model Evaluation\n\nMode: model tool-calling\nModel: `model-a`\nTool surface: `dynamic`\nBackend: `gitlab`\nTool execution: `mcp`\nCatalog tools: 2\nRuns: 1\nTask attempts: 1\n\n## Metrics\n\n| Metric | Value |\n| --- | ---: |\n| Tool-selection accuracy | 90.0% |\n| Action-selection accuracy | 80.0% |\n| First-call validation pass rate | 70.0% |\n| Schema lookup use rate | 10.0% |\n| Repair success rate | 50.0% |\n| Destructive safety | 100.0% |\n| Final task success proxy | 60.0% |\n\n## API Usage\n\n| Metric | Value |\n| --- | ---: |\n| Model requests | 1 |\n| Tool calls emitted | 2 |\n| Input tokens | 3 |\n| Output tokens | 4 |\n| Estimated cost | $0.0001 |\n"
	contentB := strings.ReplaceAll(contentA, "Dynamic Surface", "Meta-Tool")
	contentB = strings.ReplaceAll(contentB, "`dynamic`", "`meta`")
	contentB = strings.ReplaceAll(contentB, "90.0%", "95.0%")
	for path, content := range map[string]string{evalA: contentA, evalB: contentB} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write input %s: %v", path, err)
		}
	}
	out := filepath.Join(dir, "nested", "comparison.md")
	if err := writeComparisonReport(out, []string{evalA, evalB}); err != nil {
		t.Fatalf("writeComparisonReport() error = %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read comparison: %v", err)
	}
	content := string(data)
	for _, want := range []string{"# MCP Surface Evaluation Comparison", "## Evaluation Metrics", "### Delta Versus", "## API Usage", "+5.0 pp"} {
		if !strings.Contains(content, want) {
			t.Fatalf("comparison missing %q:\n%s", want, content)
		}
	}
	oneInputErr := writeComparisonReport(filepath.Join(dir, "bad.md"), []string{evalA})
	if oneInputErr == nil {
		t.Fatal("writeComparisonReport(one input) error = nil, want error")
	}
}

// TestWriteUsageComparison_UsesLegacyToolCallFallback verifies older reports
// with Tool calls but no Tool calls emitted still render usage comparisons.
func TestWriteUsageComparison_UsesLegacyToolCallFallback(t *testing.T) {
	var b strings.Builder
	writeUsageComparison(&b, []comparisonInput{{
		Kind:  "evaluation",
		Label: "old-report",
		Usage: map[string]string{
			usageModelRequests: "2",
			usageToolCalls:     "3",
		},
	}})
	if !strings.Contains(b.String(), "| `old-report` | 2 | 3 | 0 | 0 | - |") {
		t.Fatalf("usage comparison = %s", b.String())
	}
}

// TestWriteDiagnosticsComparison_RendersValidSeparator verifies dynamic
// diagnostic columns do not add an extra empty Markdown table column.
func TestWriteDiagnosticsComparison_RendersValidSeparator(t *testing.T) {
	var b strings.Builder
	writeDiagnosticsComparison(&b, []comparisonInput{{
		Label:       "run-a",
		Diagnostics: map[string]int{"model route miss": 1, "fixture gap": 2},
	}})
	if strings.Contains(b.String(), "| --- | ---: | ---: | |") {
		t.Fatalf("diagnostics table has an extra empty column:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "| --- | ---: | ---: |") {
		t.Fatalf("diagnostics table missing separator:\n%s", b.String())
	}
}

// TestSortedIntKeys_ReturnsDeterministicOrder verifies map-key rendering is
// stable for comparison reports.
func TestSortedIntKeys_ReturnsDeterministicOrder(t *testing.T) {
	keys := sortedIntKeys(map[string]int{"b": 2, "a": 1})
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("sortedIntKeys() = %v, want a,b", keys)
	}
}
