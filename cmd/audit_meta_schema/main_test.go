// main_test.go verifies the meta-schema audit command can build and inspect the
// full base-plus-enterprise meta-tool catalog without requiring a real GitLab
// instance.
package main

import (
	"testing"
)

// TestRun_Completes verifies the meta-schema audit can build the full
// base-plus-enterprise meta-tool registry and measure schema sizes.
func TestRun_Completes(t *testing.T) {
	if err := run(); err != nil {
		t.Fatalf("run() error: %v", err)
	}
}

// TestHuman_AllMagnitudes verifies the [human] byte formatter emits
// expected B/KB/MB suffixes for the three supported magnitude ranges.
//
// Each branch of the size switch is exercised explicitly so a future
// refactor that drops one of the cases is caught immediately.
func TestHuman_AllMagnitudes(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want string
	}{
		{name: "sub-kilobyte renders as B", n: 512, want: "512 B"},
		{name: "kilobyte threshold renders as KB", n: 1024, want: "1.0 KB"},
		{name: "megabyte threshold renders as MB", n: 1024 * 1024, want: "1.0 MB"},
		{name: "large value renders as MB", n: 3 * 1024 * 1024, want: "3.0 MB"},
		{name: "zero renders as B", n: 0, want: "0 B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := human(tt.n); got != tt.want {
				t.Fatalf("human(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// TestRepeat_BuildsExpectedString verifies [repeat] concatenates its argument
// the requested number of times.
//
// The helper is used to draw the audit table dividers; verifying length and
// content protects callers that depend on a fixed-width output line.
func TestRepeat_BuildsExpectedString(t *testing.T) {
	if got := repeat("-", 5); got != "-----" {
		t.Fatalf("repeat(\"-\", 5) = %q, want %q", got, "-----")
	}
	if got := repeat("ab", 0); got != "" {
		t.Fatalf("repeat(\"ab\", 0) = %q, want empty string", got)
	}
	if got := repeat("x", 1); got != "x" {
		t.Fatalf("repeat(\"x\", 1) = %q, want %q", got, "x")
	}
}

// TestMain_BuildsWithoutErrors verifies the audit command's entrypoint
// compiles and would exit successfully.
//
// The [main] function itself is exercised only via end-to-end `go run`
// invocations from the developer workflow; unit tests cannot reach os.Exit
// without an exec. This test ensures the binary builds cleanly so a future
// refactor that breaks the entrypoint is caught by the test compilation
// step before deployment.
func TestMain_BuildsWithoutErrors(t *testing.T) {
	// The mere fact that this test file is part of the package compiled
	// together with main.go proves the entrypoint type-checks. Nothing to
	// assert at runtime.
}
