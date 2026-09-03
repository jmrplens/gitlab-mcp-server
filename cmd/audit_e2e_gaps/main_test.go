// main_test.go verifies the audit_e2e_gaps command output contract against
// the real catalog and the checked-in e2e suite sources.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// suiteDir resolves the repository's e2e suite directory relative to this
// package so the test works regardless of the working directory.
func suiteDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "test", "e2e", "suite"))
	if err != nil {
		t.Fatalf("resolve suite dir: %v", err)
	}
	return dir
}

// TestRun_TSVSummary verifies the audit runs cleanly against the real suite
// and prints the exercised/uncovered summary line.
func TestRun_TSVSummary(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run(&stdout, &stderr, suiteDir(t), "tsv"); exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "e2e gap audit:") {
		t.Fatalf("run() stdout missing summary line: %q", stdout.String())
	}
}

// TestRun_JSONReportShape verifies the JSON output decodes into the report
// shape with a sane exercised/uncovered partition of the catalog.
func TestRun_JSONReportShape(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run(&stdout, &stderr, suiteDir(t), "json"); exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	var rep report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if rep.CatalogActions == 0 {
		t.Fatal("report.CatalogActions = 0, want > 0")
	}
	if got := rep.Exercised + len(rep.Uncovered); got != rep.CatalogActions {
		t.Fatalf("exercised(%d) + uncovered(%d) = %d, want %d", rep.Exercised, len(rep.Uncovered), got, rep.CatalogActions)
	}
	if rep.Exercised == 0 {
		t.Fatal("report.Exercised = 0, want > 0 (suite exercises hundreds of actions)")
	}
}

// TestRun_InvalidOutputFormat verifies the usage exit code for a bad -output.
func TestRun_InvalidOutputFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run(&stdout, &stderr, suiteDir(t), "xml"); exitCode != 2 {
		t.Fatalf("run() exit code = %d, want 2", exitCode)
	}
}

// TestRun_MissingSuiteDir verifies the scan error path exits non-zero.
func TestRun_MissingSuiteDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run(&stdout, &stderr, t.TempDir(), "tsv"); exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), "scan suite") {
		t.Fatalf("stderr = %q, want scan suite error", stderr.String())
	}
}

// sparseSuite is a one-file suite that references exactly one action through
// each invocation shape (individual tool name, meta pair, dynamic id), so
// almost every catalog action is reported as uncovered.
const sparseSuite = `package suite

// gitlab_branch_delete is mentioned in a comment and must not count.
func TestSparse(t *testing.T) {
	call("gitlab_project_get")
	meta("gitlab_issue", map[string]any{
		"action": "list",
	})
	execute("branch.create")
}
`

// writeSparseSuite writes sparseSuite into a fresh directory and returns it.
func writeSparseSuite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sparse_test.go"), []byte(sparseSuite), 0o600); err != nil {
		t.Fatalf("write sparse suite: %v", err)
	}
	return dir
}

// TestRun_SparseSuite_ReportsUncoveredRows verifies the gap report against a
// suite that exercises three actions: the JSON report lists the rest as
// uncovered rows sorted by id, and the TSV output prints one row per
// uncovered action (defaulting a blank edition to "free") before the summary.
func TestRun_SparseSuite_ReportsUncoveredRows(t *testing.T) {
	dir := writeSparseSuite(t)

	var stdout, stderr bytes.Buffer
	if exitCode := run(&stdout, &stderr, dir, "json"); exitCode != 0 {
		t.Fatalf("run(json) exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	var rep report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if rep.Exercised != 3 {
		t.Errorf("Exercised = %d, want 3 (one per invocation shape)", rep.Exercised)
	}
	if len(rep.Uncovered) != rep.CatalogActions-3 {
		t.Errorf("Uncovered = %d, want %d", len(rep.Uncovered), rep.CatalogActions-3)
	}
	for i := 1; i < len(rep.Uncovered); i++ {
		if rep.Uncovered[i-1].ID >= rep.Uncovered[i].ID {
			t.Fatalf("Uncovered not sorted by id at %d: %q >= %q", i, rep.Uncovered[i-1].ID, rep.Uncovered[i].ID)
		}
	}
	for _, row := range rep.Uncovered {
		if row.ID == "project.get" || row.ID == "issue.list" || row.ID == "branch.create" {
			t.Errorf("row %q reported uncovered although the suite references it", row.ID)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := run(&stdout, &stderr, dir, "tsv"); exitCode != 0 {
		t.Fatalf("run(tsv) exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) != len(rep.Uncovered)+1 {
		t.Fatalf("tsv printed %d lines, want %d rows plus the summary", len(lines), len(rep.Uncovered)+1)
	}
	if !strings.Contains(lines[len(lines)-1], "e2e gap audit: 3/") {
		t.Errorf("summary line = %q, want the exercised count", lines[len(lines)-1])
	}
	freeRows := 0
	for _, line := range lines[:len(lines)-1] {
		fields := strings.Split(line, "\t")
		if len(fields) != 5 {
			t.Fatalf("tsv row %q has %d fields, want 5", line, len(fields))
		}
		if fields[2] == "" {
			t.Errorf("tsv row %q has an empty edition, want free", line)
		}
		if fields[2] == "free" {
			freeRows++
		}
	}
	if freeRows == 0 {
		t.Error("no tsv row defaulted its edition to free")
	}
}

// errWriter fails every write so the JSON encoder's error branch is reachable.
type errWriter struct{}

// Write always fails.
func (errWriter) Write([]byte) (int, error) { return 0, errors.New("stdout closed") }

// TestRun_JSONEncodeFails_ReportsOnStderr verifies a stdout that rejects the
// JSON report is turned into an "encode json" diagnostic and exit code 1.
func TestRun_JSONEncodeFails_ReportsOnStderr(t *testing.T) {
	var stderr bytes.Buffer
	if exitCode := run(errWriter{}, &stderr, writeSparseSuite(t), "json"); exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), "encode json: stdout closed") {
		t.Fatalf("stderr = %q, want the encode diagnostic", stderr.String())
	}
}

// TestScanSuite_UnusableSources_ReturnsError verifies the two scanner
// failures that are not "no test files": a suite path whose glob pattern is
// malformed, and a suite entry that matches *_test.go but cannot be read
// because it is a directory.
func TestScanSuite_UnusableSources_ReturnsError(t *testing.T) {
	tests := []struct {
		name    string
		suite   func(t *testing.T) string
		wantErr string
	}{
		{
			name: "malformed glob pattern",
			suite: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "entry"), []byte("x"), 0o600); err != nil {
					t.Fatalf("write entry: %v", err)
				}
				return filepath.Join(dir, "[")
			},
			wantErr: "glob suite sources",
		},
		{
			name: "test file entry is a directory",
			suite: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				if err := os.Mkdir(filepath.Join(dir, "dir_test.go"), 0o750); err != nil {
					t.Fatalf("mkdir entry: %v", err)
				}
				return dir
			},
			wantErr: "read ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := scanSuite(tt.suite(t), map[string]struct{}{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("scanSuite() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
