// main_test.go verifies the audit_dynamic_aliases command exit code and
// output contract against the generated dynamic alias catalog, and the
// TSV/JSON rendering of findings against synthetic finding sets.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
)

// TestRun_DefaultCatalogPasses verifies the dynamic alias audit succeeds against the generated catalog.
func TestRun_DefaultCatalogPasses(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if exitCode := run(&stdout, &stderr, "tsv"); exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() stderr = %q, want empty", stderr.String())
	}
	if output := stdout.String(); !strings.Contains(output, "dynamic alias audit passed:") {
		t.Fatalf("run() stdout = %q, want pass summary", output)
	}
}

// TestRun_JSONOutput_EncodesFindings verifies the JSON format writes one
// array of findings and nothing else to stdout, exits 0 against the
// generated catalog, and keeps stderr silent.
func TestRun_JSONOutput_EncodesFindings(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if exitCode := run(&stdout, &stderr, "json"); exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	var findings []dynamic.AliasAuditFinding
	if err := json.Unmarshal(stdout.Bytes(), &findings); err != nil {
		t.Fatalf("stdout is not a findings array: %v\n%s", err, stdout.String())
	}
	for _, finding := range findings {
		if finding.Severity == "error" {
			t.Errorf("generated catalog carries an error finding: %+v", finding)
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("run() stderr = %q, want empty", stderr.String())
	}
}

// errWriter fails every write so the JSON encoder's error branch is reachable.
type errWriter struct{}

// Write always fails.
func (errWriter) Write([]byte) (int, error) { return 0, errors.New("stdout closed") }

// TestWriteFindings_Scenarios_ReportsAndExits verifies the rendering of a
// synthetic finding set: TSV lists every finding and either the pass summary
// or the failure line, the exit code follows the error count in both
// formats, an unknown format is a usage error, and a stdout that rejects the
// JSON payload is reported on stderr.
func TestWriteFindings_Scenarios_ReportsAndExits(t *testing.T) {
	warning := dynamic.AliasAuditFinding{Severity: "warning", Problem: "shadowed", Alias: "issue_get", Canonical: "issue.get", Source: "default", Message: "alias shadows a canonical id"}
	failure := dynamic.AliasAuditFinding{Severity: "error", Problem: "dangling", Alias: "ghost", Canonical: "ghost.action", Source: "default", Message: "alias points nowhere"}

	tests := []struct {
		name       string
		findings   []dynamic.AliasAuditFinding
		format     string
		wantCode   int
		wantStdout []string
		wantStderr string
	}{
		{
			name:       "tsv without errors passes",
			findings:   []dynamic.AliasAuditFinding{warning},
			format:     "tsv",
			wantCode:   0,
			wantStdout: []string{"warning\tshadowed\tissue_get\tissue.get\tdefault\talias shadows a canonical id\n", "dynamic alias audit passed: 1 finding(s)\n"},
		},
		{
			name:       "tsv with an error fails",
			findings:   []dynamic.AliasAuditFinding{warning, failure},
			format:     "tsv",
			wantCode:   1,
			wantStdout: []string{"error\tdangling\tghost\tghost.action\tdefault\talias points nowhere\n"},
			wantStderr: "dynamic alias audit failed: 1 error(s)\n",
		},
		{
			name:       "json with an error fails after encoding",
			findings:   []dynamic.AliasAuditFinding{failure},
			format:     "json",
			wantCode:   1,
			wantStdout: []string{`"Severity":"error"`, `"Alias":"ghost"`},
		},
		{
			name:       "json without findings passes",
			findings:   nil,
			format:     "json",
			wantCode:   0,
			wantStdout: []string{"null\n"},
		},
		{
			name:       "unknown format is a usage error",
			findings:   []dynamic.AliasAuditFinding{warning},
			format:     "xml",
			wantCode:   2,
			wantStderr: "invalid -output \"xml\" (want tsv or json)\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := writeFindings(&stdout, &stderr, tt.findings, tt.format); got != tt.wantCode {
				t.Errorf("writeFindings() = %d, want %d (stderr %q)", got, tt.wantCode, stderr.String())
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("stdout = %q, want containing %q", stdout.String(), want)
				}
			}
			if stderr.String() != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

// TestWriteFindings_JSONEncodeFails_ReportsOnStderr verifies a stdout that
// rejects the JSON payload turns into an "encode json" diagnostic and exit
// code 1 rather than a silent empty report.
func TestWriteFindings_JSONEncodeFails_ReportsOnStderr(t *testing.T) {
	var stderr bytes.Buffer
	if got := writeFindings(errWriter{}, &stderr, nil, "json"); got != 1 {
		t.Fatalf("writeFindings() = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "encode json: stdout closed") {
		t.Fatalf("stderr = %q, want the encode diagnostic", stderr.String())
	}
}
