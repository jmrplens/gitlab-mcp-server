package evaluator

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// parseFlags parses flags from evaluator input. Each flag below is documented
// with the user-visible behavior; flag.* descriptions are also surfaced
// verbatim by --help.
func parseFlags() options {
	var opts options
	// --tasks is a deprecated Markdown loader retained only to emit a clear
	// deprecation error when set; evaluation cases load from typed EvalCase
	// definitions instead.
	flag.StringVar(&opts.TasksPath, "tasks", "", "Deprecated; evaluation cases are loaded from typed EvalCase definitions")
	// --out controls the Markdown report path.
	flag.StringVar(&opts.Output, "out", "", "Markdown report path; defaults under dist/evaluation/mcp-surfaces")
	// --trace-dir overrides the per-task trace artifact directory.
	flag.StringVar(&opts.TraceDir, "trace-dir", "", "Directory for per-task model trace artifacts; defaults to <report>.traces in model-backed mode")
	// --terminal-log routes command progress and terminal output to a file.
	flag.StringVar(&opts.TerminalLog, "terminal-log", "", "File receiving command progress and terminal output; defaults under dist/evaluation/mcp-surfaces/terminal or beside --out")
	// --model runs a single provider:model and overrides --models/EVAL_MODELS.
	flag.StringVar(&opts.Model, "model", "", "Single provider:model or legacy Anthropic model; overrides --models and EVAL_MODELS")
	// --models runs multiple provider:model pairs from one CLI invocation.
	flag.StringVar(&opts.Models, "models", "", "Comma-separated provider:model list for local multi-model evaluation; defaults to EVAL_MODELS when --model is not set")
	// --tools-file evaluates a tools/list snapshot instead of the live catalog.
	flag.StringVar(&opts.ToolsFile, "tools-file", "", "Optional tools/list JSON snapshot to evaluate instead of the live catalog")
	// --compare adds a report to the comparison summary; repeatable.
	flag.Var(&opts.CompareReports, "compare", "Evaluation or token report file to include in a comparison summary; repeat for multiple reports")
	// --check-efficiency validates a trace JSONL against the per-task budget.
	flag.Var(&opts.CheckEfficiency, "check-efficiency", "Trace JSONL path to validate against model-call efficiency gates; repeat for multiple trace files")
	// --check-report-clean verifies a report contains no failed rows.
	flag.Var(&opts.CheckReportClean, "check-report-clean", "Evaluation report file that must have no failed task rows; repeat for multiple reports")
	// --compare-traces produces a dynamic-vs-meta comparison summary.
	flag.Var(&opts.CompareTraces, "compare-traces", "Trace JSONL path for direct dynamic versus meta comparison; provide dynamic trace first and meta trace second")
	// --efficiency-allow-task exempts a task from the efficiency budget.
	flag.Var(&opts.EfficiencyAllowTask, "efficiency-allow-task", "Task ID allowed to exceed the per-attempt call budget in --check-efficiency; repeat or comma-separate values")
	// --publish-from selects a reviewed report to publish into docs.
	flag.Var(&opts.PublishFrom, "publish-from", "Reviewed evaluation report to publish into docs; repeat for multiple reports")
	// --publish-results-doc is the Markdown results file updated by --publish-docs.
	flag.StringVar(&opts.PublishResults, "publish-results-doc", defaultPublishResultsDoc, "Markdown results document updated by --publish-docs")
	// --publish-readme is the README updated by --publish-docs.
	flag.StringVar(&opts.PublishReadme, "publish-readme", defaultPublishReadme, "README updated by --publish-docs")
	// --publish-label tags the published snapshot.
	flag.StringVar(&opts.PublishLabel, "publish-label", "", "Human-readable label for the published snapshot")
	// --publish-mode controls append vs replace-current publication.
	flag.StringVar(&opts.PublishMode, "publish-mode", publishModeReplaceCurrent, "Publication mode for model results: append or replace-current")
	// --preset selects a curated preset of edition/partition/flags.
	flag.StringVar(&opts.Preset, "preset", "", "Optional evaluation preset: docker-read, docker-mutating-safe, docker-destructive-safe, docker-enterprise-read, docker-enterprise-mutating-safe, docker-enterprise-destructive-safe, docker-capability-discovery, docker-error-recovery, or schema-enterprise")
	// --partition selects a schema fixture partition.
	flag.StringVar(&opts.Partition, "partition", "", "Optional schema fixture partition: base-read, base-mutating, base-destructive, enterprise-read, enterprise-mutating, enterprise-destructive, error-recovery, or capability-fallback")
	// --tool-surface chooses the dynamic or meta catalog.
	flag.StringVar(&opts.ToolSurface, "tool-surface", config.DefaultToolSurface, "Tool catalog surface to evaluate: dynamic or meta")
	// --edition filters cases by GitLab edition.
	flag.StringVar(&opts.Edition, "edition", editionAll, "Task edition filter: all, ce, or enterprise")
	// --coverage-report emits an uncovered-route report after the run.
	flag.StringVar(&opts.CoverageReport, "coverage-report", "", "Optional Markdown report listing uncovered high-risk routes after the selected evaluation")
	// --backend selects the mock or live GitLab catalog backend.
	flag.StringVar(&opts.Backend, "backend", backendMock, "Live catalog backend: mock or gitlab. gitlab uses GITLAB_URL/GITLAB_TOKEN, optionally loaded from --gitlab-env-file")
	// --gitlab-env-file overlays environment variables for the gitlab backend.
	flag.StringVar(&opts.GitLabEnv, "gitlab-env-file", "", "Optional env file loaded after .env for --backend=gitlab, for example test/e2e/.env.docker")
	// --docker-compose overrides the Docker Compose command.
	flag.StringVar(&opts.DockerCompose, "docker-compose", "", "Docker Compose command used when Docker presets auto-start GitLab; defaults to DOCKER_COMPOSE or 'docker compose'")
	// --docker-compose-file overrides the Docker Compose file.
	flag.StringVar(&opts.DockerComposeFile, "docker-compose-file", "", "Docker Compose file used when Docker presets auto-start GitLab; defaults to EVAL_DOCKER_COMPOSE_FILE or test/e2e/docker-compose.yml")
	// --docker-gitlab-url overrides the Docker fixture stack URL.
	flag.StringVar(&opts.DockerGitLabURL, "docker-gitlab-url", "", "GitLab URL exposed by the Docker fixture stack; defaults to EVAL_DOCKER_GITLAB_URL or http://localhost:8929")
	// --mcp-command swaps the in-memory server for an external stdio MCP server.
	flag.StringVar(&opts.MCPCommand, "mcp-command", "", "External stdio MCP server command for --execute-tools instead of the current in-memory server")
	// --mcp-arg appends one argument to --mcp-command; repeatable.
	flag.Var(&opts.MCPArgs, "mcp-arg", "External MCP server command argument; repeat for multiple args")
	// --mcp-env-file supplies env overrides for --mcp-command.
	flag.StringVar(&opts.MCPEnv, "mcp-env-file", "", "Optional env file applied only to --mcp-command")
	// --fixtures points at the fixture state JSON file.
	flag.StringVar(&opts.Fixtures, "fixtures", defaultFixtures, "Fixture state JSON path used by --prepare-fixtures and --use-fixtures")
	// --task selects a comma-separated list of task IDs.
	flag.StringVar(&opts.OnlyIDs, "task", "", "Comma-separated task IDs to run, for example MT-035,MT-040")
	// --max-tasks caps the number of selected tasks (0 = unlimited).
	flag.IntVar(&opts.MaxTasks, "max-tasks", 0, "Limit number of tasks; 0 runs all tasks")
	// --repeat repeats the selected task set.
	flag.IntVar(&opts.Repeat, "repeat", 1, "Number of times to repeat the selected task set")
	// --max-tokens limits output tokens per model request.
	flag.IntVar(&opts.MaxTokens, "max-tokens", 1024, "Max output tokens per model request")
	// --retries retries transient 429/5xx responses.
	flag.IntVar(&opts.Retries, "retries", 3, "Retries for transient model-provider 429/5xx responses")
	// --retry-wait sets the fallback wait between retries.
	flag.DurationVar(&opts.RetryWait, "retry-wait", 65*time.Second, "Fallback wait before retrying model-provider 429 responses")
	// --max-output-retries re-runs a task when it fails only because the model
	// emitted malformed tool-call output (unparseable arguments), a stochastic
	// generation glitch. Wrong tool/action/param choices and schema/MCP/GitLab
	// errors are NOT retried. 2 = up to 3 total attempts.
	flag.IntVar(&opts.MaxOutputRetries, "max-output-retries", 2, "Retries for tasks that fail solely due to malformed model tool-call output (0 disables)")
	// --pause inserts a delay between tasks.
	flag.DurationVar(&opts.Pause, "pause", 0, "Optional pause between tasks")
	// --input-cost-per-mtok sets the USD/M token price for input tokens.
	flag.Float64Var(&opts.Pricing.InputPerMTok, "input-cost-per-mtok", 0, "Optional input token price in USD per million tokens for cost estimates")
	// --output-cost-per-mtok sets the USD/M token price for output tokens.
	flag.Float64Var(&opts.Pricing.OutputPerMTok, "output-cost-per-mtok", 0, "Optional output token price in USD per million tokens for cost estimates")
	// --cache-write-cost-per-mtok sets the USD/M token price for cache writes.
	flag.Float64Var(&opts.Pricing.CacheWritePerMTok, "cache-write-cost-per-mtok", 0, "Optional prompt-cache write price in USD per million tokens for cost estimates")
	// --cache-read-cost-per-mtok sets the USD/M token price for cache reads.
	flag.Float64Var(&opts.Pricing.CacheReadPerMTok, "cache-read-cost-per-mtok", 0, "Optional prompt-cache read price in USD per million tokens for cost estimates")
	// --dry-run validates fixture routes without calling providers.
	flag.BoolVar(&opts.DryRun, "dry-run", false, "Validate fixture routes without calling model providers")
	// --fixture-smoke exercises fixture preparation without model calls.
	flag.BoolVar(&opts.FixtureSmoke, "fixture-smoke", false, "With --dry-run, exercise live per-task fixture preparation through MCP without calling model providers")
	// --publish-docs publishes reviewed reports into README and docs.
	flag.BoolVar(&opts.PublishDocs, "publish-docs", false, "Publish reviewed evaluation reports into README and docs/development/testing/model-results.md")
	// --check-docs verifies published docs without writing files.
	flag.BoolVar(&opts.CheckDocs, "check-docs", false, "Verify published evaluation docs match the selected --publish-from reports without writing files")
	// --publish-allow-harness-noise allows publishing reports with unresolved harness noise.
	flag.BoolVar(&opts.PublishAllowNoise, "publish-allow-harness-noise", false, "Allow publishing reports that explicitly mention unresolved harness noise")
	// --mcp-smoke exercises read-only MCP smoke tools before evaluation.
	flag.BoolVar(&opts.MCPSmoke, "mcp-smoke", false, "Call read-only smoke tools through MCP against --backend=gitlab before evaluation")
	// --execute-tools runs validated calls through MCP instead of simulating.
	flag.BoolVar(&opts.Execute, "execute-tools", false, "Execute validated model tool calls through MCP instead of simulated tool results; requires --backend=gitlab and E2E_MODE=docker unless --allow-live-mutations is set")
	// --expose-resources exposes MCP resources, prompts, completions, and capability metadata to models.
	flag.BoolVar(&opts.ExposeResources, "expose-resources", true, "Expose MCP resources, prompts, completions, and capability metadata to model providers through evaluator bridge tools")
	// --allow-live-mutations allows execute-tools against non-Docker GitLab.
	flag.BoolVar(&opts.AllowLive, "allow-live-mutations", false, "Allow --execute-tools against non-Docker GitLab instances; dangerous because evaluation tasks may mutate resources")
	// --prepare-fixtures creates/refreshes live Docker GitLab resources.
	flag.BoolVar(&opts.PrepareFixtures, "prepare-fixtures", false, "Create or refresh Docker GitLab resources referenced by the evaluation fixture")
	// --fixtures-only exits after prepare-fixtures writes state.
	flag.BoolVar(&opts.FixturesOnly, "fixtures-only", false, "Exit after --prepare-fixtures writes fixture state")
	// --use-fixtures substitutes fixture IDs into task prompts.
	flag.BoolVar(&opts.UseFixtures, "use-fixtures", false, "Replace fixture placeholder IDs in task prompts with IDs from --fixtures")
	// --docker-auto-start boots the Docker fixture stack automatically.
	flag.BoolVar(&opts.DockerAutoStart, "docker-auto-start", false, "For Docker presets, start and provision the Docker GitLab fixture stack before connecting to --backend=gitlab")
	// --docker-wait-timeout caps the wait for Docker GitLab readiness.
	flag.DurationVar(&opts.DockerWaitTimeout, "docker-wait-timeout", 10*time.Minute, "Maximum time to wait for Docker GitLab readiness when --docker-auto-start is enabled")
	// --skip-destructive skips destructive tasks.
	flag.BoolVar(&opts.SkipDestructive, flagSkipDestructive, false, "Skip tasks with destructive calls or destructive workflow steps")
	// --only-destructive runs only destructive tasks.
	flag.BoolVar(&opts.OnlyDestructive, flagOnlyDestructive, false, "Run only tasks with destructive calls or destructive workflow steps")
	// --skip-mutating skips tasks that mutate state.
	flag.BoolVar(&opts.SkipMutating, flagSkipMutating, false, "Skip tasks whose expected calls mutate GitLab state")
	// --only-mutating runs only mutating tasks.
	flag.BoolVar(&opts.OnlyMutating, flagOnlyMutating, false, "Run only tasks whose expected calls mutate GitLab state")
	// --skip-unavailable skips tasks whose routes/fixtures are unavailable.
	flag.BoolVar(&opts.SkipUnavailable, flagSkipUnavailable, false, "Skip tasks whose expected routes or live fixtures are unavailable")
	// --print-output echoes progress to the terminal in addition to the log.
	flag.BoolVar(&opts.PrintOutput, "print-output", false, "Echo command progress and optional report output to the terminal in addition to --terminal-log")
	// --trace-provider-bodies includes raw provider bodies in trace artifacts.
	flag.BoolVar(&opts.TraceProviderBodies, "trace-provider-bodies", false, "Include raw model provider request and response bodies in trace artifacts")
	flag.Parse()
	opts.explicitFlags = map[string]bool{}
	flag.Visit(func(f *flag.Flag) {
		opts.explicitFlags[f.Name] = true
	})
	return opts
}

// applyPresetDefaults handles apply preset defaults and returns [options].
func applyPresetDefaults(opts options) (options, error) {
	preset := strings.TrimSpace(opts.Preset)
	if preset == "" {
		return opts, nil
	}
	if !validPreset(preset) {
		return opts, fmt.Errorf("unknown --preset %q", preset)
	}
	opts.Preset = preset
	switch preset {
	case presetSchemaEnterprise:
		setStringDefault(&opts.Edition, opts, "edition", editionEnterprise)
		setBoolDefault(&opts.DryRun, opts, "dry-run")
		setBoolDefault(&opts.SkipUnavailable, opts, flagSkipUnavailable)
	case presetDockerRead:
		setStringDefault(&opts.Edition, opts, "edition", editionCE)
		applyDockerPresetDefaults(&opts, partitionBaseRead)
		setBoolDefault(&opts.SkipMutating, opts, flagSkipMutating)
		setBoolDefault(&opts.SkipDestructive, opts, flagSkipDestructive)
	case presetDockerMutatingSafe:
		setStringDefault(&opts.Edition, opts, "edition", editionCE)
		applyDockerPresetDefaults(&opts, partitionBaseMutating)
		setBoolDefault(&opts.OnlyMutating, opts, flagOnlyMutating)
		setBoolDefault(&opts.SkipDestructive, opts, flagSkipDestructive)
	case presetDockerDestructiveSafe:
		setStringDefault(&opts.Edition, opts, "edition", editionCE)
		applyDockerPresetDefaults(&opts, partitionBaseDestructive)
		setBoolDefault(&opts.OnlyDestructive, opts, flagOnlyDestructive)
	case presetDockerEnterpriseRead:
		setStringDefault(&opts.Edition, opts, "edition", editionEnterprise)
		applyDockerPresetDefaults(&opts, partitionEnterpriseRead)
		setBoolDefault(&opts.SkipMutating, opts, flagSkipMutating)
		setBoolDefault(&opts.SkipDestructive, opts, flagSkipDestructive)
	case presetDockerEnterpriseMutatingSafe:
		setStringDefault(&opts.Edition, opts, "edition", editionEnterprise)
		applyDockerPresetDefaults(&opts, partitionEnterpriseMutating)
		setBoolDefault(&opts.OnlyMutating, opts, flagOnlyMutating)
		setBoolDefault(&opts.SkipDestructive, opts, flagSkipDestructive)
	case presetDockerEnterpriseDestructiveSafe:
		setStringDefault(&opts.Edition, opts, "edition", editionEnterprise)
		applyDockerPresetDefaults(&opts, partitionEnterpriseDestructive)
		setBoolDefault(&opts.OnlyDestructive, opts, flagOnlyDestructive)
	case presetDockerCapabilityDiscovery:
		setStringDefault(&opts.Edition, opts, "edition", editionCE)
		applyDockerPresetDefaults(&opts, partitionCapabilityFallback)
		setBoolDefault(&opts.SkipMutating, opts, flagSkipMutating)
		setBoolDefault(&opts.SkipDestructive, opts, flagSkipDestructive)
	case presetDockerErrorRecovery:
		setStringDefault(&opts.Edition, opts, "edition", editionCE)
		applyDockerPresetDefaults(&opts, partitionErrorRecovery)
		setBoolDefault(&opts.SkipMutating, opts, flagSkipMutating)
		setBoolDefault(&opts.SkipDestructive, opts, flagSkipDestructive)
	}
	return opts, nil
}

// applyDockerPresetDefaults applies docker preset defaults transformations.
func applyDockerPresetDefaults(opts *options, partition string) {
	setStringDefault(&opts.Backend, *opts, "backend", backendGitLab)
	setStringDefault(&opts.GitLabEnv, *opts, "gitlab-env-file", "test/e2e/.env.docker")
	setBoolDefault(&opts.DockerAutoStart, *opts, "docker-auto-start")
	setStringDefault(&opts.Partition, *opts, "partition", partition)
	setBoolDefault(&opts.Execute, *opts, "execute-tools")
	setBoolDefault(&opts.UseFixtures, *opts, "use-fixtures")
	setBoolDefault(&opts.SkipUnavailable, *opts, flagSkipUnavailable)
}

// validPreset reports whether valid preset.
func validPreset(preset string) bool {
	switch preset {
	case presetSchemaEnterprise, presetDockerRead, presetDockerMutatingSafe, presetDockerDestructiveSafe, presetDockerEnterpriseRead, presetDockerEnterpriseMutatingSafe, presetDockerEnterpriseDestructiveSafe, presetDockerCapabilityDiscovery, presetDockerErrorRecovery:
		return true
	default:
		return false
	}
}

// setStringDefault configures string default for the evaluator package.
func setStringDefault(target *string, opts options, flagName, value string) {
	if !opts.explicitFlags[flagName] {
		*target = value
	}
}

// setBoolDefault configures bool default for the evaluator package.
func setBoolDefault(target *bool, opts options, flagName string) {
	if !opts.explicitFlags[flagName] {
		*target = true
	}
}

// normalizeEvalEdition validates the task edition selector.
func normalizeEvalEdition(edition string) (string, error) {
	edition = strings.ToLower(strings.TrimSpace(edition))
	if edition == "" {
		return editionAll, nil
	}
	switch edition {
	case editionAll, editionCE, editionEnterprise:
		return edition, nil
	default:
		return "", fmt.Errorf("--edition must be %q, %q, or %q, got %q", editionAll, editionCE, editionEnterprise, edition)
	}
}

// filterTasks filters tasks using evaluator options.

func normalizedBackend(backend string) string {
	backend = strings.TrimSpace(backend)
	if backend == "" {
		return backendMock
	}
	return backend
}

// normalizeEvalToolSurface validates the model-facing tool catalog surface.
func normalizeEvalToolSurface(toolSurface string) (string, error) {
	surface := strings.ToLower(strings.TrimSpace(toolSurface))
	if surface == "" {
		return config.DefaultToolSurface, nil
	}
	surface, _, err := config.ParseToolSurface(surface, "true")
	if err != nil {
		return "", err
	}
	switch surface {
	case config.ToolSurfaceMeta, config.ToolSurfaceDynamic:
		return surface, nil
	default:
		return "", fmt.Errorf("--tool-surface must be %q or %q, got %q", config.ToolSurfaceMeta, config.ToolSurfaceDynamic, toolSurface)
	}
}

// isDynamicEvalSurface reports whether the selected surface uses dynamic discovery.
func isDynamicEvalSurface(toolSurface string) bool {
	switch toolSurface {
	case config.ToolSurfaceDynamic:
		return true
	default:
		return false
	}
}

// toolExecutionMode converts the GitLab API response to the tool output format.
func toolExecutionMode(opts options) string {
	if opts.FixtureSmoke {
		return "fixture-smoke"
	}
	if opts.DryRun {
		return "none"
	}
	if opts.Execute {
		if strings.TrimSpace(opts.MCPCommand) != "" {
			return "mcp-external"
		}
		return "mcp"
	}
	return "simulated"
}

// defaultOutputPath returns the default output path.
func defaultOutputPath(model string) string {
	stamp := time.Now().UTC().Format(timestampLayout)
	if strings.Contains(model, ",") {
		model = "multi-model"
	}
	model = strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(model)
	return filepath.Join(defaultEvalDir, fmt.Sprintf("model-%s-%s.md", stamp, model))
}

// defaultComparisonOutputPath returns the default comparison output path.
func defaultComparisonOutputPath() string {
	stamp := time.Now().UTC().Format(timestampLayout)
	return filepath.Join(defaultEvalDir, "comparison", stamp+"-summary.md")
}

// defaultTraceDir returns the default trace dir.
func defaultTraceDir(reportPath string) string {
	ext := filepath.Ext(reportPath)
	if ext == "" {
		return reportPath + ".traces"
	}
	return strings.TrimSuffix(reportPath, ext) + ".traces"
}

func defaultTerminalLogPath(outputPath string) string {
	if strings.TrimSpace(outputPath) == "" {
		stamp := time.Now().UTC().Format(timestampLayout)
		return filepath.Join(defaultEvalDir, "terminal", stamp+".log")
	}
	ext := filepath.Ext(outputPath)
	if ext == "" {
		return outputPath + ".log"
	}
	return strings.TrimSuffix(outputPath, ext) + ".log"
}
