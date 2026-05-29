package evaluator

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

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/docgen"
)

const (
	// defaultPublishResultsDoc identifies the default publish results doc constant used by this package.
	defaultPublishResultsDoc = "docs/testing/model-results.md"
	// defaultPublishReadme identifies the default publish readme constant used by this package.
	defaultPublishReadme = "README.md"

	// publishModeAppend identifies the publish mode append constant used by this package.
	publishModeAppend = "append"
	// publishModeReplaceCurrent identifies the publish mode replace current constant used by this package.
	publishModeReplaceCurrent = "replace-current"

	// modelEvalMetaSummaryStart identifies the model eval meta summary start constant used by this package.
	modelEvalMetaSummaryStart = "<!-- START MODEL EVAL META SUMMARY -->"
	// modelEvalMetaSummaryEnd identifies the model eval meta summary end constant used by this package.
	modelEvalMetaSummaryEnd = "<!-- END MODEL EVAL META SUMMARY -->"
	// modelEvalDynamicSummaryStart identifies the model eval dynamic summary start constant used by this package.
	modelEvalDynamicSummaryStart = "<!-- START MODEL EVAL DYNAMIC SUMMARY -->"
	// modelEvalDynamicSummaryEnd identifies the model eval dynamic summary end constant used by this package.
	modelEvalDynamicSummaryEnd = "<!-- END MODEL EVAL DYNAMIC SUMMARY -->"
	// modelEvalEnterpriseMetaSummaryStart identifies the Enterprise meta summary start marker.
	modelEvalEnterpriseMetaSummaryStart = "<!-- START MODEL EVAL ENTERPRISE META SUMMARY -->"
	// modelEvalEnterpriseMetaSummaryEnd identifies the Enterprise meta summary end marker.
	modelEvalEnterpriseMetaSummaryEnd = "<!-- END MODEL EVAL ENTERPRISE META SUMMARY -->"
	// modelEvalEnterpriseDynamicSummaryStart identifies the Enterprise dynamic summary start marker.
	modelEvalEnterpriseDynamicSummaryStart = "<!-- START MODEL EVAL ENTERPRISE DYNAMIC SUMMARY -->"
	// modelEvalEnterpriseDynamicSummaryEnd identifies the Enterprise dynamic summary end marker.
	modelEvalEnterpriseDynamicSummaryEnd = "<!-- END MODEL EVAL ENTERPRISE DYNAMIC SUMMARY -->"
	// modelEvalMetaResultsStart identifies the model eval meta results start constant used by this package.
	modelEvalMetaResultsStart = "<!-- START MODEL EVAL META RESULTS -->"
	// modelEvalMetaResultsEnd identifies the model eval meta results end constant used by this package.
	modelEvalMetaResultsEnd = "<!-- END MODEL EVAL META RESULTS -->"
	// modelEvalDynamicResultsStart identifies the model eval dynamic results start constant used by this package.
	modelEvalDynamicResultsStart = "<!-- START MODEL EVAL DYNAMIC RESULTS -->"
	// modelEvalDynamicResultsEnd identifies the model eval dynamic results end constant used by this package.
	modelEvalDynamicResultsEnd = "<!-- END MODEL EVAL DYNAMIC RESULTS -->"
	// modelEvalEnterpriseMetaResultsStart identifies the Enterprise meta results start marker.
	modelEvalEnterpriseMetaResultsStart = "<!-- START MODEL EVAL ENTERPRISE META RESULTS -->"
	// modelEvalEnterpriseMetaResultsEnd identifies the Enterprise meta results end marker.
	modelEvalEnterpriseMetaResultsEnd = "<!-- END MODEL EVAL ENTERPRISE META RESULTS -->"
	// modelEvalEnterpriseDynamicResultsStart identifies the Enterprise dynamic results start marker.
	modelEvalEnterpriseDynamicResultsStart = "<!-- START MODEL EVAL ENTERPRISE DYNAMIC RESULTS -->"
	// modelEvalEnterpriseDynamicResultsEnd identifies the Enterprise dynamic results end marker.
	modelEvalEnterpriseDynamicResultsEnd = "<!-- END MODEL EVAL ENTERPRISE DYNAMIC RESULTS -->"
	// modelEvalResultsStart identifies the model eval results start constant used by this package.
	modelEvalResultsStart = "<!-- START MODEL EVAL RESULTS -->"
	// modelEvalResultsEnd identifies the model eval results end constant used by this package.
	modelEvalResultsEnd = "<!-- END MODEL EVAL RESULTS -->"
	// publishSectionMeta identifies the publish section meta constant used by this package.
	publishSectionMeta = "meta"
	// publishSectionDynamic identifies the publish section dynamic constant used by this package.
	publishSectionDynamic = "dynamic"
	// publishSectionEnterpriseMeta identifies the Enterprise meta publication section.
	publishSectionEnterpriseMeta = "enterprise-meta"
	// publishSectionEnterpriseDynamic identifies the Enterprise dynamic publication section.
	publishSectionEnterpriseDynamic = "enterprise-dynamic"
	// publishSectionUnknown identifies the publish section unknown constant used by this package.
	publishSectionUnknown = "unknown"
	// maxPublishTraceLineBytes identifies the max publish trace line bytes constant used by this package.
	maxPublishTraceLineBytes = 64 << 20

	// usageModelRequests identifies the usage model requests constant used by this package.
	usageModelRequests = "Model requests"
	// usageToolCallsEmitted identifies the usage tool calls emitted constant used by this package.
	usageToolCallsEmitted = "Tool calls emitted"
	// usageToolCalls identifies the usage tool calls constant used by this package.
	usageToolCalls = "Tool calls"
	// usageInputTokens identifies the usage input tokens constant used by this package.
	usageInputTokens = "Input tokens"
	// usageOutputTokens identifies the usage output tokens constant used by this package.
	usageOutputTokens = "Output tokens"
	// usageEstimatedCost identifies the usage estimated cost constant used by this package.
	usageEstimatedCost = "Estimated cost"
	// modelEvaluationSuffix identifies the model evaluation suffix constant used by this package.
	modelEvaluationSuffix = " Model Evaluation"
	boldIntFormat         = "**%d**"
	boldStringFormat      = "**%s**"
)

// fullDockerAttemptsByPreset stores the minimum per-model attempts for complete Docker preset reports.
var fullDockerAttemptsByPreset = map[string]int{
	presetDockerRead:                      38,
	presetDockerMutatingSafe:              33,
	presetDockerDestructiveSafe:           63,
	presetDockerEnterpriseRead:            5,
	presetDockerEnterpriseMutatingSafe:    5,
	presetDockerEnterpriseDestructiveSafe: 13,
}

// publishReport captures publish report data for published evaluation reports.
type publishReport struct {
	Path                   string
	Date                   string
	Mode                   string
	Model                  string
	ToolSurface            string
	Edition                string
	Backend                string
	Preset                 string
	ToolExecution          string
	GitBranch              string
	GitCommit              string
	Diagnostics            map[string]int
	UnresolvedHarnessNoise bool
	Rows                   []publishRow
}

// publishRow captures publish row data for published evaluation reports.
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

// publishTaskStats captures publish task stats data for published evaluation reports.
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

// publishModelMetrics captures publish model metrics data for published evaluation reports.
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

// publishModelSummary captures publish model summary data for published evaluation reports.
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

// publishEvaluationDocs publishes evaluation docs for the evaluator package.
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
		sectionLabel := publishSectionLabel(label, section.Key, reports)
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
	terminalPrintf("published evaluation docs: %s, %s\n", opts.PublishResults, opts.PublishReadme)
	return nil
}

// publishDocSectionsForReports returns the managed sections touched by reports.
func publishDocSectionsForReports(reports []publishReport) []publishDocSection {
	keys := map[string]bool{}
	for _, report := range reports {
		if len(report.Rows) == 0 {
			keys[publishSectionForReport(report)] = true
			continue
		}
		for _, row := range report.Rows {
			keys[publishSectionForRow(report, row)] = true
		}
	}
	sections := make([]publishDocSection, 0, len(keys))
	for _, key := range []string{publishSectionMeta, publishSectionDynamic, publishSectionEnterpriseMeta, publishSectionEnterpriseDynamic} {
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
		if len(report.Rows) == 0 && publishSectionForReport(report) == sectionKey {
			filtered = append(filtered, report)
			continue
		}
		sectionRows := make([]publishRow, 0, len(report.Rows))
		for _, row := range report.Rows {
			if publishSectionForRow(report, row) == sectionKey {
				sectionRows = append(sectionRows, row)
			}
		}
		if len(sectionRows) > 0 {
			sectionReport := report
			sectionReport.Rows = sectionRows
			filtered = append(filtered, sectionReport)
		}
	}
	return filtered
}

// publishSectionForReport maps a report tool surface to its publication section.
func publishSectionForReport(report publishReport) string {
	section := publishSectionForSurface(report.ToolSurface)
	if section == publishSectionUnknown {
		return section
	}
	if publishReportEdition(report) == editionEnterprise {
		return enterprisePublishSection(section)
	}
	for _, row := range report.Rows {
		if publishPresetIsEnterprise(row.Preset) {
			return enterprisePublishSection(section)
		}
	}
	return section
}

// publishSectionForRow maps a publish row to its edition-aware section.
func publishSectionForRow(report publishReport, row publishRow) string {
	section := publishSectionForSurface(report.ToolSurface)
	if section == publishSectionUnknown {
		return section
	}
	if rowEdition := publishPresetEdition(row.Preset); rowEdition != "" {
		if rowEdition == editionEnterprise {
			return enterprisePublishSection(section)
		}
		return section
	}
	if publishReportEdition(report) == editionEnterprise {
		return enterprisePublishSection(section)
	}
	return section
}

// publishSectionForSurface maps a tool surface to its base publication section.
func publishSectionForSurface(toolSurface string) string {
	surface := strings.ToLower(strings.TrimSpace(toolSurface))
	switch surface {
	case "", config.ToolSurfaceMeta:
		return publishSectionMeta
	case config.ToolSurfaceDynamic:
		return publishSectionDynamic
	default:
		return publishSectionUnknown
	}
}

// enterprisePublishSection returns the Enterprise variant of a base section.
func enterprisePublishSection(sectionKey string) string {
	switch sectionKey {
	case publishSectionMeta:
		return publishSectionEnterpriseMeta
	case publishSectionDynamic:
		return publishSectionEnterpriseDynamic
	default:
		return sectionKey
	}
}

// publishReportEdition normalizes report edition metadata for publication.
func publishReportEdition(report publishReport) string {
	edition := strings.ToLower(strings.TrimSpace(report.Edition))
	switch edition {
	case editionCE, editionEnterprise:
		return edition
	}
	if publishPresetIsEnterprise(report.Preset) {
		return editionEnterprise
	}
	return editionAll
}

// publishPresetIsEnterprise reports whether a preset belongs to Enterprise output tables.
func publishPresetIsEnterprise(preset string) bool {
	return publishPresetEdition(preset) == editionEnterprise
}

// publishPresetEdition reports the edition represented by a preset or partition.
func publishPresetEdition(preset string) string {
	switch strings.TrimSpace(preset) {
	case presetSchemaEnterprise, presetDockerEnterpriseRead, presetDockerEnterpriseMutatingSafe, presetDockerEnterpriseDestructiveSafe,
		partitionEnterpriseRead, partitionEnterpriseMutating, partitionEnterpriseDestructive:
		return editionEnterprise
	case presetDockerRead, presetDockerMutatingSafe, presetDockerDestructiveSafe, presetDockerCapabilityDiscovery,
		partitionErrorRecovery, partitionCapabilityFallback:
		return editionCE
	default:
		return ""
	}
}

// publishDocSectionForKey returns marker pairs for a publication section.
func publishDocSectionForKey(sectionKey string) publishDocSection {
	switch sectionKey {
	case publishSectionEnterpriseDynamic:
		return publishDocSection{
			Key:                publishSectionEnterpriseDynamic,
			ResultsStartMarker: modelEvalEnterpriseDynamicResultsStart,
			ResultsEndMarker:   modelEvalEnterpriseDynamicResultsEnd,
			SummaryStartMarker: modelEvalEnterpriseDynamicSummaryStart,
			SummaryEndMarker:   modelEvalEnterpriseDynamicSummaryEnd,
		}
	case publishSectionEnterpriseMeta:
		return publishDocSection{
			Key:                publishSectionEnterpriseMeta,
			ResultsStartMarker: modelEvalEnterpriseMetaResultsStart,
			ResultsEndMarker:   modelEvalEnterpriseMetaResultsEnd,
			SummaryStartMarker: modelEvalEnterpriseMetaSummaryStart,
			SummaryEndMarker:   modelEvalEnterpriseMetaSummaryEnd,
		}
	case publishSectionDynamic:
		return publishDocSection{
			Key:                publishSectionDynamic,
			ResultsStartMarker: modelEvalDynamicResultsStart,
			ResultsEndMarker:   modelEvalDynamicResultsEnd,
			SummaryStartMarker: modelEvalDynamicSummaryStart,
			SummaryEndMarker:   modelEvalDynamicSummaryEnd,
		}
	case publishSectionMeta:
		return publishDocSection{
			Key:                publishSectionMeta,
			ResultsStartMarker: modelEvalMetaResultsStart,
			ResultsEndMarker:   modelEvalMetaResultsEnd,
			SummaryStartMarker: modelEvalMetaSummaryStart,
			SummaryEndMarker:   modelEvalMetaSummaryEnd,
		}
	default:
		return publishDocSection{Key: publishSectionUnknown}
	}
}

// publishSectionLabel returns the snapshot heading used within a managed section.
func publishSectionLabel(label, sectionKey string, reports []publishReport) string {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		trimmed = strings.TrimSpace(publishSnapshotLabel("", nil))
	}
	if publishHasSiblingEditionSection(sectionKey, reports) {
		return qualifyPublishSectionLabel(trimmed, sectionKey)
	}
	return trimmed
}

// publishHasSiblingEditionSection reports whether this publish updates CE and Enterprise blocks for the same surface.
func publishHasSiblingEditionSection(sectionKey string, reports []publishReport) bool {
	keys := map[string]bool{}
	for _, section := range publishDocSectionsForReports(reports) {
		keys[section.Key] = true
	}
	sibling := ""
	switch sectionKey {
	case publishSectionMeta:
		sibling = publishSectionEnterpriseMeta
	case publishSectionDynamic:
		sibling = publishSectionEnterpriseDynamic
	case publishSectionEnterpriseMeta:
		sibling = publishSectionMeta
	case publishSectionEnterpriseDynamic:
		sibling = publishSectionDynamic
	}
	return sibling != "" && keys[sectionKey] && keys[sibling]
}

// qualifyPublishSectionLabel makes split CE/Enterprise snapshots distinguishable in published docs.
func qualifyPublishSectionLabel(label, sectionKey string) string {
	ceLabel, enterpriseLabel := "CE", "Enterprise"
	editionLabel := ceLabel
	if sectionKey == publishSectionEnterpriseMeta || sectionKey == publishSectionEnterpriseDynamic {
		editionLabel = enterpriseLabel
	}
	if strings.Contains(label, "CE+Enterprise-on-Enterprise") {
		replacement := "CE-on-Enterprise"
		if editionLabel == enterpriseLabel {
			replacement = "Enterprise"
		}
		return strings.TrimSpace(strings.Replace(strings.Replace(label, "CE+Enterprise-on-Enterprise", replacement, 1), " combined", "", 1))
	}
	if strings.Contains(label, "CE+Enterprise") {
		return strings.TrimSpace(strings.Replace(strings.Replace(label, "CE+Enterprise", editionLabel, 1), " combined", "", 1))
	}
	return label + " (" + editionLabel + ")"
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
		return publishReport{}, fmt.Errorf("publish input %s must be an eval_mcp_surfaces evaluation report", path)
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
		Edition:                input.Edition,
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

// publishRowsForReport publishes rows for report and returns [[]publishRow].
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

// shouldSplitPublishReportByPreset reports whether should split publish report by preset.
func shouldSplitPublishReportByPreset(report publishReport) bool {
	if strings.TrimSpace(report.Preset) != "" {
		return false
	}
	return report.Backend == backendGitLab && report.ToolExecution == "mcp"
}

// publishRowsByPresetFromTraces publishes rows by preset from traces and returns [[]publishRow].
func publishRowsByPresetFromTraces(report publishReport, content string) ([]publishRow, error) {
	tracePath := publishTraceJSONLPath(report.Path, content)
	if tracePath == "" {
		return nil, fmt.Errorf("publish input %s has no preset and no trace artifacts; publish full runs with trace artifacts or publish separate preset reports", report.Path)
	}
	tasks := evalTasksFromCases(AllEvalCases())
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
	scanner.Buffer(make([]byte, 64*1024), maxPublishTraceLineBytes)
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

// addTrace handles add trace for publishTraceAccumulator.
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

// publishEffectiveTraceOutcome publishes effective trace outcome for the evaluator package.
func publishEffectiveTraceOutcome(trace taskTrace, _ string) (toolOK, actionOK, firstPassOK bool) {
	if len(trace.Expected) == 0 {
		return false, false, false
	}
	for _, expected := range publishFirstOutcomeCandidateSteps(trace.Expected) {
		if trace.Summary.FirstTool != expected.Tool {
			continue
		}
		return true, trace.Summary.FirstAction == expected.Action, trace.Summary.FirstPass
	}
	first := trace.Expected[0]
	return trace.Summary.FirstTool == first.Tool, trace.Summary.FirstAction == first.Action, trace.Summary.FirstPass
}

func publishFirstOutcomeCandidateSteps(steps []traceExpectedStep) []traceExpectedStep {
	candidates := make([]traceExpectedStep, 0, len(steps))
	for _, step := range steps {
		candidates = append(candidates, step)
		if !step.OptionalStep {
			break
		}
	}
	return candidates
}

// metrics handles metrics for publishTraceAccumulator.
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

// usage handles usage for publishTraceAccumulator.
func (a *publishTraceAccumulator) usage() map[string]string {
	return map[string]string{
		usageModelRequests:    strconv.Itoa(a.Stats.ModelRequests),
		usageToolCallsEmitted: strconv.Itoa(a.Stats.ToolCalls),
		usageInputTokens:      strconv.Itoa(a.InputTokens),
		usageOutputTokens:     strconv.Itoa(a.OutputTokens),
	}
}

// publishPresetForTask publishes preset for task for the evaluator package.
func publishPresetForTask(task evalTask) string {
	for _, preset := range []string{presetDockerRead, presetDockerMutatingSafe, presetDockerDestructiveSafe, presetDockerEnterpriseRead, presetDockerEnterpriseMutatingSafe, presetDockerEnterpriseDestructiveSafe, presetSchemaEnterprise} {
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

// publishTraceJSONLPath publishes trace jsonl path for the evaluator package.
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

// publishSingleTaskStats publishes single task stats for the evaluator package.
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

// metricsFromComparison computes from comparison from comparison data.
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

// publishSingleUsage publishes single usage for the evaluator package.
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

// newPublishRow constructs publish row.
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

// publishTaskStatsByModel publishes task stats by model for the evaluator package.
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

// publishMetricsByModel publishes metrics by model for the evaluator package.
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

// publishUsageByModel publishes usage by model for the evaluator package.
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

// reportNamedTableRows extracts named table rows from generated reports.
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

// reportTableRowsForHeading extracts table rows for heading from generated reports.
func reportTableRowsForHeading(content, heading string) [][]string {
	var rows [][]string
	for _, line := range sectionLinesForHeading(strings.Split(content, "\n"), heading) {
		rows = appendReportTableRow(rows, line)
	}
	return rows
}

// sectionLinesForHeading extracts lines for heading from a managed Markdown section.
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

// markdownHeadingLevel marks down heading level for the evaluator package.
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

// parseExpectedOps parses expected ops from evaluator input.
func parseExpectedOps(value string) int {
	value = cleanReportValue(value)
	if _, after, ok := strings.Cut(value, "/"); ok {
		return parseReportInt(after)
	}
	return parseReportInt(value)
}

// publishCostTokens publishes cost tokens for the evaluator package.
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

// validatePublishReports validates publish reports for the evaluator package.
func validatePublishReports(reports []publishReport, label string, allowHarnessNoise bool) error {
	labelLower := strings.ToLower(label)
	for _, report := range reports {
		if publishSectionForSurface(report.ToolSurface) == publishSectionUnknown {
			return fmt.Errorf("publish input %s uses unsupported tool_surface %q", report.Path, report.ToolSurface)
		}
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

// reportMentionsHarnessNoise reports whether report mentions harness noise.
func reportMentionsHarnessNoise(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "harness noise") || strings.Contains(lower, "harness_noise")
}

// publishSnapshotLabel publishes snapshot label for the evaluator package.
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
	b.WriteString(renderModelResultsTable(rows, aggregate))
	fmt.Fprintf(&b, "\nPublished with `cmd/eval_mcp_surfaces --publish-docs` from reviewed Markdown reports. Raw traces and JSON artifacts are not included here.\n")
	return strings.TrimSpace(b.String()) + "\n"
}

func renderModelResultsTable(rows []publishRow, aggregate publishRow) string {
	tableRows := make([][]string, 0, len(rows)+1)
	for _, row := range rows {
		tableRows = append(tableRows, []string{
			fmt.Sprintf("`%s`", escapeTable(row.Model)),
			fmt.Sprintf("`%s`", emptyDash(row.Preset)),
			dockerBackendLabel(row),
			strconv.Itoa(row.Attempts),
			strconv.Itoa(row.ExpectedOps),
			strconv.Itoa(row.ModelRequests),
			strconv.Itoa(row.ToolCalls),
			formatMetric(row.ToolSelection),
			formatMetric(row.ActionSelection),
			formatMetric(row.FirstPass),
			formatRepairMetric(row),
			formatMetric(row.DestructiveSafety),
			formatMetric(row.FinalSuccess),
			emptyDash(row.CostTokens),
			emptyDash(rowCommitBranchDate(row)),
		})
	}
	tableRows = append(tableRows, []string{
		"**Aggregate**",
		"**all selected**",
		"-",
		fmt.Sprintf(boldIntFormat, aggregate.Attempts),
		fmt.Sprintf(boldIntFormat, aggregate.ExpectedOps),
		fmt.Sprintf(boldIntFormat, aggregate.ModelRequests),
		fmt.Sprintf(boldIntFormat, aggregate.ToolCalls),
		fmt.Sprintf(boldStringFormat, formatMetric(aggregate.ToolSelection)),
		fmt.Sprintf(boldStringFormat, formatMetric(aggregate.ActionSelection)),
		fmt.Sprintf(boldStringFormat, formatMetric(aggregate.FirstPass)),
		fmt.Sprintf(boldStringFormat, formatRepairMetric(aggregate)),
		fmt.Sprintf(boldStringFormat, formatMetric(aggregate.DestructiveSafety)),
		fmt.Sprintf(boldStringFormat, formatMetric(aggregate.FinalSuccess)),
		"-",
		"-",
	})

	return docgen.RenderMarkdownTable(
		[]string{"Model", "Preset", "Backend", "Attempts", "Expected ops", usageModelRequests, usageToolCallsEmitted, "Tool-selection", "Action-selection", "First-pass validation", "Repair success", "Destructive safety", "Final task success", "Cost/tokens", "Commit / branch / date"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignLeft, docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignRight, docgen.AlignLeft, docgen.AlignLeft},
		tableRows,
	)
}

// buildReadmeSummaryBlock constructs the request parameters from the input.
func buildReadmeSummaryBlock(label string, reports []publishReport) string {
	rows := sortedPublishRows(reports)
	summaries := publishSummariesByModel(rows)
	aggregate := aggregatePublishRows(rows)
	var b strings.Builder
	fmt.Fprintf(&b, "Current published result: **%s**.\n\n", label)
	b.WriteString(renderReadmeSummaryTable(summaries))
	fmt.Fprintf(&b, "\nThe published model-evaluation set covers %d task attempts and %d expected MCP operations. Across the selected reports, models emitted %d tool calls over %d model requests, with %s aggregate final success. See [AI Model Evaluation Results](docs/testing/model-results.md) for the detailed current matrix.\n",
		aggregate.Attempts, aggregate.ExpectedOps, aggregate.ToolCalls, aggregate.ModelRequests, formatMetric(aggregate.FinalSuccess))
	return strings.TrimSpace(b.String()) + "\n"
}

func renderReadmeSummaryTable(summaries []publishModelSummary) string {
	rows := make([][]string, 0, len(summaries))
	for _, summary := range summaries {
		provider, model := providerModel(summary.Model)
		rows = append(rows, []string{
			escapeTable(provider),
			fmt.Sprintf("`%s`", escapeTable(model)),
			compatibilityLabel(summary),
			formatMetric(summary.ToolSelection),
			formatRecoverySummary(summary),
			dockerLiveStatus(summary),
		})
	}
	return docgen.RenderMarkdownTable(
		[]string{"Provider", "Model", "Compatibility", "Tool accuracy", "Recovery", "Docker live status"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignLeft, docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight, docgen.AlignLeft},
		rows,
	)
}

// sortedPublishRows sorts publish rows deterministically.
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

// publishRowKey publishes row key for the evaluator package.
func publishRowKey(row publishRow) string {
	return strings.Join([]string{row.Model, row.Preset, row.Backend, row.ToolExecution}, "\x00")
}

// presetRank formats preset rank for report output.
func presetRank(preset string) int {
	switch preset {
	case presetDockerRead:
		return 1
	case presetDockerMutatingSafe:
		return 2
	case presetDockerDestructiveSafe:
		return 3
	case presetDockerEnterpriseRead:
		return 4
	case presetDockerEnterpriseMutatingSafe:
		return 5
	case presetDockerEnterpriseDestructiveSafe:
		return 6
	case partitionErrorRecovery:
		return 7
	case partitionCapabilityFallback:
		return 8
	case presetSchemaEnterprise:
		return 9
	default:
		return 99
	}
}

// aggregatePublishRows aggregates publish rows across reports.
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

// publishSummariesByModel publishes summaries by model for the evaluator package.
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

// providerModel prepares provider model for model-provider evaluation.
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

// compatibilityLabel formats compatibility label for report output.
func compatibilityLabel(summary publishModelSummary) string {
	if summary.ToolSelection == 100 && summary.ActionSelection == 100 && summary.FinalSuccess == 100 {
		return "OK"
	}
	return "Review"
}

// dockerLiveStatus formats docker live status for report output.
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

// dockerBackendLabel formats docker backend label for report output.
func dockerBackendLabel(row publishRow) string {
	if row.Backend == backendGitLab && row.ToolExecution == "mcp" {
		return "Docker GitLab via MCP"
	}
	if row.Backend == "" {
		return "-"
	}
	return escapeTable(row.Backend)
}

// rowCommitBranchDate formats row commit branch date for report output.
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

// updateManagedDoc updates managed doc for the evaluator package.
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
		terminalPrintf("published evaluation docs unchanged: %s\n", path)
		return nil
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o750); mkdirErr != nil {
		return fmt.Errorf("create publish doc directory: %w", mkdirErr)
	}
	if writeErr := os.WriteFile(path, []byte(updated), 0o644); writeErr != nil { // #nosec G306 -- tracked Markdown docs should remain world-readable.
		return fmt.Errorf("write publish doc %s: %w", path, writeErr)
	}
	terminalPrintf("updated evaluation docs: %s\n", path)
	return nil
}

// checkManagedDoc checks managed doc for the evaluator package.
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
	terminalPrintf("evaluation docs up to date: %s\n", path)
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

// appendSnapshotBlock appends snapshot block to the output builder.
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

// firstPositive returns the first positive value that is set.
func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// currentGitReportMetadata collects current git report metadata metadata.
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
