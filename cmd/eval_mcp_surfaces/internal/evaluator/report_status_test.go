// report_status_test.go covers the --check-report-clean gate in
// report_status.go: report loading, failed-row detection across several
// reports, and the one-line summary printed for each failed row.

package evaluator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCleanCheckReport writes a minimal evaluation report whose Task Results
// table holds the given rows and returns its path.
func writeCleanCheckReport(t *testing.T, name string, rows ...string) string {
	t.Helper()
	content := "# Report\n\n## Task Results\n\n| Model | Run | Task | Final success | Notes |\n| --- | ---: | --- | --- | --- |\n" + strings.Join(rows, "\n") + "\n"
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	return path
}

// TestRunReportCleanCheck_MixedReports_ReportsFailedCount verifies the gate
// scans every report, passes the clean one, and returns an error naming how
// many reports still contain failed task rows.
func TestRunReportCleanCheck_MixedReports_ReportsFailedCount(t *testing.T) {
	clean := writeCleanCheckReport(t, "clean.md", "| `a:b` | 1 | MT-001 | Yes | - |")
	failed := writeCleanCheckReport(t, "failed.md", "| `a:b` | 1 | MT-001 | Yes | - |", "| `a:b` | 2 | MT-002 | No | fixture preparation failed |")

	err := runReportCleanCheck(options{CheckReportClean: stringList{clean, failed}})
	if err == nil || !strings.Contains(err.Error(), "1 report(s) contain failed task rows") {
		t.Fatalf("runReportCleanCheck() error = %v, want one failed report", err)
	}
}

// TestRunReportCleanCheck_AllClean_ReturnsNil verifies a set of clean reports
// passes the gate.
func TestRunReportCleanCheck_AllClean_ReturnsNil(t *testing.T) {
	clean := writeCleanCheckReport(t, "clean.md", "| `a:b` | 1 | MT-001 | Yes | - |")
	if err := runReportCleanCheck(options{CheckReportClean: stringList{clean}}); err != nil {
		t.Fatalf("runReportCleanCheck() error = %v, want nil", err)
	}
}

// TestRunReportCleanCheck_MissingReport_ReturnsReadError verifies a report
// path that cannot be read aborts the gate with the path in the error.
func TestRunReportCleanCheck_MissingReport_ReturnsReadError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.md")
	err := runReportCleanCheck(options{CheckReportClean: stringList{missing}})
	if err == nil || !strings.Contains(err.Error(), "read report "+missing) {
		t.Fatalf("runReportCleanCheck() error = %v, want read error for %s", err, missing)
	}
}

// TestCheckReportClean_MissingTaskResults_ReturnsErrorWithPath verifies a
// report without a Task Results table is rejected and the returned status
// still records the path it came from.
func TestCheckReportClean_MissingTaskResults_ReturnsErrorWithPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(path, []byte("# Startup failure\n"), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
	status, err := checkReportClean(path)
	if err == nil || !strings.Contains(err.Error(), "check report "+path) {
		t.Fatalf("checkReportClean() error = %v, want check error", err)
	}
	if status.Path != path {
		t.Fatalf("status.Path = %q, want %q", status.Path, path)
	}
}

// TestReportFailedRowSummary_JoinsPresentFields verifies the failed-row
// summary includes only the populated fields and skips placeholder notes.
func TestReportFailedRowSummary_JoinsPresentFields(t *testing.T) {
	cases := []struct {
		name string
		row  reportFailedRow
		want string
	}{
		{name: "all fields", row: reportFailedRow{Model: "a:b", Run: "1", Task: "MT-1", FinalSuccess: "No", Notes: "boom"}, want: "model=a:b run=1 task=MT-1 final_success=No notes=boom"},
		{name: "placeholder notes", row: reportFailedRow{Task: "MT-1", Notes: "-"}, want: "task=MT-1"},
		{name: "empty row", row: reportFailedRow{}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.row.summary(); got != tc.want {
				t.Fatalf("summary() = %q, want %q", got, tc.want)
			}
		})
	}
}
