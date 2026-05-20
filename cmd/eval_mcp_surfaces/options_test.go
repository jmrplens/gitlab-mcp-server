package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestOptionNormalizationHelpers_DefaultsAndValidation verifies option helpers
// normalize evaluator defaults without accepting unsupported surfaces or presets.
func TestOptionNormalizationHelpers_DefaultsAndValidation(t *testing.T) {
	if got := normalizedBackend(" "); got != backendMock {
		t.Fatalf("normalizedBackend(blank) = %q, want mock", got)
	}
	if !validPreset(presetDockerCapabilityDiscovery) || validPreset("unknown") {
		t.Fatalf("validPreset() did not recognize only supported presets")
	}
	if got, err := normalizeEvalToolSurface(" DYNAMIC "); err != nil || got != config.ToolSurfaceDynamic {
		t.Fatalf("normalizeEvalToolSurface(dynamic) = %q, %v", got, err)
	}
	if _, err := normalizeEvalToolSurface("individual"); err == nil {
		t.Fatal("normalizeEvalToolSurface(individual) error = nil, want unsupported surface")
	}
}

// TestParseFlags_RecordsExplicitFlags verifies global CLI parsing records the
// exact flags the user supplied so presets can preserve them.
func TestParseFlags_RecordsExplicitFlags(t *testing.T) {
	originalArgs := os.Args
	originalFlagSet := flag.CommandLine
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlagSet
	})
	os.Args = []string{"eval", "--model", "openai:gpt-4.1", "--task", "MT-001", "--repeat", "2", "--execute-tools=false"}
	flag.CommandLine = flag.NewFlagSet("eval", flag.ContinueOnError)

	opts := parseFlags()
	if opts.Model != "openai:gpt-4.1" || opts.OnlyIDs != "MT-001" || opts.Repeat != 2 || opts.Execute {
		t.Fatalf("parseFlags() = %+v, want parsed model/task/repeat/execute", opts)
	}
	for _, name := range []string{"model", "task", "repeat", "execute-tools"} {
		if !opts.explicitFlags[name] {
			t.Fatalf("explicit flags = %#v, want %s", opts.explicitFlags, name)
		}
	}
}

// TestToolExecutionMode_ReflectsDryRunExecuteAndExternalModes verifies reports
// describe how tool calls will be handled.
func TestToolExecutionMode_ReflectsDryRunExecuteAndExternalModes(t *testing.T) {
	cases := []struct {
		name string
		opts options
		want string
	}{
		{name: "dry run", opts: options{DryRun: true, Execute: true}, want: "none"},
		{name: "simulated", opts: options{}, want: "simulated"},
		{name: "in memory mcp", opts: options{Execute: true}, want: "mcp"},
		{name: "external mcp", opts: options{Execute: true, MCPCommand: "gitlab-mcp-server"}, want: "mcp-external"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toolExecutionMode(tc.opts); got != tc.want {
				t.Fatalf("toolExecutionMode() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDefaultArtifactPaths_UseEvaluationDirectories verifies generated report,
// trace, and terminal paths stay under the ignored evaluation output tree.
func TestDefaultArtifactPaths_UseEvaluationDirectories(t *testing.T) {
	if got := defaultOutputPath("openai:gpt/test model"); !strings.HasPrefix(got, defaultEvalDir+string(filepath.Separator)) || !strings.Contains(got, "openai-gpt-test-model") {
		t.Fatalf("defaultOutputPath() = %q", got)
	}
	if got := defaultComparisonOutputPath(); !strings.Contains(got, filepath.Join(defaultEvalDir, "comparison")) || !strings.HasSuffix(got, "-summary.md") {
		t.Fatalf("defaultComparisonOutputPath() = %q", got)
	}
	if got := defaultTraceDir("dist/report.md"); got != "dist/report.traces" {
		t.Fatalf("defaultTraceDir() = %q, want dist/report.traces", got)
	}
	if got := defaultTerminalLogPath("dist/report.md"); got != "dist/report.log" {
		t.Fatalf("defaultTerminalLogPath() = %q, want dist/report.log", got)
	}
}

// TestApplyDockerPresetDefaults_RespectsExplicitValues verifies presets do not
// overwrite values the caller supplied explicitly.
func TestApplyDockerPresetDefaults_RespectsExplicitValues(t *testing.T) {
	opts, err := applyPresetDefaults(options{
		Preset:        presetDockerCapabilityDiscovery,
		Backend:       "custom",
		Partition:     "custom-partition",
		explicitFlags: map[string]bool{"backend": true, "partition": true},
	})
	if err != nil {
		t.Fatalf("applyPresetDefaults() error = %v", err)
	}
	if opts.Backend != "custom" || opts.Partition != "custom-partition" {
		t.Fatalf("opts = %+v, want explicit backend and partition preserved", opts)
	}
	if !opts.Execute || !opts.UseFixtures || !opts.SkipUnavailable || !opts.SkipMutating || !opts.SkipDestructive {
		t.Fatalf("opts = %+v, want Docker capability defaults enabled", opts)
	}
}
