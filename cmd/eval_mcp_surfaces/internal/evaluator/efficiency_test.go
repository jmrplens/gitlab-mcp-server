package evaluator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestReadTraceMetricSet_SyntheticTraces_ComputesSummaryMetrics verifies ReadTraceMetricSet computes summary metrics with synthetic traces.
func TestReadTraceMetricSet_SyntheticTraces_ComputesSummaryMetrics(t *testing.T) {
	tracePath := writeSyntheticTraceJSONL(t, []taskTrace{
		syntheticTrace("MT-001", "model-a", 1, 1, 1, traceSummary{FinalSuccess: true}),
		syntheticTrace("MT-002", "model-a", 1, 1, 3, traceSummary{SchemaLookupUsed: true, FinalSuccess: true}),
		syntheticTrace("MS-001", "model-b", 1, 2, 4, traceSummary{RepairAttempted: true, RepairSuccess: true, FinalSuccess: true}),
		syntheticTrace("MT-003", "model-b", 1, 1, 2, traceSummary{RepairAttempted: true, FinalSuccess: false}),
	})

	metrics, err := readTraceMetricSet([]string{tracePath})
	if err != nil {
		t.Fatalf("readTraceMetricSet() error = %v", err)
	}
	if metrics.Overall.Attempts != 4 || metrics.Overall.ExpectedOps != 5 || metrics.Overall.ActualCalls != 10 {
		t.Fatalf("overall metrics = %+v, want attempts=4 expected=5 calls=10", metrics.Overall)
	}
	if metrics.Overall.SchemaLookups != 1 || metrics.Overall.RepairAttempts != 2 || metrics.Overall.RepairSuccesses != 1 {
		t.Fatalf("lookup/repair metrics = %+v, want schema=1 repairs=2 successes=1", metrics.Overall)
	}
	if metrics.Overall.FinalSuccesses != 3 || metrics.Overall.FinalFailures != 1 {
		t.Fatalf("final metrics = %+v, want successes=3 failures=1", metrics.Overall)
	}
	if metrics.SingleStep.Attempts != 3 || metrics.SingleStep.ActualCalls != 6 {
		t.Fatalf("single-step metrics = %+v, want attempts=3 calls=6", metrics.SingleStep)
	}
	modelA := metrics.ByModel["model-a"]
	if modelA.Attempts != 2 || modelA.ExpectedOps != 2 || modelA.ActualCalls != 4 || traceExtraCalls(modelA) != 2 {
		t.Fatalf("model-a metrics = %+v, want attempts=2 expected=2 calls=4 extra=2", modelA)
	}
}

// TestEfficiencyGateViolations_OverBudgetTrace_ReportsAndAllowsTask verifies EfficiencyGateViolations reports and allows task with over budget trace.
func TestEfficiencyGateViolations_OverBudgetTrace_ReportsAndAllowsTask(t *testing.T) {
	tracePath := writeSyntheticTraceJSONL(t, []taskTrace{
		syntheticTrace("MT-066", "model-a", 1, 1, 6, traceSummary{FinalSuccess: true}),
	})
	metrics, err := readTraceMetricSet([]string{tracePath})
	if err != nil {
		t.Fatalf("readTraceMetricSet() error = %v", err)
	}

	violations := efficiencyGateViolations(metrics, nil)
	if !hasEfficiencyGate(violations, "single_step_average_calls") || !hasEfficiencyGate(violations, "total_overhead") || !hasEfficiencyGate(violations, "per_attempt_call_budget") {
		t.Fatalf("violations = %+v, want single-step, overhead, and call-budget gates", violations)
	}

	allowed := efficiencyGateViolations(metrics, map[string]bool{"MT-066": true})
	if hasEfficiencyGate(allowed, "per_attempt_call_budget") {
		t.Fatalf("allowed violations = %+v, want per-attempt budget suppressed", allowed)
	}
}

// TestRunEfficiencyCheck_EmptyTraceFails verifies RunEfficiencyCheck when empty trace fails.
func TestRunEfficiencyCheck_EmptyTraceFails(t *testing.T) {
	tracePath := writeSyntheticTraceJSONL(t, nil)

	err := runEfficiencyCheck(options{CheckEfficiency: stringList{tracePath}, Output: filepath.Join(t.TempDir(), "efficiency.md")})
	if err == nil || !strings.Contains(err.Error(), "requires at least one trace attempt") {
		t.Fatalf("runEfficiencyCheck() error = %v, want empty trace failure", err)
	}
}

// TestRunEfficiencyCheck_WritesPassingReport verifies RunEfficiencyCheck writes passing report.
func TestRunEfficiencyCheck_WritesPassingReport(t *testing.T) {
	tracePath := writeSyntheticTraceJSONL(t, []taskTrace{
		syntheticTrace("MT-001", "model-a", 1, 1, 1, traceSummary{FinalSuccess: true}),
	})
	outputPath := filepath.Join(t.TempDir(), "efficiency.md")

	if err := runEfficiencyCheck(options{CheckEfficiency: stringList{tracePath}, Output: outputPath}); err != nil {
		t.Fatalf("runEfficiencyCheck() error = %v", err)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read efficiency report: %v", err)
	}
	text := string(content)
	for _, want := range []string{"# Trace Efficiency Check", "Status: `pass`", "| Attempts | 1 |", "## By Model"} {
		if !strings.Contains(text, want) {
			t.Fatalf("efficiency report = %q, want %q", text, want)
		}
	}
}

// TestRunEfficiencyCheck_WritesViolationReportAndFails verifies RunEfficiencyCheck writes violation report and fails.
func TestRunEfficiencyCheck_WritesViolationReportAndFails(t *testing.T) {
	tracePath := writeSyntheticTraceJSONL(t, []taskTrace{
		syntheticTrace("MT-066", "model-a", 1, 1, 6, traceSummary{FinalSuccess: true}),
	})
	outputPath := filepath.Join(t.TempDir(), "efficiency.md")

	err := runEfficiencyCheck(options{CheckEfficiency: stringList{tracePath}, Output: outputPath})
	if err == nil || !strings.Contains(err.Error(), "efficiency check failed") {
		t.Fatalf("runEfficiencyCheck() error = %v, want gate failure", err)
	}
	content, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		t.Fatalf("read efficiency report: %v", readErr)
	}
	text := string(content)
	for _, want := range []string{"Status: `fail`", "## Violations", "per_attempt_call_budget"} {
		if !strings.Contains(text, want) {
			t.Fatalf("efficiency report = %q, want %q", text, want)
		}
	}
}

// TestTraceJSONLPath_DirectoryAndInvalidInputs verifies TraceJSONLPath when directory and invalid inputs.
func TestTraceJSONLPath_DirectoryAndInvalidInputs(t *testing.T) {
	dir := t.TempDir()
	got, err := traceJSONLPath(dir)
	if err != nil {
		t.Fatalf("traceJSONLPath(dir) error = %v", err)
	}
	if got != filepath.Join(dir, "traces.jsonl") {
		t.Fatalf("traceJSONLPath(dir) = %q, want traces.jsonl in dir", got)
	}
	if _, emptyErr := traceJSONLPath(" "); emptyErr == nil {
		t.Fatal("traceJSONLPath(empty) error = nil, want error")
	}
	if _, missingErr := readTraceJSONL(filepath.Join(dir, "missing.jsonl")); missingErr == nil {
		t.Fatal("readTraceJSONL(missing) error = nil, want stat/open error")
	}
	invalidPath := filepath.Join(dir, "invalid.jsonl")
	if writeErr := os.WriteFile(invalidPath, []byte("{bad-json}\n"), 0o600); writeErr != nil {
		t.Fatalf("write invalid trace: %v", writeErr)
	}
	if _, decodeErr := readTraceJSONL(invalidPath); decodeErr == nil {
		t.Fatal("readTraceJSONL(invalid) error = nil, want decode error")
	}
}

// TestCompareTraceMetricSets_OverlappingRows_ReportsComparableAndExclusiveRows verifies CompareTraceMetricSets reports comparable and exclusive rows with overlapping rows.
func TestCompareTraceMetricSets_OverlappingRows_ReportsComparableAndExclusiveRows(t *testing.T) {
	dynamicPath := writeSyntheticTraceJSONL(t, []taskTrace{
		syntheticTrace("MT-001", "model-a", 1, 1, 3, traceSummary{FinalSuccess: true}),
		syntheticTrace("MT-002", "model-a", 1, 1, 1, traceSummary{FinalSuccess: true}),
		syntheticTrace("MT-DYNAMIC", "model-a", 1, 1, 1, traceSummary{FinalSuccess: true}),
	})
	metaPath := writeSyntheticTraceJSONL(t, []taskTrace{
		syntheticTrace("MT-001", "model-a", 1, 1, 1, traceSummary{FinalSuccess: true}),
		syntheticTrace("MT-002", "model-a", 1, 1, 1, traceSummary{FinalSuccess: true}),
		syntheticTrace("MT-META", "model-a", 1, 1, 1, traceSummary{FinalSuccess: true}),
	})

	comparison, err := compareTraceMetricSets(dynamicPath, metaPath)
	if err != nil {
		t.Fatalf("compareTraceMetricSets() error = %v", err)
	}
	if comparison.ComparableRows != 2 || comparison.DynamicOnlyRows != 1 || comparison.MetaOnlyRows != 1 {
		t.Fatalf("comparison counts = %+v, want comparable=2 dynamicOnly=1 metaOnly=1", comparison)
	}
	if !slices.Equal(comparison.DynamicOnlyTasks, []string{"MT-DYNAMIC"}) || !slices.Equal(comparison.MetaOnlyTasks, []string{"MT-META"}) {
		t.Fatalf("exclusive tasks = dynamic %+v meta %+v", comparison.DynamicOnlyTasks, comparison.MetaOnlyTasks)
	}
	modelAggregate := comparison.ByModel["model-a"]
	if modelAggregate.DynamicCalls != 4 || modelAggregate.MetaCalls != 2 || modelAggregate.NetExtra != 2 || modelAggregate.DynamicGreater != 1 || modelAggregate.DynamicEqual != 1 {
		t.Fatalf("model aggregate = %+v, want dynamic=4 meta=2 net=2 greater=1 equal=1", modelAggregate)
	}
}

// TestCompareTraceMetricSets_ScalesCallsToComparableRows verifies CompareTraceMetricSets scales calls to comparable rows.
func TestCompareTraceMetricSets_ScalesCallsToComparableRows(t *testing.T) {
	dynamicPath := writeSyntheticTraceJSONL(t, []taskTrace{
		syntheticTrace("MT-001", "model-a", 1, 1, 4, traceSummary{FinalSuccess: true}),
		syntheticTrace("MT-001", "model-a", 2, 1, 6, traceSummary{FinalSuccess: true}),
	})
	metaPath := writeSyntheticTraceJSONL(t, []taskTrace{
		syntheticTrace("MT-001", "model-a", 1, 1, 3, traceSummary{FinalSuccess: true}),
	})

	comparison, err := compareTraceMetricSets(dynamicPath, metaPath)
	if err != nil {
		t.Fatalf("compareTraceMetricSets() error = %v", err)
	}
	modelAggregate := comparison.ByModel["model-a"]
	if modelAggregate.Rows != 1 || modelAggregate.DynamicCalls != 5 || modelAggregate.MetaCalls != 3 || modelAggregate.NetExtra != 2 {
		t.Fatalf("model aggregate = %+v, want rows=1 dynamic=5 meta=3 net=2", modelAggregate)
	}
}

// TestRunTraceComparison_WritesReport verifies RunTraceComparison writes report.
func TestRunTraceComparison_WritesReport(t *testing.T) {
	dynamicPath := writeSyntheticTraceJSONL(t, []taskTrace{
		syntheticTrace("MT-001", "model-a", 1, 1, 2, traceSummary{FinalSuccess: true}),
	})
	metaPath := writeSyntheticTraceJSONL(t, []taskTrace{
		syntheticTrace("MT-001", "model-a", 1, 1, 1, traceSummary{FinalSuccess: true}),
	})
	outputPath := filepath.Join(t.TempDir(), "comparison.md")

	if err := runTraceComparison(options{CompareTraces: stringList{dynamicPath, metaPath}, Output: outputPath}); err != nil {
		t.Fatalf("runTraceComparison() error = %v", err)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read comparison report: %v", err)
	}
	for _, want := range []string{"# Trace Surface Comparison", "| Comparable rows | 1 |", "## Largest Dynamic Overheads"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("comparison report = %q, want %q", string(content), want)
		}
	}
}

// TestRunTraceComparison_RequiresTwoPaths verifies RunTraceComparison requires two paths.
func TestRunTraceComparison_RequiresTwoPaths(t *testing.T) {
	err := runTraceComparison(options{CompareTraces: stringList{"one.jsonl"}})
	if err == nil || !strings.Contains(err.Error(), "requires exactly two trace JSONL paths") {
		t.Fatalf("runTraceComparison() error = %v, want arity failure", err)
	}
}

// TestEfficiencyOrderingHelpers_SortByHighestRisk verifies EfficiencyOrderingHelpers when sort by highest risk.
func TestEfficiencyOrderingHelpers_SortByHighestRisk(t *testing.T) {
	tasks := map[string]traceAggregate{
		"MT-A": {Attempts: 1, ExpectedOps: 1, ActualCalls: 4, MinCalls: 4, MaxCalls: 4},
		"MT-B": {Attempts: 1, ExpectedOps: 1, ActualCalls: 3, MinCalls: 1, MaxCalls: 3},
		"MT-C": {Attempts: 1, ExpectedOps: 1, ActualCalls: 2, MinCalls: 2, MaxCalls: 2, FinalFailures: 1},
	}
	if got := sortedTaskOutliers(tasks); !slices.Equal(got, []string{"MT-C", "MT-B", "MT-A"}) {
		t.Fatalf("sortedTaskOutliers() = %+v, want failure, spread, extra order", got)
	}

	comparisonTasks := map[string]traceComparisonAggregate{
		"MT-A": {PositiveExtra: 2, NetExtra: 5},
		"MT-B": {PositiveExtra: 3, NetExtra: 1},
		"MT-C": {PositiveExtra: 2, NetExtra: 7},
	}
	if got := sortedComparisonTasks(comparisonTasks); !slices.Equal(got, []string{"MT-B", "MT-C", "MT-A"}) {
		t.Fatalf("sortedComparisonTasks() = %+v, want positive then net order", got)
	}
}

// TestEfficiencyScalarHelpers_HandleZeroValues verifies EfficiencyScalarHelpers when handle zero values.
func TestEfficiencyScalarHelpers_HandleZeroValues(t *testing.T) {
	if got := traceOverheadPercent(traceAggregate{}); got != 0 {
		t.Fatalf("traceOverheadPercent(zero) = %.1f, want 0", got)
	}
	if got := traceAverageCalls(traceAggregate{}); got != 0 {
		t.Fatalf("traceAverageCalls(zero) = %.1f, want 0", got)
	}
	if got := scaledComparableCalls(10, 0, 1); got != 0 {
		t.Fatalf("scaledComparableCalls(zero attempts) = %d, want 0", got)
	}
	if got := scaledComparableCalls(5, 2, 1); got != 3 {
		t.Fatalf("scaledComparableCalls(round) = %d, want 3", got)
	}
	if got := stringSet([]string{"MT-001", "", "MT-002"}); len(got) != 2 || !got["MT-001"] || !got["MT-002"] {
		t.Fatalf("stringSet() = %+v, want non-empty values only", got)
	}
}

// TestMetricFromTrace_FallsBackToEventCounts verifies MetricFromTrace falls back to event counts.
func TestMetricFromTrace_FallsBackToEventCounts(t *testing.T) {
	trace := taskTrace{
		TaskID:   "MT-001",
		Expected: []traceExpectedStep{{Step: 1}, {Step: 2}},
		Events: []traceEvent{
			{Kind: "assistant_message"},
			{Kind: "assistant_message"},
			{Kind: "tool_use"},
			{Kind: "tool_use"},
			{Kind: "tool_result"},
		},
	}

	metric := metricFromTrace("trace.jsonl", trace)
	if metric.Model != "default" || metric.ExpectedSteps != 2 || metric.ModelCalls != 2 || metric.ToolCalls != 2 || metric.ActualCalls != 2 {
		t.Fatalf("metricFromTrace() = %+v, want default model and event-derived counts", metric)
	}
}

// TestWriteOptionalMarkdownReport_InvalidDirectory verifies WriteOptionalMarkdownReport when invalid directory.
func TestWriteOptionalMarkdownReport_InvalidDirectory(t *testing.T) {
	err := writeOptionalMarkdownReport(string([]byte{0}), "content", "test")
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("writeOptionalMarkdownReport() error = %v, want invalid path", err)
	}
}

// syntheticTrace supports synthetic trace assertions in main tests.
func syntheticTrace(taskID, model string, run, expectedSteps, calls int, summary traceSummary) taskTrace {
	expected := make([]traceExpectedStep, expectedSteps)
	for stepIndex := range expected {
		expected[stepIndex] = traceExpectedStep{Step: stepIndex + 1, Tool: "gitlab", Action: "project.get"}
	}
	summary.ExpectedSteps = expectedSteps
	summary.ModelCalls = calls
	summary.ToolCalls = calls
	return taskTrace{
		Run:      run,
		Model:    model,
		TaskID:   taskID,
		Expected: expected,
		Summary:  summary,
	}
}

// writeSyntheticTraceJSONL writes synthetic trace jsonl fixture data for tests.
func writeSyntheticTraceJSONL(t *testing.T, traces []taskTrace) string {
	t.Helper()
	tracePath := filepath.Join(t.TempDir(), "traces.jsonl")
	file, err := os.Create(tracePath)
	if err != nil {
		t.Fatalf("create trace JSONL: %v", err)
	}
	encoder := json.NewEncoder(file)
	for _, trace := range traces {
		if encodeErr := encoder.Encode(trace); encodeErr != nil {
			_ = file.Close()
			t.Fatalf("encode trace: %v", encodeErr)
		}
	}
	if closeErr := file.Close(); closeErr != nil {
		t.Fatalf("close trace JSONL: %v", closeErr)
	}
	return tracePath
}

// hasEfficiencyGate reports whether has efficiency gate.
func hasEfficiencyGate(violations []efficiencyGateViolation, gate string) bool {
	return slices.ContainsFunc(violations, func(violation efficiencyGateViolation) bool {
		return violation.Gate == gate
	})
}
