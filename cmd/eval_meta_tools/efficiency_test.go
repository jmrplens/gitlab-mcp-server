package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

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

func hasEfficiencyGate(violations []efficiencyGateViolation, gate string) bool {
	return slices.ContainsFunc(violations, func(violation efficiencyGateViolation) bool {
		return violation.Gate == gate
	})
}
