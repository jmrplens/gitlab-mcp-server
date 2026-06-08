package evaluator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	maxUncoveredRoutesInReport = 200
	accessRequestedInactive    = "requested but not active"
)

func shouldWriteStartupReport(opts options) bool {
	return opts.Output != "" && !opts.FixturesOnly
}

func writeStartupReport(path string, opts options) error {
	return writeStatusReport(path, opts, "running", "The evaluator created this placeholder at startup. It will be replaced by the final metrics report when the run completes.", nil)
}

func writeErrorReport(path string, opts options, runErr error) error {
	return writeStatusReport(path, opts, "failed", "The evaluator stopped before it could write the final metrics report.", runErr)
}

func writeStatusReport(path string, opts options, status, message string, runErr error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	var b strings.Builder
	writeReportHeader(&b, opts, opts.DryRun)
	fmt.Fprintf(&b, "Status: `%s`\n", status)
	if opts.TraceDir != "" && !opts.DryRun {
		fmt.Fprintf(&b, "Trace artifacts: `%s`\n", opts.TraceDir)
	}
	fmt.Fprintf(&b, "\n## Status\n\n%s\n", message)
	if runErr != nil {
		fmt.Fprintf(&b, "\n## Error\n\n")
		for line := range strings.SplitSeq(runErr.Error(), "\n") {
			fmt.Fprintf(&b, "    %s\n", line)
		}
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write status report: %w", err)
	}
	return nil
}

func writeReportHeader(b *strings.Builder, opts options, dryRun bool) {
	fmt.Fprintf(b, "# %s\n\n", reportTitle(opts.ToolSurface))
	fmt.Fprintf(b, "Date: %s\n", time.Now().UTC().Format(time.RFC3339))
	if branch, commit := currentGitReportMetadata(); branch != "" || commit != "" {
		if branch != "" {
			fmt.Fprintf(b, "Git branch: `%s`\n", branch)
		}
		if commit != "" {
			fmt.Fprintf(b, "Git commit: `%s`\n", commit)
		}
	}
	fmt.Fprintf(b, "Mode: %s\n", reportMode(dryRun))
	fmt.Fprintf(b, "Model: `%s`\n", opts.Model)
	fmt.Fprintf(b, "Tool surface: `%s`\n", opts.ToolSurface)
	if opts.Edition != "" && opts.Edition != editionAll {
		fmt.Fprintf(b, "Edition: `%s`\n", opts.Edition)
	}
	fmt.Fprintf(b, "Backend: `%s`\n", normalizedBackend(opts.Backend))
	if opts.TerminalLog != "" {
		fmt.Fprintf(b, "Terminal output: `%s`\n", opts.TerminalLog)
	}
	if opts.Preset != "" {
		fmt.Fprintf(b, "Preset: `%s`\n", opts.Preset)
	}
	fmt.Fprintf(b, "Tool execution: `%s`\n", toolExecutionMode(opts))
	if opts.ToolsFile != "" {
		fmt.Fprintf(b, "Tools file: `%s`\n", opts.ToolsFile)
	}
	if opts.Partition != "" {
		fmt.Fprintf(b, "Partition: `%s`\n", opts.Partition)
	}
	capabilityAccess := "disabled"
	resourceAccess := "disabled"
	promptAccess := "disabled"
	completionAccess := "disabled"
	if opts.ExposeResources {
		capabilityAccess = accessRequestedInactive
		resourceAccess = accessRequestedInactive
		promptAccess = accessRequestedInactive
		completionAccess = accessRequestedInactive
		if opts.CapabilityAccessActive {
			capabilityAccess = "enabled"
		}
		if opts.ResourceAccessActive {
			resourceAccess = "enabled"
		}
		if opts.PromptAccessActive {
			promptAccess = "enabled"
		}
		if opts.CompletionAccessActive {
			completionAccess = "enabled"
		}
	}
	fmt.Fprintf(b, "MCP capability bridge: `%s`\n", capabilityAccess)
	fmt.Fprintf(b, "Resource access: `%s`\n", resourceAccess)
	fmt.Fprintf(b, "Prompt access: `%s`\n", promptAccess)
	fmt.Fprintf(b, "Completion access: `%s`\n", completionAccess)
}

func reportTitle(toolSurface string) string {
	switch toolSurface {
	case config.ToolSurfaceDynamic:
		return "Dynamic Surface Model Evaluation"
	case config.ToolSurfaceMeta:
		return "Meta-Tool Model Evaluation"
	default:
		return "MCP Surface Model Evaluation"
	}
}

// reportMode extracts mode from generated reports.
func reportMode(dryRun bool) string {
	if dryRun {
		return "static route/schema validation"
	}
	return "model tool-calling"
}

func writeReport(path string, opts options, results []taskResult, catalog []modelTool, routes map[string]toolutil.ActionMap, dryRun bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	var b strings.Builder
	metrics := calculateMetrics(results)
	writeReportHeader(&b, opts, dryRun)
	fmt.Fprintf(&b, "Catalog tools: %d\n", len(catalog))
	fmt.Fprintf(&b, "Runs: %d\n", opts.Repeat)
	fmt.Fprintf(&b, "Task attempts: %d\n\n", len(results))
	if dryRun {
		fmt.Fprintf(&b, "Schema-only validation accepts a task when the expected tool/action and required parameter shape are present in the selected catalog. No live GitLab entitlement or Docker execution is required.\n\n")
	}
	if opts.TraceDir != "" && !dryRun {
		fmt.Fprintf(&b, "Trace artifacts: `%s`\n\n", opts.TraceDir)
	}
	fmt.Fprintf(&b, "## Metrics\n\n")
	b.WriteString(metricValueTableHeader)
	fmt.Fprintf(&b, "| Tool-selection accuracy | %.1f%% |\n", metrics.ToolSelection)
	fmt.Fprintf(&b, "| Action-selection accuracy | %.1f%% |\n", metrics.ActionSelection)
	fmt.Fprintf(&b, "| First-call validation pass rate | %.1f%% |\n", metrics.FirstPass)
	fmt.Fprintf(&b, "| Schema lookup use rate | %.1f%% |\n", metrics.SchemaLookup)
	fmt.Fprintf(&b, "| Resource lookup use rate | %.1f%% |\n", metrics.ResourceLookup)
	fmt.Fprintf(&b, "| MCP capability bridge use rate | %.1f%% |\n", metrics.CapabilityLookup)
	fmt.Fprintf(&b, "| Repair success rate | %.1f%% |\n", metrics.RepairSuccess)
	fmt.Fprintf(&b, "| Destructive safety | %.1f%% |\n", metrics.DestructiveSafety)
	fmt.Fprintf(&b, "| Final task success proxy | %.1f%% |\n", metrics.FinalSuccess)
	writePerModelMetrics(&b, results)
	if opts.Repeat > 1 {
		writePerRunMetrics(&b, results)
	}
	writeUsageSummary(&b, opts, results, dryRun)
	writeCapabilityBridgeUsage(&b, results, dryRun)
	writeFailureDiagnostics(&b, opts, results)
	writeRepairDiagnostics(&b, opts, results)
	writeFixtureCoverage(&b, catalog, results, routes)
	fmt.Fprintf(&b, "\n## Task Results\n\n")
	includeModel := resultsHaveMultipleModels(results)
	if includeModel {
		fmt.Fprintf(&b, "| Model | Run | Task | Expected | First final call | Steps | Schema lookup | Resource lookup | MCP bridge | First pass | Repair | Final success | Calls | Tool calls | Resource calls | MCP bridge calls | Notes |\n")
		fmt.Fprintf(&b, "| --- | ---: | --- | --- | --- | ---: | --- | --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | --- |\n")
	} else {
		fmt.Fprintf(&b, "| Run | Task | Expected | First final call | Steps | Schema lookup | Resource lookup | MCP bridge | First pass | Repair | Final success | Calls | Tool calls | Resource calls | MCP bridge calls | Notes |\n")
		fmt.Fprintf(&b, "| ---: | --- | --- | --- | ---: | --- | --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | --- |\n")
	}
	for _, result := range results {
		_, _, effectiveFirstPass := effectiveFirstOutcome(result)
		notes := strings.Join(result.Notes, "; ")
		if notes == "" {
			notes = "-"
		}
		repair := "-"
		if result.RepairAttempted {
			repair = boolText(result.RepairSuccess)
		}
		if includeModel {
			fmt.Fprintf(&b, "| `%s` | %d | %s | %s | %s | %d/%d | %s | %s | %s | %s | %s | %s | %d | %d | %d | %d | %s |\n",
				escapeTable(result.Model), result.Run, result.Task.ID, escapeTable(expectedDisplay(result.Task)), escapeTable(stepDisplay(result.FirstTool, result.FirstAction)), result.CompletedSteps, len(taskSteps(result.Task)), boolText(result.SchemaLookupUsed), boolText(result.ResourceLookupUsed), boolText(result.CapabilityLookupUsed), boolText(effectiveFirstPass), repair, boolText(result.FinalSuccess), result.ModelCalls, result.ToolCalls, result.ResourceCalls, result.CapabilityCalls, escapeTable(notes))
		} else {
			fmt.Fprintf(&b, "| %d | %s | %s | %s | %d/%d | %s | %s | %s | %s | %s | %s | %d | %d | %d | %d | %s |\n",
				result.Run, result.Task.ID, escapeTable(expectedDisplay(result.Task)), escapeTable(stepDisplay(result.FirstTool, result.FirstAction)), result.CompletedSteps, len(taskSteps(result.Task)), boolText(result.SchemaLookupUsed), boolText(result.ResourceLookupUsed), boolText(result.CapabilityLookupUsed), boolText(effectiveFirstPass), repair, boolText(result.FinalSuccess), result.ModelCalls, result.ToolCalls, result.ResourceCalls, result.CapabilityCalls, escapeTable(notes))
		}
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	terminalPrintf("wrote evaluation report: %s\n", path)
	return nil
}

func writeFailureDiagnostics(b *strings.Builder, opts options, results []taskResult) {
	counts := make(map[string]int)
	examples := make(map[string]string)
	for _, result := range results {
		if result.FinalSuccess && result.DestructiveSafe {
			continue
		}
		category := failureDiagnosticCategoryForResult(opts, result)
		if result.FinalSuccess && !result.DestructiveSafe {
			category = "destructive_safety"
		}
		counts[category]++
		if examples[category] == "" {
			examples[category] = result.Task.ID
		}
	}
	if len(counts) == 0 {
		return
	}

	title := "Failure Diagnostics"
	if opts.Execute {
		title = "Docker Live Failure Triage"
	}
	fmt.Fprintf(b, "\n## %s\n\n", title)
	fmt.Fprintf(b, "| Category | Count | Example task |\n| --- | ---: | --- |\n")
	for _, category := range failureDiagnosticCategories(opts) {
		count := counts[category]
		if count == 0 {
			continue
		}
		fmt.Fprintf(b, "| %s | %d | %s |\n", category, count, examples[category])
	}
}

func writeRepairDiagnostics(b *strings.Builder, opts options, results []taskResult) {
	counts := make(map[string]int)
	examples := make(map[string]string)
	for _, result := range results {
		if !result.RepairAttempted || !result.RepairSuccess || !result.FinalSuccess {
			continue
		}
		category := failureDiagnosticCategoryForResult(opts, result)
		counts[category]++
		if examples[category] == "" {
			examples[category] = result.Task.ID
		}
	}
	if len(counts) == 0 {
		return
	}

	fmt.Fprintf(b, "\n## Repaired First-Pass Diagnostics\n\n")
	fmt.Fprintf(b, "| Category | Count | Example task |\n| --- | ---: | --- |\n")
	for _, category := range failureDiagnosticCategories(opts) {
		count := counts[category]
		if count == 0 {
			continue
		}
		fmt.Fprintf(b, "| %s | %d | %s |\n", category, count, examples[category])
	}
}

// failureDiagnosticCategories returns the ordered report categories for the
// selected tool surface.
func failureDiagnosticCategories(opts options) []string {
	if isDynamicEvalSurface(opts.ToolSurface) {
		return []string{"ranker_miss", "alias_miss", "standalone_unavailable", "params_shape_miss", "multi_step_order_miss", "ce_or_sampling_limitation", "true_discovery_miss", "mcp_implementation_bug", "model_provider_auth", "model_provider_model_unavailable", "transient_gitlab_5xx", "timeout_resource_exhaustion", "destructive_safety", "not_found", "other"}
	}
	return []string{"mcp_implementation_bug", "gitlab_ce_limitation", "model_provider_auth", "model_provider_model_unavailable", "model_route_selection_miss", "model_parameter_shape_miss", "fixture_setup_failure", "transient_gitlab_5xx", "timeout_resource_exhaustion", "destructive_safety", "not_found", "other"}
}

// failureDiagnosticCategoryForResult classifies a failed task result for the
// selected tool surface.
func failureDiagnosticCategoryForResult(opts options, result taskResult) string {
	if isDynamicEvalSurface(opts.ToolSurface) {
		return dynamicFailureDiagnosticCategory(result)
	}
	return failureDiagnosticCategory(result.Notes)
}

func commonFailureCategory(text string) string {
	switch {
	case providerAuthFailure(text):
		return "model_provider_auth"
	case providerModelUnavailable(text):
		return "model_provider_model_unavailable"
	case implementationBugFailure(text):
		return "mcp_implementation_bug"
	case transientGitLabFailure(text):
		return "transient_gitlab_5xx"
	case resourceExhaustionFailure(text):
		return "timeout_resource_exhaustion"
	case notFoundFailure(text):
		return "not_found"
	default:
		return ""
	}
}

func providerAuthFailure(text string) bool {
	return strings.Contains(text, "invalid_api_key") || strings.Contains(text, "incorrect api key") || strings.Contains(text, "api key") && strings.Contains(text, "invalid")
}

func providerModelUnavailable(text string) bool {
	return strings.Contains(text, "not_found_error") && strings.Contains(text, "model") || strings.Contains(text, "model is not found") || strings.Contains(text, "models/") && strings.Contains(text, diagnosticNotFound)
}

func implementationBugFailure(text string) bool {
	return strings.Contains(text, "int64") || strings.Contains(text, "cannot unmarshal") || strings.Contains(text, "integer") && strings.Contains(text, "invalid")
}

func transientGitLabFailure(text string) bool {
	return textContainsAny(text, "500", "502", "503", "504", "internal server error", "bad gateway", "service unavailable", "gateway timeout")
}

func resourceExhaustionFailure(text string) bool {
	return textContainsAny(text, "timeout", "deadline exceeded", "resource exhausted", "too many requests", "429")
}

func notFoundFailure(text string) bool {
	return strings.Contains(text, "404") || strings.Contains(text, diagnosticNotFound)
}

// dynamicFailureDiagnosticCategory separates dynamic-mode failures into buckets
// that map directly to follow-up implementation work.
func dynamicFailureDiagnosticCategory(result taskResult) string {
	text := strings.ToLower(strings.Join(result.Notes, "\n"))
	if category := commonFailureCategory(text); category != "" {
		return category
	}
	switch {
	case text == "":
		return "other"
	case limitedByEditionOrSampling(text):
		return "ce_or_sampling_limitation"
	case standaloneUnavailable(text):
		return "standalone_unavailable"
	case dynamicRankerMiss(text):
		return "ranker_miss"
	case dynamicAliasMiss(text):
		return "alias_miss"
	case parameterShapeFailure(text):
		return "params_shape_miss"
	case dynamicMultiStepOrderMiss(text):
		return "multi_step_order_miss"
	case discoveryFailure(text):
		return "true_discovery_miss"
	case destructiveSafetyFailure(text):
		return "destructive_safety"
	default:
		return "other"
	}
}

func limitedByEditionOrSampling(text string) bool {
	return strings.Contains(text, "sampling_unsupported") || strings.Contains(text, "sampling capability unsupported") || editionLimitation(text)
}

func editionLimitation(text string) bool {
	return strings.Contains(text, "ce") && (strings.Contains(text, "unavailable") || strings.Contains(text, "unsupported")) || textContainsAny(text, "requires premium", "requires ultimate", "license", "not available")
}

func standaloneUnavailable(text string) bool {
	return textContainsAny(text, "expected tool gitlab_discover_project", "expected tool gitlab_interactive_", "standalone tool")
}

func parameterShapeFailure(text string) bool {
	return textContainsAny(text, diagnosticMissingRequiredParams, diagnosticMissingRequiredStandalone, diagnosticUnknownParams, diagnosticUnexpectedTopLevelParameter)
}

func discoveryFailure(text string) bool {
	return textContainsAny(text, diagnosticExpectedAction, "expected tool", "unknown action", "model returned no tool_use")
}

func destructiveSafetyFailure(text string) bool {
	return strings.Contains(text, "confirm:true") || strings.Contains(text, "destructive")
}

// dynamicRankerMiss reports whether a failure points to dynamic search ranking.
func dynamicRankerMiss(text string) bool {
	if strings.Contains(text, "dynamic ranker miss") || strings.Contains(text, "ranker miss") {
		return true
	}
	if !strings.Contains(text, "search corpus") {
		return false
	}
	for _, marker := range []string{"ranker", "expected top action", "no results", "no hits", "no matches"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// dynamicAliasMiss classifies dynamic alias miss for evaluation diagnostics.
func dynamicAliasMiss(text string) bool {
	if !strings.Contains(text, diagnosticExpectedAction) || !strings.Contains(text, "got ") {
		return false
	}
	aliasMarkers := []string{
		"repository_file.", "project_access_token.", "gitlab_server.", "deploy_key.",
		"webhook.", "badge.", "broadcast_message.", "feature_flag.",
		"group.custom_member_roles_", "merge_train.list", "project.schedule_storage_move",
		"personal_snippet.", "runner.delete", "ci_catalog.", "enterprise_user.",
	}
	for _, marker := range aliasMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// dynamicMultiStepOrderMiss identifies runs that exhausted the dynamic task budget after partial progress.
func dynamicMultiStepOrderMiss(text string) bool {
	if !strings.Contains(text, "tool-call step limit reached after") {
		return false
	}
	return !strings.Contains(text, "after 0/")
}

func failureDiagnosticCategory(notes []string) string {
	text := strings.ToLower(strings.Join(notes, "\n"))
	if category := commonFailureCategory(text); category != "" {
		return category
	}
	switch {
	case editionLimitation(text):
		return "gitlab_ce_limitation"
	case textContainsAny(text, "fixture unavailable", "fixture state", "prepare fixtures"):
		return "fixture_setup_failure"
	case strings.Contains(text, diagnosticExpectedAction) || strings.Contains(text, "expected tool"):
		return "model_route_selection_miss"
	case parameterShapeFailure(text) || strings.Contains(text, "standalone tool uses top-level"):
		return "model_parameter_shape_miss"
	case destructiveSafetyFailure(text):
		return "destructive_safety"
	default:
		return "other"
	}
}

func textContainsAny(text string, substrs ...string) bool {
	for _, substr := range substrs {
		if strings.Contains(text, substr) {
			return true
		}
	}
	return false
}

func writeTraceArtifacts(dir string, results []taskResult, traceProviderBodies bool) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create trace directory: %w", err)
	}

	var index strings.Builder
	var jsonl strings.Builder
	fmt.Fprintf(&index, "# MCP Surface Evaluation Traces\n\n")
	providerTraceDescription := "provider HTTP exchange metadata"
	if traceProviderBodies {
		providerTraceDescription = "provider HTTP request/response bodies"
	}
	fmt.Fprintf(&index, "Each JSON file records the exact task prompt, expected route sequence, %s, assistant tool calls, MCP CallTool request/response payloads, simulated tool results, validation messages, and final summary for one model-backed evaluation attempt. Provider authentication headers are not serialized. Raw provider bodies are included only when `--trace-provider-bodies` is set. `traces.jsonl` contains the same records as one JSON object per line for batch analysis.\n\n", providerTraceDescription)
	fmt.Fprintf(&index, "| Model | Run | Task | Final success | First pass | Trace file |\n")
	fmt.Fprintf(&index, "| --- | ---: | --- | --- | --- | --- |\n")

	for _, result := range results {
		trace := result.Trace
		if trace.TaskID == "" {
			continue
		}
		fileName := traceFileName(trace)
		data, err := json.MarshalIndent(trace, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal trace %s: %w", trace.TaskID, err)
		}
		if writeErr := os.WriteFile(filepath.Join(dir, fileName), data, 0o600); writeErr != nil {
			return fmt.Errorf("write trace %s: %w", trace.TaskID, writeErr)
		}
		line, err := json.Marshal(trace)
		if err != nil {
			return fmt.Errorf("marshal trace jsonl %s: %w", trace.TaskID, err)
		}
		jsonl.Write(line)
		jsonl.WriteByte('\n')
		fmt.Fprintf(
			&index, "| `%s` | %d | %s | %s | %s | [%s](%s) |\n",
			escapeTable(trace.Model),
			trace.Run,
			trace.TaskID,
			boolText(trace.Summary.FinalSuccess),
			boolText(trace.Summary.FirstPass),
			fileName,
			fileName,
		)
	}

	if err := os.WriteFile(filepath.Join(dir, "traces.jsonl"), []byte(jsonl.String()), 0o600); err != nil {
		return fmt.Errorf("write traces jsonl: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(index.String()), 0o600); err != nil {
		return fmt.Errorf("write trace index: %w", err)
	}
	terminalPrintf("wrote evaluation traces: %s\n", dir)
	return nil
}

func traceFileName(trace taskTrace) string {
	taskID := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(trace.TaskID)
	model := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-").Replace(trace.Model)
	if model == "" {
		return fmt.Sprintf("run-%03d-%s.json", trace.Run, taskID)
	}
	return fmt.Sprintf("%s-run-%03d-%s.json", model, trace.Run, taskID)
}

func writeFixtureCoverage(b *strings.Builder, catalog []modelTool, results []taskResult, routes map[string]toolutil.ActionMap) {
	summary := fixtureToolCoverage(catalog, results)
	actionSummary := fixtureActionCoverage(routes, results)
	fmt.Fprintf(b, "\n## Fixture Tool Coverage\n\n")
	b.WriteString(metricValueTableHeader)
	fmt.Fprintf(b, "| Catalog tools | %d |\n", summary.Total)
	fmt.Fprintf(b, "| Tools covered by expected steps | %d |\n", summary.Covered)
	fmt.Fprintf(b, "| Missing tools | %d |\n", len(summary.Missing))
	fmt.Fprintf(b, "| Catalog action routes | %d |\n", actionSummary.Total)
	fmt.Fprintf(b, "| Action routes covered by expected steps | %d |\n", actionSummary.Covered)
	fmt.Fprintf(b, "| Missing action routes | %d |\n", len(actionSummary.Missing))
	if len(summary.Missing) > 0 {
		fmt.Fprintf(b, "\nMissing: `%s`\n", strings.Join(summary.Missing, "`, `"))
	}
	if len(actionSummary.Missing) > 0 && len(actionSummary.Missing) <= 40 {
		fmt.Fprintf(b, "\nMissing action routes: `%s`\n", strings.Join(actionSummary.Missing, "`, `"))
	}
}

// fixtureCoverage captures fixture coverage data for live evaluation fixtures.
type fixtureCoverage struct {
	Total   int
	Covered int
	Missing []string
}

func fixtureToolCoverage(catalog []modelTool, results []taskResult) fixtureCoverage {
	catalogNames := make([]string, 0, len(catalog))
	for _, tool := range catalog {
		catalogNames = append(catalogNames, tool.Name)
	}
	sort.Strings(catalogNames)
	covered := map[string]bool{}
	for _, result := range results {
		for _, step := range taskSteps(result.Task) {
			covered[step.ExpectedTool] = true
		}
	}
	var missing []string
	for _, name := range catalogNames {
		if !covered[name] {
			missing = append(missing, name)
		}
	}
	return fixtureCoverage{Total: len(catalogNames), Covered: len(catalogNames) - len(missing), Missing: missing}
}

func fixtureActionCoverage(routes map[string]toolutil.ActionMap, results []taskResult) fixtureCoverage {
	if len(routes) == 0 {
		return fixtureCoverage{}
	}
	all := make([]string, 0)
	for tool, actions := range routes {
		for action := range actions {
			all = append(all, tool+"/"+action)
		}
	}
	sort.Strings(all)
	covered := map[string]bool{}
	for _, result := range results {
		for _, step := range taskSteps(result.Task) {
			if step.ExpectedAction != "" {
				covered[step.ExpectedTool+"/"+step.ExpectedAction] = true
			}
		}
	}
	var missing []string
	for _, name := range all {
		if !covered[name] {
			missing = append(missing, name)
		}
	}
	return fixtureCoverage{Total: len(all), Covered: len(all) - len(missing), Missing: missing}
}

func writeCoverageReportIfRequested(opts options, results []taskResult, routes map[string]toolutil.ActionMap) error {
	if opts.CoverageReport == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(opts.CoverageReport), 0o750); err != nil {
		return fmt.Errorf("create coverage report directory: %w", err)
	}
	report := buildRouteCoverageReport(opts, results, routes)
	if err := os.WriteFile(opts.CoverageReport, []byte(report), 0o600); err != nil {
		return fmt.Errorf("write coverage report: %w", err)
	}
	terminalPrintf("wrote route coverage report: %s\n", opts.CoverageReport)
	return nil
}

func buildRouteCoverageReport(opts options, results []taskResult, routes map[string]toolutil.ActionMap) string {
	covered := coveredRouteSet(results)
	uncovered := uncoveredHighRiskRoutes(routes, covered)
	domains := uncoveredHighRiskByDomain(uncovered)

	var b strings.Builder
	fmt.Fprintf(&b, "# Schema Route Coverage Report\n\n")
	fmt.Fprintf(&b, "Date: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "Tasks: `%s`\n", opts.TasksPath)
	if opts.ToolsFile != "" {
		fmt.Fprintf(&b, "Tools file: `%s`\n", opts.ToolsFile)
	}
	if opts.Partition != "" {
		fmt.Fprintf(&b, "Partition: `%s`\n", opts.Partition)
	}
	fmt.Fprintf(&b, "\n| Metric | Value |\n| --- | ---: |\n")
	fmt.Fprintf(&b, "| Catalog action routes | %d |\n", countCatalogRoutes(routes))
	fmt.Fprintf(&b, "| Covered action routes | %d |\n", len(covered))
	fmt.Fprintf(&b, "| Uncovered high-risk routes | %d |\n", len(uncovered))

	fmt.Fprintf(&b, "\n## Uncovered High-Risk Domains\n\n")
	fmt.Fprintf(&b, "| Domain | Routes |\n| --- | ---: |\n")
	for _, domain := range domains {
		fmt.Fprintf(&b, "| `%s` | %d |\n", domain.Name, domain.Count)
	}

	fmt.Fprintf(&b, "\n## Uncovered High-Risk Routes\n\n")
	fmt.Fprintf(&b, "| Route | Risk classes |\n| --- | --- |\n")
	limit := min(maxUncoveredRoutesInReport, len(uncovered))
	for _, route := range uncovered[:limit] {
		fmt.Fprintf(&b, "| `%s/%s` | `%s` |\n", route.Tool, route.Action, strings.Join(route.Risks, "`, `"))
	}
	if len(uncovered) > limit {
		fmt.Fprintf(&b, "\nShowing %d of %d uncovered high-risk routes.\n", limit, len(uncovered))
	}
	return b.String()
}

// uncoveredRoute holds uncovered route data for the evaluator package.
type uncoveredRoute struct {
	Tool   string
	Action string
	Risks  []string
}

// domainCount holds domain count data for the evaluator package.
type domainCount struct {
	Name  string
	Count int
}

// coveredRouteSet derives covered route set from catalog metadata.
func coveredRouteSet(results []taskResult) map[string]bool {
	covered := map[string]bool{}
	for _, result := range results {
		for _, step := range taskSteps(result.Task) {
			if step.ExpectedAction == "" {
				continue
			}
			covered[step.ExpectedTool+"/"+step.ExpectedAction] = true
		}
	}
	return covered
}

// uncoveredHighRiskRoutes derives uncovered high risk routes from catalog metadata.
func uncoveredHighRiskRoutes(routes map[string]toolutil.ActionMap, covered map[string]bool) []uncoveredRoute {
	var out []uncoveredRoute
	for tool, actions := range routes {
		for action := range actions {
			key := tool + "/" + action
			if covered[key] {
				continue
			}
			risks := routeRiskClasses(tool, action)
			if len(risks) == 0 {
				continue
			}
			out = append(out, uncoveredRoute{Tool: tool, Action: action, Risks: risks})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tool != out[j].Tool {
			return out[i].Tool < out[j].Tool
		}
		return out[i].Action < out[j].Action
	})
	return out
}

// routeRiskClasses assigns evaluator coverage risk labels to a catalog route.
func routeRiskClasses(tool, action string) []string {
	var risks []string
	if routeLooksEnterprise(tool, action) {
		risks = append(risks, "enterprise_schema_only")
	}
	if routeLooksDestructive(action) {
		risks = append(risks, "destructive")
	}
	if routeLooksMutating(tool, action) {
		risks = append(risks, "mutating")
	}
	if strings.Contains(action, "iid") || strings.Contains(action, "_id") || strings.Contains(action, ".id") {
		risks = append(risks, "id_iid")
	}
	if strings.Contains(action, "path") || strings.Contains(action, "project.") || strings.Contains(action, "group.") {
		risks = append(risks, "path_or_scope")
	}
	if strings.Contains(action, "file") || strings.Contains(action, "upload") || strings.Contains(action, "download") || strings.Contains(action, "base64") {
		risks = append(risks, "payload_or_file")
	}
	if strings.Contains(action, "list") || strings.Contains(action, "search") {
		risks = append(risks, "pagination")
	}
	return uniqueStrings(risks)
}

// uncoveredHighRiskByDomain derives uncovered high risk by domain from evaluator collections.
func uncoveredHighRiskByDomain(routes []uncoveredRoute) []domainCount {
	counts := map[string]int{}
	for _, route := range routes {
		counts[routeDomainName(route.Tool, route.Action)]++
	}
	domains := make([]domainCount, 0, len(counts))
	for name, count := range counts {
		domains = append(domains, domainCount{Name: name, Count: count})
	}
	sort.Slice(domains, func(i, j int) bool {
		if domains[i].Count != domains[j].Count {
			return domains[i].Count > domains[j].Count
		}
		return domains[i].Name < domains[j].Name
	})
	return domains
}

// routeDomainName returns the domain portion of a legacy or unified catalog route.
func routeDomainName(tool, action string) string {
	if tool == dynamicExecuteActionTool && action != "" {
		if before, _, ok := strings.Cut(action, "."); ok {
			return before
		}
		return action
	}
	if tool != "gitlab" || action == "" {
		return strings.TrimPrefix(tool, "gitlab_")
	}
	if before, _, ok := strings.Cut(action, "."); ok {
		return before
	}
	return action
}

// countCatalogRoutes derives count catalog routes from catalog metadata.
func countCatalogRoutes(routes map[string]toolutil.ActionMap) int {
	total := 0
	for _, actions := range routes {
		total += len(actions)
	}
	return total
}

// uniqueStrings derives unique strings from evaluator collections.
func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}

// expectedDisplay formats the expected tool/action sequence for reports.
func expectedDisplay(task evalTask) string {
	steps := taskSteps(task)
	parts := make([]string, 0, len(steps))
	for _, step := range steps {
		parts = append(parts, stepDisplay(step.ExpectedTool, step.ExpectedAction))
	}
	return strings.Join(parts, " -> ")
}

// stepDisplay formats one expected tool/action pair for Markdown reports.
func stepDisplay(tool, action string) string {
	if tool == "" {
		return "-"
	}
	if action == "" {
		return fmt.Sprintf("`%s`", tool)
	}
	return fmt.Sprintf("`%s` / `%s`", tool, action)
}

func writePerRunMetrics(b *strings.Builder, results []taskResult) {
	byRun := make(map[int][]taskResult)
	runs := make([]int, 0)
	for _, result := range results {
		if _, ok := byRun[result.Run]; !ok {
			runs = append(runs, result.Run)
		}
		byRun[result.Run] = append(byRun[result.Run], result)
	}
	sort.Ints(runs)
	fmt.Fprintf(b, "\n## Per-Run Metrics\n\n")
	fmt.Fprintf(b, "| Run | Tool | Action | First pass | Schema lookup | Resource lookup | MCP bridge | Repair success | Destructive safety | Final success |\n")
	fmt.Fprintf(b, "| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, runIndex := range runs {
		metrics := calculateMetrics(byRun[runIndex])
		fmt.Fprintf(
			b, "| %d | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% |\n",
			runIndex,
			metrics.ToolSelection,
			metrics.ActionSelection,
			metrics.FirstPass,
			metrics.SchemaLookup,
			metrics.ResourceLookup,
			metrics.CapabilityLookup,
			metrics.RepairSuccess,
			metrics.DestructiveSafety,
			metrics.FinalSuccess,
		)
	}
}

func writePerModelMetrics(b *strings.Builder, results []taskResult) {
	byModel := resultsByModel(results)
	if len(byModel) <= 1 {
		return
	}
	models := sortedStringKeys(byModel)
	fmt.Fprintf(b, "\n## Per-Model Metrics\n\n")
	fmt.Fprintf(b, "| Model | Attempts | Tool | Action | First pass | Schema lookup | Resource lookup | MCP bridge | Repair success | Destructive safety | Final success |\n")
	fmt.Fprintf(b, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, model := range models {
		metrics := calculateMetrics(byModel[model])
		fmt.Fprintf(b, "| `%s` | %d | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% |\n",
			escapeTable(model), len(byModel[model]), metrics.ToolSelection, metrics.ActionSelection, metrics.FirstPass, metrics.SchemaLookup, metrics.ResourceLookup, metrics.CapabilityLookup, metrics.RepairSuccess, metrics.DestructiveSafety, metrics.FinalSuccess)
	}
}

func writeUsageSummary(b *strings.Builder, opts options, results []taskResult, dryRun bool) {
	if dryRun {
		return
	}
	summary := aggregateUsage(results)
	fmt.Fprintf(b, "\n## API Usage\n\n")
	b.WriteString(metricValueTableHeader)
	fmt.Fprintf(b, metricIntegerValueTableRow, usageModelRequests, summary.ModelCalls)
	fmt.Fprintf(b, metricIntegerValueTableRow, usageToolCallsEmitted, summary.ToolCalls)
	fmt.Fprintf(b, metricIntegerValueTableRow, "Resource calls emitted", summary.ResourceCalls)
	fmt.Fprintf(b, metricIntegerValueTableRow, "MCP capability bridge calls emitted", summary.CapabilityCalls)
	fmt.Fprintf(b, metricIntegerValueTableRow, usageInputTokens, summary.Usage.InputTokens)
	fmt.Fprintf(b, metricIntegerValueTableRow, usageOutputTokens, summary.Usage.OutputTokens)
	fmt.Fprintf(b, "| Cache creation input tokens | %d |\n", summary.Usage.CacheCreationInputTokens)
	fmt.Fprintf(b, "| Cache read input tokens | %d |\n", summary.Usage.CacheReadInputTokens)
	pricing := resolvePricing(opts)
	if pricing.Source == "" {
		fmt.Fprintf(b, "| %s | Not configured |\n", usageEstimatedCost)
		writePerModelUsage(b, opts, results)
		return
	}
	fmt.Fprintf(b, "| %s | $%.4f |\n", usageEstimatedCost, estimateCostUSD(summary.Usage, pricing.Pricing))
	fmt.Fprintf(b, "| Pricing source | %s |\n", pricing.Source)
	writePerModelUsage(b, opts, results)
}

type capabilityBridgeUsage struct {
	Tool   string
	Kind   string
	Target string
	Calls  int
	Models map[string]struct{}
	Tasks  map[string]struct{}
}

func writeCapabilityBridgeUsage(b *strings.Builder, results []taskResult, dryRun bool) {
	if dryRun {
		return
	}
	usage := collectCapabilityBridgeUsage(results)
	if len(usage) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## MCP Capability Bridge Usage\n\n")
	fmt.Fprintf(b, "| Tool | Capability | Target | Calls | Models | Tasks |\n")
	fmt.Fprintf(b, "| --- | --- | --- | ---: | --- | --- |\n")
	for _, item := range usage {
		fmt.Fprintf(b, "| `%s` | %s | %s | %d | %s | %s |\n",
			escapeTable(item.Tool), escapeTable(item.Kind), escapeTable(item.Target), item.Calls, escapeTable(strings.Join(sortedSetValues(item.Models), ", ")), escapeTable(strings.Join(sortedSetValues(item.Tasks), ", ")))
	}
}

func collectCapabilityBridgeUsage(results []taskResult) []capabilityBridgeUsage {
	items := map[string]*capabilityBridgeUsage{}
	for _, result := range results {
		model := result.Model
		if strings.TrimSpace(model) == "" {
			model = "default"
		}
		for _, event := range result.Trace.Events {
			if event.Kind != "tool_use" || !isCapabilityBridgeName(event.Tool) {
				continue
			}
			kind, target := capabilityBridgeEventTarget(event)
			key := event.Tool + "\x00" + kind + "\x00" + target
			item := items[key]
			if item == nil {
				item = &capabilityBridgeUsage{Tool: event.Tool, Kind: kind, Target: target, Models: map[string]struct{}{}, Tasks: map[string]struct{}{}}
				items[key] = item
			}
			item.Calls++
			item.Models[model] = struct{}{}
			item.Tasks[result.Task.ID] = struct{}{}
		}
	}
	out := make([]capabilityBridgeUsage, 0, len(items))
	for _, item := range items {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tool != out[j].Tool {
			return out[i].Tool < out[j].Tool
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Target < out[j].Target
	})
	return out
}

func capabilityBridgeEventTarget(event traceEvent) (kind, target string) {
	switch event.Tool {
	case capabilityListTool:
		return "capabilities", "initialize"
	case resourceListTool:
		return "resources", "resources/list"
	case resourceReadTool:
		uri, _ := event.Input["uri"].(string)
		return "resources", valueOrUnknown(uri)
	case promptListTool:
		return "prompts", "prompts/list"
	case promptGetTool:
		name, _ := event.Input["name"].(string)
		return "prompts", valueOrUnknown(name)
	case completionTool:
		refType, _ := event.Input["ref_type"].(string)
		argumentName, _ := event.Input["argument_name"].(string)
		name, _ := event.Input["name"].(string)
		uri, _ := event.Input["uri"].(string)
		completionTarget := strings.TrimSpace(name)
		if completionTarget == "" {
			completionTarget = strings.TrimSpace(uri)
		}
		if argumentName != "" {
			completionTarget = valueOrUnknown(completionTarget) + "#" + argumentName
		}
		return "completion:" + valueOrUnknown(refType), valueOrUnknown(completionTarget)
	default:
		return "unknown", "unknown"
	}
}

func sortedSetValues(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func valueOrUnknown(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func writePerModelUsage(b *strings.Builder, opts options, results []taskResult) {
	byModel := resultsByModel(results)
	if len(byModel) <= 1 {
		return
	}
	models := sortedStringKeys(byModel)
	fmt.Fprintf(b, "\n### API Usage By Model\n\n")
	fmt.Fprintf(b, "| Model | Requests | %s | Resource calls | MCP bridge calls | %s | %s | %s |\n", usageToolCalls, usageInputTokens, usageOutputTokens, usageEstimatedCost)
	fmt.Fprintf(b, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, model := range models {
		summary := aggregateUsage(byModel[model])
		pricing := resolvePricingForModel(opts, model)
		cost := "Not configured"
		if pricing.Source != "" {
			cost = fmt.Sprintf("$%.4f", estimateCostUSD(summary.Usage, pricing.Pricing))
		}
		fmt.Fprintf(b, "| `%s` | %d | %d | %d | %d | %d | %d | %s |\n", escapeTable(model), summary.ModelCalls, summary.ToolCalls, summary.ResourceCalls, summary.CapabilityCalls, summary.Usage.InputTokens, summary.Usage.OutputTokens, cost)
	}
}

// usageSummary captures usage summary data for evaluation summaries.
type usageSummary struct {
	Usage           modelUsage
	ModelCalls      int
	ToolCalls       int
	ResourceCalls   int
	CapabilityCalls int
}

// aggregateUsage aggregates usage across reports.
func aggregateUsage(results []taskResult) usageSummary {
	var summary usageSummary
	for _, result := range results {
		summary.Usage.add(result.Usage)
		summary.ModelCalls += result.ModelCalls
		summary.ToolCalls += result.ToolCalls
		summary.ResourceCalls += result.ResourceCalls
		summary.CapabilityCalls += result.CapabilityCalls
	}
	return summary
}

// resultsHaveMultipleModels reports whether a result set compares more than one model.
func resultsHaveMultipleModels(results []taskResult) bool {
	return len(resultsByModel(results)) > 1
}

// resultsByModel groups task results by model name, using default for blank names.
func resultsByModel(results []taskResult) map[string][]taskResult {
	out := map[string][]taskResult{}
	for _, result := range results {
		model := result.Model
		if strings.TrimSpace(model) == "" {
			model = "default"
		}
		out[model] = append(out[model], result)
	}
	return out
}

// sortedStringKeys sorts string keys deterministically.
func sortedStringKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// resolvedPricing captures resolved pricing data for evaluation summaries.
type resolvedPricing struct {
	Pricing pricingOptions
	Source  string
}

// resolvePricing resolves pricing for the evaluator package.
func resolvePricing(opts options) resolvedPricing {
	return resolvePricingForModel(opts, opts.Model)
}

// resolvePricingForModel resolves pricing for model for the evaluator package.
func resolvePricingForModel(opts options, model string) resolvedPricing {
	if pricingConfigured(opts.Pricing) {
		return resolvedPricing{Pricing: opts.Pricing, Source: "flags"}
	}
	if strings.Contains(model, ",") {
		return resolvedPricing{}
	}
	if strings.Contains(strings.ToLower(model), "sonnet") {
		return resolvedPricing{
			Pricing: pricingOptions{
				InputPerMTok:      3.00,
				OutputPerMTok:     15.00,
				CacheWritePerMTok: 3.75,
				CacheReadPerMTok:  0.30,
			},
			Source: "default Claude Sonnet estimate",
		}
	}
	return resolvedPricing{}
}

// pricingConfigured reports whether any pricing dimension is available for cost estimates.
func pricingConfigured(pricing pricingOptions) bool {
	return pricing.InputPerMTok > 0 || pricing.OutputPerMTok > 0 || pricing.CacheWritePerMTok > 0 || pricing.CacheReadPerMTok > 0
}

// estimateCostUSD calculates estimate cost usd for evaluation summaries.
func estimateCostUSD(usage modelUsage, pricing pricingOptions) float64 {
	return (float64(usage.InputTokens)*pricing.InputPerMTok +
		float64(usage.OutputTokens)*pricing.OutputPerMTok +
		float64(usage.CacheCreationInputTokens)*pricing.CacheWritePerMTok +
		float64(usage.CacheReadInputTokens)*pricing.CacheReadPerMTok) / 1_000_000
}

// metrics captures metrics data for evaluation summaries.
type metrics struct {
	ToolSelection     float64
	ActionSelection   float64
	FirstPass         float64
	SchemaLookup      float64
	ResourceLookup    float64
	CapabilityLookup  float64
	RepairSuccess     float64
	DestructiveSafety float64
	FinalSuccess      float64
}

type metricCounters struct {
	toolOK             int
	actionOK           int
	firstOK            int
	lookupOK           int
	resourceLookupOK   int
	capabilityLookupOK int
	repairTotal        int
	repairOK           int
	destructiveTotal   int
	destructiveOK      int
	finalOK            int
}

// calculateMetrics derives evaluator success metrics from task results.
func calculateMetrics(results []taskResult) metrics {
	if len(results) == 0 {
		return metrics{}
	}
	counters := metricCounters{}
	for _, result := range results {
		counters.record(result)
	}
	return metrics{
		ToolSelection:     percent(counters.toolOK, len(results)),
		ActionSelection:   percent(counters.actionOK, len(results)),
		FirstPass:         percent(counters.firstOK, len(results)),
		SchemaLookup:      percent(counters.lookupOK, len(results)),
		ResourceLookup:    percent(counters.resourceLookupOK, len(results)),
		CapabilityLookup:  percent(counters.capabilityLookupOK, len(results)),
		RepairSuccess:     percent(counters.repairOK, counters.repairTotal),
		DestructiveSafety: percent(counters.destructiveOK, counters.destructiveTotal),
		FinalSuccess:      percent(counters.finalOK, len(results)),
	}
}

func (c *metricCounters) record(result taskResult) {
	firstToolOK, firstActionOK, firstPassOK := effectiveFirstOutcome(result)
	c.toolOK += boolCount(firstToolOK)
	c.actionOK += boolCount(firstActionOK)
	c.firstOK += boolCount(firstPassOK)
	c.lookupOK += boolCount(result.SchemaLookupUsed)
	c.resourceLookupOK += boolCount(result.ResourceLookupUsed)
	c.capabilityLookupOK += boolCount(result.CapabilityLookupUsed)
	c.recordRepair(result)
	c.recordDestructiveSafety(result)
	c.finalOK += boolCount(result.FinalSuccess)
}

func (c *metricCounters) recordRepair(result taskResult) {
	if !result.RepairAttempted {
		return
	}
	c.repairTotal++
	c.repairOK += boolCount(result.RepairSuccess)
}

func (c *metricCounters) recordDestructiveSafety(result taskResult) {
	if !taskHasDestructiveStep(result.Task) {
		return
	}
	c.destructiveTotal++
	c.destructiveOK += boolCount(result.DestructiveSafe)
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

// effectiveFirstOutcome returns first-call metrics after applying accepted dynamic alternatives.
func effectiveFirstOutcome(result taskResult) (toolOK, actionOK, firstPassOK bool) {
	steps := taskSteps(result.Task)
	if len(steps) == 0 {
		return false, false, false
	}
	for _, step := range firstOutcomeCandidateSteps(steps) {
		if result.FirstTool != step.ExpectedTool {
			continue
		}
		return true, result.FirstAction == step.ExpectedAction, result.FirstPass
	}
	first := steps[0]
	toolOK = result.FirstTool == first.ExpectedTool
	actionOK = result.FirstAction == first.ExpectedAction
	firstPassOK = result.FirstPass
	return toolOK, actionOK, firstPassOK
}

func firstOutcomeCandidateSteps(steps []evalStep) []evalStep {
	candidates := make([]evalStep, 0, len(steps))
	for _, step := range steps {
		candidates = append(candidates, step)
		if !step.OptionalStep {
			break
		}
	}
	return candidates
}

// percent converts a count and total into a percentage, treating empty samples as complete.
func percent(value, total int) float64 {
	if total == 0 {
		return 100
	}
	return float64(value) * 100 / float64(total)
}

// boolText formats booleans for human-readable Markdown reports.
func boolText(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

// escapeTable escapes Markdown table separators in report cells.
func escapeTable(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "\n", "<br>")
	return strings.ReplaceAll(value, "|", "\\|")
}
