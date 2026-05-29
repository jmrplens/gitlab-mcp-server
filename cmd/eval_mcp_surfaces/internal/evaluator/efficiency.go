package evaluator

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	efficiencyMaxTotalOverheadPercent = 20.0
	efficiencyMaxSingleStepAvgCalls   = 1.10
	efficiencyMaxExtraCallsPerAttempt = 4
)

type traceAttemptKey struct {
	TaskID string
	Model  string
}

type traceAttemptMetric struct {
	SourcePath      string
	TaskID          string
	Model           string
	Run             int
	ExpectedSteps   int
	ActualCalls     int
	ModelCalls      int
	ToolCalls       int
	SchemaLookup    bool
	RepairAttempted bool
	RepairSuccess   bool
	FinalSuccess    bool
}

type traceMetricSet struct {
	Overall    traceAggregate
	SingleStep traceAggregate
	ByModel    map[string]traceAggregate
	ByTask     map[string]traceAggregate
	ByKey      map[traceAttemptKey]traceAggregate
	Attempts   []traceAttemptMetric
}

type traceAggregate struct {
	Attempts        int
	ExpectedOps     int
	ActualCalls     int
	ModelCalls      int
	ToolCalls       int
	SchemaLookups   int
	RepairAttempts  int
	RepairSuccesses int
	FinalSuccesses  int
	FinalFailures   int
	MinCalls        int
	MaxCalls        int
}

type efficiencyGateViolation struct {
	Gate     string
	Observed string
	Limit    string
	Detail   string
}

type traceComparison struct {
	DynamicPath      string
	MetaPath         string
	ComparableRows   int
	DynamicOnlyRows  int
	MetaOnlyRows     int
	DynamicOnlyTasks []string
	MetaOnlyTasks    []string
	ByModel          map[string]traceComparisonAggregate
	ByTask           map[string]traceComparisonAggregate
}

type traceComparisonAggregate struct {
	Rows           int
	DynamicCalls   int
	MetaCalls      int
	PositiveExtra  int
	NetExtra       int
	DynamicGreater int
	DynamicEqual   int
}

func runEfficiencyCheck(opts options) error {
	paths := expandedStringList(opts.CheckEfficiency)
	if len(paths) == 0 {
		return errors.New("--check-efficiency requires at least one trace JSONL path")
	}
	allowedTasks := stringSet(expandedStringList(opts.EfficiencyAllowTask))
	metrics, err := readTraceMetricSet(paths)
	if err != nil {
		return err
	}
	if metrics.Overall.Attempts == 0 {
		return errors.New("efficiency check requires at least one trace attempt")
	}
	violations := efficiencyGateViolations(metrics, allowedTasks)
	report := buildEfficiencyCheckReport(paths, metrics, violations, allowedTasks)
	if writeErr := writeOptionalMarkdownReport(opts.Output, report, "efficiency check"); writeErr != nil {
		return writeErr
	}
	if len(violations) > 0 {
		return fmt.Errorf("efficiency check failed: %d violation(s)", len(violations))
	}
	return nil
}

func runTraceComparison(opts options) error {
	paths := expandedStringList(opts.CompareTraces)
	if len(paths) != 2 {
		return fmt.Errorf("--compare-traces requires exactly two trace JSONL paths: dynamic first, meta second; got %d", len(paths))
	}
	comparison, err := compareTraceMetricSets(paths[0], paths[1])
	if err != nil {
		return err
	}
	report := buildTraceComparisonReport(comparison)
	return writeOptionalMarkdownReport(opts.Output, report, "trace comparison")
}

func readTraceMetricSet(paths []string) (traceMetricSet, error) {
	metrics := traceMetricSet{
		ByModel:  map[string]traceAggregate{},
		ByTask:   map[string]traceAggregate{},
		ByKey:    map[traceAttemptKey]traceAggregate{},
		Attempts: make([]traceAttemptMetric, 0),
	}
	for _, path := range paths {
		traces, err := readTraceJSONL(path)
		if err != nil {
			return traceMetricSet{}, err
		}
		for _, trace := range traces {
			attempt := metricFromTrace(path, trace)
			metrics.addAttempt(attempt)
		}
	}
	return metrics, nil
}

func readTraceJSONL(path string) ([]taskTrace, error) {
	tracePath, err := traceJSONLPath(path)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(tracePath) // #nosec G304 -- explicit developer-provided trace artifact path.
	if err != nil {
		return nil, fmt.Errorf("read trace JSONL %s: %w", tracePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxPublishTraceLineBytes)
	traces := make([]taskTrace, 0)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var trace taskTrace
		if decodeErr := json.Unmarshal([]byte(line), &trace); decodeErr != nil {
			return nil, fmt.Errorf("decode trace JSONL %s line %d: %w", tracePath, lineNumber, decodeErr)
		}
		traces = append(traces, trace)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("scan trace JSONL %s: %w", tracePath, scanErr)
	}
	return traces, nil
}

func traceJSONLPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("trace path cannot be empty")
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		return "", fmt.Errorf("stat trace path %s: %w", trimmed, err)
	}
	if info.IsDir() {
		return filepath.Join(trimmed, "traces.jsonl"), nil
	}
	return trimmed, nil
}

func metricFromTrace(path string, trace taskTrace) traceAttemptMetric {
	return traceAttemptMetric{
		SourcePath:      path,
		TaskID:          trace.TaskID,
		Model:           traceModelLabel(trace),
		Run:             trace.Run,
		ExpectedSteps:   traceExpectedStepCount(trace),
		ActualCalls:     traceActualCallCount(trace),
		ModelCalls:      traceModelCallCount(trace),
		ToolCalls:       traceToolCallCount(trace),
		SchemaLookup:    trace.Summary.SchemaLookupUsed,
		RepairAttempted: trace.Summary.RepairAttempted,
		RepairSuccess:   trace.Summary.RepairSuccess,
		FinalSuccess:    trace.Summary.FinalSuccess,
	}
}

func traceModelLabel(trace taskTrace) string {
	model := strings.TrimSpace(trace.Model)
	if model == "" {
		return "default"
	}
	return model
}

func traceExpectedStepCount(trace taskTrace) int {
	if trace.Summary.ExpectedSteps > 0 {
		return trace.Summary.ExpectedSteps
	}
	return len(trace.Expected)
}

func traceModelCallCount(trace taskTrace) int {
	if trace.Summary.ModelCalls > 0 {
		return trace.Summary.ModelCalls
	}
	return countTraceEvents(trace, "assistant_message")
}

func traceToolCallCount(trace taskTrace) int {
	if trace.Summary.ToolCalls > 0 {
		return trace.Summary.ToolCalls
	}
	return countTraceEvents(trace, "tool_use")
}

func traceActualCallCount(trace taskTrace) int {
	toolCalls := traceToolCallCount(trace)
	if toolCalls > 0 {
		return toolCalls
	}
	return traceModelCallCount(trace)
}

func countTraceEvents(trace taskTrace, kind string) int {
	count := 0
	for _, event := range trace.Events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

func (metrics *traceMetricSet) addAttempt(attempt traceAttemptMetric) {
	metrics.Attempts = append(metrics.Attempts, attempt)
	metrics.Overall.addAttempt(attempt)
	if attempt.ExpectedSteps == 1 {
		metrics.SingleStep.addAttempt(attempt)
	}
	modelAggregate := metrics.ByModel[attempt.Model]
	modelAggregate.addAttempt(attempt)
	metrics.ByModel[attempt.Model] = modelAggregate

	taskAggregate := metrics.ByTask[attempt.TaskID]
	taskAggregate.addAttempt(attempt)
	metrics.ByTask[attempt.TaskID] = taskAggregate

	key := traceAttemptKey{TaskID: attempt.TaskID, Model: attempt.Model}
	keyAggregate := metrics.ByKey[key]
	keyAggregate.addAttempt(attempt)
	metrics.ByKey[key] = keyAggregate
}

func (aggregate *traceAggregate) addAttempt(attempt traceAttemptMetric) {
	aggregate.Attempts++
	aggregate.ExpectedOps += attempt.ExpectedSteps
	aggregate.ActualCalls += attempt.ActualCalls
	aggregate.ModelCalls += attempt.ModelCalls
	aggregate.ToolCalls += attempt.ToolCalls
	if attempt.SchemaLookup {
		aggregate.SchemaLookups++
	}
	if attempt.RepairAttempted {
		aggregate.RepairAttempts++
		if attempt.RepairSuccess {
			aggregate.RepairSuccesses++
		}
	}
	if attempt.FinalSuccess {
		aggregate.FinalSuccesses++
	} else {
		aggregate.FinalFailures++
	}
	if aggregate.Attempts == 1 || attempt.ActualCalls < aggregate.MinCalls {
		aggregate.MinCalls = attempt.ActualCalls
	}
	if attempt.ActualCalls > aggregate.MaxCalls {
		aggregate.MaxCalls = attempt.ActualCalls
	}
}

func traceExtraCalls(aggregate traceAggregate) int {
	return aggregate.ActualCalls - aggregate.ExpectedOps
}

func traceOverheadPercent(aggregate traceAggregate) float64 {
	if aggregate.ExpectedOps == 0 {
		return 0
	}
	return float64(traceExtraCalls(aggregate)) * 100 / float64(aggregate.ExpectedOps)
}

func traceAverageCalls(aggregate traceAggregate) float64 {
	if aggregate.Attempts == 0 {
		return 0
	}
	return float64(aggregate.ActualCalls) / float64(aggregate.Attempts)
}

func efficiencyGateViolations(metrics traceMetricSet, allowedTasks map[string]bool) []efficiencyGateViolation {
	var violations []efficiencyGateViolation
	if metrics.Overall.FinalSuccesses != metrics.Overall.Attempts {
		violations = append(violations, efficiencyGateViolation{
			Gate:     "final_success",
			Observed: fmt.Sprintf("%d/%d", metrics.Overall.FinalSuccesses, metrics.Overall.Attempts),
			Limit:    "100.0%",
			Detail:   "all trace attempts must finish successfully",
		})
	}
	if metrics.SingleStep.Attempts > 0 && traceAverageCalls(metrics.SingleStep) > efficiencyMaxSingleStepAvgCalls {
		violations = append(violations, efficiencyGateViolation{
			Gate:     "single_step_average_calls",
			Observed: fmt.Sprintf("%.2f", traceAverageCalls(metrics.SingleStep)),
			Limit:    fmt.Sprintf("<= %.2f", efficiencyMaxSingleStepAvgCalls),
			Detail:   "single-step tasks should normally execute directly",
		})
	}
	if traceOverheadPercent(metrics.Overall) > efficiencyMaxTotalOverheadPercent {
		violations = append(violations, efficiencyGateViolation{
			Gate:     "total_overhead",
			Observed: formatMetric(traceOverheadPercent(metrics.Overall)),
			Limit:    "<= " + formatMetric(efficiencyMaxTotalOverheadPercent),
			Detail:   "total calls should stay close to the expected-operation baseline",
		})
	}
	for _, attempt := range metrics.Attempts {
		if allowedTasks[attempt.TaskID] {
			continue
		}
		maxCalls := attempt.ExpectedSteps + efficiencyMaxExtraCallsPerAttempt
		if attempt.ActualCalls <= maxCalls {
			continue
		}
		violations = append(violations, efficiencyGateViolation{
			Gate:     "per_attempt_call_budget",
			Observed: fmt.Sprintf("%d calls", attempt.ActualCalls),
			Limit:    fmt.Sprintf("<= %d calls", maxCalls),
			Detail:   fmt.Sprintf("%s/%s run %d exceeded expected steps + %d", attempt.TaskID, attempt.Model, attempt.Run, efficiencyMaxExtraCallsPerAttempt),
		})
	}
	return violations
}

func buildEfficiencyCheckReport(paths []string, metrics traceMetricSet, violations []efficiencyGateViolation, allowedTasks map[string]bool) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Trace Efficiency Check\n\n")
	fmt.Fprintf(&builder, "Date: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&builder, "Status: `%s`\n\n", efficiencyStatus(violations))
	fmt.Fprintf(&builder, "## Inputs\n\n")
	fmt.Fprintf(&builder, "| Trace JSONL |\n| --- |\n")
	for _, path := range paths {
		fmt.Fprintf(&builder, "| `%s` |\n", escapeTable(path))
	}
	if len(allowedTasks) > 0 {
		fmt.Fprintf(&builder, "\nAllowed over-budget tasks: `%s`\n", strings.Join(sortedBoolKeys(allowedTasks), "`, `"))
	}

	fmt.Fprintf(&builder, "\n## Summary\n\n")
	fmt.Fprintf(&builder, "| Metric | Value |\n| --- | ---: |\n")
	fmt.Fprintf(&builder, "| Attempts | %d |\n", metrics.Overall.Attempts)
	fmt.Fprintf(&builder, "| Expected operations | %d |\n", metrics.Overall.ExpectedOps)
	fmt.Fprintf(&builder, "| Actual calls | %d |\n", metrics.Overall.ActualCalls)
	fmt.Fprintf(&builder, "| Extra calls | %d |\n", traceExtraCalls(metrics.Overall))
	fmt.Fprintf(&builder, "| Total overhead | %s |\n", formatMetric(traceOverheadPercent(metrics.Overall)))
	fmt.Fprintf(&builder, "| Single-step average calls | %.2f |\n", traceAverageCalls(metrics.SingleStep))
	fmt.Fprintf(&builder, "| Final success | %s (%d/%d) |\n", formatMetric(percent(metrics.Overall.FinalSuccesses, metrics.Overall.Attempts)), metrics.Overall.FinalSuccesses, metrics.Overall.Attempts)
	fmt.Fprintf(&builder, "| Schema/search attempts | %d |\n", metrics.Overall.SchemaLookups)
	fmt.Fprintf(&builder, "| Repair attempts | %d |\n", metrics.Overall.RepairAttempts)
	fmt.Fprintf(&builder, "| Repair successes | %d |\n", metrics.Overall.RepairSuccesses)

	fmt.Fprintf(&builder, "\n## Gates\n\n")
	fmt.Fprintf(&builder, "| Gate | Limit | Observed | Status | Detail |\n| --- | ---: | ---: | --- | --- |\n")
	writeEfficiencyGateRows(&builder, metrics, violations)

	if len(violations) > 0 {
		fmt.Fprintf(&builder, "\n## Violations\n\n")
		fmt.Fprintf(&builder, "| Gate | Observed | Limit | Detail |\n| --- | ---: | ---: | --- |\n")
		for _, violation := range violations {
			fmt.Fprintf(&builder, "| `%s` | %s | %s | %s |\n", violation.Gate, escapeTable(violation.Observed), escapeTable(violation.Limit), escapeTable(violation.Detail))
		}
	}

	writeEfficiencyByModel(&builder, metrics)
	writeEfficiencyTaskOutliers(&builder, metrics)
	return builder.String()
}

func efficiencyStatus(violations []efficiencyGateViolation) string {
	if len(violations) == 0 {
		return "pass"
	}
	return "fail"
}

func writeEfficiencyGateRows(builder *strings.Builder, metrics traceMetricSet, violations []efficiencyGateViolation) {
	failed := map[string]bool{}
	for _, violation := range violations {
		failed[violation.Gate] = true
	}
	rows := []struct {
		Gate     string
		Limit    string
		Observed string
		Detail   string
	}{
		{"final_success", "100.0%", fmt.Sprintf("%s (%d/%d)", formatMetric(percent(metrics.Overall.FinalSuccesses, metrics.Overall.Attempts)), metrics.Overall.FinalSuccesses, metrics.Overall.Attempts), "all attempts pass"},
		{"single_step_average_calls", fmt.Sprintf("<= %.2f", efficiencyMaxSingleStepAvgCalls), fmt.Sprintf("%.2f", traceAverageCalls(metrics.SingleStep)), "single-step directness"},
		{"total_overhead", "<= " + formatMetric(efficiencyMaxTotalOverheadPercent), formatMetric(traceOverheadPercent(metrics.Overall)), "actual calls versus expected operations"},
		{"per_attempt_call_budget", fmt.Sprintf("expected steps + %d", efficiencyMaxExtraCallsPerAttempt), "per attempt", "no unallowlisted attempts above budget"},
	}
	for _, row := range rows {
		status := "pass"
		if failed[row.Gate] {
			status = "fail"
		}
		fmt.Fprintf(builder, "| `%s` | %s | %s | `%s` | %s |\n", row.Gate, escapeTable(row.Limit), escapeTable(row.Observed), status, escapeTable(row.Detail))
	}
}

func writeEfficiencyByModel(builder *strings.Builder, metrics traceMetricSet) {
	if len(metrics.ByModel) == 0 {
		return
	}
	fmt.Fprintf(builder, "\n## By Model\n\n")
	fmt.Fprintf(builder, "| Model | Attempts | Expected ops | Calls | Extra | Overhead | Final failures | Schema/search | Repairs |\n")
	fmt.Fprintf(builder, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, model := range sortedStringKeys(metrics.ByModel) {
		aggregate := metrics.ByModel[model]
		fmt.Fprintf(builder, "| `%s` | %d | %d | %d | %d | %s | %d | %d | %d |\n",
			escapeTable(model), aggregate.Attempts, aggregate.ExpectedOps, aggregate.ActualCalls, traceExtraCalls(aggregate), formatMetric(traceOverheadPercent(aggregate)), aggregate.FinalFailures, aggregate.SchemaLookups, aggregate.RepairAttempts)
	}
}

func writeEfficiencyTaskOutliers(builder *strings.Builder, metrics traceMetricSet) {
	tasks := sortedTaskOutliers(metrics.ByTask)
	if len(tasks) == 0 {
		return
	}
	limit := min(20, len(tasks))
	fmt.Fprintf(builder, "\n## Task Call Outliers\n\n")
	fmt.Fprintf(builder, "| Task | Attempts | Expected ops | Calls | Extra | Min | Max | Spread | Final failures | Schema/search |\n")
	fmt.Fprintf(builder, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, taskID := range tasks[:limit] {
		aggregate := metrics.ByTask[taskID]
		fmt.Fprintf(builder, "| %s | %d | %d | %d | %d | %d | %d | %d | %d | %d |\n",
			taskID, aggregate.Attempts, aggregate.ExpectedOps, aggregate.ActualCalls, traceExtraCalls(aggregate), aggregate.MinCalls, aggregate.MaxCalls, aggregate.MaxCalls-aggregate.MinCalls, aggregate.FinalFailures, aggregate.SchemaLookups)
	}
}

func sortedTaskOutliers(tasks map[string]traceAggregate) []string {
	keys := sortedStringKeys(tasks)
	sort.SliceStable(keys, func(left, right int) bool {
		leftAggregate := tasks[keys[left]]
		rightAggregate := tasks[keys[right]]
		leftSpread := leftAggregate.MaxCalls - leftAggregate.MinCalls
		rightSpread := rightAggregate.MaxCalls - rightAggregate.MinCalls
		if leftAggregate.FinalFailures != rightAggregate.FinalFailures {
			return leftAggregate.FinalFailures > rightAggregate.FinalFailures
		}
		if leftSpread != rightSpread {
			return leftSpread > rightSpread
		}
		if traceExtraCalls(leftAggregate) != traceExtraCalls(rightAggregate) {
			return traceExtraCalls(leftAggregate) > traceExtraCalls(rightAggregate)
		}
		return keys[left] < keys[right]
	})
	return keys
}

func compareTraceMetricSets(dynamicPath, metaPath string) (traceComparison, error) {
	dynamicMetrics, err := readTraceMetricSet([]string{dynamicPath})
	if err != nil {
		return traceComparison{}, err
	}
	metaMetrics, err := readTraceMetricSet([]string{metaPath})
	if err != nil {
		return traceComparison{}, err
	}
	comparison := traceComparison{
		DynamicPath: dynamicPath,
		MetaPath:    metaPath,
		ByModel:     map[string]traceComparisonAggregate{},
		ByTask:      map[string]traceComparisonAggregate{},
	}
	dynamicOnlyTasks := map[string]bool{}
	metaOnlyTasks := map[string]bool{}
	for key, dynamicAggregate := range dynamicMetrics.ByKey {
		metaAggregate, ok := metaMetrics.ByKey[key]
		if !ok {
			comparison.DynamicOnlyRows += dynamicAggregate.Attempts
			dynamicOnlyTasks[key.TaskID] = true
			continue
		}
		comparison.addComparable(key, dynamicAggregate, metaAggregate)
	}
	for key, metaAggregate := range metaMetrics.ByKey {
		if _, ok := dynamicMetrics.ByKey[key]; ok {
			continue
		}
		comparison.MetaOnlyRows += metaAggregate.Attempts
		metaOnlyTasks[key.TaskID] = true
	}
	comparison.DynamicOnlyTasks = sortedBoolKeys(dynamicOnlyTasks)
	comparison.MetaOnlyTasks = sortedBoolKeys(metaOnlyTasks)
	return comparison, nil
}

func (comparison *traceComparison) addComparable(key traceAttemptKey, dynamicAggregate, metaAggregate traceAggregate) {
	rows := min(dynamicAggregate.Attempts, metaAggregate.Attempts)
	if rows == 0 {
		return
	}
	comparison.ComparableRows += rows
	modelAggregate := comparison.ByModel[key.Model]
	modelAggregate.addComparable(dynamicAggregate, metaAggregate, rows)
	comparison.ByModel[key.Model] = modelAggregate

	taskAggregate := comparison.ByTask[key.TaskID]
	taskAggregate.addComparable(dynamicAggregate, metaAggregate, rows)
	comparison.ByTask[key.TaskID] = taskAggregate
}

func (aggregate *traceComparisonAggregate) addComparable(dynamicAggregate, metaAggregate traceAggregate, rows int) {
	dynamicCalls := scaledComparableCalls(dynamicAggregate.ActualCalls, dynamicAggregate.Attempts, rows)
	metaCalls := scaledComparableCalls(metaAggregate.ActualCalls, metaAggregate.Attempts, rows)
	netExtra := dynamicCalls - metaCalls

	aggregate.Rows += rows
	aggregate.DynamicCalls += dynamicCalls
	aggregate.MetaCalls += metaCalls
	aggregate.NetExtra += netExtra
	if netExtra > 0 {
		aggregate.PositiveExtra += netExtra
	}
	if dynamicCalls > metaCalls {
		aggregate.DynamicGreater++
	}
	if dynamicCalls == metaCalls {
		aggregate.DynamicEqual++
	}
}

func scaledComparableCalls(totalCalls, attempts, rows int) int {
	if totalCalls == 0 || attempts <= 0 || rows <= 0 {
		return 0
	}
	return (totalCalls*rows + attempts/2) / attempts
}

func buildTraceComparisonReport(comparison traceComparison) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Trace Surface Comparison\n\n")
	fmt.Fprintf(&builder, "Date: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&builder, "Dynamic trace: `%s`\n", escapeTable(comparison.DynamicPath))
	fmt.Fprintf(&builder, "Meta trace: `%s`\n\n", escapeTable(comparison.MetaPath))
	fmt.Fprintf(&builder, "## Summary\n\n")
	fmt.Fprintf(&builder, "| Metric | Value |\n| --- | ---: |\n")
	fmt.Fprintf(&builder, "| Comparable rows | %d |\n", comparison.ComparableRows)
	fmt.Fprintf(&builder, "| Dynamic-only rows | %d |\n", comparison.DynamicOnlyRows)
	fmt.Fprintf(&builder, "| Meta-only rows | %d |\n", comparison.MetaOnlyRows)
	if len(comparison.DynamicOnlyTasks) > 0 {
		fmt.Fprintf(&builder, "| Dynamic-only tasks | `%s` |\n", strings.Join(comparison.DynamicOnlyTasks, "`, `"))
	}
	if len(comparison.MetaOnlyTasks) > 0 {
		fmt.Fprintf(&builder, "| Meta-only tasks | `%s` |\n", strings.Join(comparison.MetaOnlyTasks, "`, `"))
	}

	fmt.Fprintf(&builder, "\n## By Model\n\n")
	fmt.Fprintf(&builder, "| Model | Comparable rows | Dynamic calls | Meta calls | Net extra dynamic calls | Dynamic > meta rows | Dynamic = meta rows |\n")
	fmt.Fprintf(&builder, "| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, model := range sortedStringKeys(comparison.ByModel) {
		aggregate := comparison.ByModel[model]
		fmt.Fprintf(&builder, "| `%s` | %d | %d | %d | %d | %d | %d |\n",
			escapeTable(model), aggregate.Rows, aggregate.DynamicCalls, aggregate.MetaCalls, aggregate.NetExtra, aggregate.DynamicGreater, aggregate.DynamicEqual)
	}

	tasks := sortedComparisonTasks(comparison.ByTask)
	if len(tasks) > 0 {
		fmt.Fprintf(&builder, "\n## Largest Dynamic Overheads\n\n")
		fmt.Fprintf(&builder, "| Task | Dynamic calls | Meta calls | Positive overhead | Net overhead |\n")
		fmt.Fprintf(&builder, "| --- | ---: | ---: | ---: | ---: |\n")
		for _, taskID := range tasks[:min(30, len(tasks))] {
			aggregate := comparison.ByTask[taskID]
			fmt.Fprintf(&builder, "| %s | %d | %d | %d | %d |\n", taskID, aggregate.DynamicCalls, aggregate.MetaCalls, aggregate.PositiveExtra, aggregate.NetExtra)
		}
	}
	return builder.String()
}

func sortedComparisonTasks(tasks map[string]traceComparisonAggregate) []string {
	keys := sortedStringKeys(tasks)
	sort.SliceStable(keys, func(left, right int) bool {
		leftAggregate := tasks[keys[left]]
		rightAggregate := tasks[keys[right]]
		if leftAggregate.PositiveExtra != rightAggregate.PositiveExtra {
			return leftAggregate.PositiveExtra > rightAggregate.PositiveExtra
		}
		if leftAggregate.NetExtra != rightAggregate.NetExtra {
			return leftAggregate.NetExtra > rightAggregate.NetExtra
		}
		return keys[left] < keys[right]
	})
	return keys
}

func writeOptionalMarkdownReport(path, content, label string) error {
	if strings.TrimSpace(path) == "" {
		terminalPrint(content)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create %s report directory: %w", label, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s report: %w", label, err)
	}
	terminalPrintf("wrote %s report: %s\n", label, path)
	return nil
}

func expandedStringList(values stringList) []string {
	var expanded []string
	for _, value := range values {
		for part := range strings.SplitSeq(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				expanded = append(expanded, part)
			}
		}
	}
	return expanded
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func sortedBoolKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
