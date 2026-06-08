package evaluator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type comparisonInput struct {
	Path          string
	Label         string
	Kind          string
	Date          string
	Mode          string
	Model         string
	ToolSurface   string
	Edition       string
	Backend       string
	Preset        string
	Partition     string
	ToolExecution string
	ToolsFile     string
	CatalogTools  int
	Runs          int
	TaskAttempts  int
	Metrics       map[string]float64
	Usage         map[string]string
	TokenMetrics  map[string]int
	Diagnostics   map[string]int
	Coverage      map[string]int
}

// writeComparisonReport writes comparison report to disk.
func writeComparisonReport(path string, files []string) error {
	if len(files) < 2 {
		return errors.New("--compare requires at least two report files")
	}
	inputs := make([]comparisonInput, 0, len(files))
	for _, file := range files {
		input, err := parseComparisonInput(file)
		if err != nil {
			return err
		}
		inputs = append(inputs, input)
	}
	report := buildComparisonReport(inputs)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create comparison report directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		return fmt.Errorf("write comparison report: %w", err)
	}
	terminalPrintf("wrote comparison report: %s\n", path)
	return nil
}

// parseComparisonInput handles parse comparison input and returns [comparisonInput].
func parseComparisonInput(path string) (comparisonInput, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- explicit developer-provided comparison report path.
	if err != nil {
		return comparisonInput{}, fmt.Errorf("read comparison input %s: %w", path, err)
	}
	content := string(data)
	input := comparisonInput{
		Path:         path,
		Label:        comparisonLabel(path),
		Metrics:      map[string]float64{},
		Usage:        map[string]string{},
		TokenMetrics: map[string]int{},
		Diagnostics:  map[string]int{},
		Coverage:     map[string]int{},
	}
	switch {
	case strings.HasPrefix(content, "# Tools Snapshot Token Audit"):
		input.Kind = "token"
		input.ToolsFile = firstMetadataValue(content, "Tools file")
		input.TokenMetrics = parseIntTable(content, "", "Metric", "Value")
		if input.ToolsFile != "" {
			input.Label = comparisonLabelFromSnapshot(input.ToolsFile, input.Label)
		}
	case strings.HasPrefix(content, "# Meta-Tool Anthropic Evaluation"), strings.HasPrefix(content, "# Meta-Tool Model Evaluation"), strings.HasPrefix(content, "# Dynamic Surface Model Evaluation"), strings.HasPrefix(content, "# MCP Surface Model Evaluation"):
		input.Kind = "evaluation"
		input.Date = firstMetadataValue(content, "Date")
		input.Mode = firstMetadataValue(content, "Mode")
		input.Model = firstMetadataValue(content, "Model")
		input.ToolSurface = firstMetadataValue(content, "Tool surface")
		input.Edition = firstMetadataValue(content, "Edition")
		input.Backend = firstMetadataValue(content, "Backend")
		input.Preset = firstMetadataValue(content, "Preset")
		input.Partition = firstMetadataValue(content, "Partition")
		input.ToolExecution = firstMetadataValue(content, "Tool execution")
		input.ToolsFile = firstMetadataValue(content, "Tools file")
		input.CatalogTools = firstMetadataInt(content, "Catalog tools")
		input.Runs = firstMetadataInt(content, "Runs")
		input.TaskAttempts = firstMetadataInt(content, "Task attempts")
		input.Metrics = parsePercentTable(content, "Metrics")
		input.Usage = parseStringTable(content, "API Usage", "Metric", "Value")
		input.Diagnostics = parseIntTable(content, "Docker Live Failure Triage", "Category", "Count")
		if len(input.Diagnostics) == 0 {
			input.Diagnostics = parseIntTable(content, "Failure Diagnostics", "Category", "Count")
		}
		input.Coverage = parseIntTable(content, "Fixture Tool Coverage", "Metric", "Value")
		if input.ToolsFile != "" {
			input.Label = comparisonLabelFromSnapshot(input.ToolsFile, input.Label)
		}
	default:
		return comparisonInput{}, fmt.Errorf("unsupported comparison input %s", path)
	}
	return input, nil
}

// buildComparisonReport constructs the request parameters from the input.
func buildComparisonReport(inputs []comparisonInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# MCP Surface Evaluation Comparison\n\n")
	fmt.Fprintf(&b, "Date: %s\n\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "## Inputs\n\n")
	fmt.Fprintf(&b, "| Label | Kind | Source | Mode | Surface | Backend | Tasks | Catalog tools |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- | ---: | ---: |\n")
	for _, input := range inputs {
		fmt.Fprintf(&b, "| `%s` | %s | `%s` | %s | %s | %s | %d | %d |\n",
			escapeTable(input.Label), input.Kind, escapeTable(input.Path), emptyDash(input.Mode), emptyDash(input.ToolSurface), emptyDash(input.Backend), input.TaskAttempts, input.CatalogTools)
	}
	writeEvaluationComparison(&b, inputs)
	writeTokenComparison(&b, inputs)
	writeUsageComparison(&b, inputs)
	writeDiagnosticsComparison(&b, inputs)
	writeCoverageComparison(&b, inputs)
	fmt.Fprintf(&b, "\n## Notes\n\n")
	fmt.Fprintf(&b, "- Compare reports generated with the same task set, partition, model, and repeat count for release decisions.\n")
	fmt.Fprintf(&b, "- Token rows come from `cmd/audit_tokens --tools-file`; evaluation rows come from `cmd/eval_mcp_surfaces`.\n")
	fmt.Fprintf(&b, "- Raw traces and snapshot JSON remain local artifacts under ignored `dist/evaluation/mcp-surfaces/`.\n")
	return b.String()
}

// writeEvaluationComparison writes evaluation comparison to disk.
func writeEvaluationComparison(b *strings.Builder, inputs []comparisonInput) {
	evals := comparisonInputsByKind(inputs, "evaluation")
	if len(evals) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Evaluation Metrics\n\n")
	fmt.Fprintf(b, "| Label | Tool | Action | First pass | Schema lookup | Repair | Safety | Final |\n")
	fmt.Fprintf(b, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, input := range evals {
		fmt.Fprintf(
			b, "| `%s` | %s | %s | %s | %s | %s | %s | %s |\n",
			escapeTable(input.Label),
			formatMetric(input.Metrics[metricToolSelection]),
			formatMetric(input.Metrics[metricActionSelection]),
			formatMetric(input.Metrics[metricFirstCallValidationPassRate]),
			formatMetric(input.Metrics["Schema lookup use rate"]),
			formatMetric(input.Metrics[metricRepairSuccessRate]),
			formatMetric(input.Metrics[metricDestructiveSafety]),
			formatMetric(input.Metrics[metricFinalTaskSuccess]),
		)
	}
	writeMetricDeltaTable(b, evals)
}

// writeMetricDeltaTable writes metric delta table to disk.
func writeMetricDeltaTable(b *strings.Builder, evals []comparisonInput) {
	if len(evals) < 2 {
		return
	}
	baseline := evals[0]
	fmt.Fprintf(b, "\n### Delta Versus `%s`\n\n", escapeTable(baseline.Label))
	fmt.Fprintf(b, "| Label | Tool | Action | First pass | Repair | Safety | Final |\n")
	fmt.Fprintf(b, "| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, input := range evals[1:] {
		fmt.Fprintf(
			b, "| `%s` | %s | %s | %s | %s | %s | %s |\n",
			escapeTable(input.Label),
			formatDelta(input.Metrics[metricToolSelection]-baseline.Metrics[metricToolSelection]),
			formatDelta(input.Metrics[metricActionSelection]-baseline.Metrics[metricActionSelection]),
			formatDelta(input.Metrics[metricFirstCallValidationPassRate]-baseline.Metrics[metricFirstCallValidationPassRate]),
			formatDelta(input.Metrics[metricRepairSuccessRate]-baseline.Metrics[metricRepairSuccessRate]),
			formatDelta(input.Metrics[metricDestructiveSafety]-baseline.Metrics[metricDestructiveSafety]),
			formatDelta(input.Metrics[metricFinalTaskSuccess]-baseline.Metrics[metricFinalTaskSuccess]),
		)
	}
}

// writeTokenComparison writes token comparison to disk.
func writeTokenComparison(b *strings.Builder, inputs []comparisonInput) {
	tokens := comparisonInputsByKind(inputs, "token")
	if len(tokens) == 0 {
		return
	}
	baseline := tokens[0]
	fmt.Fprintf(b, "\n## Catalog Token Metrics\n\n")
	fmt.Fprintf(b, "| Label | Tools | Estimated tokens | Serialized bytes | Token delta vs `%s` |\n", escapeTable(baseline.Label))
	fmt.Fprintf(b, "| --- | ---: | ---: | ---: | ---: |\n")
	baseTokens := baseline.TokenMetrics[metricEstimatedTokens]
	for _, input := range tokens {
		delta := input.TokenMetrics[metricEstimatedTokens] - baseTokens
		fmt.Fprintf(b, "| `%s` | %d | %d | %d | %+d |\n",
			escapeTable(input.Label), input.TokenMetrics["Tools"], input.TokenMetrics[metricEstimatedTokens], input.TokenMetrics["Serialized bytes"], delta)
	}
}

// writeUsageComparison writes usage comparison to disk.
func writeUsageComparison(b *strings.Builder, inputs []comparisonInput) {
	evals := comparisonInputsByKind(inputs, "evaluation")
	var withUsage []comparisonInput
	for _, input := range evals {
		if len(input.Usage) > 0 {
			withUsage = append(withUsage, input)
		}
	}
	if len(withUsage) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## API Usage\n\n")
	fmt.Fprintf(b, "| Label | %s | %s | %s | %s | %s |\n", usageModelRequests, usageToolCalls, usageInputTokens, usageOutputTokens, usageEstimatedCost)
	fmt.Fprintf(b, "| --- | ---: | ---: | ---: | ---: | ---: |\n")
	for _, input := range withUsage {
		requests := input.Usage[usageModelRequests]
		if requests == "" {
			requests = input.Usage["Anthropic requests"]
		}
		toolCalls := input.Usage[usageToolCallsEmitted]
		if toolCalls == "" {
			toolCalls = input.Usage[usageToolCalls]
		}
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | %s | %s |\n",
			escapeTable(input.Label), valueOrZero(requests), valueOrZero(toolCalls), valueOrZero(input.Usage[usageInputTokens]), valueOrZero(input.Usage[usageOutputTokens]), emptyDash(input.Usage[usageEstimatedCost]))
	}
}

// writeDiagnosticsComparison writes diagnostics comparison to disk.
func writeDiagnosticsComparison(b *strings.Builder, inputs []comparisonInput) {
	categories := sortedIntKeys(func() map[string]int {
		merged := map[string]int{}
		for _, input := range inputs {
			for category := range input.Diagnostics {
				merged[category] = 1
			}
		}
		return merged
	}())
	if len(categories) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Failure Diagnostics\n\n")
	fmt.Fprintf(b, "| Label | %s |\n", strings.Join(categories, " | "))
	fmt.Fprintf(b, "| --- |%s\n", strings.Repeat(" ---: |", len(categories)))
	for _, input := range inputs {
		fmt.Fprintf(b, "| `%s`", escapeTable(input.Label))
		for _, category := range categories {
			fmt.Fprintf(b, " | %d", input.Diagnostics[category])
		}
		fmt.Fprintf(b, " |\n")
	}
}

// writeCoverageComparison writes coverage comparison to disk.
func writeCoverageComparison(b *strings.Builder, inputs []comparisonInput) {
	evals := comparisonInputsByKind(inputs, "evaluation")
	if len(evals) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Fixture Coverage\n\n")
	fmt.Fprintf(b, "| Label | Catalog action routes | Covered action routes | Missing action routes |\n")
	fmt.Fprintf(b, "| --- | ---: | ---: | ---: |\n")
	for _, input := range evals {
		fmt.Fprintf(b, "| `%s` | %d | %d | %d |\n",
			escapeTable(input.Label), input.Coverage["Catalog action routes"], input.Coverage["Action routes covered by expected steps"], input.Coverage["Missing action routes"])
	}
}

// comparisonInputsByKind filters comparison report inputs by snapshot kind.
func comparisonInputsByKind(inputs []comparisonInput, kind string) []comparisonInput {
	var out []comparisonInput
	for _, input := range inputs {
		if input.Kind == kind {
			out = append(out, input)
		}
	}
	return out
}

// firstMetadataValue returns the first metadata value for a key in a report header.
func firstMetadataValue(content, key string) string {
	prefix := key + ":"
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if value, found := strings.CutPrefix(line, prefix); found {
			return cleanReportValue(strings.TrimSpace(value))
		}
	}
	return ""
}

// firstMetadataInt returns the first metadata int value that is set.
func firstMetadataInt(content, key string) int {
	return parseReportInt(firstMetadataValue(content, key))
}

// parsePercentTable parses percent table from evaluator input.
func parsePercentTable(content, section string) map[string]float64 {
	out := map[string]float64{}
	for key, value := range parseStringTable(content, section, "Metric", "Value") {
		out[key] = parseReportPercent(value)
	}
	return out
}

// parseIntTable parses int table from evaluator input.
func parseIntTable(content, section, keyHeader, valueHeader string) map[string]int {
	out := map[string]int{}
	for key, value := range parseStringTable(content, section, keyHeader, valueHeader) {
		out[key] = parseReportInt(value)
	}
	return out
}

// parseStringTable parses string table from evaluator input.
func parseStringTable(content, section, keyHeader, valueHeader string) map[string]string {
	out := map[string]string{}
	for _, row := range reportTableRows(content, section) {
		if len(row) < 2 || row[0] == keyHeader || row[1] == valueHeader {
			continue
		}
		out[cleanReportValue(row[0])] = cleanReportValue(row[1])
	}
	return out
}

// reportTableRows extracts table rows from generated reports.
func reportTableRows(content, section string) [][]string {
	var rows [][]string
	if section != "" {
		for _, line := range sectionLines(strings.Split(content, "\n"), section) {
			rows = appendReportTableRow(rows, line)
		}
		return rows
	}
	for line := range strings.SplitSeq(content, "\n") {
		rows = appendReportTableRow(rows, line)
	}
	return rows
}

// appendReportTableRow appends report table row to the output builder.
func appendReportTableRow(rows [][]string, line string) [][]string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return rows
	}
	row := splitMarkdownRow(line)
	if markdownSeparatorRow(row) {
		return rows
	}
	return append(rows, row)
}

// sectionLines extracts lines from a managed Markdown section.
func sectionLines(lines []string, section string) []string {
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "## "+section {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return nil
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			end = i
			break
		}
	}
	return lines[start:end]
}

// markdownSeparatorRow reports whether a parsed row is a Markdown table separator.
func markdownSeparatorRow(row []string) bool {
	if len(row) == 0 {
		return false
	}
	for _, cell := range row {
		trimmed := strings.Trim(cell, " -:")
		if trimmed != "" {
			return false
		}
	}
	return true
}

// comparisonLabel formats comparison label for report output.
func comparisonLabel(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if base == "tools" || base == "tokens" || strings.HasPrefix(base, "schema-") || strings.HasPrefix(base, "live-") {
		parent := filepath.Base(filepath.Dir(path))
		if parent != "." && parent != string(filepath.Separator) && parent != "" {
			return parent
		}
	}
	return base
}

// comparisonLabelFromSnapshot formats comparison label from snapshot for report output.
func comparisonLabelFromSnapshot(snapshotPath, fallback string) string {
	snapshotPath = cleanReportValue(snapshotPath)
	if snapshotPath == "" {
		return fallback
	}
	parent := filepath.Base(filepath.Dir(snapshotPath))
	if parent == "." || parent == string(filepath.Separator) || parent == "" {
		return fallback
	}
	return parent
}

// cleanReportValue normalizes report cell text before numeric parsing or comparison.
func cleanReportValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "`")
	return strings.TrimSpace(value)
}

// parseReportPercent parses report percent from evaluator input.
func parseReportPercent(value string) float64 {
	value = strings.TrimSuffix(cleanReportValue(value), "%")
	value = strings.ReplaceAll(value, ",", "")
	parsed, _ := strconv.ParseFloat(value, 64)
	return parsed
}

// parseReportInt parses report int from evaluator input.
func parseReportInt(value string) int {
	value = cleanReportValue(value)
	value = strings.ReplaceAll(value, ",", "")
	fields := strings.Fields(value)
	if len(fields) > 0 {
		value = fields[0]
	}
	parsed, _ := strconv.Atoi(value)
	return parsed
}

// formatMetric renders the result as a formatted string.
func formatMetric(value float64) string {
	return fmt.Sprintf("%.1f%%", value)
}

// formatDelta renders the result as a formatted string.
func formatDelta(value float64) string {
	return fmt.Sprintf("%+.1f pp", value)
}

// sortedIntKeys sorts int keys deterministically.
func sortedIntKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// emptyDash formats empty comparison values as a table-friendly dash.
func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return escapeTable(value)
}

// valueOrZero formats empty numeric comparison values as zero.
func valueOrZero(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0"
	}
	return escapeTable(value)
}

// shouldWriteStartupReport reports whether should write startup report.
