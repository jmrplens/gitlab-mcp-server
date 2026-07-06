// main_test.go verifies the audit_e2e_gaps command output contract against
// the real catalog and the checked-in e2e suite sources.
package main

import (
	"bytes"
	"encoding/json"
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
