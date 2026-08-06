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

// TestCheckReportCleanContent_DetectsFailedTaskRows verifies that
// checkReportCleanContent parses a Task Results table with one failed row,
// counts the total rows, identifies the failing model and task, and records
// the failure notes.
//
// The test feeds a handcrafted markdown table containing one clean row and
// one failed row and asserts that the returned status reports the correct
// totals, marks the report as not clean, and exposes the failing row
// details. This protects the CI gate from approving reports that contain
// fixture preparation failures.
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

// TestEscapeTable_NormalizesMultilineCells verifies that escapeTable
// replaces newlines with <br> tags and escapes pipe characters so multi-line
// notes survive the markdown table renderer without breaking the row layout.
//
// The test feeds a known multi-line error message containing a pipe and
// asserts the helper produces the expected escaped form. This protects the
// report writer from emitting tables that GitHub's markdown renderer will
// misinterpret.
func TestEscapeTable_NormalizesMultilineCells(t *testing.T) {
	got := escapeTable("google status 404: {\n  \"error\": true\n} | retry")
	want := "google status 404: {<br>  \"error\": true<br>} \\| retry"
	if got != want {
		t.Fatalf("escapeTable() = %q, want %q", got, want)
	}
}

// TestCheckReportCleanContent_AllowsRepairedFirstPassWhenFinalSuccess
// verifies that checkReportCleanContent accepts a task whose first call
// failed but a later repair recovered the task, marking the report clean
// despite the No/Yes first-pass/Repair combination.
//
// The test feeds a Task Results row showing a No first pass and Yes final
// success and asserts the status reports clean. This protects the CI gate
// from rejecting reports that include legitimate repair-driven recoveries.
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

// TestCheckReportCleanContent_RequiresTaskResultsTable verifies that
// checkReportCleanContent returns an error when the supplied content lacks
// the Task Results table that the gate relies on.
//
// The test feeds only the startup failure header and asserts the helper
// rejects the content. This protects the CI gate from silently approving
// reports that were cut short before any task was attempted.
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

// TestFailureDiagnosticCategory_SeparatesPhase4Buckets covers FailureDiagnosticCategory with table-driven subtests for separates phase 4 buckets.
func TestFailureDiagnosticCategory_SeparatesPhase4Buckets(t *testing.T) {
	tests := []struct {
		name  string
		notes []string
		want  string
	}{
		{"unmarshal type error", []string{"json: cannot unmarshal string into Go struct field id of type int64"}, "mcp_implementation_bug"},
		{"gitlab 5xx", []string{"GitLab 503 service unavailable"}, "transient_gitlab_5xx"},
		{"premium license", []string{"feature requires Premium license"}, "gitlab_ce_limitation"},
		{"missing fixture identity", []string{"fixture state is missing project identity"}, "fixture_setup_failure"},
		{"wrong action", []string{"expected action issue.create, got project.create"}, "model_route_selection_miss"},
		{"unknown param", []string{"unknown params for gitlab/issue.create: iid"}, "model_parameter_shape_miss"},
		{"missing required param", []string{"missing required project_id"}, "model_parameter_shape_miss"},
		{"unconfirmed destructive", []string{"destructive task requires params.confirm=true"}, "destructive_safety"},
		{"deadline exceeded", []string{"context deadline exceeded"}, "timeout_resource_exhaustion"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := failureDiagnosticCategory(tt.notes); got != tt.want {
				t.Errorf("failureDiagnosticCategory(%q) = %q, want %q", strings.Join(tt.notes, "; "), got, tt.want)
			}
		})
	}
}

// TestDynamicFailureDiagnosticCategory_SeparatesDiscoveryBuckets verifies that
// dynamic-mode failures map to discovery follow-up buckets.
func TestDynamicFailureDiagnosticCategory_SeparatesDiscoveryBuckets(t *testing.T) {
	tests := []struct {
		name  string
		notes []string
		want  string
	}{
		{name: "ranker miss", notes: []string{"dynamic ranker miss: expected top action repository.compare, got pipeline.list"}, want: "ranker_miss"},
		{name: "alias miss", notes: []string{"step 1: expected action repository.file_get, got repository_file.get"}, want: "alias_miss"},
		{name: "standalone unavailable", notes: []string{"step 1: expected tool gitlab_discover_project, got gitlab_execute_action; standalone tool uses top-level input fields, not params"}, want: "standalone_unavailable"},
		{name: "params shape", notes: []string{"step 1: missing required params: project_id"}, want: "params_shape_miss"},
		{name: "standalone params shape", notes: []string{"step 1: missing required project_id"}, want: "params_shape_miss"},
		{name: "multi step order", notes: []string{"tool-call step limit reached after 2/3 scenario steps"}, want: "multi_step_order_miss"},
		{name: "true discovery", notes: []string{"model returned no tool_use block"}, want: "true_discovery_miss"},
	}
	opts := options{ToolSurface: config.ToolSurfaceDynamic}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := taskResult{Notes: tt.notes}
			if got := failureDiagnosticCategoryForResult(opts, result); got != tt.want {
				t.Fatalf("failureDiagnosticCategoryForResult() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBuildRouteCoverageReport_ListsUncoveredHighRiskRoutes verifies BuildRouteCoverageReport lists uncovered high risk routes.
func TestBuildRouteCoverageReport_ListsUncoveredHighRiskRoutes(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab": {
			"issue.list":               {},
			"project.delete":           {},
			"repository.file_get":      {},
			"merge_train.list_project": {},
		},
	}
	results := []taskResult{{Task: evalTask{ID: "covered", ExpectedTool: "gitlab", ExpectedAction: "issue.list"}}}

	report := buildRouteCoverageReport(options{TasksPath: "fixture.md", Partition: "base-read"}, results, routes)
	for _, want := range []string{"Schema Route Coverage Report", "project.delete", "repository.file_get", "merge_train.list_project", "enterprise_schema_only"} {
		if !strings.Contains(report, want) {
			t.Fatalf("coverage report missing %q:\n%s", want, report)
		}
	}
}

// TestFailureDiagnosticCategory_ClassifiesCommonLiveErrors covers FailureDiagnosticCategory with table-driven subtests for classifies common live errors.
func TestFailureDiagnosticCategory_ClassifiesCommonLiveErrors(t *testing.T) {
	tests := []struct {
		name  string
		notes []string
		want  string
	}{
		{name: "int64 coercion", notes: []string{"json: cannot unmarshal string into Go struct field issue_iid of type int64"}, want: "mcp_implementation_bug"},
		{name: "gitlab 500", notes: []string{"environmentStop: GitLab internal server error: 500"}, want: "transient_gitlab_5xx"},
		{name: "missing resource", notes: []string{"404 Not Found"}, want: "not_found"},
		{name: "provider auth", notes: []string{"qwen status 401: invalid_api_key"}, want: "model_provider_auth"},
		{name: "provider model unavailable", notes: []string{"google status 404: models/gemini-3.0-flash is not found"}, want: "model_provider_model_unavailable"},
		{name: "model validation", notes: []string{"step 2: expected action issue.update, got issue.get"}, want: "model_route_selection_miss"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := failureDiagnosticCategory(tt.notes); got != tt.want {
				t.Fatalf("failureDiagnosticCategory() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCalculateMetrics_HandlesNoRepairs verifies CalculateMetrics handles no repairs.
func TestCalculateMetrics_HandlesNoRepairs(t *testing.T) {
	results := []taskResult{{
		Task:            evalTask{ExpectedTool: "gitlab_user", ExpectedAction: "current"},
		FirstTool:       "gitlab_user",
		FirstAction:     "current",
		FirstPass:       true,
		FinalSuccess:    true,
		DestructiveSafe: true,
	}}
	measured := calculateMetrics(results)
	if measured.ToolSelection != 100 || measured.ActionSelection != 100 || measured.RepairSuccess != 100 {
		t.Fatalf("metrics = %+v, want all applicable metrics at 100", measured)
	}
}

// TestCalculateMetrics_AggregatesRepeatedAttempts verifies CalculateMetrics when aggregates repeated attempts.
func TestCalculateMetrics_AggregatesRepeatedAttempts(t *testing.T) {
	results := []taskResult{
		{
			Run:             1,
			Task:            evalTask{ExpectedTool: "gitlab_user", ExpectedAction: "current"},
			FirstTool:       "gitlab_user",
			FirstAction:     "current",
			FirstPass:       true,
			FinalSuccess:    true,
			DestructiveSafe: true,
		},
		{
			Run:             2,
			Task:            evalTask{ExpectedTool: "gitlab_user", ExpectedAction: "current"},
			FirstTool:       "gitlab_project",
			FirstAction:     "get",
			FinalSuccess:    false,
			DestructiveSafe: true,
		},
	}
	measured := calculateMetrics(results)
	if measured.ToolSelection != 50 || measured.ActionSelection != 50 || measured.FinalSuccess != 50 {
		t.Fatalf("metrics = %+v, want repeated attempts aggregated at 50%%", measured)
	}
}

// TestAggregateUsage_SumsRequestsToolCallsAndTokens verifies AggregateUsage when sums requests tool calls and tokens.
func TestAggregateUsage_SumsRequestsToolCallsAndTokens(t *testing.T) {
	results := []taskResult{
		{ModelCalls: 2, ToolCalls: 3, ResourceCalls: 1, CapabilityCalls: 2, Usage: modelUsage{InputTokens: 100, OutputTokens: 20, CacheCreationInputTokens: 50}},
		{ModelCalls: 1, ToolCalls: 1, ResourceCalls: 2, CapabilityCalls: 3, Usage: modelUsage{InputTokens: 25, OutputTokens: 5, CacheReadInputTokens: 200}},
	}
	summary := aggregateUsage(results)
	if summary.ModelCalls != 3 || summary.ToolCalls != 4 || summary.ResourceCalls != 3 || summary.CapabilityCalls != 5 {
		t.Fatalf("summary calls = %+v, want 3 requests and 4 tool calls", summary)
	}
	if summary.Usage.InputTokens != 125 || summary.Usage.OutputTokens != 25 || summary.Usage.CacheCreationInputTokens != 50 || summary.Usage.CacheReadInputTokens != 200 {
		t.Fatalf("usage = %+v, want summed tokens", summary.Usage)
	}
}

// TestCollectCapabilityBridgeUsage_GroupsToolTargetsAndModels verifies that
// collectCapabilityBridgeUsage aggregates capability bridge tool_use events
// from the task trace into per-tool, per-target, per-model buckets.
//
// The test seeds two task results with overlapping tool calls (a resource
// read for project.get and a prompt get for project_overview) and asserts
// the resulting usage slice contains three grouped entries covering the
// shared targets and the model-specific call counts. This protects the
// report from losing capability bridge utilization metrics.
func TestCollectCapabilityBridgeUsage_GroupsToolTargetsAndModels(t *testing.T) {
	results := []taskResult{
		{
			Task:  evalTask{ID: "MT-001"},
			Model: "model-a",
			Trace: taskTrace{Events: []traceEvent{
				{Kind: "tool_use", Tool: resourceReadTool, Input: map[string]any{"uri": "gitlab://tools/project.get"}},
				{Kind: "tool_use", Tool: promptGetTool, Input: map[string]any{"name": "project_overview"}},
			}},
		},
		{
			Task:  evalTask{ID: "MT-002"},
			Model: "model-b",
			Trace: taskTrace{Events: []traceEvent{
				{Kind: "tool_use", Tool: resourceReadTool, Input: map[string]any{"uri": "gitlab://tools/project.get"}},
				{Kind: "tool_use", Tool: completionTool, Input: map[string]any{"ref_type": "ref/prompt", "name": "project_overview", "argument_name": "project_id"}},
			}},
		},
	}

	usage := collectCapabilityBridgeUsage(results)
	if len(usage) != 3 {
		t.Fatalf("usage = %+v, want three grouped entries", usage)
	}
	var b strings.Builder
	writeCapabilityBridgeUsage(&b, results, false)
	report := b.String()
	requireContainsAll(t, "capability bridge usage", report, []string{
		"## MCP Capability Bridge Usage",
		"`gitlab_read_resource` | resources | gitlab://tools/project.get | 2 | model-a, model-b | MT-001, MT-002",
		"`gitlab_get_prompt` | prompts | project_overview | 1 | model-a | MT-001",
		"`gitlab_complete` | completion:ref/prompt | project_overview#project_id | 1 | model-b | MT-002",
	})
}

// TestEstimateCostUSD_UsesPerMillionPricing verifies EstimateCostUSD uses per million pricing.
func TestEstimateCostUSD_UsesPerMillionPricing(t *testing.T) {
	cost := estimateCostUSD(modelUsage{InputTokens: 1_000_000, OutputTokens: 100_000}, pricingOptions{InputPerMTok: 3, OutputPerMTok: 15})
	if cost != 4.5 {
		t.Fatalf("cost = %v, want 4.5", cost)
	}
}

// TestWriteTraceArtifacts_WritesJSONLIndexAndPerTaskFiles verifies WriteTraceArtifacts writes jsonl index and per task files.
func TestWriteTraceArtifacts_WritesJSONLIndexAndPerTaskFiles(t *testing.T) {
	trace := taskTrace{
		Run:          2,
		TaskID:       "MT-002",
		Prompt:       "Find a project.",
		SystemPrompt: systemPrompt(),
		UserPrompt:   "Task MT-002: Find a project.",
		Expected:     []traceExpectedStep{{Step: 1, Tool: "gitlab_project", Action: "get", RequiredParams: []string{"project_id"}}},
		Events:       []traceEvent{{Turn: 1, Kind: "tool_use", Tool: "gitlab_project", Action: "get"}},
		Summary:      traceSummary{FinalSuccess: true, FirstPass: true, CompletedSteps: 1, ExpectedSteps: 1},
	}
	dir := t.TempDir()
	if err := writeTraceArtifacts(dir, []taskResult{{Trace: trace}}, false); err != nil {
		t.Fatalf("writeTraceArtifacts() error = %v", err)
	}

	for _, name := range []string{"index.md", "traces.jsonl", "run-002-MT-002.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(data), "MT-002") {
			t.Fatalf("%s = %s, want task ID", name, data)
		}
	}

	index, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatalf("read index.md: %v", err)
	}
	if strings.Contains(string(index), "provider HTTP request/response bodies") {
		t.Fatalf("index.md = %s, should not promise raw provider bodies when traceProviderBodies=false", index)
	}
}

// TestFixtureToolCoverage_DynamicFindActionRequiresExpectedStep verifies dynamic
// discovery is counted only when fixtures explicitly expect it.
func TestFixtureToolCoverage_DynamicFindActionRequiresExpectedStep(t *testing.T) {
	summary := fixtureToolCoverage([]modelTool{{Name: dynamicFindTool}, {Name: dynamicExecuteActionTool}}, []taskResult{{
		Task: evalTask{ExpectedTool: dynamicExecuteActionTool, ExpectedAction: "user.current"},
	}})
	if summary.Covered != 1 || len(summary.Missing) != 1 || summary.Missing[0] != dynamicFindTool {
		t.Fatalf("fixtureToolCoverage() = %+v, want dynamic find reported missing", summary)
	}
}

// TestWriteStartupReport_CreatesPlaceholder verifies that startup reports are written before model evaluation finishes.
func TestWriteStartupReport_CreatesPlaceholder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "report.md")
	opts := options{
		Model:       "test:model",
		ToolSurface: config.ToolSurfaceDynamic,
		Backend:     backendGitLab,
		Output:      path,
		TraceDir:    defaultTraceDir(path),
	}

	if err := writeStartupReport(path, opts); err != nil {
		t.Fatalf("writeStartupReport() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read startup report: %v", err)
	}
	requireContainsAll(t, "startup report", string(data), []string{
		"# Dynamic Surface Model Evaluation",
		"Status: `running`",
		"Tool surface: `dynamic`",
		"Backend: `gitlab`",
		"It will be replaced by the final metrics report",
	})
}

// TestWriteErrorReport_RecordsFailure verifies that early failures replace the startup placeholder with an error report.
func TestWriteErrorReport_RecordsFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.md")
	opts := options{Model: "test:model", ToolSurface: config.ToolSurfaceMeta, Backend: backendMock, Output: path}
	runErr := errors.New("fixture validation failed\nmissing project fixture")

	if err := writeStartupReport(path, opts); err != nil {
		t.Fatalf("writeStartupReport() error = %v", err)
	}

	if err := writeErrorReport(path, opts, runErr); err != nil {
		t.Fatalf("writeErrorReport() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read error report: %v", err)
	}
	requireContainsAll(t, "error report", string(data), []string{
		"Status: `failed`",
		"The evaluator stopped before it could write the final metrics report.",
		"fixture validation failed",
		"missing project fixture",
	})
	if strings.Contains(string(data), "Status: `running`") {
		t.Fatalf("error report still contains startup placeholder content: %s", data)
	}
}

// TestWriteReportHeader_MetaTitle verifies meta reports keep the historical
// meta-tool title.
func TestWriteReportHeader_MetaTitle(t *testing.T) {
	var b strings.Builder
	writeReportHeader(&b, options{Model: "test:model", ToolSurface: config.ToolSurfaceMeta, Backend: backendMock, TerminalLog: "eval.log"}, false)
	requireContainsAll(t, "meta report header", b.String(), []string{
		"# Meta-Tool Model Evaluation",
		"Terminal output: `eval.log`",
	})
}

// TestWriteReportHeader_ResourceAccessState verifies that writeReportHeader
// emits the correct "Resource access" label for each of the three states the
// evaluator can produce: disabled, requested-but-not-active, and enabled.
//
// The test uses table-driven subtests to assert the exact label text in the
// rendered header for each option combination. This protects the report
// header from regressing to a single label that hides whether the run
// actually exposed MCP resources.
func TestWriteReportHeader_ResourceAccessState(t *testing.T) {
	tests := []struct {
		name string
		opts options
		want string
	}{
		{name: "disabled", opts: options{}, want: "Resource access: `disabled`"},
		{name: "requested", opts: options{ExposeResources: true}, want: "Resource access: `requested but not active`"},
		{name: "enabled", opts: options{ExposeResources: true, ResourceAccessActive: true}, want: "Resource access: `enabled`"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			writeReportHeader(&b, tt.opts, false)
			requireContainsAll(t, "resource access header", b.String(), []string{tt.want})
		})
	}
}

// TestWriteFailureDiagnostics_IncludesUnsafeDestructiveSuccess verifies safety
// misses remain visible even when a later repair completes the task.
func TestWriteFailureDiagnostics_IncludesUnsafeDestructiveSuccess(t *testing.T) {
	var b strings.Builder
	writeFailureDiagnostics(&b, options{ToolSurface: config.ToolSurfaceMeta}, []taskResult{{
		Task:            evalTask{ID: "MT-049"},
		FinalSuccess:    true,
		DestructiveSafe: false,
		Notes:           []string{"missing confirm:true"},
	}})
	requireContainsAll(t, "failure diagnostics", b.String(), []string{
		"## Failure Diagnostics",
		"| destructive_safety | 1 | MT-049 |",
	})
}

// TestWriteRepairDiagnostics_RecordsRecoveredCategory verifies successful
// repairs are summarized separately from final failures.
func TestWriteRepairDiagnostics_RecordsRecoveredCategory(t *testing.T) {
	var b strings.Builder
	writeRepairDiagnostics(&b, options{ToolSurface: config.ToolSurfaceMeta}, []taskResult{{
		Task:            evalTask{ID: "MT-012"},
		RepairAttempted: true,
		RepairSuccess:   true,
		FinalSuccess:    true,
		Notes:           []string{diagnosticMissingRequiredParams},
	}})
	requireContainsAll(t, "repair diagnostics", b.String(), []string{
		"## Repaired First-Pass Diagnostics",
		"| model_parameter_shape_miss | 1 | MT-012 |",
	})
}

// TestWriteRepairDiagnostics_IgnoresFailedFinalOutcome verifies repair
// diagnostics omit attempts whose final evaluation result still failed.
//
// The task is marked as repaired on the retry but unsuccessful overall. The
// expected output is empty so reports do not count unrecovered failures as
// successful repaired categories.
func TestWriteRepairDiagnostics_IgnoresFailedFinalOutcome(t *testing.T) {
	var b strings.Builder
	writeRepairDiagnostics(&b, options{ToolSurface: config.ToolSurfaceMeta}, []taskResult{{
		Task:            evalTask{ID: "MT-013"},
		RepairAttempted: true,
		RepairSuccess:   true,
		FinalSuccess:    false,
		Notes:           []string{diagnosticMissingRequiredParams},
	}})
	if b.Len() != 0 {
		t.Fatalf("writeRepairDiagnostics() wrote %q, want empty diagnostics", b.String())
	}
}
