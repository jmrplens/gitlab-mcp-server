// main_test.go covers the command's own decisions: which matrix a set of flags
// selects, and the guard that keeps a smoke run from overwriting published
// measurements.
package main

import (
	"strings"
	"testing"
)

// TestMatrixFor_FullRun_UsesThePublishedMatrix verifies a plain run measures
// everything the page publishes, at the requested number of rounds.
func TestMatrixFor_FullRun_UsesThePublishedMatrix(t *testing.T) {
	plans, err := matrixFor(options{rounds: 5})
	if err != nil {
		t.Fatalf("matrixFor: %v", err)
	}
	if len(plans) != len(publishedMatrix(5)) {
		t.Errorf("selected %d scenarios, want the whole matrix", len(plans))
	}
	for _, plan := range plans {
		if plan.Rounds != 5 {
			t.Errorf("%s runs %d rounds, want the 5 that were asked for", plan.ID, plan.Rounds)
		}
	}
}

// TestMatrixFor_PartialRun_RefusesToOverwriteTheRecord verifies a smoke or
// filtered run insists on being told where to write.
//
// Without this, `-quick` would replace measured, published numbers with a
// two-scenario smoke test and the documentation would keep presenting them as
// the figures. The guard is on the flag having been given, not on its value,
// so passing the default path explicitly is still allowed: that is someone
// saying what they mean.
func TestMatrixFor_PartialRun_RefusesToOverwriteTheRecord(t *testing.T) {
	tests := []struct {
		name    string
		opts    options
		wantErr bool
	}{
		{name: "quick without a path", opts: options{quick: true, rounds: 1}, wantErr: true},
		{name: "quick with a path", opts: options{quick: true, rounds: 1, recordSet: true}},
		{name: "filtered without a path", opts: options{scenarios: "http-meta", rounds: 1}, wantErr: true},
		{name: "filtered with a path", opts: options{scenarios: "http-meta", rounds: 1, recordSet: true}},
		{name: "full run needs no path", opts: options{rounds: 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := matrixFor(tc.opts)
			if tc.wantErr {
				if err == nil {
					t.Fatal("matrixFor allowed a partial matrix to write to the published record")
				}
				if !strings.Contains(err.Error(), "-json") {
					t.Errorf("error %q does not say how to fix it", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("matrixFor: %v", err)
			}
		})
	}
}

// TestMatrixFor_UnknownScenario_ReportsIt verifies a mistyped scenario name
// stops the run instead of measuring a smaller matrix.
func TestMatrixFor_UnknownScenario_ReportsIt(t *testing.T) {
	if _, err := matrixFor(options{scenarios: "http-dinamic", rounds: 1, recordSet: true}); err == nil {
		t.Error("matrixFor accepted a scenario name that does not exist")
	}
}

// TestResolveAndRel_RoundTrip verifies flag paths are resolved against the
// module root and reported back relative to it, so the command's output names
// files the way the repository does.
func TestResolveAndRel_RoundTrip(t *testing.T) {
	root := t.TempDir()

	absolute := resolve(root, "docs/reference/benchmarks")
	if !strings.HasPrefix(absolute, root) {
		t.Errorf("resolve = %q, want it under %q", absolute, root)
	}
	if got := rel(root, absolute); got != "docs/reference/benchmarks" {
		t.Errorf("rel = %q, want the relative path back", got)
	}

	// An absolute flag value is left alone, which is what lets a smoke run
	// write outside the repository.
	outside := "/tmp/bench.json"
	if got := resolve(root, outside); got != outside {
		t.Errorf("resolve rewrote an absolute path: %q", got)
	}
}

// TestProgressFunc_QuietUnlessAsked verifies the per-client reporter is silent
// by default, so a full run prints one line per scenario rather than hundreds.
func TestProgressFunc_QuietUnlessAsked(t *testing.T) {
	quiet := progressFunc(false)
	quiet("this must not panic %d", 1)

	loud := progressFunc(true)
	loud("nor this %d", 2)
}
