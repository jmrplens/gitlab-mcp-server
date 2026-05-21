// Command eval_mcp_surfaces evaluates model behavior across MCP tool surfaces.
// By default it uses a mock GitLab client for catalog generation;
// --backend=gitlab points the in-memory MCP server at a real GitLab instance
// such as the Docker E2E environment.
//
// Usage:
//
//	go run ./cmd/eval_mcp_surfaces/
//	go run ./cmd/eval_mcp_surfaces/ --max-tasks=5
//	go run ./cmd/eval_mcp_surfaces/ --dry-run
//	go run ./cmd/eval_mcp_surfaces/ --tools-file /tmp/tools_mcp_surfaces.json
//	go run ./cmd/eval_mcp_surfaces/ --publish-docs --publish-from dist/evaluation/mcp-surfaces/docker-read.md
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "eval_mcp_surfaces: %v\n", err)
		os.Exit(1)
	}
}

// run runs resources for the main package.
func run() (runErr error) {
	opts, closeTerminalOutput, err := prepareRunOptions()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeTerminalOutput(); closeErr != nil {
			runErr = errors.Join(runErr, closeErr)
		}
	}()
	defer func() {
		if runErr != nil {
			terminalLogPrintf("eval_mcp_surfaces: %v\n", runErr)
		}
	}()
	handled, immediateErr := runImmediateMode(opts)
	if handled {
		return immediateErr
	}
	opts, modelSpecs, err := resolveRunModels(opts)
	if err != nil {
		return err
	}
	finalReportWritten := false
	if shouldWriteStartupReport(opts) {
		if writeErr := writeStartupReport(opts.Output, opts); writeErr != nil {
			return writeErr
		}
		defer func() {
			if runErr == nil || finalReportWritten {
				return
			}
			if writeErr := writeErrorReport(opts.Output, opts, runErr); writeErr != nil {
				runErr = errors.Join(runErr, writeErr)
			}
		}()
	}
	tasks, fixtures, err := prepareRunTasks(opts)
	if err != nil {
		return err
	}
	if opts.PrepareFixtures && opts.FixturesOnly {
		return nil
	}
	catalog, routes, tasks, err := prepareRunCatalog(opts, tasks, fixtures)
	if err != nil {
		return err
	}
	if opts.DryRun {
		if dryRunErr := runDryRunEvaluation(opts, tasks, catalog, routes); dryRunErr != nil {
			return dryRunErr
		}
		finalReportWritten = true
		return nil
	}
	runtime, err := newEvaluationRuntime(opts, catalog)
	if err != nil {
		return err
	}
	defer runtime.close()
	results, err := runModelEvaluations(context.Background(), runtime.opts, tasks, modelSpecs, runtime.catalog, routes, runtime)
	if err != nil {
		return err
	}
	if writeErr := writeReport(runtime.opts.Output, runtime.opts, results, runtime.catalog, routes, false); writeErr != nil {
		return writeErr
	}
	finalReportWritten = true
	if coverageErr := writeCoverageReportIfRequested(runtime.opts, results, routes); coverageErr != nil {
		return coverageErr
	}
	return writeTraceArtifacts(runtime.opts.TraceDir, results, runtime.opts.TraceProviderBodies)
}

func prepareRunOptions() (options, func() error, error) {
	opts := parseFlags()
	closeTerminalOutput := func() error { return nil }
	if shouldConfigureTerminalOutput(opts) {
		var terminalErr error
		opts, closeTerminalOutput, terminalErr = configureTerminalOutput(opts)
		if terminalErr != nil {
			return options{}, nil, terminalErr
		}
	}
	var presetErr error
	opts, presetErr = applyPresetDefaults(opts)
	if presetErr != nil {
		return options{}, nil, presetErr
	}
	var surfaceErr error
	opts.ToolSurface, surfaceErr = normalizeEvalToolSurface(opts.ToolSurface)
	if surfaceErr != nil {
		return options{}, nil, surfaceErr
	}
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return options{}, nil, fmt.Errorf("load .env: %w", err)
	}
	if opts.GitLabEnv != "" {
		if err := godotenv.Overload(opts.GitLabEnv); err != nil {
			return options{}, nil, fmt.Errorf("load gitlab env file %s: %w", opts.GitLabEnv, err)
		}
	}
	return opts, closeTerminalOutput, nil
}

func runImmediateMode(opts options) (bool, error) {
	if opts.PublishDocs || opts.CheckDocs {
		return true, publishEvaluationDocs(opts)
	}
	if len(opts.CheckEfficiency) > 0 {
		return true, runEfficiencyCheck(opts)
	}
	if len(opts.CompareTraces) > 0 {
		return true, runTraceComparison(opts)
	}
	if len(opts.CompareReports) > 0 {
		if opts.Output == "" {
			opts.Output = defaultComparisonOutputPath()
		}
		return true, writeComparisonReport(opts.Output, opts.CompareReports)
	}
	return false, nil
}

func resolveRunModels(opts options) (options, []modelSpec, error) {
	var modelSpecs []modelSpec
	if !opts.DryRun {
		var modelErr error
		modelSpecs, modelErr = resolveModelSpecs(opts)
		if modelErr != nil {
			return options{}, nil, modelErr
		}
		opts.Model = modelReportLabel(modelSpecs)
	} else if opts.Model == "" {
		opts.Model = "none"
	}
	if opts.Output == "" {
		opts.Output = defaultOutputPath(opts.Model)
	}
	if opts.TraceDir == "" && !opts.DryRun {
		opts.TraceDir = defaultTraceDir(opts.Output)
	}
	return opts, modelSpecs, nil
}

func prepareRunTasks(opts options) ([]evalTask, *liveFixtureState, error) {
	var fixtures *liveFixtureState
	if opts.PrepareFixtures {
		prepared, prepareErr := prepareLiveFixtures(opts)
		if prepareErr != nil {
			return nil, nil, prepareErr
		}
		fixtures = prepared
		if writeErr := writeLiveFixtures(opts.Fixtures, fixtures); writeErr != nil {
			return nil, nil, writeErr
		}
		terminalPrintf("fixtures: wrote %s for %s\n", opts.Fixtures, fixtures.ProjectPath)
		if opts.FixturesOnly {
			return nil, fixtures, nil
		}
	}
	tasks, parseErr := parseTasksFile(opts.TasksPath)
	if parseErr != nil {
		return nil, nil, parseErr
	}
	if opts.UseFixtures || opts.PrepareFixtures {
		if fixtures == nil {
			var readErr error
			fixtures, readErr = readLiveFixtures(opts.Fixtures)
			if readErr != nil {
				return nil, nil, readErr
			}
		}
		tasks = applyLiveFixtureState(tasks, fixtures)
	}
	tasks = filterTasks(tasks, opts.OnlyIDs)
	var filterErr error
	tasks, filterErr = filterTasksByDestructive(tasks, opts.SkipDestructive, opts.OnlyDestructive)
	if filterErr != nil {
		return nil, nil, filterErr
	}
	tasks, filterErr = filterTasksByMutation(tasks, opts.SkipMutating, opts.OnlyMutating)
	if filterErr != nil {
		return nil, nil, filterErr
	}
	if len(tasks) == 0 {
		return nil, nil, errors.New("no tasks selected")
	}
	if opts.Repeat < 1 {
		return nil, nil, errors.New("repeat must be >= 1")
	}
	if problems := validateTaskFixture(tasks); len(problems) > 0 {
		return nil, nil, fmt.Errorf("fixture validation failed:\n- %s", strings.Join(problems, "\n- "))
	}
	return tasks, fixtures, nil
}

func prepareRunCatalog(opts options, tasks []evalTask, fixtures *liveFixtureState) ([]modelTool, map[string]toolutil.ActionMap, []evalTask, error) {
	catalog, routes, catalogErr := loadCatalog(opts)
	if catalogErr != nil {
		return nil, nil, nil, catalogErr
	}
	if opts.MCPSmoke {
		if smokeErr := runMCPSmoke(opts); smokeErr != nil {
			return nil, nil, nil, smokeErr
		}
	}
	tasks = normalizeTasksForCatalog(tasks, routes, opts.ToolSurface)
	if opts.Partition != "" {
		var partitionErr error
		tasks, partitionErr = filterTasksByPartition(tasks, opts.Partition)
		if partitionErr != nil {
			return nil, nil, nil, partitionErr
		}
		if len(tasks) == 0 {
			return nil, nil, nil, fmt.Errorf("no tasks selected after --partition=%s", opts.Partition)
		}
	}
	if opts.SkipUnavailable {
		tasks = filterTasksByAvailableRoutes(tasks, routes)
		if fixtures != nil {
			tasks = filterTasksByLiveFixtureState(tasks, fixtures)
		}
		if len(tasks) == 0 {
			return nil, nil, nil, errors.New("no tasks selected after --skip-unavailable")
		}
	}
	if opts.Execute && opts.UseFixtures {
		tasks = orderSharedFixtureDestructiveLast(tasks)
	}
	if opts.Preset != "" {
		var presetFilterErr error
		tasks, presetFilterErr = filterTasksByPreset(tasks, opts.Preset)
		if presetFilterErr != nil {
			return nil, nil, nil, presetFilterErr
		}
		if len(tasks) == 0 {
			return nil, nil, nil, fmt.Errorf("no tasks selected after --preset=%s", opts.Preset)
		}
	}
	if opts.MaxTasks > 0 && opts.MaxTasks < len(tasks) {
		tasks = tasks[:opts.MaxTasks]
	}
	if opts.ToolsFile == "" {
		if problems := validateTaskFixtureAgainstRoutes(tasks, routes); len(problems) > 0 {
			return nil, nil, nil, fmt.Errorf("fixture route validation failed:\n- %s", strings.Join(problems, "\n- "))
		}
	}
	return catalog, routes, tasks, nil
}

func runDryRunEvaluation(opts options, tasks []evalTask, catalog []modelTool, routes map[string]toolutil.ActionMap) error {
	if opts.ExposeResources {
		bridgeSupport := mcpBridgeSupport{Capabilities: true, Resources: true, Prompts: true, Completion: true}
		catalog = appendCapabilityBridgeTools(catalog, bridgeSupport)
		opts.CapabilityAccessActive = true
		opts.ResourceAccessActive = true
		opts.PromptAccessActive = true
		opts.CompletionAccessActive = true
	}
	toolNames := catalogToolNames(catalog)
	results := make([]taskResult, 0, len(tasks)*opts.Repeat)
	for runIndex := 1; runIndex <= opts.Repeat; runIndex++ {
		results = append(results, runStaticValidation(tasks, routes, toolNames, runIndex)...)
	}
	if err := writeReport(opts.Output, opts, results, catalog, routes, true); err != nil {
		return err
	}
	return writeCoverageReportIfRequested(opts, results, routes)
}

type evaluationRuntime struct {
	opts            options
	catalog         []modelTool
	mcpSession      *mcp.ClientSession
	executionClient *gitlabclient.Client
	bridgeSupport   mcpBridgeSupport
	close           func()
}

func newEvaluationRuntime(opts options, catalog []modelTool) (evaluationRuntime, error) {
	runtime := evaluationRuntime{opts: opts, catalog: catalog, close: func() {}}
	var closers []func()
	var mcpSession *mcp.ClientSession
	if opts.Execute {
		session, client, closeSession, execErr := newExecutionSession(opts)
		if execErr != nil {
			return evaluationRuntime{}, execErr
		}
		mcpSession = session
		runtime.executionClient = client
		closers = append(closers, closeSession)
	}
	if opts.ExposeResources && mcpSession == nil && opts.ToolsFile == "" {
		session, closeSession, resourceErr := newResourceLookupSession(opts)
		if resourceErr != nil {
			return evaluationRuntime{}, resourceErr
		}
		mcpSession = session
		closers = append(closers, closeSession)
	}
	if opts.ExposeResources && mcpSession != nil {
		runtime.bridgeSupport = probeCapabilityBridgeSupport(mcpSession)
		if runtime.bridgeSupport.any() {
			runtime.catalog = appendCapabilityBridgeTools(runtime.catalog, runtime.bridgeSupport)
			runtime.opts.CapabilityAccessActive = runtime.bridgeSupport.Capabilities
			runtime.opts.ResourceAccessActive = runtime.bridgeSupport.Resources
			runtime.opts.PromptAccessActive = runtime.bridgeSupport.Prompts
			runtime.opts.CompletionAccessActive = runtime.bridgeSupport.Completion
		}
	}
	runtime.mcpSession = mcpSession
	runtime.close = closeRuntimeSessions(closers)
	return runtime, nil
}

func closeRuntimeSessions(closers []func()) func() {
	return func() {
		for _, closeSession := range closers {
			closeSession()
		}
	}
}

func runModelEvaluations(ctx context.Context, opts options, tasks []evalTask, modelSpecs []modelSpec, catalog []modelTool, routes map[string]toolutil.ActionMap, runtime evaluationRuntime) ([]taskResult, error) {
	results := make([]taskResult, 0, len(tasks)*opts.Repeat*len(modelSpecs))
	liveAttemptRunSuffix := liveUniqueSuffix()
	for _, spec := range modelSpecs {
		apiKey, keyErr := apiKeyForModelProvider(spec.Provider)
		if keyErr != nil {
			return nil, keyErr
		}
		runner := &modelRunner{
			apiKey:      apiKey,
			provider:    spec.Provider,
			model:       spec.Model,
			modelLabel:  spec.String(),
			toolSurface: opts.ToolSurface,
			maxTokens:   opts.MaxTokens,
			retries:     opts.Retries,
			retryWait:   opts.RetryWait,
			client:      &http.Client{Timeout: 60 * time.Second},
			mcpSession:  runtime.mcpSession,
			mcpBridge:   runtime.bridgeSupport,
			traceBodies: opts.TraceProviderBodies,
		}
		for runIndex := 1; runIndex <= opts.Repeat; runIndex++ {
			if opts.Execute && opts.UseFixtures {
				if err := ensureLiveProjectActive(ctx, runtime.executionClient); err != nil {
					return nil, err
				}
			}
			for _, task := range tasks {
				taskForAttempt := task
				if opts.Execute && opts.UseFixtures {
					taskForAttempt = addLiveAttemptResourceSuffix(taskForAttempt, spec.String(), runIndex, liveAttemptRunSuffix)
					var err error
					taskForAttempt, err = ensureLiveAttemptResources(ctx, runtime.executionClient, runtime.mcpSession, taskForAttempt, opts.ToolSurface)
					if err != nil {
						return nil, err
					}
				}
				result := runner.evaluateTask(ctx, taskForAttempt, catalog, routes)
				result.Run = runIndex
				result.Model = spec.String()
				result.Trace.Run = runIndex
				result.Trace.Model = spec.String()
				result.Trace.Summary = traceSummaryFromResult(result)
				results = append(results, result)
				terminalPrintf("model=%s run=%d %s: final=%t first=%s/%s final_call=%s/%s calls=%d tools=%d\n", spec.String(), runIndex, taskForAttempt.ID, result.FinalSuccess, result.FirstTool, result.FirstAction, result.FinalTool, result.FinalAction, result.ModelCalls, result.ToolCalls)
				if opts.Pause > 0 {
					time.Sleep(opts.Pause)
				}
			}
		}
	}
	return results, nil
}

// parseFlags parses flags from evaluator input.
