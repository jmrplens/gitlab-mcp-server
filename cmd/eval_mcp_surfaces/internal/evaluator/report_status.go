package evaluator

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type reportCleanStatus struct {
	Path       string
	TotalRows  int
	FailedRows []reportFailedRow
}

type reportFailedRow struct {
	Model        string
	Run          string
	Task         string
	FinalSuccess string
	Notes        string
}

func runReportCleanCheck(opts options) error {
	var failed []reportCleanStatus
	for _, path := range opts.CheckReportClean {
		status, err := checkReportClean(path)
		if err != nil {
			return err
		}
		if status.clean() {
			fmt.Fprintf(os.Stdout, "%s: report_clean rows=%d failed=0\n", path, status.TotalRows)
			continue
		}
		failed = append(failed, status)
		fmt.Fprintf(os.Stdout, "%s: report_failed rows=%d failed=%d\n", path, status.TotalRows, len(status.FailedRows))
		for _, row := range status.FailedRows {
			fmt.Fprintf(os.Stdout, "  %s\n", row.summary())
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return fmt.Errorf("%d report(s) contain failed task rows", len(failed))
}

func checkReportClean(path string) (reportCleanStatus, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- report path is an explicit evaluator input.
	if err != nil {
		return reportCleanStatus{}, fmt.Errorf("read report %s: %w", path, err)
	}
	status, err := checkReportCleanContent(string(data))
	status.Path = path
	if err != nil {
		return status, fmt.Errorf("check report %s: %w", path, err)
	}
	return status, nil
}

func checkReportCleanContent(content string) (reportCleanStatus, error) {
	rows := reportNamedTableRows(content, "## Task Results")
	if len(rows) == 0 {
		return reportCleanStatus{}, errors.New("task results table not found")
	}
	status := reportCleanStatus{TotalRows: len(rows)}
	for _, row := range rows {
		finalSuccess := row["Final success"]
		if strings.EqualFold(finalSuccess, "Yes") {
			continue
		}
		status.FailedRows = append(status.FailedRows, reportFailedRow{
			Model:        row["Model"],
			Run:          row["Run"],
			Task:         row["Task"],
			FinalSuccess: finalSuccess,
			Notes:        row["Notes"],
		})
	}
	return status, nil
}

func (s reportCleanStatus) clean() bool {
	return len(s.FailedRows) == 0
}

func (r reportFailedRow) summary() string {
	parts := []string{}
	if r.Model != "" {
		parts = append(parts, "model="+r.Model)
	}
	if r.Run != "" {
		parts = append(parts, "run="+r.Run)
	}
	if r.Task != "" {
		parts = append(parts, "task="+r.Task)
	}
	if r.FinalSuccess != "" {
		parts = append(parts, "final_success="+r.FinalSuccess)
	}
	if r.Notes != "" && r.Notes != "-" {
		parts = append(parts, "notes="+r.Notes)
	}
	return strings.Join(parts, " ")
}
