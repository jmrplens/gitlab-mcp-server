package evaluator

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
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
	if !validPreset(presetDockerEnterpriseRead) || !validPreset(presetDockerCapabilityDiscovery) || validPreset("unknown") {
		t.Fatalf("validPreset() did not recognize only supported presets")
	}
	if got, err := normalizeEvalToolSurface(" DYNAMIC "); err != nil || got != config.ToolSurfaceDynamic {
		t.Fatalf("normalizeEvalToolSurface(dynamic) = %q, %v", got, err)
	}
	if got, err := normalizeEvalEdition(" Enterprise "); err != nil || got != editionEnterprise {
		t.Fatalf("normalizeEvalEdition(enterprise) = %q, %v", got, err)
	}
	if _, err := normalizeEvalEdition("ultimate"); err == nil {
		t.Fatal("normalizeEvalEdition(ultimate) error = nil, want unsupported edition")
	}
	if _, err := normalizeEvalToolSurface("individual"); err == nil {
		t.Fatal("normalizeEvalToolSurface(individual) error = nil, want unsupported surface")
	}
}

// TestApplyDockerEnterprisePresetDefaults_ConfiguresLiveEnterprisePartitions verifies
// Enterprise Docker presets select live GitLab execution defaults.
func TestApplyDockerEnterprisePresetDefaults_ConfiguresLiveEnterprisePartitions(t *testing.T) {
	cases := []struct {
		preset       string
		partition    string
		onlyMutating bool
		onlyDestruct bool
		skipMutating bool
		skipDestruct bool
	}{
		{preset: presetDockerEnterpriseRead, partition: partitionEnterpriseRead, skipMutating: true, skipDestruct: true},
		{preset: presetDockerEnterpriseMutatingSafe, partition: partitionEnterpriseMutating, onlyMutating: true, skipDestruct: true},
		{preset: presetDockerEnterpriseDestructiveSafe, partition: partitionEnterpriseDestructive, onlyDestruct: true},
	}
	for _, tc := range cases {
		t.Run(tc.preset, func(t *testing.T) {
			opts, err := applyPresetDefaults(options{Preset: tc.preset})
			if err != nil {
				t.Fatalf("applyPresetDefaults() error = %v", err)
			}
			if opts.Backend != backendGitLab || opts.GitLabEnv != "test/e2e/.env.docker" || opts.Partition != tc.partition || opts.Edition != editionEnterprise || !opts.Execute || !opts.UseFixtures || !opts.SkipUnavailable {
				t.Fatalf("opts = %+v, want live GitLab Docker defaults for %s", opts, tc.preset)
			}
			if opts.OnlyMutating != tc.onlyMutating || opts.OnlyDestructive != tc.onlyDestruct || opts.SkipMutating != tc.skipMutating || opts.SkipDestructive != tc.skipDestruct {
				t.Fatalf("opts = %+v, want mutating/destructive flags for %s", opts, tc.preset)
			}
		})
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
	os.Args = []string{"eval", "--model", "openai:gpt-4.1", "--task", "MT-001", "--repeat", "2", "--execute-tools=false", "--fixture-smoke"}
	flag.CommandLine = flag.NewFlagSet("eval", flag.ContinueOnError)

	opts := parseFlags()
	if opts.Model != "openai:gpt-4.1" || opts.OnlyIDs != "MT-001" || opts.Repeat != 2 || opts.Execute || !opts.FixtureSmoke {
		t.Fatalf("parseFlags() = %+v, want parsed model/task/repeat/execute", opts)
	}
	for _, name := range []string{"model", "task", "repeat", "execute-tools", "fixture-smoke"} {
		t.Run(name, func(t *testing.T) {
			if !opts.explicitFlags[name] {
				t.Fatalf("explicit flags = %#v, want %s", opts.explicitFlags, name)
			}
		})
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
		{name: "fixture smoke", opts: options{DryRun: true, FixtureSmoke: true, Execute: true}, want: "fixture-smoke"},
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
	// The paths are built with filepath, so they carry the platform's
	// separator; the prefix they must sit under is spelled with slashes.
	if got := filepath.ToSlash(defaultOutputPath("openai:gpt/test model")); !strings.HasPrefix(got, defaultEvalDir+"/") || !strings.Contains(got, "openai-gpt-test-model") {
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
	if opts.Edition != editionCE || !opts.Execute || !opts.UseFixtures || !opts.DockerAutoStart || !opts.SkipUnavailable || !opts.SkipMutating || !opts.SkipDestructive {
		t.Fatalf("opts = %+v, want Docker capability defaults enabled", opts)
	}
}

// TestApplyDockerPresetDefaults_RespectsExplicitDockerAutoStart verifies callers
// can disable evaluator-level Docker startup when another wrapper owns it.
func TestApplyDockerPresetDefaults_RespectsExplicitDockerAutoStart(t *testing.T) {
	opts, err := applyPresetDefaults(options{
		Preset:          presetDockerRead,
		DockerAutoStart: false,
		explicitFlags:   map[string]bool{"docker-auto-start": true},
	})
	if err != nil {
		t.Fatalf("applyPresetDefaults() error = %v", err)
	}
	if opts.DockerAutoStart {
		t.Fatalf("DockerAutoStart = true, want explicit false preserved: %+v", opts)
	}
}

// TestApplyPresetDefaults_UsesDockerReadDefaults verifies ApplyPresetDefaults uses docker read defaults.
func TestApplyPresetDefaults_UsesDockerReadDefaults(t *testing.T) {
	opts, err := applyPresetDefaults(options{Preset: presetDockerRead, explicitFlags: map[string]bool{}})
	if err != nil {
		t.Fatalf("applyPresetDefaults() error = %v", err)
	}
	if opts.Backend != backendGitLab {
		t.Fatalf("Backend = %q, want %q", opts.Backend, backendGitLab)
	}
	if opts.GitLabEnv != "test/e2e/.env.docker" {
		t.Fatalf("GitLabEnv = %q, want Docker env file", opts.GitLabEnv)
	}
	if opts.Partition != "base-read" {
		t.Fatalf("Partition = %q, want base-read", opts.Partition)
	}
	if !opts.Execute || !opts.UseFixtures || !opts.SkipUnavailable || !opts.SkipMutating || !opts.SkipDestructive {
		t.Fatalf("docker-read defaults not fully applied: %+v", opts)
	}
}

// TestApplyPresetDefaults_UsesDockerCapabilityDiscoveryDefaults verifies ApplyPresetDefaults uses safe Docker defaults for MCP capability discovery.
func TestApplyPresetDefaults_UsesDockerCapabilityDiscoveryDefaults(t *testing.T) {
	opts, err := applyPresetDefaults(options{Preset: presetDockerCapabilityDiscovery, explicitFlags: map[string]bool{}})
	if err != nil {
		t.Fatalf("applyPresetDefaults() error = %v", err)
	}
	if opts.Backend != backendGitLab {
		t.Fatalf("Backend = %q, want %q", opts.Backend, backendGitLab)
	}
	if opts.Partition != partitionCapabilityFallback {
		t.Fatalf("Partition = %q, want %q", opts.Partition, partitionCapabilityFallback)
	}
	if !opts.Execute || !opts.UseFixtures || !opts.SkipUnavailable || !opts.SkipMutating || !opts.SkipDestructive {
		t.Fatalf("docker-capability-discovery defaults not fully applied: %+v", opts)
	}
}

// TestApplyPresetDefaults_PreservesExplicitFlags verifies ApplyPresetDefaults preserves explicit flags.
func TestApplyPresetDefaults_PreservesExplicitFlags(t *testing.T) {
	opts, err := applyPresetDefaults(options{
		Preset:        presetDockerMutatingSafe,
		Backend:       backendMock,
		Partition:     "base-read",
		explicitFlags: map[string]bool{"backend": true, "partition": true},
	})
	if err != nil {
		t.Fatalf("applyPresetDefaults() error = %v", err)
	}
	if opts.Backend != backendMock {
		t.Fatalf("Backend = %q, want explicit backend", opts.Backend)
	}
	if opts.Partition != "base-read" {
		t.Fatalf("Partition = %q, want explicit partition", opts.Partition)
	}
	if !opts.Execute || !opts.UseFixtures || !opts.OnlyMutating || !opts.SkipDestructive {
		t.Fatalf("non-explicit preset defaults not applied: %+v", opts)
	}
}

// TestApplyPresetDefaults_RejectsUnknownPreset verifies ApplyPresetDefaults rejects unknown preset.
func TestApplyPresetDefaults_RejectsUnknownPreset(t *testing.T) {
	_, err := applyPresetDefaults(options{Preset: "surprise"})
	if err == nil {
		t.Fatal("applyPresetDefaults() error = nil, want unknown preset error")
	}
}

// TestNormalizeEvalToolSurface_AcceptsDynamicCandidates verifies that supported
// surface names accepted by configuration normalize to their canonical values.
func TestNormalizeEvalToolSurface_AcceptsDynamicCandidates(t *testing.T) {
	tests := map[string]string{
		"":        config.ToolSurfaceDynamic,
		"dynamic": config.ToolSurfaceDynamic,
		"meta":    config.ToolSurfaceMeta,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := normalizeEvalToolSurface(input)
			if err != nil {
				t.Fatalf("normalizeEvalToolSurface(%q) error = %v", input, err)
			}
			if got != want {
				t.Fatalf("normalizeEvalToolSurface(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

// TestDefaultTraceDir_ReplacesReportExtension verifies DefaultTraceDir when replaces report extension.
func TestDefaultTraceDir_ReplacesReportExtension(t *testing.T) {
	got := defaultTraceDir("dist/evaluation/mcp-surfaces/report.md")
	if got != "dist/evaluation/mcp-surfaces/report.traces" {
		t.Fatalf("defaultTraceDir() = %q, want report.traces", got)
	}
}

// TestDefaultTerminalLogPath_ReplacesReportExtension verifies terminal logs sit
// beside explicit Markdown reports.
func TestDefaultTerminalLogPath_ReplacesReportExtension(t *testing.T) {
	got := defaultTerminalLogPath("dist/evaluation/mcp-surfaces/report.md")
	if got != "dist/evaluation/mcp-surfaces/report.log" {
		t.Fatalf("defaultTerminalLogPath() = %q, want report.log", got)
	}
}

// TestDefaultTerminalLogPath_UsesIgnoredTerminalDirectory verifies the fallback
// terminal log path stays under ignored evaluation artifacts.
func TestDefaultTerminalLogPath_UsesIgnoredTerminalDirectory(t *testing.T) {
	got := defaultTerminalLogPath("")
	expectedPrefix := filepath.Join("dist", "evaluation", "mcp-surfaces", "terminal") + string(filepath.Separator)
	if !strings.HasPrefix(got, expectedPrefix) || filepath.Ext(got) != ".log" {
		t.Fatalf("defaultTerminalLogPath() = %q, want ignored terminal log path", got)
	}
}

// TestDefaultOutputPath_UsesIgnoredDistDirectory verifies DefaultOutputPath uses ignored dist directory.
func TestDefaultOutputPath_UsesIgnoredDistDirectory(t *testing.T) {
	got := filepath.ToSlash(defaultOutputPath("claude/sonnet:4 6"))
	if !strings.HasPrefix(got, "dist/evaluation/mcp-surfaces/model-") {
		t.Fatalf("defaultOutputPath() = %q, want dist evaluation path", got)
	}
	if !strings.HasSuffix(got, "-claude-sonnet-4-6.md") {
		t.Fatalf("defaultOutputPath() = %q, want sanitized model suffix", got)
	}
}

// TestDefaultOutputPath_UsesShortNameForMultiModel verifies DefaultOutputPath uses short name for multi model.
func TestDefaultOutputPath_UsesShortNameForMultiModel(t *testing.T) {
	got := defaultOutputPath("anthropic:claude-sonnet-4-6,openai:gpt-5.4-mini")
	if !strings.HasSuffix(got, "-multi-model.md") {
		t.Fatalf("defaultOutputPath() = %q, want multi-model suffix", got)
	}
}

// TestResolveEvalServerMode_FlagBeatsEnvironment verifies the resolution order
// for the server mode: an explicit --server-mode wins, otherwise
// EVAL_SURFACE_SERVER_MODE and then SERVER_MODE apply, so the Makefile alias
// and .env both work while the flag stays authoritative.
func TestResolveEvalServerMode_FlagBeatsEnvironment(t *testing.T) {
	t.Setenv("EVAL_SURFACE_SERVER_MODE", "")
	t.Setenv("SERVER_MODE", "")
	mode, err := resolveEvalServerMode("")
	if err != nil || mode != ServerModeDefault {
		t.Fatalf("resolveEvalServerMode(\"\") = %q, %v; want default", mode, err)
	}

	t.Setenv("SERVER_MODE", ServerModeReadOnly)
	mode, err = resolveEvalServerMode("")
	if err != nil || mode != ServerModeReadOnly {
		t.Errorf("SERVER_MODE alone = %q, %v; want read-only", mode, err)
	}

	t.Setenv("EVAL_SURFACE_SERVER_MODE", ServerModeSafe)
	mode, err = resolveEvalServerMode("")
	if err != nil || mode != ServerModeSafe {
		t.Errorf("EVAL_SURFACE_SERVER_MODE should win over SERVER_MODE, got %q, %v", mode, err)
	}

	mode, err = resolveEvalServerMode(ServerModeReadOnly)
	if err != nil || mode != ServerModeReadOnly {
		t.Errorf("explicit flag = %q, %v; want read-only over the environment", mode, err)
	}

	if _, invalidErr := resolveEvalServerMode("bogus"); invalidErr == nil {
		t.Error("resolveEvalServerMode(bogus) error = nil, want validation error")
	}
}

// TestApplyPresetDefaults_EveryPreset_SetsEditionPartitionAndFilters verifies
// each named preset resolves to its edition, partition and destructive or
// mutating filters, and that the schema preset stays a dry run.
func TestApplyPresetDefaults_EveryPreset_SetsEditionPartitionAndFilters(t *testing.T) {
	cases := []struct {
		preset          string
		wantEdition     string
		wantPartition   string
		wantDryRun      bool
		wantExecute     bool
		wantSkipMut     bool
		wantOnlyMut     bool
		wantSkipDestr   bool
		wantOnlyDestr   bool
		wantSkipUnavail bool
	}{
		{preset: presetSchemaEnterprise, wantEdition: editionEnterprise, wantDryRun: true, wantSkipUnavail: true},
		{preset: presetDockerRead, wantEdition: editionCE, wantPartition: partitionBaseRead, wantExecute: true, wantSkipMut: true, wantSkipDestr: true, wantSkipUnavail: true},
		{preset: presetDockerMutatingSafe, wantEdition: editionCE, wantPartition: partitionBaseMutating, wantExecute: true, wantOnlyMut: true, wantSkipDestr: true, wantSkipUnavail: true},
		{preset: presetDockerDestructiveSafe, wantEdition: editionCE, wantPartition: partitionBaseDestructive, wantExecute: true, wantOnlyDestr: true, wantSkipUnavail: true},
		{preset: presetDockerEnterpriseRead, wantEdition: editionEnterprise, wantPartition: partitionEnterpriseRead, wantExecute: true, wantSkipMut: true, wantSkipDestr: true, wantSkipUnavail: true},
		{preset: presetDockerEnterpriseMutatingSafe, wantEdition: editionEnterprise, wantPartition: partitionEnterpriseMutating, wantExecute: true, wantOnlyMut: true, wantSkipDestr: true, wantSkipUnavail: true},
		{preset: presetDockerEnterpriseDestructiveSafe, wantEdition: editionEnterprise, wantPartition: partitionEnterpriseDestructive, wantExecute: true, wantOnlyDestr: true, wantSkipUnavail: true},
		{preset: presetDockerCapabilityDiscovery, wantEdition: editionCE, wantPartition: partitionCapabilityFallback, wantExecute: true, wantSkipMut: true, wantSkipDestr: true, wantSkipUnavail: true},
		{preset: presetDockerErrorRecovery, wantEdition: editionCE, wantPartition: partitionErrorRecovery, wantExecute: true, wantSkipMut: true, wantSkipDestr: true, wantSkipUnavail: true},
	}
	for _, tc := range cases {
		t.Run(tc.preset, func(t *testing.T) {
			opts, err := applyPresetDefaults(options{Preset: " " + tc.preset + " ", explicitFlags: map[string]bool{}})
			if err != nil {
				t.Fatalf("applyPresetDefaults(%s) error = %v", tc.preset, err)
			}
			got := []bool{opts.DryRun, opts.Execute, opts.SkipMutating, opts.OnlyMutating, opts.SkipDestructive, opts.OnlyDestructive, opts.SkipUnavailable}
			want := []bool{tc.wantDryRun, tc.wantExecute, tc.wantSkipMut, tc.wantOnlyMut, tc.wantSkipDestr, tc.wantOnlyDestr, tc.wantSkipUnavail}
			if opts.Preset != tc.preset || opts.Edition != tc.wantEdition || opts.Partition != tc.wantPartition || !reflect.DeepEqual(got, want) {
				t.Fatalf("applyPresetDefaults(%s) = %+v, want edition %s partition %s flags %v", tc.preset, opts, tc.wantEdition, tc.wantPartition, want)
			}
		})
	}
}

// TestNormalizeEvalEdition_RejectsUnknownEdition verifies an unsupported
// edition selector is refused with the accepted values in the message.
func TestNormalizeEvalEdition_RejectsUnknownEdition(t *testing.T) {
	if _, err := normalizeEvalEdition("premium"); err == nil || !strings.Contains(err.Error(), "--edition must be") {
		t.Fatalf("normalizeEvalEdition(premium) error = %v, want rejection", err)
	}
}

// TestNormalizeEvalToolSurface_RejectsIndividualAndInvalidSurfaces verifies the
// evaluator only accepts the meta and dynamic surfaces: the individual surface
// parses but is refused, and an unparseable value fails in the parser.
func TestNormalizeEvalToolSurface_RejectsIndividualAndInvalidSurfaces(t *testing.T) {
	cases := []struct {
		surface string
		want    string
	}{
		{surface: config.ToolSurfaceIndividual, want: "--tool-surface must be"},
		{surface: "bogus", want: "TOOL_SURFACE"},
	}
	for _, tc := range cases {
		t.Run(tc.surface, func(t *testing.T) {
			_, err := normalizeEvalToolSurface(tc.surface)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("normalizeEvalToolSurface(%s) error = %v, want %q", tc.surface, err, tc.want)
			}
		})
	}
}

// TestDefaultTraceDir_NoExtension_AppendsSuffix verifies a report path without
// an extension still gets the .traces suffix.
func TestDefaultTraceDir_NoExtension_AppendsSuffix(t *testing.T) {
	if got := defaultTraceDir("dist/report"); got != "dist/report.traces" {
		t.Fatalf("defaultTraceDir() = %q, want dist/report.traces", got)
	}
}
