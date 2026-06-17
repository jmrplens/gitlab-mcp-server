// main_test.go verifies the audit_dynamic_aliases command exit code and
// output contract against the generated dynamic alias catalog.
//
// The single test runs [run] with in-memory writers and asserts the
// command exits cleanly and prints the canonical "dynamic alias audit
// passed" summary string.
package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun_DefaultCatalogPasses verifies the dynamic alias audit succeeds against the generated catalog.
func TestRun_DefaultCatalogPasses(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if exitCode := run(&stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() stderr = %q, want empty", stderr.String())
	}
	if output := stdout.String(); !strings.Contains(output, "dynamic alias audit passed:") {
		t.Fatalf("run() stdout = %q, want pass summary", output)
	}
}
