// Package evaluator implements the eval_mcp_surfaces command workflow.
package evaluator

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

// Run executes the eval_mcp_surfaces command workflow.
func Run() (runErr error) {
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
	if envErr := prepareRunEnvironment(opts); envErr != nil {
		return envErr
	}
	opts, modelSpecs, err := resolveRunModels(opts)
	if err != nil {
		return err
	}
	finalReportWritten := false
	cleanupReport, err := prepareRunFailureReport(
		opts,
		func() error { return runErr },
		func(err error) { runErr = err },
		func() bool { return finalReportWritten },
	)
	if err != nil {
		return err
	}
	defer cleanupReport()
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
		if dryRunErr := runDryRunEvaluation(context.Background(), opts, tasks, catalog, routes); dryRunErr != nil {
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
	results, err := runModelEvaluations(context.Background(), modelEvaluationRun{
		opts:    runtime.opts,
		tasks:   tasks,
		catalog: runtime.catalog,
		routes:  routes,
		runtime: runtime,
	}, modelSpecs)
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

func prepareRunFailureReport(opts options, currentRunErr func() error, setRunErr func(error), finalReportWritten func() bool) (func(), error) {
	if !shouldWriteStartupReport(opts) {
		return func() { /* no cleanup needed when startup report is skipped */ }, nil
	}
	if writeErr := writeStartupReport(opts.Output, opts); writeErr != nil {
		return nil, writeErr
	}
	return func() {
		if currentRunErr() == nil || finalReportWritten() {
			return
		}
		if writeErr := writeErrorReport(opts.Output, opts, currentRunErr()); writeErr != nil {
			setRunErr(errors.Join(currentRunErr(), writeErr))
		}
	}, nil
}

func prepareRunOptions() (options, func() error, error) {
	opts := parseFlags()
	closeTerminalOutput := noopCloseTerminalOutput
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
	var editionErr error
	opts.Edition, editionErr = normalizeEvalEdition(opts.Edition)
	if editionErr != nil {
		return options{}, nil, editionErr
	}
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return options{}, nil, fmt.Errorf("load .env: %w", err)
	}
	return opts, closeTerminalOutput, nil
}

func prepareRunEnvironment(opts options) error {
	if err := ensureDockerRuntimeIfNeeded(context.Background(), opts); err != nil {
		return err
	}
	if opts.GitLabEnv != "" {
		if err := godotenv.Overload(opts.GitLabEnv); err != nil {
			return fmt.Errorf("load gitlab env file %s: %w", opts.GitLabEnv, err)
		}
	}
	return nil
}

func noopCloseTerminalOutput() error {
	return nil
}

func runImmediateMode(opts options) (bool, error) {
	if opts.PublishDocs || opts.CheckDocs {
		return true, publishEvaluationDocs(opts)
	}
	if len(opts.CheckEfficiency) > 0 {
		return true, runEfficiencyCheck(opts)
	}
	if len(opts.CheckReportClean) > 0 {
		return true, runReportCleanCheck(opts)
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
	evalCases, parseErr := loadEvalCases(opts)
	if parseErr != nil {
		return nil, nil, parseErr
	}
	tasks := evalTasksFromCases(evalCases)
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
	catalog, routes, catalogEnterprise, catalogErr := loadCatalog(opts)
	if catalogErr != nil {
		return nil, nil, nil, catalogErr
	}
	if opts.MCPSmoke {
		if smokeErr := runMCPSmoke(opts); smokeErr != nil {
			return nil, nil, nil, smokeErr
		}
	}
	tasks = normalizeTasksForCatalog(tasks, routes, opts.ToolSurface)
	var err error
	if tasks, err = applyEditionFilter(tasks, opts.Edition); err != nil {
		return nil, nil, nil, err
	}
	if tasks, err = applyPartitionFilter(tasks, opts.Partition); err != nil {
		return nil, nil, nil, err
	}
	if tasks, err = applyAvailabilityFilter(tasks, routes, catalogEnterprise, fixtures, opts.SkipUnavailable); err != nil {
		return nil, nil, nil, err
	}
	if opts.Execute && opts.UseFixtures {
		tasks = orderSharedFixtureDestructiveLast(tasks)
	}
	if tasks, err = applyPresetFilter(tasks, opts.Preset); err != nil {
		return nil, nil, nil, err
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

func applyEditionFilter(tasks []evalTask, edition string) ([]evalTask, error) {
	filtered, err := filterTasksByEdition(tasks, edition)
	if err != nil {
		return nil, err
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no tasks selected after --edition=%s", edition)
	}
	return filtered, nil
}

func applyPartitionFilter(tasks []evalTask, partition string) ([]evalTask, error) {
	if partition == "" {
		return tasks, nil
	}
	filtered, err := filterTasksByPartition(tasks, partition)
	if err != nil {
		return nil, err
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no tasks selected after --partition=%s", partition)
	}
	return filtered, nil
}

func applyAvailabilityFilter(tasks []evalTask, routes map[string]toolutil.ActionMap, catalogEnterprise bool, fixtures *liveFixtureState, skipUnavailable bool) ([]evalTask, error) {
	if !skipUnavailable {
		return tasks, nil
	}
	filtered := filterTasksByAvailableRoutes(tasks, routes, catalogEnterprise)
	if fixtures != nil {
		filtered = filterTasksByLiveFixtureState(filtered, fixtures)
	}
	if len(filtered) == 0 {
		return nil, errors.New("no tasks selected after --skip-unavailable")
	}
	return filtered, nil
}

func applyPresetFilter(tasks []evalTask, preset string) ([]evalTask, error) {
	if preset == "" {
		return tasks, nil
	}
	filtered, err := filterTasksByPreset(tasks, preset)
	if err != nil {
		return nil, err
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no tasks selected after --preset=%s", preset)
	}
	return filtered, nil
}

func runDryRunEvaluation(ctx context.Context, opts options, tasks []evalTask, catalog []modelTool, routes map[string]toolutil.ActionMap) error {
	if opts.FixtureSmoke {
		return runFixtureSmokeEvaluation(ctx, opts, tasks, catalog, routes)
	}
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

func runFixtureSmokeEvaluation(ctx context.Context, opts options, tasks []evalTask, catalog []modelTool, routes map[string]toolutil.ActionMap) error {
	if !opts.Execute || !opts.UseFixtures {
		return errors.New("--fixture-smoke requires --execute-tools and --use-fixtures")
	}
	runtime, err := newEvaluationRuntime(opts, catalog) //nolint:contextcheck // Runtime setup uses existing session APIs; per-task fixture preparation below receives ctx.
	if err != nil {
		return err
	}
	defer runtime.close()
	results, err := runFixtureSmokeAttempts(ctx, modelEvaluationRun{
		opts:                 runtime.opts,
		tasks:                tasks,
		catalog:              runtime.catalog,
		routes:               routes,
		runtime:              runtime,
		liveAttemptRunSuffix: liveUniqueSuffix(),
	}, modelSpec{Model: "fixture-smoke"})
	if err != nil {
		return err
	}
	if writeErr := writeReport(runtime.opts.Output, runtime.opts, results, runtime.catalog, routes, true); writeErr != nil {
		return writeErr
	}
	return writeCoverageReportIfRequested(runtime.opts, results, routes)
}

func runFixtureSmokeAttempts(ctx context.Context, run modelEvaluationRun, spec modelSpec) ([]taskResult, error) {
	results := make([]taskResult, 0, len(run.tasks)*run.opts.Repeat)
	for runIndex := 1; runIndex <= run.opts.Repeat; runIndex++ {
		if err := ensureLiveProjectActive(ctx, run.runtime.executionClient); err != nil {
			return nil, err
		}
		for _, task := range run.tasks {
			taskForAttempt, err := prepareTaskAttempt(ctx, run.opts, spec, runIndex, task, run.runtime, run.liveAttemptRunSuffix)
			if err != nil {
				return nil, fmt.Errorf("fixture smoke %s: %w", task.ID, err)
			}
			results = append(results, fixtureSmokeResult(taskForAttempt, spec, run.opts.ToolSurface, runIndex))
			terminalPrintf("fixture-smoke model=%s run=%d %s: ok\n", spec.String(), runIndex, taskForAttempt.ID)
		}
	}
	return results, nil
}

func fixtureSmokeResult(task evalTask, spec modelSpec, toolSurface string, runIndex int) taskResult {
	steps := taskSteps(task)
	first := steps[0]
	last := steps[len(steps)-1]
	return taskResult{
		Task:            task,
		Run:             runIndex,
		Model:           spec.String(),
		ToolSurface:     toolSurface,
		FirstTool:       first.ExpectedTool,
		FirstAction:     first.ExpectedAction,
		FirstPass:       true,
		FinalTool:       last.ExpectedTool,
		FinalAction:     last.ExpectedAction,
		FinalSuccess:    true,
		DestructiveSafe: true,
		CompletedSteps:  len(steps),
		Notes:           []string{"live fixture smoke prepared resources"},
	}
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
	runtime := evaluationRuntime{opts: opts, catalog: catalog, close: noopCloseRuntime}
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

func noopCloseRuntime() {
	// No runtime sessions were opened, so there is nothing to close.
}

func closeRuntimeSessions(closers []func()) func() {
	return func() {
		for _, closeSession := range closers {
			closeSession()
		}
	}
}

type modelEvaluationRun struct {
	opts                 options
	tasks                []evalTask
	catalog              []modelTool
	routes               map[string]toolutil.ActionMap
	runtime              evaluationRuntime
	liveAttemptRunSuffix string
}

func runModelEvaluations(ctx context.Context, run modelEvaluationRun, modelSpecs []modelSpec) ([]taskResult, error) {
	results := make([]taskResult, 0, len(run.tasks)*run.opts.Repeat*len(modelSpecs))
	run.liveAttemptRunSuffix = liveUniqueSuffix()
	for _, spec := range modelSpecs {
		specResults, err := runModelSpecEvaluations(ctx, run, spec)
		if err != nil {
			return nil, err
		}
		results = append(results, specResults...)
	}
	return results, nil
}

func runModelSpecEvaluations(ctx context.Context, run modelEvaluationRun, spec modelSpec) ([]taskResult, error) {
	runner, err := newModelRunner(run.opts, spec, run.runtime)
	if err != nil {
		return nil, err
	}
	results := make([]taskResult, 0, len(run.tasks)*run.opts.Repeat)
	for runIndex := 1; runIndex <= run.opts.Repeat; runIndex++ {
		runResults, runErr := runModelEvaluationRound(ctx, run, spec, runIndex, runner)
		if runErr != nil {
			return nil, runErr
		}
		results = append(results, runResults...)
	}
	return results, nil
}

func newModelRunner(opts options, spec modelSpec, runtime evaluationRuntime) (*modelRunner, error) {
	apiKey, err := apiKeyForModelProvider(spec.Provider)
	if err != nil {
		return nil, err
	}
	return &modelRunner{
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
	}, nil
}

func runModelEvaluationRound(ctx context.Context, run modelEvaluationRun, spec modelSpec, runIndex int, runner *modelRunner) ([]taskResult, error) {
	if run.opts.Execute && run.opts.UseFixtures {
		if err := ensureLiveProjectActive(ctx, run.runtime.executionClient); err != nil {
			return nil, err
		}
	}
	results := make([]taskResult, 0, len(run.tasks))
	for taskIndex, task := range run.tasks {
		result := evaluateModelTaskAttempt(ctx, run, spec, runIndex, task, runner)
		if taskIndex == 0 {
			if err := fatalInitialProviderError(result); err != nil {
				return nil, err
			}
		}
		results = append(results, result)
	}
	return results, nil
}

func fatalInitialProviderError(result taskResult) error {
	if result.FinalSuccess || result.ToolCalls > 0 || result.CompletedSteps > 0 {
		return nil
	}
	for _, event := range result.Trace.Events {
		if event.Kind != "model_error" || event.Provider == nil {
			continue
		}
		switch event.Provider.ResponseStatus {
		case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
			note := strings.TrimSpace(strings.Join(result.Notes, "; "))
			if note == "" {
				note = fmt.Sprintf("%s returned HTTP %d", event.Provider.Provider, event.Provider.ResponseStatus)
			}
			return fmt.Errorf("model provider %s failed before tool execution with HTTP %d: %s; verify --models/EVAL_MODELS and provider credentials before running the full corpus", result.Model, event.Provider.ResponseStatus, note)
		}
	}
	return nil
}

func evaluateModelTaskAttempt(ctx context.Context, run modelEvaluationRun, spec modelSpec, runIndex int, task evalTask, runner *modelRunner) taskResult {
	attempt, err := prepareTaskAttemptValue(ctx, run.opts, spec, runIndex, task, run.runtime, run.liveAttemptRunSuffix)
	if err != nil {
		result := taskAttemptPreparationErrorResult(task, spec, run.opts.ToolSurface, runIndex, err)
		terminalPrintf("model=%s run=%d %s: fixture=false error=%v\n", spec.String(), runIndex, task.ID, err)
		return result
	}
	prepared := attempt.PreparedCase()
	configureEvalElicitationFromOutput(prepared.FixtureOutputs)
	result := runner.evaluatePreparedCase(ctx, prepared, run.catalog, run.routes)
	result.Run = runIndex
	result.Model = spec.String()
	result.Trace.Run = runIndex
	result.Trace.Model = spec.String()
	result.Trace.Summary = traceSummaryFromResult(result)
	terminalPrintf("model=%s run=%d %s: final=%t first=%s/%s final_call=%s/%s calls=%d tools=%d\n", spec.String(), runIndex, attempt.Task.ID, result.FinalSuccess, result.FirstTool, result.FirstAction, result.FinalTool, result.FinalAction, result.ModelCalls, result.ToolCalls)
	if run.opts.Pause > 0 {
		time.Sleep(run.opts.Pause)
	}
	return result
}

type preparedTaskAttempt struct {
	Task     evalTask
	Prepared *PreparedCase
}

func (attempt preparedTaskAttempt) PreparedCase() PreparedCase {
	if attempt.Prepared != nil {
		return *attempt.Prepared
	}
	return preparedCaseFromTask(attempt.Task)
}

func taskAttemptPreparationErrorResult(task evalTask, spec modelSpec, toolSurface string, runIndex int, err error) taskResult {
	steps := taskSteps(task)
	first := steps[0]
	last := steps[len(steps)-1]
	result := taskResult{
		Task:            task,
		Run:             runIndex,
		Model:           spec.String(),
		ToolSurface:     toolSurface,
		FirstTool:       first.ExpectedTool,
		FirstAction:     first.ExpectedAction,
		FinalTool:       last.ExpectedTool,
		FinalAction:     last.ExpectedAction,
		DestructiveSafe: true,
		Notes:           []string{"fixture preparation failed: " + err.Error()},
		Trace:           newTaskTrace(task, "", task.Prompt),
	}
	result.Trace.Run = runIndex
	result.Trace.Model = spec.String()
	result.Trace.Events = append(result.Trace.Events, traceEvent{Kind: "fixture_error", IsError: true, Content: err.Error()})
	result.Trace.Summary = traceSummaryFromResult(result)
	return result
}

func prepareTaskAttempt(ctx context.Context, opts options, spec modelSpec, runIndex int, task evalTask, runtime evaluationRuntime, liveAttemptRunSuffix string) (evalTask, error) {
	attempt, err := prepareTaskAttemptValue(ctx, opts, spec, runIndex, task, runtime, liveAttemptRunSuffix)
	if err != nil {
		return task, err
	}
	return attempt.Task, nil
}

func prepareTaskAttemptValue(ctx context.Context, opts options, spec modelSpec, runIndex int, task evalTask, runtime evaluationRuntime, liveAttemptRunSuffix string) (preparedTaskAttempt, error) {
	if !opts.Execute || !opts.UseFixtures {
		return preparedTaskAttempt{Task: task}, nil
	}
	if caseUsesTypedFixtureEngine(task) {
		prepared, err := PrepareCaseAttempt(ctx, FixtureContext{
			Client:         runtime.executionClient,
			MCPSession:     runtime.mcpSession,
			RuntimeEdition: EvalCaseEdition(opts.Edition),
			ToolSurface:    opts.ToolSurface,
			RunSuffix:      liveAttemptRunSuffix,
		}, *task.Case, spec.String(), runIndex)
		if err != nil {
			return preparedTaskAttempt{Task: task}, err
		}
		prepared.Steps = cloneExpectedSteps(stepsFromTask(task))
		task = taskFromPreparedCase(prepared)
		return preparedTaskAttempt{Task: task, Prepared: &prepared}, nil
	}
	return preparedTaskAttempt{Task: task}, nil
}

// parseFlags parses flags from evaluator input.
