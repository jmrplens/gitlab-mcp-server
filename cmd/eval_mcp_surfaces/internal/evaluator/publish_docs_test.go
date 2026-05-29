package evaluator

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestReadPublishReport_ParsesSingleModelReport verifies ReadPublishReport parses single model report.
func TestReadPublishReport_ParsesSingleModelReport(t *testing.T) {
	path := writeTempPublishReport(t, singleModelPublishReport("openai:gpt-5.4-nano", presetDockerRead, 2))

	report, err := readPublishReport(path)
	if err != nil {
		t.Fatalf("readPublishReport() error = %v", err)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(report.Rows))
	}
	row := report.Rows[0]
	if row.Model != "openai:gpt-5.4-nano" || row.Preset != presetDockerRead || row.Backend != backendGitLab || row.ToolExecution != "mcp" {
		t.Fatalf("row metadata = %+v", row)
	}
	if report.ToolSurface != config.ToolSurfaceMeta {
		t.Fatalf("tool surface = %q, want meta", report.ToolSurface)
	}
	if row.Attempts != 2 || row.ExpectedOps != 3 || row.ModelRequests != 3 || row.ToolCalls != 3 {
		t.Fatalf("row counts = %+v, want attempts=2 expected=3 requests=3 tools=3", row)
	}
	if row.ToolSelection != 100 || row.FinalSuccess != 100 {
		t.Fatalf("row metrics = %+v, want 100%% tool/final", row)
	}
	if got := rowCommitBranchDate(row); got != "8c696a2 / port/main-small-meta-fixes / 2026-05-05T18:00:00Z" {
		t.Fatalf("rowCommitBranchDate() = %q", got)
	}
}

// TestReadPublishReport_ParsesPerModelRows verifies ReadPublishReport parses per model rows.
func TestReadPublishReport_ParsesPerModelRows(t *testing.T) {
	path := writeTempPublishReport(t, multiModelPublishReport())

	report, err := readPublishReport(path)
	if err != nil {
		t.Fatalf("readPublishReport() error = %v", err)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(report.Rows))
	}
	rows := map[string]publishRow{}
	for _, row := range report.Rows {
		rows[row.Model] = row
	}
	if rows["anthropic:claude-haiku-4-5-20251001"].ExpectedOps != 2 {
		t.Fatalf("anthropic expected ops = %d, want 2", rows["anthropic:claude-haiku-4-5-20251001"].ExpectedOps)
	}
	if rows["google:gemini-3.1-flash-lite-preview"].ModelRequests != 2 || rows["google:gemini-3.1-flash-lite-preview"].ToolCalls != 2 {
		t.Fatalf("google usage = %+v, want requests/tools 2", rows["google:gemini-3.1-flash-lite-preview"])
	}
}

// TestValidatePublishReports_RejectsPartialDockerPresetWithoutTargetedLabel verifies ValidatePublishReports rejects partial docker preset without targeted label.
func TestValidatePublishReports_RejectsPartialDockerPresetWithoutTargetedLabel(t *testing.T) {
	path := writeTempPublishReport(t, singleModelPublishReport("openai:gpt-5.4-nano", presetDockerRead, 1))
	report, err := readPublishReport(path)
	if err != nil {
		t.Fatalf("readPublishReport() error = %v", err)
	}

	err = validatePublishReports([]publishReport{report}, "2026-05-05 Docker economy models", false)
	if err == nil || !strings.Contains(err.Error(), "partial docker-read") {
		t.Fatalf("validatePublishReports() error = %v, want partial docker-read guardrail", err)
	}

	if validateErr := validatePublishReports([]publishReport{report}, "2026-05-05 targeted Docker repair", false); validateErr != nil {
		t.Fatalf("validatePublishReports(targeted) error = %v", validateErr)
	}
}

// TestSortedPublishRows_ReplacesDuplicateModelPresetRows verifies SortedPublishRows when replaces duplicate model preset rows.
func TestSortedPublishRows_ReplacesDuplicateModelPresetRows(t *testing.T) {
	oldPath := writeTempPublishReport(t, singleModelPublishReport("google:gemini-3.1-flash-lite-preview", presetDockerMutatingSafe, fullDockerAttemptsByPreset[presetDockerMutatingSafe]))
	newPath := writeTempPublishReport(t, singleModelPublishReport("google:gemini-3.1-flash-lite-preview", presetDockerMutatingSafe, fullDockerAttemptsByPreset[presetDockerMutatingSafe]))
	reports, err := readPublishReports([]string{oldPath, newPath})
	if err != nil {
		t.Fatalf("readPublishReports() error = %v", err)
	}

	rows := sortedPublishRows(reports)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 replacement row", len(rows))
	}
	if rows[0].SourcePath != newPath {
		t.Fatalf("replacement source = %q, want latest input %q", rows[0].SourcePath, newPath)
	}
}

// TestAggregatePublishRows_RepairSuccessUsesRepairAttempts verifies AggregatePublishRows uses repair attempts for repair success.
func TestAggregatePublishRows_RepairSuccessUsesRepairAttempts(t *testing.T) {
	rows := []publishRow{
		{Attempts: fullDockerAttemptsByPreset[presetDockerRead], RepairSuccess: 100},
		{Attempts: fullDockerAttemptsByPreset[presetDockerDestructiveSafe], RepairSuccess: 50, RepairAttempts: 4, RepairSuccesses: 2},
	}

	aggregate := aggregatePublishRows(rows)
	if aggregate.RepairSuccess != 50 {
		t.Fatalf("aggregate repair = %.1f, want repair-attempt ratio 50.0", aggregate.RepairSuccess)
	}
	if got := formatRepairMetric(aggregate); got != "50.0% (2/4)" {
		t.Fatalf("formatRepairMetric() = %q, want count-qualified percentage", got)
	}
}

func TestPublishEffectiveTraceOutcome_AcceptsOptionalCapabilityPreludeSkip(t *testing.T) {
	trace := taskTrace{
		Expected: []traceExpectedStep{
			{Tool: capabilityListTool, OptionalStep: true},
			{Tool: resourceListTool},
			{Tool: resourceReadTool},
		},
		Summary: traceSummary{FirstTool: resourceListTool, FirstPass: true},
	}

	toolOK, actionOK, firstPassOK := publishEffectiveTraceOutcome(trace, config.ToolSurfaceDynamic)

	if !toolOK || !actionOK || !firstPassOK {
		t.Fatalf("publishEffectiveTraceOutcome() = %t/%t/%t, want all true", toolOK, actionOK, firstPassOK)
	}
}

// TestCurrentGitReportMetadata_ReadsGitMetadata verifies optional Git metadata
// collection reads .git files directly and returns branch plus short commit
// information without invoking an external git binary.
func TestCurrentGitReportMetadata_ReadsGitMetadata(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads", "feature"), 0o700); err != nil {
		t.Fatalf("mkdir git refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/eval\n"), 0o600); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "feature", "eval"), []byte("0123456789abcdef0123456789abcdef01234567\n"), 0o600); err != nil {
		t.Fatalf("write ref: %v", err)
	}
	t.Chdir(root)

	branch, commit := currentGitReportMetadata()
	if branch != "feature/eval" || commit != "0123456789ab" {
		t.Fatalf("currentGitReportMetadata() = branch %q commit %q", branch, commit)
	}
}

// TestResolveGitDir_SupportsGitFileAndPackedRefs verifies worktree .git files
// and packed refs are supported for Git worktree-compatible metadata.
func TestResolveGitDir_SupportsGitFileAndPackedRefs(t *testing.T) {
	root := t.TempDir()
	metadataDir := filepath.Join(root, "metadata")
	worktree := filepath.Join(root, "worktree")
	if err := os.MkdirAll(worktree, 0o700); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	if err := os.MkdirAll(metadataDir, 0o700); err != nil {
		t.Fatalf("mkdir metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: ../metadata\n"), 0o600); err != nil {
		t.Fatalf("write .git file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metadataDir, "packed-refs"), []byte("# pack-refs\nabcdef0123456789abcdef0123456789abcdef01 refs/heads/main\n"), 0o600); err != nil {
		t.Fatalf("write packed refs: %v", err)
	}
	gitDir, err := resolveGitDir(worktree)
	if err != nil {
		t.Fatalf("resolveGitDir() error = %v", err)
	}
	if gitDir != metadataDir {
		t.Fatalf("gitDir = %q, want %q", gitDir, metadataDir)
	}
	commit, err := readGitRef(gitDir, "refs/heads/main")
	if err != nil {
		t.Fatalf("readGitRef() error = %v", err)
	}
	if commit != "abcdef0123456789abcdef0123456789abcdef01" {
		t.Fatalf("commit = %q", commit)
	}
	if got := gitBranchName("refs/heads/feature/eval"); got != "feature/eval" {
		t.Fatalf("gitBranchName() = %q", got)
	}
	if got := shortGitCommit("abcdef0123456789"); got != "abcdef012345" {
		t.Fatalf("shortGitCommit() = %q", got)
	}
}

// TestApplyManagedBlock_ReplacesAndAppendsSnapshots verifies ApplyManagedBlock when replaces and appends snapshots.
func TestApplyManagedBlock_ReplacesAndAppendsSnapshots(t *testing.T) {
	content := "before\n" + modelEvalResultsStart + "\n### Old\n\nold\n" + modelEvalResultsEnd + "\nafter\n"
	replaced, err := applyManagedBlock(content, modelEvalResultsStart, modelEvalResultsEnd, "### New\n\nnew\n", publishModeReplaceCurrent, "New")
	if err != nil {
		t.Fatalf("applyManagedBlock(replace) error = %v", err)
	}
	if strings.Contains(replaced, "### Old") || !strings.Contains(replaced, "### New") {
		t.Fatalf("replace output = %q", replaced)
	}

	appended, err := applyManagedBlock(content, modelEvalResultsStart, modelEvalResultsEnd, "### New\n\nnew\n", publishModeAppend, "New")
	if err != nil {
		t.Fatalf("applyManagedBlock(append) error = %v", err)
	}
	if !strings.Contains(appended, "### New\n\nnew\n\n### Old") {
		t.Fatalf("append output = %q, want new snapshot before old", appended)
	}
}

// TestPublishEvaluationDocs_WritesAndChecksManagedDocs verifies PublishEvaluationDocs writes and checks managed docs.
func TestPublishEvaluationDocs_WritesAndChecksManagedDocs(t *testing.T) {
	tmp := t.TempDir()
	reportPath := filepath.Join(tmp, "report.md")
	if err := os.WriteFile(reportPath, []byte(singleModelPublishReport("openai:gpt-5.4-nano", presetDockerRead, fullDockerAttemptsByPreset[presetDockerRead])), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	resultsPath := filepath.Join(tmp, "model-results.md")
	readmePath := filepath.Join(tmp, "README.md")
	resultsDoc := "# Results\n\n" +
		modelEvalMetaResultsStart + "\n" + modelEvalMetaResultsEnd + "\n\n" +
		modelEvalDynamicResultsStart + "\nexisting dynamic\n" + modelEvalDynamicResultsEnd + "\n"
	if err := os.WriteFile(resultsPath, []byte(resultsDoc), 0o600); err != nil {
		t.Fatalf("write results doc: %v", err)
	}
	readmeDoc := "# README\n\n" +
		modelEvalMetaSummaryStart + "\n" + modelEvalMetaSummaryEnd + "\n\n" +
		modelEvalDynamicSummaryStart + "\nexisting dynamic summary\n" + modelEvalDynamicSummaryEnd + "\n"
	if err := os.WriteFile(readmePath, []byte(readmeDoc), 0o600); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	opts := options{
		PublishDocs:    true,
		PublishFrom:    stringList{reportPath},
		PublishResults: resultsPath,
		PublishReadme:  readmePath,
		PublishLabel:   "2026-05-05 Docker economy models",
		PublishMode:    publishModeReplaceCurrent,
	}
	if err := publishEvaluationDocs(opts); err != nil {
		t.Fatalf("publishEvaluationDocs() error = %v", err)
	}
	results, err := os.ReadFile(resultsPath)
	if err != nil {
		t.Fatalf("read results doc: %v", err)
	}
	if !strings.Contains(string(results), "| `openai:gpt-5.4-nano` | `docker-read`") || !strings.Contains(string(results), "| **Aggregate**") {
		t.Fatalf("results doc = %s", results)
	}
	if strings.Contains(string(results), "Source reports") || strings.Contains(string(results), reportPath) {
		t.Fatalf("results doc leaked local report paths: %s", results)
	}
	if !strings.Contains(string(results), "existing dynamic") {
		t.Fatalf("results doc did not preserve dynamic section: %s", results)
	}
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	if !strings.Contains(string(readme), "| OpenAI") || !strings.Contains(string(readme), "`gpt-5.4-nano`") || !strings.Contains(string(readme), fmt.Sprintf("100.0%% final across %d ops", fullDockerAttemptsByPreset[presetDockerRead]+1)) {
		t.Fatalf("readme = %s", readme)
	}
	if !strings.Contains(string(readme), "existing dynamic summary") {
		t.Fatalf("readme did not preserve dynamic summary section: %s", readme)
	}

	opts.CheckDocs = true
	opts.PublishDocs = false
	if publishErr := publishEvaluationDocs(opts); publishErr != nil {
		t.Fatalf("publishEvaluationDocs(check) error = %v", publishErr)
	}
	staleReadmeDoc := "# README\n\n" +
		modelEvalMetaSummaryStart + "\nstale\n" + modelEvalMetaSummaryEnd + "\n\n" +
		modelEvalDynamicSummaryStart + "\nexisting dynamic summary\n" + modelEvalDynamicSummaryEnd + "\n"
	if writeErr := os.WriteFile(readmePath, []byte(staleReadmeDoc), 0o600); writeErr != nil {
		t.Fatalf("write stale readme: %v", writeErr)
	}
	if checkErr := publishEvaluationDocs(opts); checkErr == nil || !strings.Contains(checkErr.Error(), "not up to date") {
		t.Fatalf("publishEvaluationDocs(stale check) error = %v, want not up to date", checkErr)
	}
}

// TestPublishEvaluationDocs_RoutesDynamicReportsToDynamicSection verifies that
// dynamic reports update only the dynamic blocks while preserving meta-tool data.
func TestPublishEvaluationDocs_RoutesDynamicReportsToDynamicSection(t *testing.T) {
	tmp := t.TempDir()
	reportPath := filepath.Join(tmp, "dynamic-report.md")
	if err := os.WriteFile(reportPath, []byte(dynamicSingleModelPublishReport("openai:gpt-5.4-nano", presetDockerRead, fullDockerAttemptsByPreset[presetDockerRead])), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	resultsPath := filepath.Join(tmp, "model-results.md")
	readmePath := filepath.Join(tmp, "README.md")
	resultsDoc := "# Results\n\n" +
		modelEvalMetaResultsStart + "\nexisting meta results\n" + modelEvalMetaResultsEnd + "\n\n" +
		modelEvalDynamicResultsStart + "\n" + modelEvalDynamicResultsEnd + "\n"
	if err := os.WriteFile(resultsPath, []byte(resultsDoc), 0o600); err != nil {
		t.Fatalf("write results doc: %v", err)
	}
	readmeDoc := "# README\n\n" +
		modelEvalMetaSummaryStart + "\nexisting meta summary\n" + modelEvalMetaSummaryEnd + "\n\n" +
		modelEvalDynamicSummaryStart + "\n" + modelEvalDynamicSummaryEnd + "\n"
	if err := os.WriteFile(readmePath, []byte(readmeDoc), 0o600); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	opts := options{
		PublishDocs:    true,
		PublishFrom:    stringList{reportPath},
		PublishResults: resultsPath,
		PublishReadme:  readmePath,
		PublishLabel:   "2026-05-09 Dynamic OpenAI Docker full run",
		PublishMode:    publishModeReplaceCurrent,
	}
	if err := publishEvaluationDocs(opts); err != nil {
		t.Fatalf("publishEvaluationDocs(dynamic) error = %v", err)
	}
	results, err := os.ReadFile(resultsPath)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	if !strings.Contains(string(results), "existing meta results") {
		t.Fatalf("dynamic publish did not preserve meta results: %s", results)
	}
	if !strings.Contains(string(results), "### 2026-05-09 Dynamic OpenAI Docker full run") || !strings.Contains(string(results), "| `openai:gpt-5.4-nano` | `docker-read`") {
		t.Fatalf("dynamic results section was not updated: %s", results)
	}
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	if !strings.Contains(string(readme), "existing meta summary") {
		t.Fatalf("dynamic publish did not preserve meta summary: %s", readme)
	}
	if !strings.Contains(string(readme), "Current published result: **2026-05-09 Dynamic OpenAI Docker full run**.") {
		t.Fatalf("dynamic readme summary was not updated: %s", readme)
	}

	opts.CheckDocs = true
	opts.PublishDocs = false
	if checkErr := publishEvaluationDocs(opts); checkErr != nil {
		t.Fatalf("publishEvaluationDocs(dynamic check) error = %v", checkErr)
	}
}

// TestPublishEvaluationDocs_RoutesEnterpriseReportsToEnterpriseSections verifies
// Enterprise dynamic and meta reports update their dedicated managed blocks.
func TestPublishEvaluationDocs_RoutesEnterpriseReportsToEnterpriseSections(t *testing.T) {
	tmp := t.TempDir()
	metaReport := filepath.Join(tmp, "enterprise-meta-report.md")
	if err := os.WriteFile(metaReport, []byte(singleModelPublishReport("openai:gpt-5.4-nano", presetDockerEnterpriseRead, fullDockerAttemptsByPreset[presetDockerEnterpriseRead])), 0o600); err != nil {
		t.Fatalf("write meta report: %v", err)
	}
	dynamicReport := filepath.Join(tmp, "enterprise-dynamic-report.md")
	if err := os.WriteFile(dynamicReport, []byte(dynamicSingleModelPublishReport("google:gemini-3.1-flash-lite-preview", presetDockerEnterpriseMutatingSafe, fullDockerAttemptsByPreset[presetDockerEnterpriseMutatingSafe])), 0o600); err != nil {
		t.Fatalf("write dynamic report: %v", err)
	}
	resultsPath := filepath.Join(tmp, "model-results.md")
	readmePath := filepath.Join(tmp, "README.md")
	resultsDoc := "# Results\n\n" +
		modelEvalMetaResultsStart + "\nexisting CE meta results\n" + modelEvalMetaResultsEnd + "\n\n" +
		modelEvalDynamicResultsStart + "\nexisting CE dynamic results\n" + modelEvalDynamicResultsEnd + "\n\n" +
		modelEvalEnterpriseMetaResultsStart + "\n" + modelEvalEnterpriseMetaResultsEnd + "\n\n" +
		modelEvalEnterpriseDynamicResultsStart + "\n" + modelEvalEnterpriseDynamicResultsEnd + "\n"
	if err := os.WriteFile(resultsPath, []byte(resultsDoc), 0o600); err != nil {
		t.Fatalf("write results doc: %v", err)
	}
	readmeDoc := "# README\n\n" +
		modelEvalMetaSummaryStart + "\nexisting CE meta summary\n" + modelEvalMetaSummaryEnd + "\n\n" +
		modelEvalDynamicSummaryStart + "\nexisting CE dynamic summary\n" + modelEvalDynamicSummaryEnd + "\n\n" +
		modelEvalEnterpriseMetaSummaryStart + "\n" + modelEvalEnterpriseMetaSummaryEnd + "\n\n" +
		modelEvalEnterpriseDynamicSummaryStart + "\n" + modelEvalEnterpriseDynamicSummaryEnd + "\n"
	if err := os.WriteFile(readmePath, []byte(readmeDoc), 0o600); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	opts := options{
		PublishDocs:    true,
		PublishFrom:    stringList{metaReport, dynamicReport},
		PublishResults: resultsPath,
		PublishReadme:  readmePath,
		PublishLabel:   "2026-05-10 Enterprise Docker full run",
		PublishMode:    publishModeReplaceCurrent,
	}
	if err := publishEvaluationDocs(opts); err != nil {
		t.Fatalf("publishEvaluationDocs(enterprise) error = %v", err)
	}
	results, err := os.ReadFile(resultsPath)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	if !strings.Contains(string(results), "existing CE meta results") || !strings.Contains(string(results), "existing CE dynamic results") {
		t.Fatalf("enterprise publish did not preserve CE results: %s", results)
	}
	if !strings.Contains(string(results), "| `openai:gpt-5.4-nano` | `docker-enterprise-read`") || !strings.Contains(string(results), "| `google:gemini-3.1-flash-lite-preview` | `docker-enterprise-mutating-safe`") {
		t.Fatalf("enterprise result sections were not updated: %s", results)
	}
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	if !strings.Contains(string(readme), "existing CE meta summary") || !strings.Contains(string(readme), "existing CE dynamic summary") {
		t.Fatalf("enterprise publish did not preserve CE summaries: %s", readme)
	}
	if count := strings.Count(string(readme), "Current published result: **2026-05-10 Enterprise Docker full run**."); count != 2 {
		t.Fatalf("enterprise readme summaries updated %d sections, want 2: %s", count, readme)
	}
}

// TestReadPublishReport_SplitsFullRunByPresetFromTraceArtifacts verifies that
// full dynamic runs without a report-level preset are still published as
// preset-scoped rows when trace artifacts are available.
func TestReadPublishReport_SplitsFullRunByPresetFromTraceArtifacts(t *testing.T) {
	tmp := t.TempDir()
	traceDir := filepath.Join(tmp, "traces")
	if err := os.MkdirAll(traceDir, 0o700); err != nil {
		t.Fatalf("mkdir traces: %v", err)
	}
	tracePath := filepath.Join(traceDir, "traces.jsonl")
	if err := os.WriteFile(tracePath, []byte(fullRunTraceJSONL()), 0o600); err != nil {
		t.Fatalf("write traces: %v", err)
	}
	reportPath := filepath.Join(tmp, "dynamic-report.md")
	if err := os.WriteFile(reportPath, []byte(dynamicFullRunPublishReportNoPreset()), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	report, err := readPublishReport(reportPath)
	if err != nil {
		t.Fatalf("readPublishReport() error = %v", err)
	}
	if len(report.Rows) != 3 {
		t.Fatalf("rows = %d, want 3 preset rows", len(report.Rows))
	}
	rows := map[string]publishRow{}
	for _, row := range report.Rows {
		rows[row.Preset] = row
	}
	for _, preset := range []string{presetDockerRead, presetDockerMutatingSafe, presetDockerDestructiveSafe} {
		if rows[preset].Attempts != 1 {
			t.Fatalf("row[%s] = %+v, want one attempt", preset, rows[preset])
		}
	}
	if rows[presetDockerMutatingSafe].ModelRequests != 2 || rows[presetDockerMutatingSafe].ToolCalls != 2 {
		t.Fatalf("mutating row counts = %+v, want requests/tools 2", rows[presetDockerMutatingSafe])
	}
	if rows[presetDockerMutatingSafe].CostTokens != "in 15 / out 4" {
		t.Fatalf("mutating cost/tokens = %q", rows[presetDockerMutatingSafe].CostTokens)
	}
	if rows[presetDockerDestructiveSafe].DestructiveSafety != 100 {
		t.Fatalf("destructive safety = %.1f, want 100", rows[presetDockerDestructiveSafe].DestructiveSafety)
	}
}

// TestPublishEvaluationDocs_SplitsCombinedDynamicRunByEdition verifies that a
// combined Dynamic report generated on an Enterprise runtime still routes CE
// preset rows to the CE Dynamic block and Enterprise rows to the Enterprise
// Dynamic block.
func TestPublishEvaluationDocs_SplitsCombinedDynamicRunByEdition(t *testing.T) {
	tmp := t.TempDir()
	reportPath, resultsPath, readmePath := writeCombinedDynamicPublishFixture(t, tmp)

	report, err := readPublishReport(reportPath)
	if err != nil {
		t.Fatalf("readPublishReport() error = %v", err)
	}
	assertCombinedDynamicReportSections(t, report)

	opts := options{
		PublishDocs:    true,
		PublishFrom:    stringList{reportPath},
		PublishResults: resultsPath,
		PublishReadme:  readmePath,
		PublishLabel:   "Docker CE+Enterprise-on-Enterprise dynamic 2026 combined targeted",
		PublishMode:    publishModeReplaceCurrent,
	}
	if publishErr := publishEvaluationDocs(opts); publishErr != nil {
		t.Fatalf("publishEvaluationDocs(combined dynamic) error = %v", publishErr)
	}
	assertCombinedDynamicPublishedDocs(t, resultsPath, readmePath)
}

func writeCombinedDynamicPublishFixture(t *testing.T, tmp string) (reportPath, resultsPath, readmePath string) {
	t.Helper()
	traceDir := filepath.Join(tmp, "traces")
	if mkdirErr := os.MkdirAll(traceDir, 0o700); mkdirErr != nil {
		t.Fatalf("mkdir traces: %v", mkdirErr)
	}
	if writeErr := os.WriteFile(filepath.Join(traceDir, "traces.jsonl"), []byte(combinedEnterpriseFullRunTraceJSONL()), 0o600); writeErr != nil {
		t.Fatalf("write traces: %v", writeErr)
	}
	reportPath = filepath.Join(tmp, "dynamic-report.md")
	if writeErr := os.WriteFile(reportPath, []byte(dynamicEnterpriseFullRunPublishReportNoPreset()), 0o600); writeErr != nil {
		t.Fatalf("write report: %v", writeErr)
	}
	resultsPath = filepath.Join(tmp, "model-results.md")
	readmePath = filepath.Join(tmp, "README.md")
	resultsDoc := "# Results\n\n" +
		modelEvalDynamicResultsStart + "\n" + modelEvalDynamicResultsEnd + "\n\n" +
		modelEvalEnterpriseDynamicResultsStart + "\n" + modelEvalEnterpriseDynamicResultsEnd + "\n"
	if writeErr := os.WriteFile(resultsPath, []byte(resultsDoc), 0o600); writeErr != nil {
		t.Fatalf("write results doc: %v", writeErr)
	}
	readmeDoc := "# README\n\n" +
		modelEvalDynamicSummaryStart + "\n" + modelEvalDynamicSummaryEnd + "\n\n" +
		modelEvalEnterpriseDynamicSummaryStart + "\n" + modelEvalEnterpriseDynamicSummaryEnd + "\n"
	if writeErr := os.WriteFile(readmePath, []byte(readmeDoc), 0o600); writeErr != nil {
		t.Fatalf("write readme: %v", writeErr)
	}
	return reportPath, resultsPath, readmePath
}

func assertCombinedDynamicReportSections(t *testing.T, report publishReport) {
	t.Helper()
	rows := map[string]publishRow{}
	for _, row := range report.Rows {
		rows[row.Preset] = row
	}
	if got := publishSectionForRow(report, rows[presetDockerRead]); got != publishSectionDynamic {
		t.Fatalf("CE row section = %q, want %q", got, publishSectionDynamic)
	}
	if got := publishSectionForRow(report, rows[presetDockerEnterpriseRead]); got != publishSectionEnterpriseDynamic {
		t.Fatalf("Enterprise row section = %q, want %q", got, publishSectionEnterpriseDynamic)
	}
}

func assertCombinedDynamicPublishedDocs(t *testing.T, resultsPath, readmePath string) {
	t.Helper()
	results, readErr := os.ReadFile(resultsPath)
	if readErr != nil {
		t.Fatalf("read results: %v", readErr)
	}
	dynamicBlock := managedBlockForTest(t, string(results), modelEvalDynamicResultsStart, modelEvalDynamicResultsEnd)
	enterpriseDynamicBlock := managedBlockForTest(t, string(results), modelEvalEnterpriseDynamicResultsStart, modelEvalEnterpriseDynamicResultsEnd)
	if !strings.Contains(dynamicBlock, "### Docker CE-on-Enterprise dynamic 2026 targeted") || !strings.Contains(dynamicBlock, "| `openai:gpt-5.4-nano` | `docker-read`") || strings.Contains(dynamicBlock, "docker-enterprise-read") {
		t.Fatalf("dynamic block = %s", dynamicBlock)
	}
	if !strings.Contains(enterpriseDynamicBlock, "### Docker Enterprise dynamic 2026 targeted") || !strings.Contains(enterpriseDynamicBlock, "| `openai:gpt-5.4-nano` | `docker-enterprise-read`") || strings.Contains(enterpriseDynamicBlock, "| `openai:gpt-5.4-nano` | `docker-read`") {
		t.Fatalf("enterprise dynamic block = %s", enterpriseDynamicBlock)
	}
	readme, readErr := os.ReadFile(readmePath)
	if readErr != nil {
		t.Fatalf("read readme: %v", readErr)
	}
	if !strings.Contains(string(readme), "Current published result: **Docker CE-on-Enterprise dynamic 2026 targeted**.") || !strings.Contains(string(readme), "Current published result: **Docker Enterprise dynamic 2026 targeted**.") {
		t.Fatalf("readme labels = %s", readme)
	}
}

// TestReadPublishReport_AllowsLargeTraceLines verifies provider-body traces do not exceed the publisher scanner buffer.
func TestReadPublishReport_AllowsLargeTraceLines(t *testing.T) {
	tmp := t.TempDir()
	traceDir := filepath.Join(tmp, "traces")
	if err := os.MkdirAll(traceDir, 0o700); err != nil {
		t.Fatalf("mkdir traces: %v", err)
	}
	largeBody := strings.Repeat("x", maxResponseBytes+1)
	trace := fmt.Sprintf(`{"run":1,"model":"openai:gpt-5.4-nano","task_id":"MT-001","expected":[{"step":1,"tool":"gitlab_execute_action","action":"user.current"}],"events":[{"kind":"assistant_message","content":%q,"usage":{"input_tokens":10,"output_tokens":2}}],"summary":{"first_tool":"gitlab_execute_action","first_action":"user.current","first_pass":true,"final_success":true,"destructive_safe":true,"expected_steps":1,"model_calls":1,"tool_calls":1}}`, largeBody) + "\n"
	if err := os.WriteFile(filepath.Join(traceDir, "traces.jsonl"), []byte(trace), 0o600); err != nil {
		t.Fatalf("write traces: %v", err)
	}
	reportPath := filepath.Join(tmp, "dynamic-report.md")
	if err := os.WriteFile(reportPath, []byte(dynamicFullRunPublishReportNoPreset()), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	report, err := readPublishReport(reportPath)
	if err != nil {
		t.Fatalf("readPublishReport() error = %v", err)
	}
	if len(report.Rows) != 1 || report.Rows[0].Attempts != 1 {
		t.Fatalf("rows = %+v, want one row with one attempt", report.Rows)
	}
	traces, err := readTraceJSONL(filepath.Join(traceDir, "traces.jsonl"))
	if err != nil {
		t.Fatalf("readTraceJSONL() error = %v", err)
	}
	if got := traces[0].Events[0].Content; got != largeBody {
		t.Fatalf("trace content length = %d, want %d", len(got), len(largeBody))
	}
}

// TestPublishFormattingHelpers_CoverBranchLabels verifies small formatting and
// classification helpers used by the generated README and results blocks.
func TestPublishFormattingHelpers_CoverBranchLabels(t *testing.T) {
	if presetRank(presetDockerRead) >= presetRank(presetDockerMutatingSafe) || presetRank("unknown") != 99 {
		t.Fatalf("unexpected preset ranks: read=%d mutating=%d unknown=%d", presetRank(presetDockerRead), presetRank(presetDockerMutatingSafe), presetRank("unknown"))
	}
	orderedPresets := []string{
		presetDockerRead,
		presetDockerMutatingSafe,
		presetDockerDestructiveSafe,
		presetDockerEnterpriseRead,
		presetDockerEnterpriseMutatingSafe,
		presetDockerEnterpriseDestructiveSafe,
		partitionErrorRecovery,
		partitionCapabilityFallback,
		presetSchemaEnterprise,
	}
	for index, preset := range orderedPresets {
		wantRank := index + 1
		if got := presetRank(preset); got != wantRank {
			t.Fatalf("presetRank(%q) = %d, want %d", preset, got, wantRank)
		}
	}

	providerCases := map[string][2]string{
		"anthropic:claude": {"Anthropic", "claude"},
		"google:gemini":    {"Google", "gemini"},
		"openai:gpt":       {"OpenAI", "gpt"},
		"qwen:qwen-max":    {"Qwen", "qwen-max"},
		"mistral:large":    {"Mistral", "large"},
		":nameless":        {"Unknown", "nameless"},
		"plain-model":      {"Unknown", "plain-model"},
	}
	for input, want := range providerCases {
		provider, model := providerModel(input)
		if provider != want[0] || model != want[1] {
			t.Fatalf("providerModel(%q) = %q/%q, want %q/%q", input, provider, model, want[0], want[1])
		}
	}

	if got := dockerLiveStatus(publishModelSummary{DockerBacked: false}); got != "Not Docker-backed" {
		t.Fatalf("dockerLiveStatus(non-docker) = %q", got)
	}
	if got := publishSectionForReport(publishReport{ToolSurface: config.ToolSurfaceDynamic}); got != publishSectionDynamic {
		t.Fatalf("publishSectionForReport(dynamic) = %q, want dynamic", got)
	}
	if got := publishSectionForReport(publishReport{ToolSurface: config.ToolSurfaceMeta, Preset: presetDockerEnterpriseRead}); got != publishSectionEnterpriseMeta {
		t.Fatalf("publishSectionForReport(enterprise meta) = %q, want enterprise-meta", got)
	}
	if got := publishSectionForReport(publishReport{ToolSurface: "experimental"}); got != publishSectionUnknown {
		t.Fatalf("publishSectionForReport(unknown) = %q, want unknown", got)
	}
	if got := dockerBackendLabel(publishRow{}); got != "-" {
		t.Fatalf("dockerBackendLabel(empty) = %q", got)
	}
	if got := compatibilityLabel(publishModelSummary{ToolSelection: 100, ActionSelection: 50, FinalSuccess: 100}); got != "Review" {
		t.Fatalf("compatibilityLabel(partial) = %q", got)
	}
	if got := formatRecoverySummary(publishModelSummary{RepairAttempts: 2, RepairSuccesses: 1, RepairSuccess: 50}); got != "50.0% (1/2)" {
		t.Fatalf("formatRecoverySummary() = %q", got)
	}
	if got := publishCostTokens(map[string]string{usageInputTokens: "10", usageOutputTokens: "5", usageEstimatedCost: "$0.01"}); got != "in 10 / out 5; $0.01" {
		t.Fatalf("publishCostTokens() = %q", got)
	}
	if got := firstPositive(0, -1, 7); got != 7 {
		t.Fatalf("firstPositive() = %d, want 7", got)
	}
}

// TestPublishSnapshotLabel_CoversFallbacks verifies explicit, RFC3339, and
// non-RFC3339 labels are generated predictably from reports.
func TestPublishSnapshotLabel_CoversFallbacks(t *testing.T) {
	if got := publishSnapshotLabel(" explicit label ", nil); got != "explicit label" {
		t.Fatalf("explicit label = %q", got)
	}
	if got := publishSnapshotLabel("", []publishReport{{Date: "2026-05-05T18:00:00Z"}}); got != "2026-05-05"+modelEvaluationSuffix {
		t.Fatalf("RFC3339 label = %q", got)
	}
	if got := publishSnapshotLabel("", []publishReport{{Date: "2026 week 18"}}); got != "2026 week 18"+modelEvaluationSuffix {
		t.Fatalf("fallback label = %q", got)
	}
}

// TestAppendSnapshotBlock_ReplacesExistingHeading verifies append mode replaces
// a matching snapshot instead of duplicating it.
func TestAppendSnapshotBlock_ReplacesExistingHeading(t *testing.T) {
	inner := "### Current\n\nold\n\n### Previous\n\nolder"
	replaced := appendSnapshotBlock(inner, "### Current\n\nnew", "Current")
	if strings.Contains(replaced, "\nold\n") || !strings.Contains(replaced, "### Previous") || !strings.Contains(replaced, "new") {
		t.Fatalf("appendSnapshotBlock(replace) = %q", replaced)
	}
	appended := appendSnapshotBlock(inner, "### New\n\nnew", "New")
	if !strings.HasPrefix(appended, "### New\n\nnew\n\n### Current") {
		t.Fatalf("appendSnapshotBlock(append) = %q", appended)
	}
}

// TestReplaceSnapshotByHeading_NotFound verifies a missing heading reports no
// replacement and leaves append mode free to prepend the snapshot.
func TestReplaceSnapshotByHeading_NotFound(t *testing.T) {
	if replaced, ok := replaceSnapshotByHeading("### One\n\none", "### Missing", "### Missing\n\nnew"); ok || replaced != "" {
		t.Fatalf("replaceSnapshotByHeading() = %q/%v, want empty false", replaced, ok)
	}
}

// TestGitCommonDir_ReadsRelativeCommonDir verifies worktree common-dir refs are
// followed when a ref is not present in the worktree metadata directory.
func TestGitCommonDir_ReadsRelativeCommonDir(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, "worktrees", "eval")
	commonDir := filepath.Join(root, "common")
	refPath := filepath.Join(commonDir, "refs", "heads", "main")
	if err := os.MkdirAll(filepath.Dir(refPath), 0o700); err != nil {
		t.Fatalf("mkdir ref dir: %v", err)
	}
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "commondir"), []byte("../../common\n"), 0o600); err != nil {
		t.Fatalf("write commondir: %v", err)
	}
	if err := os.WriteFile(refPath, []byte("fedcba9876543210fedcba9876543210fedcba98\n"), 0o600); err != nil {
		t.Fatalf("write common ref: %v", err)
	}
	if got := gitCommonDir(gitDir); got != commonDir {
		t.Fatalf("gitCommonDir() = %q, want %q", got, commonDir)
	}
	commit, err := readGitRef(gitDir, "refs/heads/main")
	if err != nil {
		t.Fatalf("readGitRef(common) error = %v", err)
	}
	if commit != "fedcba9876543210fedcba9876543210fedcba98" {
		t.Fatalf("commit = %q", commit)
	}
	if _, missingErr := readGitRef(filepath.Join(root, "missing-common"), "refs/heads/missing"); missingErr == nil {
		t.Fatal("readGitRef(missing) error = nil, want error")
	}
}

// TestUpdateManagedDoc_UnchangedAndMarkerErrors verifies unchanged documents
// are accepted and marker errors are wrapped with document context.
func TestUpdateManagedDoc_UnchangedAndMarkerErrors(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "doc.md")
	content := "# Doc\n\n" + modelEvalResultsStart + "\n### Current\n\nbody\n" + modelEvalResultsEnd + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	if err := updateManagedDoc(path, modelEvalResultsStart, modelEvalResultsEnd, "### Current\n\nbody", publishModeReplaceCurrent, "Current"); err != nil {
		t.Fatalf("updateManagedDoc(unchanged) error = %v", err)
	}
	badPath := filepath.Join(tmp, "bad.md")
	if err := os.WriteFile(badPath, []byte("# Doc\n"), 0o600); err != nil {
		t.Fatalf("write bad doc: %v", err)
	}
	if err := updateManagedDoc(badPath, modelEvalResultsStart, modelEvalResultsEnd, "block", publishModeReplaceCurrent, "Current"); err == nil || !strings.Contains(err.Error(), "missing marker") {
		t.Fatalf("updateManagedDoc(missing marker) error = %v, want missing marker", err)
	}
}

// TestPublishEvaluationDocs_RejectsInvalidInputs verifies top-level publish
// option validation and report read errors return before mutating documents.
func TestPublishEvaluationDocs_RejectsInvalidInputs(t *testing.T) {
	if err := publishEvaluationDocs(options{PublishMode: publishModeReplaceCurrent}); err == nil || !strings.Contains(err.Error(), "--publish-from") {
		t.Fatalf("publishEvaluationDocs(no reports) error = %v, want publish-from", err)
	}
	if err := publishEvaluationDocs(options{PublishFrom: stringList{"missing.md"}, PublishMode: "bad"}); err == nil || !strings.Contains(err.Error(), "--publish-mode") {
		t.Fatalf("publishEvaluationDocs(bad mode) error = %v, want publish-mode", err)
	}
	if _, err := readPublishReports([]string{"missing.md"}); err == nil {
		t.Fatal("readPublishReports(missing) error = nil, want error")
	}
}

// TestPublishParsingHelpers_CoverFallbacks verifies fallback branches for
// stats, usage, metrics, headings, and validation helpers.
func TestPublishParsingHelpers_CoverFallbacks(t *testing.T) {
	onlyStats := publishSingleTaskStats(map[string]publishTaskStats{"other": {Attempts: 3, ExpectedOps: 4}}, "missing", 9)
	if onlyStats.Attempts != 3 || onlyStats.ExpectedOps != 4 {
		t.Fatalf("publishSingleTaskStats(single fallback) = %+v", onlyStats)
	}
	fallbackStats := publishSingleTaskStats(nil, "missing", 9)
	if fallbackStats.Attempts != 9 {
		t.Fatalf("publishSingleTaskStats(fallback attempts) = %+v", fallbackStats)
	}
	usage := publishSingleUsage(nil, publishTaskStats{ModelRequests: 2, ToolCalls: 3})
	if usage[usageModelRequests] != "2" || usage[usageToolCallsEmitted] != "3" {
		t.Fatalf("publishSingleUsage() = %#v", usage)
	}

	content := "## Task Results\n\n| Run | Steps | Repair | Calls | Tool calls |\n| --- | --- | --- | --- | --- |\n| 1 | 3 | No | 2 | 1 |\n"
	stats := publishTaskStatsByModel(content, "default")
	if stats["default"].RepairAttempts != 1 || stats["default"].RepairSuccesses != 0 || stats["default"].ExpectedOps != 3 {
		t.Fatalf("publishTaskStatsByModel() = %#v", stats)
	}
	if got := publishMetricsByModel("## Per-Model Metrics\n\n| Model | Attempts |\n| --- | ---: |\n|  | 1 |\n"); len(got) != 0 {
		t.Fatalf("publishMetricsByModel(empty model) = %#v", got)
	}
	if got := publishUsageByModel("### API Usage By Model\n\n| Model | Requests |\n| --- | ---: |\n|  | 1 |\n"); len(got) != 0 {
		t.Fatalf("publishUsageByModel(empty model) = %#v", got)
	}
	if lines := sectionLinesForHeading(strings.Split("# One\nbody", "\n"), "not a heading"); lines != nil {
		t.Fatalf("sectionLinesForHeading(invalid) = %#v", lines)
	}
	if markdownHeadingLevel("###NoSpace") != 0 || markdownHeadingLevel("body") != 0 {
		t.Fatal("markdownHeadingLevel() accepted invalid headings")
	}
	if parseExpectedOps("7") != 7 {
		t.Fatal("parseExpectedOps(no slash) != 7")
	}

	reports := []publishReport{{Path: "report.md", Backend: backendGitLab, ToolExecution: "dry-run", Rows: []publishRow{{Attempts: 1}}}}
	if err := validatePublishReports(reports, "targeted", false); err == nil || !strings.Contains(err.Error(), "--execute-tools") {
		t.Fatalf("validatePublishReports(backend) error = %v, want execute-tools", err)
	}
	reports = []publishReport{{Path: "report.md", ToolExecution: "mcp", UnresolvedHarnessNoise: true, Rows: []publishRow{{Attempts: 1}}}}
	if err := validatePublishReports(reports, "targeted", false); err == nil || !strings.Contains(err.Error(), "harness noise") {
		t.Fatalf("validatePublishReports(noise) error = %v, want harness noise", err)
	}
	if err := validatePublishReports(reports, "targeted", true); err != nil {
		t.Fatalf("validatePublishReports(allow noise) error = %v", err)
	}
}

// TestPublishOrderingAndSummaryFallbacks verifies row ordering, row summaries,
// and empty aggregate branches.
func TestPublishOrderingAndSummaryFallbacks(t *testing.T) {
	reports := []publishReport{{Rows: []publishRow{
		{Model: "z", Preset: "custom", SourcePath: "b"},
		{Model: "a", Preset: presetDockerRead, SourcePath: "a"},
		{Model: "m", Preset: presetSchemaEnterprise, SourcePath: "c"},
	}}}
	rows := sortedPublishRows(reports)
	if rows[0].Model != "a" || rows[1].Model != "m" || rows[2].Model != "z" {
		t.Fatalf("sorted rows = %#v", rows)
	}
	if aggregate := aggregatePublishRows(nil); aggregate.Attempts != 0 {
		t.Fatalf("aggregate empty = %+v", aggregate)
	}
	summaries := publishSummariesByModel([]publishRow{{Model: "openai:gpt", Backend: "dry", ToolExecution: "none", Attempts: 1, FinalSuccess: 50}})
	if len(summaries) != 1 || summaries[0].DockerBacked {
		t.Fatalf("summaries = %#v, want non-Docker", summaries)
	}
	if got := dockerBackendLabel(publishRow{Backend: "local|backend"}); got != "local\\|backend" {
		t.Fatalf("dockerBackendLabel(escape) = %q", got)
	}
	if got := rowCommitBranchDate(publishRow{}); got != "- / - / -" {
		t.Fatalf("rowCommitBranchDate(empty) = %q", got)
	}
	if got := publishSnapshotLabel("", nil); !strings.HasSuffix(got, modelEvaluationSuffix) {
		t.Fatalf("publishSnapshotLabel(now) = %q", got)
	}
}

// TestManagedDocErrorBranches verifies read, check, and marker error paths.
func TestManagedDocErrorBranches(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.md")
	if err := updateManagedDoc(missing, modelEvalResultsStart, modelEvalResultsEnd, "block", publishModeReplaceCurrent, "Current"); err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("updateManagedDoc(missing) error = %v, want read", err)
	}
	if err := checkManagedDoc(missing, modelEvalResultsStart, modelEvalResultsEnd, "block", publishModeReplaceCurrent, "Current"); err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("checkManagedDoc(missing) error = %v, want read", err)
	}
	if _, err := readTextFile(missing); err == nil {
		t.Fatal("readTextFile(missing) error = nil, want error")
	}
	content := modelEvalResultsStart + "\nbody\n"
	if _, err := applyManagedBlock(content, modelEvalResultsStart, modelEvalResultsEnd, "block", publishModeReplaceCurrent, "Current"); err == nil || !strings.Contains(err.Error(), "missing marker") {
		t.Fatalf("applyManagedBlock(missing end) error = %v, want missing marker", err)
	}
	if got := appendSnapshotBlock("", "### Current\n\nbody", "Current"); got != "### Current\n\nbody\n" {
		t.Fatalf("appendSnapshotBlock(empty) = %q", got)
	}
}

// TestGitMetadataFallbackBranches verifies detached HEAD, missing git metadata,
// direct .git directories, invalid .git files, packed-ref miss handling, and
// short commit passthrough.
func TestGitMetadataFallbackBranches(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("abcdef0123456789abcdef0123456789abcdef01\n"), 0o600); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if resolved, err := resolveGitDir(root); err != nil || resolved != gitDir {
		t.Fatalf("resolveGitDir(dir) = %q, %v", resolved, err)
	}
	t.Chdir(root)
	branch, commit := currentGitReportMetadata()
	if branch != "HEAD" || commit != "abcdef012345" {
		t.Fatalf("currentGitReportMetadata(detached) = %q/%q", branch, commit)
	}

	missingRoot := t.TempDir()
	t.Chdir(missingRoot)
	branch, commit = currentGitReportMetadata()
	if branch != "" || commit != "" {
		t.Fatalf("currentGitReportMetadata(missing) = %q/%q", branch, commit)
	}

	invalidRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(invalidRoot, ".git"), []byte("not-a-gitdir"), 0o600); err != nil {
		t.Fatalf("write invalid git file: %v", err)
	}
	if _, err := resolveGitDir(invalidRoot); err == nil {
		t.Fatal("resolveGitDir(invalid file) error = nil, want error")
	}
	if got, ok := readPackedGitRef(gitDir, "refs/heads/missing"); ok || got != "" {
		t.Fatalf("readPackedGitRef(missing) = %q/%v, want empty false", got, ok)
	}
	if got := shortGitCommit("abc123"); got != "abc123" {
		t.Fatalf("shortGitCommit(short) = %q", got)
	}
}

// writeTempPublishReport writes temp publish report fixture data for tests.
func writeTempPublishReport(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	return path
}

// singleModelPublishReport supports single model publish report assertions in main tests.
func singleModelPublishReport(model, preset string, attempts int) string {
	return singleModelPublishReportForSurface(model, preset, attempts, config.ToolSurfaceMeta)
}

// singleModelPublishReportForSurface supports single model publish report for surface assertions in main tests.
func singleModelPublishReportForSurface(model, preset string, attempts int, toolSurface string) string {
	var rows strings.Builder
	for i := 1; i <= attempts; i++ {
		steps := "1/1"
		if i == attempts {
			steps = "2/2"
		}
		fmt.Fprintf(&rows, "| 1 | MT-001 | `gitlab_project` / `get` | `gitlab_project` / `get` | %s | No | Yes | - | Yes | 1 | 1 | - |\n", steps)
	}
	expectedOps := attempts + 1
	return "# Meta-Tool Model Evaluation\n\n" +
		"Date: 2026-05-05T18:00:00Z\n" +
		"Git branch: `port/main-small-meta-fixes`\n" +
		"Git commit: `8c696a2`\n" +
		"Mode: model tool-calling\n" +
		"Model: `" + model + "`\n" +
		"Tool surface: `" + toolSurface + "`\n" +
		"Backend: `gitlab`\n" +
		"Preset: `" + preset + "`\n" +
		"Tool execution: `mcp`\n" +
		"Catalog tools: 33\n" +
		"Runs: 1\n" +
		"Task attempts: " + strconv.Itoa(attempts) + "\n\n" +
		"## Metrics\n\n" +
		"| Metric | Value |\n| --- | ---: |\n" +
		"| Tool-selection accuracy | 100.0% |\n" +
		"| Action-selection accuracy | 100.0% |\n" +
		"| First-call validation pass rate | 100.0% |\n" +
		"| Schema lookup use rate | 0.0% |\n" +
		"| Repair success rate | 100.0% |\n" +
		"| Destructive safety | 100.0% |\n" +
		"| Final task success proxy | 100.0% |\n" +
		"\n## API Usage\n\n" +
		"| Metric | Value |\n| --- | ---: |\n" +
		"| Model requests | " + strconv.Itoa(expectedOps) + " |\n" +
		"| Tool calls emitted | " + strconv.Itoa(expectedOps) + " |\n" +
		"| Input tokens | 100 |\n" +
		"| Output tokens | 20 |\n" +
		"| Estimated cost | Not configured |\n" +
		"\n## Task Results\n\n" +
		"| Run | Task | Expected | First final call | Steps | Schema lookup | First pass | Repair | Final success | Calls | Tool calls | Notes |\n" +
		"| ---: | --- | --- | --- | ---: | --- | --- | --- | --- | ---: | ---: | --- |\n" +
		rows.String()
}

// dynamicSingleModelPublishReport supports dynamic single model publish report assertions in main tests.
func dynamicSingleModelPublishReport(model, preset string, attempts int) string {
	return singleModelPublishReportForSurface(model, preset, attempts, config.ToolSurfaceDynamic)
}

// multiModelPublishReport supports multi model publish report assertions in main tests.
func multiModelPublishReport() string {
	return "# Meta-Tool Model Evaluation\n\n" +
		"Date: 2026-05-05T18:00:00Z\n" +
		"Mode: model tool-calling\n" +
		"Model: `anthropic:claude-haiku-4-5-20251001,google:gemini-3.1-flash-lite-preview`\n" +
		"Tool surface: `meta`\n" +
		"Backend: `gitlab`\n" +
		"Preset: `docker-read`\n" +
		"Tool execution: `mcp`\n" +
		"Catalog tools: 33\n" +
		"Runs: 1\n" +
		"Task attempts: 4\n\n" +
		"## Metrics\n\n" +
		"| Metric | Value |\n| --- | ---: |\n" +
		"| Tool-selection accuracy | 100.0% |\n" +
		"| Action-selection accuracy | 100.0% |\n" +
		"| First-call validation pass rate | 100.0% |\n" +
		"| Repair success rate | 100.0% |\n" +
		"| Destructive safety | 100.0% |\n" +
		"| Final task success proxy | 100.0% |\n" +
		"\n## Per-Model Metrics\n\n" +
		"| Model | Attempts | Tool | Action | First pass | Schema lookup | Repair success | Destructive safety | Final success |\n" +
		"| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n" +
		"| `anthropic:claude-haiku-4-5-20251001` | 2 | 100.0% | 100.0% | 100.0% | 0.0% | 100.0% | 100.0% | 100.0% |\n" +
		"| `google:gemini-3.1-flash-lite-preview` | 2 | 100.0% | 100.0% | 100.0% | 0.0% | 100.0% | 100.0% | 100.0% |\n" +
		"\n## API Usage\n\n" +
		"| Metric | Value |\n| --- | ---: |\n" +
		"| Model requests | 4 |\n" +
		"| Tool calls emitted | 4 |\n" +
		"\n### API Usage By Model\n\n" +
		"| Model | Requests | Tool calls | Input tokens | Output tokens | Estimated cost |\n" +
		"| --- | ---: | ---: | ---: | ---: | ---: |\n" +
		"| `anthropic:claude-haiku-4-5-20251001` | 2 | 2 | 100 | 20 | Not configured |\n" +
		"| `google:gemini-3.1-flash-lite-preview` | 2 | 2 | 100 | 20 | Not configured |\n" +
		"\n## Task Results\n\n" +
		"| Model | Run | Task | Expected | First final call | Steps | Schema lookup | First pass | Repair | Final success | Calls | Tool calls | Notes |\n" +
		"| --- | ---: | --- | --- | --- | ---: | --- | --- | --- | --- | ---: | ---: | --- |\n" +
		"| `anthropic:claude-haiku-4-5-20251001` | 1 | MT-001 | `gitlab_project` / `get` | `gitlab_project` / `get` | 1/1 | No | Yes | - | Yes | 1 | 1 | - |\n" +
		"| `anthropic:claude-haiku-4-5-20251001` | 1 | MT-002 | `gitlab_project` / `list` | `gitlab_project` / `list` | 1/1 | No | Yes | - | Yes | 1 | 1 | - |\n" +
		"| `google:gemini-3.1-flash-lite-preview` | 1 | MT-001 | `gitlab_project` / `get` | `gitlab_project` / `get` | 1/1 | No | Yes | - | Yes | 1 | 1 | - |\n" +
		"| `google:gemini-3.1-flash-lite-preview` | 1 | MT-002 | `gitlab_project` / `list` | `gitlab_project` / `list` | 1/1 | No | Yes | - | Yes | 1 | 1 | - |\n"
}

// dynamicFullRunPublishReportNoPreset returns a minimal dynamic report fixture without preset metadata.
func dynamicFullRunPublishReportNoPreset() string {
	return "# Meta-Tool Model Evaluation\n\n" +
		"Date: 2026-05-09T18:00:00Z\n" +
		"Mode: model tool-calling\n" +
		"Model: `openai:gpt-5.4-nano`\n" +
		"Tool surface: `dynamic`\n" +
		"Backend: `gitlab`\n" +
		"Tool execution: `mcp`\n" +
		"Catalog tools: 3\n" +
		"Runs: 1\n" +
		"Task attempts: 3\n\n" +
		"Trace artifacts: `traces`\n\n" +
		"## Metrics\n\n" +
		"| Metric | Value |\n| --- | ---: |\n" +
		"| Tool-selection accuracy | 100.0% |\n" +
		"| Action-selection accuracy | 100.0% |\n" +
		"| First-call validation pass rate | 100.0% |\n" +
		"| Repair success rate | 100.0% |\n" +
		"| Destructive safety | 100.0% |\n" +
		"| Final task success proxy | 100.0% |\n" +
		"\n## Task Results\n\n" +
		"| Run | Task | Expected | First final call | Steps | Schema lookup | First pass | Repair | Final success | Calls | Tool calls | Notes |\n" +
		"| ---: | --- | --- | --- | ---: | --- | --- | --- | --- | ---: | ---: | --- |\n" +
		"| 1 | MT-001 | `gitlab_execute_action` / `user.current` | `gitlab_execute_action` / `user.current` | 1/1 | No | Yes | - | Yes | 1 | 1 | - |\n" +
		"| 1 | MT-010 | `gitlab_execute_action` / `issue.create` | `gitlab_execute_action` / `issue.create` | 1/1 | No | Yes | - | Yes | 2 | 2 | - |\n" +
		"| 1 | MT-008 | `gitlab_execute_action` / `group.delete` | `gitlab_execute_action` / `group.delete` | 1/1 | No | Yes | - | Yes | 1 | 1 | - |\n"
}

// dynamicEnterpriseFullRunPublishReportNoPreset returns a combined Dynamic
// report fixture generated against an Enterprise runtime with no preset metadata.
func dynamicEnterpriseFullRunPublishReportNoPreset() string {
	return strings.Replace(dynamicFullRunPublishReportNoPreset(), "Tool surface: `dynamic`\n", "Tool surface: `dynamic`\nEdition: `enterprise`\n", 1)
}

// fullRunTraceJSONL returns trace rows for all tasks in the publish report fixture.
func fullRunTraceJSONL() string {
	return strings.Join([]string{
		`{"run":1,"model":"openai:gpt-5.4-nano","task_id":"MT-001","expected":[{"step":1,"tool":"gitlab_execute_action","action":"user.current"}],"events":[{"usage":{"input_tokens":10,"output_tokens":2}}],"summary":{"first_tool":"gitlab_execute_action","first_action":"user.current","first_pass":true,"final_success":true,"destructive_safe":true,"expected_steps":1,"model_calls":1,"tool_calls":1}}`,
		`{"run":1,"model":"openai:gpt-5.4-nano","task_id":"MT-010","expected":[{"step":1,"tool":"gitlab_execute_action","action":"issue.create"}],"events":[{"usage":{"input_tokens":15,"output_tokens":4}}],"summary":{"first_tool":"gitlab_execute_action","first_action":"issue.create","first_pass":true,"final_success":true,"destructive_safe":true,"expected_steps":1,"model_calls":2,"tool_calls":2}}`,
		`{"run":1,"model":"openai:gpt-5.4-nano","task_id":"MT-008","expected":[{"step":1,"tool":"gitlab_execute_action","action":"group.delete","destructive":true}],"events":[{"usage":{"input_tokens":11,"output_tokens":3}}],"summary":{"first_tool":"gitlab_execute_action","first_action":"group.delete","first_pass":true,"final_success":true,"destructive_safe":true,"expected_steps":1,"model_calls":1,"tool_calls":1}}`,
	}, "\n") + "\n"
}

// combinedEnterpriseFullRunTraceJSONL returns one CE and one Enterprise trace row.
func combinedEnterpriseFullRunTraceJSONL() string {
	return strings.Join([]string{
		`{"run":1,"model":"openai:gpt-5.4-nano","task_id":"MT-001","expected":[{"step":1,"tool":"gitlab_execute_action","action":"user.current"}],"events":[{"usage":{"input_tokens":10,"output_tokens":2}}],"summary":{"first_tool":"gitlab_execute_action","first_action":"user.current","first_pass":true,"final_success":true,"destructive_safe":true,"expected_steps":1,"model_calls":1,"tool_calls":1}}`,
		`{"run":1,"model":"openai:gpt-5.4-nano","task_id":"MT-188","expected":[{"step":1,"tool":"gitlab_execute_action","action":"project.security_settings_get"}],"events":[{"usage":{"input_tokens":12,"output_tokens":3}}],"summary":{"first_tool":"gitlab_execute_action","first_action":"project.security_settings_get","first_pass":true,"final_success":true,"destructive_safe":true,"expected_steps":1,"model_calls":1,"tool_calls":1}}`,
	}, "\n") + "\n"
}

func managedBlockForTest(t *testing.T, content, startMarker, endMarker string) string {
	t.Helper()
	start := strings.Index(content, startMarker)
	if start == -1 {
		t.Fatalf("missing start marker %s in %s", startMarker, content)
	}
	start += len(startMarker)
	end := strings.Index(content[start:], endMarker)
	if end == -1 {
		t.Fatalf("missing end marker %s in %s", endMarker, content)
	}
	return content[start : start+end]
}
