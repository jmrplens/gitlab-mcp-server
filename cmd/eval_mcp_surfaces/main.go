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
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "eval_mcp_surfaces: %v\n", err)
		os.Exit(1)
	}
}

// run runs resources for the main package.
func run() (runErr error) {
	opts := parseFlags()
	closeTerminalOutput := func() error { return nil }
	if shouldConfigureTerminalOutput(opts) {
		var terminalErr error
		opts, closeTerminalOutput, terminalErr = configureTerminalOutput(opts)
		if terminalErr != nil {
			return terminalErr
		}
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
	var presetErr error
	opts, presetErr = applyPresetDefaults(opts)
	if presetErr != nil {
		return presetErr
	}
	var surfaceErr error
	opts.ToolSurface, surfaceErr = normalizeEvalToolSurface(opts.ToolSurface)
	if surfaceErr != nil {
		return surfaceErr
	}
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	if opts.GitLabEnv != "" {
		if err := godotenv.Overload(opts.GitLabEnv); err != nil {
			return fmt.Errorf("load gitlab env file %s: %w", opts.GitLabEnv, err)
		}
	}
	if opts.PublishDocs || opts.CheckDocs {
		return publishEvaluationDocs(opts)
	}
	if len(opts.CheckEfficiency) > 0 {
		return runEfficiencyCheck(opts)
	}
	if len(opts.CompareTraces) > 0 {
		return runTraceComparison(opts)
	}
	if len(opts.CompareReports) > 0 {
		if opts.Output == "" {
			opts.Output = defaultComparisonOutputPath()
		}
		return writeComparisonReport(opts.Output, opts.CompareReports)
	}
	var modelSpecs []modelSpec
	if !opts.DryRun {
		var modelErr error
		modelSpecs, modelErr = resolveModelSpecs(opts)
		if modelErr != nil {
			return modelErr
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
	var fixtures *liveFixtureState
	if opts.PrepareFixtures {
		prepared, prepareErr := prepareLiveFixtures(opts)
		if prepareErr != nil {
			return prepareErr
		}
		fixtures = prepared
		if writeErr := writeLiveFixtures(opts.Fixtures, fixtures); writeErr != nil {
			return writeErr
		}
		terminalPrintf("fixtures: wrote %s for %s\n", opts.Fixtures, fixtures.ProjectPath)
		if opts.FixturesOnly {
			return nil
		}
	}
	tasks, parseErr := parseTasksFile(opts.TasksPath)
	if parseErr != nil {
		return parseErr
	}
	if opts.UseFixtures || opts.PrepareFixtures {
		if fixtures == nil {
			var readErr error
			fixtures, readErr = readLiveFixtures(opts.Fixtures)
			if readErr != nil {
				return readErr
			}
		}
		tasks = applyLiveFixtureState(tasks, fixtures)
	}
	tasks = filterTasks(tasks, opts.OnlyIDs)
	var filterErr error
	tasks, filterErr = filterTasksByDestructive(tasks, opts.SkipDestructive, opts.OnlyDestructive)
	if filterErr != nil {
		return filterErr
	}
	tasks, filterErr = filterTasksByMutation(tasks, opts.SkipMutating, opts.OnlyMutating)
	if filterErr != nil {
		return filterErr
	}
	if len(tasks) == 0 {
		return errors.New("no tasks selected")
	}
	if opts.Repeat < 1 {
		return errors.New("repeat must be >= 1")
	}
	if problems := validateTaskFixture(tasks); len(problems) > 0 {
		return fmt.Errorf("fixture validation failed:\n- %s", strings.Join(problems, "\n- "))
	}

	catalog, routes, catalogErr := loadCatalog(opts)
	if catalogErr != nil {
		return catalogErr
	}
	if opts.MCPSmoke {
		if smokeErr := runMCPSmoke(opts); smokeErr != nil {
			return smokeErr
		}
	}
	tasks = normalizeTasksForCatalog(tasks, routes, opts.ToolSurface)
	if opts.Partition != "" {
		var partitionErr error
		tasks, partitionErr = filterTasksByPartition(tasks, opts.Partition)
		if partitionErr != nil {
			return partitionErr
		}
		if len(tasks) == 0 {
			return fmt.Errorf("no tasks selected after --partition=%s", opts.Partition)
		}
	}
	if opts.SkipUnavailable {
		tasks = filterTasksByAvailableRoutes(tasks, routes)
		if fixtures != nil {
			tasks = filterTasksByLiveFixtureState(tasks, fixtures)
		}
		if len(tasks) == 0 {
			return errors.New("no tasks selected after --skip-unavailable")
		}
	}
	if opts.Execute && opts.UseFixtures {
		tasks = orderSharedFixtureDestructiveLast(tasks)
	}
	if opts.Preset != "" {
		var presetFilterErr error
		tasks, presetFilterErr = filterTasksByPreset(tasks, opts.Preset)
		if presetFilterErr != nil {
			return presetFilterErr
		}
		if len(tasks) == 0 {
			return fmt.Errorf("no tasks selected after --preset=%s", opts.Preset)
		}
	}
	if opts.MaxTasks > 0 && opts.MaxTasks < len(tasks) {
		tasks = tasks[:opts.MaxTasks]
	}
	if opts.ToolsFile == "" {
		if problems := validateTaskFixtureAgainstRoutes(tasks, routes); len(problems) > 0 {
			return fmt.Errorf("fixture route validation failed:\n- %s", strings.Join(problems, "\n- "))
		}
	}

	if opts.DryRun {
		toolNames := catalogToolNames(catalog)
		results := make([]taskResult, 0, len(tasks)*opts.Repeat)
		for runIndex := 1; runIndex <= opts.Repeat; runIndex++ {
			results = append(results, runStaticValidation(tasks, routes, toolNames, runIndex)...)
		}
		if err := writeReport(opts.Output, opts, results, catalog, routes, true); err != nil {
			return err
		}
		finalReportWritten = true
		return writeCoverageReportIfRequested(opts, results, routes)
	}

	var mcpSession *mcp.ClientSession
	var executionClient *gitlabclient.Client
	var bridgeSupport mcpBridgeSupport
	if opts.Execute {
		session, client, closeSession, execErr := newExecutionSession(opts)
		if execErr != nil {
			return execErr
		}
		defer closeSession()
		mcpSession = session
		executionClient = client
	}
	if opts.ExposeResources && mcpSession == nil && opts.ToolsFile == "" {
		session, closeSession, resourceErr := newResourceLookupSession(opts)
		if resourceErr != nil {
			return resourceErr
		}
		defer closeSession()
		mcpSession = session
	}
	if opts.ExposeResources && mcpSession != nil {
		bridgeSupport = probeCapabilityBridgeSupport(mcpSession)
		if bridgeSupport.any() {
			catalog = appendCapabilityBridgeTools(catalog, bridgeSupport)
			opts.CapabilityAccessActive = bridgeSupport.Capabilities
			opts.ResourceAccessActive = bridgeSupport.Resources
			opts.PromptAccessActive = bridgeSupport.Prompts
			opts.CompletionAccessActive = bridgeSupport.Completion
		}
	}

	ctx := context.Background()
	results := make([]taskResult, 0, len(tasks)*opts.Repeat*len(modelSpecs))
	liveAttemptRunSuffix := liveUniqueSuffix()
	for _, spec := range modelSpecs {
		apiKey, keyErr := apiKeyForModelProvider(spec.Provider)
		if keyErr != nil {
			return keyErr
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
			mcpSession:  mcpSession,
			mcpBridge:   bridgeSupport,
			traceBodies: opts.TraceProviderBodies,
		}
		for runIndex := 1; runIndex <= opts.Repeat; runIndex++ {
			if opts.Execute && opts.UseFixtures {
				if err := ensureLiveProjectActive(ctx, executionClient); err != nil {
					return err
				}
			}
			for _, task := range tasks {
				taskForAttempt := task
				if opts.Execute && opts.UseFixtures {
					taskForAttempt = addLiveAttemptResourceSuffix(taskForAttempt, spec.String(), runIndex, liveAttemptRunSuffix)
					var err error
					taskForAttempt, err = ensureLiveAttemptResources(ctx, executionClient, mcpSession, taskForAttempt, opts.ToolSurface)
					if err != nil {
						return err
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

	if writeErr := writeReport(opts.Output, opts, results, catalog, routes, false); writeErr != nil {
		return writeErr
	}
	finalReportWritten = true
	if err := writeCoverageReportIfRequested(opts, results, routes); err != nil {
		return err
	}
	return writeTraceArtifacts(opts.TraceDir, results, opts.TraceProviderBodies)
}

// parseFlags parses flags from evaluator input.
