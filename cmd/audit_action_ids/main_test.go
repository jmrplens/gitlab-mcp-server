package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// auditedPattern is a small, real package to run the whole command over. The
// command's own source publishes no action IDs, so the run is a complete one
// that finds nothing, which is exactly the shape a clean run has to take.
const auditedPattern = "./cmd/audit_action_ids/..."

// TestRun_WholeCommand_ReportsAndWritesTheWorkList drives run end to end and
// holds the two things it owes a caller: a report on stdout and the work list
// on disk for the layer that fixes the cross-links.
func TestRun_WholeCommand_ReportsAndWritesTheWorkList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "action-ids.json")
	var stdout, stderr bytes.Buffer

	if code := run(repoRoot(t), []string{auditedPattern}, path, false, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), toolName+":") {
		t.Errorf("stdout = %q, want the summary line", stdout.String())
	}
	if !strings.Contains(stdout.String(), "wrote "+path) {
		t.Errorf("stdout = %q, want the work list named", stdout.String())
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("work list: %v", err)
	}
}

// TestRun_NoWorkListPath_WritesNothing holds that an empty -json is a report
// and nothing else, which is how the audit is run over one package without
// overwriting the tree's work list.
func TestRun_NoWorkListPath_WritesNothing(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer

	if code := run(repoRoot(t), []string{auditedPattern}, "", false, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "wrote ") {
		t.Errorf("stdout = %q, want no work list written", stdout.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Errorf("temporary directory holds %d entries (err %v), want none", len(entries), err)
	}
}

// TestRun_UnloadableSource_ExitsOne holds that a run that could not be made is
// a failure rather than a clean report. The audit does not fail on a finding,
// so this is the only thing that can send it home with a 1, and a silent
// success here would be a report over source nobody loaded.
func TestRun_UnloadableSource_ExitsOne(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run(repoRoot(t), []string{"./cmd/audit_action_ids/nothing/..."}, "", false, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), toolName+":") {
		t.Errorf("stderr = %q, want the failure named", stderr.String())
	}
}

// TestRun_UnwritableWorkList_ExitsOne holds that a work list that could not be
// written fails the run, since the layer downstream reads that file and an
// absent one would read as no work to do.
func TestRun_UnwritableWorkList_ExitsOne(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	var stdout, stderr bytes.Buffer

	code := run(repoRoot(t), []string{auditedPattern}, filepath.Join(blocker, "action-ids.json"), false, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), toolName+":") {
		t.Errorf("stderr = %q, want the failure named", stderr.String())
	}
}

// TestPatternsOrDefault_NoArguments_AuditTheWholeTree holds what a bare run
// covers: every package that publishes an action ID.
func TestPatternsOrDefault_NoArguments_AuditTheWholeTree(t *testing.T) {
	if got := patternsOrDefault(nil); !slices.Equal(got, defaultPatterns) {
		t.Errorf("patternsOrDefault(nil) = %v, want %v", got, defaultPatterns)
	}
	if got := patternsOrDefault([]string{auditedPattern}); !slices.Equal(got, []string{auditedPattern}) {
		t.Errorf("patternsOrDefault named a pattern of its own: %v", got)
	}
	if !slices.Contains(defaultPatterns, "./internal/tools/...") {
		t.Errorf("defaultPatterns = %v, want the tree that publishes action IDs", defaultPatterns)
	}
}
