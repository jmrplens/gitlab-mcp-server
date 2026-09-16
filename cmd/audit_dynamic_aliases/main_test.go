// main_test.go verifies the audit_dynamic_aliases command exit code and
// output contract against the generated dynamic alias catalog, the command
// line it accepts, and the TSV/JSON rendering of findings against synthetic
// finding sets.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
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

// TestRunMain_CommandLine_SelectsTheFormatAndReportsBadFlags verifies the
// command line the binary accepts: no argument audits in TSV, -output json
// switches the rendering, an unknown value for it is the usage exit code, an
// undefined flag is reported and exits 2 rather than killing the process, and
// -h prints the usage for -output and exits clean.
func TestRunMain_CommandLine_SelectsTheFormatAndReportsBadFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "no argument audits in tsv",
			args:       nil,
			wantCode:   0,
			wantStdout: "dynamic alias audit passed:",
		},
		{
			name:       "output json switches the rendering",
			args:       []string{"-output", "json"},
			wantCode:   0,
			wantStdout: `"Severity"`,
		},
		{
			name:       "an unknown output value is a usage error",
			args:       []string{"-output", "xml"},
			wantCode:   2,
			wantStderr: `invalid -output "xml"`,
		},
		{
			name:       "an undefined flag is reported and exits 2",
			args:       []string{"-nope"},
			wantCode:   2,
			wantStderr: "flag provided but not defined: -nope",
		},
		{
			name:       "help prints the usage and exits clean",
			args:       []string{"-h"},
			wantCode:   0,
			wantStderr: "-output string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			if got := runMain(tt.args, &stdout, &stderr); got != tt.wantCode {
				t.Errorf("runMain(%q) = %d, want %d (stderr %q)", tt.args, got, tt.wantCode, stderr.String())
			}
			if tt.wantStdout == "" && stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want containing %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr == "" && stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want containing %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

// TestMain_HandsTheExitCodeToOsExit verifies that main forwards whatever code
// runMain returned rather than a constant: the clean audit exits 0 and a bad
// -output value exits 2, through the same one line.
func TestMain_HandsTheExitCodeToOsExit(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{name: "the clean audit exits 0", args: []string{toolName}, wantCode: 0},
		{name: "an unknown output value exits 2", args: []string{toolName, "-output", "xml"}, wantCode: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldArgs, oldStdout, oldStderr := os.Args, os.Stdout, os.Stderr
			devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
			if err != nil {
				t.Fatalf("open %s: %v", os.DevNull, err)
			}
			os.Args, os.Stdout, os.Stderr = tt.args, devNull, devNull
			got := -1
			osExit = func(code int) { got = code }
			t.Cleanup(func() {
				os.Args, os.Stdout, os.Stderr = oldArgs, oldStdout, oldStderr
				osExit = os.Exit
				devNull.Close()
			})

			main()

			if got != tt.wantCode {
				t.Errorf("main() handed os.Exit %d, want %d", got, tt.wantCode)
			}
		})
	}
}

// TestRun_CatalogConstructionFails_ReportsTheStepOnStderr verifies that a
// catalog this audit cannot build ends the run with the failing step named on
// stderr and exit code 1, and that nothing is written to stdout: an audit that
// carried on would answer for no action at all.
func TestRun_CatalogConstructionFails_ReportsTheStepOnStderr(t *testing.T) {
	errFixture := errors.New("catalog unavailable")
	okBuild := func(*gitlabclient.Client, tools.ActionCatalogOptions) (*actioncatalog.Catalog, error) {
		return actioncatalog.NewCatalog(), nil
	}

	tests := []struct {
		name       string
		build      func(*gitlabclient.Client, tools.ActionCatalogOptions) (*actioncatalog.Catalog, error)
		standalone func(*actioncatalog.Catalog, *gitlabclient.Client, dynamic.StandaloneOptions) (*actioncatalog.Catalog, error)
		wantStderr string
	}{
		{
			name: "the catalog cannot be built",
			build: func(*gitlabclient.Client, tools.ActionCatalogOptions) (*actioncatalog.Catalog, error) {
				return nil, errFixture
			},
			standalone: func(*actioncatalog.Catalog, *gitlabclient.Client, dynamic.StandaloneOptions) (*actioncatalog.Catalog, error) {
				t.Error("addStandaloneCatalog ran after the build failed")
				return nil, errFixture
			},
			wantStderr: "build action catalog: catalog unavailable\n",
		},
		{
			name:  "the standalone routes cannot be added",
			build: okBuild,
			standalone: func(*actioncatalog.Catalog, *gitlabclient.Client, dynamic.StandaloneOptions) (*actioncatalog.Catalog, error) {
				return nil, errFixture
			},
			wantStderr: "add standalone dynamic catalog: catalog unavailable\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buildActionCatalog, addStandaloneCatalog = tt.build, tt.standalone
			t.Cleanup(func() {
				buildActionCatalog, addStandaloneCatalog = tools.BuildActionCatalog, dynamic.AddStandaloneCatalog
			})
			var stdout, stderr bytes.Buffer

			if got := run(&stdout, &stderr, "tsv"); got != 1 {
				t.Errorf("run() = %d, want 1", got)
			}
			if stderr.String() != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantStderr)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty: a failed build must report no findings", stdout.String())
			}
		})
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
