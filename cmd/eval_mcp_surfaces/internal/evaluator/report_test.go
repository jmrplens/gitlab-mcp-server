package evaluator

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestWriteStatusReport_WritesStartupAndErrorSections verifies status reports
// are useful even when the evaluator stops before final metrics are produced.
func TestWriteStatusReport_WritesStartupAndErrorSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "report.md")
	opts := options{Model: "model", ToolSurface: config.ToolSurfaceDynamic, Backend: backendGitLab, TraceDir: "traces", ExposeResources: true, CapabilityAccessActive: true}
	if err := writeErrorReport(path, opts, errors.New("line one\nline two")); err != nil {
		t.Fatalf("writeErrorReport() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	content := string(data)
	for _, want := range []string{"# Dynamic Surface Model Evaluation", "Status: `failed`", "Trace artifacts: `traces`", "    line one", "MCP capability bridge: `enabled`"} {
		if !strings.Contains(content, want) {
			t.Fatalf("report missing %q:\n%s", want, content)
		}
	}
}

// TestWriteReport_WritesFullEvaluationMarkdown verifies final report rendering
// includes header metadata, metrics, task rows, diagnostics, and coverage.
func TestWriteReport_WritesFullEvaluationMarkdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports", "eval.md")
	results := []taskResult{{
		Run: 1, Model: "model-a", ToolSurface: config.ToolSurfaceDynamic,
		Task:      evalTask{ID: "MT-001", ExpectedTool: "gitlab_project", ExpectedAction: "get", RequiredParams: []string{"project_id"}},
		FirstTool: "gitlab_project", FirstAction: "get", FinalTool: "gitlab_project", FinalAction: "get",
		CompletedSteps: 1, FirstPass: true, FinalSuccess: false, DestructiveSafe: true,
		ModelCalls: 1, ToolCalls: 1, Notes: []string{"missing final text"},
	}}
	catalog := []modelTool{{Name: "gitlab_project", InputSchema: map[string]any{"type": "object"}}}
	routes := map[string]toolutil.ActionMap{"gitlab_project": {"get": toolutil.ActionRoute{InputSchema: map[string]any{"properties": map[string]any{"project_id": map[string]any{"type": "string"}}}}}}
	if err := writeReport(path, options{Model: "model-a", ToolSurface: config.ToolSurfaceDynamic, Backend: backendMock, Repeat: 1, ExposeResources: true, ResourceAccessActive: true}, results, catalog, routes, true); err != nil {
		t.Fatalf("writeReport() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	content := string(data)
	for _, want := range []string{"# Dynamic Surface Model Evaluation", "Catalog tools: 1", "## Metrics", "## Failure Diagnostics", "## Fixture Tool Coverage", "MT-001", "missing final text"} {
		if !strings.Contains(content, want) {
			t.Fatalf("report missing %q:\n%s", want, content)
		}
	}
}

func TestCheckReportCleanContent_DetectsFailedTaskRows(t *testing.T) {
	content := "## Task Results\n\n" +
		"| Model | Run | Task | Final success | Notes |\n" +
		"| --- | ---: | --- | --- | --- |\n" +
		"| `anthropic:test` | 1 | MT-001 | Yes | - |\n" +
		"| `google:test` | 1 | MT-002 | No | fixture preparation failed |\n"
	status, err := checkReportCleanContent(content)
	if err != nil {
		t.Fatalf("checkReportCleanContent() error = %v", err)
	}
	if status.TotalRows != 2 || status.clean() || len(status.FailedRows) != 1 {
		t.Fatalf("status = %+v, want one failed row", status)
	}
	failed := status.FailedRows[0]
	if failed.Model != "google:test" {
		t.Fatalf("failed model = %q, want cleaned model value", failed.Model)
	}
	if failed.Task != "MT-002" || failed.Notes != "fixture preparation failed" {
		t.Fatalf("failed row = %+v, want MT-002 fixture failure", failed)
	}
}

func TestEscapeTable_NormalizesMultilineCells(t *testing.T) {
	got := escapeTable("google status 404: {\n  \"error\": true\n} | retry")
	want := "google status 404: {<br>  \"error\": true<br>} \\| retry"
	if got != want {
		t.Fatalf("escapeTable() = %q, want %q", got, want)
	}
}

func TestCheckReportCleanContent_AllowsRepairedFirstPassWhenFinalSuccess(t *testing.T) {
	content := "## Task Results\n\n" +
		"| Model | Run | Task | First pass | Repair | Final success | Notes |\n" +
		"| --- | ---: | --- | --- | --- | --- | --- |\n" +
		"| `openai:test` | 1 | MT-003 | No | Yes | Yes | repaired invalid params |\n"
	status, err := checkReportCleanContent(content)
	if err != nil {
		t.Fatalf("checkReportCleanContent() error = %v", err)
	}
	if !status.clean() || status.TotalRows != 1 {
		t.Fatalf("status = %+v, want clean repaired final success", status)
	}
}

func TestCheckReportCleanContent_RequiresTaskResultsTable(t *testing.T) {
	if _, err := checkReportCleanContent("# Startup failure\n\nStatus: `failed`\n"); err == nil {
		t.Fatal("checkReportCleanContent() error = nil, want missing task results error")
	}
}

// TestReportHeaderHelpers_RenderModeAndTitle verifies report labels remain
// stable across dynamic, meta, and dry-run modes.
func TestReportHeaderHelpers_RenderModeAndTitle(t *testing.T) {
	if !shouldWriteStartupReport(options{Output: "report.md"}) || shouldWriteStartupReport(options{Output: "report.md", FixturesOnly: true}) {
		t.Fatal("shouldWriteStartupReport() did not respect Output and FixturesOnly")
	}
	if got := reportTitle(config.ToolSurfaceMeta); got != "Meta-Tool Model Evaluation" {
		t.Fatalf("reportTitle(meta) = %q", got)
	}
	if got := reportMode(true); got != "static route/schema validation" {
		t.Fatalf("reportMode(dry) = %q", got)
	}
}

// TestReportMetricSections_RenderRunModelUsageAndBridgeTables verifies report
// helpers produce the operational tables consumed by trend analysis.
func TestReportMetricSections_RenderRunModelUsageAndBridgeTables(t *testing.T) {
	results := []taskResult{
		{
			Run: 1, Model: "model-a", Task: evalTask{ID: "MT-001", ExpectedTool: "gitlab_project", ExpectedAction: "get"},
			FirstTool: "gitlab_project", FirstAction: "get", FirstPass: true, FinalTool: "gitlab_project", FinalAction: "get", FinalSuccess: true, DestructiveSafe: true,
			ModelCalls: 1, ToolCalls: 1, ResourceCalls: 1, CapabilityCalls: 1, Usage: modelUsage{InputTokens: 1000, OutputTokens: 2000},
			Trace: taskTrace{Events: []traceEvent{{Kind: "tool_use", Tool: resourceReadTool, Input: map[string]any{"uri": "gitlab://tools/project.get"}}}},
		},
		{
			Run: 2, Model: "model-b", Task: evalTask{ID: "MT-002", Steps: []evalStep{{ExpectedTool: promptGetTool}, {ExpectedTool: completionTool}}},
			FirstTool: promptGetTool, FinalTool: completionTool, FinalSuccess: false, DestructiveSafe: true,
			ModelCalls: 1, ToolCalls: 2, CapabilityCalls: 2, Usage: modelUsage{InputTokens: 3000, OutputTokens: 4000},
			Trace: taskTrace{Events: []traceEvent{{Kind: "tool_use", Tool: completionTool, Input: map[string]any{"ref_type": "ref/prompt", "name": "project_overview", "argument_name": "project_id"}}}},
		},
	}
	var b strings.Builder
	writePerRunMetrics(&b, results)
	writePerModelMetrics(&b, results)
	writeUsageSummary(&b, options{Pricing: pricingOptions{InputPerMTok: 1, OutputPerMTok: 2}}, results, false)
	writeCapabilityBridgeUsage(&b, results, false)
	content := b.String()
	for _, want := range []string{"## Per-Run Metrics", "## Per-Model Metrics", "## API Usage", "$0.0160", "gitlab://tools/project.get", "completion:ref/prompt"} {
		if !strings.Contains(content, want) {
			t.Fatalf("report content missing %q:\n%s", want, content)
		}
	}
	if !resultsHaveMultipleModels(results) || len(resultsByModel(results)) != 2 {
		t.Fatalf("model grouping failed: %#v", resultsByModel(results))
	}
	if got := expectedDisplay(results[1].Task); !strings.Contains(got, promptGetTool) || !strings.Contains(got, completionTool) {
		t.Fatalf("expectedDisplay() = %q, want prompt and completion steps", got)
	}
}

// TestReportPricingHelpers_ResolveFlagsDefaultsAndUnknowns verifies pricing
// resolution prefers flags, then known defaults, then no-cost unknown models.
func TestReportPricingHelpers_ResolveFlagsDefaultsAndUnknowns(t *testing.T) {
	flagPricing := resolvePricing(options{Pricing: pricingOptions{InputPerMTok: 1}})
	if flagPricing.Source != "flags" || !pricingConfigured(flagPricing.Pricing) {
		t.Fatalf("flag pricing = %+v, want flags", flagPricing)
	}
	sonnet := resolvePricingForModel(options{}, "claude-3-7-sonnet-latest")
	if !strings.Contains(sonnet.Source, "Sonnet") || estimateCostUSD(modelUsage{InputTokens: 1_000_000}, sonnet.Pricing) == 0 {
		t.Fatalf("sonnet pricing = %+v, want default estimate", sonnet)
	}
	if unknown := resolvePricingForModel(options{}, "openai:gpt,google:gemini"); unknown.Source != "" {
		t.Fatalf("multi-model pricing = %+v, want unconfigured", unknown)
	}
}

// TestWriteCoverageReportIfRequested_WritesOnlyWhenConfigured verifies optional
// route coverage reports are skipped by default and emitted when requested.
func TestWriteCoverageReportIfRequested_WritesOnlyWhenConfigured(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {"get": toolutil.ActionRoute{}, "delete": toolutil.ActionRoute{Destructive: true}},
	}
	results := []taskResult{{Task: evalTask{ExpectedTool: "gitlab_project", ExpectedAction: "get"}}}
	if err := writeCoverageReportIfRequested(options{}, results, routes); err != nil {
		t.Fatalf("writeCoverageReportIfRequested(disabled) error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "coverage", "routes.md")
	if err := writeCoverageReportIfRequested(options{CoverageReport: path, TasksPath: "tasks.md"}, results, routes); err != nil {
		t.Fatalf("writeCoverageReportIfRequested(enabled) error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read coverage report: %v", err)
	}
	if content := string(data); !strings.Contains(content, "# Schema Route Coverage Report") || !strings.Contains(content, "gitlab_project/delete") {
		t.Fatalf("coverage report = %s, want missing delete route", content)
	}
}

// TestRouteDomainName_UsesDynamicActionDomain verifies dynamic coverage reports
// group execute_action routes by their canonical action domain.
func TestRouteDomainName_UsesDynamicActionDomain(t *testing.T) {
	if got := routeDomainName(dynamicExecuteActionTool, "repository.file_delete"); got != "repository" {
		t.Fatalf("routeDomainName(dynamic execute) = %q, want repository", got)
	}
	if got := routeDomainName("gitlab", "merge_request.create"); got != "merge_request" {
		t.Fatalf("routeDomainName(unified) = %q, want merge_request", got)
	}
}
