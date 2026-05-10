package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
)

const (
	defaultPublishResultsDoc = "docs/testing/model-results.md"
	defaultPublishReadme     = "README.md"

	publishModeAppend         = "append"
	publishModeReplaceCurrent = "replace-current"

	modelEvalMetaSummaryStart     = "<!-- START MODEL EVAL META SUMMARY -->"
	modelEvalMetaSummaryEnd       = "<!-- END MODEL EVAL META SUMMARY -->"
	modelEvalDynamic3SummaryStart = "<!-- START MODEL EVAL DYNAMIC3 SUMMARY -->"
	modelEvalDynamic3SummaryEnd   = "<!-- END MODEL EVAL DYNAMIC3 SUMMARY -->"
	modelEvalMetaResultsStart     = "<!-- START MODEL EVAL META RESULTS -->"
	modelEvalMetaResultsEnd       = "<!-- END MODEL EVAL META RESULTS -->"
	modelEvalDynamic3ResultsStart = "<!-- START MODEL EVAL DYNAMIC3 RESULTS -->"
	modelEvalDynamic3ResultsEnd   = "<!-- END MODEL EVAL DYNAMIC3 RESULTS -->"
	modelEvalSummaryStart         = "<!-- START MODEL EVAL SUMMARY -->"
	modelEvalSummaryEnd           = "<!-- END MODEL EVAL SUMMARY -->"
	modelEvalResultsStart         = "<!-- START MODEL EVAL RESULTS -->"
	modelEvalResultsEnd           = "<!-- END MODEL EVAL RESULTS -->"
	publishProjectResolveAction   = "discover_project.resolve"
	publishProjectSearchAction    = "search.projects"
	publishSamplingContinue       = "sampling_unsupported_continue"
	publishElicitationContinue    = "elicitation_unsupported_continue"

	publishSectionMeta     = "meta"
	publishSectionDynamic3 = "dynamic3"

	usageModelRequests    = "Model requests"
	usageToolCallsEmitted = "Tool calls emitted"
	usageToolCalls        = "Tool calls"
	usageInputTokens      = "Input tokens"
	usageOutputTokens     = "Output tokens"
	usageEstimatedCost    = "Estimated cost"
	modelEvaluationSuffix = " Model Evaluation"
)

var fullDockerAttemptsByPreset = map[string]int{
	presetDockerRead:            40,
	presetDockerMutatingSafe:    25,
	presetDockerDestructiveSafe: 53,
}

// publishReport holds data for main operations.
type publishReport struct {
	Path                   string
	Date                   string
	Mode                   string
	Model                  string
	ToolSurface            string
	Backend                string
	Preset                 string
	ToolExecution          string
	GitBranch              string
	GitCommit              string
	Diagnostics            map[string]int
	UnresolvedHarnessNoise bool
	Rows                   []publishRow
}

// publishRow holds data for main operations.
type publishRow struct {
	SourcePath        string
	Model             string
	Preset            string
	Backend           string
	ToolExecution     string
	Attempts          int
	ExpectedOps       int
	ModelRequests     int
	ToolCalls         int
	ToolSelection     float64
	ActionSelection   float64
	FirstPass         float64
	RepairSuccess     float64
	RepairAttempts    int
	RepairSuccesses   int
	DestructiveSafety float64
	FinalSuccess      float64
	CostTokens        string
	GitBranch         string
	GitCommit         string
	Date              string
}

// publishTaskStats holds data for main operations.
type publishTaskStats struct {
	Attempts        int
	ExpectedOps     int
	ModelRequests   int
	ToolCalls       int
	RepairAttempts  int
	RepairSuccesses int
}

// publishTaskMetrics holds count-based metrics for one publish row.
type publishTaskMetrics struct {
	ToolOK           int
	ActionOK         int
	FirstPassOK      int
	FinalSuccessOK   int
	DestructiveTotal int
	DestructiveOK    int
}

// publishModelMetrics holds data for main operations.
type publishModelMetrics struct {
	Attempts          int
	ToolSelection     float64
	ActionSelection   float64
	FirstPass         float64
	RepairSuccess     float64
	DestructiveSafety float64
	FinalSuccess      float64
}

// publishTraceAccumulator aggregates one model/preset slice from trace JSONL.
type publishTraceAccumulator struct {
	Stats        publishTaskStats
	Metrics      publishTaskMetrics
	InputTokens  int
	OutputTokens int
}

// publishModelSummary holds data for main operations.
type publishModelSummary struct {
	Model           string
	Attempts        int
	ExpectedOps     int
	ToolSelection   float64
	ActionSelection float64
	RepairSuccess   float64
	RepairAttempts  int
	RepairSuccesses int
	FinalSuccess    float64
	DockerBacked    bool
}

// publishDocSection identifies one independently managed publication section.
type publishDocSection struct {
	Key                string
	ResultsStartMarker string
	ResultsEndMarker   string
	SummaryStartMarker string
	SummaryEndMarker   string
}

// publishEvaluationDocs is an internal helper for the main package.
func publishEvaluationDocs(opts options) error {
	if len(opts.PublishFrom) == 0 {
		return errors.New("--publish-docs and --check-docs require at least one --publish-from report")
	}
	if opts.PublishMode != publishModeAppend && opts.PublishMode != publishModeReplaceCurrent {
		return fmt.Errorf("--publish-mode must be %q or %q", publishModeAppend, publishModeReplaceCurrent)
	}

	reports, readErr := readPublishReports(opts.PublishFrom)
	if readErr != nil {
		return readErr
	}
	label := publishSnapshotLabel(opts.PublishLabel, reports)
	if validateErr := validatePublishReports(reports, label, opts.PublishAllowNoise); validateErr != nil {
		return validateErr
	}

	applyManagedDoc := updateManagedDoc
	if opts.CheckDocs {
		applyManagedDoc = checkManagedDoc
	}
	for _, section := range publishDocSectionsForReports(reports) {
		sectionReports := filterPublishReportsBySection(reports, section.Key)
		sectionLabel := publishSectionLabel(label)
		resultsBlock := buildModelResultsBlock(sectionLabel, sectionReports)
		summaryBlock := buildReadmeSummaryBlock(sectionLabel, sectionReports)
		if applyErr := applyManagedDoc(opts.PublishResults, section.ResultsStartMarker, section.ResultsEndMarker, resultsBlock, opts.PublishMode, sectionLabel); applyErr != nil {
			return applyErr
		}
		if applyErr := applyManagedDoc(opts.PublishReadme, section.SummaryStartMarker, section.SummaryEndMarker, summaryBlock, publishModeReplaceCurrent, sectionLabel); applyErr != nil {
			return applyErr
		}
	}
	if opts.CheckDocs {
		return nil
	}
	fmt.Printf("published evaluation docs: %s, %s\n", opts.PublishResults, opts.PublishReadme)
	return nil
}

// publishDocSectionsForReports returns the managed sections touched by reports.
func publishDocSectionsForReports(reports []publishReport) []publishDocSection {
	keys := map[string]bool{}
	for _, report := range reports {
		keys[publishSectionForReport(report)] = true
	}
	sections := make([]publishDocSection, 0, len(keys))
	for _, key := range []string{publishSectionMeta, publishSectionDynamic3} {
		if keys[key] {
			sections = append(sections, publishDocSectionForKey(key))
		}
	}
	return sections
}

// filterPublishReportsBySection keeps reports for one managed section.
func filterPublishReportsBySection(reports []publishReport, sectionKey string) []publishReport {
	filtered := make([]publishReport, 0, len(reports))
	for _, report := range reports {
		if publishSectionForReport(report) == sectionKey {
			filtered = append(filtered, report)
		}
	}
	return filtered
}

// publishSectionForReport maps a report tool surface to its publication section.
func publishSectionForReport(report publishReport) string {
	surface := strings.ToLower(strings.TrimSpace(report.ToolSurface))
	switch surface {
	case "", config.ToolSurfaceMeta:
		return publishSectionMeta
	case config.ToolSurfaceDynamic, config.ToolSurfaceDynamic2, config.ToolSurfaceDynamic3:
		return publishSectionDynamic3
	default:
		return publishSectionMeta
	}
}

// publishDocSectionForKey returns marker pairs for a publication section.
func publishDocSectionForKey(sectionKey string) publishDocSection {
	switch sectionKey {
	case publishSectionDynamic3:
		return publishDocSection{
			Key:                publishSectionDynamic3,
			ResultsStartMarker: modelEvalDynamic3ResultsStart,
			ResultsEndMarker:   modelEvalDynamic3ResultsEnd,
			SummaryStartMarker: modelEvalDynamic3SummaryStart,
			SummaryEndMarker:   modelEvalDynamic3SummaryEnd,
		}
	default:
		return publishDocSection{
			Key:                publishSectionMeta,
			ResultsStartMarker: modelEvalMetaResultsStart,
			ResultsEndMarker:   modelEvalMetaResultsEnd,
			SummaryStartMarker: modelEvalMetaSummaryStart,
			SummaryEndMarker:   modelEvalMetaSummaryEnd,
		}
	}
}

// publishSectionLabel returns the snapshot heading used within a managed section.
func publishSectionLabel(label string) string {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		trimmed = strings.TrimSpace(publishSnapshotLabel("", nil))
	}
	return trimmed
}

// readPublishReports parses one or more local evaluation reports.
func readPublishReports(paths []string) ([]publishReport, error) {
	reports := make([]publishReport, 0, len(paths))
	for _, path := range paths {
		report, err := readPublishReport(path)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

// readPublishReport parses one local evaluation report selected for publication.
func readPublishReport(path string) (publishReport, error) {
	input, err := parseComparisonInput(path)
	if err != nil {
		return publishReport{}, err
	}
	if input.Kind != "evaluation" {
		return publishReport{}, fmt.Errorf("publish input %s must be an eval_meta_tools evaluation report", path)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- explicit developer-provided report path.
	if err != nil {
		return publishReport{}, fmt.Errorf("read publish input %s: %w", path, err)
	}
	content := string(data)
	report := publishReport{
		Path:                   path,
		Date:                   input.Date,
		Mode:                   input.Mode,
		Model:                  input.Model,
		ToolSurface:            input.ToolSurface,
		Backend:                input.Backend,
		Preset:                 input.Preset,
		ToolExecution:          input.ToolExecution,
		GitBranch:              firstMetadataValue(content, "Git branch"),
		GitCommit:              firstMetadataValue(content, "Git commit"),
		Diagnostics:            input.Diagnostics,
		UnresolvedHarnessNoise: reportMentionsHarnessNoise(content),
	}
	rows, rowsErr := publishRowsForReport(report, input, content)
	if rowsErr != nil {
		return publishReport{}, rowsErr
	}
	report.Rows = rows
	if len(report.Rows) == 0 {
		return publishReport{}, fmt.Errorf("publish input %s has no task result rows", path)
	}
	return report, nil
}

// publishRowsForReport is an internal helper for the main package.
func publishRowsForReport(report publishReport, input comparisonInput, content string) ([]publishRow, error) {
	if shouldSplitPublishReportByPreset(report) {
		rows, splitErr := publishRowsByPresetFromTraces(report, content)
		if splitErr != nil {
			return nil, splitErr
		}
		return rows, nil
	}
	taskStats := publishTaskStatsByModel(content, report.Model)
	modelMetrics := publishMetricsByModel(content)
	modelUsage := publishUsageByModel(content)
	if len(modelMetrics) == 0 {
		model := report.Model
		stats := publishSingleTaskStats(taskStats, model, input.TaskAttempts)
		usage := publishSingleUsage(input.Usage, stats)
		return []publishRow{newPublishRow(report, model, report.Preset, stats, metricsFromComparison(input), usage)}, nil
	}

	models := sortedStringKeys(modelMetrics)
	rows := make([]publishRow, 0, len(models))
	for _, model := range models {
		stats := taskStats[model]
		if stats.Attempts == 0 {
			stats.Attempts = modelMetrics[model].Attempts
		}
		usage := modelUsage[model]
		if len(usage) == 0 {
			usage = publishSingleUsage(input.Usage, stats)
		}
		rows = append(rows, newPublishRow(report, model, report.Preset, stats, modelMetrics[model], usage))
	}
	return rows, nil
}

func shouldSplitPublishReportByPreset(report publishReport) bool {
	if strings.TrimSpace(report.Preset) != "" {
		return false
	}
	return report.Backend == backendGitLab && report.ToolExecution == "mcp"
}

func publishRowsByPresetFromTraces(report publishReport, content string) ([]publishRow, error) {
	tracePath := publishTraceJSONLPath(report.Path, content)
	if tracePath == "" {
		return nil, fmt.Errorf("publish input %s has no preset and no trace artifacts; publish full runs with trace artifacts or publish separate preset reports", report.Path)
	}
	tasks, err := parseTasksFile(publishTasksPath())
	if err != nil {
		return nil, fmt.Errorf("read publish task presets: %w", err)
	}
	tasksByID := make(map[string]evalTask, len(tasks))
	for _, task := range tasks {
		tasksByID[task.ID] = task
	}
	file, err := os.Open(tracePath) // #nosec G304 -- trace path comes from an explicit developer-selected evaluation report.
	if err != nil {
		return nil, fmt.Errorf("read publish trace artifacts %s: %w", tracePath, err)
	}
	defer file.Close()

	accumulators := map[string]*publishTraceAccumulator{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxResponseBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var trace taskTrace
		if decodeErr := json.Unmarshal([]byte(line), &trace); decodeErr != nil {
			return nil, fmt.Errorf("decode publish trace %s: %w", tracePath, decodeErr)
		}
		model := cleanReportValue(trace.Model)
		if model == "" {
			model = report.Model
		}
		task, ok := tasksByID[trace.TaskID]
		if !ok {
			return nil, fmt.Errorf("publish trace %s references unknown task %s", tracePath, trace.TaskID)
		}
		preset := publishPresetForTask(task)
		key := model + "\x00" + preset
		acc := accumulators[key]
		if acc == nil {
			acc = &publishTraceAccumulator{}
			accumulators[key] = acc
		}
		acc.addTrace(trace, task, report.ToolSurface)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("scan publish trace artifacts %s: %w", tracePath, scanErr)
	}
	rows := make([]publishRow, 0, len(accumulators))
	for key, acc := range accumulators {
		model, preset, _ := strings.Cut(key, "\x00")
		rows = append(rows, newPublishRow(report, model, preset, acc.Stats, acc.metrics(), acc.usage()))
	}
	return rows, nil
}

func publishTasksPath() string {
	if _, err := os.Stat(defaultTasksPath); err == nil {
		return defaultTasksPath
	}
	packageRelative := filepath.Join("testdata", filepath.Base(defaultTasksPath))
	if _, err := os.Stat(packageRelative); err == nil {
		return packageRelative
	}
	return defaultTasksPath
}

func (a *publishTraceAccumulator) addTrace(trace taskTrace, task evalTask, toolSurface string) {
	a.Stats.Attempts++
	expectedSteps := trace.Summary.ExpectedSteps
	if expectedSteps == 0 {
		expectedSteps = len(trace.Expected)
	}
	a.Stats.ExpectedOps += expectedSteps
	a.Stats.ModelRequests += trace.Summary.ModelCalls
	a.Stats.ToolCalls += trace.Summary.ToolCalls
	if trace.Summary.RepairAttempted {
		a.Stats.RepairAttempts++
		if trace.Summary.RepairSuccess {
			a.Stats.RepairSuccesses++
		}
	}
	toolOK, actionOK, firstPassOK := publishEffectiveTraceOutcome(trace, toolSurface)
	if toolOK {
		a.Metrics.ToolOK++
	}
	if actionOK {
		a.Metrics.ActionOK++
	}
	if firstPassOK {
		a.Metrics.FirstPassOK++
	}
	if trace.Summary.FinalSuccess {
		a.Metrics.FinalSuccessOK++
	}
	if taskHasDestructiveStep(task) {
		a.Metrics.DestructiveTotal++
		if trace.Summary.DestructiveSafe {
			a.Metrics.DestructiveOK++
		}
	}
	for _, event := range trace.Events {
		if event.Usage == nil {
			continue
		}
		a.InputTokens += event.Usage.InputTokens
		a.OutputTokens += event.Usage.OutputTokens
	}
}

func publishEffectiveTraceOutcome(trace taskTrace, toolSurface string) (toolOK, actionOK, firstPassOK bool) {
	if len(trace.Expected) == 0 {
		return false, false, false
	}
	first := trace.Expected[0]
	toolOK = trace.Summary.FirstTool == first.Tool
	actionOK = trace.Summary.FirstAction == first.Action
	firstPassOK = trace.Summary.FirstPass
	if !trace.Summary.FinalSuccess || !isDynamicThreeToolEvalSurface(toolSurface) || !toolOK {
		return toolOK, actionOK, firstPassOK
	}
	if first.Action == publishProjectResolveAction && trace.Summary.FirstAction == publishProjectSearchAction {
		return true, true, true
	}
	if len(trace.Expected) < 2 {
		return toolOK, actionOK, firstPassOK
	}
	if first.Simulation != publishSamplingContinue && first.Simulation != publishElicitationContinue {
		return toolOK, actionOK, firstPassOK
	}
	if trace.Summary.FirstAction == trace.Expected[1].Action {
		return true, true, true
	}
	return toolOK, actionOK, firstPassOK
}

func (a *publishTraceAccumulator) metrics() publishModelMetrics {
	return publishModelMetrics{
		Attempts:          a.Stats.Attempts,
		ToolSelection:     percent(a.Metrics.ToolOK, a.Stats.Attempts),
		ActionSelection:   percent(a.Metrics.ActionOK, a.Stats.Attempts),
		FirstPass:         percent(a.Metrics.FirstPassOK, a.Stats.Attempts),
		RepairSuccess:     percent(a.Stats.RepairSuccesses, a.Stats.RepairAttempts),
		DestructiveSafety: percent(a.Metrics.DestructiveOK, a.Metrics.DestructiveTotal),
		FinalSuccess:      percent(a.Metrics.FinalSuccessOK, a.Stats.Attempts),
	}
}

func (a *publishTraceAccumulator) usage() map[string]string {
	return map[string]string{
		usageModelRequests:    strconv.Itoa(a.Stats.ModelRequests),
		usageToolCallsEmitted: strconv.Itoa(a.Stats.ToolCalls),
		usageInputTokens:      strconv.Itoa(a.InputTokens),
		usageOutputTokens:     strconv.Itoa(a.OutputTokens),
	}
}

func publishPresetForTask(task evalTask) string {
	for _, preset := range []string{presetDockerRead, presetDockerMutatingSafe, presetDockerDestructiveSafe, presetSchemaEnterprise} {
		if taskMatchesPreset(task, preset) {
			return preset
		}
	}
	for _, partition := range []string{partitionErrorRecovery, partitionCapabilityFallback} {
		if taskMatchesPartition(task, partition) {
			return partition
		}
	}
	return "other"
}

func publishTraceJSONLPath(reportPath, content string) string {
	traceDir := firstMetadataValue(content, "Trace artifacts")
	if traceDir == "" {
		return ""
	}
	tracePath := filepath.Join(traceDir, "traces.jsonl")
	if filepath.IsAbs(tracePath) {
		return tracePath
	}
	reportRelative := filepath.Join(filepath.Dir(reportPath), tracePath)
	if _, err := os.Stat(reportRelative); err == nil {
		return reportRelative
	}
	if _, err := os.Stat(tracePath); err == nil {
		return tracePath
	}
	return reportRelative
}

// publishSingleTaskStats is an internal helper for the main package.
func publishSingleTaskStats(statsByModel map[string]publishTaskStats, model string, fallbackAttempts int) publishTaskStats {
	stats := statsByModel[model]
	if stats.Attempts == 0 && len(statsByModel) == 1 {
		for _, only := range statsByModel {
			stats = only
		}
	}
	if stats.Attempts == 0 {
		stats.Attempts = fallbackAttempts
	}
	return stats
}

// metricsFromComparison is an internal helper for the main package.
func metricsFromComparison(input comparisonInput) publishModelMetrics {
	return publishModelMetrics{
		Attempts:          input.TaskAttempts,
		ToolSelection:     input.Metrics[metricToolSelection],
		ActionSelection:   input.Metrics[metricActionSelection],
		FirstPass:         input.Metrics[metricFirstCallValidationPassRate],
		RepairSuccess:     input.Metrics[metricRepairSuccessRate],
		DestructiveSafety: input.Metrics[metricDestructiveSafety],
		FinalSuccess:      input.Metrics[metricFinalTaskSuccess],
	}
}

// publishSingleUsage is an internal helper for the main package.
func publishSingleUsage(usage map[string]string, stats publishTaskStats) map[string]string {
	out := map[string]string{}
	maps.Copy(out, usage)
	if out[usageModelRequests] == "" && stats.ModelRequests > 0 {
		out[usageModelRequests] = strconv.Itoa(stats.ModelRequests)
	}
	if out[usageToolCallsEmitted] == "" && stats.ToolCalls > 0 {
		out[usageToolCallsEmitted] = strconv.Itoa(stats.ToolCalls)
	}
	return out
}

// newPublishRow is an internal helper for the main package.
func newPublishRow(report publishReport, model, preset string, stats publishTaskStats, metrics publishModelMetrics, usage map[string]string) publishRow {
	return publishRow{
		SourcePath:        report.Path,
		Model:             cleanReportValue(model),
		Preset:            preset,
		Backend:           report.Backend,
		ToolExecution:     report.ToolExecution,
		Attempts:          firstPositive(stats.Attempts, metrics.Attempts),
		ExpectedOps:       stats.ExpectedOps,
		ModelRequests:     firstPositive(parseReportInt(usage[usageModelRequests]), parseReportInt(usage["Requests"]), stats.ModelRequests),
		ToolCalls:         firstPositive(parseReportInt(usage[usageToolCallsEmitted]), parseReportInt(usage[usageToolCalls]), stats.ToolCalls),
		ToolSelection:     metrics.ToolSelection,
		ActionSelection:   metrics.ActionSelection,
		FirstPass:         metrics.FirstPass,
		RepairSuccess:     metrics.RepairSuccess,
		RepairAttempts:    stats.RepairAttempts,
		RepairSuccesses:   stats.RepairSuccesses,
		DestructiveSafety: metrics.DestructiveSafety,
		FinalSuccess:      metrics.FinalSuccess,
		CostTokens:        publishCostTokens(usage),
		GitBranch:         report.GitBranch,
		GitCommit:         report.GitCommit,
		Date:              report.Date,
	}
}

// publishTaskStatsByModel is an internal helper for the main package.
func publishTaskStatsByModel(content, defaultModel string) map[string]publishTaskStats {
	out := map[string]publishTaskStats{}
	for _, row := range reportNamedTableRows(content, "## Task Results") {
		model := cleanReportValue(row["Model"])
		if model == "" {
			model = defaultModel
		}
		stats := out[model]
		stats.Attempts++
		stats.ExpectedOps += parseExpectedOps(row["Steps"])
		stats.ModelRequests += parseReportInt(row["Calls"])
		stats.ToolCalls += parseReportInt(row[usageToolCalls])
		switch row["Repair"] {
		case "Yes":
			stats.RepairAttempts++
			stats.RepairSuccesses++
		case "No":
			stats.RepairAttempts++
		}
		out[model] = stats
	}
	return out
}

// publishMetricsByModel is an internal helper for the main package.
func publishMetricsByModel(content string) map[string]publishModelMetrics {
	out := map[string]publishModelMetrics{}
	for _, row := range reportNamedTableRows(content, "## Per-Model Metrics") {
		model := cleanReportValue(row["Model"])
		if model == "" {
			continue
		}
		out[model] = publishModelMetrics{
			Attempts:          parseReportInt(row["Attempts"]),
			ToolSelection:     parseReportPercent(row["Tool"]),
			ActionSelection:   parseReportPercent(row["Action"]),
			FirstPass:         parseReportPercent(row["First pass"]),
			RepairSuccess:     parseReportPercent(row["Repair success"]),
			DestructiveSafety: parseReportPercent(row["Destructive safety"]),
			FinalSuccess:      parseReportPercent(row["Final success"]),
		}
	}
	return out
}

// publishUsageByModel is an internal helper for the main package.
func publishUsageByModel(content string) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, row := range reportNamedTableRows(content, "### API Usage By Model") {
		model := cleanReportValue(row["Model"])
		if model == "" {
			continue
		}
		out[model] = map[string]string{
			usageModelRequests:    row["Requests"],
			usageToolCallsEmitted: row[usageToolCalls],
			usageInputTokens:      row[usageInputTokens],
			usageOutputTokens:     row[usageOutputTokens],
			usageEstimatedCost:    row[usageEstimatedCost],
		}
	}
	return out
}

// reportNamedTableRows is an internal helper for the main package.
func reportNamedTableRows(content, heading string) []map[string]string {
	rows := reportTableRowsForHeading(content, heading)
	if len(rows) < 2 {
		return nil
	}
	headers := rows[0]
	out := make([]map[string]string, 0, len(rows)-1)
	for _, cells := range rows[1:] {
		row := map[string]string{}
		for i, header := range headers {
			if i < len(cells) {
				row[cleanReportValue(header)] = cleanReportValue(cells[i])
			}
		}
		out = append(out, row)
	}
	return out
}

// reportTableRowsForHeading is an internal helper for the main package.
func reportTableRowsForHeading(content, heading string) [][]string {
	var rows [][]string
	for _, line := range sectionLinesForHeading(strings.Split(content, "\n"), heading) {
		rows = appendReportTableRow(rows, line)
	}
	return rows
}

// sectionLinesForHeading is an internal helper for the main package.
func sectionLinesForHeading(lines []string, heading string) []string {
	level := markdownHeadingLevel(heading)
	if level == 0 {
		return nil
	}
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == heading {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return nil
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if nextLevel := markdownHeadingLevel(strings.TrimSpace(lines[i])); nextLevel > 0 && nextLevel <= level {
			end = i
			break
		}
	}
	return lines[start:end]
}

// markdownHeadingLevel is an internal helper for the main package.
func markdownHeadingLevel(line string) int {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return 0
	}
	count := 0
	for _, r := range trimmed {
		if r != '#' {
			break
		}
		count++
	}
	if count == 0 || count >= len(trimmed) || trimmed[count] != ' ' {
		return 0
	}
	return count
}

// parseExpectedOps is an internal helper for the main package.
func parseExpectedOps(value string) int {
	value = cleanReportValue(value)
	if _, after, ok := strings.Cut(value, "/"); ok {
		return parseReportInt(after)
	}
	return parseReportInt(value)
}

// publishCostTokens is an internal helper for the main package.
func publishCostTokens(usage map[string]string) string {
	inputTokens := parseReportInt(usage[usageInputTokens])
	outputTokens := parseReportInt(usage[usageOutputTokens])
	cost := cleanReportValue(usage[usageEstimatedCost])
	parts := make([]string, 0, 3)
	if inputTokens > 0 || outputTokens > 0 {
		parts = append(parts, fmt.Sprintf("in %d / out %d", inputTokens, outputTokens))
	}
	if cost != "" && cost != "Not configured" {
		parts = append(parts, cost)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "; ")
}

// validatePublishReports is an internal helper for the main package.
func validatePublishReports(reports []publishReport, label string, allowHarnessNoise bool) error {
	labelLower := strings.ToLower(label)
	for _, report := range reports {
		if report.Backend == backendGitLab && report.ToolExecution != "mcp" {
			return fmt.Errorf("publish input %s uses backend=gitlab but Tool execution is %q; Docker metrics require --execute-tools", report.Path, report.ToolExecution)
		}
		if report.UnresolvedHarnessNoise && !allowHarnessNoise {
			return fmt.Errorf("publish input %s mentions harness noise; pass --publish-allow-harness-noise only after it is explicitly resolved or accepted", report.Path)
		}
		for _, row := range report.Rows {
			if expectedAttempts := fullDockerAttemptsByPreset[row.Preset]; expectedAttempts > 0 && row.Attempts < expectedAttempts && !strings.Contains(labelLower, "targeted") {
				return fmt.Errorf("publish input %s has partial %s row for %s (%d attempts, expected at least %d); include targeted in --publish-label or publish a full preset report", report.Path, row.Preset, row.Model, row.Attempts, expectedAttempts)
			}
		}
	}
	return nil
}

// reportMentionsHarnessNoise is an internal helper for the main package.
func reportMentionsHarnessNoise(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "harness noise") || strings.Contains(lower, "harness_noise")
}

// publishSnapshotLabel is an internal helper for the main package.
func publishSnapshotLabel(label string, reports []publishReport) string {
	label = strings.TrimSpace(label)
	if label != "" {
		return label
	}
	for _, report := range reports {
		if report.Date != "" {
			if parsed, err := time.Parse(time.RFC3339, report.Date); err == nil {
				return parsed.UTC().Format("2006-01-02") + modelEvaluationSuffix
			}
			return report.Date + modelEvaluationSuffix
		}
	}
	return time.Now().UTC().Format("2006-01-02") + modelEvaluationSuffix
}

// buildModelResultsBlock constructs the request parameters from the input.
func buildModelResultsBlock(label string, reports []publishReport) string {
	rows := sortedPublishRows(reports)
	aggregate := aggregatePublishRows(rows)
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n\n", label)
	fmt.Fprintf(&b, "| Model | Preset | Backend | Attempts | Expected ops | %s | %s | Tool-selection | Action-selection | First-pass validation | Repair success | Destructive safety | Final task success | Cost/tokens | Commit / branch / date |\n", usageModelRequests, usageToolCallsEmitted)
	fmt.Fprintf(&b, "| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- |\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %d | %d | %d | %d | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			escapeTable(row.Model), emptyDash(row.Preset), dockerBackendLabel(row), row.Attempts, row.ExpectedOps, row.ModelRequests, row.ToolCalls,
			formatMetric(row.ToolSelection), formatMetric(row.ActionSelection), formatMetric(row.FirstPass), formatRepairMetric(row), formatMetric(row.DestructiveSafety), formatMetric(row.FinalSuccess), emptyDash(row.CostTokens), emptyDash(rowCommitBranchDate(row)))
	}
	fmt.Fprintf(&b, "| **Aggregate** | **all selected** | - | **%d** | **%d** | **%d** | **%d** | **%s** | **%s** | **%s** | **%s** | **%s** | **%s** | - | - |\n",
		aggregate.Attempts, aggregate.ExpectedOps, aggregate.ModelRequests, aggregate.ToolCalls, formatMetric(aggregate.ToolSelection), formatMetric(aggregate.ActionSelection), formatMetric(aggregate.FirstPass), formatRepairMetric(aggregate), formatMetric(aggregate.DestructiveSafety), formatMetric(aggregate.FinalSuccess))
	fmt.Fprintf(&b, "\nPublished with `cmd/eval_meta_tools --publish-docs` from reviewed Markdown reports. Raw traces and JSON artifacts are not included here.\n")
	return strings.TrimSpace(b.String()) + "\n"
}

// buildReadmeSummaryBlock constructs the request parameters from the input.
func buildReadmeSummaryBlock(label string, reports []publishReport) string {
	rows := sortedPublishRows(reports)
	summaries := publishSummariesByModel(rows)
	aggregate := aggregatePublishRows(rows)
	var b strings.Builder
	fmt.Fprintf(&b, "Current published result: **%s**.\n\n", label)
	fmt.Fprintf(&b, "| Provider | Model | Compatibility | Tool accuracy | Recovery | Docker live status |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | ---: | ---: | --- |\n")
	for _, summary := range summaries {
		provider, model := providerModel(summary.Model)
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s | %s | %s |\n",
			provider, escapeTable(model), compatibilityLabel(summary), formatMetric(summary.ToolSelection), formatRecoverySummary(summary), dockerLiveStatus(summary))
	}
	fmt.Fprintf(&b, "\nThe published model-evaluation set covers %d task attempts and %d expected MCP operations. Across the selected reports, models emitted %d tool calls over %d model requests, with %s aggregate final success. See [AI Model Evaluation Results](docs/testing/model-results.md) for the detailed current matrix.\n",
		aggregate.Attempts, aggregate.ExpectedOps, aggregate.ToolCalls, aggregate.ModelRequests, formatMetric(aggregate.FinalSuccess))
	return strings.TrimSpace(b.String()) + "\n"
}

// sortedPublishRows is an internal helper for the main package.
func sortedPublishRows(reports []publishReport) []publishRow {
	var rows []publishRow
	rowIndexes := map[string]int{}
	for _, report := range reports {
		for _, row := range report.Rows {
			key := publishRowKey(row)
			if index, ok := rowIndexes[key]; ok {
				rows[index] = row
				continue
			}
			rowIndexes[key] = len(rows)
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if presetRank(rows[i].Preset) != presetRank(rows[j].Preset) {
			return presetRank(rows[i].Preset) < presetRank(rows[j].Preset)
		}
		if rows[i].Model != rows[j].Model {
			return rows[i].Model < rows[j].Model
		}
		return rows[i].SourcePath < rows[j].SourcePath
	})
	return rows
}

// publishRowKey is an internal helper for the main package.
func publishRowKey(row publishRow) string {
	return strings.Join([]string{row.Model, row.Preset, row.Backend, row.ToolExecution}, "\x00")
}

// presetRank is an internal helper for the main package.
func presetRank(preset string) int {
	switch preset {
	case presetDockerRead:
		return 1
	case presetDockerMutatingSafe:
		return 2
	case presetDockerDestructiveSafe:
		return 3
	case partitionErrorRecovery:
		return 4
	case partitionCapabilityFallback:
		return 5
	case presetSchemaEnterprise:
		return 6
	default:
		return 99
	}
}

// aggregatePublishRows is an internal helper for the main package.
func aggregatePublishRows(rows []publishRow) publishRow {
	var out publishRow
	for _, row := range rows {
		out.Attempts += row.Attempts
		out.ExpectedOps += row.ExpectedOps
		out.ModelRequests += row.ModelRequests
		out.ToolCalls += row.ToolCalls
		out.ToolSelection += row.ToolSelection * float64(row.Attempts)
		out.ActionSelection += row.ActionSelection * float64(row.Attempts)
		out.FirstPass += row.FirstPass * float64(row.Attempts)
		out.RepairAttempts += row.RepairAttempts
		out.RepairSuccesses += row.RepairSuccesses
		out.DestructiveSafety += row.DestructiveSafety * float64(row.Attempts)
		out.FinalSuccess += row.FinalSuccess * float64(row.Attempts)
	}
	if out.Attempts == 0 {
		return out
	}
	denominator := float64(out.Attempts)
	out.ToolSelection /= denominator
	out.ActionSelection /= denominator
	out.FirstPass /= denominator
	out.RepairSuccess = percent(out.RepairSuccesses, out.RepairAttempts)
	out.DestructiveSafety /= denominator
	out.FinalSuccess /= denominator
	return out
}

// publishSummariesByModel is an internal helper for the main package.
func publishSummariesByModel(rows []publishRow) []publishModelSummary {
	byModel := map[string][]publishRow{}
	for _, row := range rows {
		byModel[row.Model] = append(byModel[row.Model], row)
	}
	models := sortedStringKeys(byModel)
	summaries := make([]publishModelSummary, 0, len(models))
	for _, model := range models {
		aggregate := aggregatePublishRows(byModel[model])
		summary := publishModelSummary{
			Model:           model,
			Attempts:        aggregate.Attempts,
			ExpectedOps:     aggregate.ExpectedOps,
			ToolSelection:   aggregate.ToolSelection,
			ActionSelection: aggregate.ActionSelection,
			RepairSuccess:   aggregate.RepairSuccess,
			RepairAttempts:  aggregate.RepairAttempts,
			RepairSuccesses: aggregate.RepairSuccesses,
			FinalSuccess:    aggregate.FinalSuccess,
			DockerBacked:    true,
		}
		for _, row := range byModel[model] {
			if row.Backend != backendGitLab || row.ToolExecution != "mcp" {
				summary.DockerBacked = false
			}
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

// providerModel is an internal helper for the main package.
func providerModel(model string) (providerName, modelName string) {
	provider, modelName, ok := strings.Cut(model, ":")
	if !ok {
		return "Unknown", model
	}
	switch strings.ToLower(provider) {
	case "anthropic":
		return "Anthropic", modelName
	case "google":
		return "Google", modelName
	case "openai":
		return "OpenAI", modelName
	case "qwen":
		return "Qwen", modelName
	default:
		if provider == "" {
			return "Unknown", modelName
		}
		return strings.ToUpper(provider[:1]) + provider[1:], modelName
	}
}

// compatibilityLabel is an internal helper for the main package.
func compatibilityLabel(summary publishModelSummary) string {
	if summary.ToolSelection == 100 && summary.ActionSelection == 100 && summary.FinalSuccess == 100 {
		return "OK"
	}
	return "Review"
}

// dockerLiveStatus is an internal helper for the main package.
func dockerLiveStatus(summary publishModelSummary) string {
	if !summary.DockerBacked {
		return "Not Docker-backed"
	}
	return fmt.Sprintf("%s final across %d ops", formatMetric(summary.FinalSuccess), summary.ExpectedOps)
}

// formatRepairMetric renders the result as a formatted string.
func formatRepairMetric(row publishRow) string {
	if row.RepairAttempts == 0 {
		return "-"
	}
	return fmt.Sprintf("%s (%d/%d)", formatMetric(row.RepairSuccess), row.RepairSuccesses, row.RepairAttempts)
}

// formatRecoverySummary renders the result as a formatted string.
func formatRecoverySummary(summary publishModelSummary) string {
	if summary.RepairAttempts == 0 {
		return "No repairs"
	}
	return fmt.Sprintf("%s (%d/%d)", formatMetric(summary.RepairSuccess), summary.RepairSuccesses, summary.RepairAttempts)
}

// dockerBackendLabel is an internal helper for the main package.
func dockerBackendLabel(row publishRow) string {
	if row.Backend == backendGitLab && row.ToolExecution == "mcp" {
		return "Docker GitLab via MCP"
	}
	if row.Backend == "" {
		return "-"
	}
	return escapeTable(row.Backend)
}

// rowCommitBranchDate is an internal helper for the main package.
func rowCommitBranchDate(row publishRow) string {
	branch := row.GitBranch
	if branch == "" {
		branch = "-"
	}
	commit := row.GitCommit
	if commit == "" {
		commit = "-"
	}
	date := row.Date
	if date == "" {
		date = "-"
	}
	return commit + " / " + branch + " / " + date
}

// updateManagedDoc is an internal helper for the main package.
func updateManagedDoc(path, startMarker, endMarker, block, mode, label string) error {
	content, err := readTextFile(path)
	if err != nil {
		return err
	}
	updated, applyErr := applyManagedBlock(content, startMarker, endMarker, block, mode, label)
	if applyErr != nil {
		return fmt.Errorf("update %s: %w", path, applyErr)
	}
	if updated == content {
		fmt.Printf("published evaluation docs unchanged: %s\n", path)
		return nil
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o750); mkdirErr != nil {
		return fmt.Errorf("create publish doc directory: %w", mkdirErr)
	}
	if writeErr := os.WriteFile(path, []byte(updated), 0o644); writeErr != nil { // #nosec G306 -- tracked Markdown docs should remain world-readable.
		return fmt.Errorf("write publish doc %s: %w", path, writeErr)
	}
	fmt.Printf("updated evaluation docs: %s\n", path)
	return nil
}

// checkManagedDoc is an internal helper for the main package.
func checkManagedDoc(path, startMarker, endMarker, block, mode, label string) error {
	content, err := readTextFile(path)
	if err != nil {
		return err
	}
	updated, applyErr := applyManagedBlock(content, startMarker, endMarker, block, mode, label)
	if applyErr != nil {
		return fmt.Errorf("check %s: %w", path, applyErr)
	}
	if updated != content {
		return fmt.Errorf("%s is not up to date with selected evaluation reports", path)
	}
	fmt.Printf("evaluation docs up to date: %s\n", path)
	return nil
}

// readTextFile reads a local text file.
func readTextFile(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- explicit developer-provided documentation path.
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

// applyManagedBlock replaces or appends a managed Markdown block between markers.
func applyManagedBlock(content, startMarker, endMarker, block, mode, label string) (string, error) {
	start := strings.Index(content, startMarker)
	if start == -1 {
		return "", fmt.Errorf("missing marker %s", startMarker)
	}
	end := strings.Index(content[start:], endMarker)
	if end == -1 {
		return "", fmt.Errorf("missing marker %s", endMarker)
	}
	end += start
	innerStart := start + len(startMarker)
	inner := content[innerStart:end]
	if mode == publishModeAppend {
		block = appendSnapshotBlock(inner, block, label)
	}
	replacement := startMarker + "\n" + strings.TrimSpace(block) + "\n" + endMarker
	return content[:start] + replacement + content[end+len(endMarker):], nil
}

// appendSnapshotBlock is an internal helper for the main package.
func appendSnapshotBlock(inner, block, label string) string {
	trimmedInner := strings.TrimSpace(inner)
	trimmedBlock := strings.TrimSpace(block)
	if trimmedInner == "" {
		return trimmedBlock + "\n"
	}
	heading := "### " + label
	if replaced, ok := replaceSnapshotByHeading(trimmedInner, heading, trimmedBlock); ok {
		return replaced + "\n"
	}
	return trimmedBlock + "\n\n" + trimmedInner + "\n"
}

// replaceSnapshotByHeading replaces the snapshot section that starts at heading.
func replaceSnapshotByHeading(content, heading, replacement string) (string, bool) {
	lines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == heading {
			start = i
			break
		}
	}
	if start == -1 {
		return "", false
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "### ") {
			end = i
			break
		}
	}
	var out []string
	out = append(out, lines[:start]...)
	out = append(out, strings.Split(strings.TrimSpace(replacement), "\n")...)
	out = append(out, lines[end:]...)
	return strings.TrimSpace(strings.Join(out, "\n")), true
}

// firstPositive is an internal helper for the main package.
func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// currentGitReportMetadata is an internal helper for the main package.
func currentGitReportMetadata() (branch, commit string) {
	gitDir, err := resolveGitDir(".")
	if err != nil {
		return "", ""
	}
	head, err := readTrimmedFile(filepath.Join(gitDir, "HEAD"))
	if err != nil || head == "" {
		return "", ""
	}
	if ref, ok := strings.CutPrefix(head, "ref: "); ok {
		commit, _ = readGitRef(gitDir, ref)
		return gitBranchName(ref), shortGitCommit(commit)
	}
	return "HEAD", shortGitCommit(head)
}

// resolveGitDir returns the actual git metadata directory for a worktree.
func resolveGitDir(worktree string) (string, error) {
	dotGit := filepath.Join(worktree, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return dotGit, nil
	}
	content, err := readTrimmedFile(dotGit)
	if err != nil {
		return "", err
	}
	gitDir, ok := strings.CutPrefix(content, "gitdir: ")
	if !ok || strings.TrimSpace(gitDir) == "" {
		return "", errors.New("invalid .git file")
	}
	gitDir = strings.TrimSpace(gitDir)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(worktree, gitDir)
	}
	return filepath.Clean(gitDir), nil
}

// readGitRef resolves ref from gitDir, packed refs, or a linked common dir.
func readGitRef(gitDir, ref string) (string, error) {
	if commit, err := readTrimmedFile(filepath.Join(gitDir, filepath.FromSlash(ref))); err == nil {
		return commit, nil
	}
	if commit, ok := readPackedGitRef(gitDir, ref); ok {
		return commit, nil
	}
	commonDir := gitCommonDir(gitDir)
	if commonDir == gitDir {
		return "", errors.New("git ref not found")
	}
	if commit, readErr := readTrimmedFile(filepath.Join(commonDir, filepath.FromSlash(ref))); readErr == nil {
		return commit, nil
	}
	if commit, ok := readPackedGitRef(commonDir, ref); ok {
		return commit, nil
	}
	return "", fmt.Errorf("git ref %s not found", ref)
}

// gitCommonDir returns the shared metadata directory for a Git worktree.
func gitCommonDir(gitDir string) string {
	commonDir, err := readTrimmedFile(filepath.Join(gitDir, "commondir"))
	if err != nil || commonDir == "" {
		return gitDir
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitDir, commonDir)
	}
	return filepath.Clean(commonDir)
}

// readPackedGitRef returns ref's commit from packed-refs when present.
func readPackedGitRef(gitDir, ref string) (string, bool) {
	// #nosec G304 -- gitDir is resolved from the local repository metadata to read optional report labels.
	content, err := os.ReadFile(filepath.Join(gitDir, "packed-refs"))
	if err != nil {
		return "", false
	}
	for line := range strings.SplitSeq(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		commit, name, ok := strings.Cut(line, " ")
		if ok && name == ref {
			return commit, true
		}
	}
	return "", false
}

// gitBranchName trims refs/heads/ from a Git branch ref.
func gitBranchName(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

// shortGitCommit returns a 12-character commit prefix when possible.
func shortGitCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

// readTrimmedFile reads path and returns surrounding whitespace trimmed.
func readTrimmedFile(path string) (string, error) {
	// #nosec G304 -- callers pass paths derived from the local .git directory for optional report metadata.
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}
