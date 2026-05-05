package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestReadPublishReport_ParsesSingleModelReport verifies that ReadPublishReport handles the parses single model report scenario correctly.
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

// TestReadPublishReport_ParsesPerModelRows verifies that ReadPublishReport handles the parses per model rows scenario correctly.
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

// TestValidatePublishReports_RejectsPartialDockerPresetWithoutTargetedLabel verifies that ValidatePublishReports handles the rejects partial docker preset without targeted label scenario correctly.
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

// TestSortedPublishRows_ReplacesDuplicateModelPresetRows verifies that SortedPublishRows handles the replaces duplicate model preset rows scenario correctly.
func TestSortedPublishRows_ReplacesDuplicateModelPresetRows(t *testing.T) {
	oldPath := writeTempPublishReport(t, singleModelPublishReport("google:gemini-3.1-flash-lite-preview", presetDockerMutatingSafe, 25))
	newPath := writeTempPublishReport(t, singleModelPublishReport("google:gemini-3.1-flash-lite-preview", presetDockerMutatingSafe, 25))
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

// TestAggregatePublishRows_RepairSuccessUsesRepairAttempts verifies that AggregatePublishRows handles the repair success uses repair attempts scenario correctly.
func TestAggregatePublishRows_RepairSuccessUsesRepairAttempts(t *testing.T) {
	rows := []publishRow{
		{Attempts: 40, RepairSuccess: 100},
		{Attempts: 53, RepairSuccess: 50, RepairAttempts: 4, RepairSuccesses: 2},
	}

	aggregate := aggregatePublishRows(rows)
	if aggregate.RepairSuccess != 50 {
		t.Fatalf("aggregate repair = %.1f, want repair-attempt ratio 50.0", aggregate.RepairSuccess)
	}
	if got := formatRepairMetric(aggregate); got != "50.0% (2/4)" {
		t.Fatalf("formatRepairMetric() = %q, want count-qualified percentage", got)
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

// TestApplyManagedBlock_ReplacesAndAppendsSnapshots verifies that ApplyManagedBlock handles the replaces and appends snapshots scenario correctly.
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

// TestPublishEvaluationDocs_WritesAndChecksManagedDocs verifies that PublishEvaluationDocs handles the writes and checks managed docs scenario correctly.
func TestPublishEvaluationDocs_WritesAndChecksManagedDocs(t *testing.T) {
	tmp := t.TempDir()
	reportPath := filepath.Join(tmp, "report.md")
	if err := os.WriteFile(reportPath, []byte(singleModelPublishReport("openai:gpt-5.4-nano", presetDockerRead, 40)), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	resultsPath := filepath.Join(tmp, "model-results.md")
	readmePath := filepath.Join(tmp, "README.md")
	if err := os.WriteFile(resultsPath, []byte("# Results\n\n"+modelEvalResultsStart+"\n"+modelEvalResultsEnd+"\n"), 0o600); err != nil {
		t.Fatalf("write results doc: %v", err)
	}
	if err := os.WriteFile(readmePath, []byte("# README\n\n"+modelEvalSummaryStart+"\n"+modelEvalSummaryEnd+"\n"), 0o600); err != nil {
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
	if !strings.Contains(string(results), "| `openai:gpt-5.4-nano` | `docker-read` | Docker GitLab via MCP | 40 | 41 | 41 | 41 |") {
		t.Fatalf("results doc = %s", results)
	}
	if strings.Contains(string(results), "Source reports") || strings.Contains(string(results), reportPath) {
		t.Fatalf("results doc leaked local report paths: %s", results)
	}
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	if !strings.Contains(string(readme), "| OpenAI | `gpt-5.4-nano` | OK | 100.0% | No repairs | 100.0% final across 41 ops |") {
		t.Fatalf("readme = %s", readme)
	}

	opts.CheckDocs = true
	opts.PublishDocs = false
	if publishErr := publishEvaluationDocs(opts); publishErr != nil {
		t.Fatalf("publishEvaluationDocs(check) error = %v", publishErr)
	}
	if writeErr := os.WriteFile(readmePath, []byte("# README\n\n"+modelEvalSummaryStart+"\nstale\n"+modelEvalSummaryEnd+"\n"), 0o600); writeErr != nil {
		t.Fatalf("write stale readme: %v", writeErr)
	}
	if checkErr := publishEvaluationDocs(opts); checkErr == nil || !strings.Contains(checkErr.Error(), "not up to date") {
		t.Fatalf("publishEvaluationDocs(stale check) error = %v, want not up to date", checkErr)
	}
}

// writeTempPublishReport is an internal helper for the main package.
func writeTempPublishReport(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	return path
}

// singleModelPublishReport is an internal helper for the main package.
func singleModelPublishReport(model, preset string, attempts int) string {
	var rows strings.Builder
	for i := 1; i <= attempts; i++ {
		steps := "1/1"
		if i == attempts {
			steps = "2/2"
		}
		rows.WriteString("| 1 | MT-001 | `gitlab_project` / `get` | `gitlab_project` / `get` | " + steps + " | No | Yes | - | Yes | 1 | 1 | - |\n")
	}
	expectedOps := attempts + 1
	return "# Meta-Tool Model Evaluation\n\n" +
		"Date: 2026-05-05T18:00:00Z\n" +
		"Git branch: `port/main-small-meta-fixes`\n" +
		"Git commit: `8c696a2`\n" +
		"Mode: model tool-calling\n" +
		"Model: `" + model + "`\n" +
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

// multiModelPublishReport is an internal helper for the main package.
func multiModelPublishReport() string {
	return "# Meta-Tool Model Evaluation\n\n" +
		"Date: 2026-05-05T18:00:00Z\n" +
		"Mode: model tool-calling\n" +
		"Model: `anthropic:claude-haiku-4-5-20251001,google:gemini-3.1-flash-lite-preview`\n" +
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
