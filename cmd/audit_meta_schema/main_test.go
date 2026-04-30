// main_test.go verifies the meta-schema audit command can build and inspect the
// full base-plus-enterprise meta-tool catalog without requiring a real GitLab
// instance.
package main

import "testing"

// TestRun_Completes verifies the meta-schema audit can build the full
// base-plus-enterprise meta-tool registry and measure schema sizes.
func TestRun_Completes(t *testing.T) {
	if err := run(); err != nil {
		t.Fatalf("run() error: %v", err)
	}
}
